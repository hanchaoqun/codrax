package types

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func hmc257NamedRuntimeRequest() RequestModel {
	return RequestModel{
		Intent: IntentEnumerate, Predicates: SemanticPredicates{HasPerMemberTable: true},
		RuntimeArtifactScopeProfile:     &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeFullArtifact, SourceQuote: "inspect the named captures"},
		RuntimeQuestionProfile:          &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactOtherObservedValue}},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{},
		RequestedAnswerDimensions:       &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{{Index: 1, Required: true, Role: RequestedAnswerDimensionObservedValue, Label: "recorded callsites"}}},
	}
}

func hmc257RepoRoute() TurnRouteHint {
	return TurnRouteHint{Route: "repo", Source: "repo", NeedsRepoAccess: true, RequiredOutcomes: TurnOutcomeAnswer | TurnOutcomeMeasurement | TurnOutcomeSourceExplanation, CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired}
}

func TestHMC257NamedRuntimeDomainSharesSourceObligation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*RequestModel, *TurnRouteHint)
		want   RuntimeSourceRequirementPrecision
	}{
		{"named_capture", nil, RuntimeSourceRequirementNone},
		{"scope_without_question", func(r *RequestModel, _ *TurnRouteHint) { r.RuntimeQuestionProfile = nil }, RuntimeSourceRequirementNone},
		{"question_without_window", func(r *RequestModel, _ *TurnRouteHint) { r.RuntimeArtifactScopeProfile = nil }, RuntimeSourceRequirementNone},
		{"nil_declaration_keeps_route", func(r *RequestModel, _ *TurnRouteHint) { r.CurrentSourceExplanationProfile = nil }, RuntimeSourceRequirementSoft},
		{"true_declaration_keeps_route", func(r *RequestModel, _ *TurnRouteHint) {
			r.CurrentSourceExplanationProfile.IsCurrentSourceExplanationRequested = true
		}, RuntimeSourceRequirementSoft},
		{"current_key_code", func(r *RequestModel, _ *TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionCurrentKeyCode
		}, RuntimeSourceRequirementSoft},
		{"source_attribute", func(r *RequestModel, _ *TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionSourceAttribute
		}, RuntimeSourceRequirementSoft},
		{"source_call_chain", func(r *RequestModel, _ *TurnRouteHint) {
			r.CallChainEndpointProfile = &CallChainEndpointProfile{SinkMode: CallChainSinkResolutionDiscoverPath}
		}, RuntimeSourceRequirementSoft},
		{"source_only_false_is_not_waiver", func(r *RequestModel, _ *TurnRouteHint) {
			r.RuntimeArtifactScopeProfile, r.RuntimeQuestionProfile = nil, nil
		}, RuntimeSourceRequirementSoft},
		{"unaccepted_shape_is_not_domain", func(r *RequestModel, _ *TurnRouteHint) {
			r.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow}
			r.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeNotApplicable}
		}, RuntimeSourceRequirementSoft},
		{"required_source_location", func(r *RequestModel, _ *TurnRouteHint) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionSourceLocation
		}, RuntimeSourceRequirementPrecise},
		{"required_source_file", func(r *RequestModel, _ *TurnRouteHint) { r.AnalyzerHints.ExactTargets = []string{"src/owner.go"} }, RuntimeSourceRequirementPrecise},
		{"source_change", func(_ *RequestModel, h *TurnRouteHint) {
			h.RequiredOutcomes |= TurnOutcomeSourceChange
			h.WriteIntent = "modify"
		}, RuntimeSourceRequirementSoft},
		{"true_causal_not_source", func(r *RequestModel, _ *TurnRouteHint) {
			r.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis, RuntimeWorkRelationRequested: true, FrameCausalityRequested: true}
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionCausalAttribution
		}, RuntimeSourceRequirementNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm, hint := hmc257NamedRuntimeRequest(), hmc257RepoRoute()
			if tc.mutate != nil {
				tc.mutate(&rm, &hint)
			}
			before, _ := json.Marshal(rm)
			effective := EffectiveRequestRouteHint(&rm, hint)
			got := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, hint)
			if got != tc.want {
				t.Errorf("precision=%q want=%q effective=%+v", got, tc.want, effective)
			}
			if effective.Route != hint.Route || effective.Source != hint.Source || effective.NeedsRepoAccess != hint.NeedsRepoAccess || effective.WriteIntent != hint.WriteIntent {
				t.Fatal("source refinement changed execution access or route identity")
			}
			if again := EffectiveRequestRouteHint(&rm, effective); again != effective {
				t.Fatal("effective projection is not idempotent")
			}
			if tc.want == RuntimeSourceRequirementNone {
				if effective.RequiresCurrentSourceEvidence() || rm.CurrentSourceLaneDecision().RequiresCurrentSource() || SourceInventoryCurrentSourceApplicableInContext(rm, hint, false) {
					t.Error("named runtime domain still owns a source lane/inventory")
				}
			}
			if tc.want == RuntimeSourceRequirementPrecise && SourceInventoryCurrentSourceApplicableInContext(rm, hint, false) {
				t.Error("independent source location/file was promoted to an undeclared source-member universe")
			}
			if tc.name != "source_change" && tc.want != RuntimeSourceRequirementNone && !rm.CurrentSourceLaneDecision().RequiresCurrentSource() {
				t.Error("direct source lane discarded a legacy or independent source obligation")
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("policy projection mutated accepted request")
			}
			if tc.name == "true_causal_not_source" && (!rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || !rm.RuntimeQuestionProfile.FrameCausalityRequested) {
				t.Fatal("causal work/frame request was demoted")
			}
		})
	}
}

