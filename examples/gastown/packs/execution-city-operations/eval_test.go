package executioncityoperations

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/formula"
	"github.com/gastownhall/gascity/internal/formulatest"
	"github.com/gastownhall/gascity/internal/molecule"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEvalFormulasCompileAndRouteDeterministicGrading(t *testing.T) {
	formulatest.EnableV2ForTest(t)
	formulaDir := filepath.Join("formulas")

	cohort, err := formula.Compile(context.Background(), "eval-run-cohort", []string{formulaDir}, map[string]string{
		"cohort_ref": "fixtures/cohort.json", "surface_kind": "surface.execute",
		"grader_cmd": "tools/grade --format json", "threshold": "0.80",
		"run_id": "run-001", "eval_suite": "suite-001", "binding_prefix": "operations.",
	})
	if err != nil {
		t.Fatalf("compile cohort formula: %v", err)
	}
	assertRecipeMetadata(t, cohort, "eval-run-cohort.fan-out-cases", map[string]string{
		"gc.kind": "fanout", "gc.control_for": "plan", "gc.for_each": "output.cases",
		"gc.bond": "eval-run-case",
	})
	assertRecipeDependency(t, cohort, "eval-run-cohort.aggregate", "eval-run-cohort.fan-out-cases")

	replay, err := formula.Compile(context.Background(), "eval-replay-step", []string{formulaDir}, map[string]string{
		"fixture_ref": "fixtures/recorded-step.json", "fixture_id": "STEP#surface.execute#case-001",
		"fixture_payload_b64": "eyJmaXh0dXJlX2lkIjoiU1RFUCNzdXJmYWNlLmV4ZWN1dGUjY2FzZS0wMDEifQ==",
		"surface_kind":        "surface.execute", "grader_cmd": "tools/grade --format json",
		"run_id": "run-002", "eval_suite": "suite-001", "binding_prefix": "operations.",
	})
	if err != nil {
		t.Fatalf("compile replay formula: %v", err)
	}
	store := beads.NewMemStore()
	if _, err := molecule.Instantiate(context.Background(), store, replay, molecule.Options{Vars: map[string]string{
		"fixture_ref": "fixtures/recorded-step.json", "fixture_id": "STEP#surface.execute#case-001",
		"fixture_payload_b64": "eyJmaXh0dXJlX2lkIjoiU1RFUCNzdXJmYWNlLmV4ZWN1dGUjY2FzZS0wMDEifQ==",
		"surface_kind":        "surface.execute", "grader_cmd": "tools/grade --format json",
		"run_id": "run-002", "eval_suite": "suite-001", "binding_prefix": "operations.",
	}}); err != nil {
		t.Fatalf("instantiate replay formula: %v", err)
	}
	all, err := store.List(beads.ListQuery{AllowScan: true, IncludeClosed: true})
	if err != nil {
		t.Fatalf("list replay beads: %v", err)
	}
	var execute *beads.Bead
	for i := range all {
		if strings.Contains(all[i].Ref, "execute-under-test") && all[i].Metadata["gc.attempt"] == "1" {
			execute = &all[i]
			break
		}
	}
	if execute == nil {
		t.Fatalf("replay has no execute-under-test attempt: %+v", all)
	}
	if got := execute.Metadata["gc.kind"]; got != "surface.execute" {
		t.Fatalf("execute gc.kind = %q, want supplied surface kind", got)
	}
	if got := execute.Metadata["eval.grader_cmd"]; got != "tools/grade --format json" {
		t.Fatalf("execute grader command = %q", got)
	}
	assertRecipeMetadata(t, replay, "eval-replay-step.replay-case.execute-under-test", map[string]string{
		"gc.check_path": ".gc/system/packs/execution-city-operations/assets/scripts/eval-grader-check.sh",
	})
}

func TestEvalManifestSchemaAcceptsExampleAndRejectsMissingGateOutcome(t *testing.T) {
	schemaBytes, err := os.ReadFile(filepath.Join("schemas", "eval", "eval-run-manifest.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	exampleBytes, err := os.ReadFile(filepath.Join("schemas", "eval", "examples", "eval-run-manifest.v1.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("eval-run-manifest.v1.schema.json", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("eval-run-manifest.v1.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	example, err := jsonschema.UnmarshalJSON(bytes.NewReader(exampleBytes))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(example); err != nil {
		t.Fatalf("example does not validate: %v", err)
	}
	var planted map[string]any
	if err := json.Unmarshal(exampleBytes, &planted); err != nil {
		t.Fatal(err)
	}
	delete(planted, "gate_outcome")
	if err := schema.Validate(planted); err == nil {
		t.Fatal("manifest missing gate_outcome unexpectedly validated")
	}
	if rows, ok := planted["case_results"].([]any); !ok || len(rows) == 0 {
		t.Fatal("example must contain at least one case result")
	}
}

func assertRecipeMetadata(t *testing.T, recipe *formula.Recipe, stepID string, want map[string]string) {
	t.Helper()
	for _, step := range recipe.Steps {
		if step.ID != stepID {
			continue
		}
		for key, value := range want {
			if got := step.Metadata[key]; got != value {
				t.Fatalf("%s metadata %s = %q, want %q", stepID, key, got, value)
			}
		}
		return
	}
	t.Fatalf("missing recipe step %s", stepID)
}

func assertRecipeDependency(t *testing.T, recipe *formula.Recipe, stepID, dependsOn string) {
	t.Helper()
	for _, dep := range recipe.Deps {
		if dep.StepID == stepID && dep.DependsOnID == dependsOn && dep.Type == "blocks" {
			return
		}
	}
	t.Fatalf("missing dependency %s -> %s", stepID, dependsOn)
}
