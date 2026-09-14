package memory_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/memory"
	"gopkg.in/yaml.v3"
)

func TestProviderConfigValidatesVersionReferenceScopesAndCapabilities(t *testing.T) {
	cfg := memory.ProviderConfig{
		Version: "v1", ID: "local-memory", Provider: "graphiti",
		Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"},
		Scopes:        []memory.Scope{memory.ScopeUser, memory.ScopeProject},
		Capabilities:  []memory.Capability{memory.CapabilityRead, memory.CapabilityWrite, memory.CapabilitySearch},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProviderConfigRejectsInlineSecrets(t *testing.T) {
	cfg := memory.ProviderConfig{
		Version: "v1", ID: "provider", Provider: "graphiti",
		Configuration: memory.ConfigReference{Kind: "inline", Name: "token=super-secret"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want inline secret rejection")
	}
}

func TestProviderConfigJSONAndEvidenceRedactSensitiveValues(t *testing.T) {
	cfg := memory.ProviderConfig{
		Version: "v1", ID: "provider", Provider: "graphiti",
		Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_TOKEN"},
		Scopes:        []memory.Scope{memory.ScopeProject},
		Capabilities:  []memory.Capability{memory.CapabilityRead},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if strings.Contains(out, "GRAPHITI_TOKEN") || strings.Contains(out, "super-secret") {
		t.Fatalf("JSON leaked sensitive reference: %s", out)
	}
	if !strings.Contains(out, "redacted") {
		t.Fatalf("JSON = %s, want redacted marker", out)
	}
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if yamlOut := string(yamlBytes); strings.Contains(yamlOut, "GRAPHITI_TOKEN") || !strings.Contains(yamlOut, "[redacted]") {
		t.Fatalf("YAML leaked or omitted redaction: %s", yamlOut)
	}
	evidence := cfg.JournalEvidence()
	if strings.Contains(evidence, "GRAPHITI_TOKEN") || strings.Contains(evidence, "super-secret") {
		t.Fatalf("journal evidence leaked sensitive value: %s", evidence)
	}
}

func TestAgentAccessReportsIndependentCapabilitiesAndScopes(t *testing.T) {
	access := memory.AgentAccess{Agent: "codex", Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilitySearch}, Scopes: []memory.Scope{memory.ScopeProject}}
	if !access.Supports(memory.CapabilitySearch, memory.ScopeProject) {
		t.Fatal("Supports(search, project) = false, want true")
	}
	if access.Supports(memory.CapabilityWrite, memory.ScopeProject) {
		t.Fatal("Supports(write, project) = true, want false")
	}
	if access.Supports(memory.CapabilityRead, memory.ScopeUser) {
		t.Fatal("Supports(read, user) = true, want false")
	}
}

type localTestProvider struct {
	status       memory.ProviderStatus
	statusCalls  int
	promoteCalls int
}

func (p *localTestProvider) Status() memory.ProviderStatus {
	p.statusCalls++
	return p.status
}

func (p *localTestProvider) Promote(memory.Scope, string) error {
	p.promoteCalls++
	return nil
}

func TestProviderStatusMapsOnlyCapabilitiesTheAgentSupports(t *testing.T) {
	provider := &localTestProvider{status: memory.ProviderStatus{
		Available:    true,
		Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilityWrite, memory.CapabilitySearch},
		Scopes:       []memory.Scope{memory.ScopeUser, memory.ScopeProject},
	}}
	agent := memory.AgentAccess{
		Agent:        "codex",
		Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilitySearch},
		Scopes:       []memory.Scope{memory.ScopeProject},
	}

	got := memory.MapAgentAccess(provider.Status(), agent)
	if got.Agent != agent.Agent {
		t.Fatalf("mapped agent = %q, want %q", got.Agent, agent.Agent)
	}
	if got.Supports(memory.CapabilityRead, memory.ScopeProject) == false ||
		got.Supports(memory.CapabilitySearch, memory.ScopeProject) == false {
		t.Fatalf("mapped access = %#v, want read/search project access", got)
	}
	if got.Supports(memory.CapabilityWrite, memory.ScopeProject) {
		t.Fatal("mapped access unexpectedly granted unsupported write capability")
	}
	if got.Supports(memory.CapabilityRead, memory.ScopeUser) {
		t.Fatal("mapped access unexpectedly granted unsupported user scope")
	}
}

func TestUnavailableProviderMapsToNoAgentAccess(t *testing.T) {
	got := memory.MapAgentAccess(memory.ProviderStatus{Available: false, Capabilities: []memory.Capability{memory.CapabilityRead}, Scopes: []memory.Scope{memory.ScopeProject}}, memory.AgentAccess{
		Agent: "codex", Capabilities: []memory.Capability{memory.CapabilityRead}, Scopes: []memory.Scope{memory.ScopeProject},
	})
	if len(got.Capabilities) != 0 || len(got.Scopes) != 0 {
		t.Fatalf("unavailable provider mapped access = %#v, want no capabilities or scopes", got)
	}
}

func TestOrdinaryProviderInspectionDoesNotWrite(t *testing.T) {
	provider := &localTestProvider{status: memory.ProviderStatus{
		Available:    true,
		Capabilities: []memory.Capability{memory.CapabilityRead},
		Scopes:       []memory.Scope{memory.ScopeProject},
	}}
	cfg := memory.ProviderConfig{
		Version: "v1", ID: "local", Provider: "test-provider",
		Configuration: memory.ConfigReference{Kind: "env", Name: "TEST_PROVIDER_CONFIG"},
		Scopes:        []memory.Scope{memory.ScopeProject},
		Capabilities:  []memory.Capability{memory.CapabilityRead},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	_ = cfg.Redacted()
	_ = memory.MapAgentAccess(provider.Status(), memory.AgentAccess{
		Agent: "codex", Capabilities: []memory.Capability{memory.CapabilityRead}, Scopes: []memory.Scope{memory.ScopeProject},
	})
	if provider.promoteCalls != 0 {
		t.Fatalf("ordinary inspection promoted %d values, want 0", provider.promoteCalls)
	}
	if provider.statusCalls != 1 {
		t.Fatalf("provider status calls = %d, want 1", provider.statusCalls)
	}
}
