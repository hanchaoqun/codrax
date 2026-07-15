package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// blockedReasonIntegrityFailure records a field-local malformed
// sched_blocked_reason row outside Event.  It is intentionally not a
// schedulerRowIntegrityFailure: blocked_reason refines a D-family interval,
// but does not establish the underlying sched_switch/wakeup state machine.
type blockedReasonIntegrityFailure struct {
	Line           int
	LocalLine      int
	Ts             float64
	CPU            int
	PIDs           []int
	AffectsAllPIDs bool
	// PIDCandidateSetTruncated is stronger and narrower than
	// AffectsAllPIDs. The latter is also used for audit-only rows whose PID is
	// wholly malformed/missing; only this bit proves that canonical identities
	// existed but could not all be retained for refinement matching.
	PIDCandidateSetTruncated bool
	Fields                   []string
	SourcePath               string
}

// blockedReasonIntegrityOverflowScope is the bounded audit witness for rows
// dropped after blockedReasonIntegrityFailureCap.  A naked global "capped"
// bit is insufficient: one malformed marker early in a long capture must not
// withdraw blocked-reason refinement from every later query window.  The
// envelope retains the exact time/line domain and a bounded PID union of the
// dropped rows.  It may conservatively cover a gap between dropped rows, but
// never escapes their outer physical range or a proven-disjoint PID scope.
type blockedReasonIntegrityOverflowScope struct {
	Set            bool
	MinLine        int
	MaxLine        int
	MinTs          float64
	MaxTs          float64
	PIDs           []int
	AffectsAllPIDs bool
	// PIDDomains retain one conservative physical envelope per dropped canonical
	// PID instead of cross-producting one global time envelope with a PID union.
	// Once this bounded exact PID inventory itself overflows, AffectsAllPIDs is
	// raised as the precise resource-truncation signal.
	PIDDomains []blockedReasonIntegrityPIDDomain
}

type blockedReasonIntegrityPIDDomain struct {
	PID     int
	MinLine int
	MaxLine int
	MinTs   float64
	MaxTs   float64
}

const blockedReasonIntegrityFailureCap = 64

func (f *blockedReasonIntegrityFailure) reason() string {
	if f == nil {
		return ""
	}
	reason := fmt.Sprintf("blocked_reason_projection_incomplete invalid=%s ts=%.6f cpu=%d line=%d",
		strings.Join(f.Fields, ","), f.Ts, f.CPU, f.Line)
	if f.SourcePath != "" {
		reason += " source=" + f.SourcePath
		reason += witnessLocalLineSuffix(f.Line, f.LocalLine)
	}
	return reason
}

func blockedReasonIntegrityRawCandidate(line string) bool {
	return strings.Contains(line, "sched_blocked_reason:")
}

func blockedReasonValidationFailure(lineNo int, line string) *blockedReasonIntegrityFailure {
	var scan lineScan
	scan.reset(lineNo, line)
	return blockedReasonValidationFailureScan(&scan)
}

func blockedReasonValidationFailureScan(s *lineScan) *blockedReasonIntegrityFailure {
	if s == nil || !blockedReasonIntegrityRawCandidate(s.line) {
		return nil
	}
	m := s.match()
	if len(m) == 0 || strings.TrimSuffix(strings.TrimSpace(m[6]), ":") != "sched_blocked_reason" {
		return nil
	}
	_ = s.keyValues()
	if !s.schedulerTyped.BlockedReason || len(s.schedulerTyped.BlockedIssues) == 0 {
		return nil
	}
	ts, _ := s.timestamp()
	cpu, present, valid, _ := parseTraceCPUScalar(m[4])
	if !present || !valid {
		cpu = -1
	}
	failure := &blockedReasonIntegrityFailure{
		Line: s.lineNo, Ts: ts, CPU: cpu,
		PIDs:                     append([]int(nil), s.schedulerTyped.BlockedPIDCandidates...),
		PIDCandidateSetTruncated: s.schedulerTyped.BlockedPIDCandidatesTruncated,
	}
	for _, issue := range s.schedulerTyped.BlockedIssues {
		field := issue.Field
		if issue.Reason != "" {
			field += "_" + issue.Reason
		}
		failure.Fields = append(failure.Fields, field)
	}
	failure.Fields = uniqueSortedStrings(failure.Fields)
	sort.Ints(failure.PIDs)
	failure.AffectsAllPIDs = len(failure.PIDs) == 0 || failure.PIDCandidateSetTruncated
	return failure
}

