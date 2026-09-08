// Package adapter defines supported agent skill placement conventions.
package adapter

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Target identifies a supported coding agent.
type Target string

const (
	ClaudeCode Target = "claude-code"
	Codex      Target = "codex"
)

// Adapter maps a skill identifier to a project or global skill location.
type Adapter interface {
	Target() Target
	ProjectSkillPath(project, identifier string) string
	GlobalSkillPath(home, identifier string) string
}

type directoryAdapter struct {
	target Target
	dir    string
}

func (a directoryAdapter) Target() Target { return a.target }

func (a directoryAdapter) ProjectSkillPath(project, identifier string) string {
	return filepath.Join(project, a.dir, "skills", identifier)
}

func (a directoryAdapter) GlobalSkillPath(home, identifier string) string {
	return filepath.Join(home, a.dir, "skills", identifier)
}

var supported = map[Target]Adapter{
	ClaudeCode: directoryAdapter{target: ClaudeCode, dir: ".claude"},
	Codex:      directoryAdapter{target: Codex, dir: ".codex"},
}

// For returns the adapter for target when that target is supported.
func For(target Target) (Adapter, bool) {
	a, ok := supported[target]
	return a, ok
}

// ValidateIdentifier accepts one portable skill-directory name, never a path.
func ValidateIdentifier(identifier string) error {
	if identifier == "" || identifier == "." || identifier == ".." || strings.ContainsAny(identifier, `/\\`) || filepath.Base(identifier) != identifier {
		return fmt.Errorf("unsafe skill identifier %q", identifier)
	}
	return nil
}

// Supported returns all supported adapters in stable target order.
func Supported() []Adapter {
	return []Adapter{supported[ClaudeCode], supported[Codex]}
}
