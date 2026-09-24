package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryRuntimeContextPreservesPinnedCompletionRead(t *testing.T) {
	ctx := sourceInventoryRuntimeAnalysisContext(t)
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "Makefile"), []byte("all:\n\ttrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{UserPinnedFiles: []string{"Makefile"}, AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "Makefile", Confidence: 1}}}}}
	closure := types.NewEvidenceClosure(ctx.RepoRoot)
	raiseRequiredFileHintPendingReads(ctx, closure, nil)
	pending := closure.PendingReads()
	if len(pending) != 1 || pending[0].File != "Makefile" {
		t.Fatalf("completion dropped independently pinned file: %+v", pending)
	}
}

func TestSourceInventoryRuntimeRestoredObservationRemainsSupport(t *testing.T) {
	for _, carrier := range []string{"route", "attachment", "accepted_bundle"} {
		t.Run(carrier, func(t *testing.T) {
			ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta"})
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{HasPerMemberTable: true}, SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRolePackage}, SourceQuotes: []string{"records"}, Confidence: 1}}}
			observation := ctx.Mutable.SourceInventoryObservation()
			observation.Complete = false
			observation.SourceClasses = []types.SourceInventorySourceClassCount{{Role: types.SourcePathRoleProduction, Count: 2, Complete: false}}
			ctx.Mutable.SetSourceInventoryObservation(observation)
			switch carrier {
			case "route":
				ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			case "attachment":
				ctx.AttachedHitrace = "runtime artifact"
			case "accepted_bundle":
				ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed"}}})
			}
			beforeIR, _ := json.Marshal(ctx.AnalysisIR)
			beforeObservation, _ := json.Marshal(ctx.Mutable.SourceInventoryObservation())
			snapshot := sourceInventoryAuthoritySnapshotForCompletion(ctx, observation, nil)
			preemit := BuildSourceInventoryAnswerPreEmitAuthority(ctx, nil)
			if snapshot.PrincipalAuthority || snapshot.CompletionAuthority.Blocking || preemit.Blocking || preemit.View.PrincipalAuthority || execCommandActiveSourceInventoryProfile(ctx) {
				t.Fatalf("restored unknown declaration regained hard source authority: snapshot=%+v preemit=%+v", snapshot, preemit)
			}
			if len(snapshot.PrincipalRowSet.SupportRows) != 2 || len(snapshot.PrincipalRowSet.PrincipalRows) != 0 {
				t.Fatalf("inventory was deleted rather than kept as support: %+v", snapshot.PrincipalRowSet)
			}
			afterIR, _ := json.Marshal(ctx.AnalysisIR)
			afterObservation, _ := json.Marshal(ctx.Mutable.SourceInventoryObservation())
			if string(beforeIR) != string(afterIR) || string(beforeObservation) != string(afterObservation) || ctx.AnalysisIR.RequestModel.PerfTrace != nil {
				t.Fatal("policy projection mutated IR/evidence")
			}
			ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"src/owner.go"}
			if !execCommandActiveSourceInventoryProfile(ctx) || !BuildSourceInventoryAnswerPreEmitAuthority(ctx, nil).Blocking {
				t.Fatal("precise current-source obligation lost its original blocking view")
			}
		})
	}
}

func TestSourceInventoryRuntimeOptionalSynthesisDoesNotMintSource(t *testing.T) {
	for _, bundle := range []bool{true, false} {
		t.Run(map[bool]string{true: "bundle", false: "large_attachment"}[bundle], func(t *testing.T) {
			rm := types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{HasPerMemberTable: true}}
			ctx := &types.BusContext{}
			if bundle {
				rm.PerfTrace = &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed"}}}
			} else {
				ctx.RuntimeArtifactPreflight = lensburnLargeTracePreflight()
			}
			if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, "renamed records", nil); warning != "" || rm.SourceInventoryProfile != nil {
				t.Fatalf("runtime-only table synthesized source inventory: warning=%q profile=%+v", warning, rm.SourceInventoryProfile)
			}
		})
	}
}

