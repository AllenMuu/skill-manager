## Purpose

Provide safe, explainable skill recommendations from local project technology evidence so operators and agents can choose relevant managed skills before any activation.

## ADDED Requirements

### Requirement: Read-only recommendation invocation
The system SHALL provide `skill-manager recommend`, which evaluates the current working directory when `--project` is omitted and an explicitly supplied directory when `--project` is present. The command SHALL reject a non-existent, unreadable, or non-directory project path with a clear diagnostic and SHALL NOT modify any filesystem location.

#### Scenario: Current directory is recommended
- **WHEN** an operator runs `skill-manager recommend` from a readable project directory
- **THEN** the system evaluates that directory as the recommendation root and displays its recommendation result

#### Scenario: Explicit project is recommended
- **WHEN** an operator runs `skill-manager recommend --project ../service`
- **THEN** the system evaluates the resolved `../service` directory instead of the current working directory

#### Scenario: Invalid project path is supplied
- **WHEN** an operator supplies a path that is missing, unreadable, or not a directory
- **THEN** the command reports why the supplied path cannot be evaluated, returns a non-zero status, and makes no changes

### Requirement: Static project technology detection
The system SHALL detect technologies only from documented static project markers and configuration content. The initial marker vocabulary SHALL include `go.mod`, `package.json`, `tsconfig.json`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, `Dockerfile`, Compose files, and the `.claude`, `.codex`, and `.agents` directories. The initial technology vocabulary SHALL cover Go; Node.js and TypeScript; Maven and Gradle; Python; Rust; Ruby; Docker and Compose; the supported agent integrations; Express, NestJS, React, Next.js, Spring Boot, Django, FastAPI, Rails; and PostgreSQL, MySQL, Redis, MongoDB.

The system SHALL derive evidence only from the bytes of a supported marker, SHALL read no more than 1 MiB from an individual marker, and SHALL retain the marker path and normalized technology inferences. It SHALL NOT execute project code, run build tools, install dependencies, access the network, or use technology versions as recommendation criteria.

#### Scenario: Manifest evidence identifies a stack
- **WHEN** a project contains supported manifest or configuration markers for an initial-vocabulary technology
- **THEN** the result includes the normalized detected technologies and their marker evidence

#### Scenario: TypeScript marker identifies a project
- **WHEN** a directory contains `tsconfig.json`, with or without `package.json`
- **THEN** the system detects TypeScript and combines both markers into one scope when they are in the same directory

#### Scenario: Marker exceeds the static-read limit
- **WHEN** a supported marker is larger than 1 MiB
- **THEN** the system does not parse that marker, retains a diagnostic for it, and continues evaluating other readable markers

#### Scenario: Detection encounters executable project content
- **WHEN** a project contains scripts, hooks, or other executable content beside supported markers
- **THEN** the recommendation run reads only the documented static markers and does not execute that content

### Requirement: Bounded monorepo scope isolation
The system SHALL recursively discover project scopes beneath the requested project, including the requested root even when it has insufficient evidence. Only `go.mod`, `package.json`, `tsconfig.json`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `pyproject.toml`, `Cargo.toml`, and `Gemfile` SHALL create an independent scope. Docker, Compose, and supported agent-configuration markers SHALL add evidence only to their containing scope.

Evidence from a child scope SHALL NOT be attributed to its parent or sibling scope, and root-level evidence SHALL NOT be inherited by child scopes. The traversal SHALL not follow directory soft links, SHALL skip `.git`, `node_modules`, `vendor`, `dist`, `build`, `target`, `.cache`, `.next`, `.turbo`, `.venv`, `venv`, and `__pycache__`, and SHALL inspect no more than 10,000 directory entries. A skipped link, skipped marker, unreadable nested marker, or reached traversal limit SHALL produce a diagnostic and SHALL set the top-level scan-completeness indicator to false while preserving discovered results.

#### Scenario: Monorepo contains unrelated services
- **WHEN** a repository contains independently marked Go and Node.js services in separate nested directories
- **THEN** the result reports separate scopes whose detected technologies and recommendations do not combine the two services' evidence

#### Scenario: Root has no own markers
- **WHEN** a scanned repository has no supported markers at its root but contains independently marked nested projects
- **THEN** the result includes the root with insufficient evidence and separately reports the nested scopes without inventing a root technology stack

#### Scenario: Scan reaches its entry limit
- **WHEN** recursive discovery reaches 10,000 directory entries
- **THEN** the command returns the scopes found so far with a traversal-limit diagnostic and a false scan-completeness indicator

### Requirement: Eligible skill matching and deterministic ranking
The system SHALL recommend only eligible directory skills from the configured skill library: a candidate MUST be a directory containing `SKILL.md`. It SHALL match normalized detected technologies against a candidate's identifier, description, `SKILL.md` body text, and companion `tags`. A companion tag SHALL count as a technology tag only when it exactly matches a normalized technology-vocabulary identifier; the command SHALL NOT introduce a new companion metadata field for recommendations.

