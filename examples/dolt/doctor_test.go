package dolt_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// doctorCheckScript is the on-disk path to the dolt doctor check.
// The dolt pack wraps each doctor check in its own directory with a
// `run.sh` entry point (and a sibling `doctor.toml` descriptor).
const doctorCheckScript = "doctor/check-dolt/run.sh"

// shellQuote wraps s in single quotes, escaping any inner single
// quotes. The result is safe to splice into a /bin/sh argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}

// strPtr returns a pointer to a string literal — used so a nil
// `dolt` field can express "no shim at all" distinctly from "shim
// emits empty version".
func strPtr(s string) *string { return &s }

// lookPathInto looks up host on the host's PATH and, if found,
// symlinks it into bin under the name linkName. Returns true on
// success so callers can chain alternatives.
func lookPathInto(t *testing.T, bin, host, linkName string) bool {
	t.Helper()
	hostPath, err := exec.LookPath(host)
	if err != nil {
		return false
	}
	if err := os.Symlink(hostPath, filepath.Join(bin, linkName)); err != nil {
		t.Fatalf("symlink %q -> %q: %v", host, linkName, err)
	}
	return true
}

// doctorSandboxOpts configures the test sandbox for runDoctorCheck.
//
//	dolt == nil          → no dolt binary on PATH (simulates the
//	                       missing-binary branch at the top of run.sh).
//	dolt != nil          → install a shim whose `dolt version` first
//	                       line is the pointed-to string.
//	includeFlock / Lsof  → install (or omit) flock / lsof shims.
type doctorSandboxOpts struct {
	dolt         *string
	includeFlock bool
	includeLsof  bool
}

// doctorSandbox builds an isolated PATH directory for run.sh.
//
// The script invokes head, sed, and a timeout binary
// (timeout/gtimeout) externally. Because the sandbox replaces PATH
// wholesale (rather than prepending), we symlink real coreutils into
// the sandbox so those calls still succeed; otherwise PATH isolation
// would break the script before it reaches the logic under test.
// dolt / flock / lsof are controlled per-test via opts so we can
// toggle each missing-binary branch independently of the host's
// installed tools.
func doctorSandbox(t *testing.T, opts doctorSandboxOpts) string {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range []string{"cat", "head", "mktemp", "rm", "sed", "sleep"} {
		hostPath, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("LookPath(%q): %v", tool, err)
		}
		if err := os.Symlink(hostPath, filepath.Join(bin, tool)); err != nil {
			t.Fatalf("symlink %q: %v", tool, err)
		}
	}
	// run.sh wraps `dolt version` in run_bounded, which prefers
	// gtimeout, then timeout. Symlink whichever is on the host as
	// `timeout` in the sandbox so the bounded path is exercised.
	// macOS without coreutils ships neither binary; fall back to
	// python3, which run_bounded handles last. Skip if none of the
	// three are available — the script's behavior is unobservable.
	switch {
	case lookPathInto(t, bin, "timeout", "timeout"):
	case lookPathInto(t, bin, "gtimeout", "timeout"):
	case lookPathInto(t, bin, "python3", "python3"):
	default:
		t.Skip("neither timeout, gtimeout, nor python3 installed; cannot exercise run_bounded")
	}
	if opts.dolt != nil {
		writeExecutable(t, filepath.Join(bin, "dolt"), fmt.Sprintf(
			"#!/bin/sh\n[ \"$1\" = \"version\" ] && echo %s\nexit 0\n",
			shellQuote(*opts.dolt),
		))
	}
	if opts.includeFlock {
		writeExecutable(t, filepath.Join(bin, "flock"), "#!/bin/sh\nexit 0\n")
	}
	if opts.includeLsof {
		writeExecutable(t, filepath.Join(bin, "lsof"), "#!/bin/sh\nexit 0\n")
	}
	return bin
}

// runDoctorCheck invokes doctor/check-dolt/run.sh with PATH set to
// the provided sandbox. Returns the exit code and the combined
// stdout+stderr (the script writes its diagnostics to stdout, but
// catching both is robust against a future refactor that splits
// streams).
func runDoctorCheck(t *testing.T, sandboxBin string, extraEnv ...string) (int, string) {
	t.Helper()
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, doctorCheckScript))
	cmd.Env = append(filteredEnv(
		"PATH",
		"GC_BIN",
		"GC_DOLT_HOST",
		"GC_DOLT_PORT",
		"GC_DOLT_DOCTOR_HEALTH",
		"GC_DOLT_DOCTOR_HEALTH_TIMEOUT_SECS",
		"GC_DOLT_HEALTHCHECK_MAX_COMMITS",
		"GC_DOLT_HEALTHCHECK_MAX_NOMS_BYTES",
		"GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS",
		"GC_DOLT_HEALTHCHECK_MAX_SELECT_ONE_MS",
		"GC_DOLT_HEALTH_REAL_QUERY",
	), "PATH="+sandboxBin)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("running %s: %v\noutput:\n%s", doctorCheckScript, err, out)
	return 0, ""
}

