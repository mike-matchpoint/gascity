package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/fsys"
)

// telosWiringDoctorCheck enforces spec-17 R10 as a findings-only doctor
// check: imported telos packs must materialize into the city's pack graph
// and every fragment they ship must be wired into at least one role's
// effective injection list (R10's tightened form: import ≠ inject). When no
// telos pack is imported at all, the check emits an advisory naming the
// expected composition. The check reports; it never mutates city state.
// Config-load-fatal wiring states (e.g. an import whose source dir is
// missing — the V2 fatal-on-missing-source rule) never reach this check:
// buildDoctorChecks registers it only on a loaded config, and the
// pre-existing expanded-config-load check dies loud on that state first
// (round-2 verified 2026-07-15) — the upstream backstop, not a gap.
type telosWiringDoctorCheck struct {
	cityPath string
	cfg      *config.City
}

func newTelosWiringDoctorCheck(cityPath string, cfg *config.City) *telosWiringDoctorCheck {
	return &telosWiringDoctorCheck{cityPath: cityPath, cfg: cfg}
}

// Name returns the check identifier.
func (c *telosWiringDoctorCheck) Name() string { return "telos-pack-wiring" }

// Run reports R10 telos wiring findings for the loaded city config.
func (c *telosWiringDoctorCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	r := &doctor.CheckResult{Name: c.Name()}

	declared, err := collectAllImportsFS(fsys.OSFS{}, c.cityPath)
	if err != nil {
		r.Status = doctor.StatusError
		r.Message = fmt.Sprintf("reading declared imports: %v", err)
		return r
	}

	materialized := materializedTelosPacks(fsys.OSFS{}, collectPackDirs(c.cfg))
	materializedByName := make(map[string]telosPack, len(materialized))
	for _, pack := range materialized {
		materializedByName[pack.Name] = pack
	}

	var details []string

	// Declared telos imports must resolve to a materialized pack.
	declaredTelos := make(map[string]struct{})
	for _, key := range sortedTelosImportKeys(declared) {
		expected := telosImportExpectedPackName(key, declared[key])
		if expected == "" {
			continue
		}
		declaredTelos[expected] = struct{}{}
		if _, ok := materializedByName[expected]; !ok {
			details = append(details, fmt.Sprintf("R10: telos import %s (%s) is declared but not materialized in the pack graph", key, declared[key].Source))
		}
	}

	if len(materialized) == 0 && len(declaredTelos) == 0 {
		r.Status = doctor.StatusWarning
		r.Message = "R10 advisory: no telos packs imported"
		r.Details = []string{fmt.Sprintf("expected composition: a city's role mix imports its applicable telos packs (bundled: %s) and wires their fragments into role injection lists", strings.Join(bundledTelosPackNames(), ", "))}
		r.FixHint = "import the telos packs applicable to this city's role mix and wire their fragments into role injection lists"
		return r
	}

	// Every fragment an imported telos pack ships must be wired into at
	// least one role's effective injection list.
	union := effectiveInjectionUnion(c.cfg)
	for _, pack := range materialized {
		if len(pack.Fragments) == 0 {
			details = append(details, fmt.Sprintf("R10: telos pack %q at %s ships no template fragments", pack.Name, pack.Dir))
			continue
		}
		for _, fragment := range pack.Fragments {
			if _, wired := union[fragment]; !wired {
				details = append(details, fmt.Sprintf("R10: telos pack %q fragment %q is imported but not wired into any role's injection list (import ≠ inject)", pack.Name, fragment))
			}
		}
	}

	if len(details) > 0 {
		r.Status = doctor.StatusError
		r.Message = fmt.Sprintf("%d telos wiring issue(s) (spec-17 R10)", len(details))
		r.Details = details
		r.FixHint = `wire each telos fragment into workspace.global_fragments or a role's inject_fragments/append_fragments; run "gc import install" for unmaterialized imports`
		return r
	}

	fragmentCount := 0
	for _, pack := range materialized {
		fragmentCount += len(pack.Fragments)
	}
	r.Status = doctor.StatusOK
	r.Message = fmt.Sprintf("%d telos pack(s) materialized, %d fragment(s) wired into role injection lists", len(materialized), fragmentCount)
	return r
}

// CanFix returns false; R10 wiring is a city config decision — findings only.
func (c *telosWiringDoctorCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *telosWiringDoctorCheck) Fix(_ *doctor.CheckContext) error { return nil }

// sortedTelosImportKeys returns the import keys in deterministic order so
// findings are stable across runs.
func sortedTelosImportKeys(imports map[string]config.Import) []string {
	keys := make([]string, 0, len(imports))
	for key := range imports {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
