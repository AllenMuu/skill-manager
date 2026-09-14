package memory_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/memory"
	"gopkg.in/yaml.v3"
)

func TestProviderConfigValidatesVersionReferenceScopesAndCapabilities(t *testing.T) {
	cfg := memory.ProviderConfig{Version: "v1", ID: "local-memory", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser, memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilityWrite, memory.CapabilitySearch}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProviderConfigRejectsInlineSecrets(t *testing.T) {
	cfg := memory.ProviderConfig{Version: "v1", ID: "provider", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "inline", Name: "token=super-secret"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want inline secret rejection")
	}
}

func TestProviderConfigJSONAndEvidenceRedactSensitiveValues(t *testing.T) {
	cfg := memory.ProviderConfig{Version: "v1", ID: "provider", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_TOKEN"}, Scopes: []memory.Scope{memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityRead}}
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

func TestStatusSummarizesConfiguredProviderAndUnsupportedAgentAccess(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser}, Capabilities: []memory.Capability{memory.CapabilityRead}}
	status := memory.SummarizeStatus(cfg, []memory.AgentAccess{{Agent: "codex"}}, memory.DiscoveryOptions{})
	if status.Provider.Status != memory.ProviderConfigured {
		t.Fatalf("provider status = %q, want configured", status.Provider.Status)
	}
	if len(status.Agents) != 1 || status.Agents[0].Capabilities[0].Status != memory.CapabilityUnsupported {
		t.Fatalf("agents = %#v, want unsupported agent capability", status.Agents)
	}
}

func TestStatusDistinguishesUnsupportedProvider(t *testing.T) {
	cfg := memory.ProviderConfig{Version: "v1", ID: "other", Provider: "unknown", Configuration: memory.ConfigReference{Kind: "env", Name: "MEMORY_URL"}, Scopes: []memory.Scope{memory.ScopeUser}}
	status := memory.SummarizeStatus(cfg, nil, memory.DiscoveryOptions{})
	if status.Provider.Status != memory.ProviderUnsupported {
		t.Fatalf("provider status = %q, want unsupported", status.Provider.Status)
	}
}

func TestStatusMapsProviderStateAndAgentDeclarations(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilitySearch}}
	declared := memory.AgentAccess{Agent: "codex", Capabilities: []memory.Capability{memory.CapabilityRead, memory.CapabilitySearch}, Scopes: []memory.Scope{memory.ScopeProject}}
	status := memory.SummarizeStatus(cfg, []memory.AgentAccess{declared}, memory.DiscoveryOptions{})
	if got := status.Agents[0].Capabilities[0].Status; got != memory.CapabilityConfigured {
		t.Fatalf("configured mapping = %q, want configured", got)
	}
	status = memory.SummarizeStatus(cfg, []memory.AgentAccess{declared}, memory.DiscoveryOptions{AllowNetwork: true, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"capabilities":["read"],"scopes":["project"]}`)), Header: make(http.Header)}, nil
	})}})
	if got := status.Agents[0].Capabilities[0].Status; got != memory.CapabilityAvailable {
		t.Fatalf("available mapping = %q, want available", got)
	}
	if got := status.Agents[0].Capabilities[1].Status; got != memory.CapabilityUnsupported {
		t.Fatalf("narrow provider mapping = %q, want unsupported", got)
	}
	unsupported := memory.AgentAccess{Agent: "pi"}
	status = memory.SummarizeStatus(cfg, []memory.AgentAccess{unsupported}, memory.DiscoveryOptions{})
	if got := status.Agents[0].Capabilities[0].Status; got != memory.CapabilityUnsupported {
		t.Fatalf("unsupported mapping = %q, want unsupported", got)
	}
}
