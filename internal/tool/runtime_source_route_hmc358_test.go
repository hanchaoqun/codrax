package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC358SchedulerQueryWindowUsesNativeEndpoint(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(bus.RepoRoot, "scheduler.systrace")
	if err := os.WriteFile(path, []byte(schedulerConcurrencyPublicTrace), 0600); err != nil {
		t.Fatal(err)
	}
	for _, byLine := range []bool{false, true} {
		args := map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 1.01}
		if byLine {
			args["line_start"], args["line_end"] = 1, 8
		}
		result := hmc17NamedQuery(t, bus, args)
		payload := hmc17NamedPayload(t, result)
		if payload.WindowStats == nil || payload.WindowStats.SchedulerConcurrency == nil {
			t.Fatal("fixture did not exercise scheduler concurrency")
		}
		owner := make(map[string]bool)
		for _, r := range result.Observations {
			owner[r.ID] = r.Predicate == "scheduler_concurrency"
		}
		input := types.ObservationLedgerInputFromBusContext(bus, 0)
		input.ToolResults = []types.ToolResult{result}
		found := 0
		for _, table := range types.BuildRuntimeMeasurementContract(input).Choices() {
			if !owner[table.ObservationID] {
				continue
			}
			found++
			basis, ok := table.NativeSourceTitleBasis()
			if byLine {
				if basis.QueryWindow == "[1,1.01) s" || basis.QueryWindow == "[1,1.01] s" {
					t.Errorf("line range acquired a continuous half-open denominator: %+v", basis)
				}
			} else if !ok || basis.QueryWindow != "[1,1.01) s" || basis.QuerySource != payload.SourcePath {
				t.Errorf("native query window disagrees with its half-open body: %+v", basis)
			}
		}
		if found == 0 {
			t.Fatal("native scheduler display missing")
		}
	}
}

func TestHMC358AcceptedRuntimeRouteAndOriginAgree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   types.TurnRouteCurrentSourceEvidenceMode
		mixed  bool
		causal bool
		want   bool
	}{
		{"bounded_runtime", types.TurnRouteCurrentSourceEvidenceOptional, false, false, false},
		{"causal_runtime", types.TurnRouteCurrentSourceEvidenceOptional, false, true, false},
		{"independent_source_dimension", types.TurnRouteCurrentSourceEvidenceOptional, true, false, true},
		{"required_route", types.TurnRouteCurrentSourceEvidenceRequired, false, false, true},
		{"legacy_unspecified", types.TurnRouteCurrentSourceEvidenceUnspecified, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, payload := runtimeIntentPublicPayload(t)
			delete(payload, "current_source_explanation_profile")
			delete(payload, "external_observation_policy")
			role, label, quote := "observed_value", "events", "allocation events"
			if tc.causal {
				request += " What caused the observed allocation delay?"
				payload["runtime_question_profile"] = map[string]any{"scope": "causal_diagnosis", "runtime_work_relation_requested": true, "frame_causality_requested": false, "source_quote": "allocation events", "confidence": .95}
				role, label, quote = "causal_attribution", "cause of delay", "What caused the observed allocation delay?"
			}
			if tc.mixed {
				request += " Give the current source location in src/owner.go."
				role, label, quote = "source_location", "current source location", "current source location in src/owner.go"
			}
			payload["requested_answer_dimensions"] = map[string]any{
				"is_dimensioned_answer": true, "confidence": .95,
				"dimensions": []any{map[string]any{"index": 1, "role": role, "label": label, "source_quote": quote, "required": true}},
			}
			params, _ := json.Marshal(payload)
			hint := types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true, CurrentSourceEvidenceMode: tc.mode}
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), TurnRouteHint: hint}
			result, err := (&EmitAnalysis{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("public analysis rejected: %v %s", err, result.Summary)
			}
			rm := ctx.Mutable.RequestModel()
			intent := types.CompileAnswerIntentContract(*rm, nil)
			got := intent.HasOrigin(types.AnswerEvidenceOriginCurrentSource)
			if got != tc.want || rm.CurrentSourceLaneDecision().RequiresCurrentSource() != tc.want || !intent.HasOrigin(types.AnswerEvidenceOriginRuntimeArtifact) {
				t.Errorf("origin=%t lane=%s want source=%t; origins=%v", got, rm.CurrentSourceLaneDecision(), tc.want, intent.Origins)
			}
			if tc.mode != types.TurnRouteCurrentSourceEvidenceUnspecified {
				required := types.RuntimeSourceRequestCurrentSourceRequirementPrecision(rm, hint) != types.RuntimeSourceRequirementNone
				if required != tc.want {
					t.Errorf("source authority=%t want=%t", required, tc.want)
				}
			}
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm}
			presentation := types.CompileAnswerPresentationContract(ctx.AnalysisIR, nil)
			plan := types.CompileExploreLanePlan(*rm, nil, presentation)
			for _, lane := range plan.Lanes {
				if !tc.want && lane.Origin == types.AnswerEvidenceOriginCurrentSource {
					t.Errorf("explorer acquired an unrequested source lane: %+v", lane)
				}
			}
			if tc.causal && (!rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || !rm.RuntimeArtifactScopeProfile.Active()) {
				t.Fatal("route erased causal/explicit-window intent")
			}
			if rm.CurrentSourceExplanationProfile != nil || rm.PerfTrace != nil || rm.LogTriage != nil {
				t.Fatal("route manufactured a model declaration or measurement")
			}
		})
	}
}

func TestHMC358SamplingNoteSeparatesQueryAndPhysicalMember(t *testing.T) {
	bundle, member := samplingSourceBundleHMC241(t)
	result, _ := samplingIdentityQueryHMC239(t, bundle, "perf_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
	found := false
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}}).Choices() {
		if !table.HasNativeSamplingFacts() || table.View != types.RuntimeMeasurementSummary {
			continue
		}
		found = true
		notes := strings.Join(table.Notes, "\n")
		if !strings.Contains(notes, "查询材料："+bundle) || !strings.Contains(notes, "采样成员："+member) || strings.Contains(notes, "原件："+bundle) {
			t.Errorf("query receipt mislabeled as an original: %s", notes)
		}
	}
	if !found {
		t.Fatal("native sampling summary missing")
	}
}
