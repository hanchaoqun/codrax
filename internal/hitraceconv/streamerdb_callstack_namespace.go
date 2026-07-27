package hitraceconv

// traceDBCallstackSchedulerAlias is a narrowly proven bridge between two
// trace_streamer internal identities that retain the same kernel-visible TID:
// one owns the trace-marker namespace PID, while the other owns the scheduler
// Running intervals and host TGID. It is not a general identity merge.
type traceDBCallstackSchedulerAlias struct {
	ITID       int64
	HeaderTGID int64
	StartCPU   int64
	EndCPU     int64
}

type traceDBCallstackSchedulerAliasStatus uint8

const (
	traceDBCallstackSchedulerAliasUnavailable traceDBCallstackSchedulerAliasStatus = iota
	traceDBCallstackSchedulerAliasResolved
	traceDBCallstackSchedulerAliasAmbiguous
)

func (authority traceDBSchedulerAuthority) resolveCallstackSchedulerAlias(
	running traceDBSchedulerRunningIndex,
	emitter traceDBThread,
	markerProcess traceDBProcess,
	start, end int64,
	endRequired, closedInterval bool,
) (traceDBCallstackSchedulerAlias, traceDBCallstackSchedulerAliasStatus) {
	if !authority.initialized || !authority.complete || emitter.ITID <= 0 || emitter.TID <= 0 ||
		markerProcess.IPID != emitter.IPID || markerProcess.PID <= 0 {
		return traceDBCallstackSchedulerAlias{}, traceDBCallstackSchedulerAliasUnavailable
	}
	var resolved traceDBCallstackSchedulerAlias
	found := false
	for _, candidate := range authority.identities.ByTIDCandidates[emitter.TID] {
		if candidate.ITID == emitter.ITID {
			continue
		}
		thread, process, resolution := authority.resolveThreadSubject(candidate.ITID)
		if resolution != traceDBSchedulerThreadResolved || thread.TID != emitter.TID ||
			process.PID <= 0 || process.PID == markerProcess.PID {
			continue
		}
		if closedInterval {
			if !authority.threadClosedEndpointAllows(thread.ITID, start, end) {
				continue
			}
		} else if !authority.threadPointAllows(thread.ITID, start) {
			continue
		}
		startCPU, startStatus := running.lookupCPUAt(thread.ITID, start)
		if startStatus != traceDBSchedulerRunningKnown {
			continue
		}
		endCPU := startCPU
		if endRequired {
			var endStatus traceDBSchedulerRunningLookupStatus
			endCPU, endStatus = running.lookupCPUAt(thread.ITID, end)
			if endStatus != traceDBSchedulerRunningKnown {
				continue
			}
		}
		if found {
			return traceDBCallstackSchedulerAlias{}, traceDBCallstackSchedulerAliasAmbiguous
		}
		found = true
		resolved = traceDBCallstackSchedulerAlias{
			ITID:       thread.ITID,
			HeaderTGID: process.PID,
			StartCPU:   startCPU,
			EndCPU:     endCPU,
		}
	}
	if !found {
		return traceDBCallstackSchedulerAlias{}, traceDBCallstackSchedulerAliasUnavailable
	}
	return resolved, traceDBCallstackSchedulerAliasResolved
}
