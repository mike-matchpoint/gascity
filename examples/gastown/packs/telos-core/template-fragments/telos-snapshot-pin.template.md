{{ define "telos-snapshot-pin" }}
---

## TELOS SNAPSHOT PIN — SYSTEM DESIGN CONTEXT (BINDING)

Your city root carries a sha-pinned snapshot of the system-level telos — the
purpose, boundary, and change-law head that governs the repos this city works
on: `{{ .CityRoot }}/specs/SYSTEM-TELOS.md`.

### Read the snapshot

1. Read `{{ .CityRoot }}/specs/SYSTEM-TELOS.md` before acting on any bead.
2. Its provenance header is the pin:

   ```
   > snapshot-of: <source-path> @ <sha256-12> | specs-version: N | synced: YYYY-MM-DD
   ```

   The `specs-version` value `N` is the system telos version you were primed
   with. The `sha256-12` value pins the exact upstream content this snapshot
   mirrors.

### Surface the pin in evidence

Record the pinned version in your work evidence as `telos: system vN` (the
telos-evidence-line fragment defines the full evidence-line contract). Stale
priming must be visible in evidence review, so the version line is mandatory,
not decorative.

### Missing snapshot — fail LOUD, never silent

If `{{ .CityRoot }}/specs/SYSTEM-TELOS.md` does not exist, or its provenance
header is absent or unparseable:

- State `TELOS SNAPSHOT: MISSING` (or `TELOS SNAPSHOT: PROVENANCE UNPARSEABLE`)
  prominently in your bead notes/evidence AND in any close or handoff message.
- File a question bead naming the missing snapshot and mail your city's
  overseer session (for example the mayor). Silent-missing design context is
  itself a defect; surfacing it is part of your job.
- Do NOT proceed as if primed. Purely mechanical work may continue, but no
  design-bearing decision is made without the snapshot.

### Pointers, never copies (BINDING)

Your prompts and this pack carry POINTERS to the snapshot, never a second copy
of the law. Never paste snapshot law text into committed files, fragments, or
derived documents — read it at runtime from the city root. A copied law text is
a stale fork by construction.

{{ end }}
