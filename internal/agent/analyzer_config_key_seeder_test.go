package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestConfigKeyTokens pins the prefix-FREE token split: any config key —
// grouped, dotted/nested, or flat — reduces to its meaningful lower-cased
// tokens, with connector-length fragments (<3 chars) dropped. No assumption
// that the group is the first segment.
func TestConfigKeyTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"alpha_widget_max_size", []string{"alpha", "widget", "max", "size"}},
		{"server.idle_timeout", []string{"server", "idle", "timeout"}},
		{"max-retries-per-stage", []string{"max", "retries", "per", "stage"}}, // "per" is 3 chars → kept
		{"verbose", []string{"verbose"}},                                      // flat key, no separator
		{"  Spaced Key  ", []string{"spaced", "key"}},
		{"a_of_to_b", []string{}}, // a/of/to/b all <3 chars → dropped → empty
		{"", []string{}},
	}
	for _, c := range cases {
		got := configKeyTokens(c.in)
		if len(got) != len(c.want) {
			t.Errorf("configKeyTokens(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("configKeyTokens(%q)[%d] = %q, want %q (full=%v)", c.in, i, got[i], c.want[i], got)
			}
		}
	}
}

// writeRepoFile is a tiny temp-repo helper for the seeder test.
func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestConfigKeyGroupSiblingFiles_SeedsGroupHome verifies the config-trace
// seeder on a phantom key (absent from the repo). It pins the key
// generalization over a leading-prefix grep: the code-default struct whose
// yaml tags DROP the group prefix (keyed by the bare sub-key) is still
// reached via shared sub-key tokens, while an unrelated file carrying none
// of the key's tokens is not seeded.
func TestConfigKeyGroupSiblingFiles_SeedsGroupHome(t *testing.T) {
	root := t.TempDir()
	// YAML-override layer: prefixed sibling keys (alpha_widget_*).
	writeRepoFile(t, root, "internal/config/runtime.go", `
type RuntimeSettings struct {
	AlphaWidgetMinSize *int `+"`yaml:\"alpha_widget_min_size\"`"+`
	AlphaWidgetDefault *int `+"`yaml:\"alpha_widget_default\"`"+`
}
`)
	// Code-default struct: DE-PREFIXED yaml tags (no "alpha_" group prefix) —
	// a leading-prefix grep on "alpha_" would MISS this file; token
	// co-occurrence (widget/max/size) still reaches it.
	writeRepoFile(t, root, "internal/types/config.go", `
type WidgetDefaults struct {
	WidgetMaxSize int `+"`yaml:\"widget_max_size\"`"+`
	WidgetMinSize int `+"`yaml:\"widget_min_size\"`"+`
}
`)
	// Unrelated file: carries none of the key's tokens.
	writeRepoFile(t, root, "internal/agent/unrelated.go", "package agent\nfunc Foo() {}\n")

	ctx := &types.AgentContext{RepoRoot: root}
	rm := types.RequestModel{
		Scenario: types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			ExactTargets: []string{"alpha_widget_max_size"}, // phantom — absent from repo
		},
	}

	got := configKeyGroupSiblingFiles(ctx, rm)
	if len(got) == 0 {
		t.Fatalf("expected schema-home file(s) to be seeded, got none")
	}
	foundDePrefixedHome, foundUnrelated := false, false
	for _, p := range got {
		if p == "internal/types/config.go" {
			foundDePrefixedHome = true
		}
		if p == "internal/agent/unrelated.go" {
			foundUnrelated = true
		}
	}
	if !foundDePrefixedHome {
		t.Errorf("expected internal/types/config.go (de-prefixed code-default home) in seeds, got %v", got)
	}
	if foundUnrelated {
		t.Errorf("unrelated file (no key tokens) must not be seeded, got %v", got)
	}
	if len(got) > configKeyGroupSiblingCap() {
		t.Errorf("seeds exceeded cap %d: %v", configKeyGroupSiblingCap(), got)
	}
}

// TestConfigKeyGroupSiblingFiles_ConfidenceFloorSkipsSingleTokenNoise is the
// noise guard for generic-token keys: a file matching only ONE of the key's
// tokens (the common case for words like "size"/"max") must not be seeded,
// even if it scores on that single token. Only files co-occurring ≥2 distinct
// key tokens (the real schema home) survive the floor.
func TestConfigKeyGroupSiblingFiles_ConfidenceFloorSkipsSingleTokenNoise(t *testing.T) {
	root := t.TempDir()
	// Schema home: co-occurs all three tokens.
	writeRepoFile(t, root, "internal/config/widgets.go", `
type WidgetCfg struct {
	GadgetWidgetSize int `+"`yaml:\"gadget_widget_size\"`"+`
}
`)
	// Single-token noise: mentions "size" many times but neither "gadget"
	// nor "widget" — must be filtered out by the ≥2-distinct-token floor.
	writeRepoFile(t, root, "internal/render/layout.go", "package render\n// size size size size size\nvar size, maxSize, minSize int\n")

	ctx := &types.AgentContext{RepoRoot: root}
	rm := types.RequestModel{
		Scenario: types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			ExactTargets: []string{"gadget_widget_size"}, // phantom; tokens gadget/widget/size
		},
	}

	got := configKeyGroupSiblingFiles(ctx, rm)
	for _, p := range got {
		if p == "internal/render/layout.go" {
			t.Errorf("single-token noise file must be filtered by confidence floor, got %v", got)
		}
	}
	// The genuine schema home (≥2 tokens) should still come through.
	foundHome := false
	for _, p := range got {
		if p == "internal/config/widgets.go" {
			foundHome = true
		}
	}
	if !foundHome {
		t.Errorf("schema home matching all tokens should survive the floor, got %v", got)
	}
}

