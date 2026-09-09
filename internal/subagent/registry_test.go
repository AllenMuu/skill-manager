package subagent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/resource"
	"github.com/AllenMuu/skill-manager/internal/subagent"
)

func TestDiscoverReadsCanonicalDefinitionAndPreservesDetails(t *testing.T) {
	root := t.TempDir()
	writeDefinition(t, root, "reviewer.yaml", `version: v1
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

	registry, err := subagent.NewRegistry(root, func(id string) bool { return id == "go-helper" })
	if err != nil {
		t.Fatal(err)
	}
	definitions, diagnostics, err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(diagnostics) != 0 || len(definitions) != 1 {
		t.Fatalf("Discover() = (%v, %v), want one definition and no diagnostics", definitions, diagnostics)
	}
	got := definitions[0]
	if got.ID != "reviewer" || got.Name != "Code Reviewer" || got.Role != "Reviews changes" {
		t.Fatalf("definition identity = %#v", got)
	}
	if got.Instructions != "Review the diff and report risks." || len(got.Skills) != 1 || got.Skills[0] != "go-helper" {
		t.Fatalf("definition content = %#v", got)
	}
	if got.Compatibility.Agents[0] != "codex" || got.RequiredCapabilities[0] != resource.CapabilityFilesystemRead {
		t.Fatalf("definition declarations = %#v", got)
	}
}

func TestDiscoverReportsMalformedDefinitionWithActionableDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeDefinition(t, root, "broken.yaml", "version: v2\nid: ../unsafe\nname: Broken\n")

	registry, err := subagent.NewRegistry(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	definitions, diagnostics, err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(definitions) != 0 || len(diagnostics) != 1 {
		t.Fatalf("Discover() = (%v, %v), want one diagnostic", definitions, diagnostics)
	}
	message := diagnostics[0].Error()
	if !strings.Contains(message, "broken.yaml") || !strings.Contains(message, "version") {
		t.Errorf("diagnostic = %q, want file and version guidance", message)
	}
}

func TestDiscoverReportsMissingSkillReference(t *testing.T) {
	root := t.TempDir()
	writeDefinition(t, root, "reviewer.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
skills:
  - absent-skill
`)

	registry, err := subagent.NewRegistry(root, func(string) bool { return false })

	if err != nil {
		t.Fatal(err)
	}
	definitions, diagnostics, err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(definitions) != 0 || len(diagnostics) != 1 {
		t.Fatalf("Discover() = (%v, %v), want missing-reference diagnostic", definitions, diagnostics)
	}
	message := diagnostics[0].Error()
	if !strings.Contains(message, "absent-skill") || !strings.Contains(message, "skill") {
		t.Errorf("diagnostic = %q, want missing skill guidance", message)
	}
}

func TestDiscoverReportsTrailingYAMLDocument(t *testing.T) {
	root := t.TempDir()
	writeDefinition(t, root, "trailing.yaml", `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
---
unexpected: document
`)
	registry, err := subagent.NewRegistry(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	definitions, diagnostics, err := registry.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 0 || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Error(), "multiple") {
		t.Fatalf("Discover() = (%v, %v), want multiple-document diagnostic", definitions, diagnostics)
	}
}

func TestDiscoverRejectsDuplicateIDsWithDiagnosticsForBothFiles(t *testing.T) {
	root := t.TempDir()
	definition := `version: v1
id: reviewer
name: Reviewer
role: Review
instructions: Review changes.
`
	writeDefinition(t, root, "first.yaml", definition)
	writeDefinition(t, root, "second.yaml", definition)
	registry, err := subagent.NewRegistry(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	definitions, diagnostics, err := registry.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 0 || len(diagnostics) != 2 {
		t.Fatalf("Discover() = (%v, %v), want both duplicates rejected", definitions, diagnostics)
	}
	for _, diagnostic := range diagnostics {
		message := diagnostic.Error()
		if !strings.Contains(message, "duplicate id \"reviewer\"") || !strings.Contains(message, ".yaml") {
			t.Errorf("diagnostic = %q, want duplicate ID and counterpart path", message)
		}
	}
}

func TestNewRegistryRejectsRelativeDataRoot(t *testing.T) {
	if _, err := subagent.NewRegistry("relative", nil); err == nil {
		t.Fatal("NewRegistry(relative) error = nil")
	}
}

func writeDefinition(t *testing.T, root, name, contents string) {
	t.Helper()
	dir := filepath.Join(root, subagent.DefinitionsDirectory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
