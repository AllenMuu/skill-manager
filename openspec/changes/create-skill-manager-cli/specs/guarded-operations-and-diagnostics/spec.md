## Purpose

Ensure local skill management is transparent, reversible, and safe around project-owned files, Git working trees, and links that may become invalid over time.

## ADDED Requirements

### Requirement: Guarded filesystem mutations
The system SHALL preview file changes and require confirmation before any adoption, activation that replaces an existing path, removal, fork, reconciliation, or global initialization. It SHALL refuse to replace an unmanaged directory, ordinary file, or link to a different destination unless the user explicitly selects a conflict strategy and supplies force confirmation.

#### Scenario: Activation would replace an unmanaged directory
- **WHEN** activation finds an unmanaged skill directory at its destination
- **THEN** the system displays the conflict and makes no filesystem change without explicit force confirmation

#### Scenario: Normal activation has no conflict
- **WHEN** activation has no existing destination conflict and the user confirms the preview
- **THEN** the system creates the requested links

### Requirement: Reversible operation journal
The system SHALL record each confirmed mutating operation in a local operation journal and SHALL provide undo for the latest reversible operation.

#### Scenario: User undoes an activation
- **WHEN** the user invokes undo after a successful activation
- **THEN** the system restores the pre-operation project-link state recorded by the journal

### Requirement: Local activation diagnostics and reconciliation
The system SHALL diagnose orphaned managed links and shall reconcile a confirmed managed link to the currently configured library path. Diagnostics SHALL support scanning the current project and user-specified project roots.

#### Scenario: Library path changes
- **WHEN** a project's managed skill is an orphaned link after the library moves
- **THEN** diagnostics report it as orphaned and reconciliation recreates it to the current configured library target after confirmation

### Requirement: Git environment guidance
The system SHALL warn when managed project links are tracked or would be tracked by Git, because absolute links are machine-local. It SHALL offer a scoped `.gitignore` update only after confirmation and SHALL avoid changing unmanaged-skill tracking rules without explicit user direction.

#### Scenario: Managed link is visible to Git
- **WHEN** a project activation creates a managed link in a Git worktree
- **THEN** the system warns that the link is machine-local and presents any proposed ignore-rule change for confirmation

### Requirement: Conservative library deletion
The system SHALL refuse library-skill deletion by default because it cannot know every project that may link to the skill. It SHALL require an explicit force confirmation for deletion and SHALL recommend diagnostics before doing so.

#### Scenario: User requests library deletion without force
- **WHEN** the user attempts to delete a library skill without force confirmation
- **THEN** the system refuses the deletion and explains that existing projects may retain orphaned links

### Requirement: No skill-code execution
The system SHALL NOT execute scripts, install dependencies, or fetch external resources contained in a skill while cataloging, validating, adopting, activating, diagnosing, reconciling, or undoing operations.

#### Scenario: Skill contains an executable script
- **WHEN** the system manages a skill that contains executable files
- **THEN** it performs only filesystem and metadata operations without executing those files
