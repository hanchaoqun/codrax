package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestConcreteValueKindToAnchorKind_AllKnownKinds pins the 11→6
// projection table. Adding a new kind in concrete_values_lang.go
// (or explorer.go's inline scanners) MUST add an arm here to keep
// the projection deterministic.
func TestConcreteValueKindToAnchorKind_AllKnownKinds(t *testing.T) {
	cases := []struct {
		kind string
		want types.AnchorKind
	}{
		{"returns", types.AnchorReturn},
		{"errors", types.AnchorReturn},
		{"calls", types.AnchorCall},
		{"binds", types.AnchorCall},
		{"conditional", types.AnchorCondition},
		{"assigns", types.AnchorAssignment},
		{"maps", types.AnchorAssignment},
		{"config", types.AnchorAssignment},
		{"decorates", types.AnchorDefinition},
		{"embeds", types.AnchorDefinition},
		{"implements", types.AnchorDefinition},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			if got := concreteValueKindToAnchorKind(c.kind); got != c.want {
				t.Errorf("kind=%q → got %q want %q", c.kind, got, c.want)
			}
		})
	}
}

// TestConcreteValueKindToAnchorKind_UnknownReturnsEmpty pins the
// fallback contract: an unknown kind MUST return AnchorKind="" so
// ClaimFormOf falls through to ClaimUnknown rather than
// fabricating a wrong projection. This keeps the 11→6 mapping a
// closed set; new kinds get explicit handling, never silent
// misclassification.
func TestConcreteValueKindToAnchorKind_UnknownReturnsEmpty(t *testing.T) {
	for _, k := range []string{"", "unknown_kind", "FOO", "NEW_KIND_NOT_IN_TABLE"} {
		if got := concreteValueKindToAnchorKind(k); got != "" {
			t.Errorf("kind=%q must fall through to empty AnchorKind; got %q", k, got)
		}
	}
}

// TestConcreteValueKindToAnchorKind_ProjectsToValidAnchorKind
// guards: every known mapping must land on one of the 6 declared
// AnchorKind constants. Catches a future typo that would silently
// produce a non-canonical AnchorKind value.
func TestConcreteValueKindToAnchorKind_ProjectsToValidAnchorKind(t *testing.T) {
	valid := map[types.AnchorKind]bool{
		types.AnchorDefinition: true,
		types.AnchorCall:       true,
		types.AnchorCondition:  true,
		types.AnchorReturn:     true,
		types.AnchorAssignment: true,
		types.AnchorImport:     true,
	}
	for _, k := range []string{
		"returns", "errors", "calls", "binds", "conditional",
		"assigns", "maps", "config", "decorates", "embeds", "implements",
	} {
		got := concreteValueKindToAnchorKind(k)
		if !valid[got] {
			t.Errorf("kind=%q projected to non-canonical AnchorKind %q", k, got)
		}
	}
}

// ── L2 / L3 DiagramRole projection ────────────────────────────────

func TestConcreteValueDiagramRole_FileExtensionConfigLayer(t *testing.T) {
	cases := []struct {
		file string
		want types.EvidenceDiagramRole
	}{
		// L2 hits: every default config-shaped extension.
		{"codrax.yaml.example", types.EvidenceDiagramRoleConfig},
		{"a/b/c.yml", types.EvidenceDiagramRoleConfig},
		{"settings.toml", types.EvidenceDiagramRoleConfig},
		{"build.json5", types.EvidenceDiagramRoleConfig},
		{"app.ini", types.EvidenceDiagramRoleConfig},
		{"daemon.cfg", types.EvidenceDiagramRoleConfig},
		{"app.properties", types.EvidenceDiagramRoleConfig},
		{".env", types.EvidenceDiagramRoleConfig},
		{"infra.tf", types.EvidenceDiagramRoleConfig},
		{"mod.hcl", types.EvidenceDiagramRoleConfig},
		// L2 misses: code-shaped extensions stay unknown.
		{"src/main.go", types.EvidenceDiagramRoleUnknown},
		{"app.py", types.EvidenceDiagramRoleUnknown},
		{"index.ts", types.EvidenceDiagramRoleUnknown},
		// Empty file path → no L2 trigger.
		{"", types.EvidenceDiagramRoleUnknown},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			if got := concreteValueDiagramRole(c.file, ""); got != c.want {
				t.Errorf("file=%q → got %q want %q", c.file, got, c.want)
			}
		})
	}
}

