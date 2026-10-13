package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC232ReaderIORulersStayBoundToTypedValue(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence, types.TraceIOValueCaliberIssuerBlocked, types.TraceIOValueCaliberMixed, "", "future_unknown"} {
			for _, token := range []string{"io_latency", "io_wait", "io_burst_episode"} {
				n := types.TraceCausalProjectionNode{TypeToken: token, StateKind: token, IOValueCaliber: caliber}
				before := b1645Snapshot(t, n)
				want := tool.TraceMeasurementTypeDisplayLabel(token, caliber, zh)
				for name, face := range map[string]string{"cause": traceFinalReaderCauseLabel(n, zh), "context": traceFinalReaderContextLabel(n, zh), "roster": answerDocTraceRankReaderMeasuredCauseLabel(types.TraceRankRosterSeat{Type: token, IOValueCaliber: caliber}, zh)} {
					if face != want {
						t.Errorf("%s %s/%s = %q want %q", name, token, caliber, face, want)
					}
				}
				if types.TraceUsesIOValueCaliber(token, caliber) && traceFinalReaderActualCauseLabel(n, zh) != want {
					t.Errorf("actual-state fallback lost %s/%s ruler: %s", token, caliber, traceFinalReaderActualCauseLabel(n, zh))
				}
				r := types.ObservationRecord{Predicate: "root_cause_primary", ClaimKey: "root_cause_primary", Subject: "issuer-42", Object: token, Value: "35.000", Unit: "ms", Role: types.AnswerAggregateRolePrincipalAnswer, RichNotes: []string{"chain_relevance=on_chain", types.TraceNoteKeyIOValueCaliber + "=" + caliber}}
				if types.TraceUsesIOValueCaliber(token, caliber) && !strings.Contains(traceQueryObservationSupplementText(r, zh), want) {
					t.Errorf("supplement borrowed an IO ruler: %+v", r)
				}
				if before != b1645Snapshot(t, n) {
					t.Fatal("reader mutated typed value")
				}
			}
		}
		if got := traceFinalReaderMeasuredStateLabel("io_wait", "", zh); got != traceFinalReaderStateLabel("io_wait", zh) {
			t.Fatal("unqualified scheduler IO state was relabeled")
		}
	}
}

