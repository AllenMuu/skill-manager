# Separate Agent Manager resources, adapters, and providers

Agent Manager SHALL model a managed resource independently from both an agent runtime and a storage provider. A resource handler owns resource-kind validation and lifecycle rules; an agent adapter owns runtime detection, declared capabilities, inspection, and target-specific placement; a Memory provider owns provider configuration and capability discovery.

Existing Skill Manager state remains a first-class Skill resource domain. The `agent-manager` CLI is primary while `skill-manager` is a compatibility alias. Legacy configuration, companion metadata, project links, and journal entries are read and normalized in memory without automatic rewrites.

## Considered Options

- Preserve a Skill-only domain and add agent-specific paths directly to each new command.
- Use one generic filesystem install abstraction for every resource kind.
- Split resource lifecycle rules, agent runtime adapters, and Memory providers behind explicit capability contracts.

The third option is selected. It preserves guarded and reversible Skill link operations, exposes unsupported runtime capabilities instead of silently discarding data, and permits provider-backed Memory without treating it as a filesystem resource. A generic filesystem abstraction was rejected because links, canonical SubAgent definitions, and provider-backed Memory do not share safe mutation semantics.
