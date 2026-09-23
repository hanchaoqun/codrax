package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests prove teaching delivery and typed dispatch, not model accuracy:
// the production classifier constructs the request, while the adapter returns
// a declared policy. No live model or keyword-based routing is involved.
func TestToolDocumentationRoutingPublicPrompt(t *testing.T) {
	cases := []struct {
		name, request, source, sourceMode string
		attached                          bool
	}{
		{"host_zh", "查看当前程序公开的工具说明，解释单位和限制，不读取仓库源码。", "current_message", "optional", false},
		{"host_en", "Read the host's published tool documentation and explain units and limits without reading source.", "current_message", "optional", false},
		{"mixed_trace_zh", "先解释公开的 IO 工具说明，再查询附加 trace 的指定窗口；说明不能代替观测，不分析源码。", "mixed", "optional", true},
		{"mixed_trace_en", "Explain the published tool documentation, then investigate the attached trace window. Keep documentation separate from measurements; do not read source.", "mixed", "optional", true},
		{"mixed_source", "Explain the published tool documentation and verify how the current repository implementation uses it.", "mixed", "required", false},
	}
	for _, lane := range []string{"repl", "single_shot"} {
		for _, tc := range cases {
			t.Run(lane+"/"+tc.name, func(t *testing.T) {
				adapter := &scriptedChatAdapter{responses: []llm.Response{documentationRoutingPolicyResponse(t, tc.source, tc.sourceMode)}}
				classifier := NewChitchatClassifier(adapter)
				hint := ""
				if tc.attached {
					hint = "attachment=true"
				}
				var policy TurnPolicy
				var err error
				if lane == "single_shot" {
					policy, err = classifier.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), tc.request, hint, false)
				} else {
					policy, err = classifier.(TurnPolicyClassifier).ClassifyPolicy(context.Background(), tc.request, hint, false)
				}
				if err != nil || len(adapter.calls) != 1 {
					t.Fatalf("actual classifier first request: calls=%d err=%v", len(adapter.calls), err)
				}
				call := adapter.calls[0]
				if !reflect.DeepEqual(call.tools, []llm.ToolSchema{turnPolicyTool}) {
					t.Fatal("classifier must use the unchanged published policy schema")
				}
				user := lastUserMessage(call.messages)
				if !strings.HasSuffix(user, "## current: "+tc.request) || strings.Contains(user, "attachment=true") != tc.attached {
					t.Fatalf("current request or attachment hint changed: %q", user)
				}
				guarded := ApplyTurnPolicyGuards(policy, false, tc.attached)
				if guarded.Route != RouteRepo || !guarded.NeedsRepoAccess || guarded.NeedsOperationAccess || guarded.NeedsDataAccess || guarded.Operation != "investigate" || string(guarded.CurrentSourceEvidenceMode) != tc.sourceMode {
					t.Fatalf("typed pipeline/source boundary changed: %+v", guarded)
				}
				system := call.messages[0].Content
				for _, want := range []string{
					"host's published tool documentation",
					"route the whole request through this pipeline",
					"documentation does not replace observations",
					"source, external-observation, or published-tool-documentation inquiry",
					"Repository availability is capability, not evidence authority",
					"direct computer/file operation = operation",
					"Explicit command-operation file reads/searches/extractions",
					"Treat attachment=true as a SOFT signal",
					"explicit_change — the current turn explicitly asks Codrax to modify",
				} {
					if !strings.Contains(system, want) {
						t.Errorf("actual classifier system message missing %q", want)
					}
				}
				if strings.Contains(system, "investigate — fresh code investigation that needs repo reads") {
					t.Error("actual classifier still receives the contradictory code-only investigate definition")
				}
			})
		}
	}
}

func documentationRoutingPolicyResponse(t *testing.T, source, sourceMode string) llm.Response {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"route": "repo", "operation": "investigate", "write_intent": "analysis_only",
		"needs_repo_access": true, "needs_operation_access": false,
		"source": source, "current_source_evidence_mode": sourceMode,
		"confidence": 0.9, "reason": "published documentation and requested investigation",
		"requires_diagram": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return turnPolicyResp(string(params))
}

