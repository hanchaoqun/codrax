package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func relationRuntimeHandoffTestContext() *types.BusContext {
	ctx := &types.BusContext{
		Mutable: types.NewMutableState("Report the observed request groups and their measurements."),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsRelationalLookup: true, IsCategoryEnumeration: true,
			},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			},
		}},
		AttachedHitrace: "attached runtime artifact",
		TurnRouteHint: types.TurnRouteHint{
			Route: "repo", Source: "external_tool", NeedsRepoAccess: true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
		},
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "runtime:groups", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Role: types.AnswerAggregateRolePrincipalAnswer, Summary: "observed request groups and their measurements",
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: "capture.systrace",
				ArtifactID: "attached_trace", ArtifactKind: "trace", PayloadRef: "blob://runtime-groups",
			},
			Span: types.ObservationSpan{LineStart: 5, LineEnd: 24},
		}},
	})
	return ctx
}

func executeRelationRuntimeHandoffTest(t *testing.T, ctx *types.BusContext, facts []types.AnswerAggregateFact) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"reason":     "The observed groups are known; preserve their measured values and evidence boundary.",
		"confidence": "high", "result_kind": "resolved", "aggregate_facts": facts,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return result
}

func TestRelationRuntimeHandoffMissingSetUsesStructuredRepair(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	for _, withScalar := range []bool{false, true} {
		t.Run(map[bool]string{false: "no member set", true: "scalar facts only"}[withScalar], func(t *testing.T) {
			ctx := relationRuntimeHandoffTestContext()
			var facts []types.AnswerAggregateFact
			if withScalar {
				facts = []types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar, Label: "group mean", Value: "6", Unit: "ms", Role: types.AnswerAggregateRolePrincipalAnswer}}
			}
			result := executeRelationRuntimeHandoffTest(t, ctx, facts)
			if !result.Success || !strings.Contains(result.Summary, "relation member-set handoff is missing") || ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("missing runtime member set must still downgrade: %+v", result)
			}
			repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
			if len(repairs) != 1 {
				t.Fatalf("expected one relation handoff repair: %+v", repairs)
			}
			repair := repairs[0]
			if repair.Kind != types.RepairStructuredHandoff ||
				!reflect.DeepEqual(types.RepairDirectiveRequiredTools(repair), []string{"emit_investigation_complete"}) {
				t.Fatalf("runtime member handoff must not demand unavailable source emit: %+v", repair)
			}
			if types.ClassifyRepairDirective(repair) != types.RepairDebtPrincipalBlocking || types.RepairDirectiveIsCompletionFormDebt(repair) {
				t.Fatalf("repair must retain the principal member-set obligation: %+v", repair)
			}
			if len(repair.Files) != 0 || len(repair.Keywords) != 0 || !strings.Contains(result.Summary, "Do not call `emit_evidence`") {
				t.Fatalf("runtime repair must stay in the external observation lane: repair=%+v summary=%s", repair, result.Summary)
			}
		})
	}
}

func relationRuntimeHandoffTestMemberSet() types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Label: "observed request groups", Value: "2",
		Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "trace_query.window_stats",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)},
			{Name: "artifact_id", Value: "attached_trace"},
			{Name: "payload_ref", Value: "blob://runtime-groups"},
		},
		Members: []string{"request-group-A", "request-group-B"},
	}
}

func TestRelationRuntimeHandoffVerifiedSetCanCompleteWithoutSourceEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	ctx := relationRuntimeHandoffTestContext()
	first := executeRelationRuntimeHandoffTest(t, ctx, nil)
	if !strings.Contains(first.Summary, "relation member-set handoff is missing") {
		t.Fatalf("expected initial missing-set repair: %+v", first)
	}
	result := executeRelationRuntimeHandoffTest(t, ctx, []types.AnswerAggregateFact{relationRuntimeHandoffTestMemberSet()})
	if !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("valid principal runtime member set must satisfy the existing gate: %+v", result)
	}
	if repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs(); len(repairs) != 0 {
		t.Fatalf("successful member-set repair must clear the active principal debt: %+v", repairs)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 || len(ctx.Mutable.EvidenceClosure().CanonicalReadFiles()) != 0 {
		t.Fatal("runtime handoff must not need fabricated current-source evidence or reads")
	}
}

func TestRelationRuntimeHandoffInvalidSetsRemainUnresolved(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	for _, tc := range []struct {
		name   string
		mutate func(*types.AnswerAggregateFact)
		want   string
	}{
		{
			name: "supporting coverage is not the principal set",
			mutate: func(fact *types.AnswerAggregateFact) {
				fact.Role = types.AnswerAggregateRoleSupportingCoverage
			},
			want: "not principal_answer",
		},
		{
			name: "count mismatch",
			mutate: func(fact *types.AnswerAggregateFact) {
				fact.Value = "3"
			},
			want: "members",
		},
		{
			name: "unsupported source members",
			mutate: func(fact *types.AnswerAggregateFact) {
				fact.Provenance = ""
				fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: string(types.AnswerEvidenceOriginCurrentSource)}}
			},
			want: "no typed evidence",
		},
		{
			name: "empty set without typed zero result",
			mutate: func(fact *types.AnswerAggregateFact) {
				fact.Value = "0"
				fact.Members = nil
			},
			want: "member_set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := relationRuntimeHandoffTestContext()
			fact := relationRuntimeHandoffTestMemberSet()
			tc.mutate(&fact)
			result := executeRelationRuntimeHandoffTest(t, ctx, []types.AnswerAggregateFact{fact})
			if ctx.Mutable.IsInvestigationComplete() || !strings.Contains(result.Summary, tc.want) {
				t.Fatalf("invalid member set must remain unresolved (%q): %+v", tc.want, result)
			}
		})
	}
}

