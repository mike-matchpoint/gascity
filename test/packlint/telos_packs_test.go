package packlint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
)

// telosGuardrailsVerbatim is the binding guardrail text of the telos pack
// topology (in-repo law: the "Telos pack topology" tail sections of
// specs/agent-work-orders/GCD-WO-CSC-003/006/007). Every telos pack README
// must carry it VERBATIM.
const telosGuardrailsVerbatim = `"(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."`

var telosPackNames = []string{"telos-core", "telos-codegen", "telos-exec-monitoring", "telos-supervision"}

func telosPackDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "examples", "gastown", "packs", name)
	if _, err := os.Stat(filepath.Join(dir, "pack.toml")); err != nil {
		t.Fatalf("telos pack %s missing pack.toml: %v", name, err)
	}
	return dir
}

func TestTelosPackREADMEsCarryGuardrailsVerbatim(t *testing.T) {
	for _, pack := range telosPackNames {
		t.Run(pack, func(t *testing.T) {
			readme, err := os.ReadFile(filepath.Join(telosPackDir(t, pack), "README.md"))
			if err != nil {
				t.Fatalf("reading %s README: %v", pack, err)
			}
			if !strings.Contains(string(readme), telosGuardrailsVerbatim) {
				t.Fatalf("%s README does not carry the guardrails A/B verbatim", pack)
			}
		})
	}
}

func TestTelosExecMonitoringGuardrailBIsFirstLaw(t *testing.T) {
	readmePath := filepath.Join(telosPackDir(t, "telos-exec-monitoring"), "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("reading telos-exec-monitoring README: %v", err)
	}
	body := string(data)
	guardIdx := strings.Index(body, telosGuardrailsVerbatim)
	if guardIdx == -1 {
		t.Fatal("telos-exec-monitoring README missing the verbatim guardrails")
	}
	shipsIdx := strings.Index(body, "## What this pack ships")
	if shipsIdx == -1 {
		t.Fatal("telos-exec-monitoring README missing the ships section")
	}
	if guardIdx > shipsIdx {
		t.Fatal("guardrail B must be the pack's FIRST law: the verbatim guardrails must precede all other README content")
	}
	for _, frag := range []string{
		"telos-effectiveness-telemetry.template.md",
		"telos-gap-finding.template.md",
	} {
		data, err := os.ReadFile(filepath.Join(telosPackDir(t, "telos-exec-monitoring"), "template-fragments", frag))
		if err != nil {
			t.Fatalf("reading %s: %v", frag, err)
		}
		text := string(data)
		if !strings.Contains(text, "FIRST LAW") {
			t.Fatalf("%s does not open on the first-law guardrail", frag)
		}
		if !strings.Contains(text, "conformance verdicts stay in the single") {
			t.Fatalf("%s does not restate the verdict boundary", frag)
		}
	}
}

func TestTelosPackFragmentsFollowDefineConventionAndParse(t *testing.T) {
	for _, pack := range telosPackNames {
		t.Run(pack, func(t *testing.T) {
			fragDir := filepath.Join(telosPackDir(t, pack), "template-fragments")
			entries, err := os.ReadDir(fragDir)
			if err != nil {
				t.Fatalf("reading %s template-fragments: %v", pack, err)
			}
			if len(entries) == 0 {
				t.Fatalf("%s ships no template fragments", pack)
			}
			for _, entry := range entries {
				name := entry.Name()
				if !strings.HasSuffix(name, ".template.md") {
					t.Fatalf("%s fragment %s does not use the <name>.template.md convention", pack, name)
				}
				data, err := os.ReadFile(filepath.Join(fragDir, name))
				if err != nil {
					t.Fatalf("reading %s/%s: %v", pack, name, err)
				}
				text := string(data)
				defineName := strings.TrimSuffix(name, ".template.md")
				if !strings.HasPrefix(text, `{{ define "`+defineName+`" }}`) {
					t.Fatalf("%s fragment %s must open with {{ define %q }} (file name = define name)", pack, name, defineName)
				}
				if _, err := template.New(name).Option("missingkey=zero").Parse(text); err != nil {
					t.Fatalf("%s fragment %s does not parse as a Go template: %v", pack, name, err)
				}
			}
		})
	}
}

// TestTelosPacksCarryNoBusinessLiterals enforces the engine's REJECT-level
// generic-ness gate on the telos packs: estate/business doctrine arrives at
// runtime via the city-side SYSTEM-TELOS snapshot and per-repo cards — never
// baked into pack content.
func TestTelosPacksCarryNoBusinessLiterals(t *testing.T) {
	bannedSubstrings := []string{"matchpoint", "enrichment", "vehicle", "master/", "aws-gascity"}
	dNumber := regexp.MustCompile(`\bD[0-9]+\b`)
	for _, pack := range telosPackNames {
		t.Run(pack, func(t *testing.T) {
			root := telosPackDir(t, pack)
			if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lower := strings.ToLower(string(data))
				for _, needle := range bannedSubstrings {
					if strings.Contains(lower, needle) {
						t.Fatalf("%s contains business literal %q", path, needle)
					}
				}
				if loc := dNumber.FindString(string(data)); loc != "" {
					t.Fatalf("%s contains estate decision number %q", path, loc)
				}
				return nil
			}); err != nil {
				t.Fatalf("WalkDir(%s): %v", root, err)
			}
		})
	}
}

