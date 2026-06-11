# domain-handoff event schemas

**Canonical source of truth** for the domain-execution handoff event contract:
how an external deterministic domain runtime (for example an AWS EventBridge
pipeline) requests agentic work in a hosted execution city, how the city
requests allowlisted deterministic domain actions back, and how completion
flows in both directions. Downstream copies (for example AWS ingress/egress
validators) are mirrors and must track these files; a parity test should fail
when they drift.

Contract version: **1.1**

## Events

Two event classes. *Envelope* events are city-addressed and wrapped in the
`execution-city-operations` common envelope; *flat* events are
domain-runtime-addressed and publish the payload as the raw EventBridge detail
with the domain event source (`GASCITY_DOMAIN_EVENT_SOURCE`, default
`GasCity`). The publisher derives the class from the schema (envelope schemas
require `process_slug`).

- `events/gascity-work-requested.v1.schema.json` — `GasCityWorkRequested.v1`
  (envelope, domain -> city): the ingress adapter creates one durable bead
  routed to `payload.route` (normally `<binding>.work-dispatcher`); the
  `handoff-work-dispatch` order cooks `payload.metadata.formula_name` as a
  molecule attached to that bead.
- `events/gascity-execution-terminal.v1.schema.json` —
  `GasCityExecutionTerminal.v1` (flat, city -> domain): completion
  notification, published only by the `handoff-terminal-sweep` order.
- `events/gascity-domain-command-requested.v1.schema.json` —
  `GasCityDomainCommandRequested.v1` (flat, city -> domain): request for an
  allowlisted deterministic domain action, published only by the
  `domain-command-publish` order from an open waiter bead.
- `events/gascity-domain-command-terminal.v1.schema.json` —
  `GasCityDomainCommandTerminal.v1` (payload of an envelope event the DOMAIN
  bridge publishes back to the requesting city; consumed by the
  `domain-command-reconcile` order).

## Examples

`events/examples/*.example.json` are valid reference payloads, one per event
type, used by schema-validation tests.

## Emission

Agents and order scripts do not hand-roll publishes. The deterministic emitter
is the `execution-city-operations` pack's
`assets/scripts/publish-cross-city-event.sh`; this pack's order scripts call
it with `--schema-file` pointing at the schemas above. Infra (the event bus,
egress IAM role, and `GASCITY_EVENT_BUS` / `AWS_REGION` env) is provisioned by
the hosting harness, not by this pack.

The human-readable contract — molecule mode, runtime fan-out, domain command
waiters, terminal stamping — lives in
`template-fragments/handoff-execution-contract.template.md` and is kept in
sync with these schemas.
