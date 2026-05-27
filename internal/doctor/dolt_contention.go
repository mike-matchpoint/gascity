package doctor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	defaultDoltContentionTailBytes = int64(8 * 1024 * 1024)
	defaultDoltSlowQueryThreshold  = 3500 * time.Millisecond
)

var (
	doltLogTimestampRe  = regexp.MustCompile(`time="([^"]+)"`)
	doltLogDatabaseRe   = regexp.MustCompile(`connectionDb=([^ ]+)`)
	doltLogQueryTimesRe = regexp.MustCompile(`connectTime="([^"]+)".*queryTime="([^"]+)"`)
	doltLogMonoTimeRe   = regexp.MustCompile(`m=\+([0-9]+(?:\.[0-9]+)?)`)
)

// DoltContentionOptions configures Dolt log contention parsing.
type DoltContentionOptions struct {
	TailBytes          int64
	SlowQueryThreshold time.Duration
}

// DoltContentionSummary is a compact, JSON-serializable summary of contention
// signatures after the latest visible Dolt restart boundary.
type DoltContentionSummary struct {
	LogPath                  string                   `json:"log_path,omitempty"`
	ScannedLines             int                      `json:"scanned_lines"`
	RestartLine              int                      `json:"restart_line,omitempty"`
	BackupExportWarnings     int                      `json:"backup_export_warnings"`
	BrokenPipeCount          int                      `json:"broken_pipe_count"`
	WritePacketFailedCount   int                      `json:"write_packet_failed_count"`
	ReadPacketFailedCount    int                      `json:"read_packet_failed_count"`
	IOTimeoutCount           int                      `json:"io_timeout_count"`
	ClosedConnectionCount    int                      `json:"closed_connection_count"`
	DisconnectSignatureCount int                      `json:"disconnect_signature_count"`
	SlowQueryCount           int                      `json:"slow_query_count"`
	MaxQueryDurationSeconds  float64                  `json:"max_query_duration_s,omitempty"`
	FirstIssueAt             string                   `json:"first_issue_at,omitempty"`
	LastIssueAt              string                   `json:"last_issue_at,omitempty"`
	Databases                []string                 `json:"databases,omitempty"`
	MaintenanceLease         *MaintenanceLeaseSummary `json:"maintenance_lease,omitempty"`
}

// Healthy reports whether the summary contains no post-restart contention
// signatures and no stale maintenance lease.
func (s DoltContentionSummary) Healthy() bool {
	if s.BackupExportWarnings > 0 || s.SlowQueryCount > 0 || s.DisconnectSignatureCount >= 2 {
		return false
	}
	if s.MaintenanceLease != nil && s.MaintenanceLease.Stale {
		return false
	}
	return true
}

