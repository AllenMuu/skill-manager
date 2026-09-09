package adapter_test

import (
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/resource"
)

func TestSupportedProjectAndGlobalLocations(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	cases := []struct {
		target adapter.Target
		path   string
		global string
	}{
		{adapter.ClaudeCode, filepath.Join(project, ".claude", "skills", "demo"), filepath.Join(home, ".claude", "skills", "demo")},
		{adapter.Codex, filepath.Join(project, ".codex", "skills", "demo"), filepath.Join(home, ".codex", "skills", "demo")},
		{adapter.Pi, filepath.Join(project, ".pi", "skills", "demo"), filepath.Join(home, ".pi", "agent", "skills", "demo")},
	}
	for _, tc := range cases {
		t.Run(string(tc.target), func(t *testing.T) {
			a, ok := adapter.For(tc.target)
			if !ok {
				t.Fatalf("adapter %q was not supported", tc.target)
			}
			if got := a.ProjectSkillPath(project, "demo"); got != tc.path {
				t.Fatalf("project path = %q, want %q", got, tc.path)
			}
			if got := a.GlobalSkillPath(home, "demo"); got != tc.global {
				t.Fatalf("global path = %q, want %q", got, tc.global)
			}
			if !a.Supports(resource.Skill) {
				t.Fatalf("%s does not declare Skill support", tc.target)
			}
			if !a.HasCapability(resource.Skill, resource.CapabilityFilesystemWrite) {
				t.Fatalf("%s does not declare filesystem write capability", tc.target)
			}
		})
	}
}

func TestValidateIdentifierRejectsTraversalAndPathForms(t *testing.T) {
	for _, identifier := range []string{"", ".", "..", "a/b", "a\\b", "../demo", "demo/.."} {
		if err := adapter.ValidateIdentifier(identifier); err == nil {
			t.Fatalf("ValidateIdentifier(%q) accepted unsafe identifier", identifier)
		}
	}
	if err := adapter.ValidateIdentifier("safe-skill_2"); err != nil {
		t.Fatal(err)
	}
}
