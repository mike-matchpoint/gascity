{{ define "polecat-done-target-override" }}
---

## DONE-SEQUENCE TARGET FIELD — READ FROM METADATA, NOT HARDCODED

**This note further amends the done sequence.** The cartographer formula
emits integration-branch convoys with `metadata.target=integration/<wo-slug>`
on every work bead. The polecat's `bd update --set-metadata target=...`
call MUST read that value, not hardcode `main` or `{{ .DefaultBranch }}`.

The Issue 9 handoff override fragment (polecat-handoff-override) already
does this in the atomic-JSONL path. But the upstream prompt's "FINAL
REMINDER: RUN THE DONE SEQUENCE" section earlier in this prompt still
shows the line:

```
  --set-metadata target={{ "{{ .DefaultBranch }}" }} \
```

That line is WRONG for cartographer-emitted work beads — it clobbers the
cartographer's integration-branch target back to main and breaks the
WO-level atomic merge. Treat that earlier example as illustrative ONLY;
the canonical done sequence is the one in
"DONE SEQUENCE — USE THIS, NOT THE ONE ABOVE" (the polecat-handoff-override
fragment). If for any reason you fall through to running the upstream
"FINAL REMINDER" commands directly, replace the `target=...` argument
with:

```bash
TARGET=$(gc --rig "$GC_RIG" bd show "$BEAD_ID" --json 2>/dev/null \
  | jq -r '.[0].metadata.target // "main"')
gc bd update "$BEAD_ID" \
  --set-metadata branch="$(git branch --show-current)" \
  --set-metadata target="$TARGET" \
  --notes "Implemented: <brief summary>"
```

**Rule of thumb:** target is a property of the BEAD, not a property of
the rig or the prompt. Read it; do not assume it. See
runbooks/convoy-close-lifecycle-gap.md for the full integration-branch
rationale.
{{ end }}
