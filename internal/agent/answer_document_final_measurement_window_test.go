package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The public producer already knows the correct state totals. Its recursive
// dependency windows must not become those states' occurrence endpoints in the
// actual finalizer instruction, including its last compact-authority section.
func TestFinalMeasurementWindowPublicFinalizer(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, fixture, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("trace absent")
	}
	fixture, _, ok = strings.Cut(fixture, "\n'\n")
	if !ok {
		t.Fatal("trace not closed")
	}
	for _, lang := range []string{"zh", "en"} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/renamed=%t", lang, renamed), func(t *testing.T) {
				trace := fixture
				names := []string{"app", "cookie", "network", "threadpool"}
				if renamed {
					for i, name := range names {
						names[i] = []string{"client", "broker", "transport", "worker"}[i]
						trace = strings.ReplaceAll(trace, name, names[i])
					}
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "chain.ftrace")
				if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
					t.Fatal(err)
				}
				start, end := 2.0, 2.020
				query := func(view string, pid int) types.ToolResult {
					params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
					result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
					if err != nil || !result.Success {
						t.Fatalf("public %s: %v %s", view, err, result.Summary)
					}
					return result
				}
				rank := query("root_cause_rank", 100)
				timeline := query("thread_timeline", 300)
				// A true timeline segment retains its exact endpoints. No range
				// label is inferred from equality between a total and its width.
				if !strings.Contains(timeline.Summary, "s_sleep 2.002000..2.016000 14.000ms") {
					t.Fatalf("true timeline segment lost: %s", timeline.Summary)
				}
				ctx := tracePrincipalValueAuthorityTestContext(names[0]+"-100", 100, rank.Observations)
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{rank, timeline}})
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				ctx = ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}, types.AgentFinalizer, types.StageFinalize)
				before, _ := json.Marshal(answerDocObservationLedger(ctx))
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				prompt := ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "final-measurement-window-test"})
				if len(prompt.UserSections) == 0 || !strings.Contains(instruction, "## Final Trace Decision Boundary") {
					t.Fatal("actual finalizer context missing")
				}
				for _, want := range []struct{ subject, measured, window string }{
					{names[1] + "-200", "17.000", "2.000000..2.020000"},
					{names[2] + "-300", "14.000", "2.001000..2.018000"},
					{names[3] + "-400", "11.000", "2.002000..2.016000"},
				} {
					line := finalMeasurementWindowLine(instruction, "- state_value_authority ", "subject=`"+want.subject+"`")
					for _, token := range []string{"measured_state_occupancy=" + want.measured + "ms", "effective_attribution=1.000ms", "locator_range=`" + want.window + "`", "dependency analysis window", "not a continuous state interval"} {
						if !strings.Contains(line, token) {
							t.Errorf("state authority missing %q: %s", token, line)
						}
					}
					if strings.Contains(line, "occurrence_interval=") {
						t.Errorf("dependency domain masquerades as a state occurrence: %s", line)
					}
				}
				leader := finalMeasurementWindowLine(instruction, "  - validation_direction=", "leader_subject=`"+names[1]+"-200`; leader_effective_attribution=")
				if !strings.Contains(leader, "locator_range=`2.000000..2.020000`") || !strings.Contains(leader, "dependency analysis window") || strings.Contains(leader, "occurrence_interval=") {
					t.Errorf("compact leader still claims an occurrence: %s", leader)
				}
				after, _ := json.Marshal(answerDocObservationLedger(ctx))
				if string(before) != string(after) {
					t.Fatal("display changed source observations, values, or requested window")
				}
			})
		}
	}
}

func finalMeasurementWindowLine(text, prefix, contains string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, contains) {
			return line
		}
	}
	return ""
}

