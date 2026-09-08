// Package diagnostic inspects and repairs machine-local skill links.
package diagnostic

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/operation"
)

// Finding is one non-mutating diagnostic result.
type Finding struct{ Path, Message string }

// BeforeReconcilePublish is an optional fault-injection seam.
var BeforeReconcilePublish func(string) error

// Scan reports catalog problems and project-local integration problems.
func Scan(library, project string, journals ...*operation.Journal) ([]Finding, error) {
	library, err := filepath.Abs(library)
	if err != nil {
		return nil, err
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	_, invalid, err := catalog.Discover(library)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(invalid))
	journal := operation.New(filepath.Join(project, ".skill-manager", "journal.json"))
	if len(journals) > 0 && journals[0] != nil {
		journal = journals[0]
	}
	var managedPaths []string
	for _, d := range invalid {
		findings = append(findings, Finding{d.Path, "invalid catalog entry: " + d.Message})
	}
	for _, a := range adapter.Supported() {
		root := filepath.Dir(a.ProjectSkillPath(project, "placeholder"))
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			target, err := os.Readlink(path)
			if err != nil {
				return nil, err
			}
			if _, err := os.Stat(target); os.IsNotExist(err) {
				recorded, owned, recordErr := journal.RecordedLinkTarget(path)
				if recordErr != nil {
					return nil, recordErr
				}
				if target == filepath.Join(library, entry.Name()) || (owned && recorded == target) {
					findings = append(findings, Finding{path, "orphaned managed link: " + target})
				} else {
					findings = append(findings, Finding{path, "ambiguous dangling link: refusing automatic reconciliation"})
				}
			} else if err != nil {
				return nil, fmt.Errorf("inspect link target %s: %w", target, err)
			}
			if target == filepath.Join(library, entry.Name()) {
				managedPaths = append(managedPaths, path)
			}
		}
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() && len(entry.Name()) > 1 && entry.Name()[0] == '.' && entry.Name() != ".git" && entry.Name() != ".codex" && entry.Name() != ".claude" {
			if _, err := os.Stat(filepath.Join(project, entry.Name(), "skills")); err == nil {
				findings = append(findings, Finding{filepath.Join(project, entry.Name()), "unsupported agent skill location"})
			}
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".git")); err == nil {
		if err := exec.Command("git", "-C", project, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
			findings = append(findings, Finding{project, "Git guidance: do not track managed absolute links; initialize/repair this Git worktree to inspect exact status"})
			return findings, nil
		}
		for _, path := range managedPaths {
			status := "would be tracked"
			rel, err := filepath.Rel(project, path)
			if err != nil {
				return nil, err
			}
			if err := exec.Command("git", "-C", project, "ls-files", "--error-unmatch", "--", rel).Run(); err == nil {
				status = "tracked"
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return nil, fmt.Errorf("inspect Git tracked state: %w", err)
			} else if err := exec.Command("git", "-C", project, "check-ignore", "-q", "--", rel).Run(); err == nil {
				status = "ignored"
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return nil, fmt.Errorf("inspect Git ignore state: %w", err)
			}
			findings = append(findings, Finding{path, "Git guidance: managed absolute link is " + status + "; add this exact managed path to .gitignore after confirmation"})
		}
	}
	return findings, nil
}

