package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/config"
)

func TestLoadUsesAgentsSkillsInHomeDirectoryByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := filepath.Join(home, ".agents", "skills")
	if loaded.LibraryPath != want {
		t.Errorf("LibraryPath = %q, want %q", loaded.LibraryPath, want)
	}
}

func TestLoadUsesConfiguredLibraryPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: /var/lib/skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.LibraryPath != "/var/lib/skills" {
		t.Errorf("LibraryPath = %q, want %q", loaded.LibraryPath, "/var/lib/skills")
	}
}

func TestLoadParsesCommentedYAMLConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("# Skill Manager configuration\nlibrary: \"/var/lib/shared skills\" # local library\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.LibraryPath != "/var/lib/shared skills" {
		t.Errorf("LibraryPath = %q, want %q", loaded.LibraryPath, "/var/lib/shared skills")
	}
}

func TestLoadRejectsMalformedYAMLConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: [not closed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := config.Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want malformed YAML error")
	}
}

func TestLoadExplicitConfigDoesNotRequireHomeDirectory(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("library: /var/lib/skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", "")

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want explicit configuration to load without HOME", err)
	}
	if loaded.LibraryPath != "/var/lib/skills" {
		t.Errorf("LibraryPath = %q, want %q", loaded.LibraryPath, "/var/lib/skills")
	}
}
