# GitHub Similar-Implementation Scan

Checked 2026-08-24 against repository READMEs and source layouts on GitHub.

## Direct match

- [omrikais/skill-manager](https://github.com/omrikais/skill-manager) is the closest known implementation. It is a Node/TypeScript CLI that centralizes Claude Code and Codex skills, deploys atomic symlinks, provides a multi-select TUI, imports/adopts unmanaged skills, offers search, doctor/repair, backups and rollback, project manifests, context-aware suggestions, and an MCP server. It is broader than this project's scoped proposal, but differs in important product choices: its canonical store is `~/.skill-manager/skills`, it uses a checked-in `.skills.json` project manifest, and it supports automatic adoption and automatic activation. This project instead adopts the existing `~/.agents/skills` library, makes project links machine-local with no manifest, and requires explicit confirmation for adoption or activation.

## Strong adjacent implementations

- [getsentry/dotagents](https://github.com/getsentry/dotagents) manages agent configuration and skills across several coding-agent tools. It is a useful reference for adapter layout and deployment, but it is primarily a dotfiles/configuration manager rather than a project-by-project skill activation lifecycle.
- [graehl/agents](https://github.com/graehl/agents) maintains a canonical agent corpus and has an installer that selects one or more harnesses, including Codex, Claude, Pi, OpenCode, Grok, and Copilot. It demonstrates multi-agent installation, but does not present the proposed project-level adoption/fork/journal model.
- [Randroids-Dojo/skills](https://github.com/Randroids-Dojo/skills) documents a canonical `.agents/skills` layout and symlinked Claude/OpenCode clients, with separate project and global locations. It validates the shared-library approach but is a skill collection/distribution rather than a local manager with safe lifecycle operations.
- [zazencodes/agent-skills](https://github.com/zazencodes/agent-skills) includes a setup script that creates per-agent links from a canonical skills folder. It overlaps in multi-agent symlink setup, but does not provide a general-purpose catalog, project selection, conflict policy, or undo journal.

## Conclusion

Yes: the general idea already has working GitHub implementations, especially `omrikais/skill-manager`. The differentiated opportunity is not basic symlink deployment; it is an opinionated, safety-first local workflow around the existing `~/.agents/skills` library: no project manifest, explicit human-approved agent actions, coexistence with unmanaged project skills, deliberate adoption/fork, Git-aware local activation, and reversible journaling. Before implementation, evaluate whether to contribute to or fork the direct match instead of rebuilding its overlapping core.
