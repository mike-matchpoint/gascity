package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/nudgequeue"
	"github.com/gastownhall/gascity/internal/runtime"
)

// pingNudgeWakeSocketDialTimeout bounds how long a producer waits to dial
// the supervisor wake socket. Producers must not block on a stale or
// missing socket — legacy-mode cities and pre-start producers expect the
// dial to fail fast.
const pingNudgeWakeSocketDialTimeout = 200 * time.Millisecond

type legacyNudgePollerSweepResult struct {
	Signaled     int
	RemovedStale int
	Skipped      int
}

var (
	legacyNudgePollerProcessRunning = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
	legacyNudgePollerProcessArgs = readProcessArgs
	legacyNudgePollerSignal      = func(pid int) error {
		return syscall.Kill(pid, syscall.SIGTERM)
	}
)

// pingNudgeWakeSocket sends a best-effort wake signal to the supervisor's
// nudge dispatcher. Callers invoke this after enqueueing a queued nudge so
// the supervisor delivers within sub-second latency instead of waiting for
// the next patrol tick. Failures (no listener, dial timeout, write error)
// are intentionally silent: the patrol-tick fallback in supervisor mode
// and the per-session poller in legacy mode each guarantee eventual
// delivery without the wake.
func pingNudgeWakeSocket(cityPath string) {
	if cityPath == "" {
		return
	}
	path := nudgequeue.WakeSocketPath(cityPath)
	conn, err := net.DialTimeout("unix", path, pingNudgeWakeSocketDialTimeout)
	if err != nil {
		return
	}
	defer conn.Close() //nolint:errcheck // best-effort signaling
	_ = conn.SetWriteDeadline(time.Now().Add(pingNudgeWakeSocketDialTimeout))
	_, _ = conn.Write([]byte{1})
}

func retireLegacyNudgePollersForSupervisor(cityPath string, stderr io.Writer, logPrefix string) {
	result, err := sweepLegacyNudgePollersForSupervisor(cityPath)
	if err != nil {
		if stderr != nil {
			fmt.Fprintf(stderr, "%s: nudge dispatcher: legacy poller sweep: %v\n", logPrefix, err) //nolint:errcheck
		}
		return
	}
	if stderr != nil && (result.Signaled > 0 || result.RemovedStale > 0) {
		fmt.Fprintf(stderr, "%s: nudge dispatcher: retired legacy pollers signaled=%d stale_pidfiles=%d skipped=%d\n", logPrefix, result.Signaled, result.RemovedStale, result.Skipped) //nolint:errcheck
	}
}

func sweepLegacyNudgePollersForSupervisor(cityPath string) (legacyNudgePollerSweepResult, error) {
	var result legacyNudgePollerSweepResult
	if strings.TrimSpace(cityPath) == "" {
		return result, nil
	}
	pollerDir := citylayout.RuntimePath(cityPath, "nudges", "pollers")
	entries, err := os.ReadDir(pollerDir)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".pid" {
			continue
		}
		pidPath := filepath.Join(pollerDir, entry.Name())
		sessionName := strings.TrimSuffix(entry.Name(), ".pid")
		if err := withNudgePollerPIDLock(pidPath, func() error {
			pid, ok, err := readLegacyNudgePollerPID(pidPath)
			if err != nil {
				return err
			}
			if !ok || !legacyNudgePollerProcessRunning(pid) {
				_ = os.Remove(pidPath)
				result.RemovedStale++
				return nil
			}
			args, ok := legacyNudgePollerProcessArgs(pid)
			if !ok || !legacyNudgePollerArgsMatch(args, cityPath, sessionName) {
				result.Skipped++
				return nil
			}
			if err := legacyNudgePollerSignal(pid); err != nil && !errors.Is(err, syscall.ESRCH) {
				result.Skipped++
				return nil
			}
			_ = os.Remove(pidPath)
			result.Signaled++
			return nil
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func readLegacyNudgePollerPID(pidPath string) (int, bool, error) {
	data, err := os.ReadFile(pidPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	pidText := strings.TrimSpace(string(data))
	if pidText == "" {
		return 0, false, nil
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return 0, false, nil
	}
	return pid, true, nil
}

func readProcessArgs(pid int) ([]string, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	raw := strings.TrimRight(string(data), "\x00")
	if raw == "" {
		return nil, false
	}
	return strings.Split(raw, "\x00"), true
}

func legacyNudgePollerArgsMatch(args []string, cityPath, sessionName string) bool {
	if len(args) == 0 {
		return false
	}
	hasNudgePoll := false
	hasCity := false
	hasSession := false
	cleanCity := filepath.Clean(cityPath)
	for i, arg := range args {
		if arg == "nudge" && i+1 < len(args) && args[i+1] == "poll" {
			hasNudgePoll = true
		}
		switch {
		case arg == "--city" && i+1 < len(args):
			hasCity = filepath.Clean(args[i+1]) == cleanCity
		case strings.HasPrefix(arg, "--city="):
			hasCity = filepath.Clean(strings.TrimPrefix(arg, "--city=")) == cleanCity
		case arg == "--session" && i+1 < len(args):
			hasSession = args[i+1] == sessionName
		case strings.HasPrefix(arg, "--session="):
			hasSession = strings.TrimPrefix(arg, "--session=") == sessionName
		}
	}
	return hasNudgePoll && hasCity && hasSession
}

// startNudgeWakeListener opens the supervisor wake socket and spawns an
// accept loop that signals wakeCh on every connection. The returned
// listener is closed when ctx is canceled. Returns nil, nil when the
// socket cannot be opened (e.g. permission, path-too-long); callers fall
// back to patrol-interval dispatching.
func startNudgeWakeListener(ctx context.Context, cityPath string, wakeCh chan<- struct{}, stderr io.Writer, logPrefix string) (net.Listener, error) {
	path := nudgequeue.WakeSocketPath(cityPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating nudge wake dir: %w", err)
	}
	// A stale socket from a prior supervisor crash blocks Listen with
	// "address already in use". Removing it is safe because flock-based
	// queue access protects state; the socket carries no data of its own.
	_ = os.Remove(path)
	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on nudge wake socket: %w", err)
	}
	// TOCTOU: there is a narrow window between Listen and Chmod where
	// the socket exists at the umask-default permissions and a co-local
	// user could connect. Worst case is a spurious dispatch tick — the
	// socket carries a single signal byte with no payload or auth — so
	// this is acceptable for now. A future hardening pass could set
	// umask before Listen, or use platform-specific abstract namespace
	// sockets where supported.
	if err := os.Chmod(path, 0o600); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("chmod nudge wake socket: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = lis.Close()
	}()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if stderr != nil {
					fmt.Fprintf(stderr, "%s: nudge wake accept: %v\n", logPrefix, err) //nolint:errcheck
				}
				continue
			}
			// Drain whatever the producer sent (a single signal byte) and
			// close. The wake itself is the signal — payload is reserved
			// for future protocol extensions.
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			var buf [16]byte
			_, _ = conn.Read(buf[:])
			_ = conn.Close()
			select {
			case wakeCh <- struct{}{}:
			default:
				// Already-pending wake covers this enqueue; coalesced.
			}
		}
	}()
	return lis, nil
}

