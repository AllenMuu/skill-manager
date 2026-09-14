// Package config loads the local Skill Manager configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config contains machine-local Skill Manager settings.
type Config struct {
	Version     string
	LibraryPath string
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
	version, library, err := parseConfiguration(string(contents))
	if err != nil {
		return Config{}, fmt.Errorf("parse configuration: %w", err)
	}
	if library == "" {
		defaultPath, err := defaultLibraryPath()
		if err != nil {
			return Config{}, err
		}
		return Config{Version: version, LibraryPath: defaultPath}, nil
	}
	if !filepath.IsAbs(library) {
		library = filepath.Join(filepath.Dir(path), library)
	}
	library, err = filepath.Abs(library)
	if err != nil {
		return Config{}, fmt.Errorf("resolve configured library: %w", err)
	}
	return Config{Version: version, LibraryPath: library}, nil
}

func defaultLibraryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

func parseConfiguration(contents string) (string, string, error) {
	var fileConfig struct {
		Version string `yaml:"version"`
		Library string `yaml:"library"`
	}
	if err := yaml.Unmarshal([]byte(contents), &fileConfig); err != nil {
		return "", "", err
	}
	if fileConfig.Version == "" {
		fileConfig.Version = "v1"
	}
	if fileConfig.Version != "v1" {
		return "", "", fmt.Errorf("unsupported configuration version %q", fileConfig.Version)
	}
	return fileConfig.Version, fileConfig.Library, nil
}
