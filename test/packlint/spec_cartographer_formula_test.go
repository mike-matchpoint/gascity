package packlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecCartographerRequiresStructuralConvoyParentage(t *testing.T) {
	root := repoRoot()
	path := filepath.Join(root, "examples", "gastown", "packs", "codegen-support", "formulas", "spec-cartographer.formula.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading spec-cartographer formula: %v", err)
	}
	text := string(data)
	normalized := strings.Join(strings.Fields(text), " ")

	required := []string{
		"Structural convoy parentage is part of the emitted work",
		"`gc bd list --all --parent <convoy-id>`",
		"label-only, tracks-only, or dependency-edge-only substitute",
		"Convoy membership is a required graph component",
		"That member list is a promise that emit will create structural parent-child linkage",
		"`gc convoy add`, `tracks` edges, labels, and plain dependency edges are forbidden substitutes",
		"Convoy-parentage compile requirement",
		"missing parent references in the constructed `$RUN_DIR/emit_plan.json` are a hard failure before any branch creation or `bd create --graph`",
		"`cartographer_emit_fail \"convoy member missing structural parent\"`",
		"membership is NOT an edge and is NOT a `tracks` association",
	}

	for _, want := range required {
		if !strings.Contains(normalized, want) {
			t.Fatalf("spec-cartographer formula no longer carries required structural convoy parentage contract: missing %q", want)
		}
	}
}
