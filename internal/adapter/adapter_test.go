package adapter_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/adapter"
	"github.com/AllenMuu/skill-manager/internal/operation"
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
	if err := a.Place(plan); !errors.Is(err, adapter.ErrPlacementConfiguration) {
		t.Fatalf("Place() error = %v, want placement configuration error", err)
	}
}

func TestAdapterPlaceUsesGuardedFilesystemPlacement(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	a, ok := adapter.ForAgent(adapter.Codex)
	if !ok {
		t.Fatal("Codex adapter is unavailable")
	}
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}, Destination: destination, Journal: journal,
		Confirm: func(operation.Plan) bool { return true }}
	if err := a.Place(plan); err != nil {
		t.Fatalf("Place() error = %v", err)
	}
	if target, err := os.Readlink(destination); err != nil || target != source {
		t.Fatalf("destination = %q, %v; want source link", target, err)
	}
}

func TestPlaceFilesystemRejectsLateConflictWithoutOverwriting(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	_, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination, Journal: journal,
		BeforePublish: func() error {
			return os.WriteFile(destination, []byte("late unmanaged"), 0o644)
		},
		Confirm: func(operation.Plan) bool { return true },
	})
	if !errors.Is(err, adapter.ErrUnsafePath) {
		t.Fatalf("PlaceFilesystem() error = %v, want unsafe path", err)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil || string(got) != "late unmanaged" {
		t.Fatalf("late unmanaged destination changed: %q, %v", got, readErr)
	}
}

func TestPlaceFilesystemRejectsChangedConflictWithoutOverwriting(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(destination, []byte("original unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	_, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination, Conflict: adapter.ConflictReplace, Force: true, Journal: journal,
		BeforePublish: func() error {
			return os.WriteFile(destination, []byte("late unmanaged"), 0o644)
		},
		Confirm: func(operation.Plan) bool { return true },
	})
	if !errors.Is(err, adapter.ErrUnsafePath) {
		t.Fatalf("PlaceFilesystem() error = %v, want unsafe path", err)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil || string(got) != "late unmanaged" {
		t.Fatalf("changed unmanaged destination overwritten: %q, %v", got, readErr)
	}
}

func TestPlaceFilesystemRejectsChangedDirectoryConflictWithoutOverwriting(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "original.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	_, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination, Conflict: adapter.ConflictReplace, Force: true, Journal: journal,
		BeforePublish: func() error {
			return os.WriteFile(filepath.Join(destination, "late.txt"), []byte("late unmanaged"), 0o644)
		},
		Confirm: func(operation.Plan) bool { return true },
	})
	if !errors.Is(err, adapter.ErrUnsafePath) {
		t.Fatalf("PlaceFilesystem() error = %v, want unsafe path", err)
	}
	got, readErr := os.ReadFile(filepath.Join(destination, "late.txt"))
	if readErr != nil || string(got) != "late unmanaged" {
		t.Fatalf("changed unmanaged directory was overwritten: %q, %v", got, readErr)
	}
}

func TestPlaceFilesystemRejectsConflictCreatedAfterAtomicStaging(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "original.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	_, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination, Conflict: adapter.ConflictReplace, Force: true, Journal: journal,
		BeforeRemove: func() error {
			return os.WriteFile(destination, []byte("late unmanaged"), 0o644)
		},
		Confirm: func(operation.Plan) bool { return true },
	})
	if !errors.Is(err, adapter.ErrUnsafePath) {
		t.Fatalf("PlaceFilesystem() error = %v, want unsafe path", err)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil || string(got) != "late unmanaged" {
		t.Fatalf("late unmanaged destination overwritten: %q, %v", got, readErr)
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

func TestPlaceFilesystemRefusesUnmanagedConflictWithoutForce(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(destination, []byte("user-owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	_, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination,
		Conflict:    adapter.ConflictReplace,
		Journal:     journal,
		Confirm:     func(operation.Plan) bool { t.Fatal("confirmation must not be requested"); return true },
	})
	if !errors.Is(err, adapter.ErrForceRequired) {
		t.Fatalf("PlaceFilesystem() error = %v, want force-required", err)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil || string(got) != "user-owned" {
		t.Fatalf("unmanaged destination changed: %q, %v", got, readErr)
	}
}

func TestPlaceFilesystemRequiresConfirmationAndJournalsReplacement(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "run.sh"), []byte("#!/bin/sh\nprintf unsafe\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(destination, []byte("user-owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	confirmed := false
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	preview, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination,
		Conflict:    adapter.ConflictReplace,
		Force:       true,
		Journal:     journal,
		Confirm: func(p operation.Plan) bool {
			confirmed = true
			return len(p.Changes) == 1
		},
	})
	if err != nil {
		t.Fatalf("PlaceFilesystem() error = %v", err)
	}
	if !confirmed || len(preview.Changes) != 1 {
		t.Fatalf("preview = %#v, confirmed = %v", preview, confirmed)
	}
	target, err := os.Readlink(destination)
	if err != nil || target != source {
		t.Fatalf("destination = %q, %v; want link to source", target, err)
	}
	entry, ok, err := journal.Latest()
	if err != nil || !ok || len(entry.Before) != 1 {
		t.Fatalf("journal latest = %#v, %v, %v", entry, ok, err)
	}
	if err := journal.UndoLatest(func(operation.Plan) bool { return true }); err != nil {
		t.Fatalf("UndoLatest() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "user-owned" {
		t.Fatalf("undo restored %q, %v; want unmanaged content", got, err)
	}
}

func TestPlaceFilesystemDoesNotExecuteExecutableResourceContent(t *testing.T) {
	source := t.TempDir()
	marker := filepath.Join(t.TempDir(), "executed")
	script := filepath.Join(source, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "demo")
	journal := operation.New(filepath.Join(t.TempDir(), "journal.json"))
	plan := resource.PlacementPlan{Resource: resource.ManagedResource{
		Version: "v1", ID: "demo", Kind: resource.Skill,
		Provenance: resource.Provenance{Source: source},
	}}
	if _, err := adapter.PlaceFilesystem(plan, adapter.FilesystemPlacementOptions{
		Destination: destination, Journal: journal, Confirm: func(operation.Plan) bool { return true },
	}); err != nil {
		t.Fatalf("PlaceFilesystem() error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("resource executable ran; marker stat error = %v", err)
	}
}
