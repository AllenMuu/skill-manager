package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProviderStatus is deliberately small so callers can render stable status
// values without exposing provider credentials or implementation details.
type ProviderStatus string

const (
	ProviderConfigured  ProviderStatus = "configured"
	ProviderAvailable   ProviderStatus = "available"
	ProviderUnavailable ProviderStatus = "unavailable"
	ProviderUnsupported ProviderStatus = "unsupported"
)

type CapabilityStatus string

const (
	CapabilityConfigured  CapabilityStatus = "configured"
	CapabilityAvailable   CapabilityStatus = "available"
	CapabilityUnavailable CapabilityStatus = "unavailable"
	CapabilityUnsupported CapabilityStatus = "unsupported"
)

type CapabilityMapping struct {
	Capability Capability       `json:"capability"`
	Scope      Scope            `json:"scope"`
	Status     CapabilityStatus `json:"status"`
	Reason     string           `json:"reason,omitempty"`
}

type AgentStatus struct {
	Agent        string              `json:"agent"`
	Capabilities []CapabilityMapping `json:"capabilities"`
}

type StatusSummary struct {
	Provider DiscoveryResult `json:"provider"`
	Agents   []AgentStatus   `json:"agents"`
}

// SummarizeStatus combines provider discovery with an explicit mapping of
// provider capabilities to agent integrations. Agent Manager currently has no
// verified native shared-memory channel, so mappings remain unsupported until
// an adapter declares one; this is intentionally different from provider
// availability.
func SummarizeStatus(cfg ProviderConfig, agents []AgentAccess, options DiscoveryOptions) StatusSummary {
	provider := DiscoveryResult{ID: cfg.ID, Provider: cfg.Provider, Status: ProviderUnavailable}
	if cfg.Provider != "graphiti" {
		provider.Status = ProviderUnsupported
		provider.Reason = "no adapter is registered for the configured provider"
		provider.NextAction = "configure provider: graphiti"
	} else if err := cfg.Validate(); err != nil {
		provider.Status = ProviderUnavailable
		provider.Reason = "provider configuration is invalid"
		provider.NextAction = "fix the provider configuration reference and scopes"
	} else if strings.TrimSpace(os.Getenv(cfg.Configuration.Name)) == "" {
		provider.Status = ProviderUnavailable
		provider.Reason = "Graphiti endpoint is not configured"
		provider.NextAction = "set the referenced environment variable to a running Graphiti URL"
	} else if !options.AllowNetwork {
		provider.Status = ProviderConfigured
		provider.Reason = "provider is configured; network discovery is disabled by default"
		provider.NextAction = "rerun status with network discovery explicitly enabled"
	} else {
		provider = (GraphitiAdapter{}).Discover(context.Background(), cfg, options)
	}
	result := StatusSummary{Provider: provider, Agents: make([]AgentStatus, 0, len(agents))}
	for _, agent := range agents {
		mapped := AgentStatus{Agent: agent.Agent, Capabilities: make([]CapabilityMapping, 0, len(cfg.Capabilities)*len(cfg.Scopes))}
		for _, scope := range cfg.Scopes {
			for _, capability := range cfg.Capabilities {
				mapping := CapabilityMapping{Capability: capability, Scope: scope, Status: CapabilityUnsupported, Reason: "agent does not declare this shared-memory capability or scope"}
				if agent.Supports(capability, scope) {
					mapping.Status = capabilityStatus(provider.Status)
					mapping.Reason = ""
				}
				mapped.Capabilities = append(mapped.Capabilities, mapping)
			}
		}
		result.Agents = append(result.Agents, mapped)
	}
	return result
}

func capabilityStatus(provider ProviderStatus) CapabilityStatus {
	switch provider {
	case ProviderConfigured:
		return CapabilityConfigured
	case ProviderAvailable:
		return CapabilityAvailable
	case ProviderUnavailable:
		return CapabilityUnavailable
	default:
		return CapabilityUnsupported
	}
}

// DiscoveryOptions controls side effects. Network access is opt-in; callers
// must explicitly set AllowNetwork to true to probe a provider endpoint.
type DiscoveryOptions struct {
	AllowNetwork bool
	HTTPClient   *http.Client
}

type DiscoveryResult struct {
	ID               string         `json:"id,omitempty"`
	Provider         string         `json:"provider"`
	Status           ProviderStatus `json:"status"`
	Capabilities     []Capability   `json:"capabilities,omitempty"`
	Scopes           []Scope        `json:"scopes,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	NextAction       string         `json:"nextAction,omitempty"`
	NetworkAttempted bool           `json:"networkAttempted"`
}

type ProviderAdapter interface {
	Discover(context.Context, ProviderConfig, DiscoveryOptions) DiscoveryResult
}

// GraphitiAdapter integrates with a user-managed Graphiti HTTP service. It
// does not start, install, or configure Graphiti.
type GraphitiAdapter struct{}

func (GraphitiAdapter) Discover(ctx context.Context, cfg ProviderConfig, options DiscoveryOptions) DiscoveryResult {
	result := DiscoveryResult{ID: cfg.ID, Provider: "graphiti", Status: ProviderUnavailable}
	if cfg.Provider != "graphiti" {
		result.Status = ProviderUnsupported
		result.Reason = "no adapter is registered for the configured provider"
		result.NextAction = "configure provider: graphiti"
		return result
	}
	if err := cfg.Validate(); err != nil {
		result.Reason = "provider configuration is invalid"
		result.NextAction = "fix the provider configuration reference and scopes"
		return result
	}
	if cfg.Configuration.Kind != "env" {
		result.Reason = "Graphiti discovery currently requires an environment URL reference"
		result.NextAction = "set configuration.kind to env and point it at GRAPHITI_URL"
		return result
	}
	url := strings.TrimSpace(os.Getenv(cfg.Configuration.Name))
	if url == "" {
		result.Reason = "Graphiti endpoint is not configured"
		result.NextAction = "set the referenced environment variable to a running Graphiti URL"
		return result
	}
	if !options.AllowNetwork {
		result.Reason = "network discovery is disabled by default"
		result.NextAction = "rerun discovery with network access explicitly enabled"
		return result
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/healthcheck", nil)
	if err != nil {
		result.Reason = "Graphiti endpoint URL is invalid"
		result.NextAction = "correct the referenced endpoint URL"
		return result
	}
	result.NetworkAttempted = true
	resp, err := client.Do(req)
	if err != nil {
		result.Reason = "Graphiti endpoint could not be reached"
		result.NextAction = "start Graphiti locally and verify the referenced URL"
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Reason = fmt.Sprintf("Graphiti health check returned HTTP %d", resp.StatusCode)
		result.NextAction = "verify the Graphiti service and its healthcheck endpoint"
		return result
	}
	result.Status = ProviderAvailable
	result.Capabilities = append([]Capability(nil), cfg.Capabilities...)
	result.Scopes = append([]Scope(nil), cfg.Scopes...)
	// A compatible service may advertise a narrower set; malformed optional
	// metadata is ignored because health remains a useful availability signal.
	var metadata struct {
		Capabilities []Capability `json:"capabilities"`
		Scopes       []Scope      `json:"scopes"`
	}
	if json.NewDecoder(resp.Body).Decode(&metadata) == nil {
		if len(metadata.Capabilities) > 0 {
			result.Capabilities = metadata.Capabilities
		}
		if len(metadata.Scopes) > 0 {
			result.Scopes = metadata.Scopes
		}
	}
	return result
}
