package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSeedFidelity_RejectsRewrittenLines pins the 2026-05-02
// seed-fidelity gate: when the LLM emits a mermaid fence that
// modifies the file:line tokens the LogBundle seed had pinned, the
// validator rejects with a hint carrying the verbatim seed for
// retry. Without this gate, the LLM was observed silently replacing
// historical runtime line numbers with current-source positions.
func TestSeedFidelity_RejectsRewrittenLines(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/foo.go", Line: 250, Func: "callee"},
				{File: "internal/agent/foo.go", Line: 320, Func: "caller"},
			},
		}},
	})

	// Summary with mermaid fence that REWROTE seed lines :250/:320
	// to current-source :880/:660. Pre-fix this would have shipped
	// silently; the new gate must reject.
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"summary": "The failure originates in callee.\n\n" +
			"```mermaid\n" +
			"flowchart LR\n" +
			"    n0[\"innermost: internal/agent/foo.go:880 in callee\"]\n" +
			"    n1[\"caller: internal/agent/foo.go:660 in caller\"]\n" +
			"    n0 --> n1\n" +
			"```",
	})

	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected diagram_seed_fidelity rejection, got success")
	}
	if !strings.Contains(res.Summary, "diagram_seed_fidelity") {
		t.Fatalf("rejection should be diagram_seed_fidelity, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/agent/foo.go:250") {
		t.Fatalf("rejection should name missing seed pair :250: %s", res.Summary)
	}
}

// TestSeedFidelity_AcceptsVerbatimSeed confirms verbatim-copied
// seed lines pass: same RequestModel + LogBundle as the rejection
// test, but the mermaid fence keeps :250/:320 unchanged.
func TestSeedFidelity_AcceptsVerbatimSeed(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/foo.go", Line: 250, Func: "callee"},
				{File: "internal/agent/foo.go", Line: 320, Func: "caller"},
			},
		}},
	})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"summary": "The failure originates in callee.\n\n" +
			"```mermaid\n" +
			"flowchart LR\n" +
			"    n0[\"innermost: internal/agent/foo.go:250 in callee\"]\n" +
			"    n1[\"caller: internal/agent/foo.go:320 in caller\"]\n" +
			"    n0 --> n1\n" +
			"```",
		"citations": []map[string]interface{}{
			{"file": "internal/agent/foo.go", "line": 250},
			{"file": "internal/agent/foo.go", "line": 320},
		},
	})

	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("verbatim seed copy should pass; got: %s", res.Summary)
	}
}

// TestSeedFidelity_RejectsCollapsedFrames pins the no-collapse arm:
// the LLM must not reuse a single line number across multiple seed
// frames. This was the indexical-resolution error observed in the
// 2026-05-02 logtri_go investigation.
func TestSeedFidelity_RejectsCollapsedFrames(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/foo.go", Line: 250, Func: "callee"},
				{File: "internal/agent/foo.go", Line: 320, Func: "caller"},
			},
		}},
	})

	// Both nodes use :661 (the indexical-collapse failure mode).
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"summary": "The failure originates in callee.\n\n" +
			"```mermaid\n" +
			"flowchart LR\n" +
			"    n0[\"innermost: internal/agent/foo.go:661 in callee\"]\n" +
			"    n1[\"caller: internal/agent/foo.go:661 in caller\"]\n" +
			"    n0 --> n1\n" +
			"```",
	})

	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("collapsed-frames diagram should be rejected")
	}
	if !strings.Contains(res.Summary, "diagram_seed_fidelity") {
		t.Fatalf("rejection should be diagram_seed_fidelity, got: %s", res.Summary)
	}
}

// TestSeedFidelity_NoBundleSkipsValidation confirms the gate is a
// no-op when no LogBundle is attached — text-only questions must
// not be affected by this rule.
func TestSeedFidelity_NoBundleSkipsValidation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	// No SetLogTriage — bundle is nil.

	// Mermaid fence with arbitrary file:line; gate must not fire.
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"summary": "Architecture overview.\n\n" +
			"```mermaid\n" +
			"flowchart LR\n" +
			"    n0[\"internal/agent/foo.go:1\"]\n" +
			"    n1[\"internal/agent/bar.go:2\"]\n" +
			"    n0 --> n1\n" +
			"```",
		"citations": []map[string]interface{}{
			{"file": "internal/agent/foo.go", "line": 1},
			{"file": "internal/agent/bar.go", "line": 2},
		},
	})

	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		// May fail for other reasons (citation grounding etc.), but
		// must not fail with diagram_seed_fidelity specifically.
		if strings.Contains(res.Summary, "diagram_seed_fidelity") {
			t.Fatalf("seed-fidelity gate must skip when no bundle attached; got: %s", res.Summary)
		}
	}
}

// TestSeedFidelity_NoMermaidFenceSkipsValidation confirms the gate
// is a no-op when the LLM emitted a summary without any mermaid
// fence — the system's auto-injection path handles that case
// elsewhere, and this gate must not double-fire on it.
func TestSeedFidelity_NoMermaidFenceSkipsValidation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/foo.go", Line: 250, Func: "callee"},
				{File: "internal/agent/foo.go", Line: 320, Func: "caller"},
			},
		}},
	})

	// No mermaid fence in the summary at all.
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "The failure originates in callee, called from caller. The runtime stack lists the failure point and its parent frame.",
	})

	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "diagram_seed_fidelity") {
		t.Fatalf("no-mermaid summary must not trigger seed-fidelity; got: %s", res.Summary)
	}
}
