package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/cli"
)

func TestSubAgentsListJSONReturnsCanonicalDefinitions(t *testing.T) {
	root := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Code Reviewer
role: Reviews changes
instructions: Review the diff and report risks.
skills:
  - go-helper
compatibility:
  agents:
    - codex
requiredCapabilities:
  - filesystem-read
`)
	library := t.TempDir()
	writeSkill(t, library, "go-helper", "Go helper", "helper", "go\n")

	rootCommand := cli.NewRootCommand()
	output := &bytes.Buffer{}
	rootCommand.SetOut(output)
	rootCommand.SetErr(output)
	rootCommand.SetArgs([]string{"subagents", "list", "--root", root, "--library", library, "--json"})
	if err := rootCommand.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v; output=%s", err, output)
	}
	if len(got) != 1 || got[0]["id"] != "reviewer" || got[0]["role"] != "Reviews changes" {
		t.Fatalf("canonical list = %#v", got)
	}
	if _, ok := got[0]["compatibility"]; !ok {
		t.Fatalf("canonical list omitted compatibility: %#v", got[0])
	}
}

func TestSubAgentsUseDefaultConfiguredSkillLibraryWhenLibraryIsOmitted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	library := filepath.Join(home, ".agents", "skills")
	writeSkill(t, library, "go-helper", "Go helper", "helper", "go\n")
	root := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
skills:
  - go-helper
`)

	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "list", "--root", root, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), `"id":"reviewer"`) {
		t.Fatalf("output=%q; want definition accepted through default Skill library", output.String())
	}
}

func TestSubAgentsWithoutLibraryAreNotDependentOnHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
`)

	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "list", "--root", root, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; no-reference definitions should not require ~/.agents/skills", err)
	}
	if !strings.Contains(output.String(), `"id":"reviewer"`) {
		t.Fatalf("output=%q; want definition", output.String())
	}
}

func TestSubAgentsExplicitMissingLibraryReturnsError(t *testing.T) {
	root := t.TempDir()
	missingLibrary := filepath.Join(t.TempDir(), "missing-skills")
	command := cli.NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"subagents", "list", "--root", root, "--library", missingLibrary})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "skill library") {
		t.Fatalf("error = %v; explicit missing library should fail", err)
	}
}

func TestSubAgentsValidateReportsMissingReferencedSkill(t *testing.T) {
	root := t.TempDir()
	library := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
skills:
  - absent-skill
`)

	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "validate", "--root", root, "--library", library, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("validate should report missing references without command error: %v", err)
	}
	if !strings.Contains(output.String(), `"valid":false`) || !strings.Contains(output.String(), "absent-skill") {
		t.Fatalf("validation output=%q; want missing referenced Skill diagnostic", output.String())
	}
}

