// Package adapter defines supported agent skill placement conventions.
package adapter

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AllenMuu/skill-manager/internal/resource"
)

// ErrPlacementUnsupported indicates that generic placement has not yet been
// wired to the guarded lifecycle implementation.
var ErrPlacementUnsupported = errors.New("generic adapter placement is not supported")

// ErrPlacementConfiguration indicates that a guarded placement context was
// not supplied to the generic adapter seam.
var ErrPlacementConfiguration = errors.New("filesystem placement is not configured")

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

// Detection reports whether an agent integration is available to receive
// resources. It contains no filesystem location; placement remains an
// adapter concern.
type Detection struct {
	Target    Target
	Available bool
}

// AgentAdapter is the complete runtime boundary for agent-neutral resources.
// Resource contracts carry identity and metadata, while this interface owns
// target-specific inspection, planning, capability translation, and placement.
type AgentAdapter interface {
	Adapter
	Detect() Detection
	Inspect(resource.Kind, resource.ManagedResource) (resource.Inspection, error)
	Validate(resource.Kind, resource.ManagedResource) error
	Plan(resource.Kind, resource.ManagedResource) (resource.PlacementPlan, error)
	Place(resource.PlacementPlan) error
}

// CompareCapabilities compares a resource request with a target adapter's
// declaration. Callers should inspect IsSupported before constructing or
// confirming a mutation plan.
func CompareCapabilities(request resource.CapabilityRequest, target Adapter) resource.CapabilityResult {
	if target == nil {
		return resource.CapabilityResult{Missing: append([]resource.Capability(nil), request.Required...)}
	}
	kindSupported := target.Supports(request.Kind)
	supported := append([]resource.Capability(nil), target.Capabilities(request.Kind)...)
	return resource.CapabilityResult{
		KindSupported: kindSupported,
		Supported:     supported,
		Missing:       resource.MissingCapabilities(request.Required, supported),
	}
}

type directoryAdapter struct {
	target    Target
	dir       string
	globalDir string
}

func (a directoryAdapter) Target() Target { return a.target }

func (a directoryAdapter) Detect() Detection {
	return Detection{Target: a.target, Available: true}
}

func (a directoryAdapter) Inspect(kind resource.Kind, managed resource.ManagedResource) (resource.Inspection, error) {
	if !a.Supports(kind) {
		return resource.Inspection{}, fmt.Errorf("%s does not support resource kind %q", a.target, kind)
	}
	if managed.Kind != kind {
		return resource.Inspection{}, fmt.Errorf("resource kind %q does not match inspection kind %q", managed.Kind, kind)
	}
	if err := managed.Validate(); err != nil {
		return resource.Inspection{}, err
	}
	return resource.Inspection{Resource: managed}, nil
}

func (a directoryAdapter) Validate(kind resource.Kind, managed resource.ManagedResource) error {
	if !a.Supports(kind) {
		return fmt.Errorf("%s does not support resource kind %q", a.target, kind)
	}
	if managed.Kind != kind {
		return fmt.Errorf("resource kind %q does not match validation kind %q", managed.Kind, kind)
	}
	return managed.Validate()
}

func (a directoryAdapter) Plan(kind resource.Kind, managed resource.ManagedResource) (resource.PlacementPlan, error) {
	if err := a.Validate(kind, managed); err != nil {
		return resource.PlacementPlan{}, err
	}
	capabilities := a.Capabilities(kind)
	plan := resource.PlacementPlan{
		Resource:     managed,
		Capabilities: append([]resource.Capability(nil), capabilities...),
		Missing:      resource.MissingCapabilities(managed.RequiredCapabilities, capabilities),
	}
	if len(plan.Missing) > 0 {
		return plan, fmt.Errorf("%w: missing capabilities %v", resource.ErrUnsupportedCapabilities, plan.Missing)
	}
	return plan, nil
}

// Place validates an adapter-neutral plan. Filesystem mutations continue to
// use the existing explicit path methods and guarded lifecycle services.
func (a directoryAdapter) Place(plan resource.PlacementPlan) error {
	if err := a.Validate(plan.Resource.Kind, plan.Resource); err != nil {
		return err
	}
	if len(plan.Missing) > 0 {
		return fmt.Errorf("%w: missing capabilities %v", resource.ErrUnsupportedCapabilities, plan.Missing)
	}
	if plan.Destination == "" || plan.Journal == nil {
		return ErrPlacementConfiguration
	}
	_, err := PlaceFilesystem(plan, FilesystemPlacementOptions{
		Destination: plan.Destination,
		Conflict:    ConflictStrategy(plan.Conflict),
		Force:       plan.Force,
		Journal:     plan.Journal,
		Confirm:     plan.Confirm,
	})
	return err
}

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

var supported = map[Target]AgentAdapter{
	ClaudeCode: directoryAdapter{target: ClaudeCode, dir: ".claude"},
	Codex:      directoryAdapter{target: Codex, dir: ".codex"},
	Pi:         directoryAdapter{target: Pi, dir: ".pi", globalDir: filepath.Join(".pi", "agent")},
}

// For returns the adapter for target when that target is supported.
func For(target Target) (Adapter, bool) {
	a, ok := supported[target]
	return a, ok
}

// ForAgent returns the richer lifecycle contract for target integrations.
func ForAgent(target Target) (AgentAdapter, bool) {
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

// SupportedAgents returns adapters with the agent-neutral lifecycle contract.
func SupportedAgents() []AgentAdapter {
	return []AgentAdapter{supported[ClaudeCode], supported[Codex], supported[Pi]}
}
