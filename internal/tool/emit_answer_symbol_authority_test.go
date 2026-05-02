package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// withAuthorityEnabled wraps a test in the authority feature gate and
// restores the prior state on exit so tests stay independent.
func withAuthorityEnabled(t *testing.T, on bool, run func()) {
	t.Helper()
	prev := authority.Enabled()
	authority.Enable(on)
	defer authority.Enable(prev)
	run()
}

// TestEmitAnswerSymbol_AuthorityGateOffPreservesZeroValue defends
// the byte-identical-legacy contract: with the master gate off, the
// new Authority field stays at its zero value across every emit
// regardless of underlying evidence.
func TestEmitAnswerSymbol_AuthorityGateOffPreservesZeroValue(t *testing.T) {
	withAuthorityEnabled(t, false, func() {
		tool := &EmitAnswerSymbol{}
		ctx := newAnswerSymbolCtx()
		// Pre-load evidence with non-zero authority to ensure the
		// gate is short-circuiting BEFORE the projection runs.
		ctx.Mutable.AppendEvidence([]types.EvidenceItem{
			{
				Source:    "internal/x.go",
				LineStart: 10,
				Authority: types.AuthorityHistorical,
			},
		})
		params := json.RawMessage(`{
            "items": [
              {"name": "Foo", "file": "internal/x.go", "line": 10, "kind": "function"}
            ],
            "completeness": "lower_bound"
        }`)
		res, err := tool.Execute(ctx, params)
		if err != nil || !res.Success {
			t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
		}
		got, _ := ctx.Mutable.EmittedAnswerSymbols()
		if len(got) != 1 {
			t.Fatalf("want 1 symbol; got %d", len(got))
		}
		if got[0].Authority != types.AuthorityUnknown {
			t.Errorf("gate off: Authority = %q; want Unknown (zero)", got[0].Authority)
		}
	})
}

// TestEmitAnswerSymbol_AuthorityWeakestUnderlyingWins exercises the
// load-bearing "weakest underlying evidence wins" rule. Two evidence
// items co-anchor the same (file, line); one factual, one historical.
// The symbol's Authority must pin to historical — even one weak
// support means prose downstream must hedge.
func TestEmitAnswerSymbol_AuthorityWeakestUnderlyingWins(t *testing.T) {
	withAuthorityEnabled(t, true, func() {
		tool := &EmitAnswerSymbol{}
		ctx := newAnswerSymbolCtx()
		ctx.Mutable.AppendEvidence([]types.EvidenceItem{
			{Source: "internal/x.go", LineStart: 10, Authority: types.AuthorityFactual},
			{Source: "internal/x.go", LineStart: 10, Authority: types.AuthorityHistorical},
		})
		params := json.RawMessage(`{
            "items": [
              {"name": "Foo", "file": "internal/x.go", "line": 10, "kind": "function"}
            ],
            "completeness": "lower_bound"
        }`)
		res, err := tool.Execute(ctx, params)
		if err != nil || !res.Success {
			t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
		}
		got, _ := ctx.Mutable.EmittedAnswerSymbols()
		if got[0].Authority != types.AuthorityHistorical {
			t.Errorf("Authority = %q; want historical (weakest support)", got[0].Authority)
		}
	})
}

// TestEmitAnswerSymbol_AuthorityNoMatchingEvidenceLeavesUnknown
// asserts the safety default: when no evidence anchors the symbol,
// LowestAuthorityFor returns Unknown and the symbol's Authority is
// left at zero. Renderer treats Unknown as factual-equivalent.
func TestEmitAnswerSymbol_AuthorityNoMatchingEvidenceLeavesUnknown(t *testing.T) {
	withAuthorityEnabled(t, true, func() {
		tool := &EmitAnswerSymbol{}
		ctx := newAnswerSymbolCtx()
		// Evidence at a different file — no overlap.
		ctx.Mutable.AppendEvidence([]types.EvidenceItem{
			{Source: "internal/y.go", LineStart: 10, Authority: types.AuthorityFactual},
		})
		params := json.RawMessage(`{
            "items": [
              {"name": "Foo", "file": "internal/x.go", "line": 10, "kind": "function"}
            ],
            "completeness": "lower_bound"
        }`)
		res, err := tool.Execute(ctx, params)
		if err != nil || !res.Success {
			t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
		}
		got, _ := ctx.Mutable.EmittedAnswerSymbols()
		if got[0].Authority != types.AuthorityUnknown {
			t.Errorf("no matching evidence: Authority = %q; want Unknown", got[0].Authority)
		}
	})
}

// TestEmitAnswerSymbol_AuthorityFactualOnlySupportStaysFactual:
// when every supporting evidence item is factual, the symbol is
// also factual. Defends against an inverted comparator.
func TestEmitAnswerSymbol_AuthorityFactualOnlySupportStaysFactual(t *testing.T) {
	withAuthorityEnabled(t, true, func() {
		tool := &EmitAnswerSymbol{}
		ctx := newAnswerSymbolCtx()
		ctx.Mutable.AppendEvidence([]types.EvidenceItem{
			{Source: "internal/x.go", LineStart: 10, Authority: types.AuthorityFactual},
			{Source: "internal/x.go", LineStart: 12, Authority: types.AuthorityFactual},
		})
		params := json.RawMessage(`{
            "items": [
              {"name": "Foo", "file": "internal/x.go", "line": 10, "kind": "function"}
            ],
            "completeness": "lower_bound"
        }`)
		res, err := tool.Execute(ctx, params)
		if err != nil || !res.Success {
			t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
		}
		got, _ := ctx.Mutable.EmittedAnswerSymbols()
		if got[0].Authority != types.AuthorityFactual {
			t.Errorf("factual-only support: Authority = %q; want factual", got[0].Authority)
		}
	})
}
