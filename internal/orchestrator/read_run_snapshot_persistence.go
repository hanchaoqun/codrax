package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ReadRunSnapshotSaver is the orchestrator-side persistence interface for
// read-mode typed execution snapshots. The concrete store lives in internal/repl
// so orchestrator does not import the REPL package.
type ReadRunSnapshotSaver interface {
	Save(snapshot *types.ReadRunSnapshot) (string, error)
}

// SetReadRunSnapshotStore installs the read snapshot persister. Nil disables
// snapshot persistence without affecting read-mode execution.
func (o *Orchestrator) SetReadRunSnapshotStore(s ReadRunSnapshotSaver) {
	o.readRunSnapshotSaver = s
}

func (o *Orchestrator) persistReadRunSnapshot() {
	if o == nil || o.readRunSnapshotSaver == nil || o.busCtx == nil || o.busCtx.Mode != types.ModeRead {
		return
	}
	runID := strings.TrimSpace(o.busCtx.TraceID)
	if runID == "" {
		return
	}
	snapshot := types.ReadRunSnapshotFromBusContext(o.busCtx, runID)
	path, err := o.readRunSnapshotSaver.Save(&snapshot)
	if err != nil {
		logging.Warning("[orchestrator] read run snapshot save failed: %v", err)
		return
	}
	if strings.TrimSpace(path) != "" {
		logging.Info("[orchestrator] read run snapshot saved: %s", path)
	}
}
