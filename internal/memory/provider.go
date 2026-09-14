// Package memory defines the provider-neutral shared Memory control model.
// It deliberately contains no provider client or network implementation.
package memory

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Capability string

const (
	CapabilityRead   Capability = "read"
	CapabilityWrite  Capability = "write"
	CapabilitySearch Capability = "search"
)

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// ConfigReference identifies where provider configuration is held. It never
// contains a credential or provider secret. The referenced value remains in
// the user's environment/keychain/file and is not managed by Agent Manager.
type ConfigReference struct {
	Kind string `json:"kind" yaml:"kind"`
	Name string `json:"name" yaml:"name"`
}

type ProviderConfig struct {
	Version       string          `json:"version" yaml:"version"`
	ID            string          `json:"id" yaml:"id"`
	Provider      string          `json:"provider" yaml:"provider"`
	Configuration ConfigReference `json:"configuration" yaml:"configuration"`
	Scopes        []Scope         `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Capabilities  []Capability    `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

func (c ProviderConfig) Validate() error {
	if c.Version != "v1" {
		return fmt.Errorf("unsupported memory provider configuration version %q", c.Version)
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Provider) == "" {
		return fmt.Errorf("memory provider id and provider are required")
	}
	if err := c.Configuration.validate(); err != nil {
		return err
	}
	if len(c.Scopes) == 0 {
		return fmt.Errorf("memory provider must declare at least one scope")
	}
	for _, scope := range c.Scopes {
		if scope != ScopeUser && scope != ScopeProject {
			return fmt.Errorf("unsupported memory scope %q", scope)
		}
	}
	for _, capability := range c.Capabilities {
		if capability != CapabilityRead && capability != CapabilityWrite && capability != CapabilitySearch {
			return fmt.Errorf("unsupported memory capability %q", capability)
		}
	}
	return nil
}

func (r ConfigReference) validate() error {
	switch r.Kind {
	case "env", "file", "keychain":
	default:
		return fmt.Errorf("unsupported memory configuration reference kind %q", r.Kind)
	}
	if strings.TrimSpace(r.Name) == "" || strings.ContainsAny(r.Name, "=\n\r") {
		return fmt.Errorf("memory configuration reference must name an external value, not contain it")
	}
	return nil
}

// Redacted returns an output-safe view. Reference names are intentionally
// omitted because environment variable/keychain names can disclose credential
// usage and must not enter status output or journal evidence.
func (c ProviderConfig) Redacted() ProviderConfig { c.Configuration.Name = "[redacted]"; return c }

// MarshalJSON keeps provider references out of status output and journals.
// Callers that need to persist the actual reference should use a dedicated
// configuration store rather than serializing this control-plane model.
func (c ProviderConfig) MarshalJSON() ([]byte, error) {
	type plain ProviderConfig
	return json.Marshal(plain(c.Redacted()))
}

// MarshalYAML applies the same safety rule as MarshalJSON for YAML output.
func (c ProviderConfig) MarshalYAML() (any, error) {
	return map[string]any{"version": c.Version, "id": c.ID, "provider": c.Provider, "configuration": c.Redacted().Configuration, "scopes": c.Scopes, "capabilities": c.Capabilities}, nil
}

func (c ProviderConfig) JournalEvidence() string {
	b, _ := json.Marshal(c.Redacted())
	return string(b)
}
func (c ProviderConfig) String() string { return c.JournalEvidence() }

type AgentAccess struct {
	Agent        string       `json:"agent" yaml:"agent"`
	Capabilities []Capability `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Scopes       []Scope      `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

func (a AgentAccess) Supports(capability Capability, scope Scope) bool {
	capable, scoped := false, false
	for _, candidate := range a.Capabilities {
		if candidate == capability {
			capable = true
		}
	}
	for _, candidate := range a.Scopes {
		if candidate == scope {
			scoped = true
		}
	}
	return capable && scoped
}
