package lifecycle_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/catalog"
	"github.com/AllenMuu/skill-manager/internal/lifecycle"
	"github.com/AllenMuu/skill-manager/internal/operation"
)

func TestAddCreatesAbsoluteLinksForSelectedTargetsAndWarnsOnCompatibility(t *testing.T) {
	root, project, skill := fixture(t)
	var preview operation.Plan
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(p operation.Plan) bool { preview = p; return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []adapter.Target{adapter.ClaudeCode, adapter.Codex} {
		a, _ := adapter.For(target)
		path := a.ProjectSkillPath(project, skill.Identifier)
		got, err := os.Readlink(path)
		if err != nil || got != skill.SourcePath || !filepath.IsAbs(got) {
			t.Fatalf("%s link = %q, %v", target, got, err)
		}
	}
	if len(preview.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one compatibility warning", preview.Warnings)
	}
}

func TestAddRequiresConfirmationAndRefusesUnmanagedDestination(t *testing.T) {
	root, project, skill := fixture(t)
	a, _ := adapter.For(adapter.Codex)
	path := a.ProjectSkillPath(project, skill.Identifier)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Add error = %v, want unsafe path", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unmanaged path was changed: %v", err)
	}

	otherRoot, otherProject, otherSkill := fixture(t)
	svc = lifecycle.New(filepath.Join(otherRoot, "library"), operation.New(filepath.Join(otherRoot, "journal.json")), func(operation.Plan) bool { return false })
	if _, err := svc.Add(otherProject, otherSkill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrNotConfirmed) {
		t.Fatalf("Add error = %v, want confirmation refusal", err)
	}
	path = a.ProjectSkillPath(otherProject, otherSkill.Identifier)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed path changed: %v", err)
	}
}

func TestAddManyDeclineLeavesNoPartialLinks(t *testing.T) {
	root, project, one := fixture(t)
	two := writeSkill(t, filepath.Join(root, "library", "two"), "two")
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "j.json")), func(operation.Plan) bool { return false })
	if _, err := svc.AddMany(project, []catalog.Skill{one, two}, []adapter.Target{adapter.Codex}); !errors.Is(err, operation.ErrNotConfirmed) {
		t.Fatalf("err=%v", err)
	}
	for _, id := range []string{one.Identifier, two.Identifier} {
		if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", id)); !os.IsNotExist(err) {
			t.Fatalf("%s created", id)
		}
	}
}

func TestAddManySecondPublicationFailureRollsBack(t *testing.T) {
	root, project, one := fixture(t)
	two := writeSkill(t, filepath.Join(root, "library", "two"), "two")
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "j.json")), func(operation.Plan) bool { return true })
	calls := 0
	svc.BeforePublish = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("second")
		}
		return nil
	}
	if _, err := svc.AddMany(project, []catalog.Skill{one, two}, []adapter.Target{adapter.Codex}); err == nil {
		t.Fatal("expected failure")
	}
	for _, id := range []string{one.Identifier, two.Identifier} {
		if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", id)); !os.IsNotExist(err) {
			t.Fatalf("%s remains", id)
		}
	}
}

func TestAddManyAggregatesCompatibilityWarningsBeforeConfirmation(t *testing.T) {
	root, project, one := fixture(t)
	two := writeSkill(t, filepath.Join(root, "library", "two"), "two")
	var plan operation.Plan
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "j.json")), func(p operation.Plan) bool { plan = p; return false })
	if _, err := svc.AddMany(project, []catalog.Skill{one, two}, []adapter.Target{adapter.Codex}); !errors.Is(err, operation.ErrNotConfirmed) {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 2 {
		t.Fatalf("warnings=%#v", plan.Warnings)
	}
}

func TestListRemoveAndUndoKeepLibrary(t *testing.T) {
	root, project, skill := fixture(t)
	journal := operation.New(filepath.Join(root, "journal.json"))
	svc := lifecycle.New(filepath.Join(root, "library"), journal, func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	a, _ := adapter.For(adapter.Codex)
	unmanaged := filepath.Join(filepath.Dir(a.ProjectSkillPath(project, skill.Identifier)), "local")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	items, err := svc.List(project)
	if err != nil {
		t.Fatal(err)
	}
	if !hasStatus(items, skill.Identifier, lifecycle.Managed) || !hasStatus(items, "local", lifecycle.Unmanaged) {
		t.Fatalf("inventory = %#v", items)
	}
	if _, err := svc.Remove(project, adapter.Codex, skill.Identifier); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(a.ProjectSkillPath(project, skill.Identifier)); !os.IsNotExist(err) {
		t.Fatalf("managed link was not removed: %v", err)
	}
	if _, err := os.Stat(skill.SourcePath); err != nil {
		t.Fatalf("library skill removed: %v", err)
	}
	if _, err := svc.Remove(project, adapter.Codex, "local"); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("remove unmanaged = %v", err)
	}
	if err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(a.ProjectSkillPath(project, skill.Identifier)); err != nil || got != skill.SourcePath {
		t.Fatalf("undo link = %q, %v", got, err)
	}
}

