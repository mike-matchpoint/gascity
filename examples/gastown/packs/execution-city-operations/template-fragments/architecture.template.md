{{ define "execution-city-architecture" }}
## Execution City Architecture

City root: `{{ .CityRoot }}`.

- The controller manages lifecycle, session spawning, routing, and liveness.
- Deterministic services own bridge command dispatch, terminal reconciliation,
  hard gate evaluation, retries, DLQs, and domain state mutation.
- Agents own nondeterministic judgment: evidence sufficiency, anomaly triage,
  schema interpretation, escalation, and cross-city handoff writing.
- Beads and artifacts are the durable audit surface. Conversation context is
  never source of truth.
- Cross-city handoffs must be explicit, typed, idempotent, and evidence-backed.
{{ end }}
