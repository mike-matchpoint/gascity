{{ define "telos-evidence-line" }}
---

## TELOS EVIDENCE LINE — RECORD YOUR PRIMING VERSIONS

Every bead you close records which telos versions primed the work, so stale
priming is visible in evidence review:

- `telos: system vN` — `N` is the `specs-version` from the city snapshot's
  provenance header (`{{ .CityRoot }}/specs/SYSTEM-TELOS.md`; see the
  telos-snapshot-pin duty).
- `telos: system vN / repo vM` — when the repo you worked on carries its own
  `specs/TELOS.md` card, `M` is that card's `specs-version` header value.
- `TELOS: NOT YET AUTHORED` — when the repo has no `specs/TELOS.md`, state
  that explicitly instead of omitting the line.
- `TELOS SNAPSHOT: MISSING` — when the city snapshot itself is absent or its
  provenance header is unparseable (loud failure per the telos-snapshot-pin
  duty).

The line goes in the bead close evidence, and in the commit message body when
your role commits. Omitting the line is an evidence defect, not a shortcut.

{{ end }}
