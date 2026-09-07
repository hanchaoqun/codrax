package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These are handoff tests, not a request to expand the model's read surface:
// display limits must not become limits on already grounded backend facts.
func TestSourceInventoryLargeHandoff_EmptyCurrentUsesAllRows(t *testing.T) {
	for _, seedStable := range []bool{false, true} {
		t.Run(fmt.Sprintf("stable=%t", seedStable), func(t *testing.T) {
			ctx, observation, want := sourceInventoryLargeHandoffContext(67)
			if seedStable {
				result, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"The typed source inventory is complete.","confidence":"high","result_kind":"resolved","aggregate_facts":[]}`))
				if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
					t.Fatalf("seed must be a genuinely accepted completion: err=%v result=%+v", err, result)
				}
				ctx.Mutable.ResetInvestigationComplete()
				assertSourceInventoryLargeHandoffMembers(t, ctx, effectiveCompletionAggregateFacts(ctx, nil), want)
			}
			for _, limit := range []int{32, 40} {
				snapshot := types.BuildSourceInventoryAuthoritySnapshot(types.SourceInventoryAuthoritySnapshotInput{
					Observation: observation, RequestModel: ctx.AnalysisIR.RequestModel,
					ExistingAggregateFacts: ctx.Mutable.StableInvestigationAggregateFacts(), MaxPrincipalRows: limit,
				})
				if !snapshot.CanEnterMechanicalLanding || len(snapshot.PrincipalRowSet.PrincipalRows) != limit ||
					snapshot.PrincipalRowSet.PrincipalTotal != len(want) || snapshot.PrincipalRowSet.PrincipalHiddenCount != len(want)-limit {
					t.Fatalf("limit=%d landing=%t visible=%d total=%d hidden=%d", limit, snapshot.CanEnterMechanicalLanding,
						len(snapshot.PrincipalRowSet.PrincipalRows), snapshot.PrincipalRowSet.PrincipalTotal, snapshot.PrincipalRowSet.PrincipalHiddenCount)
				}
				assertSourceInventoryLargeHandoffMembers(t, ctx, snapshot.ProjectedPrincipalAggregates, want)
			}
			assertSourceInventoryLargeHandoffMembers(t, ctx, effectiveCompletionAggregateFactsForValidation(ctx, nil, nil), want)
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"The typed source inventory is complete.","confidence":"high","result_kind":"resolved","aggregate_facts":[]}`))
			if err != nil || !result.Success || strings.Contains(result.Summary, EmitInvestigationCompleteDowngradePrefix) || !ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("empty-current completion must consume full backend handoff: err=%v result=%+v complete=%t", err, result, ctx.Mutable.IsInvestigationComplete())
			}
			assertSourceInventoryLargeHandoffMembers(t, ctx, ctx.Mutable.StableInvestigationAggregateFacts(), want)
			if got := types.SourceInventoryObservationFromMutable(ctx.Mutable); !reflect.DeepEqual(got, observation) {
				t.Fatal("completion must not mutate the typed source observation")
			}
		})
	}
}

func TestSourceInventoryLargeHandoff_IncompleteScopeCannotLand(t *testing.T) {
	ctx, observation, _ := sourceInventoryLargeHandoffContext(67)
	observation.Complete = false
	observation.Sets[0].Complete = false
	observation.Sets[0].Total++
	observation.CompleteLenses = nil // Do not retain the fixture's former complete-query receipt.
	observation.Execution = &types.SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true}
	ctx.Mutable = types.NewMutableState("incomplete large inventory")
	ctx.Mutable.SetSourceInventoryObservation(observation)
	ctx.AnalysisIR.EvidencePlan.RequiredFiles = []string{"src/not_observed.py"}
	snapshot := sourceInventoryAuthoritySnapshotForCompletion(ctx, types.SourceInventoryObservationFromMutable(ctx.Mutable), nil)
	if snapshot.CanEnterMechanicalLanding || snapshot.RequiredFilesCovered || !snapshot.NeedsFollowup {
		t.Fatalf("an unobserved required scope must not be promoted to complete by many visible rows: landing=%t required_covered=%t needs_followup=%t", snapshot.CanEnterMechanicalLanding, snapshot.RequiredFilesCovered, snapshot.NeedsFollowup)
	}
	// This is a readiness contract, not a blanket refusal of model completion:
	// non-precise navigation debt may still end with an explicit caveat.
}

