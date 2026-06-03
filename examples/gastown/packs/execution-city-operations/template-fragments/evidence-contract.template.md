{{ define "execution-evidence-contract" }}
## Execution Evidence Contract

Every incident, handoff, and closure must cite concrete evidence. Prefer
explicit artifact URIs and IDs from the bead over broad filesystem searches.

Minimum evidence packet:

- city name, environment, and run or execution ID
- triggering bead, molecule, event, terminal event, or DLQ record
- observed behavior and expected behavior
- timestamps and retry/replay history when available
- affected deterministic service, adapter, prompt, artifact, or runtime surface
- exact artifact URIs, log snippets, validation output, or event IDs
- reproduction or replay command when a deterministic command exists
- blockers, missing evidence, and confidence level

Do not include secrets, tokens, credentials, or raw private payloads unless the
work explicitly requests a redacted evidence bundle and names the approved
storage surface.
{{ end }}