// AddGitignore appends only supplied managed project-link paths after confirmation.
func AddGitignore(project string, paths []string, confirm func(operation.Plan) bool, journals ...*operation.Journal) (operation.Plan, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return operation.Plan{}, err
	}
	gitignore := filepath.Join(project, ".gitignore")
	journal := operation.New(filepath.Join(project, ".skill-manager", "journal.json"))
	if len(journals) > 0 && journals[0] != nil {
		journal = journals[0]
	}
	plan := operation.Plan{Operation: "update managed-link Git guidance"}
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return plan, fmt.Errorf("inspect managed link %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return plan, fmt.Errorf("refusing unmanaged path %s", path)
		}
		target, err := os.Readlink(path)
		if err != nil {
			return plan, err
		}
		targetOwned := false
		if ok, err := eligible(target, filepath.Base(path)); err == nil && ok {
			targetOwned = true
		} else if err != nil {
			return plan, err
		}
		if !targetOwned {
			recorded, owned, err := journal.RecordedLinkTarget(path)
			if err != nil {
				return plan, err
			}
			targetOwned = owned && recorded == target
		}
		if !targetOwned {
			return plan, fmt.Errorf("refusing link without configured-library or journal ownership: %s", path)
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return plan, err
		}
		if strings.HasPrefix(rel, "..") {
			return plan, fmt.Errorf("managed path outside project: %s", path)
		}
		slashRel := filepath.ToSlash(rel)
		if !strings.HasPrefix(slashRel, ".codex/skills/") && !strings.HasPrefix(slashRel, ".claude/skills/") {
			return plan, fmt.Errorf("refusing unsupported managed-link placement %s", path)
		}
		lines = append(lines, "/"+filepath.ToSlash(rel))
		plan.Changes = append(plan.Changes, operation.Change{Path: gitignore, Action: "ignore managed link", Detail: "/" + filepath.ToSlash(rel)})
	}
	if existing, err := os.ReadFile(gitignore); err == nil {
		kept := plan.Changes[:0]
		for _, change := range plan.Changes {
			if !strings.Contains("\n"+string(existing)+"\n", "\n"+change.Detail+"\n") {
				kept = append(kept, change)
			}
		}
		plan.Changes = kept
		if len(plan.Changes) == 0 {
			return plan, nil
		}
	} else if !os.IsNotExist(err) {
		return plan, err
	}
	if confirm == nil || !confirm(plan) {
		return plan, operation.ErrNotConfirmed
	}
	if info, err := os.Lstat(gitignore); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return plan, fmt.Errorf("refusing unsafe .gitignore path")
	} else if err != nil && !os.IsNotExist(err) {
		return plan, err
	}
	before, err := journal.Capture([]string{gitignore})
	if err != nil {
		return plan, err
	}
	existing, err := os.ReadFile(gitignore)
	if err != nil && !os.IsNotExist(err) {
		return plan, err
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		existing = append(existing, '\n')
	}
	for _, line := range lines {
		if !strings.Contains("\n"+string(existing)+"\n", "\n"+line+"\n") {
			existing = append(existing, []byte(line+"\n")...)
		}
	}
	stage, err := os.CreateTemp(filepath.Dir(gitignore), ".skill-manager-gitignore-")
	if err != nil {
		return plan, err
	}
	name := stage.Name()
	defer os.Remove(name)
	if _, err := stage.Write(existing); err != nil {
		_ = stage.Close()
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := stage.Close(); err != nil {
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := os.Rename(name, gitignore); err != nil {
		return plan, errors.Join(err, journal.Restore(before))
	}
	after, err := journal.Capture([]string{gitignore})
	if err != nil {
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := journal.Record("update managed-link Git guidance", before, after); err != nil {
		if errors.Is(err, operation.ErrJournalCommitted) {
			return plan, err
		}
		return plan, errors.Join(err, journal.Restore(before))
	}
	return plan, nil
}

// Reconcile repoints orphaned supported-agent links to eligible configured library skills.
func Reconcile(library, project string, journal *operation.Journal, confirm func(operation.Plan) bool) (operation.Plan, error) {
	if journal == nil {
		return operation.Plan{}, errors.New("operation journal is not configured")
	}
	library, err := filepath.Abs(library)
	if err != nil {
		return operation.Plan{}, err
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return operation.Plan{}, err
	}
	plan := operation.Plan{Operation: "reconcile"}
	var paths, targets, originals []string
	for _, a := range adapter.Supported() {
		root := filepath.Dir(a.ProjectSkillPath(project, "placeholder"))
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return plan, err
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return plan, err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			old, err := os.Readlink(path)
			if err != nil {
				return plan, err
			}
			if _, err := os.Stat(old); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return plan, fmt.Errorf("inspect link target %s: %w", old, err)
			}
			recorded, managed, err := journal.RecordedLinkTarget(path)
			if err != nil {
				return plan, err
			}
			if old != filepath.Join(library, entry.Name()) && (!managed || recorded != old) {
				continue
			}
			target := filepath.Join(library, entry.Name())
			ok, err := eligible(target, entry.Name())
			if err != nil {
				return plan, err
			}
			if !ok {
				continue
			}
			plan.Changes = append(plan.Changes, operation.Change{Path: path, Action: "repoint orphaned managed link", Detail: target})
			paths = append(paths, path)
			targets = append(targets, target)
			originals = append(originals, old)
		}
	}
	if len(paths) == 0 {
		return plan, nil
	}
	if confirm == nil || !confirm(plan) {
		return plan, operation.ErrNotConfirmed
	}
	// Preflight every exact source and construct all replacements before the
	// first publication. A later concurrent edit therefore leaves every link
	// unchanged.
	for i, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return plan, operation.ErrUnexpectedState
		}
		old, err := os.Readlink(path)
		if err != nil || old != originals[i] {
			return plan, operation.ErrUnexpectedState
		}
		ok, err := eligible(targets[i], filepath.Base(path))
		if err != nil {
			return plan, err
		}
		if !ok {
			return plan, operation.ErrUnexpectedState
		}
	}
	stages := make([]string, len(paths))
	for i, path := range paths {
		stage, err := os.CreateTemp(filepath.Dir(path), ".skill-manager-reconcile-")
		if err != nil {
			for _, p := range stages {
				if p != "" {
					_ = os.Remove(p)
				}
			}
			return plan, err
		}
		if err := stage.Close(); err != nil {
			return plan, err
		}
		if err := os.Remove(stage.Name()); err != nil {
			return plan, err
		}
		if err := os.Symlink(targets[i], stage.Name()); err != nil {
			return plan, err
		}
		stages[i] = stage.Name()
	}
	defer func() {
		for _, stage := range stages {
			if stage != "" {
				_ = os.Remove(stage)
			}
		}
	}()
	before, err := journal.Capture(paths)
	if err != nil {
		return plan, err
	}
	published := []int{}
	rollback := func() error {
		snapshots := []operation.Snapshot{}
		for _, i := range published {
			snapshots = append(snapshots, before[i])
		}
		return journal.Restore(snapshots)
	}
	for i, path := range paths {
		old, err := os.Readlink(path)
		if err != nil || old != originals[i] {
			return plan, errors.Join(operation.ErrUnexpectedState, rollback())
		}
		if BeforeReconcilePublish != nil {
			if err := BeforeReconcilePublish(path); err != nil {
				return plan, errors.Join(err, rollback())
			}
		}
		// Detect a replacement that raced after the last readlink, before the
		// staged rename can overwrite it.
		latest, err := os.Readlink(path)
		if err != nil || latest != originals[i] {
			return plan, errors.Join(operation.ErrUnexpectedState, rollback())
		}
		if err := os.Rename(stages[i], path); err != nil {
			return plan, errors.Join(err, rollback())
		}
		stages[i] = ""
		published = append(published, i)
	}
	after, err := journal.Capture(paths)
	if err != nil {
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := journal.Record("reconcile", before, after); err != nil {
		if errors.Is(err, operation.ErrJournalCommitted) {
			return plan, err
		}
		return plan, errors.Join(err, journal.Restore(before))
	}
	return plan, nil
}