func TestSubAgentsInstallUndoAndRemoveThroughCLI(t *testing.T) {
	root, project, library := t.TempDir(), t.TempDir(), t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
`)
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("library: "+library+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "install", "reviewer", "--root", root, "--library", library, "--project", project, "--target", "codex", "--yes"})
	if err := command.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}
	destination := filepath.Join(project, ".codex", "agents", "reviewer.toml")
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("installed SubAgent missing: %v", err)
	}

	undo := cli.NewRootCommand()
	undo.SetOut(&bytes.Buffer{})
	undo.SetErr(&bytes.Buffer{})
	undo.SetArgs([]string{"--config", config, "undo", "--project", project, "--yes"})
	if err := undo.Execute(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("undo left installation: %v", err)
	}

	installAgain := cli.NewRootCommand()
	installAgain.SetOut(&bytes.Buffer{})
	installAgain.SetErr(&bytes.Buffer{})
	installAgain.SetArgs([]string{"subagents", "install", "reviewer", "--root", root, "--library", library, "--project", project, "--target", "codex", "--yes"})
	if err := installAgain.Execute(); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	remove := cli.NewRootCommand()
	remove.SetOut(&bytes.Buffer{})
	remove.SetErr(&bytes.Buffer{})
	remove.SetArgs([]string{"subagents", "remove", "reviewer", "--root", root, "--library", library, "--project", project, "--target", "codex", "--yes"})
	if err := remove.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("remove left installation: %v", err)
	}
}

func TestSubAgentsInstallRejectsUnsupportedTargetWithoutWriting(t *testing.T) {
	root, project, library := t.TempDir(), t.TempDir(), t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", "version: v1\nid: reviewer\nname: Reviewer\nrole: Review\ninstructions: Review changes.\n")
	command := cli.NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"subagents", "install", "reviewer", "--root", root, "--library", library, "--project", project, "--target", "pi", "--yes"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("install error = %v, want unsupported", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".pi")); !os.IsNotExist(err) {
		t.Fatalf("unsupported install wrote files: %v", err)
	}
}

func TestSubAgentsInstallRefusesCLIConflictWithoutForce(t *testing.T) {
	root, project, library := t.TempDir(), t.TempDir(), t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", "version: v1\nid: reviewer\nname: Reviewer\nrole: Review\ninstructions: Review changes.\n")
	destination := filepath.Join(project, ".codex", "agents", "reviewer.toml")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("user-owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := cli.NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"subagents", "install", "reviewer", "--root", root, "--library", library, "--project", project, "--target", "codex", "--conflict", "replace", "--yes"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "requires --force") {
		t.Fatalf("install error = %v, want force requirement", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "user-owned" {
		t.Fatalf("conflict content = %q, %v", content, err)
	}
}

func TestSubAgentsConfiguredMissingLibraryReturnsError(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "agent-manager.yaml")
	if err := os.WriteFile(configPath, []byte("library: missing-skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := cli.NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--config", configPath, "subagents", "list", "--root", root})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "skill library") {
		t.Fatalf("error = %v; configured missing library should fail", err)
	}
}

func TestSubAgentsShowHumanIncludesCanonicalDetails(t *testing.T) {
	root := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Code Reviewer
role: Reviews changes
instructions: Review the diff and report risks.
compatibility:
  agents:
    - codex
`)
	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "show", "reviewer", "--root", root})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"reviewer", "Code Reviewer", "Reviews changes", "Review the diff and report risks.", "codex"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output=%q; missing %q", output.String(), want)
		}
	}
}

func TestSubAgentsShowUnknownIDIsActionable(t *testing.T) {
	root := t.TempDir()
	writeSubAgent(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
`)
	command := cli.NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"subagents", "show", "missing", "--root", root})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "available") {
		t.Fatalf("error = %v, want actionable unknown ID", err)
	}
}

func TestSubAgentsValidateJSONReportsInvalidDefinitions(t *testing.T) {
	root := t.TempDir()
	writeSubAgent(t, root, "broken.yaml", `version: v2
id: Broken
name: Broken
role: Review
instructions: Review changes.
`)
	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "validate", "--root", root, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("validate should report diagnostics without command error: %v", err)
	}
	var report struct {
		Valid       bool `json:"valid"`
		Diagnostics []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode JSON: %v; output=%s", err, output.String())
	}
	if report.Valid || len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0].Message, "version") {
		t.Fatalf("validation report = %#v", report)
	}
}

func TestSubAgentsValidateSpecificInvalidIDReturnsItsDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeSubAgent(t, root, "broken.yaml", "version: v2\nid: broken\n")
	command := cli.NewRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"subagents", "validate", "broken", "--root", root, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("validate should report diagnostics without command error: %v", err)
	}
	if !strings.Contains(output.String(), `"valid":false`) || !strings.Contains(output.String(), "version") {
		t.Fatalf("validation output=%q; want invalid version diagnostic", output.String())
	}
}

func writeSubAgent(t *testing.T, root, name, contents string) {
	t.Helper()
	dir := filepath.Join(root, "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
