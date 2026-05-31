{{ define "debugger-architectural-context" }}
---

## ARCHITECTURAL CONTEXT — DISCIPLINE

Your `gather-context` step reads the source and intent. The producer
captured WHAT failed; you must capture WHY the code exists, so the
fix shape aligns with documented architecture instead of patching
symptoms.

### Default reading list (extend by judgment)

For every plan-mode invocation:

1. **Source files named by `$COMPONENT` and `failure_artifact`.**
   Read enough to understand the failing surface. Skim, don't
   deep-read; the synthesis lives in `description`, not file dumps.

2. **AGENTS.md files**, walking up from `$COMPONENT` to the repo
   root:
   ```bash
   d="$COMPONENT"
   while [ "$d" != "." ] && [ "$d" != "/" ]; do
     [ -f "$d/AGENTS.md" ] && AGENTS_MD_FOUND+=("$d/AGENTS.md")
     d=$(dirname "$d")
   done
   [ -f "./AGENTS.md" ] && AGENTS_MD_FOUND+=("./AGENTS.md")
   ```
   Cite the specific lines that constrain the fix shape.

3. **Specs** referencing the affected component:
   ```bash
   grep -rlE "$(echo "$COMPONENT" | sed 's|/|.\*|g')" specs/ 2>/dev/null
   ```
   Read every match. If `specs/` is empty for this component, that
   is itself evidence — a `spec_gap` resolution class.

4. **ADRs** that touch the same surface:
   ```bash
   for d in docs/adr docs/decisions; do
     [ -d "$d" ] || continue
     grep -lE "$(echo "$COMPONENT" | sed 's|/|.\*|g')" "$d"/*.md 2>/dev/null
   done
   ```

5. **Recent commit messages** on the affected paths:
   ```bash
   git log --oneline -10 -- "$COMPONENT"
   ```
   Regressions often cite the introducing commit in their failure
   mode. Confirm by reading the commit.

### Synthesis output

Write the synthesis into the bug's `description` (replace, don't
append-noise):

```
## Architectural intent synthesis

<one paragraph: what the component is supposed to do per
specs/AGENTS.md/ADRs, what the observed behavior diverges from,
what the root cause is in architectural terms (not symptom-level)>

## Root cause

<one paragraph naming the specific cause, citing file:line or symbol>

## Sources read

- Specs: <paths or "none found">
- AGENTS.md: <paths or "none found">
- ADRs: <paths or "none">
- Source: <paths>
- Recent commits inspected: <SHAs or "none">
```

Stamp structured pointers on metadata:

- `spec_paths`
- `agents_md_paths`
- `adr_paths`
- `source_paths`
- `related_commits`

### Bound payload size

Summarize, do not copy file contents. The bug's metadata + description
is read end-to-end by verify-mode and by any operator auditing. Long
file dumps waste their context budgets.
{{ end }}
