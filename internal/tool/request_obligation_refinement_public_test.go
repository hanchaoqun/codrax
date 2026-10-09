package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnalysisRequestObligationRefinementPublic(t *testing.T) {
	previous := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(previous) })
	for _, tc := range []struct {
		name       string
		profile    any
		role       string
		binding    string
		wantSource bool
	}{
		{name: "explicit false completes actual measurements", profile: map[string]any{"is_current_source_explanation_requested": false}},
		{name: "omission preserves initial obligation", wantSource: true},
		{name: "missing boolean is not explicit false", profile: map[string]any{}, wantSource: true},
		{name: "positive mixed explanation", profile: map[string]any{"is_current_source_explanation_requested": true, "source_quotes": []string{"implementation behavior"}, "modes": []string{"explain_current_mechanism"}}, wantSource: true},
		{name: "false cannot remove current code role", profile: map[string]any{"is_current_source_explanation_requested": false}, role: "current_key_code", wantSource: true},
		{name: "false cannot remove Makefile binding", profile: map[string]any{"is_current_source_explanation_requested": false}, binding: "Makefile", wantSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := documentationAnalysisPayload(t)
			payload["scenario"] = "generic"
			if tc.profile != nil {
				payload["current_source_explanation_profile"] = tc.profile
			}
			if tc.role != "" {
				payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["role"] = tc.role
			}
			if tc.binding != "" {
				payload["required_files"] = []any{map[string]any{"path": tc.binding, "confidence": 1.0, "requested_dimension_indices": []int{1}, "rationale": "independent implementation"}}
			}
			root := t.TempDir()
			if tc.binding != "" {
				if err := os.WriteFile(filepath.Join(root, tc.binding), []byte("all:\n\t@echo ready\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx := &types.BusContext{Mutable: types.NewMutableState("Explain documented units and implementation behavior"), RepoRoot: root, WorkDir: root, TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation}}
			originalHint := ctx.TurnRouteHint
			ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}}})
			encoded, _ := json.Marshal(payload)
			result, err := (&EmitAnalysis{}).Execute(ctx, encoded)
			if err != nil || !result.Success || ctx.Mutable.RequestModel() == nil {
				t.Fatalf("public analysis failed: %v %s", err, result.Summary)
			}
			rm := ctx.Mutable.RequestModel()
			if tc.profile != nil && tc.name != "missing boolean is not explicit false" && rm.CurrentSourceExplanationProfile == nil {
				t.Fatal("explicit declaration became omission")
			}
			if tc.name == "missing boolean is not explicit false" && rm.CurrentSourceExplanationProfile != nil {
				t.Fatal("missing boolean minted false declaration")
			}
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
			if authority.CurrentSourceRequired != tc.wantSource || ctx.TurnRouteHint != originalHint || rm.ExternalObservationPolicy.ExcludesCurrentSource() {
				t.Fatalf("obligation/permission convergence: %+v / %+v", authority, rm.ExternalObservationPolicy)
			}
			if tc.wantSource {
				return
			}
			if rm.ExternalObservationPolicy.CurrentSourceMode != types.ExternalObservationCurrentSourceDefault || strings.Contains(rm.ExternalObservationPolicy.Rationale, "requires current checkout evidence") {
				t.Fatalf("old synthesized policy survived: %+v", rm.ExternalObservationPolicy)
			}
			path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
			if err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 2, "time_end": 2.25})
			query, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !query.Success {
				t.Fatalf("query: %v %s", err, query.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(query)
			completed, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"The retained native measurements answer the requested observation.","confidence":"high","result_kind":"resolved"}`))
			if err != nil || !completed.Success || !ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("public completion retained false source debt: %v %+v", err, completed)
			}
		})
	}
}

func TestEmitAnalysisToolDocumentationGenericRolesRemainPublic(t *testing.T) {
	for _, role := range []string{"member_set", "count"} {
		payload := documentationAnalysisPayload(t)
		payload["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{1}}
		payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["role"] = role
		payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[1].(map[string]any)["role"] = "observed_value"
		result, rm := executeDocumentationAnalysis(t, payload)
		if !result.Success || rm == nil || !types.ToolDocumentationDimensionRequested(rm, 1) || types.ToolDocumentationDimensionRequested(rm, 2) {
			t.Fatalf("legal %s documentation harmed independent observation: %s", role, result.Summary)
		}
	}
}

func TestEmitAnalysisRejectedFalseCannotRefineAcceptedRequest(t *testing.T) {
	payload := documentationAnalysisPayload(t)
	ctx := &types.BusContext{Mutable: types.NewMutableState("Explain documented units and implementation behavior"), TurnRouteHint: types.TurnRouteHint{Source: "mixed", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation}}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace"}})
	call := func() types.ToolResult {
		data, _ := json.Marshal(payload)
		result, err := (&EmitAnalysis{}).Execute(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := call(); !result.Success {
		t.Fatalf("initial accepted request: %s", result.Summary)
	}
	before, _ := json.Marshal(ctx.Mutable.RequestModel())
	payload["current_source_explanation_profile"] = map[string]any{"is_current_source_explanation_requested": false, "confidence": 0.9}
	payload["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{1, 2}}
	for _, row := range payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any) {
		row.(map[string]any)["role"] = "observed_value"
	}
	if result := call(); result.Success || !strings.Contains(result.Summary, "index 1 carries an independent") || !strings.Contains(result.Summary, "index 2 carries an independent") {
		t.Fatalf("invalid mixed declaration was not rejected as one complete diagnostic: %s", result.Summary)
	}
	after, _ := json.Marshal(ctx.Mutable.RequestModel())
	if string(before) != string(after) || !types.EffectiveRequestRouteHint(ctx.Mutable.RequestModel(), ctx.TurnRouteHint).RequiresCurrentSourceEvidence() {
		t.Fatal("rejected candidate lowered accepted source obligations")
	}
	delete(payload, "tool_documentation_request")
	if result := call(); !result.Success {
		t.Fatalf("complete repair rejected: %s", result.Summary)
	}
	if types.EffectiveRequestRouteHint(ctx.Mutable.RequestModel(), ctx.TurnRouteHint).RequiresCurrentSourceEvidence() {
		t.Fatal("accepted false failed to refine route-only obligation")
	}
}