func blockedReasonIntegrityFailureRelevantToQuery(f *blockedReasonIntegrityFailure, q Query, onlyPID int) bool {
	if f == nil {
		return false
	}
	lineWindow := q.LineStart > 0 || q.LineEnd > 0
	if lineWindow {
		if q.LineStart > 0 && f.Line < q.LineStart {
			return false
		}
		if q.LineEnd > 0 && f.Line > q.LineEnd {
			return false
		}
	} else {
		// Ordinary interval queries retain in-window markers and the closing-side
		// +5us tail. The standard opening-side pre-window carry is governed
		// separately by scheduler-head authority: only a marker within 5us of the
		// same still-open D slice's start may cross TimeStart.
		if q.TimeStart > 0 && f.Ts < q.TimeStart {
			return false
		}
		if q.TimeEnd > 0 && f.Ts > wakeupClosingUpperBound(q.TimeEnd) {
			return false
		}
	}
	if onlyPID <= 0 || f.AffectsAllPIDs {
		return true
	}
	for _, pid := range f.PIDs {
		if pid == onlyPID {
			return true
		}
	}
	return false
}

func (s *blockedReasonIntegrityOverflowScope) observe(failure blockedReasonIntegrityFailure) {
	if s == nil {
		return
	}
	if !s.Set {
		s.Set = true
		s.MinLine, s.MaxLine = failure.Line, failure.Line
		s.MinTs, s.MaxTs = failure.Ts, failure.Ts
	} else {
		if failure.Line < s.MinLine {
			s.MinLine = failure.Line
		}
		if failure.Line > s.MaxLine {
			s.MaxLine = failure.Line
		}
		if failure.Ts < s.MinTs {
			s.MinTs = failure.Ts
		}
		if failure.Ts > s.MaxTs {
			s.MaxTs = failure.Ts
		}
	}
	s.addPIDScope(failure.PIDs, failure.AffectsAllPIDs)
	if !s.AffectsAllPIDs {
		for _, pid := range failure.PIDs {
			if pid < 0 {
				continue
			}
			s.mergePIDDomain(blockedReasonIntegrityPIDDomain{
				PID: pid, MinLine: failure.Line, MaxLine: failure.Line,
				MinTs: failure.Ts, MaxTs: failure.Ts,
			})
		}
	}
}

func (s *blockedReasonIntegrityOverflowScope) mergePIDDomain(domain blockedReasonIntegrityPIDDomain) {
	if s == nil || s.AffectsAllPIDs || domain.PID < 0 {
		return
	}
	for i := range s.PIDDomains {
		if s.PIDDomains[i].PID != domain.PID {
			continue
		}
		if domain.MinLine < s.PIDDomains[i].MinLine {
			s.PIDDomains[i].MinLine = domain.MinLine
		}
		if domain.MaxLine > s.PIDDomains[i].MaxLine {
			s.PIDDomains[i].MaxLine = domain.MaxLine
		}
		if domain.MinTs < s.PIDDomains[i].MinTs {
			s.PIDDomains[i].MinTs = domain.MinTs
		}
		if domain.MaxTs > s.PIDDomains[i].MaxTs {
			s.PIDDomains[i].MaxTs = domain.MaxTs
		}
		return
	}
	if len(s.PIDDomains) >= schedulerPIDCandidateScopeCap {
		s.AffectsAllPIDs = true
		s.PIDs = nil
		s.PIDDomains = nil
		return
	}
	s.PIDDomains = append(s.PIDDomains, domain)
}

