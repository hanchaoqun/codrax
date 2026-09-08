package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The query is deliberately one millisecond wider than the accepted request.
// Neither its complete state account nor its on-chain seat is disposable.
func traceRequestedScopeTestContext(queryEnd float64, explicit bool) *types.AgentContext {
	window := fmt.Sprintf("selected_window=2.000000..%.6f", queryEnd)
	total := (queryEnd - 2) * 1000
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/requested-scope/trace.ftrace"}
	ctx := tracePrincipalValueAuthorityTestContext("app-100", 100, []types.ObservationRecord{
		{ID: "rank", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: ref, Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:app-100", Subject: "app-100", Object: "runnable",
			Value: "0.020", Unit: "ms", Span: types.ObservationSpan{StartTs: 2.020, EndTs: 2.020020},
			RichNotes: []string{window, "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=0.020", "fix_direction=scheduling"}},
		{ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: ref, Predicate: "target_window_states", ClaimKey: "target_window_states:app-100", Subject: "app-100", Object: "state_partition",
			Value: fmt.Sprintf("%.3f", total), Unit: "ms", Span: types.ObservationSpan{StartTs: 2, EndTs: queryEnd},
			RichNotes: []string{window, "running=0", "runnable=0.020", fmt.Sprintf("sleep=%.3f", total-0.020), "d_state=0", "io_wait=0", fmt.Sprintf("total=%.3f", total)}},
	})
	if explicit {
		start, end := 2.0, 2.020
		ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000..2.020",
		}
	}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return ctx
}

func TestTraceRequestedScopeRecapDoesNotPromoteExplorationAccount(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	if !strings.Contains(got, "- supporting_state:") || strings.Contains(got, "- principal_state:") {
		t.Fatalf("a 21ms exploration account must remain visible without claiming the requested 20ms account:\n%s", got)
	}
	for _, want := range []string{"window=`2.000000..2.021000`", "accounted_total=21.000ms", "runnable=0.020ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("scope disclosure changed the original measurement %q:\n%s", want, got)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("scope authoring context mutated source observations")
	}
}

func TestTraceRequestedScopeExactAndUnspecifiedKeepPrincipalAccount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		end      float64
		explicit bool
	}{{"exact", 2.020, true}, {"unspecified", 2.021, false}} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderAnswerDocTracePrincipalValueAuthority(traceRequestedScopeTestContext(tc.end, tc.explicit))
			if !strings.Contains(got, "- principal_state:") || strings.Contains(got, "- supporting_state:") {
				t.Fatalf("exact/elected account lost its existing authority:\n%s", got)
			}
		})
	}
}

func TestTraceRequestedScopeUnknownObservationDoesNotBecomePrincipalState(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	ledger := answerDocObservationLedger(ctx)
	for i := range ledger.Records {
		ledger.Records[i].Span = types.ObservationSpan{}
		var notes []string
		for _, note := range ledger.Records[i].RichNotes {
			if !strings.HasPrefix(note, "selected_window=") {
				notes = append(notes, note)
			}
		}
		ledger.Records[i].RichNotes = notes
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: ledger.Records}}})
	got := renderAnswerDocTracePrincipalValueAuthority(ctx)
	if strings.Contains(got, "- principal_state:") {
		t.Fatalf("an observation without a query window cannot claim principal-window scope:\n%s", got)
	}
	// The existing state-account compiler requires query coordinates. The
	// ordinary observation remains available without manufacturing an account.
	observations := renderAnswerDocObservationLedger(ctx)
	if note := types.ResolveTraceQueryWindowScope(ledger.RuntimeArtifactScopeProfile, 0, 0).Format("en"); !strings.Contains(observations, "21.000") || !strings.Contains(observations, note) {
		t.Fatalf("unknown query scope must retain the original observation and boundary:\n%s", observations)
	}
}

func TestTraceRequestedScopeFinalTailRetainsBothWindows(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	set := types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))
	if len(set.Projections) != 1 || set.Projections[0].WindowEndTs != 2.021 {
		t.Fatalf("fixture must retain the real exploration projection: %+v", set)
	}
	for name, got := range map[string]string{
		"time":     renderTraceFinalTimeRoleAuthority(set),
		"selected": renderTraceFinalSelectedWindowAuthority(set, "absent"),
		"reader":   renderTraceFinalReaderDecisionCards(set, nil, "en", nil, types.TraceWakeupTargetCPUIntegrity{}, false),
	} {
		if !strings.Contains(got, "2.020000") || !strings.Contains(got, "2.021000") {
			t.Errorf("%s must distinguish the requested and actual query endpoints:\n%s", name, got)
		}
	}
}

