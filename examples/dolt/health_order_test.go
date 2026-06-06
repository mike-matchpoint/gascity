package dolt_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoltHealthOrderIsDiagnosticOnly(t *testing.T) {
	root := repoRoot(t)
	orderPath := filepath.Join(root, "orders", "dolt-health.toml")
	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read dolt-health order: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, `exec = "gc dolt health --json | gc dolt health-check"`) {
		t.Fatalf("dolt-health order should run bounded health JSON, got:\n%s", text)
	}
	for _, forbidden := range []string{"gc dolt start", "gc dolt status"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("dolt-health order must not call %q directly:\n%s", forbidden, text)
		}
	}
}

func TestDoltCompactorOrderFailsWhenCompactNotApplicable(t *testing.T) {
	root := repoRoot(t)
	orderPath := filepath.Join(root, "orders", "mol-dog-compactor.toml")
	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read mol-dog-compactor order: %v", err)
	}

	text := string(data)
	want := `GC_DOLT_COMPACT_REQUIRE_APPLICABLE=1 gc dolt compact 2>&1 | gc dolt health-check --compact-result`
	if !strings.Contains(text, want) {
		t.Fatalf("compactor order should fail visibly on not_applicable compact, got:\n%s", text)
	}
}

func TestDoltHealthCheckFailsUnreachableReportWithUsefulMessage(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	input := `{
  "server": {
    "running": true,
    "reachable": false,
    "pid": 123,
    "port": 3311,
    "latency_ms": 0
  }
}`

	cmd := exec.Command("sh", script)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("health-check unexpectedly succeeded:\n%s", out)
	}
	for _, want := range []string{"Dolt server unreachable", "running=true", "pid=123", "port=3311"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("health-check output missing %q:\n%s", want, out)
		}
	}
}

func TestDoltHealthCheckPassesReachableReport(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	input := `{
  "server": {
    "running": true,
    "reachable": true,
    "pid": 123,
    "port": 3311,
    "latency_ms": 12
  }
}`

	cmd := exec.Command("sh", script)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("health-check failed: %v\n%s", err, out)
	}
}

func TestDoltHealthCheckFailsSelectOneThreshold(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	input := `{
  "server": {
    "running": true,
    "reachable": true,
    "pid": 123,
    "port": 3311,
    "latency_ms": 4500,
    "ping_latency_ms": 4500
  }
}`

	cmd := exec.Command("sh", script)
	cmd.Env = append(filteredEnv("GC_DOLT_HEALTHCHECK_MAX_SELECT_ONE_MS"), "GC_DOLT_HEALTHCHECK_MAX_SELECT_ONE_MS=1000")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("health-check unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "Dolt SELECT 1 latency 4500ms exceeded threshold 1000ms") {
		t.Fatalf("health-check threshold output missing SELECT latency failure:\n%s", out)
	}
}

func TestDoltHealthCheckFailsRepresentativeQueryAndBloatThresholds(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; skipping nested JSON threshold check")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	input := `{
  "server": {
    "running": true,
    "reachable": true,
    "pid": 123,
    "port": 3311,
    "latency_ms": 12,
    "ping_latency_ms": 12
  },
  "real_query": {
    "enabled": true,
    "ok": false,
    "timeout": true,
    "latency_ms": 2001,
    "exit_code": 124
  },
  "databases": [
    {"name": "hq", "commits": 412000, "open_beads": 17, "noms_bytes": 27917287424}
  ],
  "storage": {
    "noms_bytes_visible": true,
    "noms_bytes_total": 27917287424
  }
}`

	cmd := exec.Command("sh", script)
	cmd.Env = append(filteredEnv(
		"GC_DOLT_HEALTH_REAL_QUERY",
		"GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS",
		"GC_DOLT_HEALTHCHECK_MAX_COMMITS",
		"GC_DOLT_HEALTHCHECK_MAX_NOMS_BYTES",
	),
		"GC_DOLT_HEALTH_REAL_QUERY=1",
		"GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS=1000",
		"GC_DOLT_HEALTHCHECK_MAX_COMMITS=200000",
		"GC_DOLT_HEALTHCHECK_MAX_NOMS_BYTES=10737418240",
	)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("health-check unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "Dolt representative query failed: timeout=true exit_code=124") {
		t.Fatalf("health-check output missing representative query failure:\n%s", out)
	}
}

func TestDoltHealthCheckFailsSlowRepresentativeQuery(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	input := `{
  "server": {
    "running": true,
    "reachable": true,
    "pid": 123,
    "port": 3311,
    "latency_ms": 12,
    "ping_latency_ms": 12
  },
  "real_query": {
    "enabled": true,
    "ok": true,
    "timeout": false,
    "latency_ms": 4500,
    "bytes": 2
  }
}`

	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(), "GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS=3000")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("health-check unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "representative query latency") {
		t.Fatalf("health-check output missing representative query threshold:\n%s", out)
	}
}

func TestDoltHealthCheckFailsCompactNotApplicable(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "commands", "health-check", "run.sh")
	cmd := exec.Command("sh", script, "--compact-result")
	cmd.Stdin = strings.NewReader("compact: not_applicable reason=non_local_host\n")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("health-check compact-result unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "Dolt compact not applicable: reason=non_local_host") {
		t.Fatalf("health-check output missing compact not applicable reason:\n%s", out)
	}
}
