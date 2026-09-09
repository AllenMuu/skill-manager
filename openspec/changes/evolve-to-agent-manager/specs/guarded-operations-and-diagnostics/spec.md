## MODIFIED Requirements

### Requirement: Guarded filesystem mutations
The system SHALL preview file changes and require confirmation before any mutating Skill or SubAgent operation. It SHALL refuse to replace an unmanaged directory, ordinary file, or link to a different destination unless the user explicitly selects a compatible conflict strategy and supplies force confirmation.

#### Scenario: SubAgent installation would replace an unmanaged directory
- **WHEN** a filesystem-based SubAgent installation finds an unmanaged destination
- **THEN** the system displays the conflict and makes no filesystem change without explicit force confirmation

### Requirement: Reversible operation journal
The system SHALL record each confirmed reversible mutating resource operation in a local operation journal and SHALL provide undo for the latest reversible operation. Existing Skill journal entries SHALL remain readable.

#### Scenario: User undoes an existing Skill activation
- **WHEN** the user invokes undo after a successful pre-migration Skill activation
- **THEN** the system restores the pre-operation project-link state recorded by the existing journal entry

### Requirement: Local resource diagnostics and reconciliation
The system SHALL diagnose resource compatibility, orphaned managed links, unsupported agent locations, and configured Memory-provider availability. It SHALL reconcile a confirmed managed Skill link to the currently configured library path.

#### Scenario: Diagnostics inspect all configured domains
- **WHEN** the user runs diagnostics for a project
- **THEN** the output reports relevant Skill link health, agent support, and Memory integration availability without mutating any resource

### Requirement: No managed-resource code execution
The system SHALL NOT execute scripts, install dependencies, or fetch external resources contained in Skills or SubAgents while cataloging, validating, installing, diagnosing, reconciling, or undoing operations.

#### Scenario: Managed resource contains an executable script
- **WHEN** the system manages a resource that contains executable files
- **THEN** it performs only filesystem and metadata operations without executing those files
