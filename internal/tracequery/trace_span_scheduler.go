package tracequery

import (
	"math"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceSpanSchedulerStates measures only the synchronous marker owner's
// scheduler states inside that marker's query-clipped interval. It neither
// selects an investigation focus nor explains why the owner slept. AccountedMs
// is the sum of the five known scheduler lanes; incomplete coverage must not be
// mistaken for zero time in the missing/unclassified part of the interval.
type TraceSpanSchedulerStates struct {
	SourcePath        string                                 `json:"source_path"`
	Thread            ThreadRef                              `json:"thread"`
	Window            TimeWindow                             `json:"window"`
	Coverage          string                                 `json:"coverage"`
	MeasurementDomain *types.TraceSchedulerMeasurementDomain `json:"measurement_domain,omitempty"`
	RunningMs         float64                                `json:"running_ms"`
	RunnableMs        float64                                `json:"runnable_ms"`
	SleepMs           float64                                `json:"sleep_ms"`
	DStateMs          float64                                `json:"d_state_ms"`
	IOWaitMs          float64                                `json:"io_wait_ms"`
	SleepIOWaitMs     float64                                `json:"sleep_io_wait_ms"`
	AccountedMs       float64                                `json:"accounted_ms"`
	HeadState         *TimelineHeadState                     `json:"head_state,omitempty"`
	IntegrityFailure  string                                 `json:"integrity_failure,omitempty"`
	Caveats           []string                               `json:"caveats,omitempty"`
}

// Matches is a display-integrity check on a typed carrier, never a causal gate.
// Equal numeric totals alone cannot bind an account to another source/owner.
func (s *TraceSpanSchedulerStates) Matches(sourcePath, subject string, start, end float64) bool {
	if s == nil || s.Thread.PID <= 0 || s.SourcePath == "" || sourcePath == "" ||
		filepath.Clean(s.SourcePath) != filepath.Clean(sourcePath) || threadLabel(s.Thread) != subject ||
		s.Window.StartTs != start || s.Window.EndTs != end || end <= start {
		return false
	}
	for _, value := range []float64{start, end, s.RunningMs, s.RunnableMs, s.SleepMs, s.DStateMs, s.IOWaitMs, s.SleepIOWaitMs, s.AccountedMs} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return false
		}
	}
	sum := s.RunningMs + s.RunnableMs + s.SleepMs + s.DStateMs + s.IOWaitMs
	if math.Abs(sum-s.AccountedMs) > 1e-6 || sum > (end-start)*1000+1e-6 || s.SleepIOWaitMs > s.SleepMs+1e-6 {
		return false
	}
	switch s.Coverage {
	case "unavailable":
		return s.AccountedMs == 0
	case "complete", "partial":
		if s.IntegrityFailure != "" || s.MeasurementDomain == nil || s.AccountedMs <= 0 {
			return false
		}
		domain := s.MeasurementDomain
		return domain.TargetTID == s.Thread.PID && domain.WindowStartTs == start && domain.WindowEndTs == end &&
			(s.Coverage != "complete" || math.Abs(sum-(end-start)*1000) <= 1e-6)
	default:
		return false
	}
}

