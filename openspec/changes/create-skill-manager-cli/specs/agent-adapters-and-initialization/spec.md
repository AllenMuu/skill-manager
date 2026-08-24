## Purpose

Provide explicit, extensible placement rules for supported coding agents while keeping the global agent environment limited to the CLI's own usage guidance.

## ADDED Requirements

### Requirement: Supported target-agent adapters
The system SHALL provide supported project adapters for Claude Code and Codex directory skills. An adapter SHALL guarantee placement in the agent-recognized project location but SHALL NOT claim that a skill's instruction content is semantically compatible.

#### Scenario: Claude Code target is selected
- **WHEN** the user activates a skill for Claude Code in a project
- **THEN** the adapter places the managed link beneath `.claude/skills/<skill-id>`

#### Scenario: Codex target is selected
- **WHEN** the user activates a skill for Codex in a project
- **THEN** the adapter places the managed link beneath `.codex/skills/<skill-id>`

### Requirement: Unsupported-agent discovery
The system SHALL report detected agent environments lacking a supported adapter as unsupported or pending validation and SHALL NOT write into their directories.

#### Scenario: Unsupported environment is detected
- **WHEN** diagnostics detect an installed agent without a supported adapter
- **THEN** the output identifies it as unsupported and no activation target is created for it

### Requirement: Explicit global initialization
The system SHALL modify the global baseline only through an explicit initialization command. Initialization SHALL verify CLI availability and install or update a minimal Operator skill for supported global agents.

#### Scenario: Initialization succeeds
- **WHEN** the user confirms initialization
- **THEN** the system verifies the CLI can be invoked and installs or updates only the Operator skill in supported global agent skill locations

#### Scenario: Project activation runs after initialization
- **WHEN** a user activates skills for a project
- **THEN** the command does not modify global agent skill locations

### Requirement: Operator-guided agent usage
The installed Operator skill SHALL direct an agent to search the library, explain its recommendation, request user confirmation, and only then invoke a mutating CLI command.

#### Scenario: Agent proposes a skill
- **WHEN** an agent identifies a potentially useful library skill
- **THEN** its guidance requires it to present the candidate and rationale before it adds the skill to the project
