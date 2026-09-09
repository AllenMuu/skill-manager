package diagnostic_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/diagnostic"
	"github.com/AllenMuu/skill-manager/internal/operation"
)

func TestScanReportsCatalogOrphanUnsupportedAndGitGuidance(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	mustMkdir(t, filepath.Join(library, "invalid"))
	mustMkdir(t, filepath.Join(project, ".codex", "skills"))
	mustLink(t, filepath.Join(library, "missing"), filepath.Join(project, ".codex", "skills", "missing"))
	mustMkdir(t, filepath.Join(project, ".cursor", "skills", "other"))
	mustWrite(t, filepath.Join(project, ".git", "index"), "")

	findings, err := diagnostic.Scan(library, project)
	if err != nil {
		t.Fatal(err)
	}
	joined := findingsText(findings)
	for _, want := range []string{"invalid catalog entry", "orphaned managed link", "unsupported agent", "Git"} {
		if !strings.Contains(joined, want) {
			t.Errorf("findings %q do not contain %q", joined, want)
		}
	}
}

func TestScanReportsTrackedIgnoredAndUntrackedManagedLinks(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	for _, id := range []string{"tracked", "ignored", "untracked"} {
		mustWrite(t, filepath.Join(lib, id, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
		mustLink(t, filepath.Join(lib, id), filepath.Join(project, ".codex", "skills", id))
	}
	runGit(t, project, "init")
	runGit(t, project, "config", "user.email", "test@example.com")
	runGit(t, project, "config", "user.name", "Test")
	mustWrite(t, filepath.Join(project, ".gitignore"), "/.codex/skills/ignored\n")
	runGit(t, project, "add", ".codex/skills/tracked")
	runGit(t, project, "commit", "-m", "fixture")
	findings, err := diagnostic.Scan(lib, project)
	if err != nil {
		t.Fatal(err)
	}
	text := findingsText(findings)
	for _, want := range []string{"tracked", "ignored", "would be tracked"} {
		if !strings.Contains(text, want) {
			t.Fatalf("findings=%q missing %q", text, want)
		}
	}
}

func TestScanReportsAdapterCapabilitiesAndUnmanagedResourcesReadOnly(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	path := filepath.Join(project, ".pi", "skills", "local")
	mustMkdir(t, library)
	mustWrite(t, filepath.Join(path, "SKILL.md"), "---\nname: local\ndescription: local\n---\n")
	before, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	findings, err := diagnostic.Scan(library, project)
	if err != nil {
		t.Fatal(err)
	}
	text := findingsText(findings)
	for _, want := range []string{"adapter pi", "filesystem-write", "unmanaged resource"} {
		if !strings.Contains(text, want) {
			t.Errorf("findings %q do not contain %q", text, want)
		}
	}
	var foundPath bool
	for _, finding := range findings {
		if finding.Path == path {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Errorf("findings do not include unmanaged path %q: %#v", path, findings)
	}
	after, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("doctor mutated unmanaged resource")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestAddGitignorePreservesNewlineAndRejectsSymlink(t *testing.T) {
	project := t.TempDir()
	link := filepath.Join(project, ".codex", "skills", "demo")
	target := filepath.Join(t.TempDir(), "demo")
	mustWrite(t, filepath.Join(target, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
	mustLink(t, target, link)
	ignore := filepath.Join(project, ".gitignore")
	mustWrite(t, ignore, "existing")
	if _, err := diagnostic.AddGitignore(project, []string{link}, func(operation.Plan) bool { return true }, operation.New(filepath.Join(project, "j.json"))); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "existing\n/.codex/skills/demo\n" {
		t.Fatalf("ignore=%q", contents)
	}
	project = t.TempDir()
	ignore = filepath.Join(project, ".gitignore")
	mustLink(t, filepath.Join(project, "target"), ignore)
	if _, err := diagnostic.AddGitignore(project, []string{filepath.Join(project, ".codex", "skills", "demo")}, func(operation.Plan) bool { return true }); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestAddGitignoreRejectsUnmanagedAndSkipsExistingRule(t *testing.T) {
	project := t.TempDir()
	unmanaged := filepath.Join(project, ".codex", "skills", "local")
	mustMkdir(t, unmanaged)
	if _, err := diagnostic.AddGitignore(project, []string{unmanaged}, func(operation.Plan) bool { return true }); err == nil {
		t.Fatal("unmanaged accepted")
	}
	link := filepath.Join(project, ".codex", "skills", "demo")
	target := filepath.Join(t.TempDir(), "demo")
	mustWrite(t, filepath.Join(target, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
	mustLink(t, target, link)
	mustWrite(t, filepath.Join(project, ".gitignore"), "/.codex/skills/demo\n")
	called := false
	plan, err := diagnostic.AddGitignore(project, []string{link}, func(operation.Plan) bool { called = true; return true })
	if err != nil || called || len(plan.Changes) != 0 {
		t.Fatalf("plan=%#v called=%v err=%v", plan, called, err)
	}
}

func TestDeleteRevalidatesConcurrentReplacementAndCatalogIO(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "library")
	path := filepath.Join(lib, "demo")
	mustWrite(t, filepath.Join(path, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
	_, err := diagnostic.DeleteLibrarySkill(lib, "demo", true, func(operation.Plan) bool {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		mustMkdir(t, path)
		return true
	})
	if err == nil {
		t.Fatal("concurrent replacement accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.DeleteLibrarySkill(filepath.Join(root, "missing", "library"), "demo", true, func(operation.Plan) bool { return true }); err == nil {
		t.Fatal("catalog I/O accepted")
	}
}

func TestReconcileRelinksOrphanOnlyAfterConfirmation(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "new-library")
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(library, "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\n")
	link := filepath.Join(project, ".codex", "skills", "demo")
	old := filepath.Join(root, "old-library", "demo")
	mustWrite(t, filepath.Join(old, "SKILL.md"), "---\nname: demo\ndescription: demo\n---\n")
	mustLink(t, old, link)
	journal := operation.New(filepath.Join(root, "journal.json"))
	after, err := journal.Capture([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Record("activate", nil, after); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(old)); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.Reconcile(library, project, journal, func(operation.Plan) bool { return false }); !errors.Is(err, operation.ErrNotConfirmed) {
		t.Fatalf("Reconcile() = %v", err)
	}
	if got, _ := os.Readlink(link); !strings.Contains(got, "old-library") {
		t.Fatalf("link changed without confirmation: %q", got)
	}
	if _, err := diagnostic.Reconcile(library, project, journal, func(operation.Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != filepath.Join(library, "demo") {
		t.Fatalf("link = %q", got)
	}
}

func TestScanAndReconcileLeaveAmbiguousDanglingLinkUntouched(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(library, "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\n")
	link := filepath.Join(project, ".codex", "skills", "demo")
	mustLink(t, filepath.Join(root, "unrelated", "demo"), link)
	findings, err := diagnostic.Scan(library, project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(findingsText(findings), "ambiguous dangling link") {
		t.Fatalf("findings = %q", findingsText(findings))
	}
	plan, err := diagnostic.Reconcile(library, project, operation.New(filepath.Join(project, ".skill-manager", "journal.json")), func(operation.Plan) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("ambiguous link was planned: %#v", plan)
	}
}

func TestScanRecognizesJournalOwnedMovedLibraryLink(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old", "demo")
	lib := filepath.Join(root, "new")
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(old, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
	mustWrite(t, filepath.Join(lib, "demo", "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
	link := filepath.Join(project, ".codex", "skills", "demo")
	mustLink(t, old, link)
	j := operation.New(filepath.Join(root, "journal.json"))
	after, err := j.Capture([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Record("activate", nil, after); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(old)); err != nil {
		t.Fatal(err)
	}
	findings, err := diagnostic.Scan(lib, project, j)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(findingsText(findings), "orphaned managed link") {
		t.Fatalf("findings=%q", findingsText(findings))
	}
}

func TestReconcileSecondPublicationFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	j := operation.New(filepath.Join(root, "j.json"))
	var links []string
	for _, id := range []string{"a", "b"} {
		mustWrite(t, filepath.Join(lib, id, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
		old := filepath.Join(root, "old", id)
		mustWrite(t, filepath.Join(old, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
		link := filepath.Join(project, ".codex", "skills", id)
		mustLink(t, old, link)
		links = append(links, link)
	}
	after, captureErr := j.Capture(links)
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	if err := j.Record("activate", nil, after); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	diagnostic.BeforeReconcilePublish = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("second")
		}
		return nil
	}
	defer func() { diagnostic.BeforeReconcilePublish = nil }()
	_, err := diagnostic.Reconcile(lib, project, j, func(operation.Plan) bool { return true })
	if err == nil {
		t.Fatal("expected failure")
	}
	for _, id := range []string{"a", "b"} {
		target, _ := os.Readlink(filepath.Join(project, ".codex", "skills", id))
		if target != filepath.Join(root, "old", id) {
			t.Fatalf("%s not rolled back: %q", id, target)
		}
	}
}

func TestReconcileDetectsLaterLinkReplacementAndRestoresPreState(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "library")
	project := filepath.Join(root, "project")
	j := operation.New(filepath.Join(root, "j.json"))
	var links []string
	for _, id := range []string{"a", "b"} {
		mustWrite(t, filepath.Join(lib, id, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
		old := filepath.Join(root, "old", id)
		mustWrite(t, filepath.Join(old, "SKILL.md"), "---\nname: d\ndescription: d\n---\n")
		link := filepath.Join(project, ".codex", "skills", id)
		mustLink(t, old, link)
		links = append(links, link)
	}
	after, _ := j.Capture(links)
	if err := j.Record("activate", nil, after); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	diagnostic.BeforeReconcilePublish = func(path string) error {
		calls++
		if calls == 2 {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(root, "concurrent", "b"), path)
		}
		return nil
	}
	defer func() { diagnostic.BeforeReconcilePublish = nil }()
	if _, err := diagnostic.Reconcile(lib, project, j, func(operation.Plan) bool { return true }); err == nil {
		t.Fatal("replacement accepted")
	}
	target, _ := os.Readlink(filepath.Join(project, ".codex", "skills", "a"))
	if target != filepath.Join(root, "old", "a") {
		t.Fatalf("a=%q", target)
	}
	target, _ = os.Readlink(filepath.Join(project, ".codex", "skills", "b"))
	if target != filepath.Join(root, "concurrent", "b") {
		t.Fatalf("concurrent b was overwritten: %q", target)
	}
}

func TestDeleteLibrarySkillRequiresForceConfirmation(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	mustWrite(t, filepath.Join(library, "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\n")
	if _, err := diagnostic.DeleteLibrarySkill(library, "demo", false, func(operation.Plan) bool { return true }); err == nil {
		t.Fatal("DeleteLibrarySkill succeeded without force")
	}
	if _, err := diagnostic.DeleteLibrarySkill(library, "demo", true, func(operation.Plan) bool { return false }); !errors.Is(err, operation.ErrNotConfirmed) {
		t.Fatalf("DeleteLibrarySkill = %v", err)
	}
	if _, err := diagnostic.DeleteLibrarySkill(library, "demo", true, func(operation.Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(library, "demo")); !os.IsNotExist(err) {
		t.Fatalf("library skill remains: %v", err)
	}
}

func findingsText(findings []diagnostic.Finding) string {
	var text []string
	for _, f := range findings {
		text = append(text, f.Message)
	}
	return strings.Join(text, "\n")
}
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
func mustLink(t *testing.T, target, path string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
