package tracequery

import (
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func resourceStackSingleSource(idx *Index) bool {
	if idx.Windowed || idx.RelationScoped || len(idx.TraceArtifacts) > 1 {
		return false
	}
	if len(idx.TraceArtifacts) == 0 {
		return true
	}
	s := idx.TraceArtifacts[0]
	if s.SourcePath == idx.Path {
		return true
	}
	return s.sourceIdentity.Initialized() && s.CausalCompatible && s.ClockAlignment == TraceClockAlignmentIdentity && s.TimeDomain != "" && sameTraceTimeDomain(s.TimeDomain, s.CanonicalTimeDomain) && traceClockMapIsIdentity(s.ClockOffsetSec, s.ClockSlope) && s.VirtualLineBase == 0 && s.LocalLineCount == idx.LineCount
}

func buildResourceStack(idx *Index, q Query) *ResourceStackResult {
	p := &ResourceStackResult{SourcePath: idx.Path, Window: ResourceStackWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd, EndInclusive: q.timeEndBackfilled}, Status: "unavailable", TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope}
	if p.TargetScope == "" {
		p.TargetScope = TargetScopeThread
	}
	fail := func(reason string) *ResourceStackResult { p.Reason = reason; return p }
	if !resourceStackSingleSource(idx) {
		return fail("requires_complete_identity_mapped_single_source")
	}
	if idx.ResourceStackMalformed > 0 {
		return fail("malformed_resource_stack_carrier")
	}
	if !isFiniteTraceNumber(q.TimeStart) || !isFiniteTraceNumber(q.TimeEnd) || q.TimeEnd < q.TimeStart || q.TimeEnd == q.TimeStart && !q.timeEndBackfilled {
		return fail("requires_finite_positive_window")
	}
	if q.LineStart != 0 || q.LineEnd != 0 || q.Pattern != "" || len(q.Patterns) != 0 || q.SpanName != "" || len(q.EventTypes) != 0 || len(q.EventNames) != 0 || len(q.EventFieldFilters) != 0 || len(q.TraceMarkActions) != 0 {
		return fail("requires_time_and_owner_selectors_only")
	}
	byID := map[int64]*ResourceStackEvent{}
	frames := map[int64][]ResourceStackFrame{}
	times := map[int64]int64{}
	seen := false
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return fail("scan_canceled")
		}
		if ev.Type != EventResourceStack {
			continue
		}
		seen = true
		r, ok := tracewire.ParseResourceStack(ev.FieldText)
		if !ok {
			return fail("malformed_resource_stack_carrier")
		}
		if ts, exists := times[r.EventRowID]; exists && ts != r.TimestampNS {
			return fail("conflicting_resource_event_timestamp")
		}
		times[r.EventRowID] = r.TimestampNS
		if r.Event != nil {
			if byID[r.EventRowID] != nil {
				return fail("duplicate_resource_event_identity")
			}
			byID[r.EventRowID] = &ResourceStackEvent{RowID: r.EventRowID, Line: ev.Line, TimestampNS: r.TimestampNS, Source: *r.Event, UnwindCompleteness: "unknown"}
		} else {
			frames[r.EventRowID] = append(frames[r.EventRowID], ResourceStackFrame{Line: ev.Line, ResourceFrame: *r.Frame})
		}
	}
	if !seen {
		return fail("no_resource_stack_observations")
	}
	for id, rows := range frames {
		if byID[id] == nil || len(rows) != byID[id].Source.FrameCount {
			return fail("resource_frame_binding_incomplete")
		}
	}
	var selected []*ResourceStackEvent
	for id, e := range byID {
		if len(frames[id]) != e.Source.FrameCount {
			return fail("resource_frame_binding_incomplete")
		}
		ts := float64(e.TimestampNS) / 1e9
		if ts < q.TimeStart || ts > q.TimeEnd || ts == q.TimeEnd && !q.timeEndBackfilled {
			continue
		}
		if q.PID != 0 {
			target := e.Source.TID
			if q.TargetScope == TargetScopeProcess {
				target = e.Source.PID
			}
			if target != q.PID {
				continue
			}
		}
		if q.Thread != "" && q.PID == 0 && q.Thread != e.Source.Thread {
			continue
		}
		e.Frames = frames[id]
		if !summarizeResourceStackEvent(e) {
			return fail("duplicate_resource_frame_identity")
		}
		selected = append(selected, e)
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].TimestampNS != selected[j].TimestampNS {
			return selected[i].TimestampNS < selected[j].TimestampNS
		}
		return selected[i].RowID < selected[j].RowID
	})
	p.Status = "available"
	p.MatchedEvents = len(selected)
	limit := q.Limit
	if limit <= 0 {
		limit = 40
	}
	if limit > 40 {
		limit = 40
	}
	for i, e := range selected {
		if i >= limit {
			p.OmittedEvents++
			continue
		}
		if len(e.Frames) > 128 {
			e.OmittedFrames = len(e.Frames) - 128
			e.Frames = e.Frames[:128]
		}
		p.Events = append(p.Events, *e)
	}
	p.Caveats = []string{"Frames are linked through an observed resource event's same-capture owner and callchain; frame rows themselves have no owner or execution timestamp.", "Source depth is preserved without declaring a business leaf. Contiguous observed depths do not prove that upstream unwinding reached the true stack end.", "Unknown symbols and NULL scalar fields stay unknown. Resource end time is metadata, not thread running or blocking time; no CPU, leak verdict or response cause is inferred."}
	return p
}

