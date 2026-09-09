package adapter_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/resource"
)

func TestSupportedProjectAndGlobalLocations(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	cases := []struct {
		target adapter.Target
		path   string
		global string
	}{
		{adapter.ClaudeCode, filepath.Join(project, ".claude", "skills", "demo"), filepath.Join(home, ".claude", "skills", "demo")},
		{adapter.Codex, filepath.Join(project, ".codex", "skills", "demo"), filepath.Join(home, ".codex", "skills", "demo")},
		{adapter.Pi, filepath.Join(project, ".pi", "skills", "demo"), filepath.Join(home, ".pi", "agent", "skills", "demo")},
	}
	for _, tc := range cases {
		t.Run(string(tc.target), func(t *testing.T) {
			a, ok := adapter.For(tc.target)
			if !ok {
				t.Fatalf("adapter %q was not supported", tc.target)
			}
			if got := a.ProjectSkillPath(project, "demo"); got != tc.path {
				t.Fatalf("project path = %q, want %q", got, tc.path)
			}
			if got := a.GlobalSkillPath(home, "demo"); got != tc.global {
				t.Fatalf("global path = %q, want %q", got, tc.global)
			}
			if !a.Supports(resource.Skill) {
				t.Fatalf("%s does not declare Skill support", tc.target)
			}
			if !a.HasCapability(resource.Skill, resource.CapabilityFilesystemWrite) {
				t.Fatalf("%s does not declare filesystem write capability", tc.target)
			}
		})
	}
}

func TestValidateIdentifierRejectsTraversalAndPathForms(t *testing.T) {
	for _, identifier := range []string{"", ".", "..", "a/b", "a\\b", "../demo", "demo/.."} {
		if err := adapter.ValidateIdentifier(identifier); err == nil {
			t.Fatalf("ValidateIdentifier(%q) accepted unsafe identifier", identifier)
		}
	}
	if err := adapter.ValidateIdentifier("safe-skill_2"); err != nil {
		t.Fatal(err)
	}
}

func TestSupportedAdaptersExposeAgentNeutralLifecycleContract(t *testing.T) {
	for _, a := range adapter.SupportedAgents() {
		var contract adapter.AgentAdapter = a
		status := contract.Detect()
		if status.Target != a.Target() || !status.Available {
			t.Fatalf("Detect() = %#v, want available %q", status, a.Target())
		}
		item := resource.ManagedResource{Version: "v1", ID: "demo", Kind: resource.Skill}
		if _, err := contract.Inspect(resource.Skill, item); err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		plan, err := contract.Plan(resource.Skill, item)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if plan.Resource.ID != item.ID {
			t.Fatalf("Plan() resource ID = %q, want %q", plan.Resource.ID, item.ID)
		}
	}
}

func TestAdapterPlacementRejectsUnimplementedMutation(t *testing.T) {
	a, ok := adapter.ForAgent(adapter.Codex)
	if !ok {
		t.Fatal("Codex adapter is unavailable")
	}
	plan, err := a.Plan(resource.Skill, resource.ManagedResource{Version: "v1", ID: "demo", Kind: resource.Skill})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Place(plan); !errors.Is(err, adapter.ErrPlacementUnsupported) {
		t.Fatalf("Place() error = %v, want ErrPlacementUnsupported", err)
	}
}

func TestCompareCapabilitiesReportsMissingWithoutPlanning(t *testing.T) {
	a, ok := adapter.For(adapter.Codex)
	if !ok {
		t.Fatal("Codex adapter is unavailable")
	}
	result := adapter.CompareCapabilities(resource.CapabilityRequest{
		Kind:     resource.Skill,
		Required: []resource.Capability{resource.CapabilityFilesystemRead, resource.CapabilityMemorySearch},
	}, a)
	if result.IsSupported() {
		t.Fatal("capability comparison reported unsupported capability as supported")
	}
	if len(result.Missing) != 1 || result.Missing[0] != resource.CapabilityMemorySearch {
		t.Fatalf("missing capabilities = %v, want [%s]", result.Missing, resource.CapabilityMemorySearch)
	}
	if len(result.Supported) != 2 {
		t.Fatalf("supported capabilities = %v, want target declaration", result.Supported)
	}
}

func TestPlanRejectsMissingCapabilitiesBeforePlacement(t *testing.T) {
	a, ok := adapter.ForAgent(adapter.Codex)
	if !ok {
		t.Fatal("Codex adapter is unavailable")
	}
	item := resource.ManagedResource{
		Version:              "v1",
		ID:                   "demo",
		Kind:                 resource.Skill,
		RequiredCapabilities: []resource.Capability{resource.CapabilityMemorySearch},
	}
	plan, err := a.Plan(resource.Skill, item)
	if !errors.Is(err, resource.ErrUnsupportedCapabilities) {
		t.Fatalf("Plan() error = %v, want unsupported capabilities", err)
	}
	if len(plan.Missing) != 1 || len(plan.Capabilities) == 0 {
		t.Fatalf("Plan() = %#v, want explicit missing capability result", plan)
	}
}

func TestPiRejectsUnsupportedResourceKindsBeforePlacement(t *testing.T) {
	a, ok := adapter.ForAgent(adapter.Pi)
	if !ok {
		t.Fatal("Pi adapter is unavailable")
	}
	for _, kind := range []resource.Kind{resource.SubAgent, resource.Memory} {
		t.Run(string(kind), func(t *testing.T) {
			item := resource.ManagedResource{Version: "v1", ID: "demo", Kind: kind}
			result := adapter.CompareCapabilities(resource.CapabilityRequest{Kind: kind}, a)
			if result.KindSupported {
				t.Fatal("unsupported kind was reported as supported")
			}
			if result.IsSupported() {
				t.Fatal("unsupported kind passed capability comparison")
			}
			if _, err := a.Inspect(kind, item); err == nil {
				t.Fatal("Inspect accepted unsupported kind")
			}
			if _, err := a.Plan(kind, item); err == nil {
				t.Fatal("Plan accepted unsupported kind")
			}
		})
	}
}

func TestPiUnsupportedResourcePlacementDoesNotWrite(t *testing.T) {
	a, ok := adapter.ForAgent(adapter.Pi)
	if !ok {
		t.Fatal("Pi adapter is unavailable")
	}
	for _, kind := range []resource.Kind{resource.SubAgent, resource.Memory} {
		t.Run(string(kind), func(t *testing.T) {
			plan := resource.PlacementPlan{Resource: resource.ManagedResource{
				Version: "v1", ID: "demo", Kind: kind,
			}}
			if err := a.Place(plan); err == nil {
				t.Fatal("Place accepted unsupported kind")
			}
		})
	}
}

func TestLegacyAdapterAccessorsRemainAssignable(t *testing.T) {
	var legacy adapter.Adapter
	legacy, ok := adapter.For(adapter.Codex)
	if !ok || legacy == nil {
		t.Fatal("For() did not return a legacy Adapter")
	}
	if got := adapter.Supported(); len(got) == 0 {
		t.Fatal("Supported() returned no legacy adapters")
	}
}
