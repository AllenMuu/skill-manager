package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	ErrPromotionNotConfirmed     = errors.New("memory promotion requires explicit confirmation")
	ErrPromotionScopeUnsupported = errors.New("memory promotion scope is unsupported")
	ErrPromotionWriteUnsupported = errors.New("memory provider does not support writes")
)

// Source identifies the origin of a promotion request. Conversation data is
// intentionally not an accepted promotion source; callers must select and
// submit knowledge explicitly.
type Source string

const (
	SourceExplicit     Source = "explicit"
	SourceResource     Source = "resource"
	SourceConversation Source = "conversation"
)

// PromotionRequest is the complete user-selected payload for a provider
// write. It contains no agent conversation or resource lifecycle context.
type PromotionRequest struct {
	Content   string `json:"content"`
	Scope     Scope  `json:"scope"`
	Source    Source `json:"source,omitempty"`
	Confirmed bool   `json:"confirmed"`
}

// KnowledgeWriter is the only interface through which a provider write can
// occur. Resource handlers and agent adapters do not receive this interface.
type KnowledgeWriter interface {
	Write(context.Context, PromotionRequest) error
}

// PromotionService validates explicit promotion intent before invoking a
// provider writer. It performs no discovery and never enables network access.
type PromotionService struct {
	Writer       KnowledgeWriter
	Capabilities []Capability
	Scopes       []Scope
}

func (s PromotionService) Promote(ctx context.Context, request PromotionRequest) error {
	if !request.Confirmed {
		return ErrPromotionNotConfirmed
	}
	if strings.TrimSpace(request.Content) == "" {
		return fmt.Errorf("memory promotion content is required")
	}
	if request.Scope != ScopeUser && request.Scope != ScopeProject {
		return ErrPromotionScopeUnsupported
	}
	if !containsScope(s.Scopes, request.Scope) {
		return ErrPromotionScopeUnsupported
	}
	if !containsCapability(s.Capabilities, CapabilityWrite) {
		return ErrPromotionWriteUnsupported
	}
	if request.Source == SourceConversation {
		return fmt.Errorf("agent conversation data cannot be promoted")
	}
	if request.Source != "" && request.Source != SourceExplicit && request.Source != SourceResource {
		return fmt.Errorf("unsupported memory promotion source %q", request.Source)
	}
	if s.Writer == nil {
		return ErrPromotionWriteUnsupported
	}
	return s.Writer.Write(ctx, request)
}

// NewConfiguredWriter selects the write adapter only for an explicitly
// configured provider. The returned writer is never used by status or normal
// resource operations.
func NewConfiguredWriter(cfg ProviderConfig) (KnowledgeWriter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case "graphiti":
		return GraphitiWriter{Config: cfg}, nil
	default:
		return nil, fmt.Errorf("no Memory writer is registered for provider %q", cfg.Provider)
	}
}

// GraphitiWriter performs the one provider mutation supported by the explicit
// promotion command. It does not start or configure Graphiti and never logs
// the configured endpoint or response body.
type GraphitiWriter struct {
	Config     ProviderConfig
	HTTPClient *http.Client
}

func (w GraphitiWriter) Write(ctx context.Context, request PromotionRequest) error {
	if !request.Confirmed {
		return ErrPromotionNotConfirmed
	}
	if request.Source == SourceConversation {
		return fmt.Errorf("agent conversation data cannot be promoted")
	}
	if w.Config.Configuration.Kind != "env" {
		return fmt.Errorf("Graphiti promotion requires an environment URL reference")
	}
	url := strings.TrimSpace(os.Getenv(w.Config.Configuration.Name))
	if url == "" {
		return fmt.Errorf("Graphiti endpoint is not configured")
	}
	body, err := json.Marshal(struct {
		Content string `json:"content"`
		Scope   Scope  `json:"scope"`
	}{request.Content, request.Scope})
	if err != nil {
		return fmt.Errorf("encode memory promotion: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(url, "/")+"/memories", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create memory promotion request: invalid provider endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	client := w.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Graphiti promotion failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Graphiti promotion failed with HTTP %d", resp.StatusCode)
	}
	return nil
}