func summarizeResourceStackEvent(e *ResourceStackEvent) bool {
	e.SourceFramesComplete = e.Source.StackStatus == "observed" || e.Source.StackStatus == "no_frames"
	e.DepthStatus = "unavailable"
	if e.Source.StackStatus != "observed" {
		return true
	}
	seenRows := map[int64]bool{}
	depths := map[int64]bool{}
	maxDepth := int64(-1)
	for _, f := range e.Frames {
		if seenRows[f.RowID] {
			return false
		}
		seenRows[f.RowID] = true
		d, ok := f.Depth.Integer()
		if !ok || d < 0 || d > math.MaxInt32 {
			e.InvalidDepths++
		} else {
			if depths[d] {
				e.DuplicateDepths++
			}
			depths[d] = true
			if d > maxDepth {
				maxDepth = d
			}
		}
		if f.Symbol.Status != "known" || strings.TrimSpace(f.Symbol.Value) == "" {
			e.UnknownSymbols++
		}
	}
	e.MissingDepths = maxDepth + 1 - int64(len(depths))
	e.DepthStatus = "contiguous_observed"
	if e.MissingDepths > 0 || e.InvalidDepths > 0 {
		e.DepthStatus = "incomplete"
	}
	if e.DuplicateDepths > 0 {
		e.DepthStatus = "ambiguous"
	}
	sort.SliceStable(e.Frames, func(i, j int) bool {
		a, ao := e.Frames[i].Depth.Integer()
		b, bo := e.Frames[j].Depth.Integer()
		if ao != bo {
			return ao
		}
		if a != b {
			return a < b
		}
		return e.Frames[i].RowID < e.Frames[j].RowID
	})
	return true
}

// ValidResourceStack validates counts and full-detail consistency while
// allowing a handoff to omit display rows without changing source totals.
func ValidResourceStack(p ResourceStackResult) bool {
	if p.TargetScope != TargetScopeThread && p.TargetScope != TargetScopeProcess {
		return false
	}
	if p.SourcePath == "" || !isFiniteTraceNumber(p.Window.StartTs) || !isFiniteTraceNumber(p.Window.EndTs) || p.Window.EndTs < p.Window.StartTs || p.Window.EndTs == p.Window.StartTs && !p.Window.EndInclusive || p.TargetPID < 0 {
		return false
	}
	if p.Status == "unavailable" {
		return p.Reason != "" && p.MatchedEvents == 0 && p.OmittedEvents == 0 && len(p.Events) == 0
	}
	if p.Status != "available" || p.Reason != "" || p.MatchedEvents < 0 || p.OmittedEvents < 0 || p.MatchedEvents != len(p.Events)+p.OmittedEvents {
		return false
	}
	ids := map[int64]bool{}
	for _, e := range p.Events {
		complete := e.Source.StackStatus == "observed" || e.Source.StackStatus == "no_frames"
		if ids[e.RowID] || e.Line <= 0 || e.TimestampNS < 0 || !e.Source.Valid() || e.SourceFramesComplete != complete || e.UnwindCompleteness != "unknown" || e.OmittedFrames < 0 || len(e.Frames)+e.OmittedFrames != e.Source.FrameCount {
			return false
		}
		ids[e.RowID] = true
		ts := float64(e.TimestampNS) / 1e9
		if ts < p.Window.StartTs || ts > p.Window.EndTs || ts == p.Window.EndTs && !p.Window.EndInclusive {
			return false
		}
		if p.TargetPID != 0 {
			pid := e.Source.TID
			if p.TargetScope == TargetScopeProcess {
				pid = e.Source.PID
			}
			if pid != p.TargetPID {
				return false
			}
		}
		if p.TargetPID == 0 && p.TargetThread != "" && p.TargetThread != e.Source.Thread {
			return false
		}
		if e.MissingDepths < 0 || e.DuplicateDepths < 0 || e.InvalidDepths < 0 || e.UnknownSymbols < 0 || e.UnknownSymbols > e.Source.FrameCount || e.DuplicateDepths+e.InvalidDepths > e.Source.FrameCount {
			return false
		}
		for _, f := range e.Frames {
			if f.Line <= 0 || !f.Valid() {
				return false
			}
		}
		copy := e
		copy.Frames = append([]ResourceStackFrame(nil), e.Frames...)
		copy.MissingDepths, copy.DuplicateDepths, copy.InvalidDepths, copy.UnknownSymbols = 0, 0, 0, 0
		if !summarizeResourceStackEvent(&copy) {
			return false
		}
		if e.OmittedFrames == 0 && (copy.DepthStatus != e.DepthStatus || copy.MissingDepths != e.MissingDepths || copy.DuplicateDepths != e.DuplicateDepths || copy.InvalidDepths != e.InvalidDepths || copy.UnknownSymbols != e.UnknownSymbols) {
			return false
		}
		if copy.DuplicateDepths > e.DuplicateDepths || copy.InvalidDepths > e.InvalidDepths || copy.UnknownSymbols > e.UnknownSymbols {
			return false
		}
		switch e.DepthStatus {
		case "unavailable", "contiguous_observed", "incomplete", "ambiguous":
		default:
			return false
		}
	}
	return true
}