func TestTelosCoreFragmentsCarrySnapshotPinDuties(t *testing.T) {
	pin, err := os.ReadFile(filepath.Join(telosPackDir(t, "telos-core"), "template-fragments", "telos-snapshot-pin.template.md"))
	if err != nil {
		t.Fatalf("reading telos-snapshot-pin fragment: %v", err)
	}
	pinText := string(pin)
	for _, want := range []string{
		"{{ .CityRoot }}/specs/SYSTEM-TELOS.md",
		"specs-version: N",
		"telos: system vN",
		"TELOS SNAPSHOT: MISSING",
		"fail LOUD, never silent",
		"never a second copy",
		"read it at runtime",
	} {
		if !strings.Contains(pinText, want) {
			t.Fatalf("telos-snapshot-pin fragment missing required duty text %q", want)
		}
	}

	evidence, err := os.ReadFile(filepath.Join(telosPackDir(t, "telos-core"), "template-fragments", "telos-evidence-line.template.md"))
	if err != nil {
		t.Fatalf("reading telos-evidence-line fragment: %v", err)
	}
	evidenceText := string(evidence)
	for _, want := range []string{
		"telos: system vN / repo vM",
		"TELOS: NOT YET AUTHORED",
		"TELOS SNAPSHOT: MISSING",
	} {
		if !strings.Contains(evidenceText, want) {
			t.Fatalf("telos-evidence-line fragment missing required evidence form %q", want)
		}
	}
}

func TestTelosCodegenFragmentCarriesReadOrderAndStopDuty(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(telosPackDir(t, "telos-codegen"), "template-fragments", "telos-codegen-priming.template.md"))
	if err != nil {
		t.Fatalf("reading telos-codegen-priming fragment: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"**City snapshot**",
		"**Repo card**",
		"**The change law binds**",
		"STOP on conflict — never code around a telos",
		"File a question bead",
		"never a solo call at the maker seat",
		"No verdicts here",
		"conformance verdicts stay in the city's single evaluator/judge",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("telos-codegen-priming fragment missing required duty text %q", want)
		}
	}
	// The priming lane emits nothing: the telemetry record shape belongs to
	// telos-exec-monitoring alone.
	if strings.Contains(text, "telos-effectiveness |") {
		t.Fatal("telos-codegen-priming fragment must not carry the telemetry emission surface")
	}
}

func TestTelosSupervisionFragmentCarriesOverseerLaw(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(telosPackDir(t, "telos-supervision"), "template-fragments", "telos-overseer-law.template.md"))
	if err != nil {
		t.Fatalf("reading telos-overseer-law fragment: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		// The fragment-not-agent law and the verdict boundary open the file.
		"FIRST LAW — this is a fragment, never an agent",
		"evaluator/judge lane",
		// Activation stays honest about its gates.
		"Activation gate (dormant-honest)",
		"import ≠ inject",
		"LOUD defect, never a silent skip",
		// JR-2026-017 standing-cadence parity survives priority steers.
		"### Standing cadences run under every steer",
		"RUN under ANY steer unless the",
		"owner NAMES them in the steer or the system itself is broken",
		// The overseer duties migrated from the mayor templates.
		"### Adjudicate from the telos — the option space is not the decision boundary",
		"TELOS-FIRST ADJUDICATION LAW",
		"opened/closed pair on the city's telemetry partition",
		"### Capability walls — the option space carries the BUILD branch",
		"sr25-gascity-parity-rider",
		"### Knowledge strengthens the town, never a private memory",
		"MEMORY-TO-SYSTEM LAW",
		"### Directives pass the net-benefit bar — never prescribe from one incident",
		"DIRECTIVE NET-BENEFIT LAW",
		"### Telos feeders of the obligations view",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("telos-overseer-law fragment missing required duty text %q", want)
		}
	}
	// The supervision lane renders no verdicts and emits no telemetry record
	// shapes: the emission surface belongs to telos-exec-monitoring alone.
	if strings.Contains(text, "telos-effectiveness |") {
		t.Fatal("telos-overseer-law fragment must not carry the telemetry emission surface")
	}
}

func TestTelosExecMonitoringFragmentsEmitOnly(t *testing.T) {
	dir := telosPackDir(t, "telos-exec-monitoring")

	telemetry, err := os.ReadFile(filepath.Join(dir, "template-fragments", "telos-effectiveness-telemetry.template.md"))
	if err != nil {
		t.Fatalf("reading telos-effectiveness-telemetry fragment: %v", err)
	}
	telemetryText := string(telemetry)
	for _, want := range []string{
		"telos-effectiveness | date=<ISO-8601> | kind=<kind> | id=<bead-or-commit> | verdict=n/a | telos-gap=<facet|n/a> | cost=<attempt-burn/wall-clock/escalations>",
		"`verdict` stays `n/a` at emission time",
		"never the emitter's call",
	} {
		if !strings.Contains(telemetryText, want) {
			t.Fatalf("telos-effectiveness-telemetry fragment missing required emission contract %q", want)
		}
	}

	gap, err := os.ReadFile(filepath.Join(dir, "template-fragments", "telos-gap-finding.template.md"))
	if err != nil {
		t.Fatalf("reading telos-gap-finding fragment: %v", err)
	}
	gapText := string(gap)
	for _, want := range []string{
		"TELOS-GAP: card | depth | pointer | nowhere | n/a",
		"Route, never repair",
		"Never attach a conformance verdict",
	} {
		if !strings.Contains(gapText, want) {
			t.Fatalf("telos-gap-finding fragment missing required finding contract %q", want)
		}
	}
}
