package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

func writeTelosPackFixture(t *testing.T, name string, fragmentFiles map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pack.toml"), []byte("[pack]\nname = \""+name+"\"\nschema = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fragDir := filepath.Join(dir, "template-fragments")
	if err := os.MkdirAll(fragDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, content := range fragmentFiles {
		if err := os.WriteFile(filepath.Join(fragDir, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImportSourceBaseName(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{"https://github.com/gastownhall/gascity.git//examples/gastown/packs/telos-core", "telos-core"},
		{"https://github.com/gastownhall/gascity.git", "gascity"},
		{"./packs/telos-codegen", "telos-codegen"},
		{".gc/system/packs/gastown", "gastown"},
		{"https://example.com/repo.git//packs/telos-core#v2", "telos-core"},
		{"./packs/telos-core/", "telos-core"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := importSourceBaseName(tc.source); got != tc.want {
			t.Errorf("importSourceBaseName(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestTelosImportExpectedPackName(t *testing.T) {
	cases := []struct {
		key    string
		source string
		want   string
	}{
		{"pack:telos-core", "https://github.com/gastownhall/gascity.git//examples/gastown/packs/telos-core", "telos-core"},
		// Key-based fallback when the source's base segment is opaque.
		{"pack:telos-exec-monitoring", "https://example.com/mirror.git", "telos-exec-monitoring"},
		{"rig:repo:telos-codegen", "./packs/telos-codegen", "telos-codegen"},
		{"pack:tools", "https://example.com/tools.git", ""},
		{"default-rig:gastown", ".gc/system/packs/gastown", ""},
	}
	for _, tc := range cases {
		got := telosImportExpectedPackName(tc.key, config.Import{Source: tc.source})
		if got != tc.want {
			t.Errorf("telosImportExpectedPackName(%q, %q) = %q, want %q", tc.key, tc.source, got, tc.want)
		}
	}
}

func TestMaterializedTelosPacksDiscoversFragments(t *testing.T) {
	telosDir := writeTelosPackFixture(t, "telos-core", map[string]string{
		"telos-snapshot-pin.template.md":  `{{ define "telos-snapshot-pin" }}pin{{ end }}`,
		"telos-evidence-line.template.md": `{{ define "telos-evidence-line" }}evidence{{ end }}`,
		"notes.md":                        "not a fragment",
	})
	otherDir := writeTelosPackFixture(t, "codegen-support", map[string]string{
		"slash-note-default.template.md": `{{ define "slash-note-default" }}note{{ end }}`,
	})

	packs := materializedTelosPacks(fsys.OSFS{}, []string{otherDir, telosDir, telosDir})
	if len(packs) != 1 {
		t.Fatalf("materializedTelosPacks = %#v, want exactly the telos pack", packs)
	}
	if packs[0].Name != "telos-core" || packs[0].Dir != telosDir {
		t.Fatalf("pack = %#v, want telos-core at %s", packs[0], telosDir)
	}
	wantFragments := []string{"telos-evidence-line", "telos-snapshot-pin"}
	if !reflect.DeepEqual(packs[0].Fragments, wantFragments) {
		t.Fatalf("fragments = %v, want %v", packs[0].Fragments, wantFragments)
	}
}

func TestBundledTelosPackNamesMatchesRegistryFamily(t *testing.T) {
	want := []string{"telos-core", "telos-codegen", "telos-exec-monitoring"}
	if got := bundledTelosPackNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("bundledTelosPackNames() = %v, want %v", got, want)
	}
}
