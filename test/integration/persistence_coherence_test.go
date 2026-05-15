//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/doctor"
)

// Persistence-coherence reproducer suite.
//
// These tests reproduce the destructive auto-import behavior reported in
// gastownhall/gascity issues #2079, #2080, #2081, #2093, #2094, #2131 and
// the local runbook gastown-reconciler-assignee-eviction-bug.md. All seven
// reduce to one mechanism inside the bd CLI: every server-mode, non-read-only
// bd subprocess auto-imports .beads/issues.jsonl into Dolt as a blanket
// overwrite, with no merge or freshness check. Combined with the
// export.auto: false setting that gascity bakes into managed configs
// (commit a7921dc4 / PR #1965), JSONL goes stale and every subsequent bd
// write reverts any Dolt mutation that occurred since the last export.
//
// Each test performs a sequence of writes that should remain coherent in
// any sane store, then asserts on the final state. Today they all fail —
// they should pass once the destructive auto-import is suppressed.

var coherenceWorkspaceCounter atomic.Int64

// realBdRunner returns a beads.CommandRunner that resolves "bd" to the
// real bd binary (via realBDBinary, set in integration_test.go) rather
// than the file-shim used by the conformance suite. envOverrides is
// merged on top of the inherited environment so each test can mirror
// gascity's prod settings (BD_EXPORT_AUTO=false, BEADS_DOLT_AUTO_START=0).
//
// The runner logs one timing line for every bd subprocess. These tests are the
// guardrail for a future state-coherence fix, and the timing lines make any
// fix that adds expensive subprocesses or export work visible in `go test -v`
// output without baking in brittle machine-specific thresholds.
func realBdRunner(t testing.TB, envOverrides map[string]string) beads.CommandRunner {
	t.Helper()
	base := beads.ExecCommandRunnerWithEnv(envOverrides)
	return func(dir, name string, args ...string) ([]byte, error) {
		displayName := name
		if name == "bd" && realBDBinary != "" {
			name = realBDBinary
		}
		start := time.Now()
		out, err := base(dir, name, args...)
		logSubprocessTiming(t, append([]string{displayName}, args...), start, err)
		return out, err
	}
}

// coherenceWorkspace bundles the per-test bd workspace + the store handle
// configured to look like a gascity-managed city.
type coherenceWorkspace struct {
	dir    string
	prefix string
	port   string
	env    []string
	store  *beads.BdStore
}

// newCoherenceWorkspace builds a fresh bd workspace where bd manages its
// own embedded Dolt server (via `bd init --server`). The workspace starts
// in the "permissive" state with export.auto=true so initial writes
// populate .beads/issues.jsonl. After setup, callers must invoke
// freezeJSONLAndDisableAutoExport on the workspace to lock in the
// canonical gascity managed-city stance (export.auto=false plus
// BD_EXPORT_AUTO=false on every subsequent runner invocation). At that
// point JSONL is fixed at whatever state was last exported, and any
// Dolt-side mutation diverges from JSONL — recreating the conditions
// observed by Mike on vehicle-graph-city and HQ in #2079, #2080, #2093.
func newCoherenceWorkspace(t *testing.T, env []string, doltPort string) *coherenceWorkspace {
	t.Helper()

	n := coherenceWorkspaceCounter.Add(1)
	prefix := fmt.Sprintf("pc%d", n)
	wsRoot := t.TempDir()
	wsDir := filepath.Join(wsRoot, fmt.Sprintf("ws-%d", n))
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("creating workspace: %v", err)
	}

	gitCmd := exec.Command("git", "init", "--quiet")
	gitCmd.Dir = wsDir
	gitCmd.Env = env
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	runRealBDInit(t, env, wsDir, prefix, doltPort)
	configureCustomTypesReal(t, env, wsDir, doctor.RequiredCustomTypes)

	// Permissive runner: matches the bd default. Initial Create()s on this
	// runner auto-export to JSONL, giving us a populated baseline to freeze.
	permissive := realBdRunner(t, nil)

	return &coherenceWorkspace{
		dir:    wsDir,
		prefix: prefix,
		port:   doltPort,
		env:    env,
		store:  beads.NewBdStoreWithPrefix(wsDir, permissive, prefix),
	}
}