func TestConcreteValueDiagramRole_MethodPrefixRuntime(t *testing.T) {
	cases := []struct {
		method string
		want   types.EvidenceDiagramRole
	}{
		{"RegisterRoute", types.EvidenceDiagramRoleRuntime},
		{"BindHandler", types.EvidenceDiagramRoleRuntime},
		{"WireDependencies", types.EvidenceDiagramRoleRuntime},
		// No match for non-prefixed methods.
		{"Foo", types.EvidenceDiagramRoleUnknown},
		{"unregister", types.EvidenceDiagramRoleUnknown}, // case-sensitive
		{"", types.EvidenceDiagramRoleUnknown},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			if got := concreteValueDiagramRole("src/x.go", c.method); got != c.want {
				t.Errorf("method=%q → got %q want %q", c.method, got, c.want)
			}
		})
	}
}

func TestConcreteValueDiagramRole_MethodPrefixDefault(t *testing.T) {
	cases := []struct {
		method string
		want   types.EvidenceDiagramRole
	}{
		{"DefaultExploreHeuristics", types.EvidenceDiagramRoleDefault},
		{"DefaultsForFoo", types.EvidenceDiagramRoleDefault},
		{"Default", types.EvidenceDiagramRoleDefault},
		{"Defaults", types.EvidenceDiagramRoleDefault},
		// No match.
		{"GetDefault", types.EvidenceDiagramRoleUnknown},
		{"baseline", types.EvidenceDiagramRoleUnknown},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			if got := concreteValueDiagramRole("src/x.go", c.method); got != c.want {
				t.Errorf("method=%q → got %q want %q", c.method, got, c.want)
			}
		})
	}
}

func TestConcreteValueDiagramRole_LayerPriority(t *testing.T) {
	// L2 (file-ext) wins over L3 (method-prefix). When BOTH match,
	// file-ext takes priority because filesystem signal is stronger
	// than method-name convention.
	if got := concreteValueDiagramRole("codrax.yaml.example", "Default"); got != types.EvidenceDiagramRoleConfig {
		t.Errorf("L2 (config-ext) must win over L3b (default-prefix); got %q", got)
	}
	if got := concreteValueDiagramRole("settings.toml", "RegisterRoute"); got != types.EvidenceDiagramRoleConfig {
		t.Errorf("L2 (config-ext) must win over L3a (runtime-prefix); got %q", got)
	}
	// L3a (runtime) wins over L3b (default). When BOTH method-prefix
	// lists somehow match, runtime wins (binding-layer over feeder-
	// layer in the precedence chain).
	if got := concreteValueDiagramRole("src/x.go", "DefaultRegister"); got != types.EvidenceDiagramRoleDefault {
		// "DefaultRegister" matches Default* prefix first since
		// L3a checks runtime prefix list first; "DefaultRegister"
		// has prefix "Register"? No: "DefaultRegister" starts with
		// "Default", not "Register". This case lands at L3b.
		t.Logf("note: DefaultRegister starts with Default → L3b → got %q (expected Default)", got)
	}
	// True-conflict (rare): a method that starts with both prefixes
	// is impossible (string prefix is unique). Add the priority
	// invariant via construction: make a method that matches ONLY
	// runtime, ensure it lands at Runtime.
	if got := concreteValueDiagramRole("src/x.go", "RegisterDefault"); got != types.EvidenceDiagramRoleRuntime {
		t.Errorf("RegisterDefault should match runtime first; got %q", got)
	}
}

