## 1. Recommendation Domain and Static Inspection

- [ ] 1.1 Add recommendation request/result, scope, technology, evidence, diagnostic, explanation, confidence, scan-completeness, empty-state, and structured-error models; verify unit tests cover all three successful statuses, partial scans, and error DTOs.
- [ ] 1.2 Define and document the initial normalized technology vocabulary and declarative static-marker registry for Go, Node.js, TypeScript, Maven, Gradle, Python, Rust, Ruby, Docker/Compose, `.claude`, `.codex`, `.agents`, the named frameworks, and the named databases; verify registry tests assert every required marker and canonical technology contribution.
- [ ] 1.3 Implement 1 MiB-bounded static parsers for the documented marker formats that retain scope-relative marker evidence and localized diagnostics without executing content or using versions as criteria; verify fixtures cover TypeScript with and without `package.json`, valid markers, malformed/oversized markers, and executable scripts that are never run.
- [ ] 1.4 Implement deterministic scope discovery that always emits the requested root, uses only documented manifests as scope boundaries, does not inherit root evidence, skips the explicit exclusion list and directory soft links, and stops after 10,000 entries; verify monorepo, root-without-markers, skipped-link, and traversal-limit fixtures.

## 2. Catalog Matching and Read-only Recommendation Service

- [ ] 2.1 Integrate the configured catalog reader so matching considers only eligible directory skills and complete normalized technology terms in identifiers, descriptions, `SKILL.md` body text, and exact canonical companion tags; verify invalid entries without `SKILL.md`, arbitrary substrings, and non-technology tags are excluded.
- [ ] 2.2 Implement explainable weighted matching, low/medium/high confidence bands, and deterministic rank ordering by score then identifier without an implicit limit; verify fixtures cover exact tags, identifier/description, body-text-only, multi-technology, and equal-score matches with retained evidence.
- [ ] 2.3 Implement the read-only recommendation application service with current-directory and explicit-project requests, path validation, catalog-access errors, per-scope empty states, partial-scan diagnostics, and enum next actions; verify recording filesystem/journal tests prove it performs no management mutation or journal write.

## 3. CLI and Output Contracts

- [ ] 3.1 Add `skill-manager recommend [--project PATH] [--json]` to the CLI command surface and connect it only to the read-only recommendation service; verify default-path, explicit-path, missing-path, unreadable-path, non-directory, and unavailable-catalog CLI tests with correct exit statuses.
- [ ] 3.2 Render human-readable grouped scope results containing technologies, recommendations, descriptions, reasons, and evidence; verify fixture snapshots cover recommendations, insufficient evidence, and no catalog match guidance.
- [ ] 3.3 Define the JSON DTO and renderer with absolute project paths, relative scope/evidence paths, `scanComplete`, diagnostics, always-present arrays, enum next actions, and nullable confidence; verify JSON contract tests for successful, incomplete, empty-state, and non-zero structured-error results and absence of interactive prompts.

## 4. Documentation and End-to-end Verification

- [ ] 4.1 Document `recommend` usage, the initial marker/tag vocabulary, scope and traversal boundaries, JSON success/error fields, and its separation from activation; verify the command help and documentation examples match the implemented CLI.
- [ ] 4.2 Add end-to-end fixture coverage for at least two technology stacks including TypeScript, an unknown stack, a detected stack with no eligible catalog match, a partial monorepo scan, and structured errors; verify `go test ./...` passes without executing fixture code, using the prerequisite catalog/CLI foundation from `create-skill-manager-cli`.
