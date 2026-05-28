package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeDoltContentionLogRecognizesIncidentSignaturesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dolt.log")
	data := strings.Join([]string{
		`time="2026-05-27T08:00:00-07:00" level=warning msg="error running query" connectionDb=hq error="backup 'backup_export' not found"`,
		`Starting server with Config HP="0.0.0.0:18860"|T="300000"|R="false"|L="warning"`,
		`time="2026-05-27T09:05:20-07:00" level=warning msg="error running query" connectTime="2026-05-27 09:05:07.747235 -0700 PDT m=+4328.903504126" connectionDb=vg connectionID=142194 error="write tcp 127.0.0.1:18860->127.0.0.1:60080: write: broken pipe\nWrite(packet) failed\nconn 142194" queryTime="2026-05-27 09:05:20.747235 -0700 PDT m=+4341.903504126"`,
		`time="2026-05-27T09:05:20-07:00" level=error msg="Error in the middle of a stream to client 142194: conn 142194: Write(packet) failed: write tcp 127.0.0.1:18860->127.0.0.1:60080: write: broken pipe"`,
		`time="2026-05-27T09:12:11-07:00" level=warning msg="nothing to commit"`,
		`time="2026-05-27T09:15:01-07:00" level=error msg="Error reading packet from client 1792: read tcp 127.0.0.1:18860->127.0.0.1:57696: use of closed network connection\nio.ReadFull(header size) failed"`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := AnalyzeDoltContentionLog(logPath, DoltContentionOptions{SlowQueryThreshold: time.Second})
	if err != nil {
		t.Fatalf("AnalyzeDoltContentionLog: %v", err)
	}
	if summary.BackupExportWarnings != 0 {
		t.Fatalf("pre-restart backup_export warnings counted: %d", summary.BackupExportWarnings)
	}
	if summary.BrokenPipeCount != 2 || summary.WritePacketFailedCount != 2 || summary.ClosedConnectionCount != 1 {
		t.Fatalf("disconnect counts = broken:%d write:%d closed:%d", summary.BrokenPipeCount, summary.WritePacketFailedCount, summary.ClosedConnectionCount)
	}
	if summary.SlowQueryCount != 1 || summary.MaxQueryDurationSeconds < 12 {
		t.Fatalf("slow query summary = count:%d max:%f", summary.SlowQueryCount, summary.MaxQueryDurationSeconds)
	}
	if got := strings.Join(summary.Databases, ","); got != "vg" {
		t.Fatalf("databases = %q, want vg", got)
	}
	if summary.Healthy() {
		t.Fatalf("summary reported healthy despite disconnect cluster")
	}
}

func TestAnalyzeDoltContentionLogIgnoresNothingToCommit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dolt.log")
	data := strings.Join([]string{
		`Starting server with Config HP="0.0.0.0:18860"|T="300000"|R="false"|L="warning"`,
		`time="2026-05-27T09:12:11-07:00" level=warning msg="nothing to commit"`,
		`time="2026-05-27T16:51:38-07:00" level=warning msg="error running query" connectTime="2026-05-27 16:51:33.941521 -0700 PDT m=+485.740775876" connectionDb=hq connectionID=17976 error="nothing to commit" queryTime="2026-05-27 16:51:38.046086 -0700 PDT m=+489.845308459"`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := AnalyzeDoltContentionLog(logPath, DoltContentionOptions{SlowQueryThreshold: time.Second})
	if err != nil {
		t.Fatalf("AnalyzeDoltContentionLog: %v", err)
	}
	if !summary.Healthy() {
		t.Fatalf("nothing-to-commit warning should not create contention: %+v", summary)
	}
	if summary.SlowQueryCount != 0 || summary.MaxQueryDurationSeconds != 0 {
		t.Fatalf("nothing-to-commit slow query summary = count:%d max:%f", summary.SlowQueryCount, summary.MaxQueryDurationSeconds)
	}
	if summary.FirstIssueAt != "" || summary.LastIssueAt != "" || len(summary.Databases) != 0 {
		t.Fatalf("nothing-to-commit should not create issue metadata: %+v", summary)
	}
}

func TestReadMaintenanceLeaseReportsActiveAndStale(t *testing.T) {
	cityPath := t.TempDir()
	now := time.Date(2026, 5, 27, 18, 0, 0, 0, time.UTC)
	writeDoctorLease(t, cityPath, os.Getpid(), "backup-sync", now.Add(-time.Minute), now.Add(time.Minute))
	lease, err := ReadMaintenanceLease(cityPath, now)
	if err != nil {
		t.Fatalf("ReadMaintenanceLease active: %v", err)
	}
	if lease == nil || !lease.Active || lease.Stale || lease.Operation != "backup-sync" {
		t.Fatalf("active lease = %+v", lease)
	}

	writeDoctorLease(t, cityPath, 99999999, "jsonl-export", now.Add(-2*time.Hour), now.Add(-time.Hour))
	lease, err = ReadMaintenanceLease(cityPath, now)
	if err != nil {
		t.Fatalf("ReadMaintenanceLease stale: %v", err)
	}
	if lease == nil || lease.Active || !lease.Stale {
		t.Fatalf("stale lease = %+v", lease)
	}
}

func TestDoltContentionCheckWarnsOnBackupExportAndStaleLease(t *testing.T) {
	cityPath := t.TempDir()
	logDir := filepath.Join(cityPath, ".gc", "runtime", "packs", "dolt")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "dolt.log")
	if err := os.WriteFile(logPath, []byte(`time="2026-05-27T10:00:00-07:00" level=warning msg="error running query" connectionDb=hq error="backup 'backup_export' already exists"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 27, 18, 0, 0, 0, time.UTC)
	writeDoctorLease(t, cityPath, 99999999, "broad-cleanup", now.Add(-2*time.Hour), now.Add(-time.Hour))
	check := NewDoltContentionCheck(cityPath)
	check.now = func() time.Time { return now }

	result := check.Run(nil)
	if result.Status != StatusWarning {
		t.Fatalf("status = %v, want warning; result=%+v", result.Status, result)
	}
	if !strings.Contains(result.Message, "backup_export") || !strings.Contains(result.Message, "stale maintenance lease") {
		t.Fatalf("message = %q", result.Message)
	}
}

func writeDoctorLease(t *testing.T, cityPath string, pid int, operation string, startedAt, expiresAt time.Time) {
	t.Helper()
	leaseDir := filepath.Join(cityPath, ".gc", "runtime", "maintenance")
	if err := os.MkdirAll(leaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{
  "owner": "test",
  "pid": %d,
  "operation": %q,
  "city_path": %q,
  "started_at": %q,
  "expires_at": %q,
  "deadline_seconds": 3600
}
`, pid, operation, cityPath, startedAt.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(leaseDir, "dolt-maintenance-lease.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