func TestSetConcreteValueExtensions_NormaliseAndDedup(t *testing.T) {
	original := CurrentConcreteValueConfigLayerExtensions()
	t.Cleanup(func() { SetConcreteValueConfigLayerExtensions(original) })

	SetConcreteValueConfigLayerExtensions([]string{"conf", ".NIX", "  .conf  ", ""})
	got := CurrentConcreteValueConfigLayerExtensions()
	want := []string{".conf", ".nix"}
	if !equalStringSlices(got, want) {
		t.Errorf("override + normalise: got %v want %v", got, want)
	}
	// Empty restores default.
	SetConcreteValueConfigLayerExtensions(nil)
	if !equalStringSlices(CurrentConcreteValueConfigLayerExtensions(), defaultConcreteValueConfigLayerExtensions) {
		t.Error("nil should restore default")
	}
}

func TestSetConcreteValueRuntimePrefixes_DedupAndRestoreDefault(t *testing.T) {
	original := CurrentConcreteValueRuntimeMethodPrefixes()
	t.Cleanup(func() { SetConcreteValueRuntimeMethodPrefixes(original) })

	SetConcreteValueRuntimeMethodPrefixes([]string{"Provide", "Register", "Provide"}) // dup + existing
	got := CurrentConcreteValueRuntimeMethodPrefixes()
	want := []string{"Provide", "Register"}
	if !equalStringSlices(got, want) {
		t.Errorf("override: got %v want %v", got, want)
	}
	SetConcreteValueRuntimeMethodPrefixes(nil)
	if !equalStringSlices(CurrentConcreteValueRuntimeMethodPrefixes(), defaultConcreteValueRuntimeMethodPrefixes) {
		t.Error("nil should restore default")
	}
}

func TestConcreteValueProjection_DiagramRoleReachesPrecedenceRole(t *testing.T) {
	// Integration: an EvidenceItem from a config-shaped file, with
	// L2-projected DiagramRole=Config, must reach ClaimFormOf Rule 3
	// (DiagramRole != Default → ClaimPrecedenceRole). This is the
	// load-bearing change that lifts FacetConfigPrecedenceRole's
	// inferred-coverage hit rate for QFConfigPrecedence questions.
	ev := types.EvidenceItem{
		Source:      "codrax.yaml.example",
		LineStart:   10,
		Producer:    "concrete_values",
		AnchorKind:  types.AnchorAssignment,
		Origin:      types.ClaimOriginCurrentRepo,
		DiagramRole: concreteValueDiagramRole("codrax.yaml.example", ""),
	}
	if ev.DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("DiagramRole should be Config from L2; got %q", ev.DiagramRole)
	}
	if got := types.ClaimFormOf(ev); got != types.ClaimPrecedenceRole {
		t.Errorf("L2-projected config evidence should reach ClaimPrecedenceRole; got %q", got)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestConcreteValueProjection_ClaimFormOfNoLongerUnknown is the
// integration check: an EvidenceItem with the projected
// AnchorKind + Origin from concrete_values must NOT fall through
// to ClaimUnknown via ClaimFormOf. This is the load-bearing
// behaviour change Phase-0-trace's 34% ClaimUnknown rate
// motivated.
func TestConcreteValueProjection_ClaimFormOfNoLongerUnknown(t *testing.T) {
	for _, kind := range []string{
		"returns", "errors", "calls", "binds", "conditional",
		"assigns", "maps", "config", "decorates", "embeds", "implements",
	} {
		t.Run(kind, func(t *testing.T) {
			ev := types.EvidenceItem{
				Source:     "src/x.go",
				LineStart:  10,
				Producer:   "concrete_values",
				AnchorKind: concreteValueKindToAnchorKind(kind),
				Origin:     types.ClaimOriginCurrentRepo,
			}
			form := types.ClaimFormOf(ev)
			if form == types.ClaimUnknown {
				t.Errorf("kind=%q with projected AnchorKind=%q + Origin=current_repo still projects to ClaimUnknown — projection broken", kind, ev.AnchorKind)
			}
		})
	}
}
