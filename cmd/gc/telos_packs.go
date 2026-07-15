package main

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gastownhall/gascity/internal/builtinpacks"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

// telosPackNamePrefix identifies packs belonging to the telos layer family
// (the bundled members are telos-core, telos-codegen, and
// telos-exec-monitoring — see internal/builtinpacks). Spec-17 R10 binds city
// bootstrap to this family: imported telos packs must materialize into the
// pack graph and every fragment they ship must be wired into role injection
// lists (import ≠ inject). The prefix is a pack-family naming convention,
// not a role name.
const telosPackNamePrefix = "telos-"

// isTelosPackName reports whether a pack name belongs to the telos family.
func isTelosPackName(name string) bool {
	return strings.HasPrefix(name, telosPackNamePrefix)
}

// bundledTelosPackNames returns the telos packs bundled with gc. Used to
// name the expected composition in advisory doctor findings.
func bundledTelosPackNames() []string {
	var names []string
	for _, pack := range builtinpacks.All() {
		if isTelosPackName(pack.Name) {
			names = append(names, pack.Name)
		}
	}
	return names
}

// telosPack describes one materialized telos pack in a city's pack graph.
type telosPack struct {
	// Name is the pack's [pack].name from its pack.toml.
	Name string
	// Dir is the materialized pack directory.
	Dir string
	// Fragments lists the template-fragment names the pack ships
	// (file stem = define name, per the telos pack convention).
	Fragments []string
}

// materializedTelosPacks scans pack dirs for materialized telos packs and
// the template fragments they ship. Fragment names follow the telos pack
// convention enforced by test/packlint: each
// template-fragments/<name>.template.md opens with {{ define "<name>" }}.
func materializedTelosPacks(fs fsys.FS, packDirs []string) []telosPack {
	var packs []telosPack
	seen := make(map[string]struct{})
	for _, dir := range packDirs {
		name := packNameFromDir(fs, dir)
		if !isTelosPackName(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		packs = append(packs, telosPack{
			Name:      name,
			Dir:       dir,
			Fragments: packTemplateFragmentNames(fs, dir),
		})
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return packs
}

// packNameFromDir reads [pack].name from dir/pack.toml, returning "" when
// the manifest is missing or unreadable.
func packNameFromDir(fs fsys.FS, dir string) string {
	data, err := fs.ReadFile(filepath.Join(dir, "pack.toml"))
	if err != nil {
		return ""
	}
	var meta struct {
		Pack struct {
			Name string `toml:"name"`
		} `toml:"pack"`
	}
	if _, err := toml.Decode(string(data), &meta); err != nil {
		return ""
	}
	return meta.Pack.Name
}

// packTemplateFragmentNames lists the fragment names a pack ships under
// template-fragments/ using the <name>.template.md convention.
func packTemplateFragmentNames(fs fsys.FS, dir string) []string {
	entries, err := fs.ReadDir(filepath.Join(dir, "template-fragments"))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isCanonicalPromptTemplatePath(name) {
			continue
		}
		names = append(names, strings.TrimSuffix(name, canonicalPromptTemplateSuffix))
	}
	sort.Strings(names)
	return names
}

// telosImportExpectedPackName reports the telos pack a declared import is
// expected to materialize, or "" when the import does not address a telos
// pack. Detection is by the family naming convention on the source's final
// path segment, falling back to the import key's final segment
// (collectAllImportsFS keys are prefixed, e.g. "pack:telos-core").
func telosImportExpectedPackName(key string, imp config.Import) string {
	if base := importSourceBaseName(imp.Source); isTelosPackName(base) {
		return base
	}
	segments := strings.Split(key, ":")
	if base := segments[len(segments)-1]; isTelosPackName(base) {
		return base
	}
	return ""
}

// importSourceBaseName extracts the final path segment of an import source,
// handling remote repo//subpath sources, optional #ref suffixes, and .git
// endings. Returns "" for an empty source.
func importSourceBaseName(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if idx := strings.Index(source, "#"); idx >= 0 {
		source = source[:idx]
	}
	// repo//subpath — take the subpath after the last "//" that is not a
	// scheme separator ("://").
	if idx := strings.LastIndex(source, "//"); idx > 0 && source[idx-1] != ':' {
		source = source[idx+2:]
	}
	source = strings.TrimSuffix(strings.TrimRight(source, "/"), ".git")
	return path.Base(filepath.ToSlash(source))
}

// effectiveInjectionUnion returns every fragment name reachable through the
// city's injection lists: workspace global fragments, agent-default appends,
// and each agent's effective fragment layering (the same
// effectivePromptFragments contract gc prime renders with).
func effectiveInjectionUnion(cfg *config.City) map[string]struct{} {
	union := make(map[string]struct{})
	add := func(names []string) {
		for _, name := range names {
			if name != "" {
				union[name] = struct{}{}
			}
		}
	}
	add(cfg.Workspace.GlobalFragments)
	add(cfg.AgentDefaults.AppendFragments)
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		add(effectivePromptFragments(
			cfg.Workspace.GlobalFragments,
			a.InjectFragments,
			a.AppendFragments,
			a.InheritedAppendFragments,
			cfg.AgentDefaults.AppendFragments,
		))
	}
	return union
}

// assertTelosFragmentsRendered enforces spec-17 R10 at the gc prime --strict
// seam: when telos packs are materialized in the city's pack graph, every
// telos fragment configured for the primed role must appear, non-empty, in
// the rendered output. Returns false after writing one diagnostic per miss.
// Findings only apply under --strict; the default render path is untouched.
func assertTelosFragmentsRendered(stderr io.Writer, cfg *config.City, agentName string, fragments []string, res PromptRenderResult) bool {
	telosByFragment := make(map[string]string)
	for _, pack := range materializedTelosPacks(fsys.OSFS{}, cfg.PackDirs) {
		for _, fragment := range pack.Fragments {
			telosByFragment[fragment] = pack.Name
		}
	}
	if len(telosByFragment) == 0 {
		// No telos packs imported — nothing to assert for this render.
		return true
	}
	ok := true
	for _, name := range fragments {
		packName, isTelos := telosByFragment[name]
		if !isTelos {
			continue
		}
		text, injected := res.InjectedFragments[name]
		switch {
		case !injected:
			fmt.Fprintf(stderr, "gc prime: R10: telos fragment %q (pack %s) is configured for agent %q but was not injected into the rendered prompt (is the prompt_template a %s template?)\n", //nolint:errcheck // best-effort stderr
				name, packName, agentName, canonicalPromptTemplateSuffix)
			ok = false
		case strings.TrimSpace(text) == "":
			fmt.Fprintf(stderr, "gc prime: R10: telos fragment %q (pack %s) rendered empty for agent %q\n", //nolint:errcheck // best-effort stderr
				name, packName, agentName)
			ok = false
		case !strings.Contains(res.Text, text):
			fmt.Fprintf(stderr, "gc prime: R10: telos fragment %q (pack %s) is absent from agent %q rendered output\n", //nolint:errcheck // best-effort stderr
				name, packName, agentName)
			ok = false
		}
	}
	return ok
}
