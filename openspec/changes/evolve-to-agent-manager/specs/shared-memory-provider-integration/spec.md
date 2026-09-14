## Purpose

Provide a local control layer for user-owned shared Memory providers so compatible agents can be configured and diagnosed without duplicating knowledge into agent-private stores by default.

## ADDED Requirements

### Requirement: Provider-independent Memory configuration
The system SHALL represent a Memory provider independently from agent adapters, including provider identity, non-secret configuration reference, supported capabilities, and supported scopes.

#### Scenario: Provider is configured
- **WHEN** the user configures a supported Memory provider
- **THEN** the system preserves provider configuration separately from every individual agent integration

### Requirement: Scoped capability diagnostics
The system SHALL report whether each supported agent integration can access configured Memory capabilities, including read, write, search, and available user or project scopes.

#### Scenario: User checks Memory status
- **WHEN** the user requests Memory status
- **THEN** the output identifies the configured provider and each agent's available and unavailable capabilities

### Requirement: Explicit shared-Memory promotion
The system SHALL require an explicit user action to persist managed knowledge to a shared Memory provider and SHALL NOT automatically copy resource content or agent-private conversation data into the provider.

#### Scenario: No promotion was requested
- **WHEN** the user configures an agent or manages a resource without requesting Memory persistence
- **THEN** the system does not write resource content or conversation history to the provider