// TestDoctorCheckVersionFloor exercises the dolt >= 1.86.2
// version-gate added in ga-iwec. The gate guards against an
// upstream GC/writer deadlock fixed in dolthub/dolt commit
// ccf7bde206 (PR #10876) — older binaries hang sql-server during
// dolt_backup('sync', ...) under heavy commit load. The gate must:
//
//  1. Reject older minors (1.85.9) AND the specific deadlock-
//     affected version (1.86.1), with an explainer pointing at
//     ccf7bde206 so on-call has the upstream context.
//  2. Accept the boundary 1.86.2 exactly.
//  3. Accept versions where the minor segment is multi-digit
//     (1.86.10); lexical string comparison would order 1.86.10
//     before 1.86.2 and reject it.
//  4. Accept the next major (2.0.0).
//  5. Reject pre-release/dev builds at the floor, while accepting
//     build metadata on a final release.
//  6. Fail closed when `dolt version` produces empty or
//     unparseable output. The "no dolt at all" path is already
//     covered by the command-not-found branch at the top of the
//     script.
func TestDoctorCheckVersionFloor(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantExit    int
		wantContain []string
		wantOmit    []string
	}{
		{
			name:        "older minor 1.85.9 rejected",
			version:     "dolt version 1.85.9",
			wantExit:    2,
			wantContain: []string{"too old", "1.85.9", "1.86.2", "ccf7bde206"},
		},
		{
			name:        "deadlock-affected 1.86.1 rejected",
			version:     "dolt version 1.86.1",
			wantExit:    2,
			wantContain: []string{"too old", "1.86.1", "1.86.2", "ccf7bde206"},
		},
		{
			name:        "boundary 1.86.2 accepted",
			version:     "dolt version 1.86.2",
			wantExit:    0,
			wantContain: []string{"dolt available", "1.86.2", "flock ok", "lsof ok"},
			wantOmit:    []string{"too old"},
		},
		{
			name:        "multi-digit minor 1.86.10 accepted",
			version:     "dolt version 1.86.10",
			wantExit:    0,
			wantContain: []string{"dolt available", "1.86.10"},
			wantOmit:    []string{"too old"},
		},
		{
			name:        "next major 2.0.0 accepted",
			version:     "dolt version 2.0.0",
			wantExit:    0,
			wantContain: []string{"dolt available", "2.0.0"},
			wantOmit:    []string{"too old"},
		},
		{
			name:        "pre-release 1.86.2-rc1 rejected",
			version:     "dolt version 1.86.2-rc1",
			wantExit:    2,
			wantContain: []string{"pre-release", "1.86.2-rc1", "1.86.2"},
			wantOmit:    []string{"dolt available"},
		},
		{
			name:        "pre-release with build metadata 1.86.2-rc1+build.5 rejected",
			version:     "dolt version 1.86.2-rc1+build.5",
			wantExit:    2,
			wantContain: []string{"pre-release", "1.86.2-rc1+build.5", "1.86.2"},
			wantOmit:    []string{"dolt available"},
		},
		{
			name:        "dev build 1.86.2-dev rejected",
			version:     "dolt version 1.86.2-dev.0",
			wantExit:    2,
			wantContain: []string{"pre-release", "1.86.2-dev.0", "1.86.2"},
			wantOmit:    []string{"dolt available"},
		},
		{
			name:        "pre-release above floor 1.99.0-rc1 rejected",
			version:     "dolt version 1.99.0-rc1",
			wantExit:    2,
			wantContain: []string{"pre-release", "1.99.0-rc1", "1.86.2"},
			wantOmit:    []string{"dolt available"},
		},
		{
			name:        "pre-release next major 2.0.0-rc1 rejected",
			version:     "dolt version 2.0.0-rc1",
			wantExit:    2,
			wantContain: []string{"pre-release", "2.0.0-rc1", "1.86.2"},
			wantOmit:    []string{"dolt available"},
		},
		{
			name:        "build metadata on 1.86.2 accepted",
			version:     "dolt version 1.86.2+build.5",
			wantExit:    0,
			wantContain: []string{"dolt available", "1.86.2+build.5"},
			wantOmit:    []string{"too old", "pre-release"},
		},
		{
			name:        "hyphenated build metadata on 1.86.2 accepted",
			version:     "dolt version 1.86.2+build-5",
			wantExit:    0,
			wantContain: []string{"dolt available", "1.86.2+build-5"},
			wantOmit:    []string{"too old", "pre-release"},
		},
		{
			name:        "v-prefixed 1.86.2 accepted",
			version:     "dolt version v1.86.2",
			wantExit:    0,
			wantContain: []string{"dolt available", "v1.86.2", "flock ok", "lsof ok"},
			wantOmit:    []string{"too old", "unrecognized"},
		},
		{
			// Empty `dolt version` output is rejected at the top
			// of the script (origin/main commit 885d07c2 added the
			// "unrecognized dolt version output" branch). The
			// version-floor gate must not trigger here — it would
			// be a false positive to claim the binary is "too old"
			// when we couldn't determine the version at all.
			name:        "empty version output rejected before gate",
			version:     "",
			wantExit:    1,
			wantContain: []string{"unrecognized dolt version output"},
			wantOmit:    []string{"too old"},
		},
		{
			name:        "non-version output fails closed",
			version:     "weird-binary-junk",
			wantExit:    1,
			wantContain: []string{"unrecognized dolt version output"},
			wantOmit:    []string{"too old"},
		},
		{
			name:        "two-component 1.86 rejected",
			version:     "dolt version 1.86",
			wantExit:    1,
			wantContain: []string{"unrecognized dolt version output"},
			wantOmit:    []string{"too old", "pre-release"},
		},
		{
			name:        "leading whitespace output rejected",
			version:     "  dolt version 1.85.9",
			wantExit:    1,
			wantContain: []string{"unrecognized dolt version output"},
			wantOmit:    []string{"too old", "pre-release"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := doctorSandbox(t, doctorSandboxOpts{
				dolt:         strPtr(tt.version),
				includeFlock: true,
				includeLsof:  true,
			})
			code, out := runDoctorCheck(t, bin)
			if code != tt.wantExit {
				t.Errorf("exit = %d, want %d\noutput:\n%s", code, tt.wantExit, out)
			}
			for _, sub := range tt.wantContain {
				if !strings.Contains(out, sub) {
					t.Errorf("output missing %q\noutput:\n%s", sub, out)
				}
			}
			for _, sub := range tt.wantOmit {
				if strings.Contains(out, sub) {
					t.Errorf("output unexpectedly contains %q\noutput:\n%s", sub, out)
				}
			}
		})
	}
}