The system SHALL match complete normalized technology terms rather than arbitrary substrings. It SHALL retain every matching catalog field and technology as matching evidence. Explicit matching companion tags SHALL rank above identifier and description matches, which SHALL rank above body-text-only matches; a body-text-only match SHALL have low confidence. The system SHALL rank all matching skills deterministically by descending score and then ascending skill identifier, without silently limiting results.

#### Scenario: Technology metadata produces a recommendation
- **WHEN** a detected technology matches an eligible skill's identifier, description, body text, or exact companion tag
- **THEN** the skill is included in that scope's ranked recommendations with its matching technology and catalog field as evidence

#### Scenario: Invalid catalog entry resembles a match
- **WHEN** a configured-library entry has matching text but does not contain `SKILL.md`
- **THEN** the entry is excluded from recommendations

#### Scenario: Ranking evidence is equal
- **WHEN** two eligible skills have equal ranking evidence for one scope
- **THEN** their relative order is ascending by skill identifier

### Requirement: Explainable human and JSON results
The system SHALL render a human-readable recommendation result by default and SHALL render machine-readable JSON when `--json` is supplied. Every recommendation in either representation SHALL include its skill identifier, description, matching evidence, and match reason.

Successful JSON output SHALL contain the resolved absolute `project` path, boolean `scanComplete`, and `scopes`. Each scope SHALL contain a `path` relative to `project` (`"."` for the requested root), `status`, `technologies`, `evidence`, `diagnostics`, `recommendations`, and `nextAction`; all array fields SHALL be present even when empty. Marker-evidence paths SHALL be relative to their scope. Each recommendation SHALL contain `identifier`, `description`, `matchReason`, `matchingEvidence`, and `confidence`, where confidence is `"low"`, `"medium"`, `"high"`, or `null`.

#### Scenario: Agent requests structured recommendations
- **WHEN** an agent runs `skill-manager recommend --project . --json`
- **THEN** the command emits JSON with the stable success fields, deterministically ordered scopes and recommendations, and no interactive prompts

#### Scenario: Operator reviews a recommendation
- **WHEN** a human runs `skill-manager recommend` without `--json`
- **THEN** the output identifies the evaluated scope and shows each recommended skill with its description, match reason, and supporting evidence

### Requirement: Actionable empty and incomplete results
The system SHALL complete successfully when no recognized markers or no matching eligible skills are found. A scope with no recognized technology SHALL have status `insufficient_evidence` and next action `use_catalog_search`; a scope with detected technologies but no eligible matching skill SHALL have status `no_catalog_match` and next action `add_skill_metadata`; a scope with recommendations SHALL have status `recommended` and next action `review_recommendations`. Successful incomplete scans SHALL retain the applicable status and next action alongside diagnostics and `scanComplete: false`.

#### Scenario: Project has no recognized markers
- **WHEN** no supported static marker is found for an evaluated scope
- **THEN** the result identifies insufficient evidence and advises the operator to select a project containing supported markers or use catalog search

#### Scenario: Detected stack has no eligible match
- **WHEN** a scope has detected technologies but no eligible catalog skill matches them
- **THEN** the result retains the detected technologies, reports that no catalog matches were found, and advises adding appropriate skill tags or using catalog search

#### Scenario: Nested marker cannot be read
- **WHEN** an independently scoped nested project's supported marker cannot be read
- **THEN** the command continues evaluating other scopes and reports the localized read failure in diagnostics

### Requirement: Catalog and JSON command errors
The system SHALL treat an unreadable or invalid configured skill library, a catalog read failure, and an invalid or unreadable requested project root as command errors rather than empty recommendation results. In human-readable mode, it SHALL write a concise diagnostic to standard error and return a non-zero status. In JSON mode, it SHALL write a JSON object with an `error` object containing stable `code`, `message`, and `path` fields to standard output and return a non-zero status.

#### Scenario: Configured skill library cannot be read
- **WHEN** the configured skill library is missing or unreadable
- **THEN** the command returns a catalog error rather than `no_catalog_match`

#### Scenario: Agent supplies an invalid project path
- **WHEN** an agent runs `recommend --project` with an invalid path and `--json`
- **THEN** standard output contains the structured error object and the command exits non-zero

### Requirement: Recommendation has no management side effects
The system SHALL treat recommendation as a read-only catalog and project-inspection operation. A recommendation run SHALL NOT create, remove, or alter project activations, project files, skill-library entries, the operation journal, or the global baseline.

#### Scenario: Recommendation precedes activation
- **WHEN** an operator runs recommendation for a project with matching skills
- **THEN** the project skill locations, configured skill library, operation journal, and global baseline remain unchanged until a separate explicit management command is confirmed
