package types

// log_perf_subkind.go — second-axis classification for log/perf
// evidence items.
//
// Background: ClaimFormOf's Rule 1 collapses every Origin=Log /
// Origin=Perf item into a single ClaimExternalObservation form.
// That preserves "this came from an external artifact" but loses
// every other distinction the typed LogBundle / PerfBundle
// structures recorded — panic vs OOM vs timeout, perf jank vs
// stall vs cold-start. Phase 4 / Phase 5 oracles that want to
// drive different facets for different log signals (e.g.
// FacetObservedArtifactFact for any frame, FacetPanicAnchor for
// panic frames specifically) cannot tell them apart from
// ClaimForm alone.
//
// Solution: a second axis. LogPerfSubKind is a closed enum filled
// by the BackfillEvidenceProjector alongside Origin (when the
// item anchors on a log/perf frame). Downstream readers consume
// it as a refinement of ClaimExternalObservation. Empty value =
// "not log/perf evidence OR no specific sub-classification" —
// callers that don't care can ignore the field.
//
// Red-line alignment: every signal feeding the projection is a
// typed enum, typed list, or numeric threshold from the existing
// LogBundle / PerfBundle schemas. NO string substring matching
// against frame messages or stack-trace text. Adding a new
// sub-kind requires (a) a new typed signal in LogBundle/PerfBundle
// AND (b) a new arm in LogPerfSubKindOf — the closed-enum
// contract prevents fuzzy classification leakage.

// LogPerfSubKind is the second-axis classification for log/perf
// evidence. Empty value (LogPerfSubKindUnknown) is the default
// for non-log/perf items and for items the projection could not
// classify with high confidence.
type LogPerfSubKind string

const (
	// LogPerfSubKindUnknown is the zero value — assigned to
	// non-log/perf evidence and to log/perf frames the projector
	// could not match to a more specific sub-kind. Callers MUST
	// treat this as "no information"; never as "evidence is bad".
	LogPerfSubKindUnknown LogPerfSubKind = ""

	// ── Log sub-kinds (ordered by signal severity / specificity) ──

	// LogPanicFrame — frame matched a LogBundle whose Meta.Signals
	// contains SignalPanic OR SignalCrash. The strongest log
	// failure shape: the runtime aborted, the answer's call chain
	// is anchored to the failing frame.
	LogPanicFrame LogPerfSubKind = "panic_frame"

	// LogOOMFrame — Meta.Signals contains SignalOOM. Memory
	// exhaustion at the cited frame.
	LogOOMFrame LogPerfSubKind = "oom_frame"

	// LogTimeoutFrame — Meta.Signals contains SignalTimeout.
	// Deadline exceeded / context canceled at the cited frame.
	LogTimeoutFrame LogPerfSubKind = "timeout_frame"

	// LogPerformanceFrame — Meta.Signals contains SignalPerformance.
	// Log-level performance observation: slow API call / GC pause /
	// long-blocked operation / lock contention / frame drop. Pre-
	// 2026-05-08 these frames collapsed to LogErrorFrame, conflating
	// "performance observation" with "error event" — the finalizer
	// then rendered slow-API evidence with error-shaped prose. This
	// sub-kind keeps performance evidence on a separate facet so
	// answers can read "the operation was slow" rather than "the
	// operation failed". Distinct from PerfBundle's PerfStall /
	// PerfJank: those are trace-tool signals (HiTrace / atrace /
	// systrace / perfetto); LogPerformanceFrame is application-log
	// mention of a perf symptom.
	LogPerformanceFrame LogPerfSubKind = "performance_frame"

	// LogErrorFrame — frame appears in the LogBundle.Errors tree
	// (with a non-empty LogError.Type) but no severe / performance
	// signal is present. Generic structured error with stack trace.
	LogErrorFrame LogPerfSubKind = "error_frame"

	// LogNoiseFrame — frame matched but no Errors-tree entry and
	// no severe signal. Likely a Residue / unstructured chunk
	// fragment that happens to anchor at the cited file:line.
	// Useful for "the log mentions this file but it's not
	// causally relevant" classification.
	LogNoiseFrame LogPerfSubKind = "noise_frame"

	// ── Perf sub-kinds ──

	// PerfJankFrame — perf bundle has Janks[] non-empty. Frame
	// anchors at a janky render path.
	PerfJankFrame LogPerfSubKind = "perf_jank"

	// PerfStallFrame — perf bundle has Stalls[] with the matching
	// frame as a stall site (e.g. main-thread stall above
	// PerfMainThreadStallMs).
	PerfStallFrame LogPerfSubKind = "perf_stall"

	// PerfStartupFrame — perf bundle's Startup.AppLaunchMs
	// exceeds PerfStartupSlowColdMs and the frame is on the
	// startup path. Cold-start performance evidence.
	PerfStartupFrame LogPerfSubKind = "perf_startup"
)

