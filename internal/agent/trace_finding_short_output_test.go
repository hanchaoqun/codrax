package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderTraceFindingShortRootCauseChinese(t *testing.T) {
	finding := &types.TraceFindingV1{PrimaryCause: &types.TraceCauseDecision{
		Status:      types.TraceCausalSupportedCandidate,
		Token:       types.TraceCausalTokenSnapshot{Token: "scheduler_latency"},
		SubjectRole: "target_thread",
		Magnitude:   &types.TypedMagnitude{Value: 12.5, Unit: "ms"},
	}}
	got := renderTraceFindingShortRootCauseValue(finding, "zh-CN")
	for _, want := range []string{"## 简短根因", "最强根因候选", "调度延迟", "目标线程", "12.500 ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("short root cause missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTraceFindingShortRootCauseUnresolvedIsBounded(t *testing.T) {
	finding := &types.TraceFindingV1{Unresolved: &types.TraceUnresolvedDecision{Reason: strings.Repeat("证据不足 ", 100)}}
	got := renderTraceFindingShortRootCauseValue(finding, "zh")
	if !strings.Contains(got, "未确定") || len([]rune(got)) > 190 {
		t.Fatalf("unexpected unresolved short output: %s", got)
	}
}
