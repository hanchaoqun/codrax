package repl

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (r *REPL) installReadRunAutoResumeSeed(effectiveRequest string) (string, func()) {
	if r == nil || r.currentMode != types.ModeRead || r.readRunSnapshotStore == nil {
		return "", nil
	}
	setter, ok := r.runner.(readRunSnapshotAutoSeedSetter)
	if !ok {
		return "", nil
	}
	requestHash := types.ReadRunRequestHash(effectiveRequest)
	if strings.TrimSpace(requestHash) == "" {
		return "", nil
	}
	snapshot, err := r.readRunSnapshotStore.LoadAutoResumeCandidate(requestHash, r.repoRoot)
	if err != nil {
		logging.Warning("[repl/read-runs] auto-resume candidate lookup failed: %v", err)
		return "", nil
	}
	if snapshot == nil {
		return "", nil
	}
	setter.SetReadRunSnapshotAutoSeed(snapshot)
	runID := strings.TrimSpace(snapshot.RunID)
	return runID, func() {
		setter.SetReadRunSnapshotAutoSeed(nil)
	}
}
