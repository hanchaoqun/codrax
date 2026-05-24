// Package tool — emit_investigation_complete_principal_span_test.go
// (2026-05-12).
//
// Mirrors emit_investigation_complete_waiver_test.go for the typed
// principal_span_waiver escape: validation rejects bad payloads,
// stores on acceptance, clears on explicit retraction, and bypasses
// the callChainPrincipalSpanDowngrade gate when active.
package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitInvestigationComplete_PrincipalSpanWaiver_RejectsInvalidReason(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"investigation done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"not_a_real_reason","rationale":"x"}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("invalid reason must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "principal_span_waiver.reason") {
		t.Errorf("rejection must name the offending field: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "endpoints_directly_adjacent") {
		t.Errorf("rejection must surface the accepted enum values: %q", res.Summary)
	}
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("rejected waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_RequiresRationale(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"endpoints_directly_adjacent","rationale":"   "}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("whitespace rationale must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "rationale") {
		t.Errorf("rejection must name the rationale requirement: %q", res.Summary)
	}
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("rejected waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_RequiresReason(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"rationale":"intermediate is plumbing only"}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("missing reason must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "principal_span_waiver.reason") {
		t.Errorf("rejection must name the reason requirement: %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_AcceptsAllSixReasons(t *testing.T) {
	for _, reason := range types.PrincipalSpanWaiverReasonValues() {
		t.Run(string(reason), func(t *testing.T) {
			mut := types.NewMutableState("trace foo to bar")
			params := `{
				"reason":"done",
				"confidence":"high",
				"result_kind":"resolved",
				"principal_span_waiver":{"reason":"` + string(reason) + `","rationale":"concrete one-sentence reason"}
			}`
			res := runEIC(t, mut, params)
			if !res.Success {
				// Other completion gates may still fail this — but a
				// gate failure must NOT be the typed-waiver validation.
				// Surface the offending summary so we can audit.
				if strings.Contains(res.Summary, "principal_span_waiver") {
					t.Fatalf("typed reason %q should pass schema validation; got %q", reason, res.Summary)
				}
			}
			stored := mut.PrincipalSpanWaiver()
			if stored == nil || stored.Reason != reason {
				t.Errorf("stored waiver mismatch: want reason=%q got=%+v", reason, stored)
			}
		})
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_ClearRejectsWithNewWaiver(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"inlined_call","rationale":"r"},
		"clear_principal_span_waiver":true
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("clear+set combo must reject")
	}
	if !strings.Contains(res.Summary, "clear_principal_span_waiver") {
		t.Errorf("rejection must name the conflicting flags: %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_ClearRetracts(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverEndpointsDirectlyAdjacent,
		Rationale: "prior",
	})
	if mut.PrincipalSpanWaiver() == nil {
		t.Fatalf("pre-state: waiver should be present")
	}
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"clear_principal_span_waiver":true
	}`
	_ = runEIC(t, mut, params)
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("clear must retract: still have %+v", mut.PrincipalSpanWaiver())
	}
}

func TestPrincipalSpanWaiver_BypassesGate(t *testing.T) {
	// Manufacture the gate's preconditions: IntentTrace + two endpoint
	// entities + emitted evidence at source and sink with no
	// intermediate item. Without a waiver the gate must reject; with
	// an active waiver the gate must pass through.
	// Span must exceed the gate's minSpanForLateCoverageGate (120 lines)
	// so the hard gate is actually exercised — anything shorter is
	// treated as a compact straight-line helper that does not need
	// intermediate evidence in the first place.
	makeCtx := func() *types.BusContext {
		mut := types.NewMutableState("trace foo to bar")
		mut.AppendEvidence([]types.EvidenceItem{
			{
				ID:         "ev1",
				Kind:       types.EvidenceDirect,
				AnchorKind: types.AnchorCall,
				Subject:    "foo",
				Object:     "internal",
				Source:     "x.go",
				LineStart:  10,
				LineEnd:    10,
			},
			{
				ID:         "ev2",
				Kind:       types.EvidenceDirect,
				AnchorKind: types.AnchorCall,
				Subject:    "internal",
				Object:     "bar",
				Source:     "x.go",
				LineStart:  300,
				LineEnd:    300,
			},
		})
		return &types.BusContext{
			Mutable: mut,
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					RawRequest: "trace foo to bar",
					Intent:     types.IntentTrace,
					AnalyzerHints: types.AnalyzerHints{
						MentionedEntities: []string{"foo", "bar"},
					},
				},
			},
		}
	}

	t.Run("no waiver - gate fires", func(t *testing.T) {
		ctx := makeCtx()
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("gate should fire when no intermediate evidence and no waiver")
		}
		if !strings.Contains(got, "principal span") && !strings.Contains(got, "intermediate evidence") {
			t.Errorf("downgrade message should describe the span gap: %q", got)
		}
	})

	t.Run("active waiver - gate bypasses", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    types.PrincipalSpanWaiverNoIntermediateUserCode,
			Rationale: "between foo and bar are nil-check plus logger setup",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got != "" {
			t.Errorf("active waiver must bypass the gate; got downgrade=%q", got)
		}
	})

	t.Run("waiver with empty rationale - gate still fires", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    types.PrincipalSpanWaiverInlinedCall,
			Rationale: "",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("waiver with empty rationale must NOT bypass (IsActive()=false)")
		}
	})

	t.Run("waiver with invalid reason - gate still fires", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    "made_up_reason",
			Rationale: "trying to sneak past",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("waiver with invalid reason must NOT bypass (IsActive()=false)")
		}
	})
}

func TestQueueProactiveCallChainClosureRepairs_QueuesPrincipalSpanBeforeCompletion(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:         "source",
			Kind:       types.EvidenceDirect,
			AnchorKind: types.AnchorCall,
			Subject:    "foo",
			Object:     "handoff",
			Source:     "x.go",
			LineStart:  10,
		},
		{
			ID:         "sink",
			Kind:       types.EvidenceDirect,
			AnchorKind: types.AnchorCall,
			Subject:    "handoff",
			Object:     "bar",
			Source:     "x.go",
			LineStart:  300,
		},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "trace foo to bar",
				Intent:     types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqCallChain),
					MentionedEntities: []string{"foo", "bar"},
				},
			},
		},
	}

	if !QueueProactiveCallChainClosureRepairs(ctx) {
		t.Fatal("expected proactive call-chain span repair to queue")
	}
	repairs := mut.EvidenceClosure().ActiveRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("active repairs = %+v, want read_file repair", repairs)
	}
	if len(repairs[0].Files) != 1 || repairs[0].Files[0] != "x.go" {
		t.Fatalf("repair files = %+v, want x.go", repairs[0].Files)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0].Start <= 10 || repairs[0].LineRanges[0].End >= 300 {
		t.Fatalf("repair line ranges = %+v, want interior source→sink span", repairs[0].LineRanges)
	}
	pending := mut.EvidenceClosure().PendingReads()
	if len(pending) != 1 || len(pending[0].LineRanges) != 1 {
		t.Fatalf("pending read should carry the surgical range too, got %+v", pending)
	}
}

func TestQueueProactiveCallChainClosureRepairs_AlreadyReadSpanQueuesEmitEvidence(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "source", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, Subject: "foo", Object: "handoff", Source: "x.go", LineStart: 10},
		{ID: "sink", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, Subject: "handoff", Object: "bar", Source: "x.go", LineStart: 300},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"x.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{"x.go": {{Start: 1, End: 300}}})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "trace foo to bar",
				Intent:     types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqCallChain),
					MentionedEntities: []string{"foo", "bar"},
				},
			},
		},
	}

	if !QueueProactiveCallChainClosureRepairs(ctx) {
		t.Fatal("expected proactive already-read call-chain span repair to queue")
	}
	repairs := closure.ActiveRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("active repairs = %+v, want emit_evidence repair", repairs)
	}
	if len(closure.PendingReads()) != 0 {
		t.Fatalf("already-read span should not queue pending reads, got %+v", closure.PendingReads())
	}
}

func TestCallChainQualifiedIntermediateDowngrade_RequiresTypedHandoffForReadQualifiedCalls(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:           "start",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorDefinition,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "buildAnalysisIR",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1289,
		},
		{
			ID:           "compile",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "compiler.Compile",
			Subject:      "buildAnalysisIR",
			Object:       "compiler.Compile",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1842,
		},
		{
			ID:           "bind",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "binder.BindByRelevance",
			Subject:      "buildAnalysisIR",
			Object:       "binder.BindByRelevance",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1868,
		},
		{
			ID:           "gate",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "gate.RunWith",
			Subject:      "buildAnalysisIR",
			Object:       "gate.RunWith",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1994,
		},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "trace buildAnalysisIR to gate.Run",
				Intent:     types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1842,
		"out := compiler.Compile(rm, sig)",
		"if out.AnswerContract.Language == \"\" {",
		"out.AnswerContract.Language = rm.Language",
		"}",
		"",
		"// Risk matrix and hypothesis planning.",
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"",
		"// Recompute budget with the real hypothesis count.",
		"sig.HypothesisCount = len(hypotheses)",
		"compiler.RecomputeBudget(&out, rm, sig)",
		"",
		"// Amplifier post-compile pass.",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"recordReconcileObservation(ctxMutable(ctx), reconcileEvent(",
		"obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,",
		"))",
		"}",
		"",
		"// Relevance-based hypothesis binding.",
		"if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {",
		"return nil, fmt.Errorf(\"binder: %w\", err)",
		"}",
	)

	got := callChainQualifiedIntermediateDowngrade(ctx, &types.EvidenceClosure{})
	if got == "" {
		t.Fatalf("qualified intermediate gate should require typed handoff for already-read calls")
	}
	for _, want := range []string{"risk.Evaluate", "hdp.Plan", "compiler.RecomputeBudget", "amplifier.AmplifyPostCompile"} {
		if !strings.Contains(got, want) {
			t.Fatalf("downgrade missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fmt.Errorf") {
		t.Fatalf("error-return helper should not be promoted as a principal intermediate:\n%s", got)
	}
}

func TestCallChainQualifiedIntermediateDowngrade_PassesWhenCandidatesAreTyped(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	base := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 1289},
		{ID: "compile", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile", Subject: "buildAnalysisIR", Object: "compiler.Compile", Source: "internal/agent/analyzer.go", LineStart: 1842},
		{ID: "risk", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "risk.Evaluate", Subject: "buildAnalysisIR", Object: "risk.Evaluate", Source: "internal/agent/analyzer.go", LineStart: 1848},
		{ID: "hdp", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "hdp.Plan", Subject: "buildAnalysisIR", Object: "hdp.Plan", Source: "internal/agent/analyzer.go", LineStart: 1849},
		{ID: "budget", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.RecomputeBudget", Subject: "buildAnalysisIR", Object: "compiler.RecomputeBudget", Source: "internal/agent/analyzer.go", LineStart: 1853},
		{ID: "post", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "amplifier.AmplifyPostCompile", Subject: "buildAnalysisIR", Object: "amplifier.AmplifyPostCompile", Source: "internal/agent/analyzer.go", LineStart: 1861},
		{ID: "bind", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "binder.BindByRelevance", Subject: "buildAnalysisIR", Object: "binder.BindByRelevance", Source: "internal/agent/analyzer.go", LineStart: 1868},
		{ID: "gate", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 1994},
	}
	mut.AppendEvidence(base)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "trace buildAnalysisIR to gate.Run",
				Intent:     types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1848,
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"sig.HypothesisCount = len(hypotheses)",
		"compiler.RecomputeBudget(&out, rm, sig)",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"}",
	)

	if got := callChainQualifiedIntermediateDowngrade(ctx, &types.EvidenceClosure{}); got != "" {
		t.Fatalf("typed qualified candidates should satisfy the gate, got:\n%s", got)
	}
}

func TestCallChainQualifiedIntermediateDowngrade_DenseCoverageKeepsCandidatesAdvisory(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	evidence := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 100},
		{ID: "a", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile", Subject: "buildAnalysisIR", Object: "compiler.Compile", Source: "internal/agent/analyzer.go", LineStart: 120},
		{ID: "b", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "risk.Evaluate", Subject: "buildAnalysisIR", Object: "risk.Evaluate", Source: "internal/agent/analyzer.go", LineStart: 140},
		{ID: "c", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "hdp.Plan", Subject: "buildAnalysisIR", Object: "hdp.Plan", Source: "internal/agent/analyzer.go", LineStart: 160},
		{ID: "d", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "amplifier.AmplifyPostCompile", Subject: "buildAnalysisIR", Object: "amplifier.AmplifyPostCompile", Source: "internal/agent/analyzer.go", LineStart: 180},
		{ID: "e", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "binder.BindByRelevance", Subject: "buildAnalysisIR", Object: "binder.BindByRelevance", Source: "internal/agent/analyzer.go", LineStart: 200},
		{ID: "f", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "analyzerRequiredFiles", Subject: "buildAnalysisIR", Object: "analyzerRequiredFiles", Source: "internal/agent/analyzer.go", LineStart: 220},
		{ID: "gate", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 240},
	}
	mut.AppendEvidence(evidence)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "trace buildAnalysisIR to gate.Run",
				Intent:     types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 120,
		"out := compiler.Compile(rm, sig)",
		"ctx.Mutable.RequestModel()",
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {",
		"required := analyzerRequiredFiles(rm)",
		"ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})",
	)

	closure := &types.EvidenceClosure{}
	if got := callChainQualifiedIntermediateDowngrade(ctx, closure); got != "" {
		t.Fatalf("dense structured call-chain coverage should keep residual qualified calls advisory, got:\n%s", got)
	}
	for _, repair := range closure.ActiveRepairs() {
		if repair.Origin == "pre_complete.call_chain_qualified_intermediate" {
			t.Fatalf("dense residual qualified calls must not become an active/blocking repair, got %+v", closure.ActiveRepairs())
		}
	}
	var found bool
	for _, repair := range closure.PendingRepairs() {
		if repair.Origin == "pre_complete.call_chain_qualified_intermediate" && repair.Kind == types.RepairEmitEvidence && repair.Advisory {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("residual qualified calls should remain visible as advisory repair, got %+v", closure.PendingRepairs())
	}
}

func TestPreCompleteCallChain_PrincipalAggregateMemberSetSkipsSpanExpansion(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 100},
		{ID: "mid", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize", Subject: "buildAnalysisIR", Object: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 160},
		{ID: "gate", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 300},
	}
	makeCtx := func() *types.BusContext {
		mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
		mut.AppendEvidence(evidence)
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "read_file",
			Success:  true,
			Summary: strings.Join([]string{
				"[internal/agent/analyzer.go: showing lines 160-160 of 320 total]",
				"  160│ rm.TermGraph = normalizer.Normalize(rm.TermGraph, resolver)",
				"[internal/agent/analyzer.go: showing lines 300-300 of 320 total]",
				"  300│ ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})",
			}, "\n"),
		})
		return &types.BusContext{
			Mutable: mut,
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					RawRequest: "trace buildAnalysisIR to gate.Run",
					Intent:     types.IntentTrace,
					AnalyzerHints: types.AnalyzerHints{
						Kind:              string(types.ReqCallChain),
						MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
					},
				},
			},
		}
	}

	if got := preCompleteContractCheckWithEvidence(makeCtx(), "", evidence); !strings.Contains(got, "call-chain principal span") {
		t.Fatalf("precondition: span gate should fire without aggregate member_set, got:\n%s", got)
	}

	aggregateFacts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "principal call chain",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"normalizer.Normalize (internal/agent/analyzer.go:160)", "gate.RunWith (internal/agent/analyzer.go:300)"},
		SupportRefs: []string{
			"normalizer.Normalize: internal/agent/analyzer.go:160",
			"gate.RunWith: internal/agent/analyzer.go:300",
		},
	}}
	skipCtx := makeCtx()
	if !callChainAggregateMemberSetCompletesPrincipalBoundary(skipCtx, aggregateFacts, evidence) {
		support := buildAggregateMemberSupportIndexWithEvidence(skipCtx, evidence)
		var unusable []string
		for _, member := range aggregateFacts[0].Members {
			if !aggregateMemberSetMemberUsable(aggregateFacts[0], member, support) {
				unusable = append(unusable, member)
			}
		}
		startHint, endHint, _ := callChainPrincipalEndpointHints(skipCtx.AnalysisIR.RequestModel)
		span, spanOK := callChainPrincipalSpanContextForEvidence(evidence, startHint, endHint)
		t.Fatalf("helper should accept principal member_set; spanOK=%v spanSupport=%v span=%+v unusable=%v support_refs=%v", spanOK, aggregateMemberSetHasSupportInsideCallChainSpan(aggregateFacts[0], span), span, unusable, aggregateFacts[0].SupportRefs)
	}
	if got := preCompleteContractCheckWithEvidence(skipCtx, "", evidence, aggregateFacts); got != "" {
		t.Fatalf("principal aggregate member_set should define the model-owned call-chain boundary, got:\n%s", got)
	}
}
