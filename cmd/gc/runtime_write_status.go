package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/doctor"
)

func buildCityRuntimeWriteSummary(cityPath string) *doctor.RuntimeWriteSummary {
	if cityPath == "" {
		return nil
	}
	tracePath := beads.RuntimeWriteTracePath(cityPath)
	summary, err := doctor.AnalyzeRuntimeWriteTrace(tracePath, doctor.RuntimeWriteOptions{})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return &doctor.RuntimeWriteSummary{TracePath: tracePath}
	}
	if summary.ScannedLines == 0 && summary.RecentDegraded == 0 && summary.HotRemoteCommands == 0 {
		return nil
	}
	return &summary
}

func renderRuntimeWriteBlock(w io.Writer, summary *doctor.RuntimeWriteSummary) {
	if summary == nil {
		return
	}
	fmt.Fprintln(w)                    //nolint:errcheck // best-effort stdout
	fmt.Fprintln(w, "Runtime writes:") //nolint:errcheck // best-effort stdout
	if summary.Healthy() {
		fmt.Fprintf(w, "  Status:      ok  (%d trace lines scanned)\n", summary.ScannedLines) //nolint:errcheck
	} else {
		fmt.Fprintf(w, "  Status:      warning  (%s)\n", runtimeWriteStatusReason(summary)) //nolint:errcheck
	}
	_, _ = fmt.Fprintf(w, "  Recent:      degraded=%d timeouts=%d hot_remote=%d\n",
		summary.RecentDegraded, summary.RecentTimeouts, summary.HotRemoteCommands)
	if len(summary.StoreKeys) > 0 {
		fmt.Fprintf(w, "  Store keys:  %s\n", strings.Join(summary.StoreKeys, ", ")) //nolint:errcheck
	}
	if summary.LastIssueAt != "" {
		fmt.Fprintf(w, "  Last issue:  %s\n", summary.LastIssueAt) //nolint:errcheck
	}
}

func runtimeWriteStatusReason(summary *doctor.RuntimeWriteSummary) string {
	var parts []string
	if summary.HotRemoteCommands > 0 {
		parts = append(parts, fmt.Sprintf("%d forbidden hot command", summary.HotRemoteCommands))
	}
	if summary.RecentDegraded > 0 {
		parts = append(parts, fmt.Sprintf("%d degraded", summary.RecentDegraded))
	}
	if summary.RecentTimeouts > 0 {
		parts = append(parts, fmt.Sprintf("%d timeout", summary.RecentTimeouts))
	}
	if len(parts) == 0 {
		return "threshold reached"
	}
	return strings.Join(parts, ", ")
}
