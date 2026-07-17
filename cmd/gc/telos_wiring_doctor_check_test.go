package main

import (
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
)

func telosCheckCityDir(t *testing.T, packToml string) string {
	t.Helper()
	dir := t.TempDir()
	writeCityToml(t, dir, "[workspace]\nname = \"demo\"\n")
	if packToml != "" {
		writePackToml(t, dir, packToml)
	}
	return dir
}

func TestTelosWiringDoctorCheckOKWhenFragmentsWired(t *testing.T) {
	clearGCEnv(t)
	cityDir := telosCheckCityDir(t, "")
	packDir := writeTelosPackFixture(t, "telos-core", map[string]string{
		"telos-snapshot-pin.template.md":  `{{ define "telos-snapshot-pin" }}pin{{ end }}`,
		"telos-evidence-line.template.md": `{{ define "telos-evidence-line" }}evidence{{ end }}`,
	})
	cfg := &config.City{
		Workspace: config.Workspace{
			Name:            "demo",
			GlobalFragments: []string{"telos-evidence-line"},
		},
		Agents: []config.Agent{
			{Name: "steward", InjectFragments: []string{"telos-snapshot-pin"}},
		},
		PackDirs: []string{packDir},
	}

	result := newTelosWiringDoctorCheck(cityDir, cfg).Run(&doctor.CheckContext{CityPath: cityDir})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want OK; result=%#v", result.Status, result)
	}
	if !strings.Contains(result.Message, "1 telos pack(s) materialized, 2 fragment(s) wired") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestTelosWiringDoctorCheckReportsUnwiredFragment(t *testing.T) {
	clearGCEnv(t)
	cityDir := telosCheckCityDir(t, "")
	packDir := writeTelosPackFixture(t, "telos-core", map[string]string{
		"telos-snapshot-pin.template.md":  `{{ define "telos-snapshot-pin" }}pin{{ end }}`,
		"telos-evidence-line.template.md": `{{ define "telos-evidence-line" }}evidence{{ end }}`,
	})
	cfg := &config.City{
		Workspace: config.Workspace{Name: "demo"},
		Agents: []config.Agent{
			{Name: "steward", InjectFragments: []string{"telos-snapshot-pin"}},
		},
		PackDirs: []string{packDir},
	}

	check := newTelosWiringDoctorCheck(cityDir, cfg)
	result := check.Run(&doctor.CheckContext{CityPath: cityDir})
	if result.Status != doctor.StatusError {
		t.Fatalf("status = %v, want Error; result=%#v", result.Status, result)
	}
	if len(result.Details) != 1 ||
		!strings.Contains(result.Details[0], `fragment "telos-evidence-line"`) ||
		!strings.Contains(result.Details[0], "import ≠ inject") {
		t.Fatalf("details = %#v", result.Details)
	}
	if check.CanFix() || !strings.Contains(result.FixHint, "inject_fragments") {
		t.Fatalf("result = %#v, want non-fixable wiring hint", result)
	}
}

func TestTelosWiringDoctorCheckReportsPackWithNoFragments(t *testing.T) {
	clearGCEnv(t)
	cityDir := telosCheckCityDir(t, "")
	packDir := writeTelosPackFixture(t, "telos-core", nil)
	cfg := &config.City{
		Workspace: config.Workspace{Name: "demo"},
		PackDirs:  []string{packDir},
	}

	result := newTelosWiringDoctorCheck(cityDir, cfg).Run(&doctor.CheckContext{CityPath: cityDir})
	if result.Status != doctor.StatusError {
		t.Fatalf("status = %v, want Error; result=%#v", result.Status, result)
	}
	if len(result.Details) != 1 || !strings.Contains(result.Details[0], "ships no template fragments") {
		t.Fatalf("details = %#v", result.Details)
	}
}

func TestTelosWiringDoctorCheckAdvisoryWhenNoTelosImported(t *testing.T) {
	clearGCEnv(t)
	cityDir := telosCheckCityDir(t, "")
	// A non-telos pack in the graph must not suppress the advisory.
	otherDir := writeTelosPackFixture(t, "codegen-support", map[string]string{
		"slash-note-default.template.md": `{{ define "slash-note-default" }}note{{ end }}`,
	})
	cfg := &config.City{
		Workspace: config.Workspace{Name: "demo"},
		Agents:    []config.Agent{{Name: "steward"}},
		PackDirs:  []string{otherDir},
	}

	result := newTelosWiringDoctorCheck(cityDir, cfg).Run(&doctor.CheckContext{CityPath: cityDir})
	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want Warning; result=%#v", result.Status, result)
	}
	if !strings.Contains(result.Message, "R10 advisory: no telos packs imported") {
		t.Fatalf("message = %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(result.Details[0], "telos-core, telos-codegen, telos-exec-monitoring, telos-supervision") {
		t.Fatalf("details = %#v", result.Details)
	}
}

func TestTelosWiringDoctorCheckReportsDeclaredButUnmaterialized(t *testing.T) {
	clearGCEnv(t)
	cityDir := telosCheckCityDir(t, `[pack]
name = "demo"
schema = 2

[imports.telos-core]
source = "https://github.com/gastownhall/gascity.git//examples/gastown/packs/telos-core"
version = "^1.0"
`)
	cfg := &config.City{Workspace: config.Workspace{Name: "demo"}}

	result := newTelosWiringDoctorCheck(cityDir, cfg).Run(&doctor.CheckContext{CityPath: cityDir})
	if result.Status != doctor.StatusError {
		t.Fatalf("status = %v, want Error; result=%#v", result.Status, result)
	}
	if len(result.Details) != 1 ||
		!strings.Contains(result.Details[0], "pack:telos-core") ||
		!strings.Contains(result.Details[0], "declared but not materialized") {
		t.Fatalf("details = %#v", result.Details)
	}
	if !strings.Contains(result.FixHint, `run "gc import install"`) {
		t.Fatalf("fix hint = %q", result.FixHint)
	}
}
