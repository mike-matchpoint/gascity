{{ define "execution-codegen-handoff-contract" }}
## Codegen Event-Bus Filing Contract

**Contract version: 1.0.** This pack is the single source of truth. The
machine-readable schemas live beside this pack at
`schemas/events/common-envelope.v1.schema.json`,
`schemas/events/repo-bug-reported.v1.schema.json`, and
`schemas/events/repo-change-requested.v1.schema.json`. Reference payloads are in
`schemas/events/examples/`. The field lists below MUST match those schemas; if
they ever diverge, the schema wins.

Code-generation cities receive typed event-bus requests. The agent decides the
request shape; the deterministic emitter
`assets/scripts/publish-cross-city-event.sh` validates against the schemas above
and performs the actual publish.

Use these generic event intents:

- `RepoBugReported.v1` for a concrete regression, runtime failure, validation
  failure, adapter failure, artifact contract failure, or broken deterministic
  behavior.
- `RepoChangeRequested.v1` for a missing capability, planned migration,
  deterministic workflow change, prompt/eval infrastructure change, runbook,
  schema support code, deployment change, or documentation update.

### Envelope (required for every event)

`event_type`, `event_version` (`v1`), `process_slug`, `city_pair_slug`,
`source_city`, `source_city_role`, `target_city`,
`target_city_role` (`code-generation-city`), `correlation_id`,
`idempotency_key`, `occurred_at` (RFC3339), `payload`. The emitter fills the
envelope; you supply the typed `payload` and the routing facts.

### `RepoBugReported.v1` payload

Required: `repo`, `severity` (`low|medium|high|critical`), `observed_behavior`,
`expected_behavior`, `reproduction_steps` (non-empty). Optional: `target_branch`
(default `main`), `failing_command`, `evidence_uris`.

### `RepoChangeRequested.v1` payload

Required: `repo`, `request`, `reason`, `required_specs_paths` (non-empty).
Optional: `target_branch` (default `main`), `evidence_uris`.

Every filing must be executable without conversation context: a one-sentence
problem statement, observed vs expected behavior, evidence URIs and a
reproduction or replay path, acceptance criteria grounded in the real runtime
path, and the blocked execution/incident/run/publication the work unblocks.
Name affected files, services, commands, schemas, and the setup/test/deploy/
smoke/rollback/cleanup commands when known. The emitter computes a deterministic
`idempotency_key`; do not file duplicate requests.

If evidence is incomplete, do not publish a vague request. Route back to
evidence bundling, witness review, or incident classification.
{{ end }}
