package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Start at the native producer, not hand-authored ToolResult summaries. The
// same dispatched query must reach both the explorer hint and tool authority.
func b1699ActualQueryContext(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	root := t.TempDir()
	capture := filepath.Join(root, "capture.ftrace")
	if err := os.WriteFile(capture, []byte("# tracer: nop\n"+
		" worker-4242 (4242) [001] .... 7.000000: tracing_mark_write: B|4242|compose\n"+
		" worker-4242 (4242) [001] .... 7.013000: tracing_mark_write: E|4242\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := parseOutputCtx("", "")
	ctx.Objective = "Explain the recorded operation and its current implementation."
	ctx.Stage, ctx.RepoRoot, ctx.WorkDir = types.StageExplore, root, root
	ctx.AttachedHitrace = capture
	ctx.Mutable = types.NewMutableState(ctx.Objective)
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true,
		CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{Index: 1, Label: "implementation", Required: true,
				Role: types.RequestedAnswerDimensionFunctionOrPurpose}},
		},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "pid": 4242, "limit": 4})
	query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !query.Success || len(query.Observations) == 0 {
		t.Fatalf("actual native query failed: %v %+v", err, query)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	return ctx, query
}

func TestB1699ActualQuerySoftRequiredHintPreservesSourceScope(t *testing.T) {
	ctx, query := b1699ActualQueryContext(t)
	before, _ := json.Marshal(ctx.AnalysisIR)
	agentAuthority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})
	busAuthority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(types.ToolBusContext(ctx, types.AgentExplorer), types.ObservationLedger{})
	for _, authority := range []types.RuntimeSourceAnswerAuthoritySnapshot{agentAuthority, busAuthority} {
		if !authority.CurrentSourceRequired || authority.CurrentSourceRequirement != types.RuntimeSourceRequirementSoft ||
			!authority.NeedsCurrentSourceEvidence || authority.CanHardBlockCompletion || !authority.CanDowngradeToCaveat {
			t.Fatalf("fixture must preserve a soft, unsatisfied current-source obligation: %+v", authority)
		}
	}
	eval := &explorerEvaluator{}
	_ = eval.BuildInitialInstruction(ctx, nil)
	sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, Iteration: 0, AllToolResults: []types.ToolResult{query}})
	if !sig.HintRequested || sig.HintKey != "explorer.mid-loop.external-observation-sufficient" {
		t.Fatalf("small runtime surface must still get its progress hint: %+v", sig)
	}
	for _, forbidden := range []string{"current-source lane is optional", "Prefer closing with", "instead of reading current-source sidecars by default"} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Errorf("same required source lane was mislabeled by hint %q: %s", forbidden, sig.Hint)
		}
	}
	for _, want := range []string{"external observation lane", "current-source", "soft", "waiver", "separate"} {
		if !strings.Contains(sig.Hint, want) {
			t.Errorf("scoped hint must preserve %q: %s", want, sig.Hint)
		}
	}
	after, _ := json.Marshal(ctx.AnalysisIR)
	if string(before) != string(after) {
		t.Fatal("hint must not rewrite the model's typed source obligation")
	}
}

func TestB1699PublicSoftSourceStartAndEmitSkipDoNotEraseObligation(t *testing.T) {
	ctx, query := b1699ActualQueryContext(t)
	// MCP has the same scope contract without the trace-specific start branch.
	ctx.AttachedHitrace = ""
	ctx.TurnRouteHint.Source = "external_tool"
	eval := &explorerEvaluator{}
	start := eval.BuildInitialInstruction(ctx, nil)
	if strings.Contains(start, "current-source evidence is optional") {
		t.Errorf("soft-required start must not call the source question optional: %s", start)
	}
	params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
		"kind": "direct", "subject": "recorded operation", "source": query.Observations[0].SourceRef.Path,
		"line_start": 2, "line_end": 3, "summary": "the runtime operation was recorded",
		"anchor_kind": "text_reference", "anchor_symbol": "compose",
	}}})
	result, err := (&tool.EmitEvidence{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || result.Repair == nil || result.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("real runtime/source rejection must stay a soft external handoff: %v %+v", err, result)
	}
	if strings.Contains(result.Summary, "This turn's completion does not require current-source citations") {
		t.Fatalf("a waived citation-count floor is not a waiver of the source question: %s", result.Summary)
	}
}

func TestB1699LegacyExternalRouteWithoutOptionalModeKeepsSoftObligation(t *testing.T) {
	// Preserve the original pre-mode payload of the older completion-hint test.
	// Its external_tool label alone never discharged the default source lane.
	ctx := parseOutputCtx("", "")
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", Confidence: 0.9}
	ctx.Mutable = types.NewMutableState("legacy external route")
	eval := &explorerEvaluator{}
	_ = eval.BuildInitialInstruction(ctx, nil)
	sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, AllMCPResponses: []types.MCPResponse{{
		ServerName: "fixture", Success: true, ResourceURI: "mcp://fixture/rows",
		Observations: []types.MCPTypedObservation{{Summary: "operation entered", LineStart: 7, LineEnd: 7}, {Summary: "operation completed", LineStart: 12, LineEnd: 12}},
	}}})
	if !sig.HintRequested || strings.Contains(sig.Hint, "current-source lane is optional") || !strings.Contains(sig.Hint, "required (soft)") {
		t.Fatalf("legacy payload must retain its actual soft-required source meaning: %+v", sig)
	}
}

