// Package lifecycle manages project-local skill links without interpreting skill content.
package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/operation"
)

var (
	ErrNotConfirmed   = operation.ErrNotConfirmed
	ErrUnsafePath     = errors.New("refusing unmanaged or unexpected path")
	ErrConflict       = errors.New("operation conflicts with existing skill")
	errConcurrentEdit = errors.New("project skill changed while operation was staged")
)

// Status identifies how a project skill is owned.
type Status string

const (
	Managed   Status = "managed"
	Unmanaged Status = "unmanaged"
	Orphaned  Status = "orphaned"
)

// Item is an entry in a project skill inventory.
type Item struct {
	Target     adapter.Target
	Identifier string
	Path       string
	SourcePath string
	Status     Status
}

// ConfirmFunc displays a plan and returns whether the user confirmed it.
type ConfirmFunc func(operation.Plan) bool

// Service performs guarded lifecycle changes against one local skill library.
type Service struct {
	LibraryPath string
	Journal     *operation.Journal
	// BeforePublish is an optional test seam invoked before each staged publish.
	BeforePublish func(string) error
	confirm       ConfirmFunc
}

// New constructs a lifecycle service. A nil confirmation function declines mutations.
func New(libraryPath string, journal *operation.Journal, confirm ConfirmFunc) *Service {
	abs, err := filepath.Abs(libraryPath)
	if err == nil {
		libraryPath = abs
	}
	return &Service{LibraryPath: libraryPath, Journal: journal, confirm: confirm}
}

