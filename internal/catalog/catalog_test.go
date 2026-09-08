package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/catalog"
)

func TestDiscoverReturnsEligibleDirectorySkill(t *testing.T) {
	library := t.TempDir()
	skillDir := filepath.Join(library, "go-helper")
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: Go Helper\ndescription: Help with Go code\n---\nUse gofmt.\n")
	writeFile(t, filepath.Join(skillDir, "reference.md"), "This file must not be interpreted by discovery.")

	skills, diagnostics, err := catalog.Discover(library)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("Discover() diagnostics = %v, want none", diagnostics)
	}
	if len(skills) != 1 {
		t.Fatalf("Discover() returned %d skills, want 1", len(skills))
	}

	skill := skills[0]
	if skill.Identifier != "go-helper" {
		t.Errorf("Identifier = %q, want %q", skill.Identifier, "go-helper")
	}
	if skill.SourcePath != skillDir {
		t.Errorf("SourcePath = %q, want %q", skill.SourcePath, skillDir)
	}
	if skill.Name != "Go Helper" {
		t.Errorf("Name = %q, want %q", skill.Name, "Go Helper")
	}
	if skill.Description != "Help with Go code" {
		t.Errorf("Description = %q, want %q", skill.Description, "Help with Go code")
	}
	if skill.Body != "Use gofmt.\n" {
		t.Errorf("Body = %q, want %q", skill.Body, "Use gofmt.\n")
	}
}

func TestDiscoverExcludesInvalidLibraryChildrenAndReportsDiagnostic(t *testing.T) {
	library := t.TempDir()
	writeFile(t, filepath.Join(library, "not-a-skill", "readme.md"), "not eligible")
	writeFile(t, filepath.Join(library, "broken", "SKILL.md"), "---\nname: Broken\n---\nbody")
	writeFile(t, filepath.Join(library, "plain-file"), "not a directory")

	skills, diagnostics, err := catalog.Discover(library)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("Discover() returned %d skills, want none", len(skills))
	}
	if len(diagnostics) != 2 {
		t.Fatalf("Discover() diagnostics = %v, want diagnostics for two invalid directories", diagnostics)
	}
}

func TestDiscoverReadsCompanionMetadata(t *testing.T) {
	library := t.TempDir()
	skillDir := filepath.Join(library, "catalog-tool")
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: Catalog Tool\ndescription: Catalog work\n---\nBody\n")
	writeFile(t, filepath.Join(skillDir, ".skill-manager.yaml"), "tags:\n  - go\n  - catalog\ncompatibility:\n  - codex\n  - claude-code\nprovenance: adopted\n")

	skills, diagnostics, err := catalog.Discover(library)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(diagnostics) != 0 || len(skills) != 1 {
		t.Fatalf("Discover() = (%v, %v), want one skill and no diagnostics", skills, diagnostics)
	}
	skill := skills[0]
	if got, want := skill.Tags, []string{"go", "catalog"}; !sameStrings(got, want) {
		t.Errorf("Tags = %v, want %v", got, want)
	}
	if got, want := skill.Compatibility, []string{"codex", "claude-code"}; !sameStrings(got, want) {
		t.Errorf("Compatibility = %v, want %v", got, want)
	}
	if skill.Provenance != "adopted" {
		t.Errorf("Provenance = %q, want %q", skill.Provenance, "adopted")
	}
}

func TestDiscoverRejectsMalformedYAML(t *testing.T) {
	library := t.TempDir()
	writeFile(t, filepath.Join(library, "bad-frontmatter", "SKILL.md"), "---\nname: [not closed\ndescription: Broken\n---\nBody\n")
	writeFile(t, filepath.Join(library, "bad-metadata", "SKILL.md"), "---\nname: Metadata\ndescription: Broken metadata\n---\nBody\n")
	writeFile(t, filepath.Join(library, "bad-metadata", ".skill-manager.yaml"), "tags: [not closed\n")

	skills, diagnostics, err := catalog.Discover(library)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("Discover() returned %d skills, want no malformed entries", len(skills))
	}
	if len(diagnostics) != 2 {
		t.Fatalf("Discover() diagnostics = %v, want diagnostics for malformed frontmatter and metadata", diagnostics)
	}
}

func TestDiscoverReadsQuotedCommaValuesAndYAMLListStyles(t *testing.T) {
	library := t.TempDir()
	skillDir := filepath.Join(library, "yaml-lists")
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: YAML Lists\ndescription: Metadata list parsing\n---\nBody\n")
	writeFile(t, filepath.Join(skillDir, ".skill-manager.yaml"), "tags: [\"go, tooling\", catalog]\ncompatibility:\n  - codex\n  - \"claude, local\"\nprovenance: \"team, maintained\"\n")

	skills, diagnostics, err := catalog.Discover(library)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(diagnostics) != 0 || len(skills) != 1 {
		t.Fatalf("Discover() = (%v, %v), want one skill and no diagnostics", skills, diagnostics)
	}
	if got, want := skills[0].Tags, []string{"go, tooling", "catalog"}; !sameStrings(got, want) {
		t.Errorf("Tags = %v, want %v", got, want)
	}
	if got, want := skills[0].Compatibility, []string{"codex", "claude, local"}; !sameStrings(got, want) {
		t.Errorf("Compatibility = %v, want %v", got, want)
	}
	if skills[0].Provenance != "team, maintained" {
		t.Errorf("Provenance = %q, want %q", skills[0].Provenance, "team, maintained")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
