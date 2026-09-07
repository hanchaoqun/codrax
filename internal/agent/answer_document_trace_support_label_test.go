package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B1601: the reduced root_evidence producer preserves useful intervals but
// does not carry the richer rank-board authority. Its historical family name
// must not turn an appendix label into a new root-cause claim.
func TestTraceObservationSupportingLabelActualParseOutput(t *testing.T) {
	tests := []struct {
		name, claim, subject, value, zh, en string
		notes                               []string
	}{
		{"reduced_pacing", "root_evidence:pacing", "app-9511", "15.565", "分析支撑观测", "analysis supporting observation", []string{"tier=context_only", "effective_impact_ms=0.000"}},
		{"reduced_unknown_legacy", "root_evidence:legacy", "target-1", "15.758", "分析支撑观测", "analysis supporting observation", nil},
		{"reduced_with_stale_primary", "root_evidence:runnable", "worker-2", "1.193", "分析支撑观测", "analysis supporting observation", []string{"tier=primary", "causality=on_wakeup_chain", "chain_relevance=on_chain"}},
		{"reduced_background", "root_evidence:pacing", "other-3", "15.758", "背景支撑观测", "background supporting observation", []string{"tier=context_only", "chain_relevance=background"}},
		{"reduced_adjacent", "root_evidence:runnable", "other-4", "0.345", "邻近支撑观测", "adjacent supporting observation", []string{"tier=context_only", "causality=adjacent_to_wakeup_chain"}},
		{"rank_context_only", "root_cause_context_only", "worker-5", "15.758", "分析支撑观测", "analysis supporting observation", []string{"tier=context_only", "effective_impact_ms=0.000"}},
		{"stale_primary_context_only", "root_cause_primary", "worker-6", "15.758", "分析支撑观测", "analysis supporting observation", []string{"tier=context_only", "causality=on_wakeup_chain", "chain_relevance=on_chain"}},
		{"rank_data_gap", "root_cause_data_gap", "unknown-7", "1.409", "证据覆盖缺口", "evidence coverage gap", []string{"tier=data_gap"}},
		{"stale_primary_data_gap", "root_cause_primary", "unknown-8", "1.409", "证据覆盖缺口", "evidence coverage gap", []string{"tier=data_gap", "chain_relevance=on_chain"}},
		{"proved_primary_preserved", "root_cause_primary", "worker-9", "8.250", "首要根因观测", "primary root-cause observation", []string{"tier=primary", "causality=on_wakeup_chain", "chain_relevance=on_chain"}},
		{"proved_secondary_preserved", "root_cause_secondary", "worker-10", "4.125", "次级根因观测", "secondary root-cause observation", []string{"tier=secondary", "causality=on_wakeup_chain", "chain_relevance=on_chain"}},
	}
	for _, tc := range tests {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				record := types.ObservationRecord{
					ID: "trace_query:b1601#" + tc.name, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
					Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
					GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceObservedDirectCause,
					SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "sample.systrace"},
					ClaimKey:  tc.claim, Predicate: "runnable_wait", Subject: tc.subject,
					Value: tc.value, Unit: "ms", Span: types.ObservationSpan{LineStart: 10, LineEnd: 20},
					RichNotes: tc.notes, SupportRefs: []string{"sample.systrace:10-20"},
				}
				mu := types.NewMutableState("")
				mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{record}}}})
				const modelText = "Model conclusion: inspect the dependency path before deciding the optimization."
				mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
					DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: modelText}},
				})
				ctx := &types.AgentContext{Mutable: mu}
				beforeDoc := traceSupportLabelCanonical(t, mu.AnswerDocumentV2())
				beforeLedger := answerDocObservationLedger(ctx)
				beforeRecords := traceSupportLabelCanonical(t, beforeLedger)
				beforeProjection := traceSupportLabelCanonical(t, types.CompileTraceCausalProjectionSet(beforeLedger))
				out, err := (&answerDocumentEvaluator{language: lang}).ParseOutput(ctx, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				want, sep, value := tc.en, ": ", "value="
				if lang == "zh" {
					want, sep, value = tc.zh, "：", "值="
				}
				for _, part := range []string{want + sep + tc.subject, value + tc.value + "ms", modelText, "sample.systrace"} {
					if !strings.Contains(out.FinalAnswer, part) {
						t.Fatalf("actual final output missing %q:\n%s", part, out.FinalAnswer)
					}
				}
				if beforeDoc != traceSupportLabelCanonical(t, mu.AnswerDocumentV2()) ||
					beforeRecords != traceSupportLabelCanonical(t, answerDocObservationLedger(ctx)) ||
					beforeProjection != traceSupportLabelCanonical(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))) {
					t.Fatal("display label repair changed model document, canonical observation ledger, or causal projection")
				}
			})
		}
	}
}

func TestTraceObservationSupportingLabelDoesNotInferTierFromProse(t *testing.T) {
	for _, tier := range []string{"", "context", "context_only_extra", "Context_Only", "data_gap_extra"} {
		record := types.ObservationRecord{
			ClaimKey: "root_cause_primary", Subject: "worker-1",
			Summary:   "tier=context_only; data_gap; background supporting observation",
			RichNotes: []string{"tier=" + tier, "chain_relevance=on_chain", "causality=on_wakeup_chain"},
		}
		if got := traceQueryObservationSupplementClaimLabel(record, true); got != "首要根因观测" {
			t.Fatalf("noncanonical tier/prose changed existing root label: tier=%q got=%q", tier, got)
		}
	}
}

func traceSupportLabelCanonical(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
