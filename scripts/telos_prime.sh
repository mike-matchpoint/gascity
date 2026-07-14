#!/bin/sh
# telos_prime.sh — estate-canonical SessionStart telos primer (telos hook lane).
#
# CANONICAL COPY: Matchpoint-Platform/scripts/telos_prime.sh (ADR-026: Platform hosts
# estate law). Vendored BYTE-IDENTICAL into every telos-scope repo at
# scripts/telos_prime.sh; sha-pinned against this copy by Matchpoint-Estate-Ops
# scripts/telos_hook_parity.py. NO per-repo content lives here — per-repo
# specificity comes from specs/TELOS.md, read AT RUNTIME, so a card bump
# regenerates nothing (design: master/telos/research/per-repo-hook-design.md §3.3, §5).
#
# Invoked by the committed .claude/settings.json SessionStart hook:
#   sh "$CLAUDE_PROJECT_DIR"/scripts/telos_prime.sh
# Emits ~500-600 chars to stdout: the card version line, the first three §3
# boundary bullets (mechanical extraction, truncated), the read-order line, and
# the change-law one-liner. When the card is absent it says exactly
# "TELOS: NOT YET AUTHORED" — never silence, never a non-zero exit
# (a hook must never break session start). POSIX sh + POSIX tools only.
set -eu

root=${CLAUDE_PROJECT_DIR:-}
if [ -z "$root" ]; then
  script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" 2>/dev/null && pwd) || script_dir=''
  case "$script_dir" in
    */scripts) root=${script_dir%/scripts} ;;
    *) root=$(pwd) ;;
  esac
fi
card="$root/specs/TELOS.md"

read_order='Read specs/TELOS.md IN FULL before any diff; §5 points at the estate head (Platform SYSTEM-TELOS).'

if [ ! -f "$card" ]; then
  printf '%s\n' 'TELOS: NOT YET AUTHORED'
  printf '%s\n' "$read_order"
  exit 0
fi

version=$(awk 'NR==1 { if (match($0, /specs-version: *[0-9]+/)) { v = substr($0, RSTART, RLENGTH); sub(/^[^0-9]*/, "", v); print v }; exit }' "$card" 2>/dev/null) || version=''
updated=$(awk 'NR==1 { if (match($0, /updated: *[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/)) { u = substr($0, RSTART, RLENGTH); sub(/^updated: */, "", u); print u }; exit }' "$card" 2>/dev/null) || updated=''

if [ -n "$version" ]; then
  printf '%s\n' "TELOS card v${version}${updated:+ (updated ${updated})} — specs/TELOS.md (BINDING)."
else
  printf '%s\n' 'TELOS card present, header unparsed — specs/TELOS.md (BINDING); read it.'
fi

bullets=$(awk '
  /^## / { in3 = ($0 ~ /^## §3/) }
  in3 && /^- / {
    line = $0
    if (length(line) > 100) {
      line = substr(line, 1, 100)
      sub(/ [^ ]*$/, "", line)
      line = line " ..."
    }
    print line
    if (++n == 3) exit
  }' "$card" 2>/dev/null) || bullets=''

if [ -n "$bullets" ]; then
  printf '%s\n' '§3 boundaries — never do (first 3 of the card'\''s list):'
  printf '%s\n' "$bullets"
else
  printf '%s\n' '(§3 boundary bullets not mechanically extractable — the card is authoritative; read it.)'
fi

printf '%s\n' "$read_order"
printf '%s\n' 'The §4 change law binds every diff in this repo.'
exit 0