func TestSourceInventoryRuntimeOptionalCompletionDoesNotDemandSource(t *testing.T) {
	rm := types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{HasPerMemberTable: true},
		PerfTrace:              &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed"}}},
		SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}, Confidence: 1},
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: types.NewMutableState("renamed records")}
	if currentSourceForcedReadGatesApply(ctx) {
		t.Fatal("optional runtime profile overrides shared source requirement")
	}
}

func sourceInventoryRuntimeAnalysisPayload(t *testing.T, profile map[string]any) json.RawMessage {
	t.Helper()
	base := withRequiredAnswerRoleProfile(withV4Required(`{"intent":"enumerate","scenario":"generic","complexity":"simple","keywords":["block_rq","block_bio","mmc","f2fs","worker","read","write","duration"],"entities":[],"question_kind":"enumeration"}`))
	var wire map[string]any
	if err := json.Unmarshal([]byte(base), &wire); err != nil {
		t.Fatal(err)
	}
	wire["predicates"].(map[string]any)["has_per_member_table"] = true
	wire["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "fact_families": []string{"count_or_duration"}, "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": 0.9}
	wire["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 1, "time_end": 14, "source_quote": "1..14 seconds", "confidence": 0.9}
	if profile != nil {
		wire["source_inventory_profile"] = profile
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sourceInventoryRuntimeAnalysisContext(t *testing.T) *types.BusContext {
	t.Helper()
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("List the records and their measured durations in 1..14 seconds"),
		TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed-stage"}}})
	return ctx
}

func TestSourceInventoryRuntimePublicAnalysisQueryCompletion(t *testing.T) {
	ctx := sourceInventoryRuntimeAnalysisContext(t)
	result, err := (&EmitAnalysis{}).Execute(ctx, sourceInventoryRuntimeAnalysisPayload(t, nil))
	if err != nil || !result.Success {
		t.Fatalf("analysis failed: %+v %v", result, err)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("runtime table minted source inventory: %+v", rm)
	}
	if rm.RuntimeArtifactScopeProfile == nil || *rm.RuntimeArtifactScopeProfile.TimeStart != 1 || *rm.RuntimeArtifactScopeProfile.TimeEnd != 14 {
		t.Fatalf("scope changed: %+v", rm.RuntimeArtifactScopeProfile)
	}
	out := compiler.Compile(*rm, budget.BudgetSignals{})
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, TaskGraph: out.TaskGraph, EvidencePlan: out.EvidencePlan, AnswerContract: out.AnswerContract}
	if currentSourceForcedReadGatesApply(ctx) || SourceInventoryLensExecutionGapForContext(ctx).Blocking {
		t.Fatal("compiled runtime table acquired source completion debt")
	}
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 14})
	query, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !query.Success || len(query.Observations) == 0 {
		t.Fatalf("real query failed: %+v %v", query, err)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	missingMembers, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"Measurements available","confidence":"high","result_kind":"resolved"}`))
	if err != nil || ctx.Mutable.IsInvestigationComplete() || !strings.Contains(missingMembers.Summary, "member-set obligation") {
		t.Fatalf("source optionality waived an independent member-set obligation: %+v %v", missingMembers, err)
	}
	complete, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"Accepted trace measurements answer the requested time window","confidence":"high","result_kind":"resolved","aggregate_facts":[{"kind":"member_set","label":"Observed storage request families","value":"2","members":["block_rq","block_bio"],"evidence_origin":"runtime_artifact"}]}`))
	if err != nil || !complete.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("runtime complete failed: %+v %v", complete, err)
	}
	after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	if string(before) != string(after) {
		t.Fatal("completion mutated query evidence")
	}
	if !reflect.DeepEqual(ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile, rm.RuntimeArtifactScopeProfile) {
		t.Fatal("completion changed explicit scope")
	}
	// Exercise the shared finalizer input after the real completion, not a
	// synthetic source-authority record or only the analyzer's classification.
	finalView := types.BuildAnswerSemanticViewForBusContext(ctx)
	if finalView.SourceInventoryRowIdentityAvailable || len(finalView.RuntimeMeasurementContract.Choices()) == 0 {
		t.Fatal("finalizer lost native measurements or gained source-inventory rows")
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if authority.CurrentSourceRequirement != types.RuntimeSourceRequirementNone || authority.CanHardBlockCompletion || !authority.AllowsProceedWithoutAdditionalCurrentSourceRead() || authority.RuntimeObservationCount == 0 || authority.CurrentSourceRecordCount != 0 {
		t.Fatalf("finalizer source posture disagrees with completed runtime query: %+v", authority)
	}
	if ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("enumeration completion manufactured causal authority")
	}
}