func TestListReportsOrphanedLibraryLink(t *testing.T) {
	root, project, skill := fixture(t)
	a, _ := adapter.For(adapter.Codex)
	path := a.ProjectSkillPath(project, skill.Identifier)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skill.SourcePath, path); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(skill.SourcePath); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	items, err := svc.List(project)
	if err != nil {
		t.Fatal(err)
	}
	if !hasStatus(items, skill.Identifier, lifecycle.Orphaned) {
		t.Fatalf("inventory = %#v", items)
	}
}

func TestAdoptRefusesConflictThenReplacesProjectDirectoryWithManagedLink(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	library := filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	writeSkill(t, filepath.Join(library, "demo"), "demo")
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrConflict) {
		t.Fatalf("adopt conflict = %v", err)
	}
	if _, err := os.Stat(local.SourcePath); err != nil {
		t.Fatalf("project skill changed on conflict: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(library, "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(local.SourcePath); err != nil || got != filepath.Join(library, "demo") {
		t.Fatalf("adopted link = %q, %v", got, err)
	}
	assertNoAdoptionStages(t, filepath.Dir(local.SourcePath))
}

func TestForkMakesIndependentCopyWithoutChangingLibrary(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fork(project, adapter.Codex, skill.Identifier); err != nil {
		t.Fatal(err)
	}
	a, _ := adapter.For(adapter.Codex)
	path := a.ProjectSkillPath(project, skill.Identifier)
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("forked path = %#v, %v", info, err)
	}
	if err := os.WriteFile(filepath.Join(path, "local.txt"), []byte("independent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skill.SourcePath, "local.txt")); !os.IsNotExist(err) {
		t.Fatalf("library changed by fork: %v", err)
	}
}

func TestAddRejectsSkillOutsideConfiguredLibraryAndUnsafeIdentifier(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	skill.SourcePath = filepath.Join(root, "elsewhere", "demo")
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("outside source = %v", err)
	}
	skill.SourcePath = filepath.Join(root, "library", "demo")
	skill.Identifier = "../escape"
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("unsafe identifier = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unsafe identifier wrote outside target: %v", err)
	}
}

func TestManagedOperationsRejectLinkToDifferentLibraryEntry(t *testing.T) {
	root, project, skill := fixture(t)
	a, _ := adapter.For(adapter.Codex)
	path := a.ProjectSkillPath(project, skill.Identifier)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "library", "other"), path); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Remove(project, adapter.Codex, skill.Identifier); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("remove wrong target = %v", err)
	}
	if _, err := svc.Fork(project, adapter.Codex, skill.Identifier); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("fork wrong target = %v", err)
	}
}

func TestAddStagesAllTargetsBeforeWriting(t *testing.T) {
	root, project, skill := fixture(t)
	codex, _ := adapter.For(adapter.Codex)
	blocked := codex.ProjectSkillPath(project, skill.Identifier)
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("add = %v", err)
	}
	claude, _ := adapter.For(adapter.ClaudeCode)
	if _, err := os.Lstat(claude.ProjectSkillPath(project, skill.Identifier)); !os.IsNotExist(err) {
		t.Fatalf("earlier target changed after later failure: %v", err)
	}
}

func TestForkManyStagesAllTargetsBeforeReplacingLinks(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	codex, _ := adapter.For(adapter.Codex)
	if err := os.Remove(codex.ProjectSkillPath(project, skill.Identifier)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(codex.ProjectSkillPath(project, skill.Identifier), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ForkMany(project, skill.Identifier, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("fork many = %v", err)
	}
	claude, _ := adapter.For(adapter.ClaudeCode)
	if info, err := os.Lstat(claude.ProjectSkillPath(project, skill.Identifier)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("earlier link changed after later failure: %#v, %v", info, err)
	}
}

func TestUndoUsesServiceConfirmation(t *testing.T) {
	root, project, skill := fixture(t)
	confirmed := true
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return confirmed })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	confirmed = false
	if err := svc.Undo(); !errors.Is(err, lifecycle.ErrNotConfirmed) {
		t.Fatalf("Undo = %v", err)
	}
}

