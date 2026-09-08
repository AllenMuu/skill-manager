package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/cli"
)

// e2eFixture builds a configured library with one eligible skill and an
// empty project, plus a CLI config pointing at them.
func e2eFixture(t *testing.T) (library, project, configPath string) {
	t.Helper()
	library = t.TempDir()
	writeSkill(t, library, "demo", "Demo", "demo skill", "tag\n")
	project = t.TempDir()
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: "+library+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return library, project, configPath
}

func runCLI(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	root := cli.NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(append([]string{"--config", configPath}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("args %v: %v", args, err)
	}
	return out.String()
}

func runCLIErr(t *testing.T, configPath string, args ...string) error {
	t.Helper()
	root := cli.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"--config", configPath}, args...))
	return root.Execute()
}

func TestEndToEndInitInstallsOperatorSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := cli.NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"init", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".claude", ".codex"} {
		if _, err := os.Stat(filepath.Join(home, dir, "skills", "skill-manager-operator", "SKILL.md")); err != nil {
			t.Fatalf("operator skill missing for %s: %v", dir, err)
		}
	}
}

func TestEndToEndListRemoveAndUndo(t *testing.T) {
	_, project, configPath := e2eFixture(t)
	out := runCLI(t, configPath, "add", "demo", "--project", project, "--target", "codex", "--yes")
	if !strings.Contains(out, "create absolute link") {
		t.Fatalf("add output=%q", out)
	}
	link := filepath.Join(project, ".codex", "skills", "demo")

	out = runCLI(t, configPath, "list", "--project", project)
	if !strings.Contains(out, "codex\tdemo\tmanaged") {
		t.Fatalf("list output=%q", out)
	}

	runCLI(t, configPath, "undo", "--project", project, "--yes")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("undo left the link: %v", err)
	}

	runCLI(t, configPath, "add", "demo", "--project", project, "--target", "codex", "--yes")
	runCLI(t, configPath, "remove", "demo", "--project", project, "--target", "codex", "--yes")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("remove left the link: %v", err)
	}
}

func TestEndToEndAdoptAndFork(t *testing.T) {
	library, project, configPath := e2eFixture(t)
	adoptable := filepath.Join(project, ".codex", "skills", "adopted")
	if err := os.MkdirAll(adoptable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adoptable, "SKILL.md"), []byte("---\nname: adopted\ndescription: adopted\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runCLI(t, configPath, "adopt", "adopted", "--project", project, "--target", "codex", "--yes")
	if _, err := os.Stat(filepath.Join(library, "adopted", "SKILL.md")); err != nil {
		t.Fatalf("adopted skill missing from library: %v", err)
	}
	if got, err := os.Readlink(adoptable); err != nil || got != filepath.Join(library, "adopted") {
		t.Fatalf("adopted path = %q, %v; want library link", got, err)
	}

	runCLI(t, configPath, "add", "demo", "--project", project, "--target", "codex", "--yes")
	forked := filepath.Join(project, ".codex", "skills", "demo")
	runCLI(t, configPath, "fork", "demo", "--project", project, "--target", "codex", "--yes")
	info, err := os.Lstat(forked)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("fork left a link instead of an independent directory")
	}
	if _, err := os.Stat(filepath.Join(forked, "SKILL.md")); err != nil {
		t.Fatalf("forked skill content missing: %v", err)
	}
}

func TestEndToEndDoctorReportsOrphanedLink(t *testing.T) {
	library, project, configPath := e2eFixture(t)
	runCLI(t, configPath, "add", "demo", "--project", project, "--target", "codex", "--yes")
	link := filepath.Join(project, ".codex", "skills", "demo")
	// Break the link destination to make the managed link orphaned.
	if err := os.RemoveAll(filepath.Join(library, "demo")); err != nil {
		t.Fatal(err)
	}
	out := runCLI(t, configPath, "doctor", "--project", project)
	if !strings.Contains(out, link) {
		t.Fatalf("doctor output=%q; want the orphaned link path", out)
	}
}

func TestEndToEndReconcileRelinksMovedLibrary(t *testing.T) {
	root := t.TempDir()
	oldLibrary := filepath.Join(root, "old-library")
	newLibrary := filepath.Join(root, "new-library")
	if err := os.MkdirAll(filepath.Join(oldLibrary, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldLibrary, "demo", "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	link := filepath.Join(project, ".codex", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(oldLibrary, "demo"), link); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: "+oldLibrary+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, configPath, "add", "demo", "--project", project, "--target", "codex", "--yes")

	// Move the library and repoint the config at its new location.
	if err := os.Rename(oldLibrary, newLibrary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("library: "+newLibrary+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, configPath, "reconcile", "--project", project, "--yes")
	if got, err := os.Readlink(link); err != nil || got != filepath.Join(newLibrary, "demo") {
		t.Fatalf("reconciled link = %q, %v; want new library link", got, err)
	}
}

func TestEndToEndUndoEmptyJournalFails(t *testing.T) {
	_, project, configPath := e2eFixture(t)
	if err := runCLIErr(t, configPath, "undo", "--project", project, "--yes"); err == nil {
		t.Fatal("undo on empty journal succeeded")
	}
}
