package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeDispatchAdvisoryForStage_FirstByteTimeoutIsUserFacing(t *testing.T) {
	err := fmt.Errorf("agent explorer execution: all retries exhausted by adapter: upstream LLM produced no usable SSE data within 40.000634747s of the request being accepted: http request before response headers: Post \"http://example.test/chat/completions\": context canceled")

	got := runtimeDispatchAdvisoryForStage(types.StageExplore, err, "zh")
	if got == "" {
		t.Fatal("expected transient dispatch advisory")
	}
	for _, want := range []string{"探索", "模型响应超时", "建议重试"} {
		if !strings.Contains(got, want) {
			t.Fatalf("advisory %q missing %q", got, want)
		}
	}
	for _, leak := range []string{"SSE", "context canceled", "adapter"} {
		if strings.Contains(got, leak) {
			t.Fatalf("advisory leaked implementation detail %q: %s", leak, got)
		}
	}
}

func TestRuntimeDispatchAdvisoryForStage_UserCancelAloneIsNotRetryNotice(t *testing.T) {
	if got := runtimeDispatchAdvisoryForStage(types.StageExplore, context.Canceled, "zh"); got != "" {
		t.Fatalf("plain user cancellation should not be rendered as retry advisory, got %q", got)
	}
}

func TestAppendRuntimeDispatchAdvisoriesToAnswer_DedupsInSeparateSection(t *testing.T) {
	answer := "正文内容"
	advisory := "本次探索阶段遇到上游模型响应超时或连接中断，系统已基于当时已收集到的信息继续生成答案；如果答案缺少证据、链路细节或关键结论，建议重试一次。"

	got := appendRuntimeDispatchAdvisoriesToAnswer(answer, []string{advisory, advisory}, "zh")
	if !strings.HasPrefix(got, answer) {
		t.Fatalf("system advisory must append after model answer, got %q", got)
	}
	if strings.Count(got, advisory) != 1 {
		t.Fatalf("advisory should be deduplicated once, got %q", got)
	}
	if !strings.Contains(got, "## 运行提示") {
		t.Fatalf("expected independent localized run-note section, got %q", got)
	}
}
