package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/fsys"
)

const telosPinFragment = `{{ define "telos-snapshot-pin" }}TELOS SNAPSHOT PIN DUTY{{ end }}`

// writeTelosStrictCity builds a city whose root pack imports a local
// telos-core pack shipping the telos-snapshot-pin fragment, with a single
// agent "steward" carrying agentToml extras (e.g. inject_fragments).
func writeTelosStrictCity(t *testing.T, agentToml, templateName, templateBody, fragmentBody string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".gc/.keep": "",
		"city.toml": `[workspace]
name = "telos-city"
`,
		"pack.toml": `[pack]
name = "demo-city"
schema = 2

[imports.telos-core]
source = "./packs/telos-core"

[[agent]]
name = "steward"
prompt_template = "prompts/` + templateName + `"
` + agentToml,
		"packs/telos-core/pack.toml": `[pack]
name = "telos-core"
schema = 2
`,
		"packs/telos-core/template-fragments/telos-snapshot-pin.template.md": fragmentBody,
		"prompts/" + templateName: templateBody,
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func chdirForPrime(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// TestDoPrimeStrictRendersTelosFragments verifies the happy path: with a
// telos pack imported and its fragment wired into the agent's
// inject_fragments, --strict succeeds and the rendered output carries the
// fragment content (spec-17 R10).
func TestDoPrimeStrictRendersTelosFragments(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\"]\n",
		"steward.template.md", "# Steward\n\nCity duties.\n", telosPinFragment)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, true)
	if code != 0 {
		t.Fatalf("doPrimeWithMode(strict) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "# Steward") || !strings.Contains(stdout.String(), "TELOS SNAPSHOT PIN DUTY") {
		t.Fatalf("stdout = %q, want base prompt plus telos fragment", stdout.String())
	}
}

// TestDoPrimeStrictErrorsOnMissingInjectFragment verifies the strict-render
// upgrade: a configured inject fragment with no registered template errors
// under --strict instead of warn-and-skip.
func TestDoPrimeStrictErrorsOnMissingInjectFragment(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\", \"not-shipped\"]\n",
		"steward.template.md", "# Steward\n\nCity duties.\n", telosPinFragment)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, true)
	if code == 0 {
		t.Fatalf("doPrimeWithMode(strict, missing fragment) = 0, want non-zero; stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `inject_fragment "not-shipped"`) {
		t.Fatalf("stderr = %q, want missing-fragment diagnostic", stderr.String())
	}
}

// TestDoPrimeDefaultKeepsWarnAndSkipOnMissingInjectFragment pins the default
// (non-strict) contract: the same missing fragment still warns to stderr and
// the prompt renders successfully.
func TestDoPrimeDefaultKeepsWarnAndSkipOnMissingInjectFragment(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\", \"not-shipped\"]\n",
		"steward.template.md", "# Steward\n\nCity duties.\n", telosPinFragment)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, false)
	if code != 0 {
		t.Fatalf("doPrimeWithMode(default, missing fragment) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "# Steward") || !strings.Contains(stdout.String(), "TELOS SNAPSHOT PIN DUTY") {
		t.Fatalf("stdout = %q, want base prompt plus telos fragment", stdout.String())
	}
	if !strings.Contains(stderr.String(), `inject_fragment "not-shipped": template not found`) {
		t.Fatalf("stderr = %q, want warn-and-skip diagnostic", stderr.String())
	}
}

// TestDoPrimeStrictErrorsWhenTelosFragmentCannotInject covers the silent
// non-injection path: a plain .md prompt_template skips template execution
// entirely, so a telos fragment configured for the role can never land in
// the output. --strict must fail loud on this (R10), since no warn exists
// on this path to upgrade.
func TestDoPrimeStrictErrorsWhenTelosFragmentCannotInject(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\"]\n",
		"steward.md", "# Steward plain\n", telosPinFragment)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, true)
	if code == 0 {
		t.Fatalf("doPrimeWithMode(strict, plain md + telos fragment) = 0, want non-zero; stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "R10") || !strings.Contains(stderr.String(), `"telos-snapshot-pin"`) {
		t.Fatalf("stderr = %q, want R10 telos diagnostic", stderr.String())
	}
}

