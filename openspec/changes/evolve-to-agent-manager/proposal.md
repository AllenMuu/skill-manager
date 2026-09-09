## Why

Skill Manager currently governs directory skills for Claude Code and Codex, but users operate several coding agents whose skills, subagents, and durable knowledge are fragmented into incompatible, agent-owned locations. The completed CLI foundation provides safe lifecycle operations; now it needs explicit extension points so new resource domains and agent integrations can be added without rewriting or weakening those protections.

## What Changes

- Rename the product and primary command to **Agent Manager**, while retaining `skill-manager` as a deprecating compatibility alias and preserving existing local skill libraries, activations, metadata, and journals.
- Introduce agent-neutral `AgentAdapter`, managed-resource kind, and resource-handler boundaries; move current skill placement and lifecycle rules behind them.
- Add agent discovery and diagnostics that inventory supported agents, their resource capabilities, managed activations, unmanaged resources, compatibility warnings, and broken links.
- Add a canonical, agent-neutral SubAgent resource with discovery, validation, capability-aware preview, guarded installation, and reversible filesystem operations for Claude Code, Codex, and Pi.
- Add a shared-memory integration boundary with provider configuration, capability discovery, explicit user/project scopes, and diagnostics; implement one provider path without duplicating provider data into individual agent stores by default.
- Update product terminology and architecture documentation. **BREAKING:** `agent-manager` becomes the documented primary command; `skill-manager` remains available as a temporary compatibility alias rather than being removed.

## Capabilities

### New Capabilities

- `agent-resource-governance`: Discover supported agents and govern typed resources through stable adapter and handler contracts.
- `subagent-resource-lifecycle`: Store, inspect, validate, preview, and safely install canonical SubAgent definitions through agent adapters.
- `shared-memory-provider-integration`: Configure and diagnose user-owned shared Memory providers independently from agent-specific integration.

### Modified Capabilities

- `skill-catalog`: Preserve the existing skill library as the authoritative Skill resource domain under Agent Manager terminology and generic resource inventory.
- `project-skill-lifecycle`: Route existing project-skill lifecycle behavior through resource and agent-adapter boundaries while retaining activation, adoption, fork, reconciliation, and conflict guarantees.
- `agent-adapters-and-initialization`: Expand target-agent integration from fixed skill locations to capability-declaring adapters, add Pi, and introduce the primary `agent-manager` CLI with a compatibility alias.
- `guarded-operations-and-diagnostics`: Extend guarded previews, journals, undo, and diagnostics to typed resource operations and cross-agent inventory without reducing filesystem safeguards.

## Impact

- Affects the Go CLI entrypoints, domain packages, filesystem adapters, configuration and journal formats, tests, `CONTEXT.md`, and a new architecture ADR.
- Maintains local-first behavior: no managed resource code execution, no implicit dependency installation, and no new network dependency for management workflows.
- Requires deterministic backward-compatibility and migration coverage for existing Skill Manager state before new SubAgent or Memory mutations are enabled.
