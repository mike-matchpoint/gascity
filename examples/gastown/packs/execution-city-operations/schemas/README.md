# execution-city-operations event schemas

**Canonical source of truth** for the cross-city handoff event contract used by
all execution-and-monitoring cities. This pack is installed from gascity source,
so the agents that emit events reach these files at runtime. Any other copy
(e.g. an AWS ingress validator in a Kubernetes harness repo) is a downstream
mirror and must track these files; a parity test should fail if they drift.

Contract version: **1.1**

## Events

Two event classes share the publisher. *Envelope* events are city-to-city and
are wrapped in the common envelope by the publisher; *flat* events are
city-to-domain-runtime and publish the payload as the raw EventBridge detail
with the domain event source (`GASCITY_DOMAIN_EVENT_SOURCE`, default
`GasCity`). The class is derived from the schema (envelope schemas require
`process_slug`), not hardcoded.

This pack registers the repo handoff events. Other packs register their own
event types by shipping a schema file and calling the publisher with
`--schema-file` (see the `domain-handoff` pack for the domain-execution
handoff family: `GasCityWorkRequested.v1`, `GasCityExecutionTerminal.v1`,
`GasCityDomainCommandRequested.v1`, `GasCityDomainCommandTerminal.v1`).

- `events/common-envelope.v1.schema.json` — shared cross-city envelope.
- `events/repo-bug-reported.v1.schema.json` — `RepoBugReported.v1` (envelope):
  a concrete code defect reported upstream to a code-generation city.
- `events/repo-change-requested.v1.schema.json` — `RepoChangeRequested.v1`
  (envelope): a missing capability / change request to a code-generation city.

The repo handoff schemas intentionally do not expose `route`, `gc_route`, or
`gc.routed_to` payload fields. Code-generation-city routing is derived by the
receiving adapter: `RepoChangeRequested.v1` becomes
`gc.routed_to=codegen-support.cartographer`, and `RepoBugReported.v1` becomes
`gc.routed_to=codegen-support.debugger`.

Repo handoffs are also resolved through a runtime ownership index. Publishers
set `payload.repo`; the hosting harness injects `GASCITY_CODEGEN_OWNERSHIP_JSON`
with repo -> owning code-generation-city entries and city purpose text. If a
repo is not indexed, the publisher must fail and the city pair must add a
durable ownership entry before emitting the event.

The human-readable field contract lives in
`template-fragments/codegen-handoff-contract.template.md` and is kept in sync
with these schemas. The domain-execution handoff contract lives in the
`domain-handoff` pack's `template-fragments/handoff-execution-contract.template.md`.

## Examples

`events/examples/*.example.json` are valid reference payloads, one per event
type, used by schema-validation tests and by `assets/scripts/publish-cross-city-event.sh --dry-run`.

## Emission

Agents do not hand-roll publishes. The deterministic emitter is
`assets/scripts/publish-cross-city-event.sh`, which builds the envelope,
validates against these schemas, and performs the event-bus put. Infra (the
event bus, egress IAM role, and `GASCITY_EVENT_BUS` / `AWS_REGION` env) is
provisioned by the hosting Kubernetes harness, not by this pack.
