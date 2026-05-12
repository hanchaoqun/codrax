package agent

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// concrete_value_anchor_kind.go — projects each
// concreteValueEntry.kind value into the typed AnchorKind axis.
//
// extractConcreteValues + the language-aware scanners in
// concrete_values_lang.go emit 11 distinct kind tags, but the
// EvidenceItem AnchorKind axis is the closed 7-value set declared
// in types/evidence.go (definition / call / condition / return /
// assignment / initializer / import). Without this projection, every
// concrete_values evidence item lands at AnchorKind="" and
// ClaimFormOf falls through to ClaimUnknown — Phase 0 trace data
// (docs/design/semantic_surface_contract_p0_trace.md §2)
// measured 34% global ClaimUnknown rate, with concrete_values the
// dominant source. With this projection the same 11 kinds resolve
// to a real AnchorKind, and Phase 4's facet-coverage / claim-form
// oracles can run inferred-coverage paths instead of degrading.
//
// Mapping is many-to-one (11 → 7) because the syntactic categories
// the extractor distinguishes (e.g. "binds" / "maps" / "config")
// share an underlying anchor shape (assignment-like) at the
// AnchorKind axis. The mapping is conservative: when a category
// has no clean fit in the anchor-kind set (e.g. "implements", which is
// closer to a structural relation than a single anchor), we map to
// the closest definition-style anchor — this errs on
// over-classification rather than dropping back to AnchorKind="",
// which is what the original "all ClaimUnknown" defect was.
//
// Returns AnchorKind="" only on truly unknown / blank input so the
// caller's ClaimFormOf still falls through to ClaimUnknown for
// genuine gaps; never for the 11 known kinds.
//
// Single source of truth — adding a new kind in
// concrete_values_lang.go MUST add an arm here too. The mapping
// table is unit-tested for exhaustive coverage in
// concrete_value_anchor_kind_test.go.
func concreteValueKindToAnchorKind(kind string) types.AnchorKind {
	switch kind {
	// Return-shape: literal returns + error returns surface the
	// callee's outcome at the return site.
	case "returns", "errors":
		return types.AnchorReturn

	// Call-shape: direct calls + binding calls (Register / Bind /
	// Wire) both anchor at a call expression.
	case "calls", "binds":
		return types.AnchorCall

	// Condition-shape: extracted guard expressions.
	case "conditional":
		return types.AnchorCondition

	// Assignment-shape: variable assignments set a value via a write.
	case "assigns":
		return types.AnchorAssignment

	// Initializer-shape: map/object/contract entries + config-leaf
	// rows set values inside declarative member surfaces.
	case "maps", "config":
		return types.AnchorInitializer

	// Definition-shape: structural relations the extractor
	// surfaces (decorator targets, struct embeds, interface
	// implementations) all anchor at a declaration site. AnchorKind
	// has no separate "decoration" / "embeds" / "implements" so
	// AnchorDefinition is the closest typed bucket.
	case "decorates", "embeds", "implements":
		return types.AnchorDefinition
	}

	// Unknown / blank — fall through to AnchorKind="" so
	// ClaimFormOf returns ClaimUnknown and the existing
	// fallback path applies.
	return ""
}

// ── L2 / L3: context-enhanced DiagramRole projection ──────────────
//
// AnchorKind alone (L1) reaches ClaimFormOf Rule 4. To reach Rule 3
// (DiagramRole != Default and != Unknown → ClaimPrecedenceRole),
// concrete_values evidence needs DiagramRole context. Two layers:
//
//   L2 — file-extension signal: a value emitted from a file with a
//        config-shaped extension IS evidence about the config layer
//        by definition. → DiagramRole=Config → ClaimPrecedenceRole.
//
//   L3 — method-name prefix signal:
//        L3a — Register* / Bind* / Wire* methods (paired with kind=
//              binds/calls) are runtime-binding-layer methods.
//              → DiagramRole=Runtime → ClaimPrecedenceRole.
//        L3b — Default* / *Defaults* methods are the default-layer
//              feeder. → DiagramRole=Default. (Default does NOT
//              reach ClaimPrecedenceRole — Rule 3 explicitly
//              excludes it — but the precise classification
//              persists for downstream consumers that DO read
//              DiagramRole=Default semantically.)
//
// All three lists are operator-tunable via codrax.yaml so projects
// using non-default conventions (Terraform .tf, Kubernetes manifests
// .yaml, DI-framework Inject* / Provide* registration) can extend
// without code changes. Defaults cover the common 80% case.

