package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Pin the regression that motivated the dedup pass: the df1 post-#5
// eval captured two answer chains that were semantically identical
// but carried different wording (`NewSubExplorer(...)` vs
// `NewSubExplorer(deps)`; `SubExplorer.Name` vs `Name`) and different
// Producer tags (`bridge_literal` vs `concrete_values`). Both reached
// the finalizer as separate top-5 entries, wasting slots that should
// have held genuinely distinct chains. See
// `eval/results/df1-20260413-07*/run-1.logs/...` for the raw
// `answer_chain[0]` / `answer_chain[1]` lines.
func TestDedupeResolutionChains_PrefersBridgeLiteralOverConcreteValues(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   "`RegisterDefaultSubAgents()` binds NewSubExplorer → `Name()` returns \"explorer\"",
			Summary:   "`RegisterDefaultSubAgents()` binds NewSubExplorer → `Name()` returns \"explorer\"",
			Producer:  "concrete_values",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(...) → `SubExplorer.Name()` returns \"explorer\"",
			Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(...) → `SubExplorer.Name()` returns \"explorer\"",
			Source:    "internal/agent/subagent.go",
			LineStart: 63,
			Producer:  "bridge_literal",
		},
	}
	out := dedupeResolutionChains(items)
	if len(out) != 1 {
		t.Fatalf("expected 1 deduped chain, got %d: %+v", len(out), out)
	}
	if out[0].Producer != "bridge_literal" {
		t.Errorf("expected bridge_literal winner, got producer=%q", out[0].Producer)
	}
	if out[0].Source == "" {
		t.Errorf("expected the bridge_literal item's Source to be preserved")
	}
}

func TestDedupeResolutionChains_InputOrderDoesNotMatter(t *testing.T) {
	bridge := types.EvidenceItem{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Subject:   "`A()` binds ONLY NewB(...) → `B.Name()` returns \"x\"",
		Summary:   "`A()` binds ONLY NewB(...) → `B.Name()` returns \"x\"",
		Producer:  "bridge_literal",
	}
	concrete := types.EvidenceItem{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Subject:   "`A()` binds NewB → `Name()` returns \"x\"",
		Summary:   "`A()` binds NewB → `Name()` returns \"x\"",
		Producer:  "concrete_values",
	}
	for _, order := range [][]types.EvidenceItem{
		{concrete, bridge},
		{bridge, concrete},
	} {
		out := dedupeResolutionChains(order)
		if len(out) != 1 || out[0].Producer != "bridge_literal" {
			t.Errorf("order=%v: expected single bridge_literal winner, got %+v", order, out)
		}
	}
}

func TestDedupeResolutionChains_DifferentLiteralsAreDistinct(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   "`Register()` binds NewFoo → `Foo.Name()` returns \"a\"",
			Summary:   "`Register()` binds NewFoo → `Foo.Name()` returns \"a\"",
			Producer:  "concrete_values",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   "`Register()` binds NewBar → `Bar.Name()` returns \"b\"",
			Summary:   "`Register()` binds NewBar → `Bar.Name()` returns \"b\"",
			Producer:  "concrete_values",
		},
	}
	out := dedupeResolutionChains(items)
	if len(out) != 2 {
		t.Fatalf("different terminal literals must not collapse, got %d items", len(out))
	}
}

func TestDedupeResolutionChains_DifferentFirstMethodsAreDistinct(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterToolDefaults()` binds NewGrep → `GrepTool.Name()` returns \"grep\"",
			Producer:  "concrete_values",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterSkillDefaults()` binds NewGrep → `GrepTool.Name()` returns \"grep\"",
			Producer:  "concrete_values",
		},
	}
	out := dedupeResolutionChains(items)
	if len(out) != 2 {
		t.Fatalf("different register roots must not collapse, got %d items", len(out))
	}
}

func TestDedupeResolutionChains_NonChainItemsPassThrough(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "returns",
			Subject:   "Foo.Name",
			Object:    "\"bar\"",
			Producer:  "concrete_values",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`R()` binds NewX → `X.Name()` returns \"x\"",
			Producer:  "concrete_values",
		},
	}
	out := dedupeResolutionChains(items)
	if len(out) != 2 {
		t.Fatalf("non-chain items must pass through, got %d", len(out))
	}
}

func TestDedupeResolutionChains_EmptyKeyChainsAllSurvive(t *testing.T) {
	// Chains with no backtick tokens (shouldn't normally happen but
	// guard against over-collapsing when the summary lacks the
	// structural anchors the dedup key depends on).
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Summary: "opaque chain a", Producer: "concrete_values"},
		{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Summary: "opaque chain b", Producer: "concrete_values"},
	}
	out := dedupeResolutionChains(items)
	if len(out) != 2 {
		t.Fatalf("keyless chains must not be merged, got %d", len(out))
	}
}

func TestNormalizeChainKey_QualifierAndArgsIgnoredOnTerminal(t *testing.T) {
	a := normalizeChainKey("`Register()` binds NewFoo → `Foo.Name()` returns \"x\"")
	b := normalizeChainKey("`Register()` binds NewFoo → `Name()` returns \"x\"")
	c := normalizeChainKey("`Register(deps)` binds NewFoo(...) → `Foo.Name(r *Reader)` returns \"x\"")
	if a != b || a != c {
		t.Errorf("keys should be equal: a=%q b=%q c=%q", a, b, c)
	}
	d := normalizeChainKey("`Register()` binds NewFoo → `Foo.Name()` returns \"y\"")
	if a == d {
		t.Errorf("different literals must produce different keys: a=%q d=%q", a, d)
	}
}
