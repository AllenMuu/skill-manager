package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/cli"
	"github.com/AllenMuu/skill-manager/internal/operation"
	"github.com/spf13/cobra"
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

func TestAgentManagerPrimaryCommandAndSkillManagerCompatibilityAlias(t *testing.T) {
	_, _, configPath := e2eFixture(t)

	primary := cli.NewAgentManagerCommand()
	if primary.Use != "agent-manager" {
		t.Fatalf("primary command = %q, want agent-manager", primary.Use)
	}
	primaryOut := &bytes.Buffer{}
	primary.SetOut(primaryOut)
	primary.SetErr(primaryOut)
	primary.SetArgs([]string{"--config", configPath, "search", "demo"})
	if err := primary.Execute(); err != nil {
		t.Fatalf("primary command: %v", err)
	}
	if !strings.Contains(primaryOut.String(), "demo") {
		t.Fatalf("primary output=%q", primaryOut.String())
	}

	legacy := cli.NewSkillManagerCommand()
	legacyOut := &bytes.Buffer{}
	legacy.SetOut(legacyOut)
	legacy.SetErr(legacyOut)
	legacy.SetArgs([]string{"--config", configPath, "search", "demo"})
	if err := legacy.Execute(); err != nil {
		t.Fatalf("legacy command: %v", err)
	}
	if !strings.Contains(legacyOut.String(), "deprecated") || !strings.Contains(legacyOut.String(), "agent-manager") {
		t.Fatalf("legacy output=%q; want migration notice", legacyOut.String())
	}
}

func TestAgentsInventoryReportsDeclaredResourceCapabilitiesAsJSON(t *testing.T) {
	root := cli.NewAgentManagerCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"agents", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("agents inventory: %v", err)
	}
	for _, want := range []string{`"id":"claude-code"`, `"id":"codex"`, `"id":"pi"`, `"resourceKinds":["skill"]`, `"filesystem-write"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inventory output=%q; missing %s", out.String(), want)
		}
	}
}

func TestAgentsInventoryReportsLocalAdapterAvailability(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := cli.NewAgentManagerCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"agents", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("agents inventory: %v", err)
	}

	var items []struct {
		ID           string `json:"id"`
		Availability string `json:"availability"`
	}
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	availability := make(map[string]string, len(items))
	for _, item := range items {
		availability[item.ID] = item.Availability
	}
	for id, want := range map[string]string{"claude-code": "configured", "codex": "detected", "pi": "unavailable"} {
		if got := availability[id]; got != want {
			t.Errorf("%s availability = %q, want %q; inventory=%s", id, got, want, out.String())
		}
	}
}

func TestAgentsInventoryDistinguishesUnsupportedProjectAgent(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".foo", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := cli.NewAgentManagerCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"agents", "--project", project, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("agents inventory: %v", err)
	}

	var items []struct {
		ID     string   `json:"id"`
		Status string   `json:"status"`
		Kinds  []string `json:"resourceKinds"`
	}
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("decode inventory: %v; output=%s", err, out.String())
	}
	var found bool
	for _, item := range items {
		if item.ID == "foo" {
			found = true
			if item.Status != "unsupported" || len(item.Kinds) != 0 {
				t.Fatalf("unsupported item = %#v, want unsupported with no resource kinds", item)
			}
		}
	}
	if !found {
		t.Fatalf("inventory=%s; missing unsupported foo agent", out.String())
	}
}

func TestMemoryStatusReportsConfiguredProviderAndPerAgentCapabilities(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: v1\nmemory:\n  version: v1\n  id: local\n  provider: graphiti\n  configuration:\n    kind: env\n    name: GRAPHITI_URL\n  scopes: [user, project]\n  capabilities: [read, search]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAPHITI_URL", "http://graphiti.test/?token=secret")
	root := cli.NewAgentManagerCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--config", configPath, "memory", "status", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "GRAPHITI_URL") || strings.Contains(out.String(), "secret") {
		t.Fatalf("status leaked sensitive data: %s", out.String())
	}
	for _, want := range []string{`"status":"configured"`, `"agent":"claude-code"`, `"status":"unsupported"`, `"status":"configured"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output=%s; missing %s", out.String(), want)
		}
	}
}

func TestMemoryConfigurePersistsExplicitProviderReference(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: v1\nlibrary: /tmp/skills\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := cli.NewAgentManagerCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "memory", "configure", "--reference", "GRAPHITI_URL"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "GRAPHITI_URL") {
		t.Fatalf("config = %s; missing explicit reference", contents)
	}
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