// defaultConcreteValueConfigLayerExtensions is the seed list of
// extensions that signal "this file IS the config layer". Operators
// extend via codrax.yaml :: concrete_values_config_layer_extensions
// when their repo uses additional conventions (e.g. .conf for
// daemon configs, .nix for Nix expressions).
var defaultConcreteValueConfigLayerExtensions = []string{
	".yaml", ".yml", ".toml", ".json5",
	".ini", ".cfg", ".properties", ".env",
	".tf", ".hcl",
}

// defaultConcreteValueRuntimeMethodPrefixes is the seed list of
// method-name prefixes that signal "this method registers/binds
// something into the runtime layer". Operators extend via
// codrax.yaml :: concrete_values_runtime_method_prefixes when
// their repo uses framework-specific naming (e.g. Provide* for
// some DI frameworks, Inject* for fx).
var defaultConcreteValueRuntimeMethodPrefixes = []string{
	"Register", "Bind", "Wire",
}

// defaultConcreteValueDefaultMethodPrefixes is the seed list of
// method-name prefixes that signal "this method returns / produces
// default values for the default layer". Tighter list than the
// runtime prefixes — `Default` alone is unambiguous; `Defaults`
// (plural) catches the common `*Defaults()` constructor pattern.
var defaultConcreteValueDefaultMethodPrefixes = []string{
	"Default", "Defaults",
}

var (
	concreteValueRoleMu                sync.RWMutex
	concreteValueConfigLayerExtensions = append([]string(nil), defaultConcreteValueConfigLayerExtensions...)
	concreteValueRuntimeMethodPrefixes = append([]string(nil), defaultConcreteValueRuntimeMethodPrefixes...)
	concreteValueDefaultMethodPrefixes = append([]string(nil), defaultConcreteValueDefaultMethodPrefixes...)
)

// SetConcreteValueConfigLayerExtensions replaces the active
// extension list. Empty / nil restores the default. Each entry is
// auto-prepended with "." if missing and lowercased; duplicates
// collapse. Called from cmd/root.go at startup.
func SetConcreteValueConfigLayerExtensions(exts []string) {
	concreteValueRoleMu.Lock()
	defer concreteValueRoleMu.Unlock()
	concreteValueConfigLayerExtensions = normaliseExtList(exts, defaultConcreteValueConfigLayerExtensions)
}

// SetConcreteValueRuntimeMethodPrefixes replaces the active
// runtime-binding method-prefix list. Empty / nil restores default.
func SetConcreteValueRuntimeMethodPrefixes(prefixes []string) {
	concreteValueRoleMu.Lock()
	defer concreteValueRoleMu.Unlock()
	concreteValueRuntimeMethodPrefixes = normalisePrefixList(prefixes, defaultConcreteValueRuntimeMethodPrefixes)
}

// SetConcreteValueDefaultMethodPrefixes replaces the active
// default-layer method-prefix list. Empty / nil restores default.
func SetConcreteValueDefaultMethodPrefixes(prefixes []string) {
	concreteValueRoleMu.Lock()
	defer concreteValueRoleMu.Unlock()
	concreteValueDefaultMethodPrefixes = normalisePrefixList(prefixes, defaultConcreteValueDefaultMethodPrefixes)
}

// CurrentConcreteValueConfigLayerExtensions returns a defensive
// copy of the active extension list. Used by tests and startup
// telemetry.
func CurrentConcreteValueConfigLayerExtensions() []string {
	concreteValueRoleMu.RLock()
	defer concreteValueRoleMu.RUnlock()
	return append([]string(nil), concreteValueConfigLayerExtensions...)
}

