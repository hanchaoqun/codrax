package hitraceconv

import (
	"fmt"
	"math"
)

// traceDBSchedulerAuthority binds scheduler consumers to one exact identity
// index and one lifecycle collection. Its zero value rejects every query; a
// consumer cannot accidentally turn a missing or mismatched authority into a
// no-cut (therefore allow-all) lifecycle.
type traceDBSchedulerAuthority struct {
	identities         traceDBThreadIndex
	lifecycle          traceDBLifecycleIndex
	frameProfile       traceDBActivityITIDProfile
	frameProfileSource string
	initialized        bool
	complete           bool
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
	return traceDBCanonicalIdleIdentityExact(authority.identities)
}

func newTraceDBSchedulerAuthority(identities traceDBThreadIndex, collection traceDBLifecycleCollection) traceDBSchedulerAuthority {
	return traceDBSchedulerAuthority{
		identities:         identities,
		lifecycle:          collection.Lifecycle,
		frameProfile:       collection.FrameProfile,
		frameProfileSource: collection.FrameProfileSource,
		initialized:        true,
		complete:           collection.CreationComplete && collection.TerminalComplete && collection.ActivityComplete,
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

// processClosedEndpointAllows proves that a positive canonical process owns
// both physical endpoints and the complete closed interval between them. It
// deliberately does not constrain a particular thread, so an async task may
// migrate between threads inside the same process generation.
func (authority traceDBSchedulerAuthority) processClosedEndpointAllows(ipid, start, end int64) bool {
	if !authority.initialized || !authority.complete || end <= start || authority.identities.AmbiguousIPID[ipid] {
		return false
	}
	process, ok := authority.identities.Processes[ipid]
	if !ok || process.IPID != ipid || process.PID <= 0 || process.PID > math.MaxInt32 {
		return false
	}
	return traceDBLifecycleProcessClosedEndpointAllows(authority.lifecycle, authority.identities, ipid, start, end)
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

// threadDisplayName exposes only the precomputed, display-only projection.
// Scheduler consumers must not reopen the raw identity index merely to recover
// a label: identity and lifecycle decisions stay behind the authority methods.
func (authority traceDBSchedulerAuthority) threadDisplayName(thread traceDBThread) string {
	return traceDBThreadDisplayNameValue(authority.identities, thread)
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
	intervals             map[int64][]traceDBRunningInterval
	rawFallbackIntervals  map[int64][]traceDBRunningInterval
	rejectedITID          map[int64]bool
	sourceTaintedITID     map[int64]bool
	lifecycleRejectedITID map[int64]bool
	sourceGlobalTaint     bool
	lifecycleGlobalTaint  bool
	rawFallbackEnabled    bool
	initialized           bool
}

type traceDBSchedulerRunningLookupStatus uint8

const (
	traceDBSchedulerRunningUnknown traceDBSchedulerRunningLookupStatus = iota
	traceDBSchedulerRunningKnown
	traceDBSchedulerRunningSourceTainted
	traceDBSchedulerRunningLifecycleRejected
)

func newTraceDBSchedulerRunningIndex(authority traceDBSchedulerAuthority,
	intervals map[int64][]traceDBRunningInterval, integrity traceDBRunningIntegrity, coverage *TraceDBCoverage,
) traceDBSchedulerRunningIndex {
	out := traceDBSchedulerRunningIndex{
		intervals:             map[int64][]traceDBRunningInterval{},
		rejectedITID:          map[int64]bool{},
		sourceTaintedITID:     map[int64]bool{},
		lifecycleRejectedITID: map[int64]bool{},
		sourceGlobalTaint:     integrity.GlobalTaint,
		lifecycleGlobalTaint:  authority.lifecycle.GlobalTaint,
		initialized:           authority.initialized,
	}
	for itid := range integrity.TaintedITIDs {
		out.sourceTaintedITID[itid] = true
		out.rejectedITID[itid] = true
	}
	rejectedRows := 0
	for itid, entries := range intervals {
		if out.sourceGlobalTaint || out.sourceTaintedITID[itid] {
			out.rejectedITID[itid] = true
			rejectedRows += len(entries)
			continue
		}
		if out.lifecycleGlobalTaint || !authority.initialized || itid != 0 && !authority.complete {
			out.lifecycleRejectedITID[itid] = true
			out.rejectedITID[itid] = true
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
			out.lifecycleRejectedITID[itid] = true
			out.rejectedITID[itid] = true
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
				fmt.Sprintf("scheduler authority audit: rejected_running_rows=%d source_tainted_itid_lanes=%d lifecycle_rejected_itid_lanes=%d total_rejected_itid_lanes=%d",
					rejectedRows, len(out.sourceTaintedITID), len(out.lifecycleRejectedITID), len(out.rejectedITID)))
		}
		if !authority.initialized || !authority.complete {
			traceDBAppendCoverageSkipped(coverage, "scheduler_lifecycle_authority_complete=false")
		}
	}
	return out
}

func (index traceDBSchedulerRunningIndex) knownCPUAt(itid, timestamp int64) (int64, bool) {
	cpu, status := index.lookupCPUAt(itid, timestamp)
	return cpu, status == traceDBSchedulerRunningKnown
}

func (index traceDBSchedulerRunningIndex) lookupCPUAt(itid, timestamp int64) (int64, traceDBSchedulerRunningLookupStatus) {
	if !index.initialized {
		return 0, traceDBSchedulerRunningUnknown
	}
	if index.lifecycleGlobalTaint || index.lifecycleRejectedITID[itid] {
		return 0, traceDBSchedulerRunningLifecycleRejected
	}
	dbStatus := traceDBSchedulerRunningUnknown
	dbCPU := int64(0)
	if index.sourceGlobalTaint || index.sourceTaintedITID[itid] {
		dbStatus = traceDBSchedulerRunningSourceTainted
	} else if cpu, known := traceDBKnownCPUAt(index.intervals, itid, timestamp); known {
		dbCPU = cpu
		dbStatus = traceDBSchedulerRunningKnown
	}
	if !index.rawFallbackEnabled {
		return dbCPU, dbStatus
	}
	rawCPU, rawKnown := traceDBKnownCPUAt(
		index.rawFallbackIntervals, itid, timestamp)
	if dbStatus == traceDBSchedulerRunningKnown {
		if rawKnown && rawCPU != dbCPU {
			// Two precise but contradictory CPU claims must never select one
			// opportunistically. Source-tainted is the existing typed
			// unavailable shape for contradictory physical placement evidence.
			return 0, traceDBSchedulerRunningSourceTainted
		}
		return dbCPU, dbStatus
	}
	if rawKnown {
		return rawCPU, traceDBSchedulerRunningKnown
	}
	return dbCPU, dbStatus
}
