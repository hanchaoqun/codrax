package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExplorerDurableProgressContinuationRecognizesFactRetryCheckpoint(t *testing.T) {
	ctx := &types.AgentContext{
		RetryHint:     "Explorer continuation checkpoint:\nContinue from preserved runtime line windows.",
		RetryHintKind: types.RetryHintKindDurableProgressContinuation,
	}
	if !explorerDurableProgressContinuationActive(ctx) {
		t.Fatal("explorer should treat fact-retry checkpoints as durable continuation")
	}
}

func TestExplorerDurableProgressContinuationIgnoresHintProseWithoutKind(t *testing.T) {
	ctx := &types.AgentContext{
		RetryHint: "A transient model stream error interrupted the previous explore dispatch after durable progress was preserved.",
	}
	if explorerDurableProgressContinuationActive(ctx) {
		t.Fatal("explorer must not infer durable continuation from retry hint prose")
	}
}
