{{ define "debugger-decision-criteria" }}
---

## DECISION CRITERIA — AUTHORITATIVE FRAMEWORK

This fragment is the LAST authority on which decision to pick. The
five-bucket framework in the main prompt is your starting point; if
that framework and any earlier guidance conflict, follow this
fragment.

### Decision order

Try the buckets in this order. Stop at the first one whose criteria
all hold:

1. `specialist_routed`
2. `investigation_needed`
3. `human_required`
4. `scope_expanded_to_feature`
5. `convoy`
6. `direct_bugfix`

The order biases toward routing the bug correctly (1), then toward
not deciding without sufficient evidence (2-3), then toward
acknowledging when the bug isn't really a bug (4), then toward the
smallest correct fix shape (5-6).

### Criteria summary

| Bucket | All must hold |
|---|---|
| `specialist_routed` | `gc.kind` ≠ `bug`, OR `bug.class` names a known specialist's domain (e.g., owned-convoy landing). |
| `investigation_needed` | Root cause unclear from evidence + context; OR reproduction revealed a different failure mode; OR `evidence_complete=false` and not safely supplementable. |
| `human_required` | Prior-art search returned zero invariant-consistent candidate shapes; AND the gap cannot be closed by adding precedent in this PR; AND no specialist owns it. |
| `scope_expanded_to_feature` | Correct fix introduces new externally-visible API; OR contradicts a documented decision; OR requires product judgment beyond the spec invariants. |
| `convoy` | Fix requires multiple INDEPENDENT or ORDERED concerns; AND each concern is reviewable on its own merits; AND the work cannot collapse into one task with multiple `files_to_touch`. |
| `direct_bugfix` | Default after the above are ruled out. Root cause clear; fix mechanical/self-contained; one concern; acceptance verifiable by named tests/lint. |

### Anti-pattern guards

- **Don't pick `convoy` because the bug touches multiple files.**
  Categorical lists name concerns, not files. Touching `module_a.py`
  AND `module_b.py` for the same contract restoration is ONE
  concern (one task).
- **Don't pick `human_required` because the architectural decision
  feels weighty.** If at least one fix shape is consistent with the
  spec invariants and no other bucket fits, you HAVE the authority
  to decide. The presence of multiple defensible candidates is a
  spec-gap signal — close it via `landing-arbiter-adr-emission`,
  not via escalation.
- **Don't pick `investigation_needed` to defer a decision you could
  make now.** Investigation costs a full polecat cycle plus a
  re-plan. If you have enough evidence to act, act.
- **Don't pick `direct_bugfix` for a refactor.** If the fix requires
  changing structure (extracting a class, renaming a module,
  reorganizing imports) AND then fixing the bug, that is `convoy`
  with two tasks.
- **Don't pick `scope_expanded_to_feature` to avoid a hard
  architectural call.** This bucket is for bugs that are actually
  product work, not for bugs whose fix shape is large.
{{ end }}
