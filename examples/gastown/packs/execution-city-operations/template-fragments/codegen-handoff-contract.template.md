{{ define "execution-codegen-handoff-contract" }}
## Codegen Event-Bus Filing Contract

Code-generation cities receive typed event-bus requests. The agent decides the
request shape; deterministic tooling performs the actual publish when a command
or order is provided.

Use these generic event intents:

- `RepoBugReported.v1` for a concrete regression, runtime failure, validation
  failure, adapter failure, artifact contract failure, or broken deterministic
  behavior.
- `RepoChangeRequested.v1` for a missing capability, planned migration,
  deterministic workflow change, prompt/eval infrastructure change, runbook,
  schema support code, deployment change, or documentation update.

Every filing must be executable without conversation context:

- target repo, environment, branch or deploy surface when known
- bug or work-order class
- one-sentence problem statement
- observed behavior and expected behavior
- evidence URIs and reproduction or replay path
- affected files, packages, services, commands, schemas, or runtime surfaces when known
- acceptance criteria grounded in the real runtime path
- setup, test, deploy, smoke, rollback, and cleanup commands when known
- temporary flags, shims, generated artifacts, stale snapshots, or compatibility paths to remove
- blocked execution, incident, run, or publication that the work unblocks
- idempotency key or duplicate-detection note when filing through the event bus

If evidence is incomplete, do not publish a vague request. Route back to
evidence bundling, witness review, or incident classification.
{{ end }}