// Add activates skill for each explicitly selected target using absolute soft links.
func (s *Service) Add(project string, skill catalog.Skill, targets []adapter.Target) (operation.Plan, error) {
	if err := adapter.ValidateIdentifier(skill.Identifier); err != nil {
		return operation.Plan{}, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	project, source, err := absolute(project, skill.SourcePath)
	if err != nil {
		return operation.Plan{}, err
	}
	if source != filepath.Join(s.LibraryPath, skill.Identifier) {
		return operation.Plan{}, fmt.Errorf("%w: skill source is not configured library entry", ErrUnsafePath)
	}
	eligible, eligibilityErr := s.eligibleConfiguredSkill(skill.Identifier, source)
	if eligibilityErr != nil {
		return operation.Plan{}, errors.Join(ErrUnsafePath, eligibilityErr)
	}
	if !eligible {
		return operation.Plan{}, fmt.Errorf("%w: library source is missing or not an eligible directory skill", ErrUnsafePath)
	}
	plan := operation.Plan{Operation: "activate"}
	paths := make([]string, 0, len(targets))
	seen := map[adapter.Target]bool{}
	for _, target := range targets {
		if seen[target] {
			return plan, fmt.Errorf("%w: duplicate target %q", ErrUnsafePath, target)
		}
		seen[target] = true
		a, ok := adapter.For(target)
		if !ok {
			return plan, fmt.Errorf("unsupported target %q", target)
		}
		path := a.ProjectSkillPath(project, skill.Identifier)
		if contains(skill.Compatibility, string(target)) == false {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s is not declared compatible with %s", skill.Identifier, target))
		}
		if err := s.safeNewLink(path, source); err != nil {
			return plan, err
		}
		plan.Changes = append(plan.Changes, operation.Change{Path: path, Action: "create absolute link", Detail: source})
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return plan, nil
	}
	if !s.confirmed(plan) {
		return plan, ErrNotConfirmed
	}
	return plan, s.mutate("activate", paths, func() error {
		eligible, err := s.eligibleConfiguredSkill(skill.Identifier, source)
		if err != nil {
			return errors.Join(ErrUnsafePath, err)
		}
		if !eligible {
			return fmt.Errorf("%w: library source changed after confirmation", ErrUnsafePath)
		}
		for _, path := range paths {
			if err := s.safeNewLink(path, source); err != nil {
				return err
			}
		}
		return nil
	}, func() error {
		for _, path := range paths {
			if _, err := os.Lstat(path); err == nil {
				continue
			}
			if err := stagedLink(path, source); err != nil {
				return err
			}
		}
		return nil
	})
}

// Activate is an alias for Add.
func (s *Service) Activate(project string, skill catalog.Skill, targets []adapter.Target) (operation.Plan, error) {
	return s.Add(project, skill, targets)
}

// AddMany activates an explicit selection as one confirmed, journaled transaction.
func (s *Service) AddMany(project string, skills []catalog.Skill, targets []adapter.Target) (operation.Plan, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return operation.Plan{}, err
	}
	plan := operation.Plan{Operation: "activate selected skills"}
	var paths, sources []string
	seen := map[string]bool{}
	for _, skill := range skills {
		if err := adapter.ValidateIdentifier(skill.Identifier); err != nil {
			return plan, errors.Join(ErrUnsafePath, err)
		}
		source, err := filepath.Abs(skill.SourcePath)
		if err != nil {
			return plan, err
		}
		if source != filepath.Join(s.LibraryPath, skill.Identifier) {
			return plan, ErrUnsafePath
		}
		ok, err := s.eligibleConfiguredSkill(skill.Identifier, source)
		if err != nil {
			return plan, err
		}
		if !ok {
			return plan, ErrUnsafePath
		}
		for _, target := range targets {
			a, ok := adapter.For(target)
			if !ok {
				return plan, fmt.Errorf("unsupported target %q", target)
			}
			path := a.ProjectSkillPath(project, skill.Identifier)
			if seen[path] {
				return plan, ErrUnsafePath
			}
			seen[path] = true
			if err := s.safeNewLink(path, source); err != nil {
				return plan, err
			}
			if !contains(skill.Compatibility, string(target)) {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s is not declared compatible with %s", skill.Identifier, target))
			}
			paths = append(paths, path)
			sources = append(sources, source)
			plan.Changes = append(plan.Changes, operation.Change{Path: path, Action: "create absolute link", Detail: source})
		}
	}
	if len(paths) == 0 {
		return plan, nil
	}
	if !s.confirmed(plan) {
		return plan, ErrNotConfirmed
	}
	before, err := s.Journal.Capture(paths)
	if err != nil {
		return plan, err
	}
	published := []int{}
	rollback := func() error {
		snapshots := []operation.Snapshot{}
		for _, i := range published {
			snapshots = append(snapshots, before[i])
		}
		return s.Journal.Restore(snapshots)
	}
	for i, path := range paths {
		if err := s.safeNewLink(path, sources[i]); err != nil {
			return plan, errors.Join(err, rollback())
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			if err := stagedLink(path, sources[i]); err != nil {
				return plan, errors.Join(err, rollback())
			}
			published = append(published, i)
			if s.BeforePublish != nil {
				if err := s.BeforePublish("add-many-published"); err != nil {
					return plan, errors.Join(err, rollback())
				}
			}
		}
	}
	after, err := s.Journal.Capture(paths)
	if err != nil {
		return plan, errors.Join(err, s.Journal.Restore(before))
	}
	if err := s.Journal.Record("activate selected skills", before, after); err != nil {
		if errors.Is(err, operation.ErrJournalCommitted) {
			return plan, err
		}
		return plan, errors.Join(err, s.Journal.Restore(before))
	}
	return plan, nil
}

// List inventories supported project skill locations without changing them.
func (s *Service) List(project string) ([]Item, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	var items []Item
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
			item := Item{Target: a.Target(), Identifier: entry.Name(), Path: path, Status: Unmanaged}
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				destination, err := os.Readlink(path)
				if err != nil {
					return nil, err
				}
				item.SourcePath = destination
				if destination == filepath.Join(s.LibraryPath, entry.Name()) {
					if _, err := os.Stat(destination); os.IsNotExist(err) {
						item.Status = Orphaned
					} else if err == nil {
						item.Status = Managed
					} else {
						return nil, fmt.Errorf("inspect managed library target %s: %w", destination, err)
					}
				}
			}
			items = append(items, item)
		}
	}
	return items, nil
}

