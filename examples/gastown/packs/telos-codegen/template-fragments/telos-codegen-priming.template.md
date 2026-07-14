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

### No verdicts here

This fragment primes design conscience only. It grants NO review or verdict
authority: conformance verdicts stay in the city's single evaluator/judge
lane. If you hold a codegen role, you never grade your own or anyone else's
work.

{{ end }}
