// Package resource defines agent-neutral managed resource contracts.
package resource

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Kind identifies a managed resource domain.
type Kind string

const (
	Skill    Kind = "skill"
	SubAgent Kind = "subagent"
	Memory   Kind = "memory"
)

// Capability is a behavior an adapter can represent for a resource.
type Capability string

const (
	CapabilityFilesystemRead  Capability = "filesystem-read"
	CapabilityFilesystemWrite Capability = "filesystem-write"
	CapabilityMemoryRead      Capability = "memory-read"
	CapabilityMemoryWrite     Capability = "memory-write"
	CapabilityMemorySearch    Capability = "memory-search"
)

// Provenance records the origin of a managed resource.
type Provenance struct {
	Source string `json:"source" yaml:"source"`
}

// Compatibility names target agents for which resource content is intended.
type Compatibility struct {
	Agents []string `json:"agents,omitempty" yaml:"agents,omitempty"`
}

// ManagedResource is the stable envelope shared by all resource handlers.
type ManagedResource struct {
	Version              string        `json:"version" yaml:"version"`
	ID                   string        `json:"id" yaml:"id"`
	Kind                 Kind          `json:"kind" yaml:"kind"`
	Provenance           Provenance    `json:"provenance" yaml:"provenance"`
	Compatibility        Compatibility `json:"compatibility,omitempty" yaml:"compatibility,omitempty"`
	RequiredCapabilities []Capability  `json:"requiredCapabilities,omitempty" yaml:"requiredCapabilities,omitempty"`
}

// Handler owns validation rules for one managed resource kind.
type Handler interface {
	Kind() Kind
	Validate(ManagedResource) error
}

// SkillHandler validates the existing directory-Skill resource domain.
type SkillHandler struct{}

// NewSkillHandler constructs the handler for managed Skills.
func NewSkillHandler() SkillHandler { return SkillHandler{} }

func (SkillHandler) Kind() Kind { return Skill }

func (SkillHandler) Validate(managed ManagedResource) error {
	if managed.Kind != Skill {
		return fmt.Errorf("Skill handler does not support resource kind %q", managed.Kind)
	}
	return managed.Validate()
}

// Validate rejects unsafe or unsupported resource contract values.
func (r ManagedResource) Validate() error {
	if r.Version != "v1" {
		return fmt.Errorf("unsupported resource version %q", r.Version)
	}
	if r.ID == "" || r.ID == "." || r.ID == ".." || filepath.Base(r.ID) != r.ID || strings.ContainsAny(r.ID, `/\\`) {
		return fmt.Errorf("unsafe resource identifier %q", r.ID)
	}
	if !validKind(r.Kind) {
		return fmt.Errorf("unsupported resource kind %q", r.Kind)
	}
	for _, capability := range r.RequiredCapabilities {
		if !validCapability(capability) {
			return fmt.Errorf("unsupported required capability %q", capability)
		}
	}
	return nil
}

func validKind(kind Kind) bool {
	return kind == Skill || kind == SubAgent || kind == Memory
}

func validCapability(capability Capability) bool {
	switch capability {
	case CapabilityFilesystemRead, CapabilityFilesystemWrite, CapabilityMemoryRead, CapabilityMemoryWrite, CapabilityMemorySearch:
		return true
	default:
		return false
	}
}

// MissingCapabilities returns required capabilities absent from supported in
// the stable order requested by the resource contract.
func MissingCapabilities(required, supported []Capability) []Capability {
	present := make(map[Capability]struct{}, len(supported))
	for _, capability := range supported {
		present[capability] = struct{}{}
	}
	missing := make([]Capability, 0, len(required))
	for _, capability := range required {
		if _, ok := present[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	return missing
}
