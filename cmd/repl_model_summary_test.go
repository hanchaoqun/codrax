package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
)

type replModelSummaryAdapter struct {
	streaming bool
	firstByte time.Duration
}

func (*replModelSummaryAdapter) Chat(context.Context, []llm.Message, []llm.ToolSchema, llm.ChatOptions) (llm.Response, error) {
	return llm.Response{}, nil
}
func (*replModelSummaryAdapter) ModelID() string               { return "model-x" }
func (*replModelSummaryAdapter) MaxContextTokens() int         { return 200000 }
func (*replModelSummaryAdapter) MaxOutputTokens() int          { return 0 }
func (*replModelSummaryAdapter) RequestTimeout() time.Duration { return 4 * time.Minute }
func (*replModelSummaryAdapter) RetryMaxAttempts() int         { return 6 }
func (a *replModelSummaryAdapter) StreamingLivenessWatchdogEnabled() bool {
	return a != nil && a.streaming
}
func (a *replModelSummaryAdapter) StreamFirstByteTimeout() time.Duration {
	if a == nil {
		return 0
	}
	return a.firstByte
}

func TestReplModelSummaryLineStreamingDoesNotAdvertiseRequestTimeoutAsTotalCap(t *testing.T) {
	adapter := &replModelSummaryAdapter{streaming: true, firstByte: 3 * time.Minute}
	zh := replModelSummaryLine("zh", adapter, false)
	for _, want := range []string{"流式等待=首包静默上限3m0s", "活跃流继续"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("streaming zh summary lost %q: %q", want, zh)
		}
	}
	if strings.Contains(zh, "超时=4m0s") {
		t.Fatalf("streaming summary must not present requestTimeout as an absolute total cap: %q", zh)
	}

	en := replModelSummaryLine("en", adapter, false)
	for _, want := range []string{"stream wait=first-byte silence limit 3m0s", "active streams continue"} {
		if !strings.Contains(en, want) {
			t.Fatalf("streaming en summary lost %q: %q", want, en)
		}
	}
}

func TestReplModelSummaryLineNonStreamingRetainsRequestTimeout(t *testing.T) {
	adapter := &replModelSummaryAdapter{streaming: false, firstByte: 3 * time.Minute}
	zh := replModelSummaryLine("zh", adapter, false)
	if !strings.Contains(zh, "超时=4m0s") || strings.Contains(zh, "流式等待=") {
		t.Fatalf("non-streaming zh summary must retain its actual request timeout: %q", zh)
	}
	en := replModelSummaryLine("en", adapter, false)
	if !strings.Contains(en, "timeout=4m0s") || strings.Contains(en, "stream wait=") {
		t.Fatalf("non-streaming en summary must retain its actual request timeout: %q", en)
	}
}