func TestAddRevalidatesDestinationAfterConfirmation(t *testing.T) {
	root, project, skill := fixture(t)
	a, _ := adapter.For(adapter.Codex)
	destination := a.ProjectSkillPath(project, skill.Identifier)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool {
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Add = %v", err)
	}
	if info, err := os.Stat(destination); err != nil || !info.IsDir() {
		t.Fatalf("new unmanaged destination changed: %#v, %v", info, err)
	}
}

func TestRemoveRevalidatesLinkAfterConfirmation(t *testing.T) {
	root, project, skill := fixture(t)
	a, _ := adapter.For(adapter.Codex)
	destination := a.ProjectSkillPath(project, skill.Identifier)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	svc = lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool {
		if err := os.Remove(destination); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte("owned"), 0o644); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if _, err := svc.Remove(project, adapter.Codex, skill.Identifier); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Remove = %v", err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "owned" {
		t.Fatalf("replacement was changed: %q, %v", data, err)
	}
}

func TestAdoptRevalidatesLibraryConflictAfterConfirmation(t *testing.T) {
	root := t.TempDir()
	project, library := filepath.Join(root, "project"), filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool {
		writeSkill(t, filepath.Join(library, "demo"), "demo")
		return true
	})
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrConflict) {
		t.Fatalf("Adopt = %v", err)
	}
	if _, err := os.Stat(local.SourcePath); err != nil {
		t.Fatalf("project source changed: %v", err)
	}
}

func TestAddRequiresExistingEligibleConfiguredLibrarySkill(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if err := os.RemoveAll(skill.SourcePath); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source = %v", err)
	}
	if err := os.WriteFile(skill.SourcePath, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("file source = %v", err)
	}
	if err := os.Remove(skill.SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(skill.SourcePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("invalid source = %v", err)
	}
}

func TestForkManyRejectsDuplicateTargets(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ForkMany(project, skill.Identifier, []adapter.Target{adapter.Codex, adapter.Codex}); err == nil {
		t.Fatal("fork with a repeated target unexpectedly succeeded")
	}
	a, _ := adapter.For(adapter.Codex)
	if got, err := os.Readlink(a.ProjectSkillPath(project, skill.Identifier)); err != nil || got != skill.SourcePath {
		t.Fatalf("partial fork was not rolled back: %q, %v", got, err)
	}
}

func TestAddRejectsDuplicateTargetsBeforeMutation(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.Codex, adapter.Codex}); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Add = %v", err)
	}
	a, _ := adapter.For(adapter.Codex)
	if _, err := os.Lstat(a.ProjectSkillPath(project, skill.Identifier)); !os.IsNotExist(err) {
		t.Fatalf("duplicate activation mutated target: %v", err)
	}
}

func TestForkManyRestoresAllLinksWhenSecondPublicationFails(t *testing.T) {
	root, project, skill := fixture(t)
	svc := lifecycle.New(filepath.Join(root, "library"), operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	if _, err := svc.Add(project, skill, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc.BeforePublish = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("publish failure")
		}
		return nil
	}
	if _, err := svc.ForkMany(project, skill.Identifier, []adapter.Target{adapter.ClaudeCode, adapter.Codex}); err == nil {
		t.Fatal("ForkMany unexpectedly succeeded")
	}
	for _, target := range []adapter.Target{adapter.ClaudeCode, adapter.Codex} {
		a, _ := adapter.For(target)
		if got, err := os.Readlink(a.ProjectSkillPath(project, skill.Identifier)); err != nil || got != skill.SourcePath {
			t.Fatalf("%s not restored: %q, %v", target, got, err)
		}
	}
}

