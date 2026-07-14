# Work Order: GCD-WO-CSC-005 — polecat diligence fragments + submit-to-evaluator done-override + refinery merge-only gating (`evaluator_gated`)

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-005-polecat-diligence-refinery-merge-only.md` in your
worktree before implementing; tail amendments are BINDING. The six fragment NAMES pinned
here are discovery targets for `GCD-WO-CSC-006` (its R1c gate set-differences them against
the six pre-existing `polecat-*` names) — they are IDENTITY, never rename.

Execution classification: Dev-only (pack content — six new codegen-support template
fragments, `evaluator_gated` branches in the gastown `mol-refinery-patrol` formula, one
edit to the `refinery-wisp-pour-vars-override` fragment — plus repo-native packlint tests;
no AWS, no deploy surface, no city runs). `boundary=dev`, **wave 23** (CSC program band
23/24/25), `blocked_by` `GasCity-Dev::GCD-WO-CSC-003-evaluator-judge-primitives`
(**same wave — apply_deps DIRECT-WRITE edge `GCD-005←GCD-003` per kit A1 §4**; this WO
imports C9 and MUST be generated against 003's MERGED content).
Consumed by: `GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot` (wave 24 — appends the
six fragment names + sets `evaluator_gated="true"` per rig),
`GasCity-Dev::GCD-WO-CSC-007-city-pack-binding-fanout` (wave 25).

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **C9** (verdict metadata +
> routing — authority `GCD-WO-CSC-003`, IMPORTED here, never re-declared), kit scope line
> for this WO ("5 polecat fragments … + `evaluator_gated` var branches in gastown
> `mol-refinery-patrol.toml` (default "false"; gated: run-tests→build-smoke,
> handle-failures→rebase-conflicts+merge-mechanics; FULL battery KEPT for
> `integration/*`→main convoy autolands)"), **ADDENDUM A1 §2** (the Overseer-Issue
> PR/commit marker fragment: "polecat-side fragment duty in GCD-WO-CSC-005"; marker line
> `Overseer-Issue: <issue-id>`; bead metadata key `overseer_issue_id`), K2, K6.
> Backlog + sequencing: `master/city-scaling-improvements/gap-analysis-and-build-plan.md`
> §5 row 5, §6. Design record:
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 (owner ruling
> **D10**: polecat diligence fragments on Codex 5.5 unchanged; refinery reduced to
> serialized merges via `evaluator_gated`, convoy-autoland full battery kept;
> resume-and-fix retries; `regenerate_on_reject` reserved). Diligence source:
> `matchpoint-loop-harness/mlh/prompts/implementer.md` (estate code root — generic blocks
> only). Process: root `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Marker-grammar authority (import citation, A1 §2): `Matchpoint-Platform/specs/agent-work-orders/PAR-WO-CSC-001-error-emission-package-and-envelope.md`
> **Step 10** pins the `Overseer-Issue` marker line grammar — regex
> `^Overseer-Issue: (?P<issue_id>[A-Za-z0-9_.:-]+)$`, key case-sensitive; the reader
> (the AGC-WO-CSC-001 webhook transformer) searches the PR body FIRST, then the
> head-commit message, first match wins, filling `payload.overseer_issue_id` (`null`
> when absent). Note (kit A3.2): the in-program reader implements the PR-body arm ONLY
> (the head-commit arm needs a GitHub API surface — post-program); the commit trailer
> written here future-proofs that head-commit lane.
> This WO cites that grammar; it never redefines it.
> Verified at authoring (2026-07-08): `GasCity-Dev` `origin/main` @
> `a47df8f5adbc7b8e4243ae344360c2dbbf2c864f` (read-only `git log -1 --format=%H
> origin/main`; the commits past `c85d92cf` are CSC spec-file-only — every pack-content
> file/line reference in this WO is byte-identical at both SHAs). Re-verify at
> execution time — GCD-WO-CSC-003 merges FIRST (same wave, serialized); bind against
> ITS merged literals.
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-005-polecat-diligence-refinery-merge-only`.

## Goal

Code-generation cities can run the WS2 evaluated pipeline end-to-end: the polecat
produces harness-grade diligent work and submits it to the **evaluator pool** instead of
the refinery, and the refinery — for beads that arrive with an evidence-backed
`judge_verdict=PASS` — degrades its serialized quality battery to a build smoke, becoming
a merge fan-in. All upstream, all generic, all inert until a city opts in. Clean end
state:

1. **Six NEW codegen-support template fragments** (`examples/gastown/packs/codegen-support/template-fragments/`),
   names pinned (IDENTITY — GCD-WO-CSC-006 discovers them):
   - `polecat-code-hygiene` — SOLID/strengthen-the-architecture, no-band-aids rule,
     **fabricated-evidence ban with the enumerated taxonomy**, pushback-is-correct,
     additive-repair-with-ADR exception.
   - `polecat-evidence-contract` — real commands / real captured output; produce the
     declared evidence the evaluator will re-run; fast-iteration output never counts as
     final evidence; commit early and often; NEVER stash.
   - `polecat-final-rebase-revalidate` — refresh onto the current target BEFORE assessing
     prerequisites; final fetch+rebase AND re-run acceptance on the rebased tree before
     submitting.
   - `polecat-autonomy-and-blockers` — resolvable decisions are the polecat's; structured
     escalation as a DECISION PACKAGE (question id, question, complete option set,
     recommendation, blast radius) via mayor mail as the rare last resort; the FIRST
     blocker STOP enumerates the COMPLETE known incompatibility set (batched, never
     one-per-cycle); the mayor ACKs receipt in-band and the polecat never resubmits an
     un-answered question; never idle-wait.
   - `polecat-submit-to-evaluator` — the done-sequence override: routes completed work to
     `gc.kind=eval_request` (C9), **explicitly superseding** the earlier refinery-handoff
     done sequences; clears ALL stale verdict keys incl. `verdict_patch_id`; writes the
     **structured residue rows** (C9 `residue`: delivered / not-delivered / known-gap,
     gaps mapped to existing beads/WOs — silent residue is an evaluator REJECT); carries
     the resume-and-fix rule for evaluator/judge rejections.
   - `polecat-overseer-issue-marker` — A1 §2: when the bead carries
     `metadata.overseer_issue_id`, the final commit message AND the submission notes
     carry the marker line `Overseer-Issue: <issue-id>` (the refinery PR body includes
     bead notes, so the marker reaches PRs automatically).
2. **`evaluator_gated` branches in `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml`**:
   new formula var `evaluator_gated` (default `"false"` — every existing city
   byte-behaviorally unchanged). When the EFFECTIVE gate is `"true"` AND the work bead
   carries an approved verdict (`judge_verdict=PASS`, or `NOT_REQUIRED` — the C9
   conditional-judge no-risk-marker handoff) AND **the verdict's `verdict_patch_id`
   equals the patch-id of the branch content the refinery is about to merge (the
   content-state guard, blueprint LAW-4 — a verdict proves exactly the content it was
   computed on; mismatch = full battery, never degrade on a stale verdict)** AND the
   merge source is not an `integration/*` landing:
   `run-tests` degrades to setup + build smoke; `handle-failures` handles only
   smoke-failure rejection + merge mechanics (clearing stale verdicts on rejection and
   PRESERVING the branch — resume-and-fix, never a gated branch delete).
   **FULL battery KEPT for `integration/*` → main convoy autolands** and for any bead
   without a fresh judge PASS (fail-safe: more testing, never less).
3. **`refinery-wisp-pour-vars-override` fragment updated** (its own text instructs:
   "If additional rig-level overrides are added … append matching `--var key=value`
   lines here"): the canonical pour gains the `evaluator_gated` var, resolved via the
   effective-var lookup, and the verification jq gains the key.
4. **Packlint tests** (`test/packlint/`, `spec_cartographer_formula_test.go` pattern)
   pinning every load-bearing literal above, plus the generic-ness grep gate.

Business reason: WS2/D10 — the harness proved that generator diligence + per-task
adversarial evaluation catches defects before merge, letting the serialized merge point
contain minutes of merging instead of hours of testing. The polecat fragments port the
harness implementer's discipline; the `evaluator_gated` flow moves the battery from the
serial refinery to the parallel evaluators (wave-23 upstream), with the convoy-autoland
full battery as the integration backstop (WS2 risk K4 mitigation, kept deliberately).

## Dependencies

- **Blocked by `GasCity-Dev::GCD-WO-CSC-003-evaluator-judge-primitives` (same wave,
  merged FIRST — direct-write serialization).** This WO IMPORTS from its merged content
  (discovery-first, REJECT-level): the C9 verdict-metadata keys and values, the
  `gc.kind=eval_request` selector value, the R3 routing state machine (quoted below as
  R1 — verify field-for-field against the merged
  `examples/gastown/packs/codegen-support/README.md` + `formulas/mol-evaluate-task.formula.toml`),
  and the `effective_rig_var` lookup block (copy VERBATIM from the merged
  `mol-evaluate-task.formula.toml` — one canonical spelling estate-wide). On any
  mismatch, the MERGED 003 content wins and this WO's quoted expectations are corrected
  in the PR description.
- **Consumed by `GCD-WO-CSC-006` (wave 24):** its Step 3 appends the six fragment names
  after the six existing ones and its "done-routing supersession gate" READS the merged
  `polecat-submit-to-evaluator` fragment, STOPPING unless it contains explicit
  supersession language over the earlier refinery-handoff done sequence — the exact
  language is pinned in Implementation Step 5. Its Step 4 sets
  `evaluator_gated = "true"` in `[rigs.formula_vars]` and expects THAT ALONE to flip the
  refinery branches — the effective-var lookup (R4) is what makes that true.
- **Producer↔consumer handshake:** this WO's submit sequence PRODUCES `eval_request`
  beads; 003's evaluator consumes them; 003's judge PRODUCES the refinery handoff this
  WO's gated refinery consumes. The shared field table is R1 — both WOs pin it.
- **Cities PAUSED (standing policy + kit K1):** authoring + repo-native structural tests
  ONLY (`make check`, packlint); no city started, no daemon, no supervisor, no
  kubectl/AWS call, no live merge/eval run. Live gated-flow behavior on the
  vehicle-graph pilot is GCD-WO-CSC-006's named un-pause follow-up — never an acceptance
  criterion here.
- **Fixture-realism doctrine (owner-ratified, REJECT-level):**
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` binds the test
  discipline (assertions FAIL when the asserted content is absent; zero-item never
  green). Its doctrine TEXT stays out of this pack (city-fragment content, D10); the
  generic evidence-contract PATTERN is exactly what `polecat-evidence-contract` carries.
- Repo gates: `CONTRIBUTING.md`, `TESTING.md`, AGENTS.md (ZFC; zero hardcoded roles in
  Go).

## Non-Goals

Bounded-context REJECT rules (kit K2, GasCity-Dev row) restated:

- **NO evaluator/judge content** — agents, formulas, verdict keys, evidence grammar,
  escalation rule are `GCD-WO-CSC-003` (merged). Re-declaring ANY C9 element here
  (instead of import-citing) is a blocker-class finding.
- **NO polecat prompt or agent.toml edits.** `examples/gastown/packs/gastown/agents/polecat/`
  is untouched — the upstream "FINAL REMINDER: RUN THE DONE SEQUENCE" section
  (`prompt.template.md:253`) STAYS; supersession happens via appended fragments in
  cities that opt in (the established override pattern: `polecat-handoff-override`
  superseded that same section the same way).
- **NO edits to the existing six polecat fragments** — `polecat-decide-and-act`,
  `polecat-handoff-override`, `polecat-done-target-override`,
  `polecat-architectural-doc-sync`, `polecat-validate-before-commit`,
  `polecat-bug-filing` remain byte-identical (Validation pins this with
  `git diff --quiet`). The new done-override supersedes by RENDER ORDER + explicit
  language, never by editing predecessors.
- **NO `mol-polecat-work` edits** — the polecat's rejection-aware resume machinery
  already exists and is KEPT (WS2 finding: "stronger than the harness's
  regenerate-from-zero; keep it").
- **NO WIP-push-cadence content (boundary vs `GCD-WO-CSC-002`, pinned).** The gastown
  `wip-push-cadence` fragment (authored by `GCD-WO-CSC-002`, same wave, default-ON in
  the gastown polecat prompt) is the SINGLE author of push discipline: push after every
  commit, `--force-with-lease` after history rewrites, pushed-commits-are-the-durable-
  copy rationale. The six fragments here MUST NOT restate any of it — they may
  REFERENCE it by fragment name where adjacent (e.g. after the final rebase, "push per
  the WIP-push cadence discipline rendered earlier in this prompt"), never duplicate
  the rules. Concretely: the strings `force-with-lease` and "after EVERY commit" must
  NOT appear in the six new fragments (packlint absence assertion, Step 9a). The
  boundary: GCD-WO-CSC-002 owns push CADENCE (durability); this WO owns COMMIT
  discipline, evidence production, and submission ROUTING.
- **NO implementation of `regenerate_on_reject`** — RESERVED name, documented by 003's
  pack README; this WO only cross-references the reservation.
- **NO new Go role logic (ZFC)** — Go edits limited to `test/packlint/*.go`.
- **NO embed.go / pack.toml changes in either pack** — gastown embeds `formulas` and
  codegen-support embeds `template-fragments` already; new/edited files land under
  existing globs. NO committed `.gc/system/packs` mirror files (mirrors are
  runtime-materialized — `cmd/gc/embed_builtin_packs.go:26,49`).
- **NO MatchPoint / business-domain literals** in any pack runtime file this WO touches
  (grep gate `matchpoint|enrichment|vehicle`; no `master/` paths, no D-numbers, no AWS
  names/mandates — the harness prompt's AWS live-retrieval mandate, prod rules,
  tenant/demo rules, and doctrine text are NOT portable and belong to city fragments,
  GCD-WO-CSC-006).
- **NO city bindings** (`[[rigs.patches]]`, `append_fragments` additions,
  `formula_vars` settings in any city repo) — GCD-WO-CSC-006/007.
- **NO router/cartographer/watch changes** (GCD-WO-CSC-004) and **NO exec-monitoring
  city or `execution-city-operations` pack changes** (D5; the sole exec-city change in
  the program is GCD-WO-CSC-008).
- **NO weakening of ungated behavior**: with `evaluator_gated` unset/false, every step
  of `mol-refinery-patrol` must behave byte-identically to today (the gated logic is
  strictly additive branches; the "FORBIDDEN: Writing code to fix failures" cardinal
  rule stays in force in BOTH modes).
- **NO refinery prompt edits** (`gastown/agents/refinery/prompt.template.md` untouched):
  its bootstrap pour cannot render formula vars (no `PromptContext` accessor) — the
  run-time effective-var lookup covers it by design (R4); adding prompt-side var
  plumbing is a finding.

## Architecture Links

- `master/city-scaling-improvements/wo-authoring-kit.md` — C9 (import), A1 §2, K2, K6
  (estate authority; load-bearing pins quoted here).
- `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 rows 3/5/14, §6.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — WS2 "Findings
  — harness diligence inventory (portable)" (the generator bullet is THIS WO's content
  source map), "Proposed design" items 3/5 (ratified D10), risks K4 (battery moves to
  parallel evaluators; convoy-autoland full battery is the accepted backstop).
- `matchpoint-loop-harness/mlh/prompts/implementer.md` (estate code root) — the
  diligence SOURCE. Portable: the CODE HYGIENE block (SOLID/strengthen, band-aid ban,
  fabricated-evidence taxonomy, pushback-is-correct, additive-repair-with-ADR), Rule 3
  (branch discipline: commit early/often, never stash, you are not the one who merges),
  Rule 8 + FINAL STEP (refresh-before-assess + final rebase-revalidate), AUTHORITY +
  Rule 7 (autonomy; structured blocker shape), the test-speed pattern (fast recipe for
  iteration, full battery once at final state, fast output never counts). NOT portable
  (strip): owner-decision sections, the 8 laws / D-numbers, master/ spine, AWS
  live-retrieval mandate, prod boundary rules, multi-repo/CO-EDIT machinery,
  fixture-realism doctrine TEXT, `just`/repo-specific recipe names.
- This repo:
  - `examples/gastown/packs/codegen-support/template-fragments/polecat-handoff-override.template.md`
    — the done-sequence being superseded (its own header "**This section supersedes the
    "FINAL REMINDER: RUN THE DONE SEQUENCE" section earlier in this prompt**" is the
    supersession-language precedent; its TARGET-from-metadata discipline is preserved
    verbatim in the new sequence).
  - `…/polecat-done-target-override.template.md` — the target-field amendment (rule of
    thumb "target is a property of the BEAD"); the new sequence carries the same
    `target_branch // target // "main"` read.
  - `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml` — the formula
    under edit (982 lines @ c85d92cf, `version = 4`, graph.v2; steps validate-identity →
    check-inbox → find-work → rebase → run-tests → handle-failures → merge-push →
    patrol-summary → next-iteration).
  - `…/codegen-support/template-fragments/refinery-wisp-pour-vars-override.template.md`
    — the canonical-pour fragment under edit; ALSO the in-repo authority for the
    pour-time gotcha ("`bd mol wisp` does not consume `[rigs.formula_vars]` at pour
    time").
  - Post-003 merged content: `…/codegen-support/agents/evaluator/`,
    `…/formulas/mol-evaluate-task.formula.toml`, `…/codegen-support/README.md` (the C9
    binding doc — import source).
  - `test/packlint/spec_cartographer_formula_test.go` + `bd_show_jq_test.go:22`
    (`repoRoot()`), `docs/guides/shareable-packs.md:234-235`,
    `cmd/gc/cmd_lint.go:314-320` (appended-fragment lint check).

## Packages To Inspect

All repo-relative; READ-first:

- `examples/gastown/packs/codegen-support/template-fragments/` — all 25 existing
  fragments: the define-wrapper format (`{{ define "<name>" }} … {{ end }}`, kebab name
  = filename minus `.template.md`), the six existing `polecat-*` names (set-difference
  target for GCD-WO-CSC-006 R1c), the supersession-heading style.
- `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml` — FULL read. Facts
  the edits hang on (verify): `[vars]` block ends with `event_timeout`; `run-tests` step
  reads five command vars + `run_tests`; `handle-failures` has the
  branch-caused-vs-pre-existing fork with the reject-to-pool recipe
  (`gc workflow delete-source $WORK --apply && gc workflow reopen-source $WORK`, then
  `gc bd update $WORK --status=open --assignee="" --set-metadata rejection_reason=…
  --set-metadata gc.routed_to="${GC_RIG:+$GC_RIG/}{{binding_prefix}}polecat"`); the
  in-formula wisp pour appears at ~6 sites (check-inbox restart, rebase-fail step 6,
  handle-failures step 2, `block_existing_pr()` in merge-push, next-iteration step 2);
  `merge-push` reads `BRANCH`/`TARGET`/`MERGE_STRATEGY`; `next-iteration` step 1 assigns
  convoy beads with `branch=<integration-branch>` for autoland.
- `examples/gastown/packs/gastown/agents/refinery/prompt.template.md` — READ-ONLY (4
  pour sites at ~lines 66/111/172/303 use template-context vars only — why the run-time
  lookup exists; NOT edited).
- `examples/gastown/packs/gastown/embed.go` (`formulas` glob present — no edit) +
  `…/codegen-support/embed.go` (`template-fragments` glob present — no edit).
- Post-003 merged files (import sources): `codegen-support/README.md` (C9 tables),
  `formulas/mol-evaluate-task.formula.toml` (the `effective_rig_var` block + the pinned
  `<RIG-FORMULA-VARS-PATH>` jq path — COPY, don't re-derive).
- `test/packlint/` — test pattern + `repoRoot()`.
- `Makefile` — `build`/`check`/`test`.

## Required Inputs

**R1 — C9 IMPORT (authority `GCD-WO-CSC-003` — verify against merged content, correct
this table in the PR if drifted).** Keys: `eval_verdict`, `eval_evidence`,
`verdict_patch_id` (content-state key — `git patch-id --stable` of the evaluated
diff), `eval_reject_count`, `judge_verdict`, `rejection_reason`, `decision_state`,
`overseer_issue_id`, `residue` (structured close-out rows). Values:
`judge_verdict` ∈ {`PASS`, `REJECT`, `NOT_REQUIRED`} — `NOT_REQUIRED` is the
conditional-judge no-risk-marker handoff (evidence-backed evaluator PASS, judge lane
skipped BY DESIGN; the refinery treats it exactly like `PASS`). Routing values:
`gc.kind=eval_request` → evaluator pool;
`judge_request` → judge; judge-PASS / evaluator `NOT_REQUIRED` → refinery handoff. The
submission transition this
WO produces (row "Submit for evaluation" of 003's R3 table): write `branch`, `target`,
notes, **`residue`** (the structured rows — this WO's submit sequence is the C9-pinned
residue WRITER); CLEAR `rejection_reason`, `eval_verdict`, `judge_verdict`,
**`verdict_patch_id`**; set
`gc.kind=eval_request`, clear `gc.routed_to`; `--status=open --assignee=""`; KEEP
`eval_reject_count` (the shared reject budget — evaluator/judge own it) and
`overseer_issue_id` (external correlation — never cleared by agents).

**R2 — PINNED FRAGMENT NAMES (identity; files `template-fragments/<name>.template.md`):**
`polecat-code-hygiene`, `polecat-evidence-contract`, `polecat-final-rebase-revalidate`,
`polecat-autonomy-and-blockers`, `polecat-submit-to-evaluator`,
`polecat-overseer-issue-marker`. All six match GCD-WO-CSC-006's R1c discovery — a plain
`grep polecat` (all six names start `polecat-`, including
`polecat-overseer-issue-marker`) — and are disjoint from the six pre-existing
`polecat-*` names.
Render-order contract: cities append these AFTER the existing six, so
`polecat-submit-to-evaluator` renders LAST among done-sequence texts — its supersession
language plus last position make it the single live done target.

**R3 — verbatim anchors being superseded (quote-verify in worktree, DO NOT edit):**
`polecat-handoff-override` renders "## DONE SEQUENCE — USE THIS, NOT THE ONE ABOVE" ending
in `REFINERY_TARGET="${GC_RIG:+$GC_RIG/}gastown.refinery"` + wake/nudge + drain-ack;
`polecat-done-target-override` amends its target read; the upstream polecat prompt's
"## FINAL REMINDER: RUN THE DONE SEQUENCE" sits at
`gastown/agents/polecat/prompt.template.md:253`. The new sequence PRESERVES from them:
the bead-metadata target read (`.metadata.target_branch // .metadata.target // "main"` —
the landing-arbiter repair-bead field MUST NOT be clobbered), the
`--set-metadata KEY=` clears-the-value idiom, and the `gc runtime drain-ack` + exit
ending ("Idle Polecat heresy" rule).

**R4 — effective-var lookup (imported).** `bd mol wisp` does not consume
`[rigs.formula_vars]` at pour time (authority: the wisp-pour-vars-override fragment
itself) and the refinery prompt cannot render formula vars — so the gate must be
resolved AT RUN TIME inside formula steps. COPY the `effective_rig_var` block (city-root
resolution + function + the execution-pinned `<RIG-FORMULA-VARS-PATH>` jq path) VERBATIM
from the merged `mol-evaluate-task.formula.toml`. Boolean rule for the gate:
`EVALUATOR_GATED="true"` iff the wisp-rendered `{{evaluator_gated}}` is `"true"` OR the
rig's `formula_vars.evaluator_gated` is `"true"`; ANY lookup failure → `"false"`
(fail-safe: full battery). This is what makes GCD-WO-CSC-006's single
`evaluator_gated = "true"` line sufficient.

**R5 — full-battery exception predicate (pinned).** Gated smoke applies ONLY when ALL
hold: (a) `EVALUATOR_GATED = "true"`; (b) the work bead's `metadata.judge_verdict` is
exactly `PASS` or `NOT_REQUIRED` (read fresh via `gc bd show $WORK`; C9
conditional-judge — both are evidence-backed approvals); (c) `$BRANCH` does NOT match
`integration/*` (case-glob on the branch read from bead metadata — convoy autolands land
`integration/<wo-id>` onto the default target and KEEP the full battery; kit-verbatim);
(d) **content-state match (LAW-4):** the bead's `metadata.verdict_patch_id` is
non-empty AND equals the patch-id the refinery computes over the content it is about
to merge — post-rebase, in its own worktree:
`git diff "origin/$TARGET...HEAD" | git patch-id --stable | awk '{print $1}'`
(a clean rebase preserves the patch-id by construction; conflict resolutions change it
and legitimately void the degrade). A mismatch is NOT a rejection and NOT a defect —
it simply runs today's full battery (never degrade on a stale verdict). Session
restarts, re-wakes, and agent swaps never void a verdict — only content change does.
Anything else — ungated city, missing/stale verdict, ad-hoc slung bead, convoy landing —
runs today's full battery unchanged.

**R6 — fabricated-evidence taxonomy (generic; same list 003's evaluator hunts — the two
sides of one contract):** hard-coded or faked PASS values / proof output; empty proof
files, or proof files that CONTRADICT what the real command produces when re-run;
self-validating CIRCULAR manifests (a step writes the very "PASS" a later step copies
and "validates"); string obfuscation, renaming, or encoding used to dodge a
grep/detector; evidence values recalled or pattern-assembled instead of captured from
the executed command.

## Implementation Steps

**Step 0 — verification gate (record in evidence).** (a) base ≥ `c85d92cf` AND
GCD-WO-CSC-003 merged: `ls examples/gastown/packs/codegen-support/agents/evaluator/`
non-empty, `grep -n "eval_request" …/agents/evaluator/agent.toml`, README present —
STOP (structured blocker, do not improvise) if absent; (b) copy the merged
`effective_rig_var` block + jq path out of `mol-evaluate-task.formula.toml`; (c) verify
the R3 anchors byte-match; (d) verify none of the six fragment files exist yet; (e)
`grep -c "bd mol wisp mol-refinery-patrol" examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml`
— record the pour-site count (expected 6; edit ALL found, whatever the count).

**Step 1 — `polecat-code-hygiene.template.md`** (define `polecat-code-hygiene`; ≤ ~60
lines, imperative, generic). Content (derive from `implementer.md` CODE HYGIENE,
stripped per Architecture Links): applies to EVERY change (in-scope work AND repairs);
write to software best practices and EXTEND the architecture — every change strengthens
the system, never leaves it more fragile; NO band-aids, NO hacks, NO masking — NEVER
disable, loosen, narrow, skip, special-case, or edit a gate/test/check/detector to make
something pass; fix the ROOT CAUSE; **ABSOLUTE BAN on fabricated evidence** with the R6
taxonomy enumerated verbatim ("each of the following is an automatic failure of your
work item"); when a gate or the adversarial evaluator PUSHES BACK, the pushback IS
correct — the only acceptable responses are a real additive/restorative root-cause fix
or an honest structured blocker (see `polecat-autonomy-and-blockers`); if a check is
itself wrong, fix the CHECK correctly and document the corrected behavior as a GENERAL
repository-wide rule (ADR/spec note in the rig repo's convention), never blind the
detector; an honest RED with a root-cause fix always beats a green reached by weakening
the system.

**Step 2 — `polecat-evidence-contract.template.md`** (define
`polecat-evidence-contract`; ≤ ~50 lines). Content: produce the evidence your work item
declares — run its exact commands and capture REAL output (a description of output is
not output); write artifacts to the paths the work item names; the adversarial evaluator
independently RE-RUNS your declared commands in its own checkout of your PUSHED branch
and traces every evidence value to its source — evidence that cannot be reproduced is
treated as fabricated (see the taxonomy above); fast/partial recipes are for ITERATION
only — the full declared validation runs at your FINAL state and is the only run that
counts as evidence; **branch discipline: commit early and often on your assigned branch;
WIP commits are fine and expected; NEVER park work in `git stash` and never leave
completed work uncommitted — only commits on the pushed branch survive and only they are
evaluated; you are NOT the one who merges** — after the evaluator and judge pass, the
refinery merges; leave the branch clean and mergeable. Close the fragment with the
one-line cadence pointer: "Push cadence is governed by the WIP-push cadence discipline
rendered earlier in this prompt — follow it." (Authoring rule: the pointer is the ONLY
cadence content allowed here — the Non-Goals boundary bans restating its rules.)

**Step 3 — `polecat-final-rebase-revalidate.template.md`** (define
`polecat-final-rebase-revalidate`; ≤ ~45 lines). Content: START from the current target
— the FIRST thing you do is `git fetch origin` then rebase your branch onto the current
target branch, and only then assess prerequisites (a stale base makes already-merged
prerequisites look absent — never escalate "prerequisites missing" without refreshing
first); FINAL STEP before submitting (mandatory): `git fetch origin`, rebase onto the
CURRENT target, resolve any conflicts yourself preserving both your intent and the
sibling change (a real edit — the code-hygiene rules apply; never leave a rebase in
progress), then RE-RUN the work item's acceptance checks on the rebased tree and refresh
the evidence; submit only once the branch is rebased, cleanly mergeable, and green
post-rebase. The evaluator's merge check is conflict-only — being behind is fine,
CONFLICTING is a rejection you can prevent right here.

**Step 4 — `polecat-autonomy-and-blockers.template.md`** (define
`polecat-autonomy-and-blockers`; ≤ ~60 lines). Content: this pipeline runs AUTONOMOUSLY
— when a design choice is resolvable from the work item, the rig repo's specs, and its
existing conventions, RESOLVE it yourself (priority: the work item's stated intent, then
the repo's conventions); do not invent scope and do not guess blindly; do NOT escalate
decisions your context already settles; a GENUINE blocker (the work item names a
contract/input that truly does not exist after you refreshed onto the current target; or
an action outside your authority) STOPS the work: **before stopping, SWEEP the remaining
scope for every further blocker/seam incompatibility you can already see — the FIRST
STOP enumerates the COMPLETE known set (blueprint QST-1: batched by default; serial
one-question-per-cycle escalation wastes a full worker lifetime per question and is a
defect, not diligence)**; then mail the mayor ONE STRUCTURED DECISION PACKAGE —
`gc mail send mayor/ -s "BLOCKED: <bead-id> — <one-line decision needed> [HIGH]"` with
body carrying, per question: a stable question id (`<bead-id>#q1`, `#q2`, …), the exact
decision needed, the complete option set, your recommendation, the blast radius, and
what evidence would unblock; set the bead back with
`--set-metadata rejection_reason="blocked: <one line per question id>"` and `gc runtime
drain-ack` — the durable mail + bead metadata IS the question record (QST-2/ACK: the
mayor acknowledges receipt in-band by mail/nudge; once sent and drain-acked, NEVER
resend an unanswered question in a later session — resubmission churn is a defect; the
answer arrives as your bead's release, answer==release);
escalation is the rare LAST resort, never a default — and never idle-wait for an answer
in a running session (the session always releases: drain-ack).

**Step 5 — `polecat-submit-to-evaluator.template.md`** (define
`polecat-submit-to-evaluator`). THE load-bearing fragment — GCD-WO-CSC-006's
supersession gate reads it. Full required shape (command block verbatim; prose may be
tightened but every rule must survive):

````
{{ define "polecat-submit-to-evaluator" }}
---

## DONE SEQUENCE — SUBMIT TO EVALUATOR (SUPERSEDES ALL EARLIER DONE SEQUENCES)

**This section supersedes BOTH the upstream "FINAL REMINDER: RUN THE DONE
SEQUENCE" section AND the "DONE SEQUENCE — USE THIS, NOT THE ONE ABOVE"
refinery handoff earlier in this prompt (polecat-handoff-override), including
its target-field amendment (polecat-done-target-override).** When this
fragment is present, completed work is NEVER assigned or routed to the
refinery by you — it is submitted to the evaluator pool. The refinery only
receives this bead later, from the judge, after an evidence-backed PASS.

Substitute `<work-bead>` and `<brief summary>`; run verbatim:

```bash
git push origin HEAD

BEAD_ID="<work-bead>"
SUMMARY="<brief summary>"
BRANCH=$(git branch --show-current)

# Target is a property of the BEAD, not the rig or the prompt. Honor, in
# order: metadata.target_branch (repair beads against integration landings —
# never clobber), metadata.target (integration-branch task beads), "main".
TARGET=$(gc --rig "$GC_RIG" bd show "$BEAD_ID" --json 2>/dev/null \
  | jq -r '.[0].metadata.target_branch // .[0].metadata.target // "main"')
[ -z "$TARGET" ] && TARGET="main"

# Overseer correlation (conditional): carry the marker into the notes so the
# refinery's eventual PR body includes it. See polecat-overseer-issue-marker.
OVERSEER_ISSUE=$(gc --rig "$GC_RIG" bd show "$BEAD_ID" --json 2>/dev/null \
  | jq -r '.[0].metadata.overseer_issue_id // empty')
NOTES="Implemented: $SUMMARY"
[ -n "$OVERSEER_ISSUE" ] && NOTES="$NOTES
Overseer-Issue: $OVERSEER_ISSUE"

# Structured residue declaration (C9 `residue` — GEN-6 close-out law). One row
# per acceptance criterion / scope item of your work item: status delivered,
# not-delivered, or known-gap; every NON-delivered row names an EXISTING bead
# or work-order id in mapped_to (fold residue into existing vehicles — never
# defer to an imaginary future WO, never omit a row). The evaluator REJECTs
# silent residue (missing rows, unmapped gaps, or a "delivered" the diff does
# not deliver). Downstream planning consumes these rows as premises.
RESIDUE='[{"item":"<criterion or scope item>","status":"delivered"},
          {"item":"<gap>","status":"known-gap","mapped_to":"<existing bead/WO id>"}]'

# Write submission metadata. --set-metadata KEY= clears the value: stale
# verdicts and rejection reasons from earlier attempts must never ride into a
# fresh evaluation (verdict_patch_id included — the verdict content-state key
# is re-earned per evaluation, C9). eval_reject_count is NOT touched — the
# evaluator/judge own the shared reject budget. overseer_issue_id is never
# cleared. residue is written fresh here on EVERY submission.
gc bd update "$BEAD_ID" \
  --set-metadata branch="$BRANCH" \
  --set-metadata target="$TARGET" \
  --set-metadata rejection_reason= \
  --set-metadata eval_verdict= \
  --set-metadata judge_verdict= \
  --set-metadata verdict_patch_id= \
  --set-metadata residue="$RESIDUE" \
  --notes "$NOTES"

# Route to the evaluator pool (typed selector on gc.kind=eval_request).
# Clearing gc.routed_to strips any stale pool route from a prior rejection.
# No wake/nudge needed: evaluator pools are demand-scaled by the controller.
gc bd update "$BEAD_ID" \
  --status=open \
  --assignee="" \
  --set-metadata gc.kind=eval_request \
  --set-metadata gc.routed_to=

gc runtime drain-ack
exit
```

**On rejection (resume-and-fix):** a bead that comes back to the pool with
`metadata.rejection_reason` and your intact branch is a RESUME, not a redo.
The evaluator's or judge's pushback IS correct: fix the root cause on the
SAME branch, re-run the acceptance checks, and resubmit through THIS
sequence (which clears the stale verdicts). Never regenerate from zero —
`regenerate_on_reject` is a reserved future mode (see the codegen-support
README), not current behavior. At the reject budget the pipeline escalates
to the mayor on its own; do not self-escalate a rejection you can fix.
{{ end }}
````

**Step 6 — `polecat-overseer-issue-marker.template.md`** (define
`polecat-overseer-issue-marker`; ≤ ~30 lines). Content: CONDITIONAL duty — before your
final commit, read `metadata.overseer_issue_id` from your work bead
(`gc --rig "$GC_RIG" bd show "$BEAD_ID" --json | jq -r
'.[0].metadata.overseer_issue_id // empty'`); if EMPTY this section is inert — do
nothing; if present: (1) the FINAL commit message on your branch (the branch HEAD at
submission) carries the trailer line `Overseer-Issue: <issue-id>` — exact key spelling
(case-sensitive), one space after the colon, the id verbatim, on its own line in
standard git-trailer position; the full line must match
`^Overseer-Issue: [A-Za-z0-9_.:-]+$` (the marker-grammar authority is cited in this
WO's provenance — the fragment states the grammar generically, no estate paths);
(2) your done-sequence notes carry the same line (the submit sequence above does this
automatically — verify, don't duplicate); (3) **set the bead's merge strategy to the PR
lane** (A2.10 RULING): `gc bd update "$BEAD_ID" --set-metadata merge_strategy=mr` — any
bead with a non-empty `overseer_issue_id` MUST merge via a pull request. Rationale
(state it in the fragment, generically): `metadata.merge_strategy` defaults to
`"direct"` (`mol-refinery-patrol.toml:380`), and a direct merge produces NO PR body — the
marker and the downstream merge-event correlation die with it, so the originating
issue's ledger never advances; `mr` routes the merge through the PR body the marker
reader scans. Purpose (generic phrasing): downstream
tracking scans pull-request bodies first, then the head-commit message, for this
marker to correlate merged work back to the originating issue — the notes line covers
the PR body (the refinery builds PR bodies from bead notes), the commit trailer covers
the head-commit lane; never invent an id and never copy one from another bead. (Kit
A3.2 note: the in-program reader is the PR-body arm only; the commit trailer
future-proofs the head-commit lane.)

**Step 7 — `mol-refinery-patrol.toml` `evaluator_gated` branches** (gastown pack; the
formula is 982 lines — edit surgically, keep every existing recipe byte-identical except
the enumerated insertions):

- **7a. Var declaration** — append after `[vars.event_timeout]`:

```toml
[vars.evaluator_gated]
description = "When \"true\" AND the work bead carries an approved verdict (judge_verdict PASS or NOT_REQUIRED) whose verdict_patch_id matches the patch-id of the content about to merge AND the merge source is not an integration/* landing: run-tests degrades to setup+build smoke and handle-failures to smoke-rejection+merge mechanics. Full battery always kept for integration/* landings, for beads without an approved verdict, and for any verdict_patch_id mismatch (never degrade on a stale verdict — verdicts key to content-states, not sessions). Effective value is wisp var OR [rigs.formula_vars] (resolved at run time — bd mol wisp does not consume rig vars at pour time; see refinery-wisp-pour-vars-override)."
default = "false"
```

- **7b. `run-tests` step** — PREPEND to the step description (before the existing
  Config lines), the gate block: the imported R4 `effective_rig_var` preamble, then:

```bash
# Session-restart-safe: re-derive the work bead + branch if this step runs
# in a fresh session (find-work/rebase context lost).
if [ -z "${WORK:-}" ]; then
  WORK=$(gc bd list --assignee=$GC_AGENT --status=open \
    --exclude-type=epic --limit=1 --json | jq -r '.[0].id // empty')
fi
BRANCH=${BRANCH:-$(gc bd show $WORK --json | jq -r '.[0].metadata.branch // empty')}
TARGET=${TARGET:-$(gc bd show $WORK --json | jq -r '.[0].metadata.target_branch // .[0].metadata.target // "main"')}
EVALUATOR_GATED="{{evaluator_gated}}"
[ "$EVALUATOR_GATED" = "true" ] || EVALUATOR_GATED=$(effective_rig_var evaluator_gated "{{evaluator_gated}}" "false")
JUDGE_VERDICT=$(gc bd show $WORK --json | jq -r '.[0].metadata.judge_verdict // empty')
# Content-state guard (C9 verdict_patch_id, blueprint LAW-4): the verdict is
# valid for exactly the content it proved. Compute the patch-id of what this
# refinery run will actually merge (post-rebase worktree HEAD vs target); a
# clean rebase preserves it, conflict resolutions change it. Mismatch or any
# read failure => full battery (never degrade on a stale verdict). Session
# restarts / re-wakes never void a verdict — only content change does.
VERDICT_PATCH_ID=$(gc bd show $WORK --json | jq -r '.[0].metadata.verdict_patch_id // empty')
PATCH_ID=$(git diff "origin/$TARGET...HEAD" 2>/dev/null | git patch-id --stable | awk '{print $1}')
GATED_SMOKE="false"
if [ "$EVALUATOR_GATED" = "true" ] && [ -n "$BRANCH" ] \
   && { [ "$JUDGE_VERDICT" = "PASS" ] || [ "$JUDGE_VERDICT" = "NOT_REQUIRED" ]; } \
   && [ -n "$VERDICT_PATCH_ID" ] && [ -n "$PATCH_ID" ] \
   && [ "$PATCH_ID" = "$VERDICT_PATCH_ID" ]; then
  case "$BRANCH" in integration/*) : ;; *) GATED_SMOKE="true" ;; esac
fi
```

  Then the branch text: **If `GATED_SMOKE = "true"`:** run ONLY `{{setup_command}}` and
  `{{build_command}}` (skip empty ones silently) — the quality battery already ran in
  the parallel evaluator lane and was judge-approved; the smoke exists to catch
  broken-at-rebase compilation only. **Else (everything else, verbatim today's
  behavior):** the existing setup/typecheck/lint/build + run_tests recipe, UNCHANGED.
  Close-condition line updated to "when the selected checks complete". Explicit note in
  the step: "Full battery is DELIBERATELY kept for `integration/*` landings (convoy
  autolands — the one-full-gate-at-final-state backstop), for any bead without an
  approved verdict (`judge_verdict` PASS/NOT_REQUIRED), and for any bead whose
  `verdict_patch_id` does not match the content about to merge — a stale verdict never
  degrades anything (fail-safe: more testing, never less; the mismatch itself is not a
  defect and not a rejection)."

- **7c. `handle-failures` step** — keep the existing text intact; APPEND a gated clause
  to the branch-caused fork: when `GATED_SMOKE = "true"` and the smoke failed because of
  the branch, use the reject-to-pool recipe (workflow cleanup + `gc bd update` back to
  the pool) with THREE deviations from the ungated recipe, each explicit in the formula
  text:
  1. add `--set-metadata eval_verdict= --set-metadata judge_verdict=
     --set-metadata verdict_patch_id=` to the
     `gc bd update` call (a rejected-by-refinery bead must never retain a stale judge
     PASS or its content-state key; the resubmission re-earns all three);
  2. prefix the reason: `rejection_reason="gated smoke failed: <failure summary>"`;
  3. **do NOT delete the branch** — the ungated recipe's
     `git push origin --delete $BRANCH` line is OMITTED in the gated clause
     (resume-and-fix, D10/C9: the returning polecat resumes on the INTACT pushed
     branch; deleting it would silently convert resume-and-fix into
     regenerate-from-zero). Local cleanup (`git checkout "$TARGET" && git branch -D
     temp`) stays.
  Re-derive `EVALUATOR_GATED`/`GATED_SMOKE` at the top of this step if unset (sessions
  can restart between steps; same guard block, idempotent). The UNGATED branch-caused
  fork keeps its existing branch-delete behavior byte-identically, and the
  pre-existing-on-target fork and the "FORBIDDEN: Writing code" gate are UNCHANGED in
  both modes.

- **7d. Pour-site propagation** — every `gc bd mol wisp mol-refinery-patrol` invocation
  INSIDE the formula (all sites found in Step 0e; expected 6: check-inbox, rebase,
  handle-failures, `block_existing_pr()`, next-iteration) gains
  `--var evaluator_gated={{evaluator_gated}}` so a gated wisp's successors stay
  observably gated on the wisp bead. (The refinery PROMPT's pour sites are NOT edited —
  Non-Goals; the run-time lookup makes prompt-poured wisps correct regardless.)

- **7e. Version + description** — bump `version = 4` → `5`; append one description
  paragraph: "v5 (evaluator-gated flow): when a city enables `evaluator_gated`,
  judge-approved beads get a build smoke instead of the full battery; integration/*
  convoy landings and non-judge-approved beads keep the full battery. See the
  codegen-support README (verdict metadata contract) for the pipeline."

**Step 8 — `refinery-wisp-pour-vars-override.template.md` update** (codegen-support;
per the fragment's own "append matching `--var key=value` lines here" instruction).
Insert BEFORE the canonical pour command: the imported R4 `effective_rig_var` lookup
block (city-root resolution + function, copied VERBATIM from the merged
`mol-evaluate-task.formula.toml` — the FULL block, it is not 3 lines) plus ONE call
line — `EVALUATOR_GATED=$(effective_rig_var evaluator_gated "" "false")` — both must be
present in the fragment (it already runs in bash context); then add to the canonical
command: `--var evaluator_gated="${EVALUATOR_GATED:-false}"`. Extend the
"Verification" jq example to also surface `evaluator_gated`. Update the fragment's
explanatory paragraph: `integration_branch_auto_land` AND `evaluator_gated` are the two
vars whose effective values differ from formula defaults in bound cities. ALSO correct
the fragment's now-false tail sentence — "The set of variables passed at pour time IS
the set the wisp will use; no city-config fallback applies." stops being true once this
WO merges: formula steps wired through `effective_rig_var` (R4) DO consult
`[rigs.formula_vars]` at run time. Replace that sentence with: "The set of variables
passed at pour time is what the wisp renders; for vars wired through the run-time
`effective_rig_var` lookup (e.g. `evaluator_gated`), `[rigs.formula_vars]` is consulted
at run time as the fallback — explicit pour-time `--var` values still win." NO other
change to the fragment.

**Step 9 — packlint tests.**

- **(a) `test/packlint/csc_polecat_fragments_test.go`** (pattern:
  `spec_cartographer_formula_test.go`): all six files exist; each opens with
  `{{ define "<pinned name>" }}`; `polecat-submit-to-evaluator` contains (normalized
  whitespace): "SUPERSEDES ALL EARLIER DONE SEQUENCES", the handoff-override +
  done-target-override supersession sentence, `gc.kind=eval_request`,
  `--assignee=""`, the FOUR verdict-clear args (`rejection_reason=`, `eval_verdict=`,
  `judge_verdict=`, `verdict_patch_id=`), the residue write (`--set-metadata
  residue=` + the three status literals `delivered`/`not-delivered`/`known-gap` +
  `mapped_to` + "silent residue"), the target read
  `.metadata.target_branch // .metadata.target // "main"`, "resume-and-fix" +
  "regenerate_on_reject" (reserved reference), and does NOT contain a refinery
  assignment (`assignee="$REFINERY_TARGET"` absent) nor `session wake`/`session nudge`;
  `polecat-code-hygiene` contains all five R6 taxonomy items + "pushback" +
  "ROOT CAUSE"; `polecat-evidence-contract` contains "NEVER park work in `git stash`" +
  "commit early" + "not the one who merges"; `polecat-final-rebase-revalidate` contains
  the fetch/rebase-then-re-run-acceptance pair + "conflict-only" prevention line;
  `polecat-autonomy-and-blockers` contains the structured-blocker field list (question
  id `#q1` grammar / decision / complete option set / recommendation / blast radius) +
  the batching rule ("COMPLETE known set") + the ACK/no-resend rule ("NEVER resend an
  unanswered question") + "LAST resort"; `polecat-overseer-issue-marker`
  contains the exact literal `Overseer-Issue: `, `overseer_issue_id`, the id-grammar
  charset (`[A-Za-z0-9_.:-]+`), the `merge_strategy=mr` instruction (A2.10 ruling), and
  the inert-when-absent rule. **Cadence-boundary
  absence battery (Non-Goals pin):** across ALL six new fragment files, the strings
  `force-with-lease` and `after EVERY commit` do NOT appear (push cadence is
  GCD-WO-CSC-002's `wip-push-cadence` territory); `polecat-evidence-contract` DOES
  contain the one-line cadence pointer ("Push cadence is governed by").
- **(b) `test/packlint/csc_refinery_gating_test.go`**: `mol-refinery-patrol.toml`
  contains `[vars.evaluator_gated]` + `default = "false"`; `version = 5`; the gate
  predicate strings (`judge_verdict`, `NOT_REQUIRED`, `verdict_patch_id`,
  `git patch-id --stable`, the mismatch-runs-full-battery note ("never degrade on a
  stale verdict"), `integration/*` case-glob, `GATED_SMOKE`); the
  session-restart re-derive line from 7b (assert the literal
  `.[0].metadata.branch // empty` appears in the `run-tests` gate block, so
  `case "$BRANCH" in integration/*)` never reads an undefined value); the
  gated-smoke command pair (setup+build only) and the retained full-battery text; the
  THREE verdict-clear args in the gated rejection (incl. `verdict_patch_id=`); the
  gated no-branch-delete pin (the gated
  clause contains "do NOT delete the branch" / omits `git push origin --delete` — assert
  the ungated fork still contains exactly its one pre-existing delete line, count
  unchanged); the retained literal "FORBIDDEN: Writing
  code to fix failures"; `--var evaluator_gated=` occurrence count ≥ the Step-0e
  pour-site count; `effective_rig_var` present. `refinery-wisp-pour-vars-override`
  contains `--var evaluator_gated=` and the updated verification key.
- **(c) generic-ness grep gate** (fold into (a)): case-insensitive
  `matchpoint|enrichment|vehicle` over the six new fragment files = zero hits; no
  `master/` path; no `sk-ant-`/token-like string.
- Planted-RED discipline: these are contains-assertions — each fails when its literal is
  absent by construction; additionally assert the ABSENCE cases named in (a) (refinery
  assignment absent in the submit fragment) so a wrong-direction merge of old content
  goes RED.

**Step 10 — full validation battery** (see Validation) + PR.

## Git Workflow

Loop execution: branch `wo/GCD-WO-CSC-005-polecat-diligence-refinery-merge-only` (or
`polecat/$BEAD_ID`) in this repo; PR to `origin/main` on `mike-matchpoint/gascity`
(never upstream). Same-wave serialization: dispatched only after GCD-WO-CSC-003 is
MERGED (harness DEPS direct-write edge) — if its deliverables are absent from
`origin/main` after the Rule-8-style refresh, that is a real dependency gap: STOP with a
structured blocker, never improvise the C9 imports. Never commit directly to `main`;
commit early/often; the harness merges.

## Test Coverage

- **Packlint string-contract tier** (Step 9): every pinned fragment name, the
  supersession language, the routing write, the verdict-clears, the gate predicate, the
  full-battery exception, the pour-site propagation, and the version bump are each
  pinned by a failing-when-absent assertion — the drift alarm for GCD-WO-CSC-006's R1c
  discovery and supersession gates.
- **Negative/absence tier** (Step 9a): the submit fragment asserting NO refinery
  assignment/wake — regression-pins the exact seam bug (two live done-targets) that
  GCD-WO-CSC-006's Step-3 gate exists to catch.
- **Repo baseline:** `make build && make check`; `go test ./test/packlint`;
  `go test ./internal/builtinpacks` (embedded content hash recomputes over the edited
  gastown formula + new codegen-support fragments — proves the embed globs cover them
  with zero embed edits).
- Every Acceptance Criterion names its backing test.

## Validation

- `make build && make check` green; `go test ./test/packlint` green;
  `go test ./internal/builtinpacks` green.
- **Byte-preservation audit:** `git diff --quiet origin/main --
  examples/gastown/packs/codegen-support/template-fragments/polecat-handoff-override.template.md
  examples/gastown/packs/codegen-support/template-fragments/polecat-done-target-override.template.md
  examples/gastown/packs/gastown/agents/polecat/ examples/gastown/packs/gastown/agents/refinery/prompt.template.md`
  exits 0 (untouched); diff to `mol-refinery-patrol.toml` contains ONLY the 7a–7e
  insertions (review the hunks — existing recipes byte-identical).
- **Generic-ness grep gate:** `grep -riE "matchpoint|enrichment|vehicle"` over the six
  new fragments + the two edited files returns no runtime-surface hits.
- C9 import audit: every verdict key/value literal in the new content matches the merged
  GCD-WO-CSC-003 README table field-for-field (record the comparison in evidence); no
  contract re-declaration (this WO's text says "see the codegen-support README" wherever
  semantics, not literals, are needed).
- `gc lint` over a scratch city composing gastown + codegen-support: clean (the six new
  fragment names parse; no dangling references).
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica /
  suspended); this WO performed no cluster/AWS interaction, started no city, no daemon,
  no supervisor; no live merge or eval ran. Live gated-flow validation on the
  vehicle-graph pilot is GCD-WO-CSC-006's named un-pause follow-up.
- Evidence artifacts: command transcripts as `{command, output_excerpt}` pairs.

## Acceptance Criteria

Each criterion names its backing test:

1. Six fragments shipped with the EXACT R2 names and define-wrappers —
   `test/packlint/csc_polecat_fragments_test.go`.
2. `polecat-submit-to-evaluator` supersedes BOTH earlier done sequences in explicit
   language, routes via `gc.kind=eval_request` with `--assignee=""` + cleared
   `gc.routed_to`, clears `rejection_reason`/`eval_verdict`/`judge_verdict`/
   `verdict_patch_id`, writes the structured `residue` rows (delivered /
   not-delivered / known-gap, gaps mapped to existing beads/WOs), preserves
   the bead-metadata target read, keeps `eval_reject_count`/`overseer_issue_id`
   untouched, and contains NO refinery assignment or wake/nudge — same test (contains +
   absence assertions).
3. Code-hygiene fragment carries the full R6 fabricated-evidence taxonomy +
   pushback-is-correct + additive-repair rule; evidence/rebase/autonomy fragments carry
   their pinned load-bearing lines — same test.
4. Overseer marker fragment: exact `Overseer-Issue: <issue-id>` trailer duty on final
   commit + notes, PLUS the `merge_strategy=mr` write for overseer-routed beads (A2.10
   ruling — no direct merges, the marker needs a PR body), conditional on
   `overseer_issue_id`, inert when absent — same test (A1 §2 discharged for the polecat
   side).
5. `mol-refinery-patrol` v5: `evaluator_gated` var (default `"false"`), run-time
   effective-gate resolution, the R5 predicate complete — approved verdict
   (`PASS`/`NOT_REQUIRED`) AND `verdict_patch_id` matching the computed patch-id of
   the content about to merge (mismatch = full battery, never a rejection) — gated
   smoke = setup+build only, gated `handle-failures`
   rejection clears stale verdicts incl. `verdict_patch_id` AND preserves the branch
   (no gated
   `git push origin --delete`; resume-and-fix), `integration/*` + missing-approval +
   patch-id-mismatch all
   keep the FULL battery, all in-formula pour sites propagate the var, ungated text
   byte-identical — `test/packlint/csc_refinery_gating_test.go` + byte-preservation
   audit.
6. `refinery-wisp-pour-vars-override` canonical pour carries the `evaluator_gated` var +
   updated verification — same test.
7. Zero domain literals; no embed/pack.toml/Go-non-test changes; predecessors
   byte-identical — grep gate + diff audits (Validation).
8. No city started; no AWS/cluster call; no live run (cities PAUSED) — Validation
   clause.

## Risks

- **Two live done-targets in a rendered polecat prompt** (the catastrophic seam): a city
  appending the new fragments WITHOUT the supersession language would leave both the
  refinery handoff and the evaluator submission "live". Mitigated three ways: the
  supersession language is pinned verbatim (Step 5), packlint pins it (9a), and
  GCD-WO-CSC-006's Step-3 gate re-checks it at binding time and STOPS on failure.
- **Gate resolution drift** (`bd mol wisp` semantics change upstream, or
  `gc config show --json` shape moves): the effective-var block is imported VERBATIM
  from 003 (single spelling), fails SAFE to full battery, and packlint pins its presence
  — a silent no-gate outcome is more-testing-not-less by design.
- **Convoy-autoland regression**: the `integration/*` predicate is a case-glob on the
  branch already read by the step — if a city ever lands integration branches under a
  different prefix, the full battery still applies to any bead lacking
  `judge_verdict=PASS` (convoy beads assigned by `next-iteration` never carry one), so
  the backstop holds by the second predicate even if the first misses. State this
  dual-predicate rationale in the 7b note.
- **Stale judge PASS after refinery rejection**: cleared explicitly in 7c AND cleared
  again by the submit sequence on resubmission — belt and suspenders across the two WOs
  (the R1 handshake) — AND structurally dead even if both clears were missed: the R5(d)
  content-state guard compares `verdict_patch_id` against the actual content about to
  merge, so a stale verdict can never buy a degrade (third, mechanical axis).
- **`refinery-rebase-conflict-auto-resolve` interaction** (city fragment that
  auto-resolves trivial rebase conflicts in some deployed cities): its path re-merges
  WITHOUT re-running the polecat — under the gated flow such a bead still carries its
  judge PASS and gets the smoke. Accepted: the auto-resolve fragment limits itself to
  trivial conflicts by its own contract, and the convoy-autoland full battery backstops
  integration drift (WS2 K4). Flag, don't fix — changing that fragment is city-binding
  scope.
- **WO size**: 6 fragments + 1 formula edit + 1 fragment edit + 2 test files = one
  coherent run. If prose balloons, tighten fragment wording — never drop a pinned
  literal or an absence assertion.

## Done Means

- [ ] Step-0 verification transcript (003 merged, anchors byte-match, pour-site count,
      imported lookup block).
- [ ] Six fragments shipped (R2 names); submit-override verbatim rules intact; marker
      fragment discharges A1 §2 polecat duty.
- [ ] `mol-refinery-patrol` v5 gated branches per 7a–7e; ungated behavior
      byte-identical; pour sites propagate the var.
- [ ] `refinery-wisp-pour-vars-override` updated per Step 8.
- [ ] Packlint tests green incl. absence assertions; `make build && make check` green;
      `go test ./internal/builtinpacks` green; byte-preservation + grep gates clean.
- [ ] C9 imports verified field-for-field against merged 003 content; zero
      re-declarations.
- [ ] PR merged to `origin/main` via the loop; no direct-to-main commit.
- [ ] No city started; live pilot validation left to GCD-WO-CSC-006's named follow-up.

## Master cutover contribution

None — platform repo (GasCity-Dev fork), no AWS resources created, renamed, or deleted
(kit K1 prod-gate language not triggered). The A1 §2 overseer-marker duty this WO
implements is the polecat half of the estate's issue-correlation thread; the exec-city
half is GCD-WO-CSC-008 and the ledger-advancement listener is AGC-WO-CSC-004 — no
cutover artifact originates here. Runtime exposure reaches hosted cities only via the
wave-24/25 bindings + deploy WOs at un-pause, under their own gates.

## Blueprint conformance (amended 2026-07-14 — LAW-4/ROL-5/ROL-6/QST/GEN-6)

Tail amendment — BINDING (see the header note). Reshaped pre-dispatch to the ratified
generation-system blueprint (`master/generation-architecture/BLUEPRINT.md` v1.4), in
lockstep with the amended C9 authority (GCD-WO-CSC-003 — same wave, merged first; the
R1 import table above already reflects it). Edits are integrated above (Goal 1/2, R1,
R5, Steps 4/5/7a/7b/7c, Step 9a/9b, AC 2/5, Risks); summary and citation map:

1. **Content-state keying of the refinery degrade (LAW-4/STM-5) — the load-bearing
   item.** `evaluator_gated` degrade is valid ONLY when the approved verdict's
   `verdict_patch_id` (C9) equals the patch-id the refinery computes over the content
   it is about to merge, post-rebase, in its own worktree (R5(d); the 7b gate block).
   Mismatch or any read failure = today's full battery — never a rejection, never a
   degrade on a stale verdict. Clean rebases preserve the patch-id by construction;
   conflict resolutions change it and legitimately void the degrade. Session
   restarts, re-wakes, and agent swaps never void a verdict — only content change
   does. The submit sequence (Step 5) and the gated rejection (7c) both clear
   `verdict_patch_id` alongside the verdicts, so the key is re-earned per
   content-state.
2. **Acting evaluator (ROL-5) — consumer side.** Owned by GCD-WO-CSC-003 (capability
   self-test, evidence honesty, unsubstantiated-PASS = infra failure); this WO's
   fragments carry the producer half unchanged: real commands, real captured output,
   evidence the evaluator re-runs (Steps 1–3). No re-declaration (C9 discipline).
3. **Conditional judge (ROL-6) — consumer side.** The refinery accepts
   `judge_verdict` `PASS` OR `NOT_REQUIRED` (the evaluator's no-risk-marker direct
   handoff) as the approved-verdict predicate (R5(b), 7a/7b); the judge lane runs
   only on risk markers, per the C9 rule this WO imports. Everything else about the
   full-battery backstop (integration/* landings, missing approval) is unchanged —
   fail-safe remains more testing, never less.
4. **Question package + batching + ACK (QST-1/2/5).** `polecat-autonomy-and-blockers`
   (Step 4) now pins: one structured DECISION PACKAGE per STOP (stable question id
   `<bead-id>#qN`, decision, complete option set, recommendation, blast radius,
   unblocking evidence); the FIRST STOP enumerates the COMPLETE known
   blocker/incompatibility set (sweep remaining scope before stopping — serial
   one-question-per-cycle escalation is a defect); the mayor ACKs receipt in-band
   (mail/nudge) and an unanswered question is NEVER resent (the durable mail + bead
   metadata is the question record; answer==release); the session always releases
   (drain-ack — the existing GasCity idiom, kept).
5. **Residue rows (GEN-6/GOV-4) — producer side.** The submit sequence (Step 5)
   writes the C9 `residue` rows on EVERY submission: one row per acceptance
   criterion/scope item — delivered / not-delivered / known-gap — every gap mapped to
   an EXISTING bead or WO (no-deferral law: residue folds into existing vehicles).
   Silent residue is an evaluator REJECT (enforced by GCD-WO-CSC-003's ordered
   checks); downstream planning (GCD-WO-CSC-004's router) consumes the rows as
   premises.