func (s *blockedReasonIntegrityOverflowScope) addPIDScope(pids []int, affectsAll bool) {
	if s == nil || s.AffectsAllPIDs {
		return
	}
	if affectsAll || len(pids) == 0 {
		s.AffectsAllPIDs = true
		s.PIDs = nil
		s.PIDDomains = nil
		return
	}
	seen := make(map[int]struct{}, len(s.PIDs)+len(pids))
	for _, pid := range s.PIDs {
		if pid >= 0 {
			seen[pid] = struct{}{}
		}
	}
	for _, pid := range pids {
		if pid >= 0 {
			seen[pid] = struct{}{}
		}
		if len(seen) > schedulerPIDCandidateScopeCap {
			s.AffectsAllPIDs = true
			s.PIDs = nil
			s.PIDDomains = nil
			return
		}
	}
	s.PIDs = s.PIDs[:0]
	for pid := range seen {
		s.PIDs = append(s.PIDs, pid)
	}
	sort.Ints(s.PIDs)
}

func (s *blockedReasonIntegrityOverflowScope) merge(other blockedReasonIntegrityOverflowScope) {
	if s == nil || !other.Set {
		return
	}
	if !s.Set {
		*s = blockedReasonIntegrityOverflowScope{
			Set: other.Set, MinLine: other.MinLine, MaxLine: other.MaxLine,
			MinTs: other.MinTs, MaxTs: other.MaxTs,
			PIDs: append([]int(nil), other.PIDs...), AffectsAllPIDs: other.AffectsAllPIDs,
			PIDDomains: append([]blockedReasonIntegrityPIDDomain(nil), other.PIDDomains...),
		}
		return
	}
	if other.MinLine < s.MinLine {
		s.MinLine = other.MinLine
	}
	if other.MaxLine > s.MaxLine {
		s.MaxLine = other.MaxLine
	}
	if other.MinTs < s.MinTs {
		s.MinTs = other.MinTs
	}
	if other.MaxTs > s.MaxTs {
		s.MaxTs = other.MaxTs
	}
	s.addPIDScope(other.PIDs, other.AffectsAllPIDs)
	if s.AffectsAllPIDs {
		return
	}
	domains := other.PIDDomains
	if len(domains) == 0 {
		// Backward-compatible construction path for tests/legacy in-memory
		// scopes that predate PIDDomains: their own envelope is still exact for
		// the supplied PID set.
		domains = make([]blockedReasonIntegrityPIDDomain, 0, len(other.PIDs))
		for _, pid := range other.PIDs {
			domains = append(domains, blockedReasonIntegrityPIDDomain{PID: pid, MinLine: other.MinLine, MaxLine: other.MaxLine, MinTs: other.MinTs, MaxTs: other.MaxTs})
		}
	}
	for _, domain := range domains {
		s.mergePIDDomain(domain)
		if s.AffectsAllPIDs {
			return
		}
	}
}

func (s blockedReasonIntegrityOverflowScope) clone() blockedReasonIntegrityOverflowScope {
	s.PIDs = append([]int(nil), s.PIDs...)
	s.PIDDomains = append([]blockedReasonIntegrityPIDDomain(nil), s.PIDDomains...)
	return s
}