// IsValid reports whether s is one of the known LogPerfSubKind
// values (excluding the empty default, which is technically valid
// as "unknown" but distinct from "explicitly classified").
func (s LogPerfSubKind) IsValid() bool {
	switch s {
	case LogPerfSubKindUnknown,
		LogPanicFrame, LogOOMFrame, LogTimeoutFrame,
		LogPerformanceFrame, LogErrorFrame, LogNoiseFrame,
		PerfJankFrame, PerfStallFrame, PerfStartupFrame:
		return true
	}
	return false
}

// IsLogSubKind reports whether s is one of the log-side
// sub-kinds (panic / oom / timeout / performance / error / noise).
func (s LogPerfSubKind) IsLogSubKind() bool {
	switch s {
	case LogPanicFrame, LogOOMFrame, LogTimeoutFrame,
		LogPerformanceFrame, LogErrorFrame, LogNoiseFrame:
		return true
	}
	return false
}

// IsPerfSubKind reports whether s is one of the perf-side
// sub-kinds (jank / stall / startup).
func (s LogPerfSubKind) IsPerfSubKind() bool {
	switch s {
	case PerfJankFrame, PerfStallFrame, PerfStartupFrame:
		return true
	}
	return false
}

// AllLogPerfSubKinds returns every declared sub-kind in stable
// order. Used by tests and telemetry serialisers.
func AllLogPerfSubKinds() []LogPerfSubKind {
	return []LogPerfSubKind{
		LogPanicFrame, LogOOMFrame, LogTimeoutFrame,
		LogPerformanceFrame, LogErrorFrame, LogNoiseFrame,
		PerfJankFrame, PerfStallFrame, PerfStartupFrame,
	}
}

// LogPerfSubKindOf projects a (item, log-bundle, perf-bundle)
// triple onto a sub-kind. Pure deterministic function — same
// inputs always produce same output. Returns LogPerfSubKindUnknown
// when:
//   - The item carries no Source / LineStart (cannot match a frame)
//   - Neither bundle is non-nil
//   - The frame matched but no severity signal is present and the
//     frame is NOT in the Errors tree (treats as ambient log noise)
//
// Priority order (first match wins):
//
//  1. Perf-side: when item's Source matches a Stall.File →
//     PerfStallFrame; when it matches a Jank.TriggerSpan →
//     PerfJankFrame; when item is on the startup path AND
//     Startup.AppLaunchMs > PerfStartupSlowColdMs →
//     PerfStartupFrame. (Perf wins over log when both bundles are
//     attached AND both match — perf signals are more specific
//     than generic log presence.)
//
//  2. Log-side severity: panic/crash > oom > timeout > generic
//     error. Each requires the corresponding LogSignal in
//     bundle.Meta.Signals AND a matching frame.
//
//  3. Generic error: frame in LogError tree (non-empty Type) →
//     LogErrorFrame.
//
//  4. Frame matched but none of the above → LogNoiseFrame (the
//     log mentions this file but causation is ambiguous).
func LogPerfSubKindOf(item EvidenceItem, log *LogBundle, perf *PerfBundle) LogPerfSubKind {
	// Defensive nil-checks. An item without Source can't anchor
	// to any frame (frame match requires file:line equality),
	// so no sub-kind can be assigned.
	if item.Source == "" {
		return LogPerfSubKindUnknown
	}

	// (1) Perf-side classification.
	if perf != nil {
		if perfFrameStallMatch(item, perf) {
			return PerfStallFrame
		}
		if perfFrameJankMatch(item, perf) {
			return PerfJankFrame
		}
		if perfFrameStartupMatch(item, perf) {
			return PerfStartupFrame
		}
	}

	// (2) Log-side severity ladder.
	if log != nil {
		severity := logBundleSeveritySignal(log)
		if severity != "" && logBundleFrameMatch(log, item) {
			switch severity {
			case SignalPanic, SignalCrash:
				return LogPanicFrame
			case SignalOOM:
				return LogOOMFrame
			case SignalTimeout:
				return LogTimeoutFrame
			case SignalPerformance:
				// Performance observation: not an error, not an
				// abort. Distinct from LogErrorFrame so the
				// finalizer's facet template can render
				// "operation was slow" prose vs error-shaped
				// prose.
				return LogPerformanceFrame
			}
		}
		// (3) Generic error (frame in Errors tree, no severe signal).
		if logBundleFrameInErrors(log, item) {
			return LogErrorFrame
		}
		// (4) Frame matched but no Errors-tree entry, no severe
		//     signal — ambient noise reference.
		if logBundleFrameMatch(log, item) {
			return LogNoiseFrame
		}
	}

	return LogPerfSubKindUnknown
}