func TestDoctorCheckVersionFloorDoesNotRequireVersionSort(t *testing.T) {
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.10"),
		includeFlock: true,
		includeLsof:  true,
	})
	sortPath := filepath.Join(bin, "sort")
	if err := os.Remove(sortPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove sort shim: %v", err)
	}
	writeExecutable(t, sortPath, "#!/bin/sh\necho 'sort -V unsupported' >&2\nexit 64\n")

	code, out := runDoctorCheck(t, bin)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 without sort -V\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "dolt available") {
		t.Fatalf("output = %s, want successful version probe", out)
	}
}

func installDoctorHealthGCShim(t *testing.T, bin, report string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "gc-invocations.log")
	writeExecutable(t, filepath.Join(bin, "gc"), fmt.Sprintf(`#!/bin/sh
echo "$*" >> %s
if [ "$1" = "dolt" ] && [ "$2" = "health" ] && [ "$3" = "--json" ]; then
  if [ "${GC_FAKE_HEALTH_SLEEP:-0}" != "0" ]; then
    sleep "$GC_FAKE_HEALTH_SLEEP"
  fi
  cat <<'JSON'
%s
JSON
  exit 0
fi
if [ "$1" = "dolt" ] && [ "$2" = "health-check" ]; then
  cat >/dev/null
  if [ "${GC_FAKE_HEALTHCHECK_FAIL:-0}" != "0" ]; then
    echo "Dolt commit count exceeded threshold: hq=412000 > 50000" >&2
    exit 1
  fi
  exit 0
fi
echo "unexpected gc invocation: $*" >&2
exit 64
`, shellQuote(logPath), report))
	return logPath
}

