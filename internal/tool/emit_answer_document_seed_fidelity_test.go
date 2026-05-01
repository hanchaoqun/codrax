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

// driftBoundedSeedFidelityCtx assembles a BusContext whose
// AnswerSurfacePlan resolves to AnswerSummarySurfaceDriftBoundedRootCause
// — Scenario=RootCause + LogBundle multi-frame + Shape=Explanation
// + LogSourceDriftAnchors non-empty (achieved by including frames
// whose lines do not match the evidence items' current-source
// positions). Used to test arm-routing in drift-bounded mode
// directly, bypassing the normalize pipeline that would otherwise
// rewrite p.Summary before our gate sees it.
func driftBoundedSeedFidelityCtx() *types.BusContext {
	ctx := newDocBusCtx("")
	ctx.Language = "en"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioRootCause,
			Intent:   types.IntentRootCause,
		},
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
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/foo.go",
			LineStart:       612,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "callee",
			GroundingStatus: types.GroundingGrounded,
			Producer:        EmitEvidenceProducer,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/foo.go",
			LineStart:       367,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "caller",
			GroundingStatus: types.GroundingGrounded,
			Producer:        EmitEvidenceProducer,
		},
	})
	return ctx
}

// TestSeedFidelity_DriftBoundedRejectsCollapse pins the 2026-05-02
// audit follow-up: in drift-bounded mode, the seed-fidelity gate
// must NOT skip arm 2 (no-collapse). When the system-rendered fence
// in normalizeLogSourceDriftSummarySurface falls back empty (surface
// items extraction failed AND CompiledDiagramFence empty), the LLM's
// user fence is appended as-is. If that user fence reused one
// observed line across multiple seed frames (the indexical-collapse
// failure observed in the logtri_go investigation), the gate must
// still reject — that error is wrong in every mode.
//
// Calls validateSummaryDiagramSeedFidelity directly to bypass the
// normalize pipeline that would otherwise rewrite the LLM's user
// fence with system-rendered content. The intent is to exercise
// the validator's drift-bounded arm-routing semantics.
func TestSeedFidelity_DriftBoundedRejectsCollapse(t *testing.T) {
	ctx := driftBoundedSeedFidelityCtx()
	if plan := answerSurfacePlan(ctx); plan == nil ||
		plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		t.Fatalf("test setup must trigger drift-bounded mode; got mode=%v",
			plan.SummarySurfaceMode)
	}
	collapsedSummary := "The failure originates in callee.\n\n" +
		"```mermaid\n" +
		"flowchart LR\n" +
		"    n0[\"innermost: internal/agent/foo.go:612 in callee\"]\n" +
		"    n1[\"caller: internal/agent/foo.go:612 in caller\"]\n" +
		"    n0 --> n1\n" +
		"```"
	err := validateSummaryDiagramSeedFidelity(collapsedSummary, ctx)
	if err == nil {
		t.Fatal("drift-bounded + collapsed-frames diagram should be rejected by arm 2")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Collapsed distinct frames") {
		t.Fatalf("drift-bounded rejection should name arm 2 'Collapsed distinct frames'; got: %s", msg)
	}
	if strings.Contains(msg, "Missing seed anchors") {
		t.Fatalf("arm 1 (missing seed pairs) must be waived in drift-bounded mode; got: %s", msg)
	}
}

// TestSeedFidelity_DriftBoundedAllowsRewrittenLines confirms the
// inverse direction: in drift-bounded mode the LLM is allowed to
// use current-source line numbers (the system-designed behaviour)
// without arm 1 falsely rejecting. As long as each seed frame maps
// to a DISTINCT current-source line, the gate passes.
func TestSeedFidelity_DriftBoundedAllowsRewrittenLines(t *testing.T) {
	ctx := driftBoundedSeedFidelityCtx()
	if plan := answerSurfacePlan(ctx); plan == nil ||
		plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		t.Fatalf("test setup must trigger drift-bounded mode; got mode=%v",
			plan.SummarySurfaceMode)
	}
	rewrittenSummary := "The failure originates in callee, called from caller.\n\n" +
		"```mermaid\n" +
		"flowchart LR\n" +
		"    n0[\"innermost: internal/agent/foo.go:612 in callee\"]\n" +
		"    n1[\"caller: internal/agent/foo.go:367 in caller\"]\n" +
		"    n0 --> n1\n" +
		"```"
	if err := validateSummaryDiagramSeedFidelity(rewrittenSummary, ctx); err != nil {
		t.Fatalf("drift-bounded + distinct rewritten lines should not trigger seed-fidelity; got: %v", err)
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
