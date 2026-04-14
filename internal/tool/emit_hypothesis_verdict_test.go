package tool

// P2.1 Session 1 Phase 3 — emit_hypothesis_verdict structural tests.
//
// Pin every branch of the validator that ships in P2.1 because the
// extractor (Turn B) drains this buffer through MutableState.MarkHypothesis
// (the D7 carve-out on the AnalysisIR header invariant). Anything we
// silently let through here ends up in the IR's HypothesisSet[*].Status
// field which downstream consumers (finalizer prompt builder, contract
// checker) read as authoritative.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newVerdictCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState(types.TaskList{})}
}

func TestEmitHypothesisVerdict_AcceptsValidBatch(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{
        "items": [
          {"hypothesis_id": "H1", "status": "confirmed", "rationale": "explorer.go:42 returns true literally", "citation": "internal/agent/explorer.go:42"},
          {"hypothesis_id": "H2", "status": "rejected", "citation": "internal/agent/finalizer.go:100-105"},
          {"hypothesis_id": "H3", "status": "inconclusive", "rationale": "no concrete return literal in the read window"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 3 {
		t.Fatalf("want 3 verdicts in buffer, got %d", len(got))
	}
	if got[0].Status != types.HypConfirmed {
		t.Errorf("[0].Status = %q, want %q", got[0].Status, types.HypConfirmed)
	}
	if got[1].Status != types.HypRejected || got[1].Citation != "internal/agent/finalizer.go:100-105" {
		t.Errorf("[1] mismatch: %+v", got[1])
	}
	if got[2].Status != types.HypInconclusive || got[2].Citation != "" {
		t.Errorf("[2] inconclusive should preserve empty citation: %+v", got[2])
	}
}

func TestEmitHypothesisVerdict_ConfirmedRequiresCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"confirmed","rationale":"trust me"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("confirmed without citation must be rejected")
	}
	if !strings.Contains(res.Summary, "requires a citation") {
		t.Errorf("expected citation diagnosis, got: %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectedRequiresCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"rejected"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("rejected without citation must be rejected")
	}
}

func TestEmitHypothesisVerdict_InconclusiveCitationOptional(t *testing.T) {
	// The whole point of inconclusive is that you investigated but
	// could not point at a definitive cite. A bare inconclusive
	// without rationale is allowed but lossy; pin that it parses.
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H7","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("inconclusive without citation should be accepted, got: %s", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectsUnknownStatus(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"maybe"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown status must be rejected")
	}
	if !strings.Contains(res.Summary, "unknown status") {
		t.Errorf("expected status enum diagnosis, got: %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectsMissingHypothesisID(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("missing hypothesis_id must be rejected")
	}
}

func TestEmitHypothesisVerdict_RejectsMalformedCitation(t *testing.T) {
	cases := []struct {
		name string
		cite string
	}{
		{"prose colon", "the function explorer.go: returns true"},
		{"line zero", "internal/agent/explorer.go:0"},
		{"non-numeric", "internal/agent/explorer.go:abc"},
		{"missing line", "internal/agent/explorer.go:"},
		{"reversed range", "internal/agent/explorer.go:50-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitHypothesisVerdict{}
			ctx := newVerdictCtx()
			params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"confirmed","citation":"` + tc.cite + `"}]}`)
			res, _ := tool.Execute(ctx, params)
			if res.Success {
				t.Errorf("malformed citation %q should be rejected", tc.cite)
			}
		})
	}
}

func TestEmitHypothesisVerdict_RejectsUnknownFields(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"inconclusive","note":"extra"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown field must be rejected")
	}
}

func TestEmitHypothesisVerdict_RejectsEmptyItems(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("empty items must be rejected")
	}
}

func TestEmitHypothesisVerdict_RequiresMutable(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := &types.BusContext{}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

// looksLikeCitation unit boundary: exercised through the tool above
// for the failure cases; this table covers positive and edge cases
// directly so future refactors of the helper cannot diverge from
// internal/orchestrator/contract_check.go's regex.
func TestLooksLikeCitation_Boundary(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"internal/agent/explorer.go:42", true},
		{"README.md:1", true},
		{"path/with-dashes/file.go:1234", true},
		{"path/file.go:50-75", true},
		{"path/file.go:50-50", true},
		{"a.b.c:10", true},
		{"path/file.go:0", false},
		{"path/file.go:01", true}, // leading-zero tolerated for parity with contract_check.go's `\d{1,6}` regex
		{"path/file.go", false},
		{"path/file.go:", false},
		{":42", false},
		{"prose with colon: 42", false},
		{"path/file.go:abc", false},
		{"path/file.go:10-", false},
		{"path/file.go:-10", false}, // dash at index 0 not handled as range
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			if got := looksLikeCitation(tc.s); got != tc.want {
				t.Errorf("looksLikeCitation(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}
