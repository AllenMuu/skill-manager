# Graphiti is the first shared-Memory provider adapter

Graphiti is the first provider adapter because it can be run as a user-owned
local HTTP service and has a small health/discovery boundary. Agent Manager
owns only the provider configuration reference and diagnostics; it does not
install, start, migrate, or write Graphiti data.

The adapter requires a `v1` provider configuration whose reference is an
environment variable containing the Graphiti base URL (for example,
`GRAPHITI_URL`). Credentials, if required by a deployment, remain in the
external environment or credential store and never enter Agent Manager output
or journal evidence. The adapter probes `GET /healthcheck` only when a caller
explicitly enables network discovery. Discovery is unavailable by default,
including when a URL is configured, so ordinary inventory and status checks
cannot cause network access.

Graphiti deployments are not assumed to be installed, reachable, or uniform;
an unavailable service produces a reason and a next action. Optional health
metadata may advertise narrower capabilities and scopes, but malformed
metadata does not turn a healthy endpoint into a write operation. Provider
writes remain outside this task and require a future explicit promotion flow.

The probe uses Graphiti's standard `GET /healthcheck` endpoint.