func TestSourceInventoryLargeHandoff_MissingRequestedAttributeRemainsUnprovided(t *testing.T) {
	ctx, observation, _ := sourceInventoryLargeHandoffContext(67)
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile.RequestedFields = append(ctx.AnalysisIR.RequestModel.SourceInventoryProfile.RequestedFields, types.SourceInventoryFieldPackage)
	// A declaration roster proves identity and location, not absent package
	// attributes. Keep the loss beyond both display limits to exercise the tail.
	observation.Sets[0].Members[66].Attributes = nil
	ctx.Mutable = types.NewMutableState("large inventory with unprovided attribute")
	ctx.Mutable.SetSourceInventoryObservation(observation)
	before := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	_ = effectiveCompletionAggregateFactsForValidation(ctx, nil, nil)
	after := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("handoff must not mutate or fill absent per-member attributes")
	}
	snapshot := types.BuildSourceInventoryAuthoritySnapshot(types.SourceInventoryAuthoritySnapshotInput{
		Observation: after, RequestModel: ctx.AnalysisIR.RequestModel, MaxPrincipalRows: 67,
	})
	if len(snapshot.PrincipalRowSet.PrincipalRows) != 67 || !reflect.DeepEqual(snapshot.RequestedFields, ctx.AnalysisIR.RequestModel.SourceInventoryProfile.RequestedFields) {
		t.Fatal("the complete row carrier and requested fields must remain available for attribute coverage inspection")
	}
	t.Logf("attribute disclosure boundary: requested=package rows=%d tail_attributes=%d mechanical_landing=%t", len(snapshot.PrincipalRowSet.PrincipalRows), len(snapshot.PrincipalRowSet.PrincipalRows[66].Member.Attributes), snapshot.CanEnterMechanicalLanding)
	for i, row := range snapshot.PrincipalRowSet.PrincipalRows {
		if row.Member.Name != fmt.Sprintf("run_%03d", i) {
			t.Fatalf("unexpected row identity at %d: %s", i, row.Member.Name)
		}
		if i == 66 {
			if len(row.Member.Attributes) != 0 {
				t.Fatalf("tail row borrowed or synthesized package attributes: %+v", row.Member.Attributes)
			}
			continue
		}
		if !reflect.DeepEqual(row.Member.Attributes, before.Sets[0].Members[i].Attributes) {
			t.Fatalf("supplied attributes for row %d changed", i)
		}
	}
	// Deliberately do not pin CanEnterMechanicalLanding here: absent,
	// unavailable and not-applicable attributes have no distinct typed status
	// yet. A nil field must not silently become a new hard-refusal contract.
}

func sourceInventoryLargeHandoffContext(count int) (*types.BusContext, types.SourceInventoryObservation, []string) {
	ctx := sourceInventoryTestContext("", nil, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields: []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation}, Confidence: 0.95,
	})
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "all functions"}
	observation := types.SourceInventoryObservation{
		Active: true, Complete: true, Scopes: []string{"src"}, Lens: []string{"members"},
		Provenance: []string{types.SourceInventoryProvenanceRepoLensToolQuery, types.SourceInventoryProvenanceStageExplore},
		Sets:       []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleFunction, Complete: true, Count: count, Total: count}},
	}
	want := make([]string, count)
	for i := range want {
		want[i] = fmt.Sprintf("run_%03d", i)
		file := fmt.Sprintf("src/worker_%03d.py", i)
		observation.Sets[0].Members = append(observation.Sets[0].Members, types.SourceInventoryObservationMember{
			Name: want[i], Key: file + "::" + want[i], Role: types.AnswerCandidateRoleFunction,
			File: file, Line: 7, SupportRef: want[i] + ": " + file + ":7", Language: "python", SourceClass: types.SourcePathRoleProduction,
			Attributes: []types.SourceInventoryObservationAttribute{{Name: "workers", Role: types.AnswerCandidateRolePackage, File: file, Line: 1, SupportRef: "workers: " + file + ":1"}},
		})
	}
	ctx.Mutable.SetSourceInventoryObservation(observation)
	return ctx, types.SourceInventoryObservationFromMutable(ctx.Mutable), want
}

func assertSourceInventoryLargeHandoffMembers(t *testing.T, ctx *types.BusContext, facts []types.AnswerAggregateFact, want []string) {
	t.Helper()
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
	if len(refs) != 1 {
		t.Fatalf("expected one complete typed principal roster, got %+v", facts)
	}
	fact := refs[0].Fact
	if !reflect.DeepEqual(fact.Members, want) || fact.Value != strconv.Itoa(len(want)) || len(fact.SupportRefs) != len(want) {
		t.Fatalf("full handoff changed: members=%v value=%s support_count=%d want_count=%d", fact.Members, fact.Value, len(fact.SupportRefs), len(want))
	}
	for i, ref := range fact.SupportRefs {
		member, location, ok := types.ParseAnswerSupportRefMemberLocation(ref)
		if !ok || member != want[i] || location.File != fmt.Sprintf("src/worker_%03d.py", i) || location.LineStart != 7 || location.LineEnd != 7 {
			t.Fatalf("member[%d] lost its own source identity: %q", i, ref)
		}
	}
}
