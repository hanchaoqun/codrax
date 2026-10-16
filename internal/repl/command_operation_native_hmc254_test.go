package repl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc254OperationAnalysisPolicy() TurnPolicy {
	return TurnPolicy{Route: RouteOperation, Operation: "investigate", Source: "artifact", NeedsRepoAccess: true, NeedsOperationAccess: true,
		WriteIntent: WriteIntentAnalysisOnly, RiskLevel: "low", SideEffects: []string{"local_file_write"}, TargetSurface: "file_artifact", Confidence: .95,
		RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeExternalArtifact, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
}

func TestHMC254InitialNativeReaderPublicBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observation.data")
	if err := os.WriteFile(path, []byte("unknown data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		edit    func(map[string]any, *TurnPolicy)
		request string
		cancel  bool
		good    bool
	}{
		{name: "exact", good: true},
		{name: "required_source", good: true, edit: func(_ map[string]any, p *TurnPolicy) {
			p.RequiredOutcomes |= types.TurnOutcomeSourceExplanation
			p.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceRequired
			p.Source = "mixed"
		}},
		{name: "wrong_path", edit: func(d map[string]any, _ *TurnPolicy) {
			d["input_paths"] = []string{filepath.Join(t.TempDir(), "observation.data")}
		}},
		{name: "directory", edit: func(d map[string]any, _ *TurnPolicy) { d["input_paths"] = []string{filepath.Dir(path)} }},
		{name: "old_request", request: "What remains?"},
		{name: "cancel", cancel: true},
		{name: "command", edit: func(d map[string]any, _ *TurnPolicy) {
			d["steps"] = []any{map[string]any{"program": "echo", "args": []string{"unexpected"}}}
		}},
		{name: "empty_step", edit: func(d map[string]any, _ *TurnPolicy) { d["steps"] = []any{map[string]any{}} }},
		{name: "continue", edit: func(d map[string]any, _ *TurnPolicy) { d["continue_after"] = true }},
		{name: "risk", edit: func(d map[string]any, _ *TurnPolicy) { d["risk_level"] = "low" }},
		{name: "questions", edit: func(d map[string]any, _ *TurnPolicy) { d["questions"] = []any{map[string]any{"question": "confirm?"}} }},
		{name: "approval_draft", edit: func(d map[string]any, _ *TurnPolicy) { d["requires_confirmation"] = true }},
		{name: "approval_policy", edit: func(_ map[string]any, p *TurnPolicy) { p.RequiresConfirmation = true }},
		{name: "independent", edit: func(_ map[string]any, p *TurnPolicy) { p.DeliveryOwner = independentOperationDeliveryOwner }},
		{name: "unknown_owner", edit: func(_ map[string]any, p *TurnPolicy) { p.DeliveryOwner = "unknown" }},
		{name: "computer_action", edit: func(_ map[string]any, p *TurnPolicy) { p.RequiredOutcomes |= types.TurnOutcomeComputerAction }},
		{name: "source_change", edit: func(_ map[string]any, p *TurnPolicy) { p.RequiredOutcomes |= types.TurnOutcomeSourceChange }},
		{name: "write", edit: func(_ map[string]any, p *TurnPolicy) { p.WriteIntent = WriteIntentExplicitChange }},
		{name: "overwrite", edit: func(_ map[string]any, p *TurnPolicy) { p.SideEffects = []string{"local_file_overwrite"} }},
		{name: "browser", edit: func(_ map[string]any, p *TurnPolicy) { p.TargetSurface = "browser_ui" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := hmc254OperationAnalysisPolicy()
			policy.PresentationDirective = "facts table"
			policy.RequiresDiagram = true
			d := map[string]any{"status": "needs_native_reader", "risk_level": "none", "requires_confirmation": false, "input_paths": []string{path}, "steps": []any{}}
			if tc.edit != nil {
				tc.edit(d, &policy)
			}
			raw, _ := json.Marshal(d)
			a := &scriptedChatAdapter{responses: []llm.Response{commandOperationPlanResp(string(raw)), commandOperationPlanResp(string(raw))}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			request := tc.request
			if request == "" {
				request = "Read `" + path + "`."
			}
			answer, err := RunCommandOperationCLI(ctx, request, policy, CommandOperationCLIConfig{Planner: NewCommandOperationPlanner(a), RepoRoot: t.TempDir()})
			got, ok := NativeOperationHandoffPolicy(policy, err)
			if ok != tc.good || answer != "" || err == nil {
				t.Fatalf("delegated=%t answer=%q err=%v", ok, answer, err)
			}
			if ok && (got.Route != RouteRepo || got.NeedsOperationAccess || len(got.SideEffects) > 0 || got.CurrentSourceEvidenceMode != policy.CurrentSourceEvidenceMode || got.RequiredOutcomes != policy.RequiredOutcomes || got.Source != policy.Source || got.PresentationDirective != policy.PresentationDirective || !got.RequiresDiagram) {
				t.Fatalf("handoff changed independent obligations: %+v", got)
			}
		})
	}
}

func TestHMC254NativeReaderRejectsReplanContinuationAndMixedDecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("opaque"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"status": "needs_native_reader", "risk_level": "none", "requires_confirmation": false, "input_paths": []string{path}, "steps": []any{}})
	for _, lane := range []string{"replan", "continuation", "records", "mixed_decision"} {
		t.Run(lane, func(t *testing.T) {
			resp := commandOperationPlanResp(string(raw))
			if lane == "mixed_decision" {
				resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{Name: "exec_command", Params: []byte(`{"command":"echo unexpected"}`)})
			}
			a := &scriptedChatAdapter{responses: []llm.Response{resp, resp}}
			p := NewCommandOperationPlanner(a).(*llmCommandOperationPlanner)
			req := commandOperationPlannerRequest{UserLine: "Read `" + path + "`.", RepoRoot: t.TempDir(), Policy: hmc254OperationAnalysisPolicy()}
			switch lane {
			case "replan":
				req.Replan = true
			case "continuation":
				req.Continuation = true
			case "records":
				req.Records = []commandOperationResultRecord{{Result: operation.CommandOperationResult{PlanID: "already-ran"}}}
			}
			_, err := p.planCommandOperation(context.Background(), req)
			var handoff *nativeCommandHandoff
			if err == nil || errors.As(err, &handoff) {
				t.Fatalf("executed/mixed request delegated: %v", err)
			}
		})
	}
}