// Only the already-bounded ordinary synchronous span display is enriched. The
// full marker inventory, semantic-span lane, query target, selected window and
// ranking accounts stay untouched. A single physical source is required: a
// marker's header TID is not a cross-capture scheduler identity.
func stampTraceSpanSchedulerStates(idx *Index, q Query, spans []TraceSpanSummary) {
	if idx == nil || idx.RelationScoped || len(idx.TraceArtifacts) != 1 || !idx.TraceArtifacts[0].CausalCompatible ||
		idx.TraceArtifacts[0].VirtualLineBase != 0 ||
		filepath.Clean(idx.TraceArtifacts[0].SourcePath) != filepath.Clean(idx.Path) {
		return
	}
	var cache *chainQueryCache
	for i := range spans {
		span := &spans[i]
		if q.runCancel.sample() {
			return
		}
		if span.Kind != "sync" || span.SemanticClass != "" || span.Thread.PID <= 0 ||
			span.EndLine <= span.StartLine || span.EndTs <= span.StartTs ||
			filepath.Clean(span.SourcePath) != filepath.Clean(idx.Path) {
			continue
		}
		if cache == nil {
			// One per-PID event index for the bounded span set. The existing
			// indexed timeline event traversal and coverage machinery retain
			// scheduler-head, lifecycle, malformed-input and clock-order gates.
			cache = newChainQueryCache(idx, q.runCancel)
		}
		local := q
		local.PID, local.Thread, local.ThreadInput = span.Thread.PID, "", ""
		local.TargetScope = TargetScopeThread
		local.TimeStart, local.TimeEnd = span.StartTs, span.EndTs
		local.TimeStartSet, local.TimeEndSet = true, true
		tl := traceSpanSchedulerTimeline(cache, q, local, span.Thread)
		measured := &TraceSpanSchedulerStates{
			SourcePath: span.SourcePath, Thread: span.Thread, Window: tl.Window,
			Coverage: "unavailable", HeadState: tl.HeadState,
			IntegrityFailure: tl.IntegrityFailure, Caveats: append([]string(nil), tl.Caveats...),
		}
		// Build from the exact existing timeline account, including the S-state
		// IO refinement overlay. Do not infer a state from the marker name,
		// marker elapsed time, global Top-N totals, or its payload process ID.
		account := buildTargetWindowStateAccount(idx, tl, tl.IntegrityFailure == "", span.Thread, tl.Window, nil)
		if account != nil && tl.MeasurementDomain != nil {
			measured.MeasurementDomain = types.CloneTraceSchedulerMeasurementDomain(tl.MeasurementDomain)
			measured.RunningMs, measured.RunnableMs = account.RunningMs, account.RunnableMs
			measured.SleepMs, measured.DStateMs, measured.IOWaitMs = account.SleepMs, account.DStateMs, account.IOWaitMs
			measured.SleepIOWaitMs = account.SleepIOWaitMs
			measured.AccountedMs = account.RunningMs + account.RunnableMs + account.SleepMs + account.DStateMs + account.IOWaitMs
			if measured.AccountedMs > 0 {
				measured.Coverage = "partial"
				if math.Abs(measured.AccountedMs-(span.EndTs-span.StartTs)*1000) <= 1e-6 &&
					(tl.HeadState == nil || tl.HeadState.Status != "unknown") {
					measured.Coverage = "complete"
				}
			}
		}
		if !q.runCancel.fired() && measured.Matches(span.SourcePath, threadLabel(span.Thread), span.StartTs, span.EndTs) {
			span.SchedulerStates = measured
		}
	}
}

// A windowed index owns a proven checkpoint at the original query boundary,
// not at every later marker boundary. Re-querying only at the marker start can
// therefore lose a governing scheduler state whose physical event is outside
// retained padding. Reuse the owner's original-window timeline once, then
// intersect its actual intervals with each marker. Never crop/split totals or
// install derived checkpoints into the shared index.
func traceSpanSchedulerTimeline(cache *chainQueryCache, original, local Query, thread ThreadRef) TimelineResult {
	if cache != nil && cache.idx != nil && cache.idx.Windowed && original.LineStart == 0 && original.LineEnd == 0 &&
		original.TimeStart > 0 && original.TimeStart <= local.TimeStart && original.TimeEnd >= local.TimeEnd {
		owner := original
		owner.PID, owner.Thread, owner.ThreadInput = thread.PID, "", ""
		owner.TargetScope = TargetScopeThread
		base := cache.timeline(owner, thread)
		// A broad-window integrity failure is not a verdict about every child:
		// let the existing marker-local lane decide those cases independently.
		// An unknown broad head is never upgraded by clipping a carried state.
		if base.IntegrityFailure == "" && base.MeasurementDomain != nil && base.HeadState != nil && base.HeadState.Status != "unknown" {
			clipped := TimelineResult{
				Thread: thread, Window: queryResultTimeWindow(local),
				Intervals: clampIntervals(base.Intervals, local),
				Caveats:   append([]string(nil), base.Caveats...),
				HeadState: &TimelineHeadState{Status: "unknown", BoundaryTs: local.TimeStart, Reason: "marker_head_state_unclassified"},
			}
			for _, interval := range clipped.Intervals {
				if interval.StartTs != local.TimeStart || interval.EndTs <= local.TimeStart || interval.State == StateUnknown {
					continue
				}
				clipped.HeadState = &TimelineHeadState{
					Status: "recovered", BoundaryTs: local.TimeStart, State: interval.State,
					ActualStartTs: interval.ActualStartTs, SourceLine: interval.StartLine,
				}
				break
			}
			clipped.MeasurementDomain = buildTimelineMeasurementDomain(local, clipped)
			return clipped
		}
	}
	return cache.timeline(local, thread)
}
