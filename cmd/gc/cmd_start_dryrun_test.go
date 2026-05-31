package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

func TestPrintDryRunPreview(t *testing.T) {
	ds := map[string]TemplateParams{
		"mayor":         {SessionName: "mayor", TemplateName: "mayor", Command: "echo hello"},
		"hw--polecat-1": {SessionName: "hw--polecat-1", TemplateName: "hw/polecat-1", Command: "echo hello"},
		"hw--polecat-2": {SessionName: "hw--polecat-2", TemplateName: "hw/polecat-2", Command: "echo hello"},
	}

	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents: []config.Agent{
			{Name: "mayor", MaxActiveSessions: intPtr(1)},
			{Name: "polecat", Dir: "hw", MaxActiveSessions: intPtr(1)},
			{Name: "worker", Suspended: true},
		},
	}

	var stdout bytes.Buffer
	printDryRunPreview(ds, cfg, "test", &stdout)
	out := stdout.String()

	if !strings.Contains(out, "3 agent(s) would start") {
		t.Errorf("should report 3 agents, got:\n%s", out)
	}
	if !strings.Contains(out, "mayor") {
		t.Errorf("should list mayor, got:\n%s", out)
	}
	if !strings.Contains(out, "hw/polecat-1") {
		t.Errorf("should list hw/polecat-1, got:\n%s", out)
	}
	if !strings.Contains(out, "1 agent(s) suspended") {
		t.Errorf("should mention 1 suspended, got:\n%s", out)
	}
	if !strings.Contains(out, "No side effects executed (--dry-run).") {
		t.Errorf("should show dry-run footer, got:\n%s", out)
	}
}

func TestPrintDryRunPreviewEmpty(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "empty"},
	}

	var stdout bytes.Buffer
	printDryRunPreview(nil, cfg, "empty", &stdout)
	out := stdout.String()

	if !strings.Contains(out, "0 agent(s) would start") {
		t.Errorf("should report 0 agents, got:\n%s", out)
	}
	if !strings.Contains(out, "(no agents to start)") {
		t.Errorf("should show empty message, got:\n%s", out)
	}
}

func TestStartDryRunFlagExists(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newStartCmd(&stdout, &stderr)
	f := cmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("missing --dry-run flag")
	}
	if f.Shorthand != "n" {
		t.Errorf("--dry-run shorthand = %q, want %q", f.Shorthand, "n")
	}
}

func TestStartDryRunJSON(t *testing.T) {
	cityPath := filepath.Join(t.TempDir(), "city")
	if err := os.MkdirAll(cityPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte("[workspace]\nname = \"dry-run-json\"\n\n[beads]\nprovider = \"file\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCityScaffold(cityPath); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapScopedFileProviderCityFS(fsys.OSFS{}, cityPath); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GC_HOME", filepath.Join(t.TempDir(), "gc-home"))
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_BEADS_SCOPE_ROOT", "")

	oldDryRun := dryRunMode
	oldExtraConfigFiles := extraConfigFiles
	oldNoStrict := noStrictMode
	t.Cleanup(func() {
		dryRunMode = oldDryRun
		extraConfigFiles = oldExtraConfigFiles
		noStrictMode = oldNoStrict
	})
	dryRunMode = true
	extraConfigFiles = nil
	noStrictMode = false

	var stdout, stderr bytes.Buffer
	if code := doStartWithNameOverrideJSON([]string{cityPath}, false, &stdout, &stderr, "", true); code != 0 {
		t.Fatalf("doStartWithNameOverrideJSON code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Dry-run:") {
		t.Fatalf("stdout contains human dry-run preview instead of JSON only:\n%s", stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if payload["ok"] != true || payload["command"] != "start" || payload["action"] != "dry-run" {
		t.Fatalf("payload = %#v, want ok start dry-run", payload)
	}
	if payload["city_path"] != cityPath {
		t.Fatalf("city_path = %q, want %q", payload["city_path"], cityPath)
	}
}
