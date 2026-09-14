## MODIFIED Requirements

### Requirement: Explicit multi-target project activation
The system SHALL activate a selected library Skill only for user-selected target agents through their supported adapters, creating an absolute soft link in each adapter's recognized project Skill location. It SHALL support an explicit option to select all detected supported target agents.

#### Scenario: Skill is activated for selected agents
- **WHEN** the user confirms activation of a Skill for selected supported agents
- **THEN** the system creates one absolute soft link per selected target in that adapter's corresponding project Skill location

#### Scenario: Compatibility is not declared for a target
- **WHEN** the user selects a target agent not included in a Skill's compatibility declaration
- **THEN** the system displays a compatibility warning and permits the explicitly confirmed activation

### Requirement: Project skill inventory
The system SHALL list project Skills by target agent and distinguish managed Skills, unmanaged directory Skills, orphaned links, and unsupported agent locations.

#### Scenario: Project contains managed and unmanaged skills
- **WHEN** the user lists resources in a project containing a library link and an independent directory Skill
- **THEN** the output labels the linked Skill as managed and the independent directory as unmanaged
