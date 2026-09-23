package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The analyzer may legitimately classify occupancy as count/duration rather
// than pressure. Already-queried resource evidence must survive that choice;
// rendering it neither changes the classification nor requires new analysis.
func TestIOInFlightPublicHandoffAcrossClassifications(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name    string
			profile *types.RuntimeQuestionProfile
		}{
			{"count_duration", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}},
			{"io_latency", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}}},
			{"resource_pressure", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}},
			{"no_fact_family", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}},
			{"no_profile", nil},
			{"causal", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx, result, native := ioInFlightPublicContext(t, lang, types.RuntimeQuestionScopeBoundedFactSet, "")
				if native == nil || len(native.Groups) != 4 || native.Window == nil || native.Window.StartTs != 1 || native.Window.EndTs != 1.01 {
					t.Fatalf("public native query prerequisite failed: %+v", native)
				}
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = tc.profile
				ctx.AnalysisIR.RequestModel.RuntimeTargets = nil // The live request has no named thread.
				ledger := answerDocObservationLedger(ctx)
				before, err := json.Marshal([]any{result, ledger, ctx.AnalysisIR.RequestModel})
				if err != nil {
					t.Fatal(err)
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				const heading = "#### IO In-Flight Measurements"
				if n := strings.Count(prompt, heading); n != 1 {
					t.Errorf("independent occupancy handoff count=%d, want exactly one for %s", n, tc.name)
				}
				_, section, _ := strings.Cut(prompt, heading)
				section, _, _ = strings.Cut(section, "\n### ")
				seenGroups, seenCoverage := 0, 0
				var occupancy []types.ObservationRecord
				for _, row := range ledger.Records {
					if row.Predicate != "io_inflight" && row.Predicate != "io_inflight_coverage" {
						continue
					}
					occupancy = append(occupancy, row)
					if row.Predicate == "io_inflight" {
						seenGroups++
					} else {
						seenCoverage++
					}
					line := ""
					for _, candidate := range strings.Split(section, "\n") {
						if strings.HasPrefix(candidate, "  - id=`"+row.ID+"`;") {
							line = candidate
							break
						}
					}
					for _, want := range []string{row.Summary, row.SourceRef.Path, row.SourceRef.PayloadRef, "owner_scope=`selected_window_context`", "query_window=`1.000000..1.010000`"} {
						if !strings.Contains(line, want) {
							t.Errorf("same-ID occupancy %s missing %q in %s", row.ID, want, line)
						}
					}
					for _, note := range row.RichNotes {
						key, value, found := strings.Cut(note, "=")
						if found && strings.HasPrefix(key, "io_inflight_") && !strings.Contains(line, value) {
							t.Errorf("same-ID occupancy %s lost producer note %s", row.ID, note)
						}
					}
				}
				if seenGroups != 4 || seenCoverage != 2 {
					t.Fatalf("typed prerequisite: groups=%d coverage=%d", seenGroups, seenCoverage)
				}
				projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: occupancy})
				if projection.PrimaryRootCause != nil || len(projection.PrimaryRootCauses)+len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.BackgroundCauses) != 0 {
					t.Fatal("resource handoff acquired causal seats")
				}
				after, err := json.Marshal([]any{result, answerDocObservationLedger(ctx), ctx.AnalysisIR.RequestModel})
				if err != nil || string(before) != string(after) {
					t.Fatal("handoff mutated query evidence or model-owned classification")
				}
				// No classification may reintroduce the wider account for a
				// smaller explicit user window, even through the shared handoff.
				start, end := 1.002, 1.004
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				narrow := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if strings.Contains(narrow, heading) {
					t.Error("wider occupancy leaked into narrower explicit-window answer")
				}
			})
		}
	}
}

func TestIOInFlightPublicHandoffPreservesScopeAndAbsence(t *testing.T) {
	t.Run("named_target_count_does_not_authorize_global_pressure", func(t *testing.T) {
		ctx, _, native := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
		if native == nil || len(native.Groups) != 4 {
			t.Fatal("native occupancy prerequisite missing")
		}
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}
		if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); strings.Contains(prompt, "#### IO In-Flight Measurements") {
			t.Fatal("shared handoff bypassed existing named-target fact-family projection")
		}
	})
	t.Run("no_io_is_not_measured_zero", func(t *testing.T) {
		path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_scheduler_concurrency", "events.systrace"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, _, native := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, path)
		if native != nil {
			t.Fatal("non-IO fixture unexpectedly has an IO occupancy account")
		}
		if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); strings.Contains(prompt, "#### IO In-Flight Measurements") {
			t.Fatal("absent IO manufactured a measured-occupancy section")
		}
	})
	t.Run("nil_analysis_with_real_ledger", func(t *testing.T) {
		ctx, _, native := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
		if native == nil {
			t.Fatal("native occupancy prerequisite missing")
		}
		ctx.AnalysisIR = nil
		if prompt := renderAnswerDocObservationLedger(ctx); strings.Contains(prompt, "#### IO In-Flight Measurements") {
			t.Fatal("nil analysis manufactured the classified occupancy handoff")
		}
	})
}
