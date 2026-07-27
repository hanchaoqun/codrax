package cmd

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/orchestrator"
)

// wfid_lane_test.go — WFID-LANE (§15.12 批丙): the PRODUCTION store wired by
// newRuntimeStores must take the identity-aware finder lane. The compile-time
// assertion in root.go makes interface drift a build error; this pin walks
// the actual wiring value so a future wiring change (a different store type,
// a wrapper losing the method set) goes red too.
func TestRuntimeStoresWriteWorkflowLaneIsIdentityAware(t *testing.T) {
	stores := newRuntimeStores(t.TempDir())
	if stores.WriteWorkflowRunStore == nil {
		t.Fatal("production write workflow store must be wired")
	}
	if _, ok := interface{}(stores.WriteWorkflowRunStore).(orchestrator.WriteWorkflowRunIdentityMatchedLoader); !ok {
		t.Fatalf("production store %T must implement the identity-aware finder lane — the legacy single-result lane fail-closes on cross-repo candidates", stores.WriteWorkflowRunStore)
	}
}