func TestTraceRequestedScopeActualPromptAndSupplementKeepExplorationValues(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := traceRequestedScopeTestContext(2.021, true)
			ctx.Language = lang
			ledger := answerDocObservationLedger(ctx)
			set := types.CompileTraceCausalProjectionSet(ledger)
			note := set.Projections[0].WindowScope.Format(lang)
			if note == "" {
				t.Fatal("fixture did not compile explicit requested scope")
			}
			before, _ := json.Marshal(ledger)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "- supporting_state:") || !strings.Contains(prompt, note) || !strings.Contains(prompt, "accounted_total=21.000ms") {
				t.Fatalf("real finalizer prompt lost the original account or scope:\n%s", prompt)
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "MODEL WORDS REMAIN UNCHANGED"}}}
			docBefore, _ := json.Marshal(doc)
			supplement := renderTraceQueryObservationSupplement(ctx, doc, lang)
			if !strings.Contains(supplement, note) || !strings.Contains(supplement, "0.020") {
				t.Fatalf("supplement must retain the post-request measurement with its query scope:\n%s", supplement)
			}
			docAfter, _ := json.Marshal(doc)
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			if string(before) != string(after) || string(docBefore) != string(docAfter) {
				t.Fatal("scope publication mutated model content or the input ledger")
			}
		})
	}
}

func TestTraceRequestedScopeObservationDoesNotGuessQueryFromOccurrence(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	ledger := answerDocObservationLedger(ctx)
	record := ledger.Records[0]
	record.RichNotes = nil
	record.Span = types.ObservationSpan{StartTs: 2, EndTs: 2.020}
	for _, lang := range []string{"zh", "en"} {
		want := types.ResolveTraceQueryWindowScope(ledger.RuntimeArtifactScopeProfile, 0, 0).Format(lang)
		if got := traceQueryObservationRequestedScopeNote(record, ledger.RuntimeArtifactScopeProfile, lang); got != want || got == "" {
			t.Fatalf("occurrence endpoints must not be treated as query endpoints: got=%q want=%q", got, want)
		}
		if got := traceQueryObservationRequestedScopeNote(record, nil, lang); got != "" {
			t.Fatalf("legacy unscoped observation should retain its old presentation: %q", got)
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{record}}}})
	got := renderAnswerDocObservationLedger(ctx)
	want := types.ResolveTraceQueryWindowScope(ledger.RuntimeArtifactScopeProfile, 0, 0).Format("en")
	if !strings.Contains(got, want) {
		t.Fatalf("ordinary observation context dropped the unknown-query boundary:\n%s", got)
	}
}

func TestTraceRequestedScopeCompactUsesNodeQueryNotOccurrence(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	profile := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	for _, queryEnd := range []float64{2.021, 2.020, 0} {
		projection := types.TraceCausalProjection{
			ArtifactLabel: "trace.ftrace", WindowStartTs: 2, WindowEndTs: 2.021,
			WindowScope: types.ResolveTraceQueryWindowScope(profile, 2, 2.021),
			RankedSeats: []types.TraceCausalProjectionNode{{
				EvidenceID: "scope-seat", Subject: "worker-200", Rank: 1, EffectiveImpactMS: 0.020,
				FixDirection: "scheduling", ChainRelevance: "on_chain",
				StartTs: 2.020, EndTs: 2.020020,
				RankQueryWindowStartTs: 2, RankQueryWindowEndTs: queryEnd,
			}},
		}
		got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
		want := projection.WindowScope.ForWindow(2, queryEnd)
		if !strings.Contains(got, "window_role=`"+string(want.Role)+"`") || !strings.Contains(got, want.Format("en")) {
			t.Fatalf("node query %.6f did not retain its own scope:\n%s", queryEnd, got)
		}
		if strings.Contains(got, "requested_or_elected_window") || !strings.Contains(got, "0.020ms") {
			t.Fatalf("scope role retained the mixed label or lost the original seat value:\n%s", got)
		}
	}
}

func TestTraceRequestedScopeCandidateViewPreservesDecisionAndLegacyShape(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.021, true)
	scope := types.ResolveTraceQueryWindowScope(ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile, 2, 2.021)
	for _, withScope := range []bool{false, true} {
		candidate := types.TraceFindingCandidateV1{}
		candidate.Decision.CandidateID = "unchanged-candidate"
		candidate.Decision.SubjectName = "worker-200"
		candidate.Decision.Magnitude = &types.TypedMagnitude{Value: 0.020, Unit: "ms"}
		if withScope {
			candidate.Decision.EvidenceFacts = &types.TraceCauseEvidenceFacts{WindowScope: &scope}
		}
		before, _ := json.Marshal(candidate)
		groups, _, err := traceRootCauseRosterGroups([]types.TraceFindingCandidateV1{candidate}, &types.TraceFindingContract{})
		if err != nil || len(groups) != 1 {
			t.Fatalf("render candidate roster: groups=%d err=%v", len(groups), err)
		}
		got := string(groups[0].json)
		if withScope {
			if !strings.Contains(got, `"window_scope"`) || !strings.Contains(got, scope.Format("en")) {
				t.Fatalf("candidate reader context lost the selected seat's query scope: %s", got)
			}
		} else if strings.Contains(got, "window_scope") {
			t.Fatalf("legacy candidate must not invent window metadata: %s", got)
		}
		after, _ := json.Marshal(candidate)
		if string(before) != string(after) {
			t.Fatal("candidate context rewrote a decision")
		}
	}
}