func TestRelationRuntimeHandoffRetainsCurrentSourceAndIndependentEvidenceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.BusContext)
	}{
		{
			name: "ordinary source lookup",
			mutate: func(ctx *types.BusContext) {
				ctx.Mutable = types.NewMutableState("List the implementations in this repository.")
				ctx.AttachedHitrace = ""
				ctx.TurnRouteHint = types.TurnRouteHint{}
				ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
			},
		},
		{
			name: "precise mixed source target",
			mutate: func(ctx *types.BusContext) {
				ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"worker.go"}
			},
		},
		{
			name: "source answer contract",
			mutate: func(ctx *types.BusContext) {
				ctx.AnalysisIR.AnswerContract.ExactResolution = &types.ExactResolutionContract{TargetLabel: "worker.go"}
			},
		},
		{
			name: "soft source requirement remains a source carrier",
			mutate: func(ctx *types.BusContext) {
				ctx.TurnRouteHint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceRequired
			},
		},
		{
			name: "landed source support",
			mutate: func(ctx *types.BusContext) {
				ctx.EvidenceItems = []types.EvidenceItem{{
					ID: "source:worker", Kind: types.EvidenceDirect, Source: "worker.go",
					Summary: "current worker implementation", LineStart: 9, LineEnd: 14,
					GroundingStatus: types.GroundingGrounded,
				}}
			},
		},
		{
			name: "attachment and citation policy without observation",
			mutate: func(ctx *types.BusContext) {
				ctx.Mutable = types.NewMutableState("Report the observed request groups.")
			},
		},
		{
			name: "waiver without observation",
			mutate: func(ctx *types.BusContext) {
				ctx.Mutable = types.NewMutableState("Report the observed request groups.")
				ctx.Mutable.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
					Reason: types.EvidenceFloorWaiverExternalTrace, Rationale: "artifact-local rows",
				})
			},
		},
		{
			name: "retained model aggregate without independent producer",
			mutate: func(ctx *types.BusContext) {
				ctx.Mutable = types.NewMutableState("Report the observed request groups.")
				ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
					Kind: types.AnswerAggregateScalar, Label: "retained group mean", Value: "6", Unit: "ms",
					Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "trace_query.window_stats",
					Dimensions: []types.AnswerAggregateDimension{
						{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)},
						{Name: "payload_ref", Value: "blob://model-authored-only"},
					},
				}})
				ctx.Mutable.RetainInvestigationAggregateFacts()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := relationRuntimeHandoffTestContext()
			tc.mutate(ctx)
			// Even an origin-labeled scalar draft cannot manufacture independent
			// observations or erase a required/already-landed source lane.
			facts := []types.AnswerAggregateFact{{
				Kind: types.AnswerAggregateScalar, Label: "group mean", Value: "6", Unit: "ms",
				Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "trace_query",
				Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)}},
			}}
			summary := relationMemberSetHandoffDowngrade(ctx, ctx.Mutable.EvidenceClosure(), facts)
			repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
			if !strings.Contains(summary, "relation member-set handoff is missing") || len(repairs) != 1 {
				t.Fatalf("expected unresolved source relation handoff: summary=%s repairs=%+v", summary, repairs)
			}
			repair := repairs[0]
			if repair.Kind != types.RepairEmitEvidence || repair.Origin != "pre_complete.relation_member_set" ||
				!reflect.DeepEqual(types.RepairDirectiveRequiredTools(repair), []string{"emit_evidence"}) ||
				strings.Contains(summary, "Do not call `emit_evidence`") {
				t.Fatalf("source/unsupported lane must retain the existing repair: summary=%s repair=%+v", summary, repair)
			}
		})
	}
}

func TestRelationRuntimeHandoffAddressableResourceUsesSharedExternalAuthority(t *testing.T) {
	ctx := relationRuntimeHandoffTestContext()
	ctx.Mutable = types.NewMutableState("List the observed resource groups.")
	ctx.AttachedHitrace = ""
	ctx.MCPResponses = []types.MCPResponse{{
		ServerName: "fixture", Method: "tools/call:lookup_groups", Success: true,
		ResourceURI: "mcp://fixture/groups", MIMEType: "application/vnd.codrax.observation+json",
		Observations: []types.MCPTypedObservation{{
			Summary: "observed resource groups", ResourceURI: "mcp://fixture/groups", Selector: "$.groups",
		}},
	}}
	summary := relationMemberSetHandoffDowngrade(ctx, ctx.Mutable.EvidenceClosure(), nil)
	repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairStructuredHandoff ||
		!strings.Contains(summary, "Do not call `emit_evidence`") {
		t.Fatalf("addressable resource evidence must use the shared external repair lane: summary=%s repairs=%+v", summary, repairs)
	}
}
