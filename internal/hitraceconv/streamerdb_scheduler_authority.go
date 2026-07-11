package hitraceconv

import "fmt"

// traceDBSchedulerAuthority binds scheduler consumers to one exact identity
// index and one lifecycle collection. Its zero value rejects every query; a
// consumer cannot accidentally turn a missing or mismatched authority into a
// no-cut (therefore allow-all) lifecycle.
type traceDBSchedulerAuthority struct {
	identities  traceDBThreadIndex
	lifecycle   traceDBLifecycleIndex
	initialized bool
	complete    bool
}

// traceDBSchedulerSubject is proof that a scheduler identity came from a
// present scalar which passed the strict canonical ITID projection. In
// particular, a missing/NULL scalar which merely defaulted to zero must never
// acquire the idle bypass.
type traceDBSchedulerSubject struct {
	itid  int64
	exact bool
}

type traceDBSchedulerThreadResolution uint8

const (
	traceDBSchedulerThreadInvalid traceDBSchedulerThreadResolution = iota
	traceDBSchedulerThreadMissing
	traceDBSchedulerProcessMissing
	traceDBSchedulerThreadAmbiguous
	traceDBSchedulerThreadResolved
)

func (authority traceDBSchedulerAuthority) schedulerSubjectFromExactITID(itid int64, exact bool) (traceDBSchedulerSubject, bool) {
	subject := traceDBSchedulerSubject{itid: itid, exact: exact}
	if !authority.schedulerSubjectIsExact(subject) {
		return traceDBSchedulerSubject{}, false
	}
	return subject, true
}

func (authority traceDBSchedulerAuthority) schedulerSubjectIsExact(subject traceDBSchedulerSubject) bool {
	if !authority.initialized || !subject.exact || subject.itid < 0 || subject.itid > maxTraceDBInternalID {
		return false
	}
	if subject.itid != 0 {
		return true
	}
	if authority.identities.AmbiguousITID[0] || authority.identities.AmbiguousIPID[0] {
		return false
	}
	if materialized, ok := authority.identities.ByITID[0]; ok &&
		(materialized.ITID != 0 || materialized.TID != 0 || materialized.IPID != 0) {
		return false
	}
	if materialized, ok := authority.identities.Processes[0]; ok &&
		(materialized.IPID != 0 || materialized.PID != 0) {
		return false
	}
	return true
}

func newTraceDBSchedulerAuthority(identities traceDBThreadIndex, collection traceDBLifecycleCollection) traceDBSchedulerAuthority {
	return traceDBSchedulerAuthority{
		identities:  identities,
		lifecycle:   collection.Lifecycle,
		initialized: true,
		complete:    collection.CreationComplete && collection.TerminalComplete && collection.ActivityComplete,
	}
}

func (authority traceDBSchedulerAuthority) threadPointAllows(itid, timestamp int64) bool {
	if !authority.initialized || !authority.complete {
		return false
	}
	thread, process, ok := authority.threadSubject(itid)
	if !ok || !traceDBLifecycleThreadPointAllows(authority.lifecycle, authority.identities, thread.ITID, timestamp) {
		return false
	}
	return process.PID <= 0 || traceDBLifecycleProcessPointAllows(authority.lifecycle, authority.identities, process.IPID, timestamp)
}

func (authority traceDBSchedulerAuthority) threadSourceIntervalAllows(itid, start, end int64) bool {
	if !authority.initialized || !authority.complete {
		return false
	}
	thread, process, ok := authority.threadSubject(itid)
	if !ok || !traceDBLifecycleThreadSourceIntervalAllows(authority.lifecycle, authority.identities, thread.ITID, start, end) {
		return false
	}
	return process.PID <= 0 || traceDBLifecycleProcessSourceIntervalAllows(authority.lifecycle, authority.identities, process.IPID, start, end)
}

func (authority traceDBSchedulerAuthority) threadClosedEndpointAllows(itid, start, end int64) bool {
	if !authority.initialized || !authority.complete {
		return false
	}
	thread, process, ok := authority.threadSubject(itid)
	if !ok || !traceDBLifecycleThreadClosedEndpointAllows(authority.lifecycle, authority.identities, thread.ITID, start, end) {
		return false
	}
	return process.PID <= 0 || traceDBLifecycleProcessClosedEndpointAllows(authority.lifecycle, authority.identities, process.IPID, start, end)
}

func (authority traceDBSchedulerAuthority) schedulerPointAllows(subject traceDBSchedulerSubject, timestamp int64) bool {
	if !authority.schedulerSubjectIsExact(subject) {
		return false
	}
	if subject.itid != 0 {
		return authority.threadPointAllows(subject.itid, timestamp)
	}
	return timestamp >= 0 && !authority.lifecycle.GlobalTaint &&
		!traceDBLifecycleHasPoint(authority.lifecycle.GlobalPoison, timestamp)
}

