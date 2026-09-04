# Skill Manager CLI and Recommendations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the Go Skill Manager foundation and its read-only, explainable project-stack recommendation command.

**Architecture:** Keep domain records and application services independent from Cobra command rendering. Filesystem reads and writes sit behind narrow repositories and adapters; recommendation receives only read-only catalog and project-inspection interfaces. JSON output is a stable DTO layer over domain results.

**Tech Stack:** Go standard library, Cobra CLI, `testing`, `t.TempDir` fixtures, YAML parsing for `.skill-manager.yaml`.

**Spec:** `openspec/changes/create-skill-manager-cli/` and `openspec/changes/recommend-skills-by-project-stack/` in the primary workspace; implementation must satisfy their proposal, specs, design, and tasks.

## Global Constraints

- Never execute skill or project content, install dependencies, or access the network during Skill Manager operations.
- Every mutation previews changes, requires confirmation, records a reversible journal entry, and protects unmanaged paths by default.
- Recommendation is read-only: it never changes project activation, library, journal, or global baseline.
- Recommendation scans at most 10,000 entries and 1 MiB per supported marker, never follows directory soft links, and emits stable JSON fields.

---

### Task 1: Go module, catalog records, and configuration

**Files:**
- Create: `go.mod`, `cmd/skill-manager/main.go`, `internal/catalog/catalog.go`, `internal/catalog/catalog_test.go`, `internal/config/config.go`, `internal/config/config_test.go`

**Interfaces:**
- Produces `catalog.Skill`, `catalog.Discover(root string) ([]Skill, []Diagnostic, error)`, and `config.Load(path string) (Config, error)`.
- `Skill` carries identifier, absolute source path, description, body text, tags, and compatibility declarations.

- [ ] Write tests for valid and invalid `SKILL.md` directories, default-library resolution, and companion metadata parsing.
- [ ] Run `go test ./internal/catalog ./internal/config` and verify each test fails because the package is absent.
- [ ] Implement the smallest discovery and configuration code that passes those tests without traversing skill resources or executing files.
- [ ] Re-run `go test ./internal/catalog ./internal/config` and `gofmt -w cmd internal`; both must succeed.

### Task 2: Read-only catalog CLI

**Files:**
- Create: `internal/search/search.go`, `internal/search/search_test.go`, `internal/cli/root.go`, `internal/cli/search.go`, `internal/cli/search_test.go`

**Interfaces:**
- Consumes `catalog.Skill`.
- Produces `search.Match(query string, skills []catalog.Skill) []catalog.Skill` and a Cobra `search` command with `--json`.

- [ ] Write failing tests for identifier, description, tag, and body-text search plus deterministic JSON output.
- [ ] Run the focused test package and verify failure is caused by missing search behavior.
- [ ] Implement normalized matching and human/JSON rendering without mutating catalog or project state.
- [ ] Re-run focused tests and `go test ./...`; both must pass.

### Task 3: Adapters, guarded mutations, and lifecycle services

**Files:**
- Create: `internal/adapter/adapter.go`, `internal/adapter/adapter_test.go`, `internal/operation/plan.go`, `internal/operation/journal.go`, `internal/operation/operation_test.go`, `internal/lifecycle/service.go`, `internal/lifecycle/service_test.go`

**Interfaces:**
- `adapter.Adapter` supplies project/global skill paths for Claude Code and Codex.
- `operation.Plan` contains previewable changes and `operation.Journal` records reversible confirmed operations.
- `lifecycle.Service` implements add, list, remove, adopt, and fork.

- [ ] Write failing fixture tests for selected-target links, unmanaged-path refusal, confirmation, journal undo, adoption conflict, and fork independence.
- [ ] Run focused tests and verify failures precede implementation.
- [ ] Implement absolute-link placement, staged guarded changes, journal persistence, and lifecycle operations with no force replacement by default.
- [ ] Re-run focused tests and `go test ./...`; both must pass.

### Task 4: Initialization, diagnostics, reconciliation, and CLI commands

**Files:**
- Create: `internal/diagnostic/doctor.go`, `internal/diagnostic/doctor_test.go`, `internal/initcmd/init.go`, `internal/initcmd/init_test.go`, command files under `internal/cli/`

