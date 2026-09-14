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

func TestPromotionRequiresConfirmationAndScope(t *testing.T) {
	writer := &recordingWriter{}
	service := memory.PromotionService{Writer: writer, Capabilities: []memory.Capability{memory.CapabilityWrite}, Scopes: []memory.Scope{memory.ScopeProject}}

	if err := service.Promote(context.Background(), memory.PromotionRequest{Content: "knowledge", Scope: memory.ScopeProject}); !errors.Is(err, memory.ErrPromotionNotConfirmed) {
		t.Fatalf("unconfirmed promotion error = %v, want ErrPromotionNotConfirmed", err)
	}
	if writer.calls != 0 {
		t.Fatal("unconfirmed promotion called provider")
	}
	if err := service.Promote(context.Background(), memory.PromotionRequest{Content: "knowledge", Scope: memory.ScopeUser, Confirmed: true}); !errors.Is(err, memory.ErrPromotionScopeUnsupported) {
		t.Fatalf("unsupported scope error = %v, want ErrPromotionScopeUnsupported", err)
	}
	if writer.calls != 0 {
		t.Fatal("unsupported promotion called provider")
	}
}

func TestPromotionWritesOnlyExplicitKnowledge(t *testing.T) {
	writer := &recordingWriter{}
	service := memory.PromotionService{Writer: writer, Capabilities: []memory.Capability{memory.CapabilityWrite}, Scopes: []memory.Scope{memory.ScopeUser}}
	request := memory.PromotionRequest{Content: "explicit note", Scope: memory.ScopeUser, Confirmed: true}
	if err := service.Promote(context.Background(), request); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if writer.calls != 1 || writer.request != request {
		t.Fatalf("writer received calls=%d request=%#v, want one explicit request", writer.calls, writer.request)
	}
}

func TestPromotionRejectsEmptyContentAndConversationSource(t *testing.T) {
	writer := &recordingWriter{}
	service := memory.PromotionService{Writer: writer, Capabilities: []memory.Capability{memory.CapabilityWrite}, Scopes: []memory.Scope{memory.ScopeUser}}
	for _, request := range []memory.PromotionRequest{
		{Scope: memory.ScopeUser, Confirmed: true},
		{Content: "transcript", Scope: memory.ScopeUser, Source: memory.SourceConversation, Confirmed: true},
	} {
		if err := service.Promote(context.Background(), request); err == nil {
			t.Fatalf("Promote(%#v) error = nil, want rejection", request)
		}
	}
	if writer.calls != 0 {
		t.Fatal("invalid promotion called provider")
	}
}

func TestGraphitiWriterWritesOnlyAfterPromotionServiceConfirmation(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityWrite}}
	writer := memory.GraphitiWriter{Config: cfg, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/memories" {
			t.Fatalf("request = %s %s, want POST /memories", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}}
	service := memory.PromotionService{Writer: writer, Capabilities: cfg.Capabilities, Scopes: cfg.Scopes}
	if err := service.Promote(context.Background(), memory.PromotionRequest{Content: "selected", Scope: memory.ScopeProject, Confirmed: true}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
}

func TestGraphitiWriterRejectsUnconfirmedOrConversationWrites(t *testing.T) {
	t.Setenv("GRAPHITI_URL", "http://graphiti.test")
	cfg := memory.ProviderConfig{Version: "v1", ID: "local", Provider: "graphiti", Configuration: memory.ConfigReference{Kind: "env", Name: "GRAPHITI_URL"}, Scopes: []memory.Scope{memory.ScopeProject}, Capabilities: []memory.Capability{memory.CapabilityWrite}}
	calls := 0
	writer := memory.GraphitiWriter{Config: cfg, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}}
	for _, request := range []memory.PromotionRequest{
		{Content: "unconfirmed", Scope: memory.ScopeProject},
		{Content: "transcript", Scope: memory.ScopeProject, Source: memory.SourceConversation, Confirmed: true},
	} {
		if err := writer.Write(context.Background(), request); err == nil {
			t.Fatalf("Write(%#v) error = nil, want rejection", request)
		}
	}
	if calls != 0 {
		t.Fatalf("writer made %d HTTP requests for rejected writes", calls)
	}
}

type recordingWriter struct {
	calls   int
	request memory.PromotionRequest
}

func (w *recordingWriter) Write(ctx context.Context, request memory.PromotionRequest) error {
	w.calls++
	w.request = request
	return nil
}
