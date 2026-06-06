package dolt_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetentionSweepFailsOnExternalEndpoint(t *testing.T) {
	root := repoRoot(t)
	cityPath := t.TempDir()
	script := filepath.Join(root, "commands", "retention-sweep", "run.sh")

	cmd := exec.Command("sh", script, "--class", "operational_churn", "--older-than", "48h", "--single-commit", "--json")
	cmd.Env = append(filteredEnv(
		"GC_CITY_PATH",
		"GC_PACK_DIR",
		"GC_DOLT_DATA_DIR",
		"GC_DOLT_HOST",
		"GC_DOLT_PORT",
		"GC_DOLT_USER",
		"GC_DOLT_PASSWORD",
		"GC_DOLT_MANAGED_LOCAL",
		"PATH",
	),
		"GC_CITY_PATH="+cityPath,
		"GC_PACK_DIR="+root,
		"GC_DOLT_DATA_DIR="+filepath.Join(cityPath, ".beads", "dolt"),
		"GC_DOLT_HOST=dolt.city.svc.cluster.local",
		"GC_DOLT_PORT=3307",
		"GC_DOLT_USER=root",
		"GC_DOLT_PASSWORD=",
		"GC_DOLT_MANAGED_LOCAL=0",
		"PATH="+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("retention-sweep external unexpectedly succeeded:\n%s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("retention-sweep returned non-exit error: %v\n%s", err, out)
	}
	if code := exitErr.ExitCode(); code != 2 {
		t.Fatalf("exit code = %d, want 2\n%s", code, out)
	}
	if !strings.Contains(string(out), "retention-sweep: not_applicable reason=managed_local_false") {
		t.Fatalf("output missing structured not_applicable:\n%s", out)
	}
}

func TestRetentionSweepLocalManagedNoCandidatesDoesNotCommit(t *testing.T) {
	root := repoRoot(t)
	cityPath := t.TempDir()
	dataDir := filepath.Join(cityPath, ".beads", "dolt")
	if err := os.MkdirAll(filepath.Join(dataDir, "hq", ".dolt"), 0o755); err != nil {
		t.Fatalf("mkdir dolt db: %v", err)
	}
	fakeBin := t.TempDir()
	bdLog := filepath.Join(t.TempDir(), "bd.log")
	writeExecutable(t, filepath.Join(fakeBin, "bd"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GC_RETENTION_TEST_BD_LOG"
if [[ "${1:-}" == "sql" && "${2:-}" == "--json" ]]; then
  query="${3:-}"
  if [[ "$query" == *"information_schema.columns"* ]]; then
    printf '[{"c":32}]\n'
    exit 0
  fi
	  if [[ "$query" == *"AS issues"* ]]; then
	    printf '[{"issues":0,"wisps":0}]\n'
	    exit 0
	  fi
fi
printf 'unexpected fake bd args: %s\n' "$*" >&2
exit 64
`)

	script := filepath.Join(root, "commands", "retention-sweep", "run.sh")
	cmd := exec.Command("sh", script, "--class", "operational_churn", "--older-than", "48h", "--single-commit", "--json")
	cmd.Env = append(filteredEnv(
		"GC_CITY_PATH",
		"GC_PACK_DIR",
		"GC_DOLT_DATA_DIR",
		"GC_DOLT_HOST",
		"GC_DOLT_PORT",
		"GC_DOLT_USER",
		"GC_DOLT_PASSWORD",
		"GC_DOLT_MANAGED_LOCAL",
		"GC_RETENTION_TEST_BD_LOG",
		"PATH",
	),
		"GC_CITY_PATH="+cityPath,
		"GC_PACK_DIR="+root,
		"GC_DOLT_DATA_DIR="+dataDir,
		"GC_DOLT_HOST=127.0.0.1",
		"GC_DOLT_PORT=3307",
		"GC_DOLT_USER=root",
		"GC_DOLT_PASSWORD=",
		"GC_DOLT_MANAGED_LOCAL=1",
		"GC_RETENTION_TEST_BD_LOG="+bdLog,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("retention-sweep no-candidate run failed: %v\n%s", err, out)
	}
	var payload struct {
		Applied    bool `json:"applied"`
		Candidates struct {
			Total int `json:"total"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("retention-sweep output is not JSON: %v\n%s", err, out)
	}
	if payload.Applied || payload.Candidates.Total != 0 {
		t.Fatalf("retention-sweep no-candidate payload = %+v, output:\n%s", payload, out)
	}
	logData, err := os.ReadFile(bdLog)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	if strings.Contains(string(logData), "dolt commit") {
		t.Fatalf("no-candidate run must not commit:\n%s", logData)
	}
}

func TestRetentionSweepLocalManagedUsesOneCommit(t *testing.T) {
	root := repoRoot(t)
	cityPath := t.TempDir()
	dataDir := filepath.Join(cityPath, ".beads", "dolt")
	if err := os.MkdirAll(filepath.Join(dataDir, "hq", ".dolt"), 0o755); err != nil {
		t.Fatalf("mkdir dolt db: %v", err)
	}
	fakeBin := t.TempDir()
	bdLog := filepath.Join(t.TempDir(), "bd.log")
	sqlLog := filepath.Join(t.TempDir(), "sweep.sql")
	countState := filepath.Join(t.TempDir(), "count-state")
	writeExecutable(t, filepath.Join(fakeBin, "bd"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GC_RETENTION_TEST_BD_LOG"
if [[ "${1:-}" == "sql" && "${2:-}" == "--json" ]]; then
  query="${3:-}"
  if [[ "$query" == *"information_schema.columns"* ]]; then
    printf '[{"c":32}]\n'
    exit 0
  fi
  if [[ "$query" == *"AS issues"* ]]; then
    if [[ ! -f "$GC_RETENTION_TEST_COUNT_STATE" ]]; then
      printf used > "$GC_RETENTION_TEST_COUNT_STATE"
      printf '[{"issues":2,"wisps":1}]\n'
    else
      printf '[{"issues":0,"wisps":0}]\n'
    fi
    exit 0
  fi
fi
if [[ "${1:-}" == "sql" ]]; then
  printf '%s\n' "${2:-}" > "$GC_RETENTION_TEST_SQL_LOG"
  printf 'OK, 0 rows affected\n'
  exit 0
fi
if [[ "${1:-}" == "dolt" && "${2:-}" == "commit" ]]; then
  exit 0
fi
printf 'unexpected fake bd args: %s\n' "$*" >&2
exit 64
`)

	script := filepath.Join(root, "commands", "retention-sweep", "run.sh")
	cmd := exec.Command("sh", script, "--class", "operational_churn", "--older-than", "48h", "--single-commit", "--json")
	cmd.Env = append(filteredEnv(
		"GC_CITY_PATH",
		"GC_PACK_DIR",
		"GC_DOLT_DATA_DIR",
		"GC_DOLT_HOST",
		"GC_DOLT_PORT",
		"GC_DOLT_USER",
		"GC_DOLT_PASSWORD",
		"GC_DOLT_MANAGED_LOCAL",
		"GC_RETENTION_TEST_BD_LOG",
		"GC_RETENTION_TEST_SQL_LOG",
		"GC_RETENTION_TEST_COUNT_STATE",
		"PATH",
	),
		"GC_CITY_PATH="+cityPath,
		"GC_PACK_DIR="+root,
		"GC_DOLT_DATA_DIR="+dataDir,
		"GC_DOLT_HOST=127.0.0.1",
		"GC_DOLT_PORT=3307",
		"GC_DOLT_USER=root",
		"GC_DOLT_PASSWORD=",
		"GC_DOLT_MANAGED_LOCAL=1",
		"GC_RETENTION_TEST_BD_LOG="+bdLog,
		"GC_RETENTION_TEST_SQL_LOG="+sqlLog,
		"GC_RETENTION_TEST_COUNT_STATE="+countState,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("retention-sweep failed: %v\n%s", err, out)
	}
	var payload struct {
		Applied bool `json:"applied"`
		Deleted struct {
			Total int `json:"total"`
		} `json:"deleted"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("retention-sweep output is not JSON: %v\n%s", err, out)
	}
	if !payload.Applied || payload.Deleted.Total != 3 {
		t.Fatalf("retention-sweep JSON missing expected summary:\n%s", out)
	}
	bdLogData, err := os.ReadFile(bdLog)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	if count := strings.Count(string(bdLogData), "dolt commit"); count != 1 {
		t.Fatalf("retention-sweep must create exactly one bd dolt commit, got %d:\n%s", count, bdLogData)
	}
	sqlData, err := os.ReadFile(sqlLog)
	if err != nil {
		t.Fatalf("read SQL log: %v", err)
	}
	sql := string(sqlData)
	for _, want := range []string{
		"CREATE TEMPORARY TABLE gc_retention_sweep_issue_ids",
		"i.status = 'closed'",
		"w.status = 'closed'",
		"COALESCE(i.pinned, 0) = 0",
		"JSON_EXTRACT(COALESCE(w.metadata, JSON_OBJECT()), '$.\"gc.retention_class\"')",
		"NOT EXISTS (SELECT 1 FROM wisp_dependencies",
		"DELETE FROM labels",
		"DELETE FROM issues",
		"DELETE FROM wisp_labels",
		"DELETE FROM wisps",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("retention-sweep SQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(string(bdLogData), "bd delete") {
		t.Fatalf("retention-sweep must not shell out per bead:\n%s", bdLogData)
	}
}