// MaintenanceLeaseSummary describes the managed Dolt maintenance lease file.
type MaintenanceLeaseSummary struct {
	Path             string `json:"path"`
	Owner            string `json:"owner,omitempty"`
	PID              int    `json:"pid,omitempty"`
	Operation        string `json:"operation,omitempty"`
	CityPath         string `json:"city_path,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	DeadlineSeconds  int    `json:"deadline_seconds,omitempty"`
	Active           bool   `json:"active"`
	Stale            bool   `json:"stale"`
	AgeSeconds       int64  `json:"age_s,omitempty"`
	ExpiresInSeconds int64  `json:"expires_in_s,omitempty"`
}

// AnalyzeDoltContentionLog parses recent Dolt log lines and summarizes
// contention signatures after the latest restart line in that recent window.
func AnalyzeDoltContentionLog(path string, opts DoltContentionOptions) (DoltContentionSummary, error) {
	if opts.TailBytes <= 0 {
		opts.TailBytes = defaultDoltContentionTailBytes
	}
	if opts.SlowQueryThreshold <= 0 {
		opts.SlowQueryThreshold = defaultDoltSlowQueryThreshold
	}
	lines, err := readDoltLogTail(path, opts.TailBytes)
	if err != nil {
		return DoltContentionSummary{LogPath: path}, err
	}
	summary := DoltContentionSummary{LogPath: path}
	start := 0
	for i, line := range lines {
		if strings.Contains(line, "Starting server with Config") {
			start = i + 1
			summary.RestartLine = i + 1
		}
	}
	dbs := map[string]struct{}{}
	for _, line := range lines[start:] {
		summary.ScannedLines++
		matched := false
		if strings.Contains(line, "backup 'backup_export'") {
			summary.BackupExportWarnings++
			matched = true
		}
		if strings.Contains(line, "broken pipe") {
			summary.BrokenPipeCount++
			matched = true
		}
		if strings.Contains(line, "Write(packet) failed") {
			summary.WritePacketFailedCount++
			matched = true
		}
		if strings.Contains(line, "Read(packet) failed") {
			summary.ReadPacketFailedCount++
			matched = true
		}
		if strings.Contains(line, "i/o timeout") {
			summary.IOTimeoutCount++
			matched = true
		}
		if strings.Contains(line, "use of closed network connection") {
			summary.ClosedConnectionCount++
			matched = true
		}
		if duration, ok := doltLogQueryDuration(line); ok && duration >= opts.SlowQueryThreshold {
			summary.SlowQueryCount++
			if seconds := duration.Seconds(); seconds > summary.MaxQueryDurationSeconds {
				summary.MaxQueryDurationSeconds = seconds
			}
			matched = true
		}
		if matched {
			if ts := doltLogTimestamp(line); ts != "" {
				if summary.FirstIssueAt == "" {
					summary.FirstIssueAt = ts
				}
				summary.LastIssueAt = ts
			}
			if db := doltLogDatabase(line); db != "" {
				dbs[db] = struct{}{}
			}
		}
	}
	summary.DisconnectSignatureCount = summary.BrokenPipeCount + summary.WritePacketFailedCount +
		summary.ReadPacketFailedCount + summary.IOTimeoutCount + summary.ClosedConnectionCount
	for db := range dbs {
		summary.Databases = append(summary.Databases, db)
	}
	sort.Strings(summary.Databases)
	return summary, nil
}

func readDoltLogTail(path string, maxBytes int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if info.Size() > maxBytes {
		offset = info.Size() - maxBytes
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		reader := bufio.NewReader(f)
		if _, err := reader.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return scanDoltLogLines(reader)
	}
	return scanDoltLogLines(f)
}

func scanDoltLogLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func doltLogTimestamp(line string) string {
	matches := doltLogTimestampRe.FindStringSubmatch(line)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func doltLogDatabase(line string) string {
	matches := doltLogDatabaseRe.FindStringSubmatch(line)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func doltLogQueryDuration(line string) (time.Duration, bool) {
	matches := doltLogQueryTimesRe.FindStringSubmatch(line)
	if len(matches) != 3 {
		return 0, false
	}
	startMono := doltLogMonoSeconds(matches[1])
	endMono := doltLogMonoSeconds(matches[2])
	if startMono >= 0 && endMono >= startMono {
		return time.Duration((endMono - startMono) * float64(time.Second)), true
	}
	start, okStart := parseDoltLogWallTime(matches[1])
	end, okEnd := parseDoltLogWallTime(matches[2])
	if !okStart || !okEnd || end.Before(start) {
		return 0, false
	}
	return end.Sub(start), true
}

func doltLogMonoSeconds(value string) float64 {
	matches := doltLogMonoTimeRe.FindStringSubmatch(value)
	if len(matches) != 2 {
		return -1
	}
	var seconds float64
	if _, err := fmt.Sscanf(matches[1], "%f", &seconds); err != nil {
		return -1
	}
	return seconds
}

func parseDoltLogWallTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " m=+"); idx >= 0 {
		value = value[:idx]
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
	} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// ReadMaintenanceLease reads the managed Dolt maintenance lease for a city.
func ReadMaintenanceLease(cityPath string, now time.Time) (*MaintenanceLeaseSummary, error) {
	if now.IsZero() {
		now = time.Now()
	}
	path := filepath.Join(cityPath, ".gc", "runtime", "maintenance", "dolt-maintenance-lease.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var raw struct {
		Owner           string `json:"owner"`
		PID             int    `json:"pid"`
		Operation       string `json:"operation"`
		CityPath        string `json:"city_path"`
		StartedAt       string `json:"started_at"`
		ExpiresAt       string `json:"expires_at"`
		DeadlineSeconds int    `json:"deadline_seconds"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	lease := &MaintenanceLeaseSummary{
		Path:            path,
		Owner:           raw.Owner,
		PID:             raw.PID,
		Operation:       raw.Operation,
		CityPath:        raw.CityPath,
		StartedAt:       raw.StartedAt,
		ExpiresAt:       raw.ExpiresAt,
		DeadlineSeconds: raw.DeadlineSeconds,
	}
	if started, err := time.Parse(time.RFC3339, raw.StartedAt); err == nil {
		lease.AgeSeconds = int64(now.Sub(started).Seconds())
	}
	if expires, err := time.Parse(time.RFC3339, raw.ExpiresAt); err == nil {
		lease.ExpiresInSeconds = int64(expires.Sub(now).Seconds())
		lease.Stale = !expires.After(now)
	}
	running := processRunning(raw.PID)
	lease.Active = !lease.Stale && running
	if !running {
		lease.Stale = true
	}
	return lease, nil
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// DoltContentionCheck reports recent Dolt contention signatures.
type DoltContentionCheck struct {
	cityPath string
	logPath  string
	now      func() time.Time
}

// NewDoltContentionCheck creates a check for the managed Dolt log.
func NewDoltContentionCheck(cityPath string) *DoltContentionCheck {
	return &DoltContentionCheck{
		cityPath: cityPath,
		logPath:  filepath.Join(cityPath, ".gc", "runtime", "packs", "dolt", "dolt.log"),
		now:      time.Now,
	}
}

// Name returns the stable doctor check identifier.
func (c *DoltContentionCheck) Name() string { return "dolt-contention" }

// Run executes the Dolt contention check.
func (c *DoltContentionCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	summary, err := AnalyzeDoltContentionLog(c.logPath, DoltContentionOptions{})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.Status = StatusOK
			r.Message = "dolt log not present"
			return r
		}
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("read dolt log: %v", err)
		return r
	}
	lease, err := ReadMaintenanceLease(c.cityPath, c.now())
	if err != nil {
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("read maintenance lease: %v", err)
		return r
	}
	summary.MaintenanceLease = lease
	r.Details = doltContentionDetails(summary)
	if summary.Healthy() {
		r.Status = StatusOK
		r.Message = fmt.Sprintf("no contention signatures after latest restart (%d lines scanned)", summary.ScannedLines)
		if summary.MaintenanceLease != nil && summary.MaintenanceLease.Active {
			r.Message += fmt.Sprintf("; maintenance lease active: %s", summary.MaintenanceLease.Operation)
		}
		return r
	}
	r.Status = StatusWarning
	r.Message = doltContentionWarningMessage(summary)
	r.FixHint = "inspect gc status, gc doctor -v, and .gc/runtime/packs/dolt/dolt.log; verify backup_export is removed and maintenance leases are not stale"
	return r
}

