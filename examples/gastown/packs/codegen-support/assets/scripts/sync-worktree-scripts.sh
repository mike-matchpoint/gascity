#!/bin/sh
# sync-worktree-scripts.sh — copy city scripts into an agent worktree.
#
# Usage: sync-worktree-scripts.sh <src-dir> <dst-dir>
#
# Mirrors *.sh files from <src-dir> into <dst-dir>, preserving the
# executable bit. The worktree's .gc/scripts/ is NOT tracked by the
# rig repo, so worktree-setup.sh's `git fetch + pull --rebase` does
# NOT propagate edits to scripts that are maintained at the city level
# (cartographer-inventory.sh, cartographer-ghost-sweep.sh, etc).
#
# Called from agent pre_start after worktree-setup.sh has ensured the
# worktree exists. Idempotent — skips files that already match the
# source byte-for-byte, so mtime stays stable on no-op syncs and
# `find -newer` checks behave predictably.

set -eu

SRC="${1:?usage: sync-worktree-scripts.sh <src-dir> <dst-dir>}"
DST="${2:?missing dst-dir}"

if [ ! -d "$SRC" ]; then
    echo "sync-worktree-scripts: src $SRC missing — skipping" >&2
    exit 0
fi

mkdir -p "$DST"

for SRC_FILE in "$SRC"/*.sh; do
    [ -f "$SRC_FILE" ] || continue
    BASENAME=$(basename "$SRC_FILE")
    DST_FILE="$DST/$BASENAME"
    if [ ! -f "$DST_FILE" ] || ! cmp -s "$SRC_FILE" "$DST_FILE"; then
        cp "$SRC_FILE" "$DST_FILE"
        chmod +x "$DST_FILE"
        echo "sync-worktree-scripts: updated $BASENAME" >&2
    fi
done
