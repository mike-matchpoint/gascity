package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/doctor"
)

func buildCityDoltContentionSummary(cityPath string) *doctor.DoltContentionSummary {
	if cityPath == "" {
		return nil
	}
	logPath := filepath.Join(cityPath, ".gc", "runtime", "packs", "dolt", "dolt.log")
	summary, err := doctor.AnalyzeDoltContentionLog(logPath, doctor.DoltContentionOptions{})
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return &doctor.DoltContentionSummary{LogPath: logPath}
		}
		summary = doctor.DoltContentionSummary{LogPath: logPath}
	}
	lease, err := doctor.ReadMaintenanceLease(cityPath, time.Now())
	if err == nil {
		summary.MaintenanceLease = lease
	}
	if summary.ScannedLines == 0 && summary.MaintenanceLease == nil {
		return nil
	}
	return &summary
}

func renderDoltContentionBlock(w io.Writer, summary *doctor.DoltContentionSummary) {
	if summary == nil {
		return
	}
	fmt.Fprintln(w)                     //nolint:errcheck // best-effort stdout
	fmt.Fprintln(w, "Dolt contention:") //nolint:errcheck // best-effort stdout
	if summary.Healthy() {
		fmt.Fprintf(w, "  Status:      ok  (%d lines after latest restart)\n", summary.ScannedLines) //nolint:errcheck
	} else {
		fmt.Fprintf(w, "  Status:      warning  (%s)\n", doltContentionStatusReason(summary)) //nolint:errcheck
	}
	_, _ = fmt.Fprintf(w, "  Signatures:  backup_export=%d disconnect=%d slow_queries=%d\n",
		summary.BackupExportWarnings, summary.DisconnectSignatureCount, summary.SlowQueryCount)
	if summary.MaxQueryDurationSeconds > 0 {
		fmt.Fprintf(w, "  Max query:   %.1fs\n", summary.MaxQueryDurationSeconds) //nolint:errcheck
	}
	if len(summary.Databases) > 0 {
		fmt.Fprintf(w, "  Databases:   %s\n", strings.Join(summary.Databases, ", ")) //nolint:errcheck
	}
	if lease := summary.MaintenanceLease; lease != nil {
		state := "stale"
		if lease.Active {
			state = "active"
		}
		fmt.Fprintf(w, "  Lease:       %s %s pid=%d expires_in=%ds\n", state, lease.Operation, lease.PID, lease.ExpiresInSeconds) //nolint:errcheck
	}
}

func doltContentionStatusReason(summary *doctor.DoltContentionSummary) string {
	var parts []string
	if summary.BackupExportWarnings > 0 {
		parts = append(parts, fmt.Sprintf("%d backup_export", summary.BackupExportWarnings))
	}
	if summary.DisconnectSignatureCount >= 2 {
		parts = append(parts, fmt.Sprintf("%d disconnect", summary.DisconnectSignatureCount))
	}
	if summary.SlowQueryCount > 0 {
		parts = append(parts, fmt.Sprintf("%d slow", summary.SlowQueryCount))
	}
	if summary.MaintenanceLease != nil && summary.MaintenanceLease.Stale {
		parts = append(parts, "stale lease")
	}
	if len(parts) == 0 {
		return "threshold reached"
	}
	return strings.Join(parts, ", ")
}
