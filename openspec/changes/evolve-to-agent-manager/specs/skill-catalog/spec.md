## MODIFIED Requirements

### Requirement: Configured directory-skill library
The system SHALL preserve `~/.agents/skills` as its default configurable skill-library path and SHALL treat it as the authoritative library for the Skill resource kind under Agent Manager. A catalog entry SHALL be eligible for management only when it is a directory containing `SKILL.md`.

#### Scenario: Existing default library is searched after migration
- **WHEN** no alternate library path has been configured after upgrading to Agent Manager
- **THEN** catalog commands discover eligible Skill resources below `~/.agents/skills`

#### Scenario: Invalid library entry is encountered
- **WHEN** a library child does not contain `SKILL.md`
- **THEN** the system excludes it from managed Skill results and reports it as incompatible when diagnostics are requested

### Requirement: Companion management metadata
The system SHALL continue to read optional `.skill-manager.yaml` files beside a directory skill for tags, compatibility declarations, and provenance. The absence of companion metadata SHALL NOT prevent discovery or activation.

#### Scenario: Legacy skill has no companion metadata
- **WHEN** a skill contains only `SKILL.md` and optional resources
- **THEN** it remains searchable and eligible for management after the product migration