func mapBlockedReasonIntegrityOverflowScope(s blockedReasonIntegrityOverflowScope, source TraceArtifactSource) (blockedReasonIntegrityOverflowScope, bool) {
	if !s.Set {
		return blockedReasonIntegrityOverflowScope{}, true
	}
	minTs, minOK := source.toCanonicalTsChecked(s.MinTs)
	maxTs, maxOK := source.toCanonicalTsChecked(s.MaxTs)
	if !minOK || !maxOK {
		return blockedReasonIntegrityOverflowScope{}, false
	}
	s.MinLine += source.VirtualLineBase
	s.MaxLine += source.VirtualLineBase
	if minTs <= maxTs {
		s.MinTs, s.MaxTs = minTs, maxTs
	} else {
		s.MinTs, s.MaxTs = maxTs, minTs
	}
	s.PIDs = append([]int(nil), s.PIDs...)
	s.PIDDomains = append([]blockedReasonIntegrityPIDDomain(nil), s.PIDDomains...)
	for i := range s.PIDDomains {
		minTs, minOK := source.toCanonicalTsChecked(s.PIDDomains[i].MinTs)
		maxTs, maxOK := source.toCanonicalTsChecked(s.PIDDomains[i].MaxTs)
		if !minOK || !maxOK {
			return blockedReasonIntegrityOverflowScope{}, false
		}
		s.PIDDomains[i].MinLine += source.VirtualLineBase
		s.PIDDomains[i].MaxLine += source.VirtualLineBase
		if minTs <= maxTs {
			s.PIDDomains[i].MinTs, s.PIDDomains[i].MaxTs = minTs, maxTs
		} else {
			s.PIDDomains[i].MinTs, s.PIDDomains[i].MaxTs = maxTs, minTs
		}
	}
	return s, true
}

func blockedReasonIntegrityOverflowRelevantToQuery(s blockedReasonIntegrityOverflowScope, q Query, onlyPID int) bool {
	if !s.Set {
		return false
	}
	lineWindow := q.LineStart > 0 || q.LineEnd > 0
	if lineWindow {
		if q.LineStart > 0 && s.MaxLine < q.LineStart {
			return false
		}
		if q.LineEnd > 0 && s.MinLine > q.LineEnd {
			return false
		}
	} else {
		if q.TimeStart > 0 && s.MaxTs < q.TimeStart {
			return false
		}
		if q.TimeEnd > 0 && s.MinTs > wakeupClosingUpperBound(q.TimeEnd) {
			return false
		}
	}
	if s.AffectsAllPIDs {
		return true
	}
	if len(s.PIDDomains) > 0 {
		for _, domain := range s.PIDDomains {
			if (onlyPID <= 0 || domain.PID == onlyPID) && blockedReasonPIDDomainIntersectsQuery(domain, q) {
				return true
			}
		}
		return false
	}
	if onlyPID <= 0 {
		return true
	}
	for _, pid := range s.PIDs {
		if pid == onlyPID {
			return true
		}
	}
	return false
}

func blockedReasonPIDDomainIntersectsQuery(domain blockedReasonIntegrityPIDDomain, q Query) bool {
	if q.LineStart > 0 || q.LineEnd > 0 {
		return (q.LineStart <= 0 || domain.MaxLine >= q.LineStart) &&
			(q.LineEnd <= 0 || domain.MinLine <= q.LineEnd)
	}
	return (q.TimeStart <= 0 || domain.MaxTs >= q.TimeStart) &&
		(q.TimeEnd <= 0 || domain.MinTs <= wakeupClosingUpperBound(q.TimeEnd))
}

func appendBlockedReasonIntegrityFailure(idx *Index, failure blockedReasonIntegrityFailure) {
	if idx == nil {
		return
	}
	for _, existing := range idx.blockedReasonIntegrityFailures {
		if existing.Line == failure.Line && existing.Ts == failure.Ts &&
			strings.Join(existing.Fields, ",") == strings.Join(failure.Fields, ",") {
			return
		}
	}
	if len(idx.blockedReasonIntegrityFailures) >= blockedReasonIntegrityFailureCap {
		idx.blockedReasonIntegrityFailuresCapped = true
		idx.blockedReasonIntegrityOverflow.observe(failure)
		if blockedReasonFailureHasIdentityIssue(failure) {
			// A wholly malformed/missing PID has no authority to bind any
			// canonical thread. Keep it in the general audit envelope, but do not
			// let an audit-only global scope withdraw unrelated D/IO refinement.
			// Exact candidate subsets remain scoped; only a genuinely truncated
			// canonical set requires a global hard gate.
			positivePIDs := make([]int, 0, len(failure.PIDs))
			for _, pid := range failure.PIDs {
				if pid > 0 {
					positivePIDs = append(positivePIDs, pid)
				}
			}
			if len(positivePIDs) > 0 || failure.PIDCandidateSetTruncated {
				identityFailure := failure
				identityFailure.PIDs = positivePIDs
				identityFailure.AffectsAllPIDs = failure.PIDCandidateSetTruncated
				idx.blockedReasonIdentityOverflow.observe(identityFailure)
			}
		}
		return
	}
	failure.PIDs = append([]int(nil), failure.PIDs...)
	failure.Fields = append([]string(nil), failure.Fields...)
	idx.blockedReasonIntegrityFailures = append(idx.blockedReasonIntegrityFailures, failure)
}