func TestSourceInventoryRuntimeMultipleWindowsAndTopicsRemainTyped(t *testing.T) {
	ctx := sourceInventoryRuntimeAnalysisContext(t)
	ctx.Mutable = types.NewMutableState("List first-phase records in 1..2 seconds and second-phase records in 5..7 seconds")
	ctx.RuntimeArtifactPreflight = lensburnLargeTracePreflight()
	var payload map[string]any
	if err := json.Unmarshal(sourceInventoryRuntimeAnalysisPayload(t, nil), &payload); err != nil {
		t.Fatal(err)
	}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_windows": []any{
		map[string]any{"time_start": 1, "time_end": 2, "source_quote": "1..2 seconds"},
		map[string]any{"time_start": 5, "time_end": 7, "source_quote": "5..7 seconds"},
	}, "confidence": 0.9}
	payload["sub_topics"] = []any{map[string]any{"summary": "first-phase records", "entities": []string{"first-phase"}}, map[string]any{"summary": "second-phase records", "entities": []string{"second-phase"}}}
	raw, _ := json.Marshal(payload)
	result, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("multi-scope analysis failed: %+v %v", result, err)
	}
	rm := ctx.Mutable.RequestModel()
	if rm.SourceInventoryProfile != nil || len(rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows()) != 2 || len(rm.SubTopics) != 2 {
		t.Fatalf("runtime inventory repair lost windows/topics: %+v", rm)
	}
	windows := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows()
	if *windows[0].TimeStart != 1 || *windows[0].TimeEnd != 2 || *windows[1].TimeStart != 5 || *windows[1].TimeEnd != 7 {
		t.Fatal("disjoint time windows were widened or reordered")
	}
}

func TestSourceInventoryRuntimeModelOriginIsSystemOwned(t *testing.T) {
	profile := map[string]any{"is_source_inventory": true, "target_roles": []string{"function"}, "requested_fields": []string{"name"}, "source_quotes": []string{"records"}}
	ctx := sourceInventoryRuntimeAnalysisContext(t)
	result, err := (&EmitAnalysis{}).Execute(ctx, sourceInventoryRuntimeAnalysisPayload(t, profile))
	if err != nil || !result.Success {
		t.Fatalf("provided profile analysis failed: %+v %v", result, err)
	}
	rm := ctx.Mutable.RequestModel()
	if rm.SourceInventoryProfile == nil || rm.SourceInventoryProfile.DeclarationOrigin != types.SourceInventoryDeclarationModelProvided {
		t.Fatalf("normalized declaration lost: %+v", rm.SourceInventoryProfile)
	}
	if types.SourceInventoryPrincipalAuthorityActive(*rm) || types.SourceInventoryRequiredFileCoverageShape(*rm) || !types.SourceInventoryPrincipalNavigationActive(*rm) {
		t.Fatal("provided low-precision declaration must remain soft navigation")
	}
	if !strings.Contains(result.Summary, "low-confidence advisory") {
		t.Fatalf("default confidence teaching disappeared: %s", result.Summary)
	}
	profile["declaration_origin"] = "model_provided"
	ctx = sourceInventoryRuntimeAnalysisContext(t)
	result, err = (&EmitAnalysis{}).Execute(ctx, sourceInventoryRuntimeAnalysisPayload(t, profile))
	if result.Success || ctx.Mutable.RequestModel() != nil {
		t.Fatalf("model could forge system origin: %+v %v", result, err)
	}
	if !strings.Contains(result.Summary, "declaration_origin") {
		t.Fatalf("missing precise invalid-field repair: %s", result.Summary)
	}
}
