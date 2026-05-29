package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExplorerDurableProgressContinuationRecognizesFactRetryCheckpoint(t *testing.T) {
	ctx := &types.AgentContext{
		RetryHint: "Explorer continuation checkpoint:\nContinue from preserved runtime line windows.",
	}
	if !explorerDurableProgressContinuationActive(ctx) {
		t.Fatal("explorer should treat fact-retry checkpoints as durable continuation")
	}
}
