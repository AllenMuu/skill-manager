package operation_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/operation"
)

func TestPlanRendersChangesAndWarnings(t *testing.T) {
	plan := operation.Plan{Operation: "activate", Changes: []operation.Change{{Path: "/project/.codex/skills/demo", Action: "create link", Detail: "/library/demo"}}, Warnings: []string{"not declared compatible with codex"}}
	if got := plan.String(); got == "" || !strings.Contains(got, "create link") || !strings.Contains(got, "not declared compatible") {
		t.Fatalf("plan did not render its preview: %q", got)
	}
}

func TestJournalUndoRestoresLatestPreOperationState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "project", "skill")
	journal := operation.New(filepath.Join(root, "journal.json"))
	before, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/library/demo", path); err != nil {
		t.Fatal(err)
	}
	after, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Record("activate", before, after); err != nil {
		t.Fatal(err)
	}
	if err := journal.UndoLatest(func(operation.Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("path after undo = %v, want absent", err)
	}
}

func TestJournalUndoRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skill")
	journal := operation.New(filepath.Join(root, "journal.json"))
	before, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/library/demo", path); err != nil {
		t.Fatal(err)
	}
	after, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Record("activate", before, after); err != nil {
		t.Fatal(err)
	}
	var preview operation.Plan
	if err := journal.UndoLatest(func(plan operation.Plan) bool { preview = plan; return false }); !errors.Is(err, operation.ErrNotConfirmed) {
		t.Fatalf("UndoLatest = %v", err)
	}
	if len(preview.Changes) == 0 {
		t.Fatal("undo did not preview changes")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("declined undo changed path: %v", err)
	}
}

func TestJournalUndoRefusesUnexpectedCurrentState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skill")
	journal := operation.New(filepath.Join(root, "journal.json"))
	before, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/library/demo", path); err != nil {
		t.Fatal(err)
	}
	after, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Record("activate", before, after); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := journal.UndoLatest(func(operation.Plan) bool { return true }); !errors.Is(err, operation.ErrUnexpectedState) {
		t.Fatalf("UndoLatest = %v", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("unexpected state was not preserved: %#v, %v", info, err)
	}
}

func TestRestorePreflightPreservesAllCurrentPathsWhenBackupIsMissing(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	if err := os.WriteFile(first, []byte("before-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("before-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(root, "journal.json"))
	snapshots, err := journal.Capture([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("current-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("current-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(snapshots[1].Backup); err != nil {
		t.Fatal(err)
	}
	if err := journal.Restore(snapshots); err == nil {
		t.Fatal("Restore unexpectedly succeeded with missing backup")
	}
	for _, want := range []struct{ path, text string }{{first, "current-first"}, {second, "current-second"}} {
		got, err := os.ReadFile(want.path)
		if err != nil || string(got) != want.text {
			t.Fatalf("%s = %q, %v", want.path, got, err)
		}
	}
}

func TestUndoPropagatesMissingExpectedAfterState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skill")
	journal := operation.New(filepath.Join(root, "journal.json"))
	before, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/library/demo", path); err != nil {
		t.Fatal(err)
	}
	after, err := journal.Capture([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Record("activate", before, after); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	err = journal.UndoLatest(func(operation.Plan) bool { return true })
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UndoLatest = %v, want missing-target cause", err)
	}
}

func TestRestoreRestoresMovedCurrentPathWhenSecondPublishFails(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	if err := os.WriteFile(first, []byte("before-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("before-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(root, "journal.json"))
	snapshots, err := journal.Capture([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("current-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("current-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	journal.BeforeRestorePublish = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("publish failure")
		}
		return nil
	}
	if err := journal.Restore(snapshots); err == nil {
		t.Fatal("Restore unexpectedly succeeded")
	}
	for _, want := range []struct{ path, text string }{{first, "current-first"}, {second, "current-second"}} {
		got, err := os.ReadFile(want.path)
		if err != nil || string(got) != want.text {
			t.Fatalf("%s = %q, %v", want.path, got, err)
		}
	}
}

func TestRestoreRemovesPublishedCandidateWhenItHadNoCurrentPath(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	if err := os.WriteFile(first, []byte("before-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("before-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(root, "journal.json"))
	snapshots, err := journal.Capture([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("current-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	journal.BeforeRestorePublish = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("publish failure")
		}
		return nil
	}
	if err := journal.Restore(snapshots); err == nil {
		t.Fatal("Restore unexpectedly succeeded")
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("published candidate remained at absent current path: %v", err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != "current-second" {
		t.Fatalf("second current state = %q, %v", got, err)
	}
}