func (authority traceDBSchedulerAuthority) schedulerSourceIntervalAllows(subject traceDBSchedulerSubject, start, end int64) bool {
	if !authority.schedulerSubjectIsExact(subject) {
		return false
	}
	if subject.itid != 0 {
		return authority.threadSourceIntervalAllows(subject.itid, start, end)
	}
	return start >= 0 && end > start && !authority.lifecycle.GlobalTaint &&
		!traceDBLifecycleRangeHasPoint(authority.lifecycle.GlobalPoison, start, end, false)
}

// schedulerNextPointAllows validates the complete dependency from a query
// point to its nearest sched start. Exact idle may describe a point but never
// borrows a closed interval across time; causal wakeup/blocked endpoints have
// their own non-idle gates.
func (authority traceDBSchedulerAuthority) schedulerNextPointAllows(subject traceDBSchedulerSubject, query, candidate int64) bool {
	if candidate < query || !authority.schedulerSubjectIsExact(subject) {
		return false
	}
	if candidate == query {
		return authority.schedulerPointAllows(subject, candidate)
	}
	if subject.itid == 0 {
		return false
	}
	return authority.threadClosedEndpointAllows(subject.itid, query, candidate)
}

func (authority traceDBSchedulerAuthority) threadSubject(itid int64) (traceDBThread, traceDBProcess, bool) {
	thread, process, resolution := authority.resolveThreadSubject(itid)
	return thread, process, resolution == traceDBSchedulerThreadResolved
}

func (authority traceDBSchedulerAuthority) resolveThreadSubject(itid int64) (traceDBThread, traceDBProcess, traceDBSchedulerThreadResolution) {
	if itid <= 0 {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerThreadInvalid
	}
	if authority.identities.AmbiguousITID[itid] {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerThreadAmbiguous
	}
	thread, ok := authority.identities.ByITID[itid]
	if !ok {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerThreadMissing
	}
	if thread.ITID != itid || thread.TID <= 0 || authority.identities.AmbiguousIPID[thread.IPID] {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerThreadAmbiguous
	}
	process, ok := authority.identities.Processes[thread.IPID]
	if !ok {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerProcessMissing
	}
	if process.IPID != thread.IPID || process.PID < 0 {
		return traceDBThread{}, traceDBProcess{}, traceDBSchedulerThreadAmbiguous
	}
	return thread, process, traceDBSchedulerThreadResolved
}

type traceDBSchedulerRunningIndex struct {
	intervals   map[int64][]traceDBRunningInterval
	taintedITID map[int64]bool
	globalTaint bool
	initialized bool
}

func newTraceDBSchedulerRunningIndex(authority traceDBSchedulerAuthority,
	intervals map[int64][]traceDBRunningInterval, integrity traceDBRunningIntegrity, coverage *TraceDBCoverage,
) traceDBSchedulerRunningIndex {
	out := traceDBSchedulerRunningIndex{
		intervals:   map[int64][]traceDBRunningInterval{},
		taintedITID: map[int64]bool{},
		globalTaint: integrity.GlobalTaint || authority.lifecycle.GlobalTaint,
		initialized: authority.initialized,
	}
	for itid := range integrity.TaintedITIDs {
		out.taintedITID[itid] = true
	}
	rejectedRows := 0
	for itid, entries := range intervals {
		if out.globalTaint || out.taintedITID[itid] || itid != 0 && !authority.complete {
			out.taintedITID[itid] = true
			rejectedRows += len(entries)
			continue
		}
		laneValid := true
		subject, subjectOK := authority.schedulerSubjectFromExactITID(itid, true)
		for _, entry := range entries {
			if !subjectOK || !authority.schedulerSourceIntervalAllows(subject, entry.Start, entry.End) {
				laneValid = false
				break
			}
		}
		if !laneValid {
			out.taintedITID[itid] = true
			rejectedRows += len(entries)
			continue
		}
		out.intervals[itid] = entries
	}
	if coverage != nil {
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		coverage.FieldSources["scheduler_lifecycle"] = "same collector authority; Running intervals require half-open thread and positive-process generation admission"
		if rejectedRows > 0 {
			coverage.RowsEmitted -= rejectedRows
			if coverage.RowsEmitted < 0 {
				coverage.RowsEmitted = 0
			}
			traceDBAppendCoverageSkipped(coverage,
				fmt.Sprintf("scheduler authority audit: rejected_running_rows=%d total_tainted_itid_lanes=%d", rejectedRows, len(out.taintedITID)))
		}
		if !authority.initialized || !authority.complete {
			traceDBAppendCoverageSkipped(coverage, "scheduler_lifecycle_authority_complete=false")
		}
	}
	return out
}

func (index traceDBSchedulerRunningIndex) knownCPUAt(itid, timestamp int64) (int64, bool) {
	if !index.initialized || index.globalTaint || index.taintedITID[itid] {
		return 0, false
	}
	return traceDBKnownCPUAt(index.intervals, itid, timestamp)
}
