package memory_test

import (
	"encoding/json"
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