// freezeJSONLAndDisableAutoExport switches the workspace from the
// permissive setup mode to gascity's canonical managed-city stance:
//   - .beads/config.yaml has export.auto: false (persists across bd calls)
//   - every subsequent CommandRunner invocation also sets BD_EXPORT_AUTO=false
//   - JSONL is locked at the state captured by `bd export` here, so any
//     later mutation drifts JSONL out of sync with Dolt
//
// After this call, the destructive auto-import path is loaded: the next
// non-read-only bd write subprocess that reads the stale JSONL will overwrite
// Dolt with it.
func (ws *coherenceWorkspace) freezeJSONLAndDisableAutoExport(t *testing.T) {
	t.Helper()
	runRealBDExportAll(t, ws.env, ws.dir)
	runRealBDConfigSet(t, ws.env, ws.dir, "export.auto", "false")

	frozen := realBdRunner(t, map[string]string{
		"BD_EXPORT_AUTO": "false",
	})
	ws.store = beads.NewBdStoreWithPrefix(ws.dir, frozen, ws.prefix)
}

// runRealBDInit initializes a bd workspace against the real bd binary
// (not the file shim) so its data lands in Dolt and the auto-import code
// path is the one under test.
func runRealBDInit(t *testing.T, env []string, dir, prefix, port string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bdInitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, realBDBinary, "init", "--server",
		"--server-host", "127.0.0.1", "--server-port", port, "-p", prefix,
		"--skip-hooks", "--skip-agents")
	cmd.Dir = dir
	cmd.Env = env
	start := time.Now()
	out, err := cmd.CombinedOutput()
	logSubprocessTiming(t, []string{"bd", "init", "--server"}, start, errorsForTiming(ctx.Err(), err))
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bd init timed out: %s", out)
	}
	if err != nil {
		t.Fatalf("bd init: %v: %s", err, out)
	}
}

func runRealBDConfigSet(t *testing.T, env []string, dir, key, value string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bdInitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, realBDBinary, "config", "set", key, value)
	cmd.Dir = dir
	cmd.Env = env
	start := time.Now()
	out, err := cmd.CombinedOutput()
	logSubprocessTiming(t, []string{"bd", "config", "set", key, value}, start, errorsForTiming(ctx.Err(), err))
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bd config set %s=%s timed out: %s", key, value, out)
	}
	if err != nil {
		t.Fatalf("bd config set %s=%s: %v: %s", key, value, err, out)
	}
}

// runRealBDExportAll captures every issue (including infra beads) into
// .beads/issues.jsonl. Used to deliberately populate JSONL before the
// test freezes it, mirroring the production state where JSONL was
// populated by an earlier bd run and is now stale.
func runRealBDExportAll(t *testing.T, env []string, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bdInitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, realBDBinary, "export", "-o", ".beads/issues.jsonl", "--all")
	cmd.Dir = dir
	cmd.Env = env
	start := time.Now()
	out, err := cmd.CombinedOutput()
	logSubprocessTiming(t, []string{"bd", "export", "-o", ".beads/issues.jsonl", "--all"}, start, errorsForTiming(ctx.Err(), err))
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bd export timed out: %s", out)
	}
	if err != nil {
		t.Fatalf("bd export: %v: %s", err, out)
	}
}

func configureCustomTypesReal(t *testing.T, env []string, dir string, types []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bdInitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, realBDBinary, "config", "set", "types.custom", strings.Join(types, ","))
	cmd.Dir = dir
	cmd.Env = env
	start := time.Now()
	out, err := cmd.CombinedOutput()
	logSubprocessTiming(t, []string{"bd", "config", "set", "types.custom", strings.Join(types, ",")}, start, errorsForTiming(ctx.Err(), err))
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bd config set types.custom timed out: %s", out)
	}
	if err != nil {
		t.Fatalf("bd config set types.custom: %v: %s", err, out)
	}
}

// stringPtr returns a pointer to s. Helper for UpdateOpts fields.
func stringPtr(s string) *string { return &s }

