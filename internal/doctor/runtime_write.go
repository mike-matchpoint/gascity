package doctor

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

const (
	defaultRuntimeWriteTailBytes    = int64(4 * 1024 * 1024)
	defaultRuntimeWriteRecentWindow = 10 * time.Minute
)

// RuntimeWriteOptions configures runtime-write trace parsing.
type RuntimeWriteOptions struct {
	TailBytes    int64
	RecentWindow time.Duration
	Now          func() time.Time
}

// RuntimeWriteSummary is a compact, JSON-serializable summary of bounded
// runtime Beads write degradation.
type RuntimeWriteSummary struct {
	TracePath         string         `json:"trace_path,omitempty"`
	ScannedLines      int            `json:"scanned_lines"`
	RecentDegraded    int            `json:"recent_degraded"`
	RecentTimeouts    int            `json:"recent_timeouts"`
	HotRemoteCommands int            `json:"hot_remote_commands"`
	FirstIssueAt      string         `json:"first_issue_at,omitempty"`
	LastIssueAt       string         `json:"last_issue_at,omitempty"`
	StoreKeys         []string       `json:"store_keys,omitempty"`
	Outcomes          map[string]int `json:"outcomes,omitempty"`
}

// Healthy reports whether the trace contains no active runtime write
// degradation and no forbidden hot-path remote/backup command signatures.
func (s RuntimeWriteSummary) Healthy() bool {
	return s.RecentDegraded == 0 && s.HotRemoteCommands == 0
}

// AnalyzeRuntimeWriteTrace parses recent runtime-write trace lines.
func AnalyzeRuntimeWriteTrace(path string, opts RuntimeWriteOptions) (RuntimeWriteSummary, error) {
	if opts.TailBytes <= 0 {
		opts.TailBytes = defaultRuntimeWriteTailBytes
	}
	if opts.RecentWindow <= 0 {
		opts.RecentWindow = defaultRuntimeWriteRecentWindow
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	current := now()
	if current.IsZero() {
		current = time.Now()
	}
	recentSince := current.Add(-opts.RecentWindow)

	lines, err := readDoltLogTail(path, opts.TailBytes)
	if err != nil {
		return RuntimeWriteSummary{TracePath: path}, err
	}
	summary := RuntimeWriteSummary{TracePath: path}
	outcomes := map[string]int{}
	storeKeys := map[string]struct{}{}
	for _, line := range lines {
		if !strings.Contains(line, "runtime_write") {
			continue
		}
		summary.ScannedLines++
		fields := parseRuntimeWriteTraceFields(line)
		ts := runtimeWriteTraceTimestamp(line)
		op := strings.TrimSpace(fields["op"])
		outcome := strings.TrimSpace(fields["outcome"])
		if outcome != "" {
			outcomes[outcome]++
		}
		storeKey := strings.TrimSpace(fields["store_key"])
		if runtimeWriteTraceHasForbiddenHotCommand(line) {
			summary.HotRemoteCommands++
			if storeKey != "" {
				storeKeys[storeKey] = struct{}{}
			}
			summary.markIssue(ts)
		}
		if runtimeWriteOutcomeCountsAsDegraded(op, outcome) && runtimeWriteTraceRecent(ts, recentSince) {
			summary.RecentDegraded++
			if outcome == string(beads.WriteOutcomeAmbiguousTimeout) {
				summary.RecentTimeouts++
			}
			if storeKey != "" {
				storeKeys[storeKey] = struct{}{}
			}
			summary.markIssue(ts)
		}
	}
	if len(outcomes) > 0 {
		summary.Outcomes = outcomes
	}
	for key := range storeKeys {
		summary.StoreKeys = append(summary.StoreKeys, key)
	}
	sort.Strings(summary.StoreKeys)
	return summary, nil
}

func runtimeWriteOutcomeCountsAsDegraded(op, outcome string) bool {
	outcome = strings.TrimSpace(outcome)
	if outcome == "" || outcome == "success" {
		return false
	}
	if outcome != string(beads.WriteOutcomeNotFound) {
		return true
	}
	switch strings.TrimSpace(op) {
	case "create", "update", "close", "close-all":
		return true
	default:
		return false
	}
}

func (s *RuntimeWriteSummary) markIssue(ts time.Time) {
	if ts.IsZero() {
		return
	}
	stamp := ts.UTC().Format(time.RFC3339)
	if s.FirstIssueAt == "" {
		s.FirstIssueAt = stamp
	}
	s.LastIssueAt = stamp
}

func parseRuntimeWriteTraceFields(line string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Fields(line) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, `"`)
	}
	return fields
}

