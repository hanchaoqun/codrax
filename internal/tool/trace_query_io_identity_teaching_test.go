package tool

import (
	"strings"
	"testing"
)

// HMC-IO-TEACHING (2026-09-17): replace the stale nearest-time join clause
// with the producer-identity contract already enforced by computeBlockIOByInode.
// This corrects existing teaching in place: no query, JSON schema, dispatch
// view, or runtime prose gate changes. The Description byte golden is updated
// only for this clause; live h2/h3 dispatch weighing remains in the eval batch.
func TestTraceQueryIOIdentityTeachingMatchesExactJoinContract(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"block_io_by_inode for inode-local activity and storage latency with producer-supplied inode/entry identity",
		"never joined by same-thread temporal proximity",
		"missing cross-layer identity leaves separate context, not causal proof",
	} {
		if !strings.Contains(description, want) {
			t.Errorf("IO teaching is missing the existing exact-identity boundary %q", want)
		}
	}
	for _, stale := range []string{"join inode activity with nearest", "nearest block/storage latency"} {
		if strings.Contains(description, stale) {
			t.Errorf("IO teaching contradicts the exact-identity engine: %q", stale)
		}
	}
}