// DeleteLibrarySkill deletes one eligible library directory only after force and confirmation.
func DeleteLibrarySkill(library, identifier string, force bool, confirm func(operation.Plan) bool, journals ...*operation.Journal) (operation.Plan, error) {
	if !force {
		return operation.Plan{}, errors.New("library deletion requires force confirmation because it can orphan managed links; run doctor first")
	}
	if err := adapter.ValidateIdentifier(identifier); err != nil {
		return operation.Plan{}, err
	}
	library, err := filepath.Abs(library)
	if err != nil {
		return operation.Plan{}, err
	}
	path := filepath.Join(library, identifier)
	ok, err := eligible(path, identifier)
	if err != nil {
		return operation.Plan{}, err
	}
	if !ok {
		return operation.Plan{}, fmt.Errorf("refusing ineligible library skill %q", identifier)
	}
	plan := operation.Plan{Operation: "delete library skill", Changes: []operation.Change{{Path: path, Action: "delete library skill"}}, Warnings: []string{"deletion can orphan managed links; run doctor before and after deletion"}}
	if confirm == nil || !confirm(plan) {
		return plan, operation.ErrNotConfirmed
	}
	// Revalidate after confirmation: an attacker or concurrent process may have
	// replaced the directory with an unmanaged path while the plan was visible.
	ok, err = eligible(path, identifier)
	if err != nil {
		return plan, err
	}
	if !ok {
		return plan, operation.ErrUnexpectedState
	}
	journal := operation.New(filepath.Join(filepath.Dir(library), ".skill-manager", "journal.json"))
	if len(journals) > 0 && journals[0] != nil {
		journal = journals[0]
	}
	before, err := journal.Capture([]string{path})
	if err != nil {
		return plan, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(path), ".skill-manager-delete-")
	if err != nil {
		return plan, err
	}
	defer os.RemoveAll(stage)
	moved := filepath.Join(stage, identifier)
	if err := os.Rename(path, moved); err != nil {
		return plan, err
	}
	after, err := journal.Capture([]string{path})
	if err != nil {
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := journal.Record("delete library skill", before, after); err != nil {
		if errors.Is(err, operation.ErrJournalCommitted) {
			return plan, err
		}
		return plan, errors.Join(err, journal.Restore(before))
	}
	if err := os.RemoveAll(moved); err != nil {
		return plan, err
	}
	return plan, nil
}
func eligible(path, identifier string) (bool, error) {
	skills, _, err := catalog.Discover(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	for _, s := range skills {
		if s.Identifier == identifier && s.SourcePath == path {
			return true, nil
		}
	}
	return false, nil
}
