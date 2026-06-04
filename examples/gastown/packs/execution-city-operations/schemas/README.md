# execution-city-operations event schemas

**Canonical source of truth** for the cross-city handoff event contract used by
all execution-and-monitoring cities. This pack is installed from gascity source,
so the agents that emit events reach these files at runtime. Any other copy
(e.g. an AWS ingress validator in a Kubernetes harness repo) is a downstream
mirror and must track these files; a parity test should fail if they drift.

Contract version: **1.0**

## Events

- `events/common-envelope.v1.schema.json` — shared cross-city envelope.
- `events/repo-bug-reported.v1.schema.json` — `RepoBugReported.v1`: a concrete
  code defect reported upstream to a code-generation city.
- `events/repo-change-requested.v1.schema.json` — `RepoChangeRequested.v1`: a
  missing capability / change request to a code-generation city.

The human-readable field contract lives in
`template-fragments/codegen-handoff-contract.template.md` and is kept in sync
with these schemas.

## Examples

`events/examples/*.example.json` are valid reference payloads, one per event
type, used by schema-validation tests and by `assets/scripts/publish-cross-city-event.sh --dry-run`.

## Emission

Agents do not hand-roll publishes. The deterministic emitter is
`assets/scripts/publish-cross-city-event.sh`, which builds the envelope,
validates against these schemas, and performs the event-bus put. Infra (the
event bus, egress IAM role, and `GASCITY_EVENT_BUS` / `AWS_REGION` env) is
provisioned by the hosting Kubernetes harness, not by this pack.
