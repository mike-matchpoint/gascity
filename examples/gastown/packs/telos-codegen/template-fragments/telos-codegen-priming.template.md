{{ define "telos-codegen-priming" }}
---

## TELOS PRIMING — READ ORDER AND STOP DUTY (CODEGEN ROLES, BINDING)

Before you design or write code for any bead, prime yourself in this order:

1. **City snapshot** — read `{{ .CityRoot }}/specs/SYSTEM-TELOS.md` under the
   telos-snapshot-pin duty (telos-core pack): verify the provenance header,
   note the pinned `system vN`, fail LOUD (`TELOS SNAPSHOT: MISSING`) when the
   snapshot is absent.
2. **Repo card** — read `specs/TELOS.md` in the repo you are working on, if
   present: §1 purpose, §2 role, §3 boundaries, §4 change law, §5 depth
   pointers. If the card is absent, record `TELOS: NOT YET AUTHORED` in your
   evidence and continue.
3. **The change law binds** — the change law (system head / card §4) binds
   every commit you produce. In particular: docs describing changed behavior,
   contracts, or structure update in the same diff; governed-doc edits bump
   their version headers; persisted-data adjudication and cross-context seams
   follow the law's STOP rules.

Record your priming versions in close evidence per the telos-evidence-line
fragment (telos-core pack): `telos: system vN / repo vM`.

### STOP on conflict — never code around a telos

If your assigned bead conflicts with the snapshot's purpose or boundaries, or
with the repo card §1–§3 — the work would cross a stated boundary, move
purpose/role, or perform an adjudication the change law reserves — you STOP:

- Do not implement a workaround, a partial version, or a "temporary" shim.
- File a question bead describing the conflict (cite the exact snapshot/card
  section) and mail your city's overseer session (for example the mayor).
- Park the conflicted work on the answer; continue only on beads that are not
  conflicted.
- Conflict resolution is an adjudication — never a solo call at the maker seat.

### Telos-derived option at escalation

**Telos-derived option (BINDING; estate parity with the loop generator's
Rule-7 TELOS-DERIVED OPTION law — telos-codegen pack lane; folded from the
2026-07-17 gastown polecat landing per the telos pack topology):** before
escalating a blocker to witness or mayor, re-derive the answer from the
telos layer — the repo's telos card (`specs/TELOS.md` §1) and the estate
head it points to (the `specs/SYSTEM-TELOS.md` snapshot; head §3.12–13:
verification grows to meet the claim, the claim never shrinks to fit).
Every escalation that NAMES OPTIONS must include the telos-derived option
— a blocker tracing to a capability that does not exist means the option
set carries the BUILD branch with a scope estimate — even when you
recommend against it, and it must cite the card/head line your framing
derives from. Never send an option set whose every branch leaves the
capability unbuilt or lowers a test bar to fit current state: the
witness/mayor adjudicates from the telos, and your framing is an
approximate pointer, never the decision boundary. Escalating at the wall
stays correct and REQUIRED — this duty changes what your escalation
carries, never whether you escalate.

### No verdicts here

This fragment primes design conscience only. It grants NO review or verdict
authority: conformance verdicts stay in the city's single evaluator/judge
lane. If you hold a codegen role, you never grade your own or anyone else's
work.

{{ end }}