func errorsForTiming(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func logSubprocessTiming(t testing.TB, cmd []string, start time.Time, err error) {
	t.Helper()
	status := "ok"
	if err != nil {
		status = "error"
	}
	t.Logf("timing: bd subprocess status=%s duration=%s cmd=%q",
		status, roundDuration(time.Since(start)), strings.Join(cmd, " "))
}

func roundDuration(d time.Duration) time.Duration {
	if d >= time.Second {
		return d.Round(10 * time.Millisecond)
	}
	if d >= time.Millisecond {
		return d.Round(time.Millisecond)
	}
	return d
}

func timedCreate(t testing.TB, store *beads.BdStore, name string, b beads.Bead) (beads.Bead, time.Duration) {
	t.Helper()
	start := time.Now()
	created, err := store.Create(b)
	elapsed := time.Since(start)
	logStoreTiming(t, name, elapsed, err)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return created, elapsed
}

func logStoreTiming(t testing.TB, name string, elapsed time.Duration, err error) {
	t.Helper()
	status := "ok"
	if err != nil {
		status = "error"
	}
	t.Logf("timing: store operation status=%s duration=%s op=%q",
		status, roundDuration(elapsed), name)
}

func logDurationSummary(t testing.TB, label string, durations []time.Duration) {
	t.Helper()
	if len(durations) == 0 {
		t.Logf("timing-summary: %s count=0", label)
		return
	}
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total time.Duration
	for _, d := range sorted {
		total += d
	}
	pick := func(percentile float64) time.Duration {
		if len(sorted) == 1 {
			return sorted[0]
		}
		idx := int(float64(len(sorted)-1) * percentile)
		return sorted[idx]
	}
	t.Logf("timing-summary: %s count=%d min=%s p50=%s p95=%s max=%s total=%s avg=%s",
		label,
		len(sorted),
		roundDuration(sorted[0]),
		roundDuration(pick(0.50)),
		roundDuration(pick(0.95)),
		roundDuration(sorted[len(sorted)-1]),
		roundDuration(total),
		roundDuration(total/time.Duration(len(sorted))),
	)
}

// dumpCoherenceState reads the bd-side view of beadID plus the on-disk
// JSONL fragment for it, returning a single multi-line string suitable
// for t.Logf. Use when an assertion fails so the diagnostic is captured
// in CI artifacts without re-running the failing test.
func dumpCoherenceState(ws *coherenceWorkspace, beadID string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "workspace=%s prefix=%s port=%s\n", ws.dir, ws.prefix, ws.port)

	if bead, err := ws.store.Get(beadID); err == nil {
		raw, _ := json.MarshalIndent(bead, "", "  ")
		fmt.Fprintf(&buf, "bd view of %s:\n%s\n", beadID, raw)
	} else {
		fmt.Fprintf(&buf, "bd view of %s: error: %v\n", beadID, err)
	}

	jsonlPath := filepath.Join(ws.dir, ".beads", "issues.jsonl")
	if data, err := os.ReadFile(jsonlPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, beadID) {
				fmt.Fprintf(&buf, "jsonl line: %s\n", line)
			}
		}
		if buf.Len() == 0 {
			fmt.Fprintf(&buf, "jsonl had no line for %s (file size=%d)\n", beadID, len(data))
		}
	} else {
		fmt.Fprintf(&buf, "reading jsonl: %v\n", err)
	}
	return buf.String()
}

// withCoherenceFixture runs fn against a fresh workspace sharing one
// Dolt server across all subtests of a top-level Test. Centralizing
// fixture wiring keeps each subtest focused on the write sequence.
func withCoherenceFixture(t *testing.T, fn func(ws *coherenceWorkspace)) {
	t.Helper()
	requireDoltIntegration(t)
	env := newIsolatedToolEnv(t, true)

	doltDataDir := filepath.Join(t.TempDir(), "dolt")
	port := startSharedDoltServer(t, env, doltDataDir)

	ws := newCoherenceWorkspace(t, env, port)
	fn(ws)
}