// TestConfigKeyGroupSiblingFiles_TreatsRegexPunctuationLiterally pins that
// config-key tokens are not interpreted as regex. `foo+bar` and
// `widget(size)` would both match the noisy file under regex semantics
// (`fooooobar` and `widgetsize`); only the literal schema surface should seed.
func TestConfigKeyGroupSiblingFiles_TreatsRegexPunctuationLiterally(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "internal/config/runtime.go", `
type RuntimeSettings struct {
	FooBarWidgetSize *int `+"`yaml:\"foo+bar_widget(size)\"`"+`
}
`)
	writeRepoFile(t, root, "internal/noise/regex_like.go", "package noise\n// foooooobar widgetsize\n")

	ctx := &types.AgentContext{RepoRoot: root}
	rm := types.RequestModel{
		Scenario: types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			ExactTargets: []string{"foo+bar_widget(size)"},
		},
	}

	got := configKeyGroupSiblingFiles(ctx, rm)
	foundSchema, foundRegexNoise := false, false
	for _, p := range got {
		if p == "internal/config/runtime.go" {
			foundSchema = true
		}
		if p == "internal/noise/regex_like.go" {
			foundRegexNoise = true
		}
	}
	if !foundSchema {
		t.Fatalf("literal schema file should be seeded, got %v", got)
	}
	if foundRegexNoise {
		t.Fatalf("regex-shaped noise file must not be seeded by literal key search, got %v", got)
	}
}

// TestConfigKeyGroupSiblingFiles_UsesSingleBatchSearch confirms the seeder
// asks the search layer for all key tokens at once. This protects analyzer
// latency on large repositories: the helper may choose rg or a bounded
// fallback internally, but the seeder itself must not loop one full scan per
// token.
func TestConfigKeyGroupSiblingFiles_UsesSingleBatchSearch(t *testing.T) {
	old := configKeyLiteralFileSearch
	defer func() { configKeyLiteralFileSearch = old }()

	calls := 0
	var gotTokens []string
	var gotRepo string
	var gotIgnoreCase bool
	configKeyLiteralFileSearch = func(tokens []string, repoRoot string, ignoreCase bool) map[string][]string {
		calls++
		gotTokens = append([]string(nil), tokens...)
		gotRepo = repoRoot
		gotIgnoreCase = ignoreCase
		return map[string][]string{
			"alpha":  {"internal/config/runtime.go"},
			"widget": {"internal/config/runtime.go"},
			"size":   {"internal/config/runtime.go", "internal/noise/size.go"},
		}
	}

	root := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: root}
	rm := types.RequestModel{
		Scenario: types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			ExactTargets: []string{"alpha_widget_size"},
		},
	}

	got := configKeyGroupSiblingFiles(ctx, rm)
	if calls != 1 {
		t.Fatalf("config seeder should call one batch search, got %d calls", calls)
	}
	if gotRepo != root || !gotIgnoreCase {
		t.Fatalf("batch search args = repo %q ignoreCase %v, want repo %q ignoreCase true", gotRepo, gotIgnoreCase, root)
	}
	wantTokens := []string{"alpha", "widget", "size"}
	if len(gotTokens) != len(wantTokens) {
		t.Fatalf("tokens = %v, want %v", gotTokens, wantTokens)
	}
	for i := range wantTokens {
		if gotTokens[i] != wantTokens[i] {
			t.Fatalf("tokens = %v, want %v", gotTokens, wantTokens)
		}
	}
	if len(got) == 0 || got[0] != "internal/config/runtime.go" {
		t.Fatalf("expected runtime schema file first, got %v", got)
	}
	for _, p := range got {
		if p == "internal/noise/size.go" {
			t.Fatalf("single-token noise should not survive distinct-token floor, got %v", got)
		}
	}
}

// TestConfigKeyGroupSiblingFiles_GateRejectsNonConfig confirms the precise
// typed gate: a request that is neither AnswerSubject=config_key nor
// Scenario=config_trace produces no seeds even when an entity happens to be
// snake_case — the seeder must not fire on arbitrary snake_case tokens.
func TestConfigKeyGroupSiblingFiles_GateRejectsNonConfig(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "internal/types/config.go", "// explore_per_tool_default_cap\n")
	ctx := &types.AgentContext{RepoRoot: root}
	rm := types.RequestModel{
		Scenario:      types.ScenarioGeneric,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"run_analyze_phase"}, // snake_case but NOT a config-key target
		},
	}
	if got := configKeyGroupSiblingFiles(ctx, rm); len(got) != 0 {
		t.Errorf("non-config-key request must produce no seeds, got %v", got)
	}
}