func TestB1699ActualQueryAndSourceReadPreserveSatisfiedMixedLane(t *testing.T) {
	ctx, query := b1699ActualQueryContext(t)
	source := filepath.Join(ctx.RepoRoot, "parser.go")
	if err := os.WriteFile(source, []byte("package fixture\nfunc Parse() bool { return true }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"path": source, "line_offset": 0, "limit": 2})
	read, err := (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read failed: %v %+v", err, read)
	}
	ctx.Mutable.AppendDispatchToolResult(read)
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})
	if !authority.CurrentSourceSatisfied || authority.NeedsCurrentSourceEvidence || !authority.HasMixedRuntimeCurrentSourceCarrier() || !authority.CanCompleteWithCombinedProof {
		t.Fatalf("actual source proof must satisfy its separate lane: %+v", authority)
	}
	eval := &explorerEvaluator{}
	_ = eval.BuildInitialInstruction(ctx, nil)
	sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, AllToolResults: []types.ToolResult{query, read}})
	if !sig.HintRequested || !strings.Contains(sig.Hint, "Use relevant grounded source evidence") ||
		!strings.Contains(sig.Hint, "still-missing implementation facts") || strings.Contains(sig.Hint, "current-source lane is optional") ||
		strings.Contains(sig.Hint, "must read") || strings.Contains(sig.Hint, "source proof is missing") {
		t.Fatalf("satisfied mixed lane must reuse available proof, not require a fresh read: %+v", sig)
	}
}

func TestB1699ActualQueryPreservesOptionalExcludedAndPreciseBoundaries(t *testing.T) {
	for _, mode := range []string{"optional", "excluded", "precise"} {
		t.Run(mode, func(t *testing.T) {
			ctx, query := b1699ActualQueryContext(t)
			rm := &ctx.AnalysisIR.RequestModel
			switch mode {
			case "optional":
				ctx.TurnRouteHint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceOptional
				rm.RequestedAnswerDimensions = nil
			case "excluded":
				rm.ExternalObservationPolicy.CurrentSourceMode = types.ExternalObservationCurrentSourceExclude
				rm.ExternalObservationPolicy.ExclusionKind = types.ExternalObservationSourceExclusionExplicitUserBoundary
				rm.ExternalObservationPolicy.SourceQuotes = []string{"only the attached observations, no source analysis"}
			case "precise":
				rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "internal/parser.go", Confidence: 0.95}}
			}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})
			eval := &explorerEvaluator{}
			_ = eval.BuildInitialInstruction(ctx, nil)
			sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, AllToolResults: []types.ToolResult{query}})
			if mode == "precise" {
				if !authority.CanHardBlockCompletion || authority.CanDowngradeToCaveat || sig.HintKey == "explorer.mid-loop.external-observation-sufficient" {
					t.Fatalf("precise source proof must not acquire soft external closure guidance: %+v / %+v", authority, sig)
				}
			} else if authority.CurrentSourceRequired || !sig.HintRequested || !strings.Contains(sig.Hint, "current-source lane is optional") || !strings.Contains(sig.Hint, "emit_investigation_complete") {
				t.Fatalf("optional/excluded source must retain runtime-only closure guidance: %+v / %+v", authority, sig)
			}
		})
	}
}

func TestB1699ActualQueryPreservesTypedCaveatWaiver(t *testing.T) {
	ctx, query := b1699ActualQueryContext(t)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"current implementation"}, Confidence: 0.9,
	}
	result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{
		"reason":"The trace records the operation's begin and end. The current implementation is not verified; preserve this as an observation-only boundary.",
		"confidence":"high","result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_trace","rationale":"The recorded operation is runtime evidence; implementation details remain unverified."}
	}`))
	if err != nil || !result.Success || ctx.Mutable.InvestigationCompleteReason() == "" || !ctx.Mutable.StableEvidenceFloorWaiver().IsActive() {
		t.Fatalf("scoped hints must not harden soft source obligations or remove the accepted typed waiver: %v %+v", err, result)
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})
	if !authority.CurrentSourceRequired || authority.CurrentSourceRequirement != types.RuntimeSourceRequirementSoft || authority.CanHardBlockCompletion || !authority.CanDowngradeToCaveat {
		t.Fatalf("a citation waiver must not mutate the source obligation: %+v", authority)
	}
	// The accepted waiver may lower the count floor, but a later external-row
	// handoff must describe the still-present source scope faithfully.
	params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
		"kind": "direct", "subject": "recorded operation", "source": query.Observations[0].SourceRef.Path,
		"line_start": 2, "line_end": 3, "summary": "the runtime operation was recorded",
		"anchor_kind": "text_reference", "anchor_symbol": "compose",
	}}})
	skip, err := (&tool.EmitEvidence{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !skip.Success || skip.Repair == nil || skip.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("runtime rows must still use the external handoff after accepted waiver: %v %+v", err, skip)
	}
	if strings.Contains(skip.Summary, "This turn's completion does not require current-source citations") {
		t.Fatalf("citation-count waiver must not erase the separate soft source question in guidance: %s", skip.Summary)
	}
}
