package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The pre-stage declaration order is a contract: log_triage before
// perf_triage (mixed-artifact runs rely on the log bundle being
// observable by the perf lane and every later consumer).
func TestPreStageOrder_LogBeforePerf(t *testing.T) {
	if len(preStages) < 2 {
		t.Fatalf("expected at least log+perf pre-stages, got %d", len(preStages))
	}
	if preStages[0].Stage != types.StageLogTriage {
		t.Fatalf("first pre-stage must be log_triage, got %s", preStages[0].Stage)
	}
	if preStages[1].Stage != types.StagePerfTriage {
		t.Fatalf("second pre-stage must be perf_triage, got %s", preStages[1].Stage)
	}
}