func doltContentionDetails(summary DoltContentionSummary) []string {
	details := []string{
		fmt.Sprintf("backup_export=%d disconnect=%d slow_queries=%d max_query_s=%.3f",
			summary.BackupExportWarnings, summary.DisconnectSignatureCount, summary.SlowQueryCount, summary.MaxQueryDurationSeconds),
	}
	if len(summary.Databases) > 0 {
		details = append(details, "databases="+strings.Join(summary.Databases, ","))
	}
	if summary.FirstIssueAt != "" || summary.LastIssueAt != "" {
		details = append(details, fmt.Sprintf("first_issue=%s last_issue=%s", summary.FirstIssueAt, summary.LastIssueAt))
	}
	if lease := summary.MaintenanceLease; lease != nil {
		state := "stale"
		if lease.Active {
			state = "active"
		}
		details = append(details, fmt.Sprintf("maintenance_lease=%s operation=%s owner=%s pid=%d expires_in_s=%d",
			state, lease.Operation, lease.Owner, lease.PID, lease.ExpiresInSeconds))
	}
	return details
}

func doltContentionWarningMessage(summary DoltContentionSummary) string {
	var parts []string
	if summary.BackupExportWarnings > 0 {
		parts = append(parts, fmt.Sprintf("%d backup_export warnings", summary.BackupExportWarnings))
	}
	if summary.DisconnectSignatureCount >= 2 {
		parts = append(parts, fmt.Sprintf("%d disconnect signatures", summary.DisconnectSignatureCount))
	}
	if summary.SlowQueryCount > 0 {
		parts = append(parts, fmt.Sprintf("%d slow query signatures (max %.1fs)", summary.SlowQueryCount, summary.MaxQueryDurationSeconds))
	}
	if summary.MaintenanceLease != nil && summary.MaintenanceLease.Stale {
		parts = append(parts, "stale maintenance lease")
	}
	if len(parts) == 0 {
		parts = append(parts, "contention signature threshold reached")
	}
	return "recent Dolt contention: " + strings.Join(parts, ", ")
}

// CanFix reports whether the check has an automatic repair path.
func (c *DoltContentionCheck) CanFix() bool { return false }

// Fix is intentionally a no-op because contention cleanup needs operator review.
func (c *DoltContentionCheck) Fix(_ *CheckContext) error { return nil }

// WarmupEligible reports whether the check should run during warmup.
func (c *DoltContentionCheck) WarmupEligible() bool { return false }