// CurrentConcreteValueRuntimeMethodPrefixes returns a defensive
// copy of the active runtime-prefix list.
func CurrentConcreteValueRuntimeMethodPrefixes() []string {
	concreteValueRoleMu.RLock()
	defer concreteValueRoleMu.RUnlock()
	return append([]string(nil), concreteValueRuntimeMethodPrefixes...)
}

// CurrentConcreteValueDefaultMethodPrefixes returns a defensive
// copy of the active default-prefix list.
func CurrentConcreteValueDefaultMethodPrefixes() []string {
	concreteValueRoleMu.RLock()
	defer concreteValueRoleMu.RUnlock()
	return append([]string(nil), concreteValueDefaultMethodPrefixes...)
}

// normaliseExtList trims, lowercases, prepends ".", de-dupes the
// raw list. Empty/nil result restores defaults — operator who
// wants to disable the layer should leave the yaml field unset.
func normaliseExtList(raw, def []string) []string {
	if len(raw) == 0 {
		return append([]string(nil), def...)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, s := range raw {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return append([]string(nil), def...)
	}
	return out
}

// normalisePrefixList trims + de-dupes the raw list. Case-sensitive
// (method-name prefix matching is case-sensitive in Go).
func normalisePrefixList(raw, def []string) []string {
	if len(raw) == 0 {
		return append([]string(nil), def...)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return append([]string(nil), def...)
	}
	return out
}

// concreteValueDiagramRole computes the DiagramRole an evidence
// item should carry based on its source file extension and method
// name. Layer order: L2 (file-ext) > L3a (runtime-prefix) > L3b
// (default-prefix) > Unknown.
//
// L2 wins over L3 because filesystem signal is stronger than
// method-name convention: a value emitted from a *.yaml file IS
// in the config layer regardless of what method the extractor
// attributed it to. L3a wins over L3b for symmetric reason: if a
// method is BOTH a Register*-shape AND somehow also Default*-
// matches, treat as Runtime (binding-layer wins over feeder-layer
// in the precedence chain).
//
// Returns DiagramRoleUnknown when no signal matches; caller can
// either leave the field unset or fall back to L1's AnchorKind
// projection alone.
func concreteValueDiagramRole(file, methodName string) types.EvidenceDiagramRole {
	concreteValueRoleMu.RLock()
	exts := concreteValueConfigLayerExtensions
	runtime := concreteValueRuntimeMethodPrefixes
	defaults := concreteValueDefaultMethodPrefixes
	concreteValueRoleMu.RUnlock()

	// L2 — file-extension signal.
	//
	// Peel one variant-suffix layer (e.g. .example / .sample /
	// .dist / .tmpl / .tpl — the same list mentioned_file_entities
	// uses for sibling-glob detection) so e.g. `codrax.yaml.example`
	// matches the underlying `.yaml` config-layer signal. Without
	// the peel, filepath.Ext returns `.example`, which is the
	// variant marker, not the role-bearing extension.
	if file != "" {
		base := strings.ToLower(file)
		// Try direct ext first; fall back to peeled-once form.
		for try := 0; try < 2; try++ {
			ext := filepath.Ext(base)
			if ext == "" {
				break
			}
			for _, allowed := range exts {
				if ext == allowed {
					return types.EvidenceDiagramRoleConfig
				}
			}
			// On miss, peel the suffix when it matches a known
			// variant marker and re-check. Bounded to one peel so
			// repeated suffixes can't loop.
			peeled := false
			for _, sfx := range mentionedFileSiblingSuffixesSnapshot() {
				if ext == strings.ToLower(sfx) {
					base = strings.TrimSuffix(base, ext)
					peeled = true
					break
				}
			}
			if !peeled {
				break
			}
		}
	}

	// L3a — runtime-binding method prefix.
	if methodName != "" {
		for _, p := range runtime {
			if strings.HasPrefix(methodName, p) {
				return types.EvidenceDiagramRoleRuntime
			}
		}
	}

	// L3b — default-layer method prefix.
	if methodName != "" {
		for _, p := range defaults {
			if strings.HasPrefix(methodName, p) {
				return types.EvidenceDiagramRoleDefault
			}
		}
	}

	return types.EvidenceDiagramRoleUnknown
}