func blockedReasonFailureHasIdentityIssue(failure blockedReasonIntegrityFailure) bool {
	for _, field := range failure.Fields {
		if strings.HasPrefix(field, "pid_") {
			return true
		}
	}
	return false
}

func blockedReasonIntegrityFailuresForQuery(idx *Index, q Query, onlyPID int) ([]blockedReasonIntegrityFailure, bool) {
	if idx == nil {
		return nil, false
	}
	out := make([]blockedReasonIntegrityFailure, 0, min(len(idx.blockedReasonIntegrityFailures), 8))
	for i := range idx.blockedReasonIntegrityFailures {
		if blockedReasonIntegrityFailureRelevantToQuery(&idx.blockedReasonIntegrityFailures[i], q, onlyPID) {
			out = append(out, idx.blockedReasonIntegrityFailures[i])
		}
	}
	capped := idx.blockedReasonIntegrityFailuresCapped &&
		blockedReasonIntegrityOverflowRelevantToQuery(idx.blockedReasonIntegrityOverflow, q, onlyPID)
	return out, capped
}

// blockedReasonRefinementCappedForQuery answers the narrower hard-gate
// question: can a dropped malformed row belong to a positive thread whose
// D/IO/caller refinement this consumer would publish? PID 0 remains visible
// in the global integrity audit, but cannot refine a positive scheduler task
// and therefore must not withdraw every positive-TID lane in a global view.
func blockedReasonRefinementCappedForQuery(idx *Index, q Query, onlyPID int) bool {
	if idx == nil {
		return false
	}
	// A retained row can itself exceed the canonical candidate-set bound.
	// This is a precise, physical truncation signal, unlike pid=bad/missing.
	for i := range idx.blockedReasonIntegrityFailures {
		failure := &idx.blockedReasonIntegrityFailures[i]
		if failure.PIDCandidateSetTruncated && blockedReasonIntegrityFailureRelevantToQuery(failure, q, 0) {
			return true
		}
	}
	if !idx.blockedReasonIntegrityFailuresCapped ||
		!blockedReasonIntegrityOverflowRelevantToQuery(idx.blockedReasonIdentityOverflow, q, onlyPID) {
		return false
	}
	if onlyPID > 0 || idx.blockedReasonIdentityOverflow.AffectsAllPIDs {
		return true
	}
	for _, pid := range idx.blockedReasonIdentityOverflow.PIDs {
		if pid > 0 {
			return true
		}
	}
	return false
}

