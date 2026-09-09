// Package resource defines agent-neutral managed resource contracts.
package resource

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/catalog"
)

// ErrDetectionUnsupported indicates that a handler requires a concrete
// storage integration before it can discover resources.
var ErrDetectionUnsupported = errors.New("resource detection is not supported")

// ErrUnsupportedCapabilities indicates that a target cannot represent a
// requested resource capability.
var ErrUnsupportedCapabilities = errors.New("resource capabilities are unsupported")

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
	Version              string            `json:"version" yaml:"version"`
	ID                   string            `json:"id" yaml:"id"`
	Kind                 Kind              `json:"kind" yaml:"kind"`
	Provenance           Provenance        `json:"provenance" yaml:"provenance"`
	Compatibility        Compatibility     `json:"compatibility,omitempty" yaml:"compatibility,omitempty"`
	RequiredCapabilities []Capability      `json:"requiredCapabilities,omitempty" yaml:"requiredCapabilities,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// DetectionRequest describes an agent-neutral discovery request. Runtime
// locations are intentionally supplied by the integration, not this contract.
type DetectionRequest struct {
	Kind Kind
	// Root is the configured catalog root for handlers that discover local
	// resources. An empty root preserves the validation-only behavior.
	Root string
}

// Inspection is the stable result of examining a managed resource.
type Inspection struct {
	Resource ManagedResource
	Warnings []string
}

// PlanRequest supplies target capabilities without exposing runtime paths.
type PlanRequest struct {
	Capabilities []Capability
}

// PlacementPlan is an agent-neutral operation description. An adapter may
// translate it into a runtime-specific destination when applying the plan.
type PlacementPlan struct {
	Resource     ManagedResource
	Capabilities []Capability
	Missing      []Capability
	Warnings     []string
}

// ResourceHandler owns discovery, inspection, validation, and lifecycle
// planning rules for one managed resource kind.
type ResourceHandler interface {
	Kind() Kind
	Detect(DetectionRequest) ([]ManagedResource, error)
	Inspect(ManagedResource) (Inspection, error)
	Validate(ManagedResource) error
	Plan(ManagedResource, PlanRequest) (PlacementPlan, error)
}

// Handler is the legacy validation-only contract retained for source
// compatibility. New code should use ResourceHandler.
type Handler interface {
	Kind() Kind
	Validate(ManagedResource) error
}

// SkillHandler validates the existing directory-Skill resource domain.
type SkillHandler struct{}

// NewSkillHandler constructs the handler for managed Skills.
func NewSkillHandler() SkillHandler { return SkillHandler{} }

func (SkillHandler) Kind() Kind { return Skill }

func (SkillHandler) Detect(request DetectionRequest) ([]ManagedResource, error) {
	if request.Kind != "" && request.Kind != Skill {
		return nil, fmt.Errorf("Skill handler does not support resource kind %q", request.Kind)
	}
	if request.Root == "" {
		return nil, ErrDetectionUnsupported
	}
	skills, _, err := catalog.Discover(request.Root)
	if err != nil {
		return nil, err
	}
	resources := make([]ManagedResource, 0, len(skills))
	for _, skill := range skills {
		resources = append(resources, SkillResource(skill))
	}
	return resources, nil
}

// SkillResource normalizes a catalog Skill into the agent-neutral contract.
// The source path remains provenance; placement is still owned by an adapter.
func SkillResource(skill catalog.Skill) ManagedResource {
	metadata := map[string]string{}
	if skill.Name != "" {
		metadata["name"] = skill.Name
	}
	if skill.Description != "" {
		metadata["description"] = skill.Description
	}
	if len(skill.Tags) > 0 {
		metadata["tags"] = strings.Join(skill.Tags, ",")
	}
	return ManagedResource{
		Version:              "v1",
		ID:                   skill.Identifier,
		Kind:                 Skill,
		Provenance:           Provenance{Source: skill.SourcePath},
		Compatibility:        Compatibility{Agents: append([]string(nil), skill.Compatibility...)},
		RequiredCapabilities: []Capability{CapabilityFilesystemRead, CapabilityFilesystemWrite},
		Metadata:             metadata,
	}
}

func (h SkillHandler) Inspect(managed ManagedResource) (Inspection, error) {
	if err := h.Validate(managed); err != nil {
		return Inspection{}, err
	}
	return Inspection{Resource: managed}, nil
}

func (SkillHandler) Validate(managed ManagedResource) error {
	if managed.Kind != Skill {
		return fmt.Errorf("Skill handler does not support resource kind %q", managed.Kind)
	}
	return managed.Validate()
}

func (h SkillHandler) Plan(managed ManagedResource, request PlanRequest) (PlacementPlan, error) {
	if err := h.Validate(managed); err != nil {
		return PlacementPlan{}, err
	}
	return PlacementPlan{
		Resource:     managed,
		Capabilities: append([]Capability(nil), request.Capabilities...),
		Missing:      MissingCapabilities(managed.RequiredCapabilities, request.Capabilities),
	}, nil
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