func TestEndToEndDoctorReportsCapabilitiesAndUnmanagedResources(t *testing.T) {
	_, project, configPath := e2eFixture(t)
	local := filepath.Join(project, ".pi", "skills", "local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nname: local\ndescription: local\n---\n")
	if err := os.WriteFile(filepath.Join(local, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	out := runCLI(t, configPath, "doctor", "--project", project)
	for _, want := range []string{"adapter pi", "filesystem-write", "unmanaged resource", local} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output=%q; missing %q", out, want)
		}
	}
	after, err := os.ReadFile(filepath.Join(local, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(content) {
		t.Fatal("doctor mutated unmanaged resource")
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

func TestPublicCLIActivationParityAcrossEntrypointsAndAdapters(t *testing.T) {
	entrypoints := []struct {
		name string
		new  func() *cobra.Command
	}{
		{name: "agent-manager", new: cli.NewAgentManagerCommand},
		{name: "skill-manager", new: cli.NewSkillManagerCommand},
	}
	targets := []adapter.Target{adapter.ClaudeCode, adapter.Codex, adapter.Pi}
	for _, entrypoint := range entrypoints {
		for _, target := range targets {
			t.Run(entrypoint.name+"/"+string(target), func(t *testing.T) {
				library, project, configPath := e2eFixture(t)
				root := entrypoint.new()
				out := &bytes.Buffer{}
				root.SetOut(out)
				root.SetErr(out)
				root.SetArgs([]string{"--config", configPath, "add", "demo", "--project", project, "--target", string(target), "--yes"})
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}

				a, ok := adapter.For(target)
				if !ok {
					t.Fatalf("missing adapter for %s", target)
				}
				link := a.ProjectSkillPath(project, "demo")
				got, err := os.Readlink(link)
				if err != nil || got != filepath.Join(library, "demo") || !filepath.IsAbs(got) {
					t.Fatalf("%s link = %q, %v", target, got, err)
				}

				journalPath := filepath.Join(project, ".skill-manager", "journal.json")
				entry, found, err := operation.New(journalPath).Latest()
				if err != nil || !found {
					t.Fatalf("latest journal entry = %#v, found=%v, err=%v", entry, found, err)
				}
				if entry.Version != "v1" || entry.ResourceKind != "skill" || entry.Operation != "activate" {
					t.Fatalf("journal entry = %#v", entry)
				}
				if len(entry.Before) != 1 || entry.Before[0].Exists || len(entry.After) != 1 || !entry.After[0].Exists || entry.After[0].Path != link {
					t.Fatalf("journal snapshots = %#v", entry)
				}

				root = entrypoint.new()
				root.SetOut(&bytes.Buffer{})
				root.SetErr(&bytes.Buffer{})
				root.SetArgs([]string{"--config", configPath, "undo", "--project", project, "--yes"})
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(link); !os.IsNotExist(err) {
					t.Fatalf("undo left %s: %v", link, err)
				}
				if _, err := os.Stat(got); err != nil {
					t.Fatalf("undo removed library skill: %v", err)
				}
				if _, found, err := operation.New(journalPath).Latest(); err != nil || found {
					t.Fatalf("journal after undo = found=%v, err=%v", found, err)
				}
			})
		}
	}
}

func TestPublicCLIRecoversLegacySkillJournalAcrossEntrypointsAndAdapters(t *testing.T) {
	entrypoints := []struct {
		name string
		new  func() *cobra.Command
	}{
		{name: "agent-manager", new: cli.NewAgentManagerCommand},
		{name: "skill-manager", new: cli.NewSkillManagerCommand},
	}
	for _, entrypoint := range entrypoints {
		for _, target := range []adapter.Target{adapter.ClaudeCode, adapter.Codex, adapter.Pi} {
			t.Run(entrypoint.name+"/"+string(target), func(t *testing.T) {
				library, project, configPath := e2eFixture(t)
				root := entrypoint.new()
				root.SetOut(&bytes.Buffer{})
				root.SetErr(&bytes.Buffer{})
				root.SetArgs([]string{"--config", configPath, "add", "demo", "--project", project, "--target", string(target), "--yes"})
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				journalPath := filepath.Join(project, ".skill-manager", "journal.json")
				contents, err := os.ReadFile(journalPath)
				if err != nil {
					t.Fatal(err)
				}
				var records []map[string]any
				if err := json.Unmarshal(contents, &records); err != nil {
					t.Fatal(err)
				}
				delete(records[0], "version")
				delete(records[0], "resourceKind")
				legacy, err := json.Marshal(records)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(journalPath, legacy, 0o644); err != nil {
					t.Fatal(err)
				}

				root = entrypoint.new()
				root.SetOut(&bytes.Buffer{})
				root.SetErr(&bytes.Buffer{})
				root.SetArgs([]string{"--config", configPath, "undo", "--project", project, "--yes"})
				if err := root.Execute(); err != nil {
					t.Fatalf("legacy undo: %v", err)
				}
				a, _ := adapter.For(target)
				if _, err := os.Lstat(a.ProjectSkillPath(project, "demo")); !os.IsNotExist(err) {
					t.Fatalf("legacy undo left activation: %v", err)
				}
				if _, err := os.Stat(filepath.Join(library, "demo")); err != nil {
					t.Fatalf("legacy undo removed library skill: %v", err)
				}
				if _, found, err := operation.New(journalPath).Latest(); err != nil || found {
					t.Fatalf("legacy journal after undo = found=%v, err=%v", found, err)
				}
			})
		}
	}
}