**Interfaces:**
- `doctor.Scan(project string) []Finding` reports invalid entries, orphaned links, unsupported agents, and Git guidance.
- `initcmd.Service` installs only the Operator skill after a confirmed plan.

- [ ] Write failing tests for orphan detection, confirmed reconciliation, conservative library deletion, global-baseline isolation, and operator-skill content.
- [ ] Run focused tests and verify each failure is due to missing behavior.
- [ ] Implement diagnostics, reconciliation, initialization, command help, and the complete guarded command surface.
- [ ] Re-run focused tests and `go test ./...`; both must pass.

### Task 5: Recommendation models, vocabulary, and static detectors

**Files:**
- Create: `internal/recommend/model.go`, `internal/recommend/vocabulary.go`, `internal/recommend/detect.go`, and focused `_test.go` files.

**Interfaces:**
- `recommend.Inspect(project string) (Inspection, error)` returns project-relative scopes, technologies, evidence, diagnostics, and `ScanComplete`.
- `recommend.Technology` is a canonical identifier and display label; no version is a match criterion.

- [ ] Write failing tests for Go, Node.js, TypeScript with and without `package.json`, initial framework/database vocabulary, oversized marker diagnostics, and no code execution.
- [ ] Run `go test ./internal/recommend` and verify each failure is caused by missing detector behavior.
- [ ] Implement 1 MiB bounded static readers and the documented marker registry without reading arbitrary source files.
- [ ] Re-run `go test ./internal/recommend`; it must pass.

### Task 6: Scope discovery and explainable ranking

**Files:**
- Modify: `internal/recommend/detect.go`, `internal/recommend/model.go`
- Create: `internal/recommend/rank.go`, `internal/recommend/rank_test.go`

**Interfaces:**
- `recommend.DiscoverScopes(project string) Inspection` always includes `.` and never follows directory soft links.
- `recommend.Rank(scope Scope, skills []catalog.Skill) []Recommendation` returns score-ordered, identifier-tiebroken explanations.

- [ ] Write failing tests for isolated Go/Node monorepos, exclusions, 10,000-entry partial scans, exact tag matches, body-text low confidence, non-technology tags, and equal-score ordering.
- [ ] Run focused tests and verify their failures are expected.
- [ ] Implement scope isolation, documented exclusions, matching weights, confidence bands, and stable sort without result truncation.
- [ ] Re-run focused tests and `go test ./...`; both must pass.

### Task 7: Recommendation service, renderers, and command

**Files:**
- Create: `internal/recommend/service.go`, `internal/recommend/render.go`, `internal/cli/recommend.go`, and focused `_test.go` files.

**Interfaces:**
- `recommend.Service.Recommend(project string) (Result, error)` produces `recommended`, `insufficient_evidence`, or `no_catalog_match` scopes.
- JSON success uses `project`, `scanComplete`, and `scopes`; JSON failure uses `error.code`, `error.message`, and `error.path`.

- [ ] Write failing command tests for default/explicit paths, catalog failures, human output, success JSON, partial JSON, empty JSON, and non-zero structured JSON errors.
- [ ] Run focused command tests and verify failure before implementation.
- [ ] Implement the read-only service and Cobra command with stable output, enum next actions, relative evidence paths, and no interactive prompt.
- [ ] Re-run focused tests and `go test ./...`; both must pass.

### Task 8: Documentation and whole-system verification

**Files:**
- Create or modify: `README.md`, `docs/recommend.md`, integration fixtures under `internal/cli/testdata/`

- [ ] Write failing end-to-end fixture tests spanning catalog discovery, guarded lifecycle behavior, Go and TypeScript recommendations, unknown projects, no-match projects, partial scans, and JSON errors.
- [ ] Run `go test ./...` and verify the newly added end-to-end tests fail for missing coverage or behavior.
- [ ] Add usage documentation for every CLI command, marker vocabulary, output contract, activation separation, Git guidance, and recovery.
- [ ] Run `gofmt -w cmd internal`, `go test ./...`, `go vet ./...`, `openspec validate create-skill-manager-cli --strict`, and `openspec validate recommend-skills-by-project-stack --strict`; all must pass.
