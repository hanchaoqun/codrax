package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractPredicateAxis(t *testing.T) {
	cases := []struct {
		name    string
		request string
		want    types.PredicateAxis
	}{
		// AxisCall — the motivating case for session-12 Layer 1.
		{"zh_call_verb", "explorer是如何调用subagent的？", types.AxisCall},
		{"en_call_verb", "how does the explorer call the subagent", types.AxisCall},
		{"en_invoke_verb", "how is the handler invoked", types.AxisCall},
		{"en_dispatch_verb", "how does the orchestrator dispatch stages", types.AxisCall},
		{"zh_dispatch_verb", "请问 orchestrator 怎么分发到 agent", types.AxisCall},

		// AxisRegister
		{"zh_register", "SubAgent 是在哪里注册的", types.AxisRegister},
		{"en_register", "where are subagents registered", types.AxisRegister},
		{"zh_bind", "工具是怎么绑定到 agent 的", types.AxisRegister},

		// AxisDefine — broader definitional question
		{"en_define", "what is the AnalysisIR struct", types.AxisDefine},
		{"zh_define", "AnalysisIR 的定义在哪", types.AxisDefine},

		// AxisReturn
		{"en_return", "what does Foo.Name return", types.AxisReturn},
		{"zh_return", "该函数返回什么值", types.AxisReturn},

		// AxisCondition
		{"en_condition", "when does the hypothesis get rejected", types.AxisCondition},
		{"zh_condition", "什么时候会触发重试", types.AxisCondition},

		// AxisConfigure
		{"en_configure", "where is log-level configured", types.AxisConfigure},

		// AxisImplement
		{"en_implement", "how is the SubAgent interface implemented", types.AxisImplement},

		// Priority tie-breaker: AxisCall > AxisRegister
		{"priority_call_over_register", "how does explorer call subagents and register them",
			types.AxisCall},
		// Priority tie-breaker: AxisRegister > AxisImplement
		{"priority_register_over_implement", "where is the handler registered and implemented",
			types.AxisRegister},

		// No verb cue → Unknown
		{"no_verb", "subagent 和 explorer", types.AxisUnknown},
		{"empty", "", types.AxisUnknown},
		{"whitespace_only", "   \n\t  ", types.AxisUnknown},

		// Anti-false-positive: "list" / count are intent cues, not axis cues
		{"count_not_axis", "how many skills exist", types.AxisUnknown},
		{"list_not_axis", "list all agents", types.AxisUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPredicateAxis(tc.request)
			if got != tc.want {
				t.Errorf("extractPredicateAxis(%q) = %q, want %q", tc.request, got, tc.want)
			}
		})
	}
}

func TestReconcilePredicateAxis(t *testing.T) {
	// LLM-emitted value is preserved (non-override discipline).
	if got, reason := reconcilePredicateAxis(types.AxisReturn, "how does X call Y"); got != types.AxisReturn || reason != "" {
		t.Errorf("declared AxisReturn preserved: got (%q, %q), want (AxisReturn, '')", got, reason)
	}
	// Empty declared + verb cue present → filled.
	if got, reason := reconcilePredicateAxis(types.AxisUnknown, "how does X call Y"); got != types.AxisCall {
		t.Errorf("empty declared + call verb: got %q, want AxisCall (reason=%q)", got, reason)
	}
	if got, _ := reconcilePredicateAxis(types.AxisUnknown, "explorer 如何调用 subagent"); got != types.AxisCall {
		t.Errorf("empty declared + zh call verb: got %q, want AxisCall", got)
	}
	// Empty declared + no cue → stays empty.
	if got, reason := reconcilePredicateAxis(types.AxisUnknown, "foo bar baz"); got != types.AxisUnknown || reason != "" {
		t.Errorf("no cue: got (%q, %q), want (AxisUnknown, '')", got, reason)
	}
}

func TestPredicateAxisEnumValidity(t *testing.T) {
	axes := types.AllPredicateAxes()
	if len(axes) == 0 {
		t.Fatal("AllPredicateAxes returned empty slice")
	}
	for _, a := range axes {
		if !a.IsValid() {
			t.Errorf("declared axis %q failed IsValid", a)
		}
	}
	if !types.AxisUnknown.IsValid() {
		t.Error("AxisUnknown should be valid (explicit zero sentinel)")
	}
	if types.PredicateAxis("bogus").IsValid() {
		t.Error("bogus axis should not be valid")
	}
}