func TestFinalMeasurementWindowLocatorSourceBinding(t *testing.T) {
	base := types.ObservationRecord{
		ID: "exact-record", Producer: "trace_query", Subject: "renamed-42",
		Predicate: "root_cause_secondary", Object: "priority_inversion_candidate",
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/one.trace", ArtifactID: "one.trace"},
		Span:      types.ObservationSpan{LineStart: 3, LineEnd: 9, StartTs: 1, EndTs: 1.017},
		Value:     "17.000", Unit: "ms", ObservedAt: "typed-query",
		RichNotes: []string{types.TraceNoteKeySource + "=wakeup_chain.causal_impacts"},
	}
	nodeFor := func(record types.ObservationRecord) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: record.ID, Subject: record.Subject, Predicate: record.Predicate,
			StartTs: record.Span.StartTs, EndTs: record.Span.EndTs,
			StateKind: "s_sleep", SleepMS: 17, EffectiveImpactMS: 1, CumulativeImpactMS: 1,
			MeasurementOrigins: types.TraceSchedulerMeasurementOriginsFromRecord(record),
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*types.TraceCausalProjectionNode, *[]types.ObservationRecord)
		want string
	}{
		{"exact_recursive", nil, "dependency analysis window"},
		{"aggregate_recursive", func(n *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].Predicate, n.Predicate = "wakeup_causal_aggregate", "wakeup_causal_aggregate"
		}, "dependency analysis window"},
		{"unknown_source", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].RichNotes = []string{types.TraceNoteKeySource + "=unknown"}
		}, "record range (single occurrence unproven)"},
		{"source_prose_not_identity", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].RichNotes = []string{types.TraceNoteKeySource + "=prose wakeup_chain.causal_impacts"}
		}, "record range (single occurrence unproven)"},
		{"non_query_producer", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].Producer = "perf_trace"
		}, "record range (single occurrence unproven)"},
		{"duplicate_id", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			*r = append(*r, (*r)[0])
		}, "record range (single occurrence unproven)"},
		{"duplicate_id_other_capture", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			other := (*r)[0]
			other.SourceRef.Path = "/captures/two.trace"
			*r = append(*r, other)
		}, "record range (single occurrence unproven)"},
		{"same_id_other_capture", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].SourceRef.Path = "/captures/two.trace"
		}, "record range (single occurrence unproven)"},
		{"unknown_capture", func(n *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			(*r)[0].SourceRef = types.ObservationSourceRef{}
			n.MeasurementOrigins = types.TraceSchedulerMeasurementOriginsFromRecord((*r)[0])
		}, "record range (single occurrence unproven)"},
		{"changed_bounds", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.EndTs = 1.020
		}, "record range (single occurrence unproven)"},
		{"merged_origins", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.MeasurementOrigins = append(n.MeasurementOrigins, n.MeasurementOrigins[0])
		}, "record range (single occurrence unproven)"},
		{"missing_origins", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.MeasurementOrigins = nil
		}, "record range (single occurrence unproven)"},
		{"different_subject", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.Subject = "other-42"
		}, "record range (single occurrence unproven)"},
		{"different_predicate", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.Predicate = "wakeup_causal_impact"
		}, "record range (single occurrence unproven)"},
		{"missing_record", func(_ *types.TraceCausalProjectionNode, r *[]types.ObservationRecord) {
			*r = nil
		}, "record range (single occurrence unproven)"},
		{"missing_id", func(n *types.TraceCausalProjectionNode, _ *[]types.ObservationRecord) {
			n.EvidenceID = ""
		}, "record range (single occurrence unproven)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := []types.ObservationRecord{base}
			node := nodeFor(base)
			if tc.edit != nil {
				tc.edit(&node, &records)
			}
			before, _ := json.Marshal([]any{node, records})
			var b strings.Builder
			traceDecisionWriteMeasurementLocator(&b, node, records)
			got := b.String()
			if !strings.Contains(got, tc.want) || !strings.Contains(got, fmt.Sprintf("locator_range=`%.6f..%.6f`", node.StartTs, node.EndTs)) || strings.Contains(got, "occurrence_interval=") {
				t.Fatalf("locator borrowed a stronger role or changed bounds: %s", got)
			}
			after, _ := json.Marshal([]any{node, records})
			if string(before) != string(after) {
				t.Fatal("display mutated source identity or measurements")
			}
		})
	}
	for _, source := range []string{"top_sleep", "top_runnable", "top_running", "top_io_wait", "top_d_state", "state_churn", "unknown", ""} {
		t.Run("drilldown/"+source, func(t *testing.T) {
			record := base
			record.Predicate = "state_drilldown"
			record.RichNotes = []string{types.TraceNoteKeySource + "=" + source}
			node := nodeFor(record)
			projection := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{BackgroundCauses: []types.TraceCausalProjectionNode{node}}}}
			got := renderTraceFinalStateValueAuthority(projection, record)
			want := "cumulative state measurement scope"
			if source == "unknown" || source == "" {
				want = "state drilldown measurement scope (single occurrence unproven)"
			}
			if !strings.Contains(got, want) || strings.Contains(got, "occurrence_interval=") {
				t.Fatalf("a 17ms total in a 17ms hull still cannot mint an occurrence: %s", got)
			}
		})
	}
	for _, bounds := range [][2]float64{{1, 0}, {1, 1}, {1, math.NaN()}, {1, math.Inf(1)}, {math.NaN(), 2}, {math.Inf(-1), 2}, {math.Inf(1), 2}, {-1, 2}} {
		node := nodeFor(base)
		node.StartTs, node.EndTs = bounds[0], bounds[1]
		var b strings.Builder
		traceDecisionWriteMeasurementLocator(&b, node, []types.ObservationRecord{base})
		if b.Len() != 0 {
			t.Errorf("invalid range was published: %s", b.String())
		}
	}
	// Rebased traces can start at zero. Keep that coordinate without turning
	// a valid locator into proof of one continuous state segment.
	zero := base
	zero.Span.StartTs, zero.Span.EndTs = 0, .017
	var b strings.Builder
	traceDecisionWriteMeasurementLocator(&b, nodeFor(zero), []types.ObservationRecord{zero})
	if !strings.Contains(b.String(), "locator_range=`0.000000..0.017000`") || !strings.Contains(b.String(), "dependency analysis window") {
		t.Errorf("zero-origin trace locator was lost: %s", b.String())
	}
}