func TestHMC257PreparedDomainIsPrivateAndRevalidated(t *testing.T) {
	ctx, source, _, preparer := namedPreparedSelectionFixture(t)
	ctx.RuntimeArtifactPreflight = RuntimeArtifactPreflightProfile{}
	ctx.AnalysisIR = &AnalysisIR{RequestModel: RequestModel{CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{}}}
	ctx.TurnRouteHint = hmc257RepoRoute()
	before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	for _, view := range []*RequestModel{RuntimeSourceAuthorityRequestModelFromAgentContext(ctx), RuntimeSourceAuthorityRequestModelFromBusContext(ToolBusContext(ctx, "explorer"))} {
		if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(view, ctx.TurnRouteHint); p != RuntimeSourceRequirementNone {
			t.Errorf("validated prepared material did not refine source lane: %q", p)
		}
		if view.PerfTrace != nil || view.LogTriage != nil {
			t.Fatal("material domain fabricated a runtime observation bundle")
		}
		data, _ := json.Marshal(view)
		if string(data) != string(before) {
			t.Fatalf("private material identity serialized: %s", data)
		}
		var restored RequestModel
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(&restored, ctx.TurnRouteHint); p != RuntimeSourceRequirementSoft {
			t.Fatal("serialized history restored current material authority")
		}
	}
	if preparer.calls != 0 {
		t.Fatal("domain inspection performed preparation")
	}
	// Even an in-memory copy of a projected view cannot retain private
	// applicability after its current context loses the authenticated receipt.
	projected := RuntimeSourceAuthorityRequestModelFromAgentContext(ctx)
	clone := ctx.ShallowClone()
	clone.AnalysisIR = &AnalysisIR{RequestModel: *projected}
	clone.TraceInputPreparer, clone.AttachedTraceMaterial = nil, nil
	if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(RuntimeSourceAuthorityRequestModelFromAgentContext(clone), clone.TurnRouteHint); p != RuntimeSourceRequirementSoft {
		t.Fatal("copied private domain survived without current preparation")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	clone = ctx.ShallowClone()
	clone.Ctx = cancelled
	if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(RuntimeSourceAuthorityRequestModelFromAgentContext(clone), clone.TurnRouteHint); p != RuntimeSourceRequirementSoft {
		t.Fatal("cancelled preparation validation granted material domain")
	}
	if err := os.WriteFile(source, []byte("changed generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(RuntimeSourceAuthorityRequestModelFromAgentContext(ctx), ctx.TurnRouteHint); p != RuntimeSourceRequirementSoft {
		t.Fatal("stale private receipt retained domain authority")
	}
	if got, _ := json.Marshal(ctx.AnalysisIR.RequestModel); string(got) != string(before) {
		t.Fatal("context enrichment changed accepted IR")
	}
}

func TestHMC257QueryDomainDoesNotMintFactsOrSurviveReset(t *testing.T) {
	mut := NewMutableState("analyze named records")
	rm := RequestModel{CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{}}
	bus := &BusContext{Mutable: mut, AnalysisIR: &AnalysisIR{RequestModel: rm}, TurnRouteHint: hmc257RepoRoute()}
	mut.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	view := RuntimeSourceAuthorityRequestModelFromBusContext(bus)
	if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(view, bus.TurnRouteHint); p != RuntimeSourceRequirementNone {
		t.Fatalf("current query domain absent: %q", p)
	}
	if view.PerfTrace != nil || view.LogTriage != nil || !reflect.DeepEqual(bus.AnalysisIR.RequestModel, rm) {
		t.Fatal("query-domain view published synthetic facts")
	}
	mut.ResetTurnAArtifacts()
	if p := RuntimeSourceRequestCurrentSourceRequirementPrecision(RuntimeSourceAuthorityRequestModelFromBusContext(bus), bus.TurnRouteHint); p != RuntimeSourceRequirementSoft {
		t.Fatal("prior-turn query domain survived reset")
	}
}

func TestHMC257NamedDomainBudgetStillRequiresActualRuntimeFacts(t *testing.T) {
	rm, hint := hmc257NamedRuntimeRequest(), hmc257RepoRoute()
	input := RuntimeSourceAnswerAuthorityInput{RequestModel: &rm, RouteHint: hint}
	withoutFacts := BuildRuntimeSourceAnswerAuthoritySnapshot(input)
	if RuntimeSourceAuthorityRequestCarrierActive(hint, &rm, withoutFacts) || withoutFacts.HasRuntimeCarrier() {
		t.Fatal("request domain alone activated a fact-backed budget")
	}
	input.Ledger = ObservationLedger{Records: []ObservationRecord{runtimeSourceTraceRecord("trace:named-member", "trace_query")}}
	withFacts := BuildRuntimeSourceAnswerAuthoritySnapshot(input)
	if !RuntimeSourceAuthorityRequestCarrierActive(hint, &rm, withFacts) || withFacts.CurrentSourceRequired {
		t.Fatalf("actual runtime facts lost shared material-domain handoff: %+v", withFacts)
	}
}
