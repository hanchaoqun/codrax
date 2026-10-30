package tracequery

import "strings"

// sourceEventCoverage is a value-only receipt for one physical file generation.
// maxEventTs is the maximum valid physical ftrace event timestamp, not the last
// retained event, last physical row, query end, or a synthetic state-closing event.
// Timestamp order remains a separate proof. A complete scan with no valid event
// is distinct from a complete scan whose maximum timestamp is exactly zero.
// This receipt grants neither query completeness nor causal/clock-map authority.
type sourceEventCoverage struct {
	path       string
	identity   traceFileIdentity
	complete   bool
	hasEvent   bool
	maxEventTs float64
}

func (c sourceEventCoverage) matches(path string, identity traceFileIdentity) bool {
	return c.complete && c.path == path && c.identity.Initialized() && c.identity.SameVersion(identity)
}

// A scan may gather pending coverage only by visiting every physical line from
// byte zero. Old anchor-prefix/order metadata cannot reconstruct this permission.
// Reaching EOF is necessary but not sufficient: parseSingle publishes the receipt
// only after cancellation and held-file/path generation checks succeed.
type sourceEventCoverageScan struct {
	fromStart  bool
	reachedEOF bool
	lines      int
	mixedTime  bool
	hasEvent   bool
	maxEventTs float64
}

func (s *sourceEventCoverageScan) observe(scan *lineScan, ts float64, hasTS bool) {
	if !s.fromStart {
		return
	}
	lineNo, line := scan.lineNo, scan.line
	if lineNo != s.lines+1 {
		s.fromStart = false
		return
	}
	s.lines = lineNo
	m := scan.match()
	if len(m) == 0 {
		// Recognized timestamped carriers have their own clock/entity contracts.
		// Ignoring one while certifying an earlier ftrace maximum would over-clip
		// a mixed source. Unrecognized prose and partial headers grant nothing.
		s.mixedTime = s.mixedTime || hasTS
		return
	}
	if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
		return
	}
	if _, ok := leadingExactEventNameFromPrefixMatch(line, scan.mIndexes); !ok || !hasTS {
		s.mixedTime = s.mixedTime || hasTS
		return
	}
	if _, ok := parseFtraceHeaderTID(m[2]); !ok {
		s.mixedTime = true
		return
	}
	if _, present, valid, _ := parseTraceCPUScalar(m[4]); !present || !valid {
		s.mixedTime = true
		return
	}
	if !s.hasEvent || ts > s.maxEventTs {
		s.maxEventTs = ts
	}
	s.hasEvent = true
}

func (r *anchorRecorder) publishSourceEventCoverage(path string, identity traceFileIdentity) {
	if !r.set.sourceEventCoverage.matches(path, identity) {
		r.set.sourceEventCoverage = sourceEventCoverage{}
	}
	s := r.sourceCoverageScan
	if s.fromStart && s.reachedEOF && !s.mixedTime && identity.Initialized() {
		r.set.sourceEventCoverage = sourceEventCoverage{
			path: path, identity: identity, complete: true,
			hasEvent: s.hasEvent, maxEventTs: s.maxEventTs,
		}
	}
}

func (idx *Index) measuredPhysicalEventEnd() (float64, bool) {
	// A global scheduler consumer must not borrow another bundle member's EOF.
	// Per-state source attribution/affine bounds are deliberately not inferred.
	if idx == nil || len(idx.TraceArtifacts) != 1 {
		return 0, false
	}
	source := idx.TraceArtifacts[0]
	c := source.eventCoverage
	if !source.CausalCompatible || source.ClockAlignment != TraceClockAlignmentIdentity ||
		!source.timestampOrder.AllowsTimeEndEarlyStop() || !c.hasEvent ||
		!c.matches(source.SourcePath, source.sourceIdentity) {
		return 0, false
	}
	return c.maxEventTs, true
}
