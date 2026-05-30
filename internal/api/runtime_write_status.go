package api

import (
	"errors"
	"os"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/doctor"
)

func (s *Server) statusRuntimeWrite() *StatusRuntimeWrite {
	if s == nil || s.state == nil {
		return nil
	}
	return statusRuntimeWriteForCityPath(s.state.CityPath())
}

func statusRuntimeWriteForCityPath(cityPath string) *StatusRuntimeWrite {
	if cityPath == "" {
		return nil
	}
	tracePath := beads.RuntimeWriteTracePath(cityPath)
	summary, err := doctor.AnalyzeRuntimeWriteTrace(tracePath, doctor.RuntimeWriteOptions{})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return &StatusRuntimeWrite{TracePath: tracePath}
	}
	if summary.ScannedLines == 0 && summary.RecentDegraded == 0 && summary.HotRemoteCommands == 0 {
		return nil
	}
	return statusRuntimeWriteFromDoctor(summary)
}

func statusRuntimeWriteFromDoctor(summary doctor.RuntimeWriteSummary) *StatusRuntimeWrite {
	outcomes := map[string]int(nil)
	if len(summary.Outcomes) > 0 {
		outcomes = make(map[string]int, len(summary.Outcomes))
		for key, value := range summary.Outcomes {
			outcomes[key] = value
		}
	}
	return &StatusRuntimeWrite{
		TracePath:         summary.TracePath,
		ScannedLines:      summary.ScannedLines,
		RecentDegraded:    summary.RecentDegraded,
		RecentTimeouts:    summary.RecentTimeouts,
		HotRemoteCommands: summary.HotRemoteCommands,
		FirstIssueAt:      summary.FirstIssueAt,
		LastIssueAt:       summary.LastIssueAt,
		StoreKeys:         append([]string(nil), summary.StoreKeys...),
		Outcomes:          outcomes,
	}
}
