package resource_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/resource"
)

func TestValidateAcceptsVersionedResourceContract(t *testing.T) {
	r := resource.ManagedResource{
		Version: "v1",
		ID:      "java-reviewer",
		Kind:    resource.SubAgent,
		Provenance: resource.Provenance{
			Source: "local",
		},
		Compatibility: resource.Compatibility{Agents: []string{"codex"}},
		RequiredCapabilities: []resource.Capability{
			resource.CapabilityFilesystemWrite,
		},
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidResourceContract(t *testing.T) {
	tests := []resource.ManagedResource{
		{Version: "v1", ID: "../unsafe", Kind: resource.Skill},
		{Version: "v2", ID: "valid", Kind: resource.Skill},
		{Version: "v1", ID: "valid", Kind: "unknown"},
		{Version: "v1", ID: "valid", Kind: resource.Memory, RequiredCapabilities: []resource.Capability{"unknown"}},
	}
	for _, r := range tests {
		t.Run(r.ID+string(r.Kind), func(t *testing.T) {
			if err := r.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			} else if strings.TrimSpace(err.Error()) == "" {
				t.Fatal("Validate() returned an empty error")
			}
		})
	}
}

func TestSkillHandlerAcceptsOnlySkillResources(t *testing.T) {
	handler := resource.NewSkillHandler()
	if handler.Kind() != resource.Skill {
		t.Fatalf("Kind() = %q, want skill", handler.Kind())
	}
	if err := handler.Validate(resource.ManagedResource{Version: "v1", ID: "demo", Kind: resource.Skill}); err != nil {
		t.Fatalf("Validate(Skill) error = %v", err)
	}
	if err := handler.Validate(resource.ManagedResource{Version: "v1", ID: "reviewer", Kind: resource.SubAgent}); err == nil {
		t.Fatal("Validate(SubAgent) error = nil")
	}
}

func TestMissingCapabilitiesPreservesRequiredOrder(t *testing.T) {
	missing := resource.MissingCapabilities(
		[]resource.Capability{resource.CapabilityFilesystemRead, resource.CapabilityFilesystemWrite, resource.CapabilityMemorySearch},
		[]resource.Capability{resource.CapabilityFilesystemRead},
	)
	want := []resource.Capability{resource.CapabilityFilesystemWrite, resource.CapabilityMemorySearch}
	if len(missing) != len(want) {
		t.Fatalf("MissingCapabilities() = %v, want %v", missing, want)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Fatalf("MissingCapabilities() = %v, want %v", missing, want)
		}
	}
}

func TestResourceHandlerContractIsAgentNeutral(t *testing.T) {
	var handler resource.ResourceHandler = resource.NewSkillHandler()
	item := resource.ManagedResource{Version: "v1", ID: "demo", Kind: resource.Skill}
	inspection, err := handler.Inspect(item)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspection.Resource.ID != item.ID {
		t.Fatalf("Inspect() resource ID = %q, want %q", inspection.Resource.ID, item.ID)
	}
	plan, err := handler.Plan(item, resource.PlanRequest{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Resource.ID != item.ID {
		t.Fatalf("Plan() resource ID = %q, want %q", plan.Resource.ID, item.ID)
	}
}

func TestSkillHandlerDetectionIsExplicitlyUnsupported(t *testing.T) {
	handler := resource.NewSkillHandler()
	if _, err := handler.Detect(resource.DetectionRequest{Kind: resource.Skill}); !errors.Is(err, resource.ErrDetectionUnsupported) {
		t.Fatalf("Detect(valid) error = %v, want ErrDetectionUnsupported", err)
	}
	if _, err := handler.Detect(resource.DetectionRequest{Kind: resource.Memory}); err == nil {
		t.Fatal("Detect(invalid) error = nil")
	}
}