func TestAdoptRefusesProjectEditAfterCopyIsStaged(t *testing.T) {
	root := t.TempDir()
	project, library := filepath.Join(root, "project"), filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	svc.BeforePublish = func(step string) error {
		if step == "adopt-staged" {
			return os.WriteFile(filepath.Join(local.SourcePath, "edit"), []byte("user"), 0o644)
		}
		return nil
	}
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Adopt = %v", err)
	}
	if _, err := os.Lstat(local.SourcePath); err != nil {
		t.Fatalf("project skill was removed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(local.SourcePath, "edit")); err != nil || string(got) != "user" {
		t.Fatalf("concurrent edit was not preserved: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(library, "demo")); !os.IsNotExist(err) {
		t.Fatalf("staged library copy was retained: %v", err)
	}
}

func TestAdoptPreservesEditCreatedAfterProjectTreeMovesAside(t *testing.T) {
	root := t.TempDir()
	project, library := filepath.Join(root, "project"), filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	svc.BeforePublish = func(step string) error {
		if step == "adopt-moved" {
			if err := os.MkdirAll(local.SourcePath, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(local.SourcePath, "edit"), []byte("user"), 0o644)
		}
		return nil
	}
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Adopt = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(local.SourcePath, "edit")); err != nil || string(got) != "user" {
		t.Fatalf("post-move edit was not preserved: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(library, "demo")); !os.IsNotExist(err) {
		t.Fatalf("library copy was retained: %v", err)
	}
}

func TestAdoptPreservesMovedOriginalWhenDestinationIsRecreated(t *testing.T) {
	root := t.TempDir()
	project, library := filepath.Join(root, "project"), filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	if err := os.WriteFile(filepath.Join(local.SourcePath, "resource"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	svc.BeforePublish = func(step string) error {
		if step == "adopt-moved" {
			if err := os.MkdirAll(local.SourcePath, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(local.SourcePath, "edit"), []byte("user"), 0o644)
		}
		return nil
	}
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Adopt = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(local.SourcePath, "edit")); err != nil || string(got) != "user" {
		t.Fatalf("concurrent edit = %q, %v", got, err)
	}
	matches, err := filepath.Glob(local.SourcePath + ".skill-manager-recovery-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("recovery path = %#v, %v", matches, err)
	}
	if got, err := os.ReadFile(filepath.Join(matches[0], "resource")); err != nil || string(got) != "original" {
		t.Fatalf("original resource = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(matches[0], "SKILL.md")); err != nil {
		t.Fatalf("original SKILL.md missing: %v", err)
	}
	assertNoAdoptionStages(t, filepath.Dir(local.SourcePath))
}

func TestAdoptKeepsPreviousStageWhenRecoveryPublicationFails(t *testing.T) {
	root := t.TempDir()
	project, library := filepath.Join(root, "project"), filepath.Join(root, "library")
	local := writeSkill(t, filepath.Join(project, ".codex", "skills", "demo"), "demo")
	if err := os.WriteFile(filepath.Join(local.SourcePath, "resource"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := lifecycle.New(library, operation.New(filepath.Join(root, "journal.json")), func(operation.Plan) bool { return true })
	svc.BeforePublish = func(step string) error {
		switch step {
		case "adopt-moved":
			if err := os.MkdirAll(local.SourcePath, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(local.SourcePath, "edit"), []byte("user"), 0o644)
		case "adopt-recovery":
			return errors.New("recovery publication failure")
		}
		return nil
	}
	if _, err := svc.Adopt(project, adapter.Codex, "demo"); !errors.Is(err, lifecycle.ErrUnsafePath) {
		t.Fatalf("Adopt = %v", err)
	}
	stages, err := filepath.Glob(filepath.Join(filepath.Dir(local.SourcePath), ".skill-manager-stage-*", "previous"))
	if err != nil || len(stages) != 1 {
		t.Fatalf("unrecovered previous = %#v, %v", stages, err)
	}
	if got, err := os.ReadFile(filepath.Join(stages[0], "resource")); err != nil || string(got) != "original" {
		t.Fatalf("previous resource = %q, %v", got, err)
	}
}

func assertNoAdoptionStages(t *testing.T, parent string) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(parent, ".skill-manager-stage-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("stages = %#v, %v", stages, err)
	}
}

func fixture(t *testing.T) (string, string, catalog.Skill) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	skill := writeSkill(t, filepath.Join(root, "library", "demo"), "demo")
	skill.Compatibility = []string{string(adapter.ClaudeCode)}
	return root, project, skill
}
func writeSkill(t *testing.T, dir, id string) catalog.Skill {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return catalog.Skill{Identifier: id, SourcePath: dir}
}
func hasStatus(items []lifecycle.Item, id string, status lifecycle.Status) bool {
	for _, item := range items {
		if item.Identifier == id && item.Status == status {
			return true
		}
	}
	return false
}
