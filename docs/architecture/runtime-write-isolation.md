# Runtime Write Isolation

Status: active for the runtime write isolation cutover started 2026-05-30.
Order-firing stabilization validated on 2026-06-01 with a 60 minute live soak
after a 10 minute reload warmup.

GasCity runtime loops must keep controller progress bounded even when the
Beads CLI or Dolt is slow. Beads remains the durable source of truth for work,
session state, and order audit rows; this contract adds a bounded write-side
adapter around existing Beads CLI commands for hot runtime callers.

## Write Classes

| Class | Budget | Route | Fallback | Failure behavior |
| --- | ---: | --- | --- | --- |
| `hot-state` | 10s caller | runtime writer | none | report degraded state, keep controller loop alive |
| `reservation` | 10s caller | runtime writer with deterministic ID/idempotency | none | defer unsafe action on ambiguous or failed reservation |
| `cursor-reservation` | 10s caller | runtime writer with deterministic cursor key | none | defer event action before execution |
| `post-action-critical` | 30s caller | runtime writer and compensator | none | record ambiguous/failed write for bounded repair |
| `audit-repair` | 10s caller | runtime writer queue | none | pause or retry through compensation |
| `foreground-authoritative` | command timeout | existing `Store` methods and Beads CLI | allowed | return command error after operator budget |
| `maintenance` | explicit deadline | serialized maintenance/operator path | allowed outside hot ticks | retry/report through maintenance state |

`internal/beads.RuntimeCreate`, `RuntimeUpdate`, `RuntimeCloseAll`, and
`RuntimePing` are the canonical entry points for hot writes. `BdStore.Create`,
`Update`, `CloseAll`, `Ping`, `Get`, and `List` remain
foreground-authoritative APIs.

## Durable Authority

Runtime write isolation does not introduce a new durable data store and does
not write Beads or Dolt tables directly. The runtime writer still invokes
`bd`, but every hot invocation is run through a caller-owned context,
process-group cleanup, a short wait delay, an explicit write class, and an
idempotency key where duplicate outcomes are possible.

Local order runtime leases are ephemeral single-flight and compensation state
only. They never become authoritative work or session state. A
tracking-required order action may start only after the local lease and a
durable Beads tracking reservation are created with a deterministic ID inside
the reservation budget. If that reservation fails or times out ambiguously, the
order is deferred before execution.

Only orders marked in source as both idempotent and tracking-degraded-allowed
may run while tracking persistence is degraded. The default for existing orders
is tracking-required.

## Runtime Writer Manager

Each canonical store key owns one runtime write manager. The manager provides:

- one active Beads writer per store key.
- a bounded queue of 128 runtime writes.
- idempotency-key coalescing for duplicate queued or active writes.
- explicit `DegradedWriteError` outcomes: `not-started`,
  `ambiguous-timeout`, `failed`, `partial`, and `unsupported`.
- per-store stats for queue depth, drops, collapses, timeouts, failures,
  completions, oldest backlog age, and breaker state.
- per-class stats for queue depth, active count, queue limit, oldest backlog
  age, and breaker state. Runtime callers that only depend on one write class
  must use class-scoped stats so unrelated maintenance, post-action, or
  hot-state pressure does not block safer reservation writes.
- per-class circuit breakers that open after repeated runtime write timeouts or
  failures for that class. A hot-state breaker must not reject reservation
  writes, and a maintenance/audit breaker must not make order dispatch look
  wedged when the reservation class is healthy.
- a bounded queue-wait window before a runtime write is reported as
  `not-started`, plus a pre-start guard that skips queued jobs whose remaining
  caller budget is too small to safely launch `bd`.

Runtime write methods deliberately unwrap `CachingStore` before writing. Hot
writes must not invoke `CachingStore` post-write `Get` readbacks or silently
fall back to foreground Beads CLI semantics after a runtime timeout.

## Observability

Runtime write errors must carry enough information for order dispatch, session
reconciliation, status, and doctor to explain the degraded path: caller, class,
operation, idempotency key, store key, outcome, and wrapped error. When
`GC_BD_TRACE` is set, runtime writes append a diagnostic line containing the
caller, class, operation, command subcommand, duration, timeout, outcome, and
store key without logging raw metadata payloads.

Doctor and status surfaces should prefer these runtime writer stats and
degradation records over foreground Beads probes so diagnostics do not wedge
behind the same Beads/Dolt stall they are reporting.

Recovered transient Beads/Dolt write conflicts must be recorded as a single
logical successful runtime write with retry metadata, not as an intermediate
degraded result followed by success. This preserves operator visibility into
retry pressure without creating false doctor/status degradation when the write
eventually lands.

## Order Dispatch Contract

The order dispatcher may throttle tracking reservations per tick to keep reload
bursts and steady-state Dolt write pressure bounded. Dispatch candidates must
continue rotating across ticks so front-of-list orders cannot starve under
partial writer pressure or per-tick caps.

Order dispatch backpressure is scoped to the reservation writer class only. A
dispatch tick may defer tracking-required orders when the reservation class
breaker is open or its class queue is full. It must not defer orders because an
unrelated write class is active, queued, or degraded.

Order tracking post-action writes are allowed to outlive the action execution
context. Once an order action has started, recording the execution result and
closing the tracking bead are completion/audit work and should not be canceled
just because the action context expired.

Order tracking reservation writes are allowed to outlive the dispatch tick
deadline, but not dispatcher cancellation. A long dispatch tick can spend most
of its deadline closing or labeling earlier order work; the next reservation
must still enter the runtime writer with the reservation class budget so the
write path increases throughput instead of suppressing later orders. The
reservation context therefore strips only the parent deadline and preserves the
parent `Done`/`Err` cancellation signal.

Local order leases remain subordinate to durable tracking rows. If a local
lease from a prior controller start suppresses dispatch, and the referenced
tracking bead is already durably closed with order-tracking labels, the
dispatcher may release that stale local lease and retry reservation. A lease
from the current controller start still suppresses dispatch so an active order
cannot be duplicated.

Live soak acceptance for this cutover is:

- Warmup: allow the existing reload gate and `order-firing-current` warning for
  the first 10 minutes after restart.
- Steady state: for the remaining soak, `gc doctor` must return no
  `order-firing-current` overdue flags, order dispatch wedge counters must stay
  at zero, and order dispatch must continue creating tracking rows.
- Failure: any post-warmup overdue order flag, dispatch wedge event, dispatch
  wedge tick, or reservation-class backpressure deferral that turns into stale
  orders blocks cutover.

## Regression Rules

- Hot runtime write callers pass a `beads.WritePolicy` at the call site.
- Hot runtime write paths never call foreground `Store` write methods after a
  runtime timeout.
- Runtime writes never go through `CachingStore.Create`, `Update`, `Close`, or
  `CloseAll` readback paths.
- Deterministic creates treat a duplicate ID with matching reservation
  metadata or idempotency metadata as success, and a duplicate with mismatched
  metadata as a conflict.
- New controller/order/session hot writes must be added to this contract and
  covered by runner-spy, timeout, process-cleanup, or static guard tests.