func TestHMC254REPLInitialOperationDelegatesUnchangedWithoutRecordingExecution(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	if err != nil {
		t.Fatal(err)
	}
	request := "Read `" + path + "` from 1 to 2 seconds and retain the evidence package."
	policy := hmc254OperationAnalysisPolicy()
	classifier := &stubTurnPolicyClassifier{policy: policy}
	r, runner, _ := newTurnPolicyREPL(t, newPolicyStore(t), classifier, &stubResponder{}, request+"\n/exit\n")
	raw, _ := json.Marshal(map[string]any{"status": "needs_native_reader", "risk_level": "none", "requires_confirmation": false, "input_paths": []string{path}, "steps": []any{}})
	a := &scriptedChatAdapter{responses: []llm.Response{commandOperationPlanResp(string(raw))}}
	r.operationPlanner = NewCommandOperationPlanner(a)
	r.operationEnabled = true
	r.repoRoot = t.TempDir()
	r.runtimeAnchor = t.TempDir()
	if err := r.Loop(); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 1 || len(runner.requests) != 1 || !strings.Contains(runner.requests[0], request) || len(r.operationResults) != 0 || len(r.operationHistory) != 0 {
		t.Fatalf("calls=%d runs=%v results=%d history=%d", len(a.calls), runner.requests, len(r.operationResults), len(r.operationHistory))
	}
	if runner.seenModes[0] != types.ModeRead || runner.seenRouteHints[0].Route != "repo" || runner.seenRouteHints[0].NeedsOperationAccess || r.attachedHitrace != "" || r.attachedLog != "" {
		t.Fatalf("not isolated read navigation: %+v", runner.seenRouteHints)
	}
}

func TestHMC254InitialOperationCanRequestNativeReaderBeforeExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observations.input")
	if err := os.WriteFile(path, []byte("opaque native input"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"status": "needs_native_reader", "risk_level": "none", "requires_confirmation": false, "input_paths": []string{path}, "steps": []any{}})
	a := &scriptedChatAdapter{responses: []llm.Response{commandOperationPlanResp(string(raw))}}
	answer, err := RunCommandOperationCLI(context.Background(), "检查 `"+path+"` 并保存本次分析的证据包", hmc254OperationAnalysisPolicy(), CommandOperationCLIConfig{Planner: NewCommandOperationPlanner(a), RepoRoot: t.TempDir()})
	var handoff interface{ NativeObservationInputs() []string }
	if answer != "" || !errors.As(err, &handoff) {
		t.Fatalf("expected typed navigation before command execution, answer=%q error=%v", answer, err)
	}
	if len(a.calls) != 1 || len(handoff.NativeObservationInputs()) != 1 || handoff.NativeObservationInputs()[0] != path {
		t.Fatalf("handoff changed inputs or planned again: calls=%d inputs=%v", len(a.calls), handoff.NativeObservationInputs())
	}
}
