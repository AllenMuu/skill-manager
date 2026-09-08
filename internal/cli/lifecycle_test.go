package cli_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/cli"
)

func TestRootExposesGuardedCommandSurface(t *testing.T) {
	root := cli.NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"init", "select", "add", "list", "remove", "adopt", "fork", "doctor", "reconcile", "undo", "delete"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("help missing %q: %s", name, out.String())
		}
	}
}

func TestSelectAggregatesMultiSkillPreviewAndConfirmation(t *testing.T) {
	library, project, configPath := selectFixture(t)
	root := cli.NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetIn(strings.NewReader("\ndemo,two\ncodex\ny\n"))
	root.SetArgs([]string{"--config", configPath, "select", "--project", project})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "activate selected skills") != 1 {
		t.Fatalf("output=%q", out.String())
	}
	for _, id := range []string{"demo", "two"} {
		if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", id)); err != nil {
			t.Fatal(err)
		}
	}
	_ = library
}

func TestSelectDeclineLeavesNoLinks(t *testing.T) {
	_, project, configPath := selectFixture(t)
	root := cli.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader("\ndemo,two\ncodex\nn\n"))
	root.SetArgs([]string{"--config", configPath, "select", "--project", project})
	if err := root.Execute(); err == nil {
		t.Fatal("decline accepted")
	}
	for _, id := range []string{"demo", "two"} {
		if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", id)); !os.IsNotExist(err) {
			t.Fatalf("%s activated", id)
		}
	}
}

func TestSelectRejectsInvalidSelectionAndTarget(t *testing.T) {
	_, project, configPath := selectFixture(t)
	for _, input := range []string{"demo\nmissing\ncodex\ny\n", "demo\ndemo\nunknown\ny\n"} {
		root := cli.NewRootCommand()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetIn(strings.NewReader(input))
		root.SetArgs([]string{"--config", configPath, "select", "--project", project})
		if err := root.Execute(); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestSelectPropagatesWriterAndScannerFailures(t *testing.T) {
	_, project, configPath := selectFixture(t)
	root := cli.NewRootCommand()
	root.SetOut(failingWriter{err: errors.New("write failure")})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader("demo\n"))
	root.SetArgs([]string{"--config", configPath, "select", "--project", project})
	if err := root.Execute(); err == nil || err.Error() != "write failure" {
		t.Fatalf("writer err=%v", err)
	}
	root = cli.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"--config", configPath, "select", "--project", project})
	if err := root.Execute(); err == nil {
		t.Fatal("scanner exhaustion accepted")
	}
	root = cli.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(errorReader{err: errors.New("input failure")})
	root.SetArgs([]string{"--config", configPath, "select", "--project", project})
	if err := root.Execute(); err == nil || err.Error() != "input failure" {
		t.Fatalf("scanner I/O err=%v", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func selectFixture(t *testing.T) (string, string, string) {
	t.Helper()
	library := t.TempDir()
	writeSkill(t, library, "demo", "Demo", "demo skill", "tag\n")
	writeSkill(t, library, "two", "Two", "two skill", "tag\n")
	project := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: "+library+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return library, project, configPath
}

func TestAddPreviewWriterFailureAbortsMutation(t *testing.T) {
	library := t.TempDir()
	writeSkill(t, library, "demo", "Demo", "demo", "tag\n")
	project := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: "+library+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := cli.NewRootCommand()
	root.SetOut(failingWriter{err: errors.New("preview failed")})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "add", "demo", "--project", project, "--target", "codex", "--yes"})
	if err := root.Execute(); err == nil || err.Error() != "preview failed" {
		t.Fatalf("add error=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".codex", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("link created despite failed preview: %v", err)
	}
}

func TestAddRequiresTargetUnlessAllDetected(t *testing.T) {
	root := cli.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"add", "demo"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("add error = %v", err)
	}
}