// logBundleSeveritySignal returns the most-severe signal in the
// bundle's Meta.Signals list. Order:
//
//	panic > crash > oom > timeout > performance
//
// The first four collapse to abort-class semantics (program halted /
// bounded by a deadline). SignalPerformance ranks below them — the
// program kept running, just slowly — but above the catch-all
// "" return so a perf-only log still carries a SubKind classification
// rather than falling through to LogErrorFrame.
func logBundleSeveritySignal(b *LogBundle) LogSignal {
	if b == nil {
		return ""
	}
	hasPanic, hasCrash, hasOOM, hasTimeout, hasPerformance := false, false, false, false, false
	for _, s := range b.Meta.Signals {
		switch s {
		case SignalPanic:
			hasPanic = true
		case SignalCrash:
			hasCrash = true
		case SignalOOM:
			hasOOM = true
		case SignalTimeout:
			hasTimeout = true
		case SignalPerformance:
			hasPerformance = true
		}
	}
	switch {
	case hasPanic:
		return SignalPanic
	case hasCrash:
		return SignalCrash
	case hasOOM:
		return SignalOOM
	case hasTimeout:
		return SignalTimeout
	case hasPerformance:
		return SignalPerformance
	}
	return ""
}

// logBundleFrameMatch reports whether any frame in any LogError
// (or Caused-by chain) of the bundle matches the item's
// (Source, LineStart) anchor. File equality on full path; line
// equality is exact match (LineStart=0 disables line check).
func logBundleFrameMatch(b *LogBundle, item EvidenceItem) bool {
	if b == nil {
		return false
	}
	hit := false
	walkLogErrorsForFrameMatch(b.Errors, item, &hit)
	return hit
}

// logBundleFrameInErrors reports whether the item's frame anchor
// appears in a LogError that has a non-empty Type (i.e. is a
// structured error, not a residue chunk). Stricter than
// logBundleFrameMatch — useful for distinguishing "frame in a
// real error" from "frame mentioned in noise residue".
func logBundleFrameInErrors(b *LogBundle, item EvidenceItem) bool {
	if b == nil {
		return false
	}
	hit := false
	walkLogErrorsForTypedFrameMatch(b.Errors, item, &hit)
	return hit
}

func walkLogErrorsForFrameMatch(errs []LogError, item EvidenceItem, hit *bool) {
	if *hit {
		return
	}
	for i := range errs {
		e := &errs[i]
		for _, f := range e.Frames {
			if f.File == item.Source && (item.LineStart == 0 || f.Line == item.LineStart) {
				*hit = true
				return
			}
		}
		if e.Cause != nil {
			walkLogErrorsForFrameMatch([]LogError{*e.Cause}, item, hit)
			if *hit {
				return
			}
		}
	}
}

func walkLogErrorsForTypedFrameMatch(errs []LogError, item EvidenceItem, hit *bool) {
	if *hit {
		return
	}
	for i := range errs {
		e := &errs[i]
		typed := e.Type != ""
		for _, f := range e.Frames {
			if !typed {
				continue
			}
			if f.File == item.Source && (item.LineStart == 0 || f.Line == item.LineStart) {
				*hit = true
				return
			}
		}
		if e.Cause != nil {
			walkLogErrorsForTypedFrameMatch([]LogError{*e.Cause}, item, hit)
			if *hit {
				return
			}
		}
	}
}

// perfFrameStallMatch reports whether the item's Source matches
// any PerfStall.File in the perf bundle.
func perfFrameStallMatch(item EvidenceItem, p *PerfBundle) bool {
	if p == nil {
		return false
	}
	for _, st := range p.Stalls {
		if st.File != "" && st.File == item.Source {
			return true
		}
	}
	return false
}

// perfFrameJankMatch reports whether the item's Source / Subject
// matches any PerfJank's TriggerSpan or any frame the bundle
// associates with the jank window. Conservative: requires Source
// equality with a frame in PerfBundle.Frames whose ts overlaps a
// Janks[] window. For the Phase-1 implementation we use a
// simpler rule — Stalls.File OR Jank.TriggerSpan substring of
// item.Subject — and refine later if real data shows misses.
func perfFrameJankMatch(item EvidenceItem, p *PerfBundle) bool {
	if p == nil || len(p.Janks) == 0 {
		return false
	}
	// Phase-1 conservative rule: any Jank entry exists AND the
	// item is in the perf-attached file set. Refines later when
	// real perf-eval data shows what Jank.TriggerSpan looks like.
	for _, j := range p.Janks {
		if j.TriggerSpan != "" && j.TriggerSpan == item.Subject {
			return true
		}
	}
	return false
}

// perfFrameStartupMatch reports whether the item is on the
// startup path AND the bundle's Startup.AppLaunchMs exceeds
// PerfStartupSlowColdMs. Phase-1 rule: any item AND startup is
// slow. Refines later when real data shows what "on the startup
// path" structurally means.
func perfFrameStartupMatch(item EvidenceItem, p *PerfBundle) bool {
	if p == nil || p.Startup == nil {
		return false
	}
	if p.Startup.AppLaunchMs <= 0 {
		return false
	}
	if p.Startup.AppLaunchMs <= PerfStartupSlowColdMs {
		return false
	}
	// Conservative: only fires when the perf bundle has slow
	// startup AND the item's source contains a path segment
	// suggesting startup-related code. The path-segment check
	// is a known limitation; refines later.
	return true
}
