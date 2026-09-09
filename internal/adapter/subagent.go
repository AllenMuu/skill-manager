package adapter

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/AllenMuu/skill-manager/internal/resource"
	"github.com/AllenMuu/skill-manager/internal/subagent"
)

// ErrSubAgentUnsupported means that a target-specific preview cannot represent
// every canonical SubAgent field or required capability. The returned plan is
// still useful for diagnostics, but must not be passed to a mutating client.
var ErrSubAgentUnsupported = errors.New("SubAgent representation is unsupported")

// SubAgentScope selects the native project or user-level agent directory.
type SubAgentScope string

const (
	SubAgentProject SubAgentScope = "project"
	SubAgentGlobal  SubAgentScope = "global"
)

// SubAgentRequest supplies the already-resolved root for a preview. Root is a
// project directory for project scope and a home directory for global scope.
// This keeps home-directory lookup and filesystem writes outside adapters.
type SubAgentRequest struct {
	Root  string
	Scope SubAgentScope
}

// SubAgentInspection reports target compatibility without rendering or
// writing a native file.
type SubAgentInspection struct {
	Target                  Target
	Definition              subagent.Definition
	Destination             string
	Format                  string
	Supported               bool
	UnsupportedFields       []string
	UnsupportedCapabilities []resource.Capability
}

// SubAgentPlan is a render-only target-specific preview. Installation is
// deliberately not part of Task 4.3; Content is never written here.
type SubAgentPlan struct {
	Target                  Target
	Definition              subagent.Definition
	Destination             string
	Format                  string
	Content                 string
	UnsupportedFields       []string
	UnsupportedCapabilities []resource.Capability
}

// SubAgentAdapter exposes the target-specific SubAgent inspection seam.
type SubAgentAdapter interface {
	AgentAdapter
	InspectSubAgent(subagent.Definition, SubAgentRequest) (SubAgentInspection, error)
	PlanSubAgent(subagent.Definition, SubAgentRequest) (SubAgentPlan, error)
}

func (a directoryAdapter) InspectSubAgent(definition subagent.Definition, request SubAgentRequest) (SubAgentInspection, error) {
	if err := validateSubAgentDefinition(definition); err != nil {
		return SubAgentInspection{}, err
	}
	plan := a.subAgentPlan(definition, request)
	inspection := SubAgentInspection{
		Target: a.target, Definition: definition, Destination: plan.Destination,
		Format: plan.Format, Supported: len(plan.UnsupportedFields) == 0 && len(plan.UnsupportedCapabilities) == 0,
		UnsupportedFields:       append([]string(nil), plan.UnsupportedFields...),
		UnsupportedCapabilities: append([]resource.Capability(nil), plan.UnsupportedCapabilities...),
	}
	return inspection, nil
}

func (a directoryAdapter) PlanSubAgent(definition subagent.Definition, request SubAgentRequest) (SubAgentPlan, error) {
	if err := validateSubAgentDefinition(definition); err != nil {
		return SubAgentPlan{}, err
	}
	plan := a.subAgentPlan(definition, request)
	if !planSupported(plan) {
		return plan, fmt.Errorf("%w for %s: unsupported fields=%v capabilities=%v", ErrSubAgentUnsupported, a.target, plan.UnsupportedFields, plan.UnsupportedCapabilities)
	}
	return plan, nil
}

func (a directoryAdapter) subAgentPlan(definition subagent.Definition, request SubAgentRequest) SubAgentPlan {
	plan := SubAgentPlan{Target: a.target, Definition: definition}
	if request.Root == "" {
		plan.UnsupportedFields = []string{"placement root"}
		return plan
	}
	base := request.Root
	if request.Scope == SubAgentGlobal {
		base = request.Root
	} else if request.Scope != "" && request.Scope != SubAgentProject {
		plan.UnsupportedFields = []string{"placement scope"}
		return plan
	}

	switch a.target {
	case ClaudeCode:
		plan.Format = "claude-code-markdown"
		plan.Destination = filepath.Join(base, ".claude", "agents", definition.ID+".md")
		plan.Content = renderClaudeSubAgent(definition)
		plan.UnsupportedFields = unsupportedCanonicalFields(definition)
	case Codex:
		plan.Format = "codex-toml"
		plan.Destination = filepath.Join(base, ".codex", "agents", definition.ID+".toml")
		plan.Content = renderCodexSubAgent(definition)
		plan.UnsupportedFields = unsupportedCanonicalFields(definition)
	case Pi:
		// Pi's documented native locations contain skills, settings, and
		// context files; its subagent workflow is an extension, not a native
		// definition format. Never guess a write location.
		plan.UnsupportedFields = []string{"id", "name", "role", "instructions", "skills", "compatibility", "requiredCapabilities"}
	}
	plan.UnsupportedCapabilities = unsupportedSubAgentCapabilities(a, definition.RequiredCapabilities)
	return plan
}

func validateSubAgentDefinition(definition subagent.Definition) error {
	return definition.Validate(func(string) bool { return true })
}

func unsupportedSubAgentCapabilities(a directoryAdapter, required []resource.Capability) []resource.Capability {
	return resource.MissingCapabilities(required, a.Capabilities(resource.SubAgent))
}

func unsupportedCanonicalFields(definition subagent.Definition) []string {
	fields := make([]string, 0, 2)
	if len(definition.Skills) > 0 {
		fields = append(fields, "skills")
	}
	if len(definition.Compatibility.Agents) > 0 {
		fields = append(fields, "compatibility")
	}
	return fields
}

func planSupported(plan SubAgentPlan) bool {
	return plan.Destination != "" && plan.Content != "" && len(plan.UnsupportedFields) == 0 && len(plan.UnsupportedCapabilities) == 0
}

func renderClaudeSubAgent(definition subagent.Definition) string {
	description := definition.Name
	if definition.Role != "" {
		description += " — " + definition.Role
	}
	return "---\nname: " + definition.ID + "\ndescription: " + strconv.Quote(description) + "\n---\n\n" + definition.Instructions + "\n"
}

func renderCodexSubAgent(definition subagent.Definition) string {
	description := definition.Name
	if definition.Role != "" {
		description += " — " + definition.Role
	}
	return "name = " + strconv.Quote(definition.ID) + "\ndescription = " + strconv.Quote(description) + "\ndeveloper_instructions = " + strconv.Quote(definition.Instructions) + "\n"
}

// InspectSubAgent is a convenience for a project-scoped preview.
func InspectSubAgent(target Target, definition subagent.Definition, project string) (SubAgentInspection, error) {
	a, ok := ForAgent(target)
	if !ok {
		return SubAgentInspection{}, fmt.Errorf("unsupported target %q", target)
	}
	renderer, ok := a.(SubAgentAdapter)
	if !ok {
		return SubAgentInspection{}, fmt.Errorf("target %q has no SubAgent adapter", target)
	}
	return renderer.InspectSubAgent(definition, SubAgentRequest{Root: project, Scope: SubAgentProject})
}

// PlanSubAgent is a convenience for a project-scoped render preview.
func PlanSubAgent(target Target, definition subagent.Definition, project string) (SubAgentPlan, error) {
	a, ok := ForAgent(target)
	if !ok {
		return SubAgentPlan{}, fmt.Errorf("unsupported target %q", target)
	}
	renderer, ok := a.(SubAgentAdapter)
	if !ok {
		return SubAgentPlan{}, fmt.Errorf("target %q has no SubAgent adapter", target)
	}
	return renderer.PlanSubAgent(definition, SubAgentRequest{Root: project, Scope: SubAgentProject})
}