func TestToolDocumentationRoutingPublicPipelineDispatch(t *testing.T) {
	for _, attached := range []bool{false, true} {
		name, source := "documentation_only", "current_message"
		if attached {
			name, source = "documentation_and_trace", "mixed"
		}
		t.Run(name, func(t *testing.T) {
			request := "Read published tool documentation; investigate the attached trace only when supplied. Do not read source."
			adapter := &scriptedChatAdapter{responses: []llm.Response{documentationRoutingPolicyResponse(t, source, "optional")}}
			classifier := NewChitchatClassifier(adapter)
			responder := &stubLocalResponder{}
			r, runner, _ := newTurnPolicyREPL(t, newPolicyStore(t), classifier, responder, request+"\n/exit\n")
			if attached {
				r.attachedHitrace = "# tracer: nop\n"
			}
			operationAdapter := &scriptedChatAdapter{}
			r.operationEnabled = true
			r.operationPlanner = NewCommandOperationPlanner(operationAdapter)
			if err := r.Loop(); err != nil {
				t.Fatal(err)
			}
			if len(adapter.calls) != 1 || len(runner.requests) != 1 || !strings.Contains(runner.requests[0], request) {
				t.Fatalf("actual classifier/dispatcher must preserve the whole request: calls=%d requests=%v", len(adapter.calls), runner.requests)
			}
			if len(runner.seenRouteHints) != 1 || runner.seenRouteHints[0].RequiresCurrentSourceEvidence() || len(responder.localCalls) != 0 || len(operationAdapter.calls) != 0 {
				t.Fatalf("documentation must not mint source or operation obligations: %+v", runner.seenRouteHints)
			}
		})
	}
}

func TestToolDocumentationRoutingPublicActualOperationPreserved(t *testing.T) {
	request := "Run pwd to inspect the current directory; the attached trace is unrelated."
	adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"route":"operation","needs_repo_access":false,"current_source_evidence_mode":"optional","needs_operation_access":true,"operation":"computer_operation","operation_kind":"computer_operation","write_intent":"analysis_only","source":"current_message","confidence":0.9,"reason":"explicit machine inspection","risk_level":"low","side_effects":[],"target_surface":"desktop","requires_diagram":false}`)}}
	classifier := NewChitchatClassifier(adapter).(SingleShotTurnPolicyClassifier)
	raw, err := classifier.ClassifyPolicySingleShot(context.Background(), request, "attachment=true", false)
	if err != nil {
		t.Fatal(err)
	}
	guarded := ApplyTurnPolicyGuards(raw, false, true)
	if guarded.Route != RouteOperation || !IsConcreteOperationPolicy(guarded) || guarded.NeedsRepoAccess || guarded.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceOptional {
		t.Fatalf("unrelated attachment must not force a real operation into analysis: %+v", guarded)
	}
	workDir := t.TempDir()
	commandPolicy := operation.DefaultCommandPolicy()
	commandPolicy.DefaultWorkDir = workDir
	planner := fakeCLICommandPlanner{req: operation.CommandOperationRequest{
		Text: request, Goal: "inspect current directory", WorkDir: workDir, RiskLevel: "low",
		Steps: []operation.CommandStep{{ID: "pwd", Title: "inspect directory", Program: "/bin/pwd", RiskLevel: "low"}},
	}}
	var progress bytes.Buffer
	answer, err := RunCommandOperationCLI(context.Background(), request, guarded, CommandOperationCLIConfig{
		Planner: planner, Policy: commandPolicy, RepoRoot: workDir, RuntimeAnchor: t.TempDir(), Language: "en", Progress: &progress,
	})
	if err != nil || !strings.Contains(answer, workDir) {
		t.Fatalf("actual native operation did not survive routing: answer=%q err=%v progress=%s", answer, err, progress.String())
	}
}
