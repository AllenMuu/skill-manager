# Skill Manager

Skill Manager manages the availability of local agent skills across a shared skill library, agent-wide locations, and individual repositories. It discovers skills from a configurable local library, activates them per project through machine-local soft links, and keeps every mutation guarded, journaled, and reversible.

Skill Manager never executes skill-provided code and never installs dependencies.

## How activation works

- **Skill library** — the authoritative local collection of reusable skills (default `~/.agents/skills`, configurable). Each skill is a directory containing `SKILL.md` and optional companion metadata (`.skill-manager.yaml`).
- **Project activation** — selected skills appear in a repository's agent-recognized skill location (`.claude/skills/<id>`, `.codex/skills/<id>`) as absolute soft links into the library. Activation is machine-local by design: it is not a committed manifest and is not portable between machines.
- **Target agents** — Claude Code and Codex are supported in the first release. A project can activate the same skill for multiple agents.
- **Global baseline** — a deliberately small set of resources installed agent-wide. Only `init` touches global locations; project commands never do.

## Build

```text
go build ./cmd/skill-manager
```

## Configuration

Skill Manager reads a small YAML file with the library location:

```yaml
library: ~/.agents/skills
```

Pass it with `--config <path>`; without it the default `~/.agents/skills` is used.

## Getting started

```text
skill-manager init                 # verify CLI and install the global Operator skill
skill-manager search <query>        # find skills in the library
skill-manager select --project .    # interactive search, multi-select, and activation
skill-manager add <skill> --project . --target codex
skill-manager list --project .      # show managed/unmanaged/orphaned skills
```

## Commands

| Command | Purpose |
|---|---|
| `init` | Verify the CLI and install or update the minimal global Operator skill (guarded, confirmed) |
| `search <query>` | Search library skill identifiers, descriptions, bodies, and tags |
| `recommend` | Detect the project stack statically and rank matching skills (read-only) |
| `select` | Interactive workflow: search, choose skills, choose targets, confirm |
| `add <skill>` | Activate a library skill for selected target agents |
| `list` | Inventory project skills with their status |
| `remove <skill>` | Remove one managed project link |
| `adopt <skill>` | Move an eligible project skill into the library and link it back |
| `fork <skill>` | Replace a managed link with an independent project-local copy |
| `doctor` | Report invalid library entries, orphaned links, unsupported agents, and Git tracking state |
| `reconcile` | Repair orphaned managed links after a library move (confirmed) |
| `undo` | Restore the pre-operation state of the latest journaled operation |
| `delete <skill>` | Delete an eligible library skill (requires `--force`, refuses by default) |

Every mutating command previews its plan, requires confirmation (`--yes` or an interactive prompt), records a reversible journal entry, and leaves unmanaged directories, ordinary files, and unexpected links untouched by default.

## Conflict handling

When activation would replace an existing unmanaged path, Skill Manager refuses by default. To replace it, select the conflict strategy and supply force confirmation:

```text
skill-manager add <skill> --project . --target codex --conflict replace --force
```

The replacement is journaled like any other operation, so `undo` restores the previous content.

## Compatibility warnings

Skills may declare target-agent compatibility in `.skill-manager.yaml`:

```yaml
compatibility:
  - codex
```

Installing for an undeclared target produces a warning in the plan preview. A warning never blocks an explicit installation.

## Git guidance

Managed links are machine-local. Committing them would make activation portable, which conflicts with the local-only model, so Skill Manager reports their Git tracking state:

```text
skill-manager doctor --project .                    # show tracked/ignored/would-be-tracked links
skill-manager doctor --project . --update-gitignore  # offer exact scoped ignores after confirmation
```

`doctor` only ever appends the exact paths of managed links it owns; it never rewrites unrelated tracking rules.

## Recovery

- `skill-manager undo` — revert the latest journaled operation.
- `skill-manager doctor` — diagnose invalid library entries, orphaned links, and unsupported agents.
- `skill-manager reconcile` — relink orphaned managed links after the library moved (uses journal ownership, confirmed, rolled back on failure).

## Recommend

`recommend` is a read-only command that detects the current project's technology stack from static markers and ranks matching skills from the library. It never executes project code, installs dependencies, uses the network, or modifies any file:

```text
skill-manager recommend                 # evaluate the current directory
skill-manager recommend --project ../service
skill-manager recommend --project . --json
```

Detected technologies come from the documented marker vocabulary: `go.mod`, `package.json`, `tsconfig.json`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, `Dockerfile`, Compose files, and the `.claude`, `.codex`, and `.agents` directories. The technology vocabulary covers Go, Node.js, TypeScript, Maven, Gradle, Python, Rust, Ruby, Docker, Compose, Express, NestJS, React, Next.js, Spring Boot, Django, FastAPI, Rails, PostgreSQL, MySQL, Redis, MongoDB, and the supported agents. Matching skills are ranked by exact companion tags first, then identifier and description text, then body text; each recommendation explains why it matched. Traversal skips dependency and build directories and directory soft links, reads at most 1 MiB per marker, and stops after 10,000 entries with `scanComplete: false`.

Monorepos report each independently marked sub-project as its own scope, so unrelated services never blend into one stack. Scopes without recognized markers return `insufficient_evidence` with a pointer to catalog search; stacks with no matching skill return `no_catalog_match` with guidance to add technology tags to companion metadata.

## Roadmap

- **Agent Manager** — generalize into multi-agent resource governance covering skills, subagents, and shared memory (see the GitHub issues).
- **React WebUI** — a post-CLI web interface over the same local services.
- **Wails** — desktop packaging of the WebUI.