// TestDoPrimeDefaultUnchangedForPlainTemplateWithTelosFragment pins that the
// default path stays exactly as before for the same configuration --strict
// rejects: cities in the wild must not break.
func TestDoPrimeDefaultUnchangedForPlainTemplateWithTelosFragment(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\"]\n",
		"steward.md", "# Steward plain\n", telosPinFragment)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, false)
	if code != 0 {
		t.Fatalf("doPrimeWithMode(default, plain md) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "# Steward plain") {
		t.Fatalf("stdout = %q, want plain prompt body", stdout.String())
	}
}

// TestDoPrimeStrictErrorsOnEmptyTelosFragment verifies that a telos fragment
// which renders to empty output fails --strict: an empty fragment leaves no
// verifiable trace in the rendered prompt.
func TestDoPrimeStrictErrorsOnEmptyTelosFragment(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	dir := writeTelosStrictCity(t,
		"inject_fragments = [\"telos-snapshot-pin\"]\n",
		"steward.template.md", "# Steward\n\nCity duties.\n",
		`{{ define "telos-snapshot-pin" }}{{ end }}`)
	chdirForPrime(t, dir)

	var stdout, stderr bytes.Buffer
	code := doPrimeWithMode([]string{"steward"}, &stdout, &stderr, false, true)
	if code == 0 {
		t.Fatalf("doPrimeWithMode(strict, empty telos fragment) = 0, want non-zero; stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "rendered empty") {
		t.Fatalf("stderr = %q, want rendered-empty diagnostic", stderr.String())
	}
}

// TestRenderPromptWithMetaOptionsStrictInject exercises the render core
// directly: strict errors on a missing fragment where the default path
// warns, skips, and records injected fragments.
func TestRenderPromptWithMetaOptionsStrictInject(t *testing.T) {
	dir := t.TempDir()
	packDir := writeTelosPackFixture(t, "telos-core", map[string]string{
		"telos-snapshot-pin.template.md": telosPinFragment,
	})
	if err := os.WriteFile(filepath.Join(dir, "steward.template.md"), []byte("# Steward\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	_, err := renderPromptWithMetaOptions(fsys.OSFS{}, dir, "demo", "steward.template.md", PromptContext{}, "", &stderr,
		[]string{packDir}, []string{"telos-snapshot-pin", "not-shipped"}, nil, promptRenderOptions{StrictInject: true})
	if err == nil || !strings.Contains(err.Error(), `inject_fragment "not-shipped"`) {
		t.Fatalf("strict render error = %v, want missing-fragment error", err)
	}

	stderr.Reset()
	res := renderPromptWithMeta(fsys.OSFS{}, dir, "demo", "steward.template.md", PromptContext{}, "", &stderr,
		[]string{packDir}, []string{"telos-snapshot-pin", "not-shipped"}, nil)
	if !strings.Contains(res.Text, "TELOS SNAPSHOT PIN DUTY") {
		t.Fatalf("default render text = %q, want telos fragment appended", res.Text)
	}
	if got := res.InjectedFragments["telos-snapshot-pin"]; got != "TELOS SNAPSHOT PIN DUTY" {
		t.Fatalf("InjectedFragments = %#v, want telos-snapshot-pin recorded", res.InjectedFragments)
	}
	if _, ok := res.InjectedFragments["not-shipped"]; ok {
		t.Fatalf("InjectedFragments = %#v, must not record skipped fragment", res.InjectedFragments)
	}
	if !strings.Contains(stderr.String(), `inject_fragment "not-shipped": template not found`) {
		t.Fatalf("stderr = %q, want default warn-and-skip preserved", stderr.String())
	}
}