// blockedReasonRefinementUnavailableForInterval is the hard-gate twin of the
// interval matcher. A dropped identity candidate may withdraw only an interval
// whose exact physical closing domain it can inhabit; an earlier bad row for
// the same PID must not erase a later valid sibling in a broad query window.
func blockedReasonRefinementUnavailableForInterval(idx *Index, q Query, pid int, start, end float64, physicalClosure bool) bool {
	if idx == nil || pid <= 0 || end <= start {
		return false
	}
	upper := end
	if physicalClosure {
		upper = wakeupClosingUpperBound(end)
	}
	markerInDomain := func(ts float64, line int) bool {
		if q.LineStart > 0 && line < q.LineStart {
			return false
		}
		if q.LineEnd > 0 && line > q.LineEnd {
			return false
		}
		return ts > start && ts <= upper
	}
	for i := range idx.blockedReasonIntegrityFailures {
		failure := &idx.blockedReasonIntegrityFailures[i]
		if failure.PIDCandidateSetTruncated && markerInDomain(failure.Ts, failure.Line) {
			return true
		}
	}
	if !idx.blockedReasonIntegrityFailuresCapped || !idx.blockedReasonIdentityOverflow.Set {
		return false
	}
	scope := idx.blockedReasonIdentityOverflow
	if scope.AffectsAllPIDs {
		return scope.MaxTs > start && scope.MinTs <= upper &&
			(q.LineStart <= 0 || scope.MaxLine >= q.LineStart) &&
			(q.LineEnd <= 0 || scope.MinLine <= q.LineEnd)
	}
	if len(scope.PIDDomains) > 0 {
		for _, domain := range scope.PIDDomains {
			if domain.PID != pid || domain.MaxTs <= start || domain.MinTs > upper {
				continue
			}
			if (q.LineStart <= 0 || domain.MaxLine >= q.LineStart) && (q.LineEnd <= 0 || domain.MinLine <= q.LineEnd) {
				return true
			}
		}
		return false
	}
	for _, candidate := range scope.PIDs {
		if candidate == pid {
			return scope.MaxTs > start && scope.MinTs <= upper
		}
	}
	return false
}

func blockedReasonIntegrityCaveats(idx *Index, q Query, onlyPID int) []string {
	failures, capped := blockedReasonIntegrityFailuresForQuery(idx, q, onlyPID)
	if len(failures) == 0 && !capped {
		return nil
	}
	out := make([]string, 0, len(failures)+1)
	for i := range failures {
		if failures[i].PIDCandidateSetTruncated {
			out = append(out, "blocked_reason_integrity_degraded=true; "+failures[i].reason()+"; blocked_reason_pid_candidate_set_truncated=true affected_pid_scope=all; canonical PID candidate inventory exceeded its deterministic bound, so D/IO/caller interval refinement fails closed while base scheduler states remain available")
		} else {
			out = append(out, "blocked_reason_integrity_degraded=true; "+failures[i].reason()+"; only the affected blocked-reason field projection was withdrawn")
		}
	}
	if capped {
		if blockedReasonRefinementCappedForQuery(idx, q, onlyPID) {
			scope := "all"
			if !idx.blockedReasonIdentityOverflow.AffectsAllPIDs {
				positive := make([]int, 0, len(idx.blockedReasonIdentityOverflow.PIDs))
				for _, pid := range idx.blockedReasonIdentityOverflow.PIDs {
					if pid > 0 {
						positive = append(positive, pid)
					}
				}
				scope = fmt.Sprint(positive)
			}
			out = append(out, "blocked_reason_integrity_degraded=true; blocked_reason_integrity_audit_truncated=true; affected_pid_scope="+scope+"; blocked-reason D/IO/caller refinement must fail closed only for this scope, while base scheduler states and unaffected PIDs remain available")
		} else {
			suffix := "truncated audit details remain represented by per-event typed known/unknown fields; no additional D/IO withdrawal is required"
			if idx.blockedReasonIdentityOverflow.Set && !idx.blockedReasonIdentityOverflow.AffectsAllPIDs {
				hasPositive := false
				for _, pid := range idx.blockedReasonIdentityOverflow.PIDs {
					hasPositive = hasPositive || pid > 0
				}
				if !hasPositive {
					suffix = "identity-overflow rows are scoped to pid=0 and cannot bind a positive scheduler task, so positive-thread D/IO/caller refinement remains available"
				} else {
					suffix = "identity-overflow PID scope is disjoint from this query target; its D/IO/caller refinement remains available"
				}
			}
			out = append(out, "blocked_reason_integrity_degraded=true; blocked_reason_integrity_audit_truncated=true; "+suffix)
		}
	}
	return out
}
