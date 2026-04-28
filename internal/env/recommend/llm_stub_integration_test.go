package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubAdapter is a minimal llm.Adapter that records every Chat
// invocation and returns a canned emit_env_recommendation tool
// call. Used by the LLM-stage integration tests so we exercise
// the full Stage 3 path without a real provider.
type stubAdapter struct {
	calls    atomic.Int32
	response llm.Response
	chatErr  error
	delay    time.Duration
}

func (s *stubAdapter) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}
	if s.chatErr != nil {
		return llm.Response{}, s.chatErr
	}
	return s.response, nil
}
func (*stubAdapter) ModelID() string               { return "stub" }
func (*stubAdapter) MaxContextTokens() int         { return 8000 }
func (*stubAdapter) MaxOutputTokens() int          { return 0 }
func (*stubAdapter) RequestTimeout() time.Duration { return 30 * time.Second }
func (*stubAdapter) RetryMaxAttempts() int         { return 1 }

// stubResponse builds a canned emit_env_recommendation Response
// for the stubAdapter to return.
func stubResponse(strategy, command, why string) llm.Response {
	params, _ := json.Marshal(map[string]any{
		"recommendations": []map[string]any{
			{
				"strategy":      strategy,
				"command":       command,
				"why":           why,
				"needs_network": true,
				"estimated_sec": 30,
			},
		},
	})
	return llm.Response{
		ToolCalls: []llm.ToolCall{
			{Name: "emit_env_recommendation", Params: json.RawMessage(params)},
		},
	}
}

// resetEnvForLLMTest stubs $HOME so the disk cache writes to a
// temp dir, resets the cache singleton, and clears the LLM
// adapter so test ordering doesn't leak.
func resetEnvForLLMTest(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheOnce = sync.Once{}
	cacheRef = nil
	SetLLMAdapter(nil)
	ResetMetrics()
}

// TestLLMStub_StageThreeWritesToCache covers the canonical
// integration path: first Recommend triggers Stage 3 (LLM stub),
// records the result, and writes through to disk cache; the
// second Recommend hits Stage 2 without invoking the adapter.
func TestLLMStub_StageThreeWritesToCache(t *testing.T) {
	resetEnvForLLMTest(t)
	defer SetLLMAdapter(nil)

	stub := &stubAdapter{
		response: stubResponse("user_install", "!pip install --user pytest", "stub recommendation"),
	}
	SetLLMAdapter(stub)

	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = true
	settings.LLMTimeoutSec = 5

	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	envFacts := &types.EnvFacts{OSFamily: "debian"}

	// First call → Stage 3 LLM.
	recs1 := Recommend(d, envFacts, settings)
	if len(recs1) == 0 {
		t.Fatal("first Recommend should produce stub recommendation")
	}
	if recs1[0].Strategy != types.StrategyUserInstall {
		t.Errorf("rec[0] strategy: want UserInstall, got %v", recs1[0].Strategy)
	}
	if stub.calls.Load() != 1 {
		t.Errorf("stub Chat calls after first Recommend: want 1, got %d", stub.calls.Load())
	}
	m1 := Snapshot()
	if m1.LLMSuccess != 1 {
		t.Errorf("LLMSuccess after first Recommend: want 1, got %d", m1.LLMSuccess)
	}

	// Second call → should hit Stage 2 cache, no further Chat.
	recs2 := Recommend(d, envFacts, settings)
	if len(recs2) == 0 {
		t.Fatal("second Recommend should hit cache")
	}
	if stub.calls.Load() != 1 {
		t.Errorf("cache should prevent second Chat call; got count=%d", stub.calls.Load())
	}
	m2 := Snapshot()
	if m2.CacheHits == m1.CacheHits {
		t.Errorf("CacheHits did not increment: pre=%d post=%d", m1.CacheHits, m2.CacheHits)
	}
}

// TestLLMStub_TimeoutFallsThroughToDocsLink verifies the timeout
// path: stub Chat takes longer than LLMTimeoutSec; metrics record
// the timeout and Stage 4 DocsLink fallback fires.
func TestLLMStub_TimeoutFallsThroughToDocsLink(t *testing.T) {
	resetEnvForLLMTest(t)
	defer SetLLMAdapter(nil)

	stub := &stubAdapter{
		response: stubResponse("user_install", "!pip install foo", "stub"),
		delay:    1500 * time.Millisecond,
	}
	SetLLMAdapter(stub)

	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = true
	settings.LLMTimeoutSec = 1 // expire well before the stub's 1500ms delay

	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	envFacts := &types.EnvFacts{OSFamily: "debian"}
	recs := Recommend(d, envFacts, settings)

	if len(recs) == 0 {
		t.Fatal("LLM timeout should still produce DocsLink fallback")
	}
	if recs[0].Strategy != types.StrategyDocsLink {
		t.Errorf("on timeout, top-1 should be DocsLink; got %v: %q", recs[0].Strategy, recs[0].Command)
	}
	m := Snapshot()
	if m.LLMTimeouts != 1 {
		t.Errorf("LLMTimeouts: want 1, got %d", m.LLMTimeouts)
	}
	if m.DocsLinkFallbacks != 1 {
		t.Errorf("DocsLinkFallbacks: want 1, got %d", m.DocsLinkFallbacks)
	}
}

// TestLLMStub_AdapterErrorIncrementsErrors verifies a Chat error
// (non-timeout) bumps LLMErrors and falls through to DocsLink.
func TestLLMStub_AdapterErrorIncrementsErrors(t *testing.T) {
	resetEnvForLLMTest(t)
	defer SetLLMAdapter(nil)

	stub := &stubAdapter{chatErr: errors.New("upstream 500")}
	SetLLMAdapter(stub)

	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = true
	settings.LLMTimeoutSec = 5

	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "rust"}
	envFacts := &types.EnvFacts{OSFamily: "darwin"}
	recs := Recommend(d, envFacts, settings)

	if len(recs) == 0 {
		t.Fatal("adapter error should still produce DocsLink fallback")
	}
	m := Snapshot()
	if m.LLMErrors != 1 {
		t.Errorf("LLMErrors: want 1, got %d", m.LLMErrors)
	}
	if m.LLMSuccess != 0 {
		t.Errorf("LLMSuccess should be 0 on error path; got %d", m.LLMSuccess)
	}
}
