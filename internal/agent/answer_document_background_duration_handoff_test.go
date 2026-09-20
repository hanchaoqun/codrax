package agent

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBackgroundContextDoesNotInferMeasurementFromMilliseconds(t *testing.T) {
	for _, aggregate := range []bool{false, true} {
		n := types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "io_latency", ImpactMS: 18.55, Unit: "ms"}
		if aggregate {
			n.SubjectKind = types.TraceCausalSubjectKindAggregateMetric
		}
		value, meaning := traceDecisionContextValuePresentation(n)
		if value != "18.550ms" || strings.Contains(meaning, "measured") || strings.Contains(meaning, "measurement") {
			t.Errorf("legacy/uncalibrated scalar became measured solely because it has ms: %q %q", value, meaning)
		}
		n.ImpactMS, n.RankValueCaliber = 47, types.TraceRankValueCaliberNativeDuration
		value, meaning = traceDecisionContextValuePresentation(n)
		if value != "47.000ms" || !strings.Contains(meaning, "measured") {
			t.Errorf("native duration marker lost its positive measurement meaning: %q %q", value, meaning)
		}
	}
}

func TestBackgroundDurationHandoffRetainsNativeFamilyRuler(t *testing.T) {
	for _, fold := range []string{"sum_disjoint", "interval_union", "max_overlap_fallback"} {
		t.Run(fold, func(t *testing.T) {
			n := types.TraceCausalProjectionNode{
				Subject: "worker-903", Predicate: "root_cause_background", Object: "workqueue_activity", TypeToken: "workqueue_activity",
				ImpactMS: 23, Unit: "ms", RankValueCaliber: types.TraceRankValueCaliberNativeDuration,
				FamilyMemberCount: 2, FamilyMemberMaxMS: 23, FamilyFoldCaliber: fold,
			}
			before, _ := json.Marshal(n)
			text := renderAnswerDocTraceDecisionHandoffSet(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
				ArtifactLabel: "sample.systrace", BackgroundCauses: []types.TraceCausalProjectionNode{n},
			}}}, runtimeTraceGuidanceView{})
			want := types.FormatTraceFamilyMeasurement(2, 23, fold, "en")
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(line, "lane=`background`") && strings.Contains(line, "subject=`worker-903`") {
					for _, part := range []string{"value=23.000ms", want, "target_causal_authority=`not_provided`", "cross_axis_addition=`forbidden`"} {
						if !strings.Contains(line, part) {
							t.Errorf("native family lost %q at the actual handoff row: %s", part, line)
						}
					}
					after, _ := json.Marshal(n)
					if string(before) != string(after) {
						t.Fatal("handoff mutated native family facts")
					}
					return
				}
			}
			t.Fatal("native family missing from context")
		})
	}
}

// The native rank deliberately reduces a long off-chain request's ordering
// weight. The actual finalizer must receive its measured time, never that
// private ordering cap disguised as another measurement of the same request.
func TestBackgroundDurationPublicQueriesReachFinalizerWithoutRankingCap(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []struct {
		name       string
		start, end float64
	}{
		{"exploration", .999, 1.052},
		{"business_instance", 1, 1.05},
	} {
		t.Run(window.name, func(t *testing.T) {
			ctx := &types.AgentContext{
				RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
				AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Analyze the response using the recorded evidence"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Intent: types.IntentRootCause, Language: "zh", PerfTrace: &types.PerfBundle{},
					RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
				}},
			}
			for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
				params, err := json.Marshal(map[string]any{
					"source": "path", "path": path, "view": view, "pid": 100,
					"time_start": window.start, "time_end": window.end, "trace_flavor": "harmony_hitrace",
				})
				if err != nil {
					t.Fatal(err)
				}
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !result.Success {
					t.Fatalf("public %s: %v %+v", view, err, result)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			before, err := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if err != nil {
				t.Fatal(err)
			}
			ledger := answerDocObservationLedger(ctx)
			set := types.CompileTraceCausalProjectionSet(ledger)
			backgroundFound, onChainFound, requestFound := false, false, false
			for _, projection := range set.Projections {
				for _, node := range projection.BackgroundCauses {
					if node.Subject != "backup-900" || node.Object != "io_latency" {
						continue
					}
					backgroundFound = true
					if math.Abs(node.ImpactMS-47) > 1e-6 || node.EffectiveImpactMS != 0 || node.RankValueCaliber != types.TraceRankValueCaliberNativeDuration {
						t.Errorf("background request is not measured 47ms with zero causal attribution: %+v", node)
					}
				}
				for _, node := range projection.OnChainCauses {
					if node.Subject == "backup-900" {
						t.Errorf("measurement recovery promoted background request onto chain: %+v", node)
					}
					if node.Subject == "document-worker-200" && node.Object == "io_latency" && math.Abs(node.EffectiveImpactMS-31) < 1e-6 {
						onChainFound = true
					}
				}
			}
			for _, record := range ledger.Records {
				if record.Subject == "document-worker-200" && record.Predicate == "io_latency" && record.Value == "35.000" &&
					strings.Contains(strings.Join(record.RichNotes, "\n"), "issuer_blocked=31.000") {
					requestFound = true
				}
			}
			if !backgroundFound || !onChainFound || !requestFound {
				t.Fatalf("native three rulers lost: background=%t chain=%t request=%t", backgroundFound, onChainFound, requestFound)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			contextFound := false
			for _, line := range strings.Split(prompt, "\n") {
				if !strings.Contains(line, "subject=`backup-900`") || !strings.Contains(line, "kind=`io_latency`") || !strings.Contains(line, "reader_calibration=") {
					continue
				}
				contextFound = true
				for _, want := range []string{"lane=`background`", "value=47.000ms", "target_causal_authority=`not_provided`", "cross_axis_addition=`forbidden`"} {
					if !strings.Contains(line, want) {
						t.Errorf("actual finalizer context is missing %q: %s", want, line)
					}
				}
			}
			if !contextFound {
				t.Fatal("finalizer did not receive a measured background request row")
			}
			after, err := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if err != nil || string(before) != string(after) {
				t.Fatal("context construction mutated the accepted tool evidence")
			}
		})
	}
}