// TestPersistenceCoherence_CloseSurvivesSubsequentBdCall reproduces #2079:
// a close-status write lands in Dolt but the next bd write invocation reverts
// it via stale-JSONL auto-import. Captured in the wild by the refinery on
// vehicle-graph-city merging vg-o2mx.2 (issue #2079 comment, 2026-05-13).
func TestPersistenceCoherence_CloseSurvivesSubsequentBdCall(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{Title: "target bead"})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		if err := ws.store.Update(target.ID, beads.UpdateOpts{Status: stringPtr("closed")}); err != nil {
			t.Fatalf("close target: %v", err)
		}

		// Sanity: the close landed before we provoke the second bd call.
		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after close: %v", err)
		}
		if got.Status != "closed" {
			t.Fatalf("precondition: target.Status = %q, want %q", got.Status, "closed")
		}

		// Provoke: any subsequent bd write command triggers a stale-JSONL auto-import.
		if _, err := ws.store.Create(beads.Bead{Title: "decoy bead"}); err != nil {
			t.Fatalf("create decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy: %v", err)
		}
		if got.Status != "closed" {
			t.Errorf("target reverted to %q after subsequent bd call (#2079)\n%s",
				got.Status, dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_SetMetadataSurvivesSubsequentBdCall reproduces #2080
// and the #2093 metadata-wipe test: a metadata write to bead A is reverted
// by the next bd command on any bead. Captured live on HQ vgc-8xw via
// "probe_marker" (issue #2093 HQ reproduction, 2026-05-14).
func TestPersistenceCoherence_SetMetadataSurvivesSubsequentBdCall(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{Title: "metadata target"})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		marker := fmt.Sprintf("probe-%d", time.Now().UnixNano())
		if err := ws.store.SetMetadata(target.ID, "probe_marker", marker); err != nil {
			t.Fatalf("set metadata on target: %v", err)
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after set-metadata: %v", err)
		}
		if got.Metadata["probe_marker"] != marker {
			t.Fatalf("precondition: probe_marker = %q, want %q", got.Metadata["probe_marker"], marker)
		}

		// Trigger the destructive auto-import via an unrelated bd command.
		decoy, err := ws.store.Create(beads.Bead{Title: "decoy"})
		if err != nil {
			t.Fatalf("create decoy: %v", err)
		}
		if err := ws.store.SetMetadata(decoy.ID, "probe_other", "x"); err != nil {
			t.Fatalf("set metadata on decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy write: %v", err)
		}
		if got.Metadata["probe_marker"] != marker {
			t.Errorf("probe_marker on %s = %q, want %q after subsequent bd writes (#2080/#2093)\n%s",
				target.ID, got.Metadata["probe_marker"], marker, dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_AssigneeSurvivesSubsequentBdCall reproduces the
// runbook bug (gastown-reconciler-assignee-eviction-bug.md) plus #2080.
// A polecat claim writes assignee=<id>; the next bd command from any
// other agent reverts it to null. Captured live on vg-dfehn 2026-05-14
// 23:40Z with rejection_reason=null (Observation 4 in the runbook —
// the clean reproducer with no refinery in the loop).
func TestPersistenceCoherence_AssigneeSurvivesSubsequentBdCall(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{Title: "claim target"})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		assignee := "test-rig/test.polecat-vgc-abc12"
		if err := ws.store.Update(target.ID, beads.UpdateOpts{
			Status:   stringPtr("in_progress"),
			Assignee: stringPtr(assignee),
		}); err != nil {
			t.Fatalf("claim target: %v", err)
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after claim: %v", err)
		}
		if got.Assignee != assignee {
			t.Fatalf("precondition: assignee = %q, want %q", got.Assignee, assignee)
		}

		// Provoke the auto-import via an unrelated write.
		if _, err := ws.store.Create(beads.Bead{Title: "decoy"}); err != nil {
			t.Fatalf("create decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy: %v", err)
		}
		if got.Assignee != assignee {
			t.Errorf("assignee on %s = %q, want %q (runbook reconciler eviction reproducer)\n%s",
				target.ID, got.Assignee, assignee, dumpCoherenceState(ws, target.ID))
		}
		if got.Status != "in_progress" {
			t.Errorf("status on %s = %q, want %q (status co-revert with assignee, runbook obs 1)\n%s",
				target.ID, got.Status, "in_progress", dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_StateAwakeMetadataPersistsAcrossBdCalls
// reproduces #2094: the named-session auto-wake path writes
// metadata.state=awake plus last_woke_at via bd update, and the next bd
// command reverts both fields back to state=asleep. The reconciler reads
// the reverted state every tick and concludes "still asleep, do nothing"
// indefinitely. HQ reproduction: vgc-8xw in issue #2093 Test 2.
func TestPersistenceCoherence_StateAwakeMetadataPersistsAcrossBdCalls(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{
			Title:    "session bead",
			Metadata: map[string]string{"state": "asleep"},
		})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		wokeAt := time.Now().UTC().Format(time.RFC3339)
		if err := ws.store.SetMetadataBatch(target.ID, map[string]string{
			"state":        "awake",
			"last_woke_at": wokeAt,
			"wake_reason":  "test",
		}); err != nil {
			t.Fatalf("set wake metadata: %v", err)
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after wake: %v", err)
		}
		if got.Metadata["state"] != "awake" {
			t.Fatalf("precondition: state = %q, want %q", got.Metadata["state"], "awake")
		}

		// Any other bd write command — the same shape the reconciler emits on
		// the next tick — reverts the wake.
		if _, err := ws.store.Create(beads.Bead{Title: "decoy"}); err != nil {
			t.Fatalf("create decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy: %v", err)
		}
		if got.Metadata["state"] != "awake" {
			t.Errorf("state on %s = %q, want %q after subsequent bd call (#2094)\n%s",
				target.ID, got.Metadata["state"], "awake", dumpCoherenceState(ws, target.ID))
		}
		if got.Metadata["last_woke_at"] != wokeAt {
			t.Errorf("last_woke_at on %s = %q, want %q (#2094)\n%s",
				target.ID, got.Metadata["last_woke_at"], wokeAt, dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_RoutedToMetadataPersistsAfterHandoff reproduces
// the polecat→refinery handoff scenario from the runbook (Observation 2)
// and #2080: the polecat sets gc.routed_to=<refinery> as part of the
// done sequence, and the next bd command reverts it back to <polecat>.
// Live capture: vg-7wwnq 23:24:57Z (runbook Observation 3.5).
func TestPersistenceCoherence_RoutedToMetadataPersistsAfterHandoff(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{
			Title:    "handoff target",
			Metadata: map[string]string{"gc.routed_to": "test-rig/test.polecat"},
		})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		if err := ws.store.SetMetadata(target.ID, "gc.routed_to", "test-rig/test.refinery"); err != nil {
			t.Fatalf("set handoff routed_to: %v", err)
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after handoff: %v", err)
		}
		if got.Metadata["gc.routed_to"] != "test-rig/test.refinery" {
			t.Fatalf("precondition: routed_to = %q, want %q",
				got.Metadata["gc.routed_to"], "test-rig/test.refinery")
		}

		if _, err := ws.store.Create(beads.Bead{Title: "decoy"}); err != nil {
			t.Fatalf("create decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy: %v", err)
		}
		if got.Metadata["gc.routed_to"] != "test-rig/test.refinery" {
			t.Errorf("gc.routed_to on %s = %q, want %q after handoff (runbook obs 3.5 / #2080 saitoc)\n%s",
				target.ID, got.Metadata["gc.routed_to"], "test-rig/test.refinery",
				dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_StatusInProgressSurvivesAcrossBdCalls reproduces
// runbook Observation 1: cartographer step beads flip
// in_progress → open every ~60s because the next bd command after a
// status write reverts it. The cartographer re-claims, the cycle
// repeats. ~10m wasted wall-time per cartographer run.
func TestPersistenceCoherence_StatusInProgressSurvivesAcrossBdCalls(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{Title: "step bead"})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		if err := ws.store.Update(target.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("claim step: %v", err)
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after claim: %v", err)
		}
		if got.Status != "in_progress" {
			t.Fatalf("precondition: status = %q, want %q", got.Status, "in_progress")
		}

		if _, err := ws.store.Create(beads.Bead{Title: "decoy"}); err != nil {
			t.Fatalf("create decoy: %v", err)
		}

		got, err = ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after decoy: %v", err)
		}
		if got.Status != "in_progress" {
			t.Errorf("status on %s = %q, want %q (runbook obs 1 / #2081 stale-view sweep)\n%s",
				target.ID, got.Status, "in_progress", dumpCoherenceState(ws, target.ID))
		}
	})
}

// TestPersistenceCoherence_MultipleSequentialWritesAllSurvive reproduces
// the cascade observed in #2131 and the runbook reconciler chain: when
// many writes happen in sequence (controller close → agent update →
// controller close → ...), each one is reverted by the next, and the
// final state reflects only what was already in stale JSONL.
//
// This is the most damaging variant in practice because order-tracking,
// session-state, and pool-assignment are all multi-write flows where
// even one in-the-middle revert wedges the whole sequence.
func TestPersistenceCoherence_MultipleSequentialWritesAllSurvive(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		target, err := ws.store.Create(beads.Bead{Title: "cascade target"})
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		// Sequence: each write should land and the cumulative state should
		// reflect all of them. In the broken regime, only the LAST write
		// shows up — every preceding one is reverted by the next bd call.
		steps := []struct {
			key, value string
		}{
			{"step_1", "claimed"},
			{"step_2", "branch_pushed"},
			{"step_3", "pr_opened"},
			{"step_4", "review_passed"},
			{"step_5", "merged"},
		}
		for _, step := range steps {
			if err := ws.store.SetMetadata(target.ID, step.key, step.value); err != nil {
				t.Fatalf("set %s: %v", step.key, err)
			}
		}

		got, err := ws.store.Get(target.ID)
		if err != nil {
			t.Fatalf("get after cascade: %v", err)
		}
		for _, step := range steps {
			if got.Metadata[step.key] != step.value {
				t.Errorf("metadata[%s] = %q, want %q (cascade write revert, #2131)\n%s",
					step.key, got.Metadata[step.key], step.value,
					dumpCoherenceState(ws, target.ID))
			}
		}
	})
}

// TestPersistenceCoherence_ConcurrentBeadsAllPersist proves the bug is
// not bead-isolated: writes on bead A get reverted by writes on bead B
// and vice versa. This is the core observation from #2093's HQ
// reproduction — auto-import overwrites ALL rows from stale JSONL, not
// just the row being modified.
func TestPersistenceCoherence_ConcurrentBeadsAllPersist(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		a, err := ws.store.Create(beads.Bead{Title: "bead A"})
		if err != nil {
			t.Fatalf("create A: %v", err)
		}
		b, err := ws.store.Create(beads.Bead{Title: "bead B"})
		if err != nil {
			t.Fatalf("create B: %v", err)
		}
		ws.freezeJSONLAndDisableAutoExport(t)

		if err := ws.store.SetMetadata(a.ID, "marker", "A-payload"); err != nil {
			t.Fatalf("set A marker: %v", err)
		}
		if err := ws.store.SetMetadata(b.ID, "marker", "B-payload"); err != nil {
			t.Fatalf("set B marker (provokes auto-import that wipes A): %v", err)
		}
		if err := ws.store.SetMetadata(a.ID, "marker2", "A-payload-2"); err != nil {
			t.Fatalf("re-set A marker (provokes auto-import that wipes B): %v", err)
		}

		gotA, err := ws.store.Get(a.ID)
		if err != nil {
			t.Fatalf("get A: %v", err)
		}
		gotB, err := ws.store.Get(b.ID)
		if err != nil {
			t.Fatalf("get B: %v", err)
		}

		if gotA.Metadata["marker"] != "A-payload" {
			t.Errorf("A.marker = %q, want %q (cross-bead auto-import revert, #2093)\n%s",
				gotA.Metadata["marker"], "A-payload", dumpCoherenceState(ws, a.ID))
		}
		if gotA.Metadata["marker2"] != "A-payload-2" {
			t.Errorf("A.marker2 = %q, want %q\n%s",
				gotA.Metadata["marker2"], "A-payload-2", dumpCoherenceState(ws, a.ID))
		}
		if gotB.Metadata["marker"] != "B-payload" {
			t.Errorf("B.marker = %q, want %q (cross-bead auto-import revert, #2093)\n%s",
				gotB.Metadata["marker"], "B-payload", dumpCoherenceState(ws, b.ID))
		}
	})
}

// TestPersistenceCoherence_ConcurrentWriteTimingVisibility does not assert a
// wall-clock threshold. Its job is to make the cost profile of a 25-agent-style
// write burst visible while the coherence fix is being developed. Future fixes
// that add export subprocesses, async waits, or locking should leave clear
// timing evidence here before we promote them into broader concurrent tests.
func TestPersistenceCoherence_ConcurrentWriteTimingVisibility(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		const workers = 25
		beadsByWorker := make([]beads.Bead, workers)
		createDurations := make([]time.Duration, workers)
		for i := 0; i < workers; i++ {
			created, elapsed := timedCreate(t, ws.store, fmt.Sprintf("setup-create-%02d", i), beads.Bead{
				Title: fmt.Sprintf("timing target %02d", i),
			})
			beadsByWorker[i] = created
			createDurations[i] = elapsed
		}
		logDurationSummary(t, "timing setup creates", createDurations)

		ws.freezeJSONLAndDisableAutoExport(t)

		var wg sync.WaitGroup
		start := make(chan struct{})
		durations := make([]time.Duration, workers)
		errs := make([]error, workers)
		for i := 0; i < workers; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				opName := fmt.Sprintf("concurrent-set-metadata-%02d", i)
				begin := time.Now()
				errs[i] = ws.store.SetMetadata(beadsByWorker[i].ID, "timing_worker", fmt.Sprintf("%02d", i))
				durations[i] = time.Since(begin)
				logStoreTiming(t, opName, durations[i], errs[i])
			}()
		}

		burstStart := time.Now()
		close(start)
		wg.Wait()
		t.Logf("timing-summary: concurrent write burst workers=%d wall=%s", workers, roundDuration(time.Since(burstStart)))
		logDurationSummary(t, "concurrent SetMetadata operations", durations)

		for i, err := range errs {
			if err != nil {
				t.Errorf("worker %02d SetMetadata failed: %v", i, err)
			}
		}
	})
}
