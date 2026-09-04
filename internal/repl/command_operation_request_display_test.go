package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// TestCommandOperationE2E_AutoExecuteAnswersAgainstRequestNotDisplay pins
// the request/display split on the operation route's auto-execute arms:
// the answer synthesis model's `## user_request` receives the request
// text the planner planned from — the expanded prompt template, the
// replayed pasted follow-up, or the clarification-combined request —
// while the history record keeps the display form. EVOLUTION RECORD
// (batch six fold-in, F6-prompt-surface #4): operationDispatch called
// executeCommandOperationPlan(plan, display, line) against the
// (plan, request, display) signature since 2c8bd6b6b, so whenever
// line != display the final report was synthesized against
// "/mytemplate args" (or "[pasted N line(s)] …", or only the
// clarification reply) and the memory turn recorded the expanded text as
// the display — red on 381f36cc9 (user_request carried the display form,
// Request carried the expanded text).
func TestCommandOperationE2E_AutoExecuteAnswersAgainstRequestNotDisplay(t *testing.T) {
	const (
		request = "EXPANDED-REQUEST show me the go toolchain version"
		display = "/mytemplate args"
	)
	const cleanPlan = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`
	// A clean low-risk plan executes through the initial-decision
	// auto-execute arm; a ready plan whose program field carries
	// structured JSON fails the deterministic lint and executes through
	// the validate-and-run arm (lint result → one replan → run). Both arms
	// must hand the request/display pair on unswapped.
	for name, responses := range map[string][]llm.Response{
		"initial decision auto-execute": {
			commandOperationPlanResp(cleanPlan),
			{Content: "Go version reported.", StopReason: "end_turn"},
		},
		"lint-failed ready plan validate-and-run": {
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"{\"cmd\":\"go\"}","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(cleanPlan),
			{Content: "Go version reported.", StopReason: "end_turn"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := newPolicyStore(t)
			classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
			adapter := &scriptedChatAdapter{responses: append([]llm.Response(nil), responses...)}
			wantCalls := len(responses)
			r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "/exit\n")
			r.operationEnabled = true
			r.operationPlanner = NewCommandOperationPlanner(adapter)
			r.operationPolicy = operation.DefaultCommandPolicy()

			r.operationDispatch(request, display, commandOperationPolicy("low"))

			if len(runner.requests) != 0 {
				t.Fatalf("auto operation should not enter source pipeline; runner requests=%v", runner.requests)
			}
			if len(adapter.calls) != wantCalls {
				t.Fatalf("planner(+replan)+answer calls=%d, want %d", len(adapter.calls), wantCalls)
			}
			for i := 0; i < wantCalls-1; i++ {
				plannerUser := lastUserRoleContent(adapter.calls[i].messages)
				if !strings.Contains(plannerUser, request) || strings.Contains(plannerUser, display) {
					t.Fatalf("planner call %d must plan from the request text, not the display form:\n%s", i+1, plannerUser)
				}
			}
			answerUser := lastUserRoleContent(adapter.calls[wantCalls-1].messages)
			if got := userRequestSection(answerUser); got != request {
				t.Fatalf("answer synthesis ## user_request = %q, want the request text %q (the display form must never reach the model)\n%s", got, request, answerUser)
			}
			recent := store.Recent()
			if len(recent) == 0 {
				t.Fatal("expected the operation turn in memory")
			}
			last := recent[len(recent)-1]
			if last.Request != display {
				t.Fatalf("memory turn Request = %q, want the display form %q", last.Request, display)
			}
			if last.RequestForSummary != request {
				t.Fatalf("memory turn RequestForSummary = %q, want the request text %q", last.RequestForSummary, request)
			}
		})
	}
}

// userRequestSection returns the body of the `## user_request` section of
// an answer-synthesis user message ("" when the section is absent).
func userRequestSection(content string) string {
	const marker = "## user_request\n"
	at := strings.Index(content, marker)
	if at < 0 {
		return ""
	}
	section := content[at+len(marker):]
	if end := strings.Index(section, "\n\n"); end >= 0 {
		section = section[:end]
	}
	return strings.TrimSpace(section)
}

func lastUserRoleContent(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}
