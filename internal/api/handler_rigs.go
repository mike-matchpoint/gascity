package api

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/config"
	gitpkg "github.com/gastownhall/gascity/internal/git"
	"github.com/gastownhall/gascity/internal/runtime"
	workdirutil "github.com/gastownhall/gascity/internal/workdir"
	"github.com/gastownhall/gascity/internal/worker"
)

type rigResponse struct {
	Name          string     `json:"name"`
	Path          string     `json:"path"`
	Suspended     bool       `json:"suspended"`
	Prefix        string     `json:"prefix,omitempty"`
	DefaultBranch string     `json:"default_branch,omitempty"`
	AgentCount    int        `json:"agent_count"`
	RunningCount  int        `json:"running_count"`
	LastActivity  *time.Time `json:"last_activity,omitempty"`
	Git           *gitStatus `json:"git,omitempty"`
}

type gitStatus struct {
	Branch       string `json:"branch"`
	Clean        bool   `json:"clean"`
	ChangedFiles int    `json:"changed_files"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
}

// buildRigResponse creates a rigResponse with agent counts and last activity.
func (s *Server) buildRigResponse(cfg *config.City, rig config.Rig, sp runtime.Provider, cityName, cityPath string, inventory runtime.Inventory, hasInventory bool) rigResponse {
	tmpl := cfg.Workspace.SessionTemplate
	var agentCount, runningCount, suspendedCount int
	var maxActivity time.Time

	for _, a := range cfg.Agents {
		if workdirutil.ConfiguredRigName(cityPath, a, cfg.Rigs) != rig.Name {
			continue
		}
		var processNames []string
		if !hasInventory {
			processNames = config.AgentProcessNames(cfg, a, exec.LookPath)
		}
		expanded := expandAgent(a, cityName, tmpl, sp)
		for _, ea := range expanded {
			agentCount++
			sessionName := agent.SessionNameFor(cityName, ea.qualifiedName, tmpl)
			obs := observeRigProviderSession(sp, sessionName, processNames, inventory, hasInventory)
			if obs.Running {
				runningCount++
			}
			if a.Suspended || ea.suspended || obs.Suspended {
				suspendedCount++
			}
			if obs.LastActivity != nil && obs.LastActivity.After(maxActivity) {
				maxActivity = *obs.LastActivity
			}
		}
	}

	resp := rigResponse{
		Name:          rig.Name,
		Path:          rig.Path,
		Suspended:     rig.Suspended || (agentCount > 0 && suspendedCount == agentCount),
		Prefix:        rig.Prefix,
		DefaultBranch: rig.DefaultBranch,
		AgentCount:    agentCount,
		RunningCount:  runningCount,
	}
	if !maxActivity.IsZero() {
		resp.LastActivity = &maxActivity
	}
	return resp
}

func observeRigProviderSession(sp runtime.Provider, sessionName string, processNames []string, inventory runtime.Inventory, hasInventory bool) worker.LiveObservation {
	if hasInventory {
		obs, known := inventory.Observe(sessionName)
		if !known {
			return worker.LiveObservation{SessionName: sessionName}
		}
		out := worker.LiveObservation{
			SessionName: sessionName,
			Running:     obs.Running,
			Alive:       obs.Alive,
			Suspended:   obs.SuspendedKnown && obs.Suspended,
		}
		if obs.LastActivityKnown {
			last := obs.LastActivity
			out.LastActivity = &last
		}
		return out
	}
	return observeProviderSession(sp, sessionName, processNames)
}

// gitStatusTimeout bounds how long git operations can take per rig.
const gitStatusTimeout = 3 * time.Second

// fetchGitStatus uses internal/git to get branch/status/ahead-behind info.
// Returns nil on any error or timeout (rig may not be a git repo).
// The context-based timeout ensures that git subprocesses are killed on
// expiry, preventing goroutine and process leaks.
func fetchGitStatus(path string) *gitStatus {
	ctx, cancel := context.WithTimeout(context.Background(), gitStatusTimeout)
	defer cancel()
	return fetchGitStatusCtx(ctx, path)
}

func fetchGitStatusCtx(ctx context.Context, path string) *gitStatus {
	g := gitpkg.New(path)
	if !g.IsRepoCtx(ctx) {
		return nil
	}

	branch, err := g.CurrentBranchCtx(ctx)
	if err != nil {
		return nil
	}

	porcelain, err := g.StatusPorcelainCtx(ctx)
	if err != nil {
		return nil
	}

	var changedFiles int
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.TrimSpace(line) != "" {
			changedFiles++
		}
	}

	gs := &gitStatus{
		Branch:       branch,
		Clean:        changedFiles == 0,
		ChangedFiles: changedFiles,
	}

	// Ahead/behind (best-effort — fails if no upstream set).
	ahead, behind, err := g.AheadBehindCtx(ctx)
	if err == nil {
		gs.Ahead = ahead
		gs.Behind = behind
	}

	return gs
}
