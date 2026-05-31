package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeRuntimeWriteTraceCountsRecentDegradationAndForbiddenCommands(t *testing.T) {
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	tracePath := filepath.Join(t.TempDir(), "runtime-write.trace")
	lines := []string{
		runtimeWriteTraceLine(now.Add(-30*time.Minute), "update", "bd:update", "update old", "failed", "city", "old failure"),
		runtimeWriteTraceLine(now.Add(-2*time.Minute), "update", "bd:update", "update recent", "failed", "city", "conflict"),
		runtimeWriteTraceLine(now.Add(-1*time.Minute), "close-all", "bd:close", "close recent", "ambiguous-timeout", "rig", "deadline exceeded"),
		runtimeWriteTraceLine(now.Add(-45*time.Second), "get", "bd:show", "show missing", "not-found", "city", "not found"),
		runtimeWriteTraceLine(now.Add(-30*time.Second), "ping", "bd:list", "list --json --limit 0", "success", "city", ""),
		runtimeWriteTraceLine(now.Add(-20*time.Second), "backup-check", "bd:remote", "dolt remote -v", "success", "city", ""),
	}
	if err := os.WriteFile(tracePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := AnalyzeRuntimeWriteTrace(tracePath, RuntimeWriteOptions{
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("AnalyzeRuntimeWriteTrace: %v", err)
	}
	if summary.ScannedLines != 6 {
		t.Fatalf("ScannedLines = %d, want 6", summary.ScannedLines)
	}
	if summary.RecentDegraded != 2 || summary.RecentTimeouts != 1 {
		t.Fatalf("recent summary = degraded:%d timeouts:%d", summary.RecentDegraded, summary.RecentTimeouts)
	}
	if summary.HotRemoteCommands != 1 {
		t.Fatalf("HotRemoteCommands = %d, want 1", summary.HotRemoteCommands)
	}
	if got := strings.Join(summary.StoreKeys, ","); got != "city,rig" {
		t.Fatalf("StoreKeys = %q, want city,rig", got)
	}
	if summary.Outcomes["success"] != 2 || summary.Outcomes["failed"] != 2 || summary.Outcomes["ambiguous-timeout"] != 1 || summary.Outcomes["not-found"] != 1 {
		t.Fatalf("Outcomes = %+v", summary.Outcomes)
	}
	if summary.Healthy() {
		t.Fatalf("summary reported healthy despite recent degradation and forbidden command")
	}
}

func TestRuntimeWriteCheckWarnsOnRecentDegradation(t *testing.T) {
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	tracePath := filepath.Join(t.TempDir(), "runtime-write.trace")
	line := runtimeWriteTraceLine(now.Add(-time.Minute), "update", "bd:update", "update recent", "ambiguous-timeout", "city", "deadline exceeded")
	if err := os.WriteFile(tracePath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check := NewRuntimeWriteCheck("")
	check.tracePath = tracePath
	check.now = func() time.Time { return now }

	result := check.Run(nil)
	if result.Status != StatusWarning {
		t.Fatalf("status = %v, want warning; result=%+v", result.Status, result)
	}
	if !strings.Contains(result.Message, "runtime writes degraded recently") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestRuntimeWriteCheckErrorsOnForbiddenHotCommands(t *testing.T) {
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	tracePath := filepath.Join(t.TempDir(), "runtime-write.trace")
	line := runtimeWriteTraceLine(now.Add(-time.Minute), "backup-check", "bd:remote", "backup.enabled=true backup_export", "success", "city", "")
	if err := os.WriteFile(tracePath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check := NewRuntimeWriteCheck("")
	check.tracePath = tracePath
	check.now = func() time.Time { return now }

	result := check.Run(nil)
	if result.Status != StatusError {
		t.Fatalf("status = %v, want error; result=%+v", result.Status, result)
	}
	if !strings.Contains(result.Message, "forbidden remote/backup") {
		t.Fatalf("message = %q", result.Message)
	}
}

func runtimeWriteTraceLine(ts time.Time, op, command, args, outcome, storeKey, errMsg string) string {
	return fmt.Sprintf(`%s runtime_write caller=test class=hot-state op=%s command=%s args=%q duration=2s timeout=1s outcome=%s store_key=%s err=%q`,
		ts.Format(time.RFC3339Nano), op, command, args, outcome, storeKey, errMsg)
}