func TestHMC232SupplementRootFamilyNeedsTypedCauseQualification(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, tc := range []struct {
			name  string
			role  types.AnswerAggregateRole
			notes []string
			cause bool
		}{
			{"legacy_explicit_chain", "", []string{"chain_relevance=on_chain"}, true},
			{"principal_explicit_causality", types.AnswerAggregateRolePrincipalAnswer, []string{"causality=on_wakeup_chain"}, true},
			{"principal_missing_qualification", types.AnswerAggregateRolePrincipalAnswer, nil, false},
			{"supporting_explicit_chain", types.AnswerAggregateRoleSupportingCoverage, []string{"chain_relevance=on_chain"}, true},
			{"adjacent", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=adjacent"}, false},
			{"background_over_stale_chain", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "causality=background"}, false},
			{"context", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=context_only"}, false},
			{"symptom", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=target_self_state"}, false},
			{"caliber_side", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=caliber_side"}, false},
			{"data_gap", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=data_gap"}, false},
			{"unproved", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "causality=unproven"}, false},
		} {
			r := types.ObservationRecord{ClaimKey: "root_cause_primary", Predicate: "root_cause_primary", Role: tc.role, RichNotes: tc.notes}
			face := traceQueryObservationSupplementClaimLabel(r, zh)
			cause := strings.Contains(face, "根因观测") || strings.Contains(face, "root-cause observation")
			if cause != tc.cause {
				t.Errorf("%s: %q cause=%v want=%v", tc.name, face, cause, tc.cause)
			}
		}
	}
}

func TestHMC232SupplementDisplayRoleCausalConflictsActualParseOutput(t *testing.T) {
	for _, role := range []types.AnswerAggregateRole{"", types.AnswerAggregateRolePrincipalAnswer, types.AnswerAggregateRoleSupportingCoverage} {
		for _, tc := range []struct {
			name, claim, zh, en string
			notes               []string
		}{
			{"proved_relevance", "root_cause_primary", "首要根因观测", "primary root-cause observation", []string{"chain_relevance=on_chain"}},
			{"proved_wakeup", "root_cause_secondary", "次级根因观测", "secondary root-cause observation", []string{"causality=on_wakeup_chain"}},
			{"proved_self", "root_cause_primary", "首要根因观测", "primary root-cause observation", []string{"causality=self_deterministic"}},
			{"background_over_wakeup", "root_cause_primary", "背景支撑观测", "background supporting observation", []string{"chain_relevance=background", "causality=on_wakeup_chain"}},
			{"background_over_chain", "root_cause_primary", "背景支撑观测", "background supporting observation", []string{"chain_relevance=on_chain", "causality=background"}},
			{"adjacent_over_wakeup", "root_cause_primary", "邻近支撑观测", "adjacent supporting observation", []string{"chain_relevance=adjacent", "causality=on_wakeup_chain"}},
			{"context_over_chain", "root_cause_primary", "分析支撑观测", "analysis supporting observation", []string{"tier=context_only", "chain_relevance=on_chain"}},
			{"caliber_over_chain", "root_cause_primary", "分析支撑观测", "analysis supporting observation", []string{"tier=caliber_side", "chain_relevance=on_chain"}},
			{"gap_over_chain", "root_cause_primary", "证据覆盖缺口", "evidence coverage gap", []string{"tier=data_gap", "chain_relevance=on_chain"}},
			{"state_over_chain", "root_cause_primary", "目标状态观测（待沿链下钻）", "target-state observation (follow the chain for cause)", []string{"tier=target_self_state", "chain_relevance=on_chain"}},
			{"unproved_over_chain", "root_cause_primary", "待核实候选观测", "unproved candidate observation", []string{"causality=unproven", "chain_relevance=on_chain"}},
			{"unknown_qualification", "root_cause_primary", "", "", []string{"chain_relevance=on_chain_extra", "causality=on_wakeup_chain_extra"}},
			{"missing_qualification", "root_cause_primary", "", "", nil},
			{"unknown_tier_preserves_exact_chain", "root_cause_primary", "首要根因观测", "primary root-cause observation", []string{"tier=context_only_extra", "chain_relevance=on_chain"}},
			{"reduced_family_stays_support", "root_evidence:legacy", "分析支撑观测", "analysis supporting observation", []string{"tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain"}},
		} {
			for _, lang := range []string{"zh", "en"} {
				t.Run(string(role)+"/"+tc.name+"/"+lang, func(t *testing.T) {
					record := types.ObservationRecord{
						ID: "trace_query:hmc232#role-conflict", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
						Producer: "trace_query", Role: role, GroundingPolicy: types.ClaimGroundingHard,
						ProvenanceLane: types.ObservationProvenanceObservedDirectCause,
						SourceRef:      types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "role-conflict.systrace"},
						ClaimKey:       tc.claim, Predicate: "runnable_wait", Subject: "worker-42", Value: "8.250", Unit: "ms",
						Span: types.ObservationSpan{LineStart: 10, LineEnd: 20}, RichNotes: tc.notes,
						SupportRefs: []string{"role-conflict.systrace:10-20"},
					}
					mu := types.NewMutableState("")
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{record}}}})
					mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
						DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "Readout."}},
					})
					ctx := &types.AgentContext{Mutable: mu}
					beforeDoc := traceSupportLabelCanonical(t, mu.AnswerDocumentV2())
					beforeLedger := traceSupportLabelCanonical(t, answerDocObservationLedger(ctx))
					beforeProjection := traceSupportLabelCanonical(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
					out, err := (&answerDocumentEvaluator{language: lang}).ParseOutput(ctx, nil, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					want, sep := tc.en, ": "
					if lang == "zh" {
						want, sep = tc.zh, "："
					}
					if want == "" {
						want = "unattributed supporting observation"
						if role == types.AnswerAggregateRoleSupportingCoverage {
							want = "analysis supporting observation"
						}
						if lang == "zh" {
							want = "未归因支撑观测"
							if role == types.AnswerAggregateRoleSupportingCoverage {
								want = "分析支撑观测"
							}
						}
					}
					if !strings.Contains(out.FinalAnswer, want+sep+record.Subject) || !strings.Contains(out.FinalAnswer, "8.250ms") {
						t.Fatalf("role changed precise causal-position display, want %q with measured value:\n%s", want, out.FinalAnswer)
					}
					if beforeDoc != traceSupportLabelCanonical(t, mu.AnswerDocumentV2()) ||
						beforeLedger != traceSupportLabelCanonical(t, answerDocObservationLedger(ctx)) ||
						beforeProjection != traceSupportLabelCanonical(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))) {
						t.Fatal("display changed the document, observation ledger, or causal projection")
					}
				})
			}
		}
	}
}
