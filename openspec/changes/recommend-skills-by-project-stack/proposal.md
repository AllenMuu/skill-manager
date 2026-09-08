## Why

Catalog search requires an operator to translate an unfamiliar project's technology stack into skill-search terms. A read-only recommendation command can make the existing local skill library useful at project entry without changing project activation or executing project content.

## What Changes

- Add a `recommend` CLI command that scans the current directory by default, or an explicit `--project` path, for documented project markers without executing code or installing dependencies.
- Detect technologies from project manifests and configuration markers, including languages, frameworks, build tools, databases, containers, and agent integrations; retain marker evidence and isolate independent nested projects in monorepos.
- Rank eligible directory skills from the configured skill library by matches against identifiers, descriptions, `SKILL.md` body text, companion tags, and companion metadata.
- Present human-readable recommendations and stable JSON output with detected technologies, evidence, match reasons, and confidence information when available.
- Return an actionable, successful no-recommendation result for unknown or insufficient project evidence, and keep the entire workflow separate from activation, the global baseline, and the operation journal.

## Capabilities

### New Capabilities

- `project-skill-recommendation`: Detect a project's technology evidence and produce explainable, read-only ranked recommendations from the configured skill catalog.

### Modified Capabilities

- None.

## Impact

- Adds a read-only CLI surface, project-marker detection and matching/ranking services, and a documented extensible technology vocabulary.
- Reuses catalog discovery and companion metadata; it does not add network access, dependency installation, activation behavior, filesystem mutations, or journal entries.
- Requires unit and fixture coverage for at least two stacks, unknown evidence, and monorepo boundaries, plus CLI JSON contract coverage.
