// Package adapter defines supported agent skill placement conventions.
package adapter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/resource"
)

// Target identifies a supported coding agent.
type Target string

const (
	ClaudeCode Target = "claude-code"
	Codex      Target = "codex"
	Pi         Target = "pi"
)

// Adapter maps a skill identifier to a project or global skill location.
type Adapter interface {
	Target() Target
	ProjectSkillPath(project, identifier string) string
	GlobalSkillPath(home, identifier string) string
	ResourceKinds() []resource.Kind
	Capabilities(resource.Kind) []resource.Capability
	Supports(resource.Kind) bool
	HasCapability(resource.Kind, resource.Capability) bool
}

type directoryAdapter struct {
	target    Target
	dir       string
	globalDir string
}

func (a directoryAdapter) Target() Target { return a.target }

func (a directoryAdapter) ProjectSkillPath(project, identifier string) string {
	return filepath.Join(project, a.dir, "skills", identifier)
}

func (a directoryAdapter) GlobalSkillPath(home, identifier string) string {
	dir := a.globalDir
	if dir == "" {
		dir = a.dir
	}
	return filepath.Join(home, dir, "skills", identifier)
}

func (a directoryAdapter) Supports(kind resource.Kind) bool { return kind == resource.Skill }

func (a directoryAdapter) ResourceKinds() []resource.Kind { return []resource.Kind{resource.Skill} }

func (a directoryAdapter) Capabilities(kind resource.Kind) []resource.Capability {
	if !a.Supports(kind) {
		return nil
	}
	return []resource.Capability{resource.CapabilityFilesystemRead, resource.CapabilityFilesystemWrite}
}

func (a directoryAdapter) HasCapability(kind resource.Kind, capability resource.Capability) bool {
	for _, declared := range a.Capabilities(kind) {
		if declared == capability {
			return true
		}
	}
	return false
}

var supported = map[Target]Adapter{
	ClaudeCode: directoryAdapter{target: ClaudeCode, dir: ".claude"},
	Codex:      directoryAdapter{target: Codex, dir: ".codex"},
	Pi:         directoryAdapter{target: Pi, dir: ".pi", globalDir: filepath.Join(".pi", "agent")},
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
	return []Adapter{supported[ClaudeCode], supported[Codex], supported[Pi]}
}
