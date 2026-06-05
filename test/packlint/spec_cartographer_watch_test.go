package packlint

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSpecCartographerWatchExactGuardStopsBroadIndexMiss(t *testing.T) {
	slingCalled := runSpecCartographerWatchHarness(t, "031-gascity-first-deploy-runtime-proof-and-cleanup", true)
	if slingCalled {
		t.Fatal("watcher called gc sling even though the exact source label exists")
	}
}

func TestSpecCartographerWatchStillSlingsUnplannedWorkOrder(t *testing.T) {
	slingCalled := runSpecCartographerWatchHarness(t, "999-unplanned-validation", false)
	if !slingCalled {
		t.Fatal("watcher did not call gc sling for an unplanned work order")
	}
}

func runSpecCartographerWatchHarness(t *testing.T, woID string, exactGuardHit bool) bool {
	t.Helper()

	root := repoRoot()
	script := filepath.Join(root, "examples", "gastown", "packs", "codegen-support", "assets", "scripts", "spec-cartographer-watch.sh")
	tmp := t.TempDir()
	rigPath := filepath.Join(tmp, "rig")
	woDir := filepath.Join(rigPath, "specs", "agent-work-orders")
	if err := os.MkdirAll(woDir, 0o755); err != nil {
		t.Fatalf("creating work-order dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(woDir, woID+".md"), []byte("test work order\n"), 0o644); err != nil {
		t.Fatalf("writing work-order file: %v", err)
	}

	marker := filepath.Join(tmp, "sling-called")
	cmd := exec.Command("bash", "-c", `
set -euo pipefail
gc() {
  if [ "$#" -ge 3 ] && [ "$1" = rig ] && [ "$2" = list ] && [ "$3" = --json ]; then
    printf '{"rigs":[{"name":"Rig","path":"%s","hq":false,"suspended":false}]}\n' "$TEST_RIG_PATH"
    return 0
  fi
  if [ "$#" -ge 2 ] && [ "$1" = work ] && [ "$2" = count ]; then
    printf '{"ok":true,"count":0}\n'
    return 0
  fi
  if [ "$#" -ge 1 ] && [ "$1" = sling ]; then
    : > "$TEST_SLING_MARKER"
    printf '{"root_bead_id":"new-root"}\n'
    return 0
  fi
  if [ "$#" -ge 5 ] && [ "$1" = --rig ] && [ "$3" = bd ] && [ "$4" = show ]; then
    return 1
  fi
  if [ "$#" -ge 5 ] && [ "$1" = --rig ] && [ "$3" = bd ] && [ "$4" = list ]; then
    joined=" $* "
    case "$joined" in
      *" --status=in_progress "*) printf '[]\n'; return 0 ;;
      *" --status=open "*) printf '[]\n'; return 0 ;;
      *" --label source:work-order:${WO_ID} "*)
        if [ "$EXACT_GUARD_HIT" = 1 ]; then
          printf '[{"id":"existing","labels":["source:work-order:'"$WO_ID"'"]}]\n'
        else
          printf '[]\n'
        fi
        return 0
        ;;
      *" --metadata-field work_order_id=${WO_ID} "*)
        if [ "$EXACT_GUARD_HIT" = 1 ]; then
          printf '[{"id":"existing-meta","metadata":{"work_order_id":"'"$WO_ID"'"}}]\n'
        else
          printf '[]\n'
        fi
        return 0
        ;;
      *) printf '[]\n'; return 0 ;;
    esac
  fi
  printf 'unexpected gc call: %s\n' "$*" >&2
  return 64
}
export -f gc
bash "$SCRIPT"
`)
	exactGuard := "0"
	if exactGuardHit {
		exactGuard = "1"
	}
	cmd.Env = append(os.Environ(),
		"SCRIPT="+script,
		"TEST_RIG_PATH="+rigPath,
		"TEST_SLING_MARKER="+marker,
		"WO_ID="+woID,
		"EXACT_GUARD_HIT="+exactGuard,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("spec-cartographer-watch harness failed: %v\n%s", err, out)
	}
	return fileExists(marker)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
