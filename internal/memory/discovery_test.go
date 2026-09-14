package memory_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/AllenMuu/skill-manager/internal/memory"
)

func TestGraphitiDiscoveryDoesNotUseNetworkByDefault(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test/?token=super-secret")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser}}
	called := false
	result := memory.GraphitiAdapter{}.Discover(context.Background(), cfg, memory.DiscoveryOptions{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected network request")
	})}})
	if result.Status != memory.ProviderUnavailable {
		t.Fatalf("status = %q, want unavailable", result.Status)
	}
	if result.NextAction == "" {
		t.Fatal("unavailable result has no actionable next step")
	}
	if result.NetworkAttempted || called {
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

func TestGraphitiDiscoveryHonorsExplicitEmptyMetadata(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser}, Capabilities: []memory.Capability{memory.CapabilityRead}}
	result := memory.GraphitiAdapter{}.Discover(context.Background(), cfg, memory.DiscoveryOptions{AllowNetwork: true, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"capabilities":[],"scopes":[]}`)), Header: make(http.Header)}, nil
	})}})
	if result.Status != memory.ProviderAvailable || len(result.Capabilities) != 0 || len(result.Scopes) != 0 {
		t.Fatalf("result = %#v, want available provider with explicitly empty support", result)
	}
}

func TestGraphitiDiscoveryUnavailableProbeIsActionableAndRedacted(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test/?token=super-secret")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeUser}}
	tests := []struct {
		name   string
		client *http.Client
	}{
		{name: "transport error", client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })}},
		{name: "non-success response", client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := memory.GraphitiAdapter{}.Discover(context.Background(), cfg, memory.DiscoveryOptions{AllowNetwork: true, HTTPClient: tt.client})
			if result.Status != memory.ProviderUnavailable || result.NextAction == "" {
				t.Fatalf("result = %#v, want actionable unavailable status", result)
			}
			if strings.Contains(result.Reason, "graphiti.test") || strings.Contains(result.Reason, "super-secret") {
				t.Fatalf("reason leaked endpoint or credential: %q", result.Reason)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
