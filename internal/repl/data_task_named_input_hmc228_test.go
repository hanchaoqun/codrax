package repl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc228DataPlanner struct {
	plan       dataquery.TaskPlan
	candidates []dataquery.CandidateFile
}

func TestHMC228NativeDataHandoffPublicBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.data")
	if err := os.WriteFile(path, []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		paths   []string
		script  string
		actions []dataquery.DataAction
		cancel  bool
		request string
		good    bool
	}{
		{name: "exact_unknown", paths: []string{path}, good: true},
		{name: "empty"}, {name: "directory", paths: []string{filepath.Dir(path)}},
		{name: "same_basename", paths: []string{filepath.Join(t.TempDir(), "input.data")}},
		{name: "old_request", paths: []string{path}, request: "What remains?"},
		{name: "with_script", paths: []string{path}, script: "emit_result(1)"},
		{name: "with_action", paths: []string{path}, actions: []dataquery.DataAction{{Kind: dataquery.DataActionInspectMaterial}}},
		{name: "cancel", paths: []string{path}, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			request := tc.request
			if request == "" {
				request = "Read `" + path + "`."
			}
			p := &hmc228DataPlanner{plan: dataquery.TaskPlan{Status: "needs_native_reader", InputPaths: tc.paths, Script: tc.script, Actions: tc.actions}}
			_, err := RunDataTaskCLI(ctx, request, TurnPolicy{Route: RouteData, NeedsDataAccess: true}, DataTaskCLIConfig{Planner: p, RepoRoot: t.TempDir(), RuntimeAnchor: t.TempDir()})
			policy := TurnPolicy{Route: RouteData, NeedsDataAccess: true, DataTaskKind: "data_join", OperationKind: "data_join", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, PresentationDirective: "a table", RequiresDiagram: true}
			got, ok := NativeDataHandoffPolicy(policy, err)
			if ok != tc.good {
				t.Fatalf("delegated=%t error=%v", ok, err)
			}
			if ok && (got.OperationKind != "" || got.DataTaskKind != "" || got.NeedsDataAccess || got.PresentationDirective != policy.PresentationDirective || !got.RequiresDiagram || got.CurrentSourceEvidenceMode != policy.CurrentSourceEvidenceMode) {
				t.Fatalf("handoff policy drift: %+v", got)
			}
			if _, ok := NativeDataHandoffPolicy(TurnPolicy{Route: RouteWrite}, err); ok {
				t.Fatal("native data handoff changed write route")
			}
		})
	}
}

func TestHMC228REPLActualDataDispatchDelegatesOnceAndKeepsCurrentRequest(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	if err != nil {
		t.Fatal(err)
	}
	request := "Read `" + path + "` from 1 to 2 seconds."
	for _, mode := range []UserMode{UserModeAuto, UserModeData} {
		t.Run(string(mode), func(t *testing.T) {
			classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{Route: RouteData, NeedsDataAccess: true, Operation: "data_aggregation", DataTaskKind: "data_aggregation", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional, Source: "data", Confidence: .95, PresentationDirective: "native values table"}}
			r, runner, _ := newTurnPolicyREPL(t, newPolicyStore(t), classifier, &stubResponder{}, request+"\n/exit\n")
			p := &stubDataTaskPlanner{plan: dataquery.TaskPlan{Status: "needs_native_reader", InputPaths: []string{path}}}
			r.dataTaskPlanner = p
			r.repoRoot = t.TempDir()
			r.runtimeAnchor = t.TempDir()
			r.userMode = mode
			if err := r.Loop(); err != nil {
				t.Fatal(err)
			}
			if p.calls != 1 || p.evalCalls != 0 || p.repairCalls != 0 || len(runner.requests) != 1 || !strings.Contains(runner.requests[0], request) {
				t.Fatalf("dispatch plan=%d eval=%d repair=%d runs=%v", p.calls, p.evalCalls, p.repairCalls, runner.requests)
			}
			if runner.seenModes[0] != types.ModeRead || runner.seenRouteHints[0].Route != "repo" || runner.seenRouteHints[0].NeedsDataAccess {
				t.Fatalf("not read-only native dispatch: %+v %v", runner.seenRouteHints, runner.seenModes)
			}
			if r.attachedHitrace != "" || r.attachedLog != "" {
				t.Fatal("navigation became sticky attachment")
			}
		})
	}
}

func TestHMC228ActualPlannerSeesNativeCandidateAndSingleSharedTeaching(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"status": "needs_native_reader", "input_paths": []string{path}, "output_contract": map[string]any{"format": "markdown", "explanation_allowed": true, "complete_reference": false}})
	a := &scriptedChatAdapter{responses: []llm.Response{{ToolCalls: []llm.ToolCall{{Name: "emit_data_task_plan", Params: raw}}}}}
	_, err = RunDataTaskCLI(context.Background(), "Read `"+path+"`.", TurnPolicy{Route: RouteData, NeedsDataAccess: true}, DataTaskCLIConfig{Planner: NewDataTaskPlanner(a), RepoRoot: t.TempDir(), RuntimeAnchor: t.TempDir()})
	if _, ok := NativeDataHandoffPolicy(TurnPolicy{Route: RouteData}, err); !ok {
		t.Fatal(err)
	}
	if len(a.calls) != 1 {
		t.Fatalf("calls=%d", len(a.calls))
	}
	var system, user string
	for _, m := range a.calls[0].messages {
		if m.Role == "system" {
			system = m.Content
		}
		if m.Role == "user" {
			user = m.Content
		}
	}
	if strings.Count(system, dataTaskNativeReaderTeaching) != 1 || strings.Count(string(a.calls[0].tools[0].Parameters), dataTaskNativeReaderTeaching) != 1 || !strings.Contains(user, "native_reader_candidate=\"trace_query\"") {
		t.Fatalf("planner teaching/candidate missing: %s", user)
	}
}

func (p *hmc228DataPlanner) PlanDataTask(_ context.Context, _, _ string, _ TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	p.candidates = candidates
	return p.plan, nil
}

func TestHMC228DataCLIReceivesCurrentExternalInputsAndTypedNativeHandoff(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "samples.csv")
	if err := os.WriteFile(external, []byte("name,value\na,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	native, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	if err != nil {
		t.Fatal(err)
	}
	p := &hmc228DataPlanner{plan: dataquery.TaskPlan{Status: "needs_native_reader", InputPaths: []string{native}}}
	request := "Read `" + native + "` alongside `" + external + "`."
	answer, err := RunDataTaskCLI(context.Background(), request, TurnPolicy{Route: RouteData, NeedsDataAccess: true}, DataTaskCLIConfig{Planner: p, RepoRoot: root, RuntimeAnchor: t.TempDir()})
	var handoff interface{ NativeObservationInputs() []string }
	if answer != "" || !errors.As(err, &handoff) {
		t.Fatalf("answer=%q err=%v: want typed native reader delegation, not data consumption", answer, err)
	}
	seen := map[string]dataquery.CandidateFile{}
	for _, c := range p.candidates {
		seen[c.Path] = c
	}
	if c, ok := seen[external]; !ok || c.Kind != "csv" || len(c.SampleRows) != 1 {
		t.Fatalf("external CSV lost: %+v", p.candidates)
	}
	if _, ok := seen[native]; !ok {
		t.Fatalf("named native input lost: %+v", p.candidates)
	}
}