// dispatchAllQueuedNudges runs one supervisor-side dispatcher pass: scan
// the queue for pending agents, resolve each to a nudgeTarget via
// sessionBeads, and try delivery. Returns the number of targets that
// successfully delivered at least one item.
//
// This is a no-op when the dispatcher is configured for "legacy" mode —
// the per-session `gc nudge poll` processes own delivery in that case.
func dispatchAllQueuedNudges(cityPath string, cfg *config.City, store beads.Store, sp runtime.Provider, sessionBeads *sessionBeadSnapshot) (int, error) {
	if cfg == nil || sessionBeads == nil || cityPath == "" {
		return 0, nil
	}
	if !nudgeDispatcherIsSupervisor(cfg) {
		return 0, nil
	}
	state, err := nudgequeue.LoadState(cityPath)
	if err != nil {
		return 0, fmt.Errorf("loading nudge queue: %w", err)
	}
	if len(state.Pending) == 0 && len(state.InFlight) == 0 {
		return 0, nil
	}
	now := time.Now()
	pendingAgents := make(map[string]bool, len(state.Pending))
	for _, item := range state.Pending {
		if item.Agent == "" {
			continue
		}
		if !item.DeliverAfter.IsZero() && item.DeliverAfter.After(now) {
			continue
		}
		pendingAgents[item.Agent] = true
	}
	// In-flight items with expired leases are recoverable on the next
	// claim attempt. Including their agents lets us retry without waiting
	// for the patrol tick to discover them.
	for _, item := range state.InFlight {
		if item.Agent == "" {
			continue
		}
		if item.LeaseUntil.IsZero() || !item.LeaseUntil.Before(now) {
			continue
		}
		pendingAgents[item.Agent] = true
	}
	if len(pendingAgents) == 0 {
		return 0, nil
	}

	delivered := 0
	var firstErr error
	for _, b := range sessionBeads.Open() {
		target := resolveNudgeTargetFromSessionBead(cityPath, cfg, b)
		if target.sessionName == "" {
			continue
		}
		// ACP sessions also flow through this dispatcher. The inject-on-hook
		// drain path still catches deliveries when the agent receives external
		// prompts, but a warm-idle ACP session never fires its hook on its
		// own — queued patrol wisps would otherwise pile up forever. The
		// atomic queue claim in claimDueQueuedNudgesForTarget guarantees a
		// nudge is delivered exactly once across the dispatcher + drain paths.
		matched := false
		for _, key := range target.queueKeys() {
			if pendingAgents[key] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		obs, err := workerObserveNudgeTarget(target, store, sp)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !obs.Running {
			continue
		}
		ok, err := tryDeliverQueuedNudgesByPoller(target, store, sp, defaultNudgePollQuiescence, obs)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if ok {
			delivered++
		}
	}
	return delivered, firstErr
}
