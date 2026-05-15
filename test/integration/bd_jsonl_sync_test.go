//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

// bd subprocess synchronization-invariant suite.
//
// Each test asserts the single invariant "after operation X completes,
// .beads/issues.jsonl and Dolt represent the same state for the affected
// bead." A failure means the operation left the two stores diverged.
//
// This is the fine-grained counterpart to persistence_coherence_test.go:
// that suite proves user-visible state survives subsequent bd calls; this
// suite proves the underlying contract (bd's export-on-write keeps JSONL
// equal to Dolt) holds operation by operation. When sync holds, the
// destructive auto-import path is a no-op and persistence is automatic.
// When sync breaks, every subsequent bd subprocess will revert Dolt to
// the stale JSONL — which is the bug we're fixing.
//
// Tests are intentionally surgical: one bd operation per test, one
// invariant assertion, one diff in the failure output. Running the suite
// produces a precise list of which operations break sync under which env
// configuration. A correct fix lands all of them green.

// jsonlBead is the subset of fields bd writes to issues.jsonl that this
// suite compares. bd's wire format uses `issue_type`/`parent` where the
// Go Bead type uses Type/ParentID; tags here match the wire format.
type jsonlBead struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Status   string            `json:"status"`
	Type     string            `json:"issue_type"`
	Priority *int              `json:"priority,omitempty"`
	Assignee string            `json:"assignee,omitempty"`
	Labels   []string          `json:"labels,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	ParentID string            `json:"parent,omitempty"`
}

// readBeadFromJSONL scans .beads/issues.jsonl for the line matching
// beadID and returns the parsed record. The bool return distinguishes
// "missing from JSONL" (false) from "present but parse failed" (test
// fails). Missing-from-JSONL is itself a sync break, not a test error.
func readBeadFromJSONL(t *testing.T, wsDir, beadID string) (jsonlBead, bool) {
	t.Helper()
	path := filepath.Join(wsDir, ".beads", "issues.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return jsonlBead{}, false
		}
		t.Fatalf("read %s: %v", path, err)
	}
	needle := []byte(`"id":"` + beadID + `"`)
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 || !bytes.Contains(line, needle) {
			continue
		}
		var b jsonlBead
		if err := json.Unmarshal(line, &b); err != nil {
			t.Fatalf("parse JSONL line for %s: %v\nline: %s", beadID, err, line)
		}
		if b.ID == beadID {
			return b, true
		}
	}
	return jsonlBead{}, false
}

// metadataEqual compares two metadata maps, treating nil and empty as
// equivalent. bd may omit empty metadata from JSONL.
func metadataEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || vb != va {
			return false
		}
	}
	return true
}

// labelsEqual compares two label slices order-independently.
func labelsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// intPtrEqual compares two *int values, treating both-nil as equal.
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// assertJSONLMatchesDolt fails the test if .beads/issues.jsonl does not
// reflect the same logical state as Dolt for the given bead ID. The
// failure message enumerates each diverging field, so the cause of the
// sync break is obvious from the test output alone.
//
// Fields compared: status, assignee, metadata, labels, priority, title,
// type, parent. Skipped: created_at, updated_at, owner — these vary for
// non-semantic reasons (timestamp precision, actor inheritance) and are
// not load-bearing for the persistence-coherence invariant.
//
// **Side-effect note.** Dolt is read via a bd subprocess invoked with
// BD_EXPORT_AUTO=false. Without this, a default-env `bd show` will
// trigger bd's deferred auto-export — populating JSONL as a side effect
// of the assertion and producing a false-positive pass. Suppressing the
// export on the verification path is what makes this an honest
// observation of whether the OPERATION under test synced the stores,
// rather than whether sync can be achieved by running yet another bd
// command.
func assertJSONLMatchesDolt(t *testing.T, ws *coherenceWorkspace, beadID string) {
	t.Helper()

	verifyStore := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
	doltBead, err := verifyStore.Get(beadID)
	if err != nil {
		t.Fatalf("get %s from Dolt: %v", beadID, err)
	}

	jb, found := readBeadFromJSONL(t, ws.dir, beadID)
	if !found {
		t.Errorf("sync break: %s exists in Dolt but is absent from JSONL\n  Dolt: status=%q assignee=%q metadata=%v",
			beadID, doltBead.Status, doltBead.Assignee, doltBead.Metadata)
		return
	}

	var diffs []string
	if doltBead.Status != jb.Status {
		diffs = append(diffs, fmt.Sprintf("  status:   Dolt=%q  JSONL=%q", doltBead.Status, jb.Status))
	}
	if doltBead.Assignee != jb.Assignee {
		diffs = append(diffs, fmt.Sprintf("  assignee: Dolt=%q  JSONL=%q", doltBead.Assignee, jb.Assignee))
	}
	if !metadataEqual(doltBead.Metadata, jb.Metadata) {
		diffs = append(diffs, fmt.Sprintf("  metadata: Dolt=%v  JSONL=%v", doltBead.Metadata, jb.Metadata))
	}
	if !labelsEqual(doltBead.Labels, jb.Labels) {
		diffs = append(diffs, fmt.Sprintf("  labels:   Dolt=%v  JSONL=%v", doltBead.Labels, jb.Labels))
	}
	if !intPtrEqual(doltBead.Priority, jb.Priority) {
		diffs = append(diffs, fmt.Sprintf("  priority: Dolt=%v  JSONL=%v", doltBead.Priority, jb.Priority))
	}
	if doltBead.Title != jb.Title {
		diffs = append(diffs, fmt.Sprintf("  title:    Dolt=%q  JSONL=%q", doltBead.Title, jb.Title))
	}
	if doltBead.Type != jb.Type {
		diffs = append(diffs, fmt.Sprintf("  type:     Dolt=%q  JSONL=%q", doltBead.Type, jb.Type))
	}
	if doltBead.ParentID != jb.ParentID {
		diffs = append(diffs, fmt.Sprintf("  parent:   Dolt=%q  JSONL=%q", doltBead.ParentID, jb.ParentID))
	}

	if len(diffs) > 0 {
		t.Errorf("sync break for %s after operation:\n%s", beadID, strings.Join(diffs, "\n"))
	}
}

// withEnvStore returns a BdStore for this workspace whose subprocess
// invocations apply the given env overrides on top of the inherited
// environment. Used to test specific configurations (e.g.,
// BD_EXPORT_AUTO=false) without touching .beads/config.yaml.
func (ws *coherenceWorkspace) withEnvStore(env map[string]string) *beads.BdStore {
	return beads.NewBdStoreWithPrefix(ws.dir, realBdRunner(env), ws.prefix)
}

// setManagedConfigExportFalse writes export.auto: false into
// .beads/config.yaml, matching gascity's EnsureCanonicalConfig stance
// from PR #1965. Used to isolate "config-driven" suppression from
// "env-driven" suppression.
func (ws *coherenceWorkspace) setManagedConfigExportFalse(t *testing.T) {
	t.Helper()
	runRealBDConfigSet(t, ws.env, ws.dir, "export.auto", "false")
}

// ============================================================
//  Group 1 — write ops, default bd env (export.auto=true default)
// ============================================================
//
// Default bd defers exports behind a 60s throttle. The first write in a
// window flushes; subsequent writes within the window don't. So back-to-
// back writes (the common case in a busy city) leave JSONL stale even
// without any gascity-side configuration.

// TestBdSync_CreateAlone_JSONLMatchesDolt: a single create on a fresh
// workspace. Throttle has never armed; bd flushes the export.
// Expected today: PASS (this is bd's only reliable sync case).
func TestBdSync_CreateAlone_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "lonely create"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_TwoCreatesBackToBack_JSONLMatchesDolt: the second create
// fires within the 60s throttle window armed by the first. bd defers
// the second export. JSONL has only the first bead.
// Expected today: FAIL on the second bead (missing from JSONL).
func TestBdSync_TwoCreatesBackToBack_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		first, err := ws.store.Create(beads.Bead{Title: "first"})
		if err != nil {
			t.Fatalf("create first: %v", err)
		}
		second, err := ws.store.Create(beads.Bead{Title: "second"})
		if err != nil {
			t.Fatalf("create second: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, first.ID)
		assertJSONLMatchesDolt(t, ws, second.ID)
	})
}

// TestBdSync_UpdateStatus_JSONLMatchesDolt: claim a freshly created bead
// by setting status=in_progress. The update fires inside the throttle
// window armed by the create.
// Expected today: FAIL — JSONL still shows status=open.
func TestBdSync_UpdateStatus_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "status target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update status: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateAssignee_JSONLMatchesDolt: assign a bead to an agent.
// Mirrors the polecat claim path documented in the runbook.
// Expected today: FAIL — JSONL still shows assignee="".
func TestBdSync_UpdateAssignee_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "assignee target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Assignee: stringPtr("test-rig/polecat-1")}); err != nil {
			t.Fatalf("update assignee: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateMetadata_JSONLMatchesDolt: set a metadata key via
// --set-metadata. Mirrors #2080 / #2093 metadata-wipe pattern.
// Expected today: FAIL — JSONL has no metadata for this bead.
func TestBdSync_UpdateMetadata_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "metadata target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.SetMetadata(bead.ID, "gc.routed_to", "test-rig/refinery"); err != nil {
			t.Fatalf("set-metadata: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateMetadataBatch_JSONLMatchesDolt: set multiple metadata
// keys in one Update call. Mirrors the session wake path (#2094) which
// writes state=awake + last_woke_at + wake_reason together.
// Expected today: FAIL — JSONL has no metadata for this bead.
func TestBdSync_UpdateMetadataBatch_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "session bead"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.SetMetadataBatch(bead.ID, map[string]string{
			"state":        "awake",
			"last_woke_at": time.Now().UTC().Format(time.RFC3339),
			"wake_reason":  "test",
		}); err != nil {
			t.Fatalf("batch metadata: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_AddLabel_JSONLMatchesDolt: append a label to an existing
// bead. Distinct write path from --set-metadata.
// Expected today: FAIL — JSONL has no labels for this bead.
func TestBdSync_AddLabel_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "label target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Labels: []string{"priority/p1"}}); err != nil {
			t.Fatalf("add label: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_Close_JSONLMatchesDolt: close a bead via status=closed.
// Mirrors #2079: the close lands in Dolt but doesn't make it to JSONL,
// so the next bd subprocess reverts it.
// Expected today: FAIL — JSONL still shows status=open.
func TestBdSync_Close_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "close target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("closed")}); err != nil {
			t.Fatalf("close: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// ============================================================
//  Group 2 — write ops with BD_EXPORT_AUTO=false (env override)
// ============================================================
//
// The gc bd wrapper sets BD_EXPORT_AUTO=false on every controller- and
// agent-facing call (cmd/gc/cmd_bd.go:69 → applyControlBdEnv). With this
// env var, bd skips even the first export that the default-env tests in
// Group 1 would catch. Every operation should diverge JSONL from Dolt.

// TestBdSync_CreateExportAutoFalse_JSONLMatchesDolt: a create with the
// env override that gascity applies in prod.
// Expected today: FAIL — env suppresses the export bd would otherwise
// have written.
func TestBdSync_CreateExportAutoFalse_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		store := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		bead, err := store.Create(beads.Bead{Title: "create with env=false"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateStatusExportAutoFalse_JSONLMatchesDolt: claim under
// the gascity production env.
// Expected today: FAIL.
func TestBdSync_UpdateStatusExportAutoFalse_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "status env target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		store := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		if err := store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update status: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateMetadataExportAutoFalse_JSONLMatchesDolt: set
// metadata under the gascity production env.
// Expected today: FAIL.
func TestBdSync_UpdateMetadataExportAutoFalse_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "metadata env target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		store := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		if err := store.SetMetadata(bead.ID, "probe", "value"); err != nil {
			t.Fatalf("set-metadata: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_CloseExportAutoFalse_JSONLMatchesDolt: close under the
// gascity production env.
// Expected today: FAIL.
func TestBdSync_CloseExportAutoFalse_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "close env target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		store := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		if err := store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("closed")}); err != nil {
			t.Fatalf("close: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// ============================================================
//  Group 3 — write ops with .beads/config.yaml export.auto=false
// ============================================================
//
// Same suppression as Group 2 but via the on-disk config file rather
// than the env var. This is the persistent stance gascity bakes in via
// EnsureCanonicalConfig (internal/beads/contract/files.go:226). Bd
// subprocesses invoked by agents that bypass the gc wrapper inherit
// this config and behave identically.

// TestBdSync_UpdateStatusManagedConfig_JSONLMatchesDolt: status update
// with the managed-config suppression (not env-driven).
// Expected today: FAIL.
func TestBdSync_UpdateStatusManagedConfig_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		ws.setManagedConfigExportFalse(t)
		bead, err := ws.store.Create(beads.Bead{Title: "managed cfg status"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update status: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_CloseManagedConfig_JSONLMatchesDolt: close with
// managed-config suppression.
// Expected today: FAIL.
func TestBdSync_CloseManagedConfig_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		ws.setManagedConfigExportFalse(t)
		bead, err := ws.store.Create(beads.Bead{Title: "managed cfg close"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("closed")}); err != nil {
			t.Fatalf("close: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// ============================================================
//  Group 4 — reads must not perturb either store
// ============================================================
//
// Reads (`bd show`, `bd list`) should be observationally pure. They may
// trigger the auto-import pre-flight internally, but the externally
// visible state — Dolt's row content and JSONL's snapshot — must be
// unchanged before vs. after. The bug pattern documents that even reads
// can disturb state in practice; here we lock down the contract.

// TestBdSync_Show_PreservesDoltAndJSONL: a default-env bd show should
// not perturb Dolt's row content nor JSONL's representation of the
// bead. Pre/post snapshots use the BD_EXPORT_AUTO=false verification
// store so that the SNAPSHOT itself doesn't trigger the very export-
// flush we're measuring.
//
// Expected today: FAIL — empirical evidence shows default-env bd show
// flushes pending exports, mutating JSONL as a side effect of the read.
func TestBdSync_Show_PreservesDoltAndJSONL(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "show target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// Drift JSONL from Dolt so any side-effect flush by the show
		// surfaces as a JSONL diff.
		if err := ws.store.SetMetadata(bead.ID, "before_show", "yes"); err != nil {
			t.Fatalf("set-metadata: %v", err)
		}

		probe := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		preDolt, err := probe.Get(bead.ID)
		if err != nil {
			t.Fatalf("pre-show probe Get: %v", err)
		}
		preJB, _ := readBeadFromJSONL(t, ws.dir, bead.ID)

		// The operation under test: a default-env (export-allowed) Get.
		if _, err := ws.store.Get(bead.ID); err != nil {
			t.Fatalf("show: %v", err)
		}

		postDolt, err := probe.Get(bead.ID)
		if err != nil {
			t.Fatalf("post-show probe Get: %v", err)
		}
		postJB, _ := readBeadFromJSONL(t, ws.dir, bead.ID)

		if preDolt.Status != postDolt.Status ||
			!metadataEqual(preDolt.Metadata, postDolt.Metadata) {
			t.Errorf("read changed Dolt state for %s:\n  pre:  status=%q metadata=%v\n  post: status=%q metadata=%v",
				bead.ID, preDolt.Status, preDolt.Metadata, postDolt.Status, postDolt.Metadata)
		}
		if preJB.Status != postJB.Status ||
			!metadataEqual(preJB.Metadata, postJB.Metadata) {
			t.Errorf("read changed JSONL state for %s:\n  pre:  status=%q metadata=%v\n  post: status=%q metadata=%v",
				bead.ID, preJB.Status, preJB.Metadata, postJB.Status, postJB.Metadata)
		}
	})
}

// TestBdSync_List_PreservesDoltAndJSONL: a default-env bd list should
// not perturb Dolt's row content nor JSONL's representation of any
// listed bead. Same probe-vs-operation env split as the show test.
//
// Expected today: FAIL — list flushes pending exports too.
func TestBdSync_List_PreservesDoltAndJSONL(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "list target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.SetMetadata(bead.ID, "before_list", "yes"); err != nil {
			t.Fatalf("set-metadata: %v", err)
		}

		probe := ws.withEnvStore(map[string]string{"BD_EXPORT_AUTO": "false"})
		preDolt, err := probe.Get(bead.ID)
		if err != nil {
			t.Fatalf("pre-list probe Get: %v", err)
		}
		preJB, _ := readBeadFromJSONL(t, ws.dir, bead.ID)

		if _, err := ws.store.List(beads.ListQuery{AllowScan: true}); err != nil {
			t.Fatalf("list: %v", err)
		}

		postDolt, err := probe.Get(bead.ID)
		if err != nil {
			t.Fatalf("post-list probe Get: %v", err)
		}
		postJB, _ := readBeadFromJSONL(t, ws.dir, bead.ID)

		if preDolt.Status != postDolt.Status ||
			!metadataEqual(preDolt.Metadata, postDolt.Metadata) {
			t.Errorf("list changed Dolt state for %s:\n  pre:  status=%q metadata=%v\n  post: status=%q metadata=%v",
				bead.ID, preDolt.Status, preDolt.Metadata, postDolt.Status, postDolt.Metadata)
		}
		if preJB.Status != postJB.Status ||
			!metadataEqual(preJB.Metadata, postJB.Metadata) {
			t.Errorf("list changed JSONL state for %s:\n  pre:  status=%q metadata=%v\n  post: status=%q metadata=%v",
				bead.ID, preJB.Status, preJB.Metadata, postJB.Status, postJB.Metadata)
		}
	})
}

// ============================================================
//  Group 5 — baseline pass: explicit bd export brings sync
// ============================================================

// TestBdSync_ExplicitExport_JSONLMatchesDolt: an explicit `bd export`
// after a sequence of updates must always reconcile JSONL to Dolt. This
// is the workaround the refinery used in #2093, and it's the path any
// fix must preserve.
// Expected today: PASS — proves the sync helper itself isn't lying.
func TestBdSync_ExplicitExport_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "explicit export target"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update status: %v", err)
		}
		if err := ws.store.SetMetadata(bead.ID, "explicit", "yes"); err != nil {
			t.Fatalf("set-metadata: %v", err)
		}

		// The user-facing workaround: force the export.
		runRealBDExportAll(t, ws.env, ws.dir)

		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// ============================================================
//  Group 6 — throttle isolation
// ============================================================
//
// Documents the throttle's role in the bug. Without the wait, updates
// inside the 60s window leave JSONL stale. With the wait + a subsequent
// bd command, bd flushes the deferred export and sync is restored.

// TestBdSync_UpdateInsideThrottleWindow_JSONLMatchesDolt: a single
// update right after a create. bd defers the export.
// Expected today: FAIL.
func TestBdSync_UpdateInsideThrottleWindow_JSONLMatchesDolt(t *testing.T) {
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "throttle inside"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// No sleep: well within the 60s throttle from the create.
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}

// TestBdSync_UpdateOutsideThrottleWindow_JSONLMatchesDolt: wait past the
// 60s throttle, do a single update. bd should flush the deferred
// export at end of the update subprocess.
// Expected today: PASS — empirical evidence in /tmp/bd-throttle showed
// the export landed after the wait. This test locks that behavior in
// and would surface a regression if bd's throttle semantics change.
//
// Skipped in -short mode because of the 65s wait.
func TestBdSync_UpdateOutsideThrottleWindow_JSONLMatchesDolt(t *testing.T) {
	if testing.Short() {
		t.Skip("requires 65s wait to clear bd's auto-export throttle")
	}
	withCoherenceFixture(t, func(ws *coherenceWorkspace) {
		bead, err := ws.store.Create(beads.Bead{Title: "throttle outside"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		time.Sleep(65 * time.Second)
		if err := ws.store.Update(bead.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
			t.Fatalf("update: %v", err)
		}
		assertJSONLMatchesDolt(t, ws, bead.ID)
	})
}