func TestDoctorCheckRunsStructuredHealthWhenEndpointPresent(t *testing.T) {
	report := `{
  "server": {"running": true, "reachable": true, "pid": 10, "port": 3307, "latency_ms": 12, "ping_latency_ms": 12},
  "real_query": {"enabled": false, "ok": false, "timeout": false, "latency_ms": 0, "exit_code": 0},
  "databases": [{"name": "hq", "commits": 7, "open_beads": 2, "noms_bytes": 1048576}],
  "storage": {"noms_bytes_visible": true, "noms_bytes_total": 1048576}
}`
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: true,
		includeLsof:  true,
	})
	logPath := installDoctorHealthGCShim(t, bin, report)

	code, out := runDoctorCheck(t, bin, "GC_DOLT_PORT=3307")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "health ok") {
		t.Fatalf("output missing structured health success:\n%s", out)
	}
	if !strings.Contains(out, `"noms_bytes_total": 1048576`) {
		t.Fatalf("output should include health report details for gc doctor --verbose:\n%s", out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gc invocation log: %v", err)
	}
	for _, want := range []string{"dolt health --json", "dolt health-check"} {
		if !strings.Contains(string(logData), want) {
			t.Fatalf("gc invocation log missing %q:\n%s", want, logData)
		}
	}
}

func TestDoctorCheckStructuredHealthFailureSurfacesAsDoctorError(t *testing.T) {
	report := `{
  "server": {"running": true, "reachable": true, "pid": 10, "port": 3307, "latency_ms": 12, "ping_latency_ms": 12},
  "real_query": {"enabled": false, "ok": false, "timeout": false, "latency_ms": 0, "exit_code": 0},
  "databases": [{"name": "hq", "commits": 412000, "open_beads": 2, "noms_bytes": null}],
  "storage": {"noms_bytes_visible": false, "noms_bytes_total": 0}
}`
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: true,
		includeLsof:  true,
	})
	installDoctorHealthGCShim(t, bin, report)

	code, out := runDoctorCheck(t, bin, "GC_DOLT_PORT=3307", "GC_FAKE_HEALTHCHECK_FAIL=1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "Dolt commit count exceeded threshold: hq=412000 > 50000") {
		t.Fatalf("output missing health threshold failure:\n%s", out)
	}
	if !strings.Contains(out, `"commits": 412000`) {
		t.Fatalf("output missing health report details:\n%s", out)
	}
}

func TestDoctorCheckSkipsStructuredHealthWithoutEndpoint(t *testing.T) {
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: true,
		includeLsof:  true,
	})
	writeExecutable(t, filepath.Join(bin, "gc"), "#!/bin/sh\necho unexpected gc invocation >&2\nexit 64\n")

	code, out := runDoctorCheck(t, bin)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "structured health skipped (no active Dolt endpoint)") {
		t.Fatalf("output missing no-endpoint skip:\n%s", out)
	}
	if strings.Contains(out, "unexpected gc invocation") {
		t.Fatalf("doctor should not invoke gc without an active endpoint:\n%s", out)
	}
}

func TestDoctorCheckStructuredHealthTimeoutIsBounded(t *testing.T) {
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: true,
		includeLsof:  true,
	})
	installDoctorHealthGCShim(t, bin, `{"server":{"reachable":true}}`)

	start := time.Now()
	code, out := runDoctorCheck(t, bin,
		"GC_DOLT_PORT=3307",
		"GC_DOLT_DOCTOR_HEALTH_TIMEOUT_SECS=1",
		"GC_FAKE_HEALTH_SLEEP=10",
	)
	if code != 2 {
		t.Fatalf("exit = %d, want 2\noutput:\n%s", code, out)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("health timeout took %s, want bounded well under 5s\noutput:\n%s", elapsed, out)
	}
	if !strings.Contains(out, "dolt health timed out after 1s") {
		t.Fatalf("output missing timeout message:\n%s", out)
	}
}

// TestDoctorCheckMissingFlock asserts the script exits 2 with the
// flock install hint when flock is absent. flock guards against
// concurrent dolt server starts; running without it can race two
// servers onto the same data directory and corrupt state.
func TestDoctorCheckMissingFlock(t *testing.T) {
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: false,
		includeLsof:  true,
	})
	code, out := runDoctorCheck(t, bin)
	if code != 2 {
		t.Errorf("exit = %d, want 2\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "flock not found") {
		t.Errorf("output missing %q\noutput:\n%s", "flock not found", out)
	}
}

// TestDoctorCheckMissingLsof asserts the script exits 2 with the
// lsof install hint when lsof is absent. lsof is required for the
// port-conflict detection path in runtime.sh / health.sh; failing
// fast here keeps the rest of the pack from misdiagnosing port
// state later.
func TestDoctorCheckMissingLsof(t *testing.T) {
	bin := doctorSandbox(t, doctorSandboxOpts{
		dolt:         strPtr("dolt version 1.86.2"),
		includeFlock: true,
		includeLsof:  false,
	})
	code, out := runDoctorCheck(t, bin)
	if code != 2 {
		t.Errorf("exit = %d, want 2\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "lsof not found") {
		t.Errorf("output missing %q\noutput:\n%s", "lsof not found", out)
	}
}
