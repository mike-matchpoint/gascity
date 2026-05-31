{{ define "debugger-evidence-supplementation" }}
---

## EVIDENCE — DISCIPLINE

Producers populate the bug's metadata at filing time. Your
`validate-evidence` step gates whether the bug is ready for direct
emission or needs an investigation task first.

### When to act on the bug as-filed

ALL of the following must hold:

- `metadata.bug.class` is set and matches one of the known classes
  (`test_failure`, `runtime_failure`, `regression`,
  `integration_failure`, `contract_violation`,
  `pre-existing-test-failure`).
- `metadata.component` is a real path in the repo.
- `metadata.observed` and `metadata.expected` are non-empty.
- `metadata.evidence_complete=true` OR you can supplement the gaps
  from a focused read (see "Safe supplementation" below).
- `reproduce-or-confirm` returned `repro_status=reproduces` OR
  `repro_status=cannot_run` for a credible reason (e.g., no
  `repro_command` recorded but the failure is observable in source
  inspection — like a static contract violation).

### Safe supplementation

You MAY enrich evidence when:

- `failure_artifact` is a readable path AND the path is inside the
  worktree AND the file is text < 1 MB. Read it, summarize relevant
  excerpts into the bug's description, set the relevant metadata
  fields, set `evidence_supplemented=true`.
- `stack_trace_path` is a readable path under the same constraints.
- The failure can be observed by static inspection (grep for the
  symbol, read the file, confirm the contract violation). Record
  what was inspected in metadata.

You MUST NOT:

- Re-run the producer's environment to reproduce a failure that
  cannot be reproduced in your worktree (network calls to
  production, credentials, large data fixtures).
- Invent missing fields. If `expected` is empty and the spec
  doesn't disambiguate, route to investigation.

### When to route to investigation

ANY of the following triggers `investigation_needed`:

- `evidence_complete=false` AND `evidence_gaps` lists fields you
  cannot fill from safe supplementation.
- `repro_status=not_reproducing` AND the bug is < 24 hours old
  (transient or environment-dependent — investigation should
  characterize).
- Reproduction reveals a DIFFERENT failure mode than recorded
  (`observed` mismatches the actual failure).
- The bug carries `investigation_findings` from a prior cycle but
  the findings are themselves inconclusive.

Investigation tasks are EVIDENCE tasks, not FIX tasks. Their
`done_when` is "answers recorded in metadata.findings; no production
code changed."
{{ end }}