// Remove removes only a managed project-side soft link.
func (s *Service) Remove(project string, target adapter.Target, identifier string) (operation.Plan, error) {
	path, err := s.skillPath(project, target, identifier)
	if err != nil {
		return operation.Plan{}, err
	}
	managed, err := s.managedLink(path, identifier)
	if err != nil {
		return operation.Plan{}, err
	}
	if !managed {
		return operation.Plan{}, ErrUnsafePath
	}
	plan := operation.Plan{Operation: "remove", Changes: []operation.Change{{Path: path, Action: "remove managed link"}}}
	if !s.confirmed(plan) {
		return plan, ErrNotConfirmed
	}
	return plan, s.mutate("remove", []string{path}, func() error {
		managed, err := s.managedLink(path, identifier)
		if err != nil {
			return err
		}
		if !managed {
			return ErrUnsafePath
		}
		return nil
	}, func() error {
		managed, err := s.managedLink(path, identifier)
		if err != nil {
			return err
		}
		if !managed {
			return ErrUnsafePath
		}
		return os.Remove(path)
	})
}

// Adopt moves an eligible unmanaged project directory into the library and links it back.
func (s *Service) Adopt(project string, target adapter.Target, identifier string) (operation.Plan, error) {
	path, err := s.skillPath(project, target, identifier)
	if err != nil {
		return operation.Plan{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return operation.Plan{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return operation.Plan{}, ErrUnsafePath
	}
	skills, _, err := catalog.Discover(filepath.Dir(path))
	if err != nil {
		return operation.Plan{}, err
	}
	var found bool
	for _, skill := range skills {
		if skill.Identifier == identifier {
			found = true
		}
	}
	if !found {
		return operation.Plan{}, fmt.Errorf("%w: project directory is not an eligible skill", ErrUnsafePath)
	}
	libraryPath := filepath.Join(s.LibraryPath, identifier)
	if _, err := os.Lstat(libraryPath); err == nil {
		return operation.Plan{}, ErrConflict
	} else if !os.IsNotExist(err) {
		return operation.Plan{}, err
	}
	plan := operation.Plan{Operation: "adopt", Changes: []operation.Change{{Path: libraryPath, Action: "copy project skill into library", Detail: path}, {Path: path, Action: "replace directory with managed link", Detail: libraryPath}}}
	if !s.confirmed(plan) {
		return plan, ErrNotConfirmed
	}
	return plan, s.mutate("adopt", []string{path, libraryPath}, func() error {
		adoptable, err := s.adoptable(path, identifier)
		if err != nil {
			return err
		}
		if !adoptable {
			return ErrUnsafePath
		}
		if _, err := os.Lstat(libraryPath); !os.IsNotExist(err) {
			if err == nil {
				return ErrConflict
			}
			return err
		}
		return nil
	}, func() error {
		adoptable, err := s.adoptable(path, identifier)
		if err != nil {
			return err
		}
		if !adoptable {
			return ErrUnsafePath
		}
		if _, err := os.Lstat(libraryPath); err == nil {
			return ErrConflict
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := stagedCopy(path, libraryPath); err != nil {
			return err
		}
		if s.BeforePublish != nil {
			if err := s.BeforePublish("adopt-staged"); err != nil {
				return err
			}
		}
		equal, err := sameTree(path, libraryPath)
		if err != nil {
			return errors.Join(ErrUnsafePath, err)
		}
		if !equal {
			if err := os.RemoveAll(libraryPath); err != nil {
				return errors.Join(ErrUnsafePath, errConcurrentEdit, err)
			}
			return errors.Join(ErrUnsafePath, errConcurrentEdit)
		}
		hook := func(step string) error {
			if s.BeforePublish != nil {
				return s.BeforePublish(step)
			}
			return nil
		}
		if err := stagedDirectoryLink(path, libraryPath, hook); err != nil {
			if errors.Is(err, errConcurrentEdit) {
				cleanupErr := os.RemoveAll(libraryPath)
				return errors.Join(ErrUnsafePath, err, cleanupErr)
			}
			return err
		}
		return nil
	})
}

// Fork replaces a managed link with an independent project-local copy.
func (s *Service) Fork(project string, target adapter.Target, identifier string) (operation.Plan, error) {
	return s.ForkMany(project, identifier, []adapter.Target{target})
}

// ForkMany forks selected managed links as one guarded transaction.
func (s *Service) ForkMany(project, identifier string, targets []adapter.Target) (operation.Plan, error) {
	if err := adapter.ValidateIdentifier(identifier); err != nil {
		return operation.Plan{}, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	plan := operation.Plan{Operation: "fork"}
	paths := make([]string, 0, len(targets))
	sources := make([]string, 0, len(targets))
	seen := map[adapter.Target]bool{}
	for _, target := range targets {
		if seen[target] {
			return plan, fmt.Errorf("%w: duplicate target %q", ErrUnsafePath, target)
		}
		seen[target] = true
		path, err := s.skillPath(project, target, identifier)
		if err != nil {
			return plan, err
		}
		managed, err := s.managedLink(path, identifier)
		if err != nil {
			return plan, err
		}
		if !managed {
			return plan, ErrUnsafePath
		}
		source, err := os.Readlink(path)
		if err != nil {
			return plan, err
		}
		plan.Changes = append(plan.Changes, operation.Change{Path: path, Action: "replace managed link with independent copy", Detail: source})
		paths, sources = append(paths, path), append(sources, source)
	}
	if len(paths) == 0 {
		return plan, nil
	}
	if !s.confirmed(plan) {
		return plan, ErrNotConfirmed
	}
	return plan, s.mutate("fork", paths, func() error {
		for i, path := range paths {
			managed, err := s.managedLink(path, identifier)
			if err != nil {
				return err
			}
			if !managed {
				return ErrUnsafePath
			}
			if target, err := os.Readlink(path); err != nil || target != sources[i] {
				return ErrUnsafePath
			}
		}
		return nil
	}, func() error {
		candidates := make([]stagedCandidate, len(paths))
		for i := range paths {
			candidate, err := prepareCopy(sources[i], paths[i])
			if err != nil {
				for _, prepared := range candidates {
					_ = os.RemoveAll(prepared.root)
				}
				return err
			}
			candidates[i] = candidate
		}
		defer func() {
			for _, candidate := range candidates {
				_ = os.RemoveAll(candidate.root)
			}
		}()
		for i, path := range paths {
			if err := os.Remove(path); err != nil {
				return err
			}
			if err := publishCopy(candidates[i], path); err != nil {
				return err
			}
			if s.BeforePublish != nil {
				if err := s.BeforePublish("fork-published"); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Undo restores the pre-operation state of the latest confirmed operation.
func (s *Service) Undo() error {
	if s.Journal == nil {
		return fmt.Errorf("operation journal is not configured")
	}
	return s.Journal.UndoLatest(s.confirm)
}

func (s *Service) skillPath(project string, target adapter.Target, identifier string) (string, error) {
	if err := adapter.ValidateIdentifier(identifier); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	project, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	a, ok := adapter.For(target)
	if !ok {
		return "", fmt.Errorf("unsupported target %q", target)
	}
	return a.ProjectSkillPath(project, identifier), nil
}
func (s *Service) safeNewLink(path, source string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err == nil && target == source {
			return nil
		}
	}
	return ErrUnsafePath
}
func (s *Service) managedLink(path, identifier string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect managed link %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("read managed link %s: %w", path, err)
	}
	return target == filepath.Join(s.LibraryPath, identifier), nil
}
func (s *Service) confirmed(plan operation.Plan) bool { return s.confirm != nil && s.confirm(plan) }
func (s *Service) mutate(name string, paths []string, validate func() error, change func() error) error {
	if s.Journal == nil {
		return fmt.Errorf("operation journal is not configured")
	}
	if err := validate(); err != nil {
		return err
	}
	before, err := s.Journal.Capture(paths)
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		if errors.Is(err, errConcurrentEdit) {
			return err
		}
		return rollbackError(err, s.Journal.Restore(before))
	}
	after, err := s.Journal.Capture(paths)
	if err != nil {
		return rollbackError(err, s.Journal.Restore(before))
	}
	if err := s.Journal.Record(name, before, after); err != nil {
		if errors.Is(err, operation.ErrJournalCommitted) {
			return err
		}
		return rollbackError(err, s.Journal.Restore(before))
	}
	return nil
}

func rollbackError(err, rollback error) error {
	if rollback != nil {
		return errors.Join(err, rollback)
	}
	return err
}
func (s *Service) eligibleConfiguredSkill(identifier, source string) (bool, error) {
	info, err := os.Lstat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return false, err
		}
		return false, fmt.Errorf("inspect skill source %s: %w", source, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	skills, _, err := catalog.Discover(filepath.Dir(source))
	if err != nil {
		return false, fmt.Errorf("discover skill source %s: %w", source, err)
	}
	for _, skill := range skills {
		if skill.Identifier == identifier && skill.SourcePath == source {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) adoptable(path, identifier string) (bool, error) {
	return s.eligibleConfiguredSkill(identifier, path)
}
func absolute(project, source string) (string, string, error) {
	p, err := filepath.Abs(project)
	if err != nil {
		return "", "", err
	}
	s, err := filepath.Abs(source)
	return p, s, err
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func copyTree(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.Symlink(target, destination)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported skill file type at %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func sameTree(left, right string) (bool, error) {
	li, err := os.Lstat(left)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", left, err)
	}
	ri, err := os.Lstat(right)
	if err != nil || li.Mode() != ri.Mode() {
		if err != nil {
			return false, fmt.Errorf("inspect %s: %w", right, err)
		}
		return false, nil
	}
	if li.Mode()&os.ModeSymlink != 0 {
		l, e1 := os.Readlink(left)
		r, e2 := os.Readlink(right)
		if e1 != nil {
			return false, e1
		}
		if e2 != nil {
			return false, e2
		}
		return l == r, nil
	}
	if li.IsDir() {
		le, e1 := os.ReadDir(left)
		re, e2 := os.ReadDir(right)
		if e1 != nil || e2 != nil || len(le) != len(re) {
			if e1 != nil {
				return false, e1
			}
			if e2 != nil {
				return false, e2
			}
			return false, nil
		}
		for i := range le {
			if le[i].Name() != re[i].Name() {
				return false, nil
			}
			equal, err := sameTree(filepath.Join(left, le[i].Name()), filepath.Join(right, re[i].Name()))
			if err != nil {
				return false, err
			}
			if !equal {
				return false, nil
			}
		}
		return true, nil
	}
	if !li.Mode().IsRegular() {
		return false, nil
	}
	l, e1 := os.ReadFile(left)
	r, e2 := os.ReadFile(right)
	if e1 != nil {
		return false, e1
	}
	if e2 != nil {
		return false, e2
	}
	return bytes.Equal(l, r), nil
}

// stagedLink creates a sibling temporary link and atomically publishes it.
func stagedLink(destination, source string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	// Symlink creation is an atomic no-replace publication primitive.
	if err := os.Symlink(source, destination); err != nil {
		return fmt.Errorf("publish managed link: %w", err)
	}
	return nil
}

// stagedCopy copies into a sibling temporary directory before atomically replacing destination.
func stagedCopy(source, destination string) error {
	candidate, err := prepareCopy(source, destination)
	if err != nil {
		return err
	}
	defer os.RemoveAll(candidate.root)
	return publishCopy(candidate, destination)
}

type stagedCandidate struct{ root, path string }

func prepareCopy(source, destination string) (stagedCandidate, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return stagedCandidate{}, err
	}
	root, err := os.MkdirTemp(filepath.Dir(destination), ".skill-manager-stage-")
	if err != nil {
		return stagedCandidate{}, err
	}
	candidate := stagedCandidate{root: root, path: filepath.Join(root, "skill")}
	if err := copyTree(source, candidate.path); err != nil {
		_ = os.RemoveAll(root)
		return stagedCandidate{}, err
	}
	return candidate, nil
}
func publishCopy(candidate stagedCandidate, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return ErrUnsafePath
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(candidate.path, destination)
}

// stagedDirectoryLink preserves an existing directory until a staged link is ready.
func stagedDirectoryLink(destination, source string, hook func(string) error) error {
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".skill-manager-stage-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	link, previous := filepath.Join(stage, "link"), filepath.Join(stage, "previous")
	if err := os.Symlink(source, link); err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafePath
	}
	if err := os.Rename(destination, previous); err != nil {
		return err
	}
	if hook != nil {
		if err := hook("adopt-moved"); err != nil {
			cleanup = false
			preserveErr := preserveMovedOriginal(destination, previous, err, hook)
			if _, statErr := os.Lstat(previous); os.IsNotExist(statErr) {
				cleanup = true
			}
			return preserveErr
		}
	}
	if _, err := os.Lstat(destination); err == nil {
		cleanup = false
		preserveErr := preserveMovedOriginal(destination, previous, nil, hook)
		if _, statErr := os.Lstat(previous); os.IsNotExist(statErr) {
			cleanup = true
		}
		return preserveErr
	} else if !os.IsNotExist(err) {
		cleanup = false
		preserveErr := preserveMovedOriginal(destination, previous, err, hook)
		if _, statErr := os.Lstat(previous); os.IsNotExist(statErr) {
			cleanup = true
		}
		return preserveErr
	}
	equal, err := sameTree(previous, source)
	if err != nil || !equal {
		cleanup = false
		preserveErr := preserveMovedOriginal(destination, previous, err, hook)
		if _, statErr := os.Lstat(previous); os.IsNotExist(statErr) {
			cleanup = true
		}
		return preserveErr
	}
	if err := os.Rename(link, destination); err != nil {
		if rollbackErr := os.Rename(previous, destination); rollbackErr != nil {
			return fmt.Errorf("publish staged link: %w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	return nil
}

// preserveMovedOriginal restores the original when its destination is still free;
// otherwise it moves it to an explicit sibling recovery path without deleting either tree.
func preserveMovedOriginal(destination, previous string, cause error, hook func(string) error) error {
	if _, err := os.Lstat(destination); os.IsNotExist(err) {
		if restoreErr := os.Rename(previous, destination); restoreErr != nil {
			return errors.Join(errConcurrentEdit, cause, restoreErr)
		}
		return errors.Join(errConcurrentEdit, cause)
	}
	recovery := fmt.Sprintf("%s.skill-manager-recovery-%d", destination, time.Now().UnixNano())
	if hook != nil {
		if err := hook("adopt-recovery"); err != nil {
			return errors.Join(errConcurrentEdit, cause, err, fmt.Errorf("original remains at %s", previous))
		}
	}
	if err := os.Rename(previous, recovery); err != nil {
		return errors.Join(errConcurrentEdit, cause, fmt.Errorf("preserve moved original at %s: %w (original remains at %s)", recovery, err, previous))
	}
	return errors.Join(errConcurrentEdit, cause, fmt.Errorf("preserved moved original at %s", recovery))
}
