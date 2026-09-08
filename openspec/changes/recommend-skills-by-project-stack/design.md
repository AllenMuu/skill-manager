## Context

The initial CLI change establishes a Go domain core, a configured local skill catalog, companion metadata, and a terminal layer that must remain separate from application behavior. This change adds a read-only consumer of that catalog and is implemented only after `create-skill-manager-cli` supplies those prerequisites. See [proposal.md](./proposal.md) for motivation and [project-skill-recommendation/spec.md](./specs/project-skill-recommendation/spec.md) for the behavioral contract.

The repository is still specification-first, so package names below describe intended boundaries rather than an existing source layout. Recommendation must retain the established local-first rule: it cannot execute a marker, start a tool, install a dependency, use the network, or participate in guarded management operations.

## Goals / Non-Goals

**Goals:**

- Make detection, matching, ranking, and presentation independently testable from filesystem fixtures.
- Make every technology inference and recommendation explainable from retained evidence.
- Support the documented initial vocabulary, including TypeScript, without making future marker or tag additions invasive.
- Keep JSON output a terminal DTO with stable field names and deterministic ordering.

**Non-Goals:**

- Inferring technologies from arbitrary source files, README prose, lockfiles alone, or executed build output.
- Claiming that a skill is technically compatible, safe, or automatically selected for activation.
- Persisting project detection results, adding a project manifest, writing an index, or changing catalog metadata automatically.
- Inferring technologies from versions or adding a recommendation-specific companion-metadata field.
- Scanning beyond the documented directory exclusions, 10,000-entry traversal budget, 1 MiB per-marker read budget, or directory soft-link boundary.

## Decisions

### Build recommendation as a read-only application service

The CLI will parse `recommend [--project PATH] [--json]`, resolve and validate the target directory, and call a `RecommendationService`. The service receives a read-only project inspector plus the existing catalog reader; it returns a domain result that the terminal renders as text or JSON. It will not receive activation, adapter-write, journal, or confirmation dependencies.

This makes no-side-effect behavior structural and lets tests use a recording filesystem or fixture directories. Adding recommendation to `select` was rejected because interactive selection conflates advice with a future mutation and prevents useful agent JSON workflows.

### Use a declarative marker registry and normalized technology vocabulary

An in-code registry will describe each supported marker: relative filename or directory, static parser, scope-boundary strength, and normalized technology contributions. Parsers read at most 1 MiB of marker bytes and use format-aware decoding where practical; malformed or oversized markers yield bounded diagnostics rather than command failure. Initial entries cover the required manifests and directories, plus Go, Node.js, TypeScript, Maven, Gradle, Python, Rust, Ruby, Docker/Compose, supported agents, the seven named frameworks, and the four named databases.

Detected technology records retain a canonical identifier, category, display label, and one or more relative marker paths. This canonical vocabulary is the join point for detectors and companion tags. A new marker or alias therefore requires one registry/vocabulary addition and fixture coverage, rather than changes to ranking code.

Free-text catalog fields are normalized through the same tokenizer and aliases, but their matches are marked as text-derived rather than metadata-derived. Only existing companion `tags` that exactly equal a canonical technology identifier count as technology metadata; no `technologies` field is added. A centralized external technology taxonomy or a new metadata schema was rejected because each adds a second data source or compatibility boundary before the first release needs it.

### Discover scope boundaries before collecting scope evidence

The inspector first walks the requested project tree using a deterministic traversal that examines at most 10,000 entries, skips the explicit excluded directories in the spec, and never follows directory soft links. Scope-forming manifests identify candidate roots; Docker, Compose, and agent directories contribute evidence only to their containing scope. It then assigns a marker to its closest discovered scope; the requested root receives only markers located at its own level, is always emitted, and never contributes evidence to child scopes. A nested scope is emitted independently even when the requested root has no own marker.

This two-phase process prevents a repository's aggregate scan from presenting unrelated services as one hybrid stack. Treating every nested configuration file as parent evidence was rejected because it violates the monorepo requirement; treating only the requested root as a project was rejected because it loses useful child services.

### Rank with explainable weighted evidence and stable tie-breaking

For each scope, the matcher builds one explanation per candidate from complete normalized technology-term hits. Exact companion tags receive the strongest weight, identifier/description matches receive the next weight, and body-text matches receive the lowest weight. Multiple distinct technology matches add weight; repeated occurrences of the same token do not. The result retains matched technologies, catalog fields, and a concise match reason; it returns every qualifying skill rather than applying an implicit result limit.

The raw score is internal. Presentation exposes a confidence only when the score can be derived from recognized evidence; it uses documented low/medium/high bands rather than a misleading precision score. Sort by descending score, then ascending skill identifier. A black-box semantic ranking model was rejected because users need to understand and challenge recommendations, and raw occurrence counts were rejected because verbose skill bodies would dominate tags.

### Model successful empty states explicitly

The service returns a result status for each evaluated scope: `recommended`, `insufficient_evidence`, or `no_catalog_match`. Invalid project roots and unavailable catalog access are command errors. Unknown technology markers, malformed or oversized markers, unreadable nested markers, and zero matches are successful result states with diagnostics. A top-level `scanComplete` flag makes partial discovery machine-visible. Both renderers consume the same status and next-action field.

This avoids treating a normal discovery outcome as a failed command while preserving enough distinction for agents to decide whether to run catalog search. Returning an empty recommendation array alone was rejected because it cannot distinguish missing project evidence from a catalog gap.

### Keep output DTOs explicit and version-stable by field

The JSON renderer serializes a success object with absolute `project`, boolean `scanComplete`, and `scopes`. Each scope has a project-relative `path`, `status`, `technologies`, `evidence`, `diagnostics`, `recommendations`, and `nextAction`; all arrays are present when empty and evidence paths are scope-relative. A recommendation contains `identifier`, `description`, `matchReason`, `matchingEvidence`, and a `low`/`medium`/`high`/`null` `confidence`. Command failures render a standard-output JSON `error` object with `code`, `message`, and `path`, then return non-zero. Arrays are sorted deterministically; no map iteration determines observable output order. Human output is derived from this same result but can be formatted for scanning.

The first release will not include an output schema version because this is a new command with no installed consumers. Tests will lock the published field names and empty-state shapes. A generic map-based renderer was rejected because it makes a scriptable output contract accidental.

## Risks / Trade-offs

- [Markers cannot capture every framework or unusual project layout] → Document the initial vocabulary, emit actionable empty states, and add registry entries with fixtures over time.
- [Monorepo traversal can become costly or enter dependency trees] → Use the explicit exclusions, avoid following directory soft links, cap traversal at 10,000 entries, and emit diagnostics with `scanComplete: false` when any coverage is lost.
- [Free-text matching can produce weak suggestions] → Weight explicit metadata above text, retain the matching field in evidence, and label low-confidence results.
- [Manifest parsing varies across ecosystems] → Limit parsing to the documented vocabulary and 1 MiB static reads; malformed optional files produce localized diagnostics rather than aborting other scopes.
- [Future changes to JSON fields could disrupt agents] → Define renderer DTOs explicitly and protect them with JSON contract tests.

## Migration Plan

1. Complete the prerequisite `create-skill-manager-cli` catalog and CLI foundation, then add the read-only recommendation packages and fixture tests without changing existing management command behavior.
2. Register `recommend` in CLI help and publish its human and JSON usage examples plus the initial marker/tag vocabulary.
3. Release as an additive command; existing skill libraries work immediately because metadata remains optional.
4. Roll back by removing the command and recommendation packages. No project activation, library entry, journal record, or baseline artifact requires recovery.
