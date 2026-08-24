## 1. Go CLI Foundation and Domain Model

- [ ] 1.1 Scaffold a Go module with an executable `skill-manager` entry point and verify `go test ./...` and the help command run successfully.
- [ ] 1.2 Implement configuration and filesystem-domain models for directory skills, companion metadata, absolute managed links, and the local operation journal; verify with unit tests using `t.TempDir` fixtures.
- [ ] 1.3 Implement Claude Code and Codex adapter interfaces for global and project skill locations, plus read-only unsupported-agent detection; verify adapter path and detection tests pass.
- [ ] 1.4 Keep CLI commands and prompts separate from application services, domain packages, and filesystem infrastructure; verify domain and application packages do not depend on terminal presentation code.

## 2. Catalog and Agent-Friendly Discovery

- [ ] 2.1 Implement configured-library discovery and validation of directory skills without executing skill contents; verify invalid entries are excluded and reported by catalog tests.
- [ ] 2.2 Implement parsing of `SKILL.md` metadata and optional `.skill-manager.yaml`, including tags and compatibility declarations; verify legacy and metadata-bearing fixtures pass.
- [ ] 2.3 Implement text search and JSON output for catalog results; verify identifier, description, tag, and body-text matches through automated tests.
- [ ] 2.4 Implement the interactive `select` workflow with search, multi-selection, target-agent selection, and final confirmation; verify its selection-to-add handoff with terminal interaction tests or a documented manual fixture.

## 3. Project Skill Lifecycle

- [ ] 3.1 Implement project target detection, inventory listing, and absolute-link activation for explicitly selected Claude Code and Codex targets; verify fixture projects contain the expected managed links and labels.
- [ ] 3.2 Implement compatibility warnings and `--all-detected` target selection without treating a warning as an installation failure; verify both paths in CLI integration tests.
- [ ] 3.3 Implement managed-link removal that refuses unmanaged project skills and preserves the library source; verify the refusal and successful removal cases.
- [ ] 3.4 Implement adoption of eligible project directory skills with conflict detection and conversion to managed links; verify success and same-identifier conflict scenarios.
- [ ] 3.5 Implement forking of a managed project skill into an independent directory; verify changing the fork fixture does not alter the library fixture.

## 4. Guarded Operations and Recovery

- [ ] 4.1 Implement plan rendering, confirmation handling, conflict strategies, and force double-confirmation for all mutating operations; verify unmanaged directories, files, and unexpected links remain unchanged by default.
- [ ] 4.2 Implement operation-journal persistence and undo for recent reversible activation, removal, adoption, and fork operations; verify each operation restores its recorded prior state.
- [ ] 4.3 Implement `doctor` for current-project and explicit-root scans, including invalid library entries, orphaned links, and unsupported agents; verify reported findings with fixture layouts.
- [ ] 4.4 Implement confirmed `reconcile` for orphaned managed links and conservative library deletion that requires force confirmation; verify a moved-library fixture repairs links and deletion is refused by default.
- [ ] 4.5 Implement Git tracked-link warnings and proposed scoped ignore updates that require confirmation; verify unmanaged skill tracking rules are never modified implicitly.

## 5. Global Baseline and Release Verification

- [ ] 5.1 Implement guarded `init` to verify CLI availability and install or update only the minimal Operator skill for Claude Code and Codex; verify project commands leave global locations untouched.
- [ ] 5.2 Author the Operator skill instructions for search, recommendation rationale, human confirmation, and subsequent CLI invocation; verify the installed files conform to each supported agent's directory-skill layout.
- [ ] 5.3 Add end-to-end fixture coverage for `init`, `search`, `select`, `add`, `list`, `remove`, `adopt`, `fork`, `doctor`, `reconcile`, and `undo`; verify the complete automated suite passes without executing a fixture skill script.
- [ ] 5.4 Add CLI usage documentation covering local-only activation, Git guidance, compatibility warnings, recovery commands, and the post-CLI React WebUI and Wails roadmap; verify each first-release command is available from `skill-manager --help`.
