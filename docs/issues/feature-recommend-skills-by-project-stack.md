---
title: "[Feature] Recommend skills based on the current project's technology stack"
type: feature
labels:
  - enhancement
---

## Is your feature request related to a problem? Please describe.

Skill Manager can search the local skill catalog, but users still need to know which skills are relevant to the project they are currently working on. A project may contain a mix of languages, frameworks, build tools, databases, and agent integrations that are difficult to map to skill metadata by hand.

## Describe the solution you'd like

Add a read-only recommendation command that detects the current project's technology stack and ranks matching skills from the configured Skill Manager library.

Suggested usage:

```text
skill-manager recommend --project .
skill-manager recommend --project . --json
```

The recommendation workflow should:

- Inspect common project markers such as `go.mod`, `package.json`, `pom.xml`, `build.gradle`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, Docker files, and relevant configuration files.
- Detect languages, frameworks, build tools, databases, and other technologies without executing project code or installing dependencies.
- Match detected technologies against skill identifiers, descriptions, body text, tags, and companion metadata.
- Rank results and show the detected evidence, match reasons, and an optional confidence score.
- Support human-readable output and machine-readable JSON output for agent workflows.
- Handle monorepos, unknown technologies, and projects with insufficient evidence without failing the command.
- Keep recommendation separate from activation. The command must not create links or modify project files.

## Describe alternatives you've considered

- Relying on free-text catalog search requires users or agents to identify the relevant technology terms first.
- Requiring users to maintain a project skill manifest would add portable project state and conflict with Skill Manager's machine-local activation model.
- Automatically activating every matching skill would make project state harder to review and could enable skills that the user did not choose.

## Acceptance criteria

- [ ] `recommend` works against the current directory by default and accepts an explicit project path.
- [ ] Detection is based on documented, testable project markers and does not execute project code.
- [ ] Recommendations are limited to eligible catalog entries containing `SKILL.md`.
- [ ] Each recommendation includes the skill identifier, description, matching evidence, and match reason.
- [ ] JSON output contains detected technologies and ranked skill recommendations with stable fields.
- [ ] Monorepos can be scanned without incorrectly treating unrelated nested projects as one stack.
- [ ] Projects with no recognized markers receive a clear, actionable response.
- [ ] Recommendation runs do not modify the project, skill library, operation journal, or global baseline.
- [ ] Unit and fixture tests cover at least two technology stacks, an unknown stack, and a monorepo.

## Additional context

This feature should build on the existing catalog search and companion metadata model. It may require a documented vocabulary for technology tags and a small, extensible detector-to-tag mapping. Recommendations should explain why a skill was suggested so users can review the result before using the existing selection and activation workflows.
