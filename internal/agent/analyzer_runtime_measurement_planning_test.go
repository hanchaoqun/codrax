package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeMeasurementPlanningContext(t *testing.T) (*types.AgentContext, types.RequestModel) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.25
	rm := types.RequestModel{
		RawRequest: "Report measurement counts between 2 and 2.25 seconds; do not analyze code.", Language: "en",
		Intent: types.IntentEnumerate, Scenario: types.ScenarioGeneric, Complexity: types.ComplexitySimple,
		AnalyzerHints:               types.AnalyzerHints{Entities: []string{"capture_alpha", "capture_beta", "capture_gamma", "capture_delta"}},
		Predicates:                  types.SemanticPredicates{IsCrossComponent: true, HasPerMemberTable: true},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOtherObservedValue}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "between 2 and 2.25 seconds"},
		ExternalObservationPolicy:   &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"do not analyze code"}},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "systrace"}, Observations: []types.PerfObservation{{}}})
	mut.SetRequestModel(rm)
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut, AttachedHitrace: material.Preview(), AttachedHitraceSource: path, AttachedTraceMaterial: material,
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}})}
	return ctx, rm
}

func TestBuildAnalysisIRRuntimeMeasurementPlanningPreservesDimensionsAndFinalizer(t *testing.T) {
	ctx, rm := runtimeMeasurementPlanningContext(t)
	rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1}
	for i := 1; i <= 19; i++ {
		label := fmt.Sprintf("measurement%d", i)
		rm.RawRequest += " " + label
		rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Index: i, Label: label, SourceQuote: label, Role: types.RequestedAnswerDimensionObservedValue, Required: true})
	}
	ctx.Mutable.SetRequestModel(rm)
	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.RequestModel.SubTopics) != 0 || !reflect.DeepEqual(ir.RequestModel.RequestedAnswerDimensions, rm.RequestedAnswerDimensions) || !ir.RequestModel.Predicates.HasPerMemberTable {
		t.Fatalf("planning lost declared dimensions: %+v", ir.RequestModel.RequestedAnswerDimensions)
	}
	ctx.Stage = types.StageFinalize
	ctx.AnalysisIR = ir
	ctx.RepoRoot = t.TempDir()
	ctx.WorkDir = ctx.RepoRoot
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": ctx.AttachedTraceMaterial.QueryPath(), "view": "window_stats", "time_start": 2, "time_end": 2.25})
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("native measurement: %v / %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	groups := map[string]bool{}
	for _, choice := range choices {
		if choice.View != types.RuntimeMeasurementSummary {
			continue
		}
		groups[choice.ObservationID] = true
		if !strings.Contains(prompt, "observation_id=\""+choice.ObservationID+"\"") {
			t.Fatalf("actual finalizer dropped native selector %s", choice.ObservationID)
		}
	}
	if len(groups) < 8 {
		t.Fatalf("lost measured endpoint populations: %d", len(groups))
	}
	if ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("planning or finalizer preview manufactured completion/root authority")
	}
}

func TestBuildAnalysisIRRuntimeMeasurementPlanningMaterialBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		alter    func(*testing.T, *types.AgentContext, *types.RequestModel)
		optimize bool
	}{
		{"bounded preview", nil, true},
		{"no receipt", func(_ *testing.T, c *types.AgentContext, _ *types.RequestModel) { c.AttachedTraceMaterial = nil }, false},
		{"two sources", func(_ *testing.T, c *types.AgentContext, _ *types.RequestModel) {
			c.RuntimeArtifactPreflight.Artifacts = append(c.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: "/another.systrace", Carrier: "request_path"})
		}, false},
		{"log plus trace", func(_ *testing.T, c *types.AgentContext, _ *types.RequestModel) { c.AttachedLog = "separate log" }, false},
		{"bundle multiple physical members", func(t *testing.T, c *types.AgentContext, _ *types.RequestModel) {
			dir := t.TempDir()
			p := filepath.Join(dir, "capture.tracebundle.json")
			member := filepath.Join(dir, "events.systrace")
			bindings := map[string]filegeneration.Identity{}
			for _, path := range []string{p, member} {
				if err := os.WriteFile(path, []byte("# material\n"), 0600); err != nil {
					t.Fatal(err)
				}
				id, err := filegeneration.FromPath(path)
				if err != nil {
					t.Fatal(err)
				}
				bindings[path] = id
			}
			preview := "# codrax-source: " + p + "\n# preview\n"
			m, err := attachment.BindTraceMaterial(p, p, preview, bindings)
			if err != nil {
				t.Fatal(err)
			}
			c.AttachedTraceMaterial = m
			c.AttachedHitrace = preview
			c.AttachedHitraceSource = p
			c.RuntimeArtifactPreflight.Artifacts = []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: p, Carrier: "attachment"}}
		}, false},
		{"cancelled", func(_ *testing.T, c *types.AgentContext, _ *types.RequestModel) {
			x, cancel := context.WithCancel(context.Background())
			cancel()
			c.Ctx = x
		}, false},
		{"preview drift", func(_ *testing.T, c *types.AgentContext, _ *types.RequestModel) { c.AttachedHitrace += "changed" }, false},
		{"explicit topics", func(_ *testing.T, _ *types.AgentContext, r *types.RequestModel) {
			r.SubTopics = []types.SubTopic{{Summary: "first", Entities: []string{"capture_alpha"}}, {Summary: "second", Entities: []string{"capture_beta"}}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, rm := runtimeMeasurementPlanningContext(t)
			if tc.alter != nil {
				tc.alter(t, ctx, &rm)
			}
			ctx.Mutable.SetRequestModel(rm)
			ir, err := buildAnalysisIR(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.optimize {
				if len(ir.RequestModel.SubTopics) != 0 {
					t.Fatalf("names became topics: %+v", ir.RequestModel.SubTopics)
				}
			} else if len(ir.RequestModel.SubTopics) == 0 {
				t.Fatal("uncertain material/explicit topics lost legacy planning")
			}
			if rm.SubTopics != nil && !reflect.DeepEqual(rm.SubTopics, ir.RequestModel.SubTopics) {
				t.Fatal("changed explicit topics")
			}
			if !ir.RequestModel.Predicates.HasPerMemberTable || !ir.RequestModel.RuntimeQuestionProfile.BoundedFactSet() {
				t.Fatal("lost answer obligations")
			}
		})
	}
}
