{{ define "landing-arbiter-adr-emission" }}
---

## SPEC-UPDATE REPAIR BEADS — WHEN YOUR DECISION ESTABLISHES OR REFINES A PATTERN

If your decision in `landing-arbiter-prior-art-search` Step C selected a shape from multiple invariant-consistent candidates, or added an exception, or established a structural shape not yet recorded in the spec set, you have created precedent. The next arbiter facing the same conflict shape must be able to find your decision in the spec set and reach the same answer without re-deriving it.

Emit a second repair bead — a `spec_update` bead — alongside the code repair bead(s). Both are children of the convoy; both must close before refinery re-attempts the landing.

### When to emit a spec-update bead

Emit when ANY of:

1. `prior-art-search` Step C had more than one invariant-consistent candidate and you chose between them by minimizing new precedent.
2. Step C had zero invariant-consistent candidates and you added an exception (rare; document why) rather than escalating to human.
3. Your decision involved renaming, splitting, extracting, or moving a module — even if the candidate set was a single shape — and no existing ADR or invariant section describes the resulting pattern.
4. The conflict revealed an existing invariant that is too loose to bite (e.g., a categorical list that was over-readable as a file-count rule). The spec update tightens the rule.

Do NOT emit when:

- The decision was `continue_rebase` or `merge_commit`. Those involve no architectural shape changes.
- Your chosen shape already has a binding ADR or specific rule citation in `classification_reasoning`. Replication of an existing pattern needs no new precedent.
- The repair is purely mechanical reconciliation (take-both imports, parameter forwarding, single-line merges) with no structural-shape choice.

### Bead shape

| field | value |
|---|---|
| `--type=task` | (same as code repair beads) |
| `--parent=$source_convoy` | so child-complete tracking works |
| `--title` | `Spec update: <one-sentence statement of the precedent being recorded>` |
| `metadata.gc.kind` | `owned_convoy_architectural_doc_update` |
| `metadata.gc.routed_to` | `$GC_RIG/gastown.polecat` |
| `metadata.target_branch` | `$source_branch` (same as code repair beads — lands on the integration branch, not main) |
| `metadata.source_convoy` | `$source_convoy` |
| `metadata.landing_failure_id` | `$BUG` |
| `metadata.doc_only` | `true` |
| `metadata.work_order_path` | same as on the bug |
| labels | inherit from convoy |

### Bead body

The body MUST specify:

1. **The decision being recorded** — exactly what shape was chosen, in one paragraph.
2. **The candidate shapes that were rejected** — for each, one sentence citing the corresponding rejection line in your `classification_reasoning`.
3. **Where the update lands.** Default targets, in preference order:
   1. A new rule or section in an existing `specs/*.md` invariant doc (most often `specs/14-monorepo-structure.md`).
   2. A new ADR under `specs/adr/<NNN>-<slug>.md`, numbered after the highest existing ADR. ADRs are correct for decisions that *change* the resolution of a prior closed question.
   3. An update to an existing ADR's "Consequences" section, when the decision refines (not reverses) a prior ADR.
4. **The exact prose to add.** Write the diff. The polecat copy-pastes it into the file under its own validation; it does not invent the wording from your bead body.
5. **The corresponding checklist update** in `specs/architecture-compliance-checklist.md`, if the new rule has a checkable surface.

### Sequencing

- Emit the code repair bead(s) first.
- Emit the spec-update bead second.
- Set `metadata.repair_beads` on the convoy to a comma-separated list including BOTH beads (code + spec).
- The handoff scan's child-complete check covers both; refinery re-attempts the landing only after both close.

### Anti-patterns

- Emitting a spec-update bead for every decision regardless of trigger. Spec docs are not meant to grow without bound; the "more than one candidate" / "structural shape" triggers are the discriminator.
- Specifying "update the docs" without naming the file or writing the prose. The polecat is not an architect — it implements your written wording.
- Targeting `main` instead of `$source_branch`. Spec updates land on the integration branch alongside the code; refinery merges them together.
- Putting the rationale only in the bead body. The rationale belongs in the spec file or ADR — that is the durable surface. The bead is ephemeral.

{{ end }}
