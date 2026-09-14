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
	ProviderAvailable   ProviderStatus = "available"
	ProviderUnavailable ProviderStatus = "unavailable"
	ProviderUnsupported ProviderStatus = "unsupported"
)

// DiscoveryOptions controls side effects. Network access is opt-in; callers
// must explicitly set AllowNetwork to true to probe a provider endpoint.
type DiscoveryOptions struct {
	AllowNetwork bool
	HTTPClient   *http.Client
}

type DiscoveryResult struct {
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
	result := DiscoveryResult{Provider: "graphiti", Status: ProviderUnavailable}
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
