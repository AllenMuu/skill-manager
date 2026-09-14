package memory_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/memory"
)

func TestGraphitiDiscoveryDoesNotUseNetworkByDefault(t *testing.T) {
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser}}
	result := memory.GraphitiAdapter{}.Discover(context.Background(), cfg, memory.DiscoveryOptions{})
	if result.Status != memory.ProviderUnavailable {
		t.Fatalf("status = %q, want unavailable", result.Status)
	}
	if result.NextAction == "" {
		t.Fatal("unavailable result has no actionable next step")
	}
	if result.NetworkAttempted {
		t.Fatal("default discovery attempted network access")
	}
}

func TestGraphitiDiscoveryUsesExplicitNetworkOptInAndReportsCapabilities(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser, memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityRead}}
	result := memory.GraphitiAdapter{}.Discover(context.Background(), cfg, memory.DiscoveryOptions{AllowNetwork: true, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/healthcheck" {
			t.Fatalf("path = %s, want /healthcheck", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"capabilities":["read","search"],"scopes":["project"]}`)), Header: make(http.Header)}, nil
	})}})
	if result.Status != memory.ProviderAvailable || !result.NetworkAttempted {
		t.Fatalf("result = %#v, want available network probe", result)
	}
	if len(result.Capabilities) != 2 || result.Capabilities[1] != memory.CapabilitySearch {
		t.Fatalf("capabilities = %v, want Graphiti advertisement", result.Capabilities)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
