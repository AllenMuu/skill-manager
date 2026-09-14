// Package config loads the local Skill Manager configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AllenMuu/skill-manager/internal/memory"
	"gopkg.in/yaml.v3"
)

// Config contains machine-local Skill Manager settings.
type Config struct {
	Version     string
	LibraryPath string
	Memory      *memory.ProviderConfig
}

// Load returns the default skill library when path is empty. A supplied config
// file may set its library field to select another local skill library.
func Load(path string) (Config, error) {
	if path == "" {
		defaultPath, err := defaultLibraryPath()
		if err != nil {
			return Config{}, err
		}
		return Config{Version: "v1", LibraryPath: defaultPath}, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration: %w", err)
	}
	version, library, provider, err := parseConfiguration(string(contents))
	if err != nil {
		return Config{}, fmt.Errorf("parse configuration: %w", err)
	}
	if library == "" {
		defaultPath, err := defaultLibraryPath()
		if err != nil {
			return Config{}, err
		}
		return Config{Version: version, LibraryPath: defaultPath, Memory: provider}, nil
	}
	if !filepath.IsAbs(library) {
		library = filepath.Join(filepath.Dir(path), library)
	}
	library, err = filepath.Abs(library)
	if err != nil {
		return Config{}, fmt.Errorf("resolve configured library: %w", err)
	}
	return Config{Version: version, LibraryPath: library, Memory: provider}, nil
}

func defaultLibraryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

func parseConfiguration(contents string) (string, string, *memory.ProviderConfig, error) {
	var fileConfig struct {
		Version string                 `yaml:"version"`
		Library string                 `yaml:"library"`
		Memory  *memory.ProviderConfig `yaml:"memory"`
	}
	if err := yaml.Unmarshal([]byte(contents), &fileConfig); err != nil {
		return "", "", nil, err
	}
	if fileConfig.Version == "" {
		fileConfig.Version = "v1"
	}
	if fileConfig.Version != "v1" {
		return "", "", nil, fmt.Errorf("unsupported configuration version %q", fileConfig.Version)
	}
	return fileConfig.Version, fileConfig.Library, fileConfig.Memory, nil
}

// SaveMemory stores an explicit provider reference in the selected local
// configuration file. The reference name is persisted only in this local
// configuration; status and journal serialization remain redacted.
func SaveMemory(path string, provider memory.ProviderConfig) error {
	if path == "" {
		return fmt.Errorf("memory provider configuration requires --config")
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return fmt.Errorf("parse configuration: %w", err)
	}
	if document == nil {
		document = make(map[string]any)
	}
	document["version"] = "v1"
	document["memory"] = struct {
		Version       string                 `yaml:"version"`
		ID            string                 `yaml:"id"`
		Provider      string                 `yaml:"provider"`
		Configuration memory.ConfigReference `yaml:"configuration"`
		Scopes        []memory.Scope         `yaml:"scopes"`
		Capabilities  []memory.Capability    `yaml:"capabilities"`
	}{provider.Version, provider.ID, provider.Provider, provider.Configuration, provider.Scopes, provider.Capabilities}
	out, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	return nil
}