func runtimeWriteTraceTimestamp(line string) time.Time {
	first, _, _ := strings.Cut(strings.TrimSpace(line), " ")
	if first == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, first)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func runtimeWriteTraceRecent(ts time.Time, recentSince time.Time) bool {
	if ts.IsZero() {
		return true
	}
	return !ts.Before(recentSince)
}

func runtimeWriteTraceHasForbiddenHotCommand(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "dolt remote -v") ||
		strings.Contains(lower, "remote -v") ||
		strings.Contains(lower, "backup.enabled=true") ||
		strings.Contains(lower, "backup_export")
}

// RuntimeWriteCheck reports bounded Beads runtime-write degradation.
type RuntimeWriteCheck struct {
	cityPath  string
	tracePath string
	now       func() time.Time
}

// NewRuntimeWriteCheck creates a doctor check for bounded runtime Beads writes.
func NewRuntimeWriteCheck(cityPath string) *RuntimeWriteCheck {
	return &RuntimeWriteCheck{cityPath: cityPath, now: time.Now}
}

// Name returns the check identifier.
func (c *RuntimeWriteCheck) Name() string { return "runtime-write" }

// Run checks the default runtime-write trace for degraded hot writes.
func (c *RuntimeWriteCheck) Run(ctx *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	cityPath := strings.TrimSpace(c.cityPath)
	if cityPath == "" && ctx != nil {
		cityPath = strings.TrimSpace(ctx.CityPath)
	}
	tracePath := strings.TrimSpace(c.tracePath)
	if tracePath == "" {
		tracePath = beads.RuntimeWriteTracePath(cityPath)
	}
	summary, err := AnalyzeRuntimeWriteTrace(tracePath, RuntimeWriteOptions{Now: c.now})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.Status = StatusOK
			r.Message = "runtime write trace not present"
			return r
		}
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("runtime write trace unreadable: %v", err)
		return r
	}
	r.Details = runtimeWriteCheckDetails(summary)
	switch {
	case summary.HotRemoteCommands > 0:
		r.Status = StatusError
		r.Message = fmt.Sprintf("hot runtime trace contains %d forbidden remote/backup command signature(s)", summary.HotRemoteCommands)
		r.FixHint = "remove hot-path Dolt remote/backup calls, rerun runtime validation, then rotate or archive the stale trace"
	case summary.RecentDegraded > 0:
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("runtime writes degraded recently: %d degraded, %d timeout", summary.RecentDegraded, summary.RecentTimeouts)
		r.FixHint = "inspect .gc/runtime/beads/runtime-write.trace and Dolt contention status before trusting scheduler freshness"
	default:
		r.Status = StatusOK
		r.Message = fmt.Sprintf("runtime writes healthy (%d trace lines scanned)", summary.ScannedLines)
	}
	return r
}

// CanFix returns false because runtime-write degradation needs operator
// investigation rather than mechanical remediation.
func (c *RuntimeWriteCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *RuntimeWriteCheck) Fix(_ *CheckContext) error { return nil }

func runtimeWriteCheckDetails(summary RuntimeWriteSummary) []string {
	var details []string
	if summary.TracePath != "" {
		details = append(details, "trace: "+summary.TracePath)
	}
	if len(summary.StoreKeys) > 0 {
		details = append(details, "store_keys: "+strings.Join(summary.StoreKeys, ", "))
	}
	if len(summary.Outcomes) > 0 {
		keys := make([]string, 0, len(summary.Outcomes))
		for key := range summary.Outcomes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, summary.Outcomes[key]))
		}
		details = append(details, "outcomes: "+strings.Join(parts, ", "))
	}
	return details
}
