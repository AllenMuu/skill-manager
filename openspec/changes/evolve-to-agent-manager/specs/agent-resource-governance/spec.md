## Purpose

Provide an agent-neutral control plane that discovers supported coding agents and manages distinct resource domains without coupling domain behavior to a runtime's filesystem conventions.

## ADDED Requirements

### Requirement: Agent inventory and capability discovery
The system SHALL inventory detected and configured agents with a stable identifier, availability state, supported resource kinds, and declared capabilities. It SHALL report agents lacking a supported adapter without treating them as writable targets.

#### Scenario: Supported and unsupported agents are discovered
- **WHEN** the user requests the agent inventory or diagnostics
- **THEN** the output distinguishes supported agents and their resource capabilities from detected unsupported environments

### Requirement: Agent-neutral resource contracts
The system SHALL represent each managed resource with a stable identifier, resource kind, provenance, compatibility declaration, and resource-specific metadata. A resource handler SHALL own lifecycle validation while an agent adapter SHALL own runtime placement and capability translation.

#### Scenario: A resource is evaluated for an agent
- **WHEN** the user requests inspection or an operation targeting an agent
- **THEN** the system evaluates compatibility and adapter capabilities without embedding runtime-specific path rules in the resource contract

### Requirement: Explicit unsupported-capability handling
The system SHALL warn or fail before a requested operation when an adapter cannot represent a required resource capability. It SHALL NOT silently drop requested capabilities or write an incomplete representation.

#### Scenario: A target lacks a required capability
- **WHEN** an operation requires a resource capability unsupported by its target agent
- **THEN** the preview identifies the unsupported capability and no mutation occurs unless the operation has an explicitly defined compatible fallback
