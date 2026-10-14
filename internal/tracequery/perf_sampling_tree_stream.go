package tracequery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// StreamPerfSamplingTree reaches EOF on every frozen source. Its retention
// ceiling is independent of display limits; a ceiling hit never publishes a
// partial denominator. Non-perf events retain lifecycle metadata needed by the
// existing exact perf identity ledger, not a second thread/name matcher.
func StreamPerfSamplingTree(ctx context.Context, path string, q Query) (Result, error) {
	return streamPerfSamplingTree(ctx, path, q, 65536, 64<<20)
}

func streamPerfSamplingTree(ctx context.Context, path string, q Query, recordLimit, byteLimit int) (Result, error) {
	if CanonicalViewName(q.View) != ViewPerfSamplingTree {
		return Result{}, fmt.Errorf("sampling stream requires view=%s", ViewPerfSamplingTree)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	sel, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer sel.close()
	var sources []TraceArtifactSource
	if len(sel.artifactSpecs) == 0 {
		entry, ok := sel.universe.entry(sel.indexPath)
		if !ok {
			return Result{}, fmt.Errorf("sampling source absent from frozen universe")
		}
		sources = []TraceArtifactSource{singleTraceArtifactSourceWithIdentity(sel.indexPath, entry.identity, 0, 0)}
	} else {
		for _, s := range sel.artifactSpecs {
			sources = append(sources, s.source)
		}
	}
	idx := &Index{Path: sel.indexPath, Size: sel.universe.totalBytes}
	var events []Event
	bytes, overflow := 0, false
	clockUnavailable := false
	for _, source := range sources {
		entry, ok := sel.universe.entry(source.SourcePath)
		if !ok {
			return Result{}, fmt.Errorf("sampling source absent from frozen universe")
		}
		if len(sel.artifactSpecs) > 0 && (!source.CausalCompatible || source.ClockAlignment != TraceClockAlignmentIdentity || source.TimeDomain == "" || !sameTraceTimeDomain(source.TimeDomain, source.CanonicalTimeDomain) || !traceClockMapIsIdentity(source.ClockOffsetSec, source.ClockSlope)) {
			clockUnavailable = true
		}
		base := idx.LineCount
		part, err := samplingScanSource(ctx, sel, source.SourcePath, q.TraceFlavorHint, func(ev Event) bool {
			if ev.Type != EventPerfSample {
				return true
			}
			if overflow {
				return true
			}
			cost := samplingRetainedEventBytes(ev)
			if len(events) >= recordLimit || cost > byteLimit-bytes {
				overflow = true
				events = nil
				return true
			}
			ev.Line += base
			samplingOwnRetainedStrings(&ev)
			events = append(events, ev)
			bytes += cost
			return true
		}, func(line HeldLineObservation) {
			failure := schedulerRowValidationFailure(line.Line, line.Text)
			if failure == nil && !line.Parsed {
				failure = schedulerRejectedRowFailure(line.Line, line.Text)
			}
			if failure != nil {
				failure.SourcePath = source.SourcePath
				failure.LocalLine = line.Line
				failure.Line += base
				appendSchedulerRowIntegrityFailure(idx, *failure)
			}
		})
		if err != nil {
			return Result{}, err
		}
		source.VirtualLineBase = base
		source.LocalLineCount = part.LineCount
		source.EventCount = part.ParsedKnown
		source.SourceBytes = entry.identity.Size()
		source.SourceModUnixNano = entry.identity.ModUnixNano()
		source.sourceIdentity = entry.identity
		source.timestampOrder = part.TimestampOrder
		source.clockRegressions = part.ClockRegressions
		idx.TraceArtifacts = append(idx.TraceArtifacts, source)
		if part.hasTimestampBounds() {
			idx.observeTimestampBounds(part.FirstTs)
			idx.observeTimestampBounds(part.LastTs)
		}
		idx.LineCount += part.LineCount
		idx.ScannedLineCount += part.ScannedLineCount
		idx.ParsedKnown += part.ParsedKnown
		idx.UnparsedLines += part.UnparsedLines
		idx.ParseLinePanics += part.ParseLinePanics
		idx.ClockRegressions += part.ClockRegressions
	}
	// Source-name filtering alone is unsafe: any anchored event can be the
	// first reappearance after an exact task death. Replay the existing tracker
	// for sampled candidate TIDs and retain only boundary/first-presence rows.
	// This bounds dense unrelated scheduler traffic without hiding a lifecycle.
	if !overflow {
		candidates := map[int]bool{}
		for _, ev := range events {
			if tid, ok := perfIdentityCandidateTIDFromEvent(ev); ok {
				candidates[tid] = true
			}
		}
		for _, source := range idx.TraceArtifacts {
			tracker := newThreadIncarnationTracker()
			_, err := resourceStackScanSource(ctx, sel, source.SourcePath, q.TraceFlavorHint, func(ev Event) bool {
				if overflow || len(candidates) == 0 {
					return true
				}
				first := false
				for _, pid := range schedulerEventRolePIDs(ev) {
					if candidates[pid] {
						if _, seen := tracker.seen[pid]; !seen {
							first = true
						}
					}
				}
				pid, reset := schedulerLifecycleResetPID(ev)
				reset = reset && candidates[pid]
				conflicts := tracker.observeAllForPIDSet(ev, candidates)
				if ev.Type == EventPerfSample || (!first && !reset && len(conflicts) == 0) {
					return true
				}
				ev = compactPerfGenerationPrefixEvent(ev).asEvent()
				cost := samplingRetainedEventBytes(ev)
				if len(events) >= recordLimit || cost > byteLimit-bytes {
					overflow = true
					events = nil
					return true
				}
				ev.Line += source.VirtualLineBase
				samplingOwnRetainedStrings(&ev)
				events = append(events, ev)
				bytes += cost
				return true
			})
			if err != nil {
				return Result{}, err
			}
		}
		sort.SliceStable(events, func(i, j int) bool { return events[i].Line < events[j].Line })
	}
	idx.Events = events
	q = normalizeQuery(idx, q).WithRunContext(ctx)
	var p *PerfSamplingTreeResult
	if overflow || clockUnavailable {
		reason := "complete_scan_retention_budget_exceeded"
		if clockUnavailable {
			reason = "source_clock_mapping_unavailable_query_child_independently"
		}
		p = &PerfSamplingTreeResult{SourcePath: idx.Path, Window: ResourceStackWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd, EndInclusive: q.timeEndBackfilled}, TargetPID: q.PID, TargetThread: q.Thread, TargetScope: TargetScopeThread, Status: "unavailable", Reason: reason}
	} else {
		if err := prebuildPerfIdentityLedger(ctx, idx); err != nil {
			return Result{}, err
		}
		p = buildPerfSamplingTree(idx, q)
	}
	caveats := append(append([]string(nil), sel.caveats...), p.Caveats...)
	var selection *ThreadSelectorResolution
	if p.Status == "available" {
		selection = threadSelectorResolutionForQuery(idx, q)
		if selection != nil {
			caveats = append(caveats, "Thread selector diagnostics cover retained sampling and sampled-thread lifecycle metadata only; missing names or candidates do not establish absence from the full capture. Numeric TID is preserved; name candidates are discovery hints, not automatic retargeting.")
			// The streaming index intentionally omits unrelated thread metadata.
			// No observed name on either side is insufficient to certify a
			// mismatch. Explicitly unknown sample identities were scrubbed by
			// native admission and must not be reconstructed from the header.
			if selection.Selected.Comm == "" && len(selection.NameCandidates) == 0 {
				selection = nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := errors.Join(sel.validateIndex(idx), sel.validate(ctx)); err != nil {
		return Result{}, err
	}
	if err := sel.close(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewPerfSamplingTree, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, EventCount: idx.ParsedKnown, UnparsedLineCount: idx.UnparsedLines, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions, TraceArtifacts: idx.TraceArtifacts, PerfSamplingTree: p, ThreadSelection: selection, Caveats: caveats}, nil
}

// Streaming parsing uses a non-retaining interner. Charge every retained
// string, including private carriers (FramesV1 is intentionally absent from
// Event JSON), rather than the truncated FieldText preview. Repeated aliases
// are conservatively charged again. Reflection here is a field census, not a
// semantic admission rule, and keeps newly added string metadata accounted.
func samplingRetainedEventBytes(ev Event) int {
	stringsIn := func(value any) int {
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return 0
			}
			v = v.Elem()
		}
		n := 0
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.Kind() == reflect.String {
				n += f.Len()
			}
		}
		return n
	}
	return int(eventSizeBytes+eventSideTableBytes(&ev)) + stringsIn(ev) + stringsIn(ev.PerfFields) + stringsIn(ev.ConstraintFields)
}

// Parser substrings may share the complete raw row backing storage. Detach
// the retained strings so the byte census above is also the owned heap size.
func samplingOwnRetainedStrings(ev *Event) {
	for _, value := range []any{ev, ev.PerfFields, ev.ConstraintFields} {
		v := reflect.ValueOf(value)
		if v.IsNil() {
			continue
		}
		v = v.Elem()
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.Kind() == reflect.String && f.CanSet() {
				f.SetString(strings.Clone(f.String()))
			}
		}
	}
}

func samplingScanSource(ctx context.Context, selection *traceIndexSelection, path string, flavor TraceFlavor, observe func(Event) bool, lineObserver func(HeldLineObservation)) (*Index, error) {
	entry, ok := selection.universe.entry(path)
	if !ok {
		return nil, fmt.Errorf("sampling source missing")
	}
	f, identity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if !entry.identity.SameVersion(identity) {
		return nil, fmt.Errorf("sampling source changed before scan")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	idx, err := streamScanReader(ctx, path, info, io.NewSectionReader(f, 0, identity.Size()), flavor, observe, lineObserver, false, 0, false)
	if err != nil {
		return nil, traceReadErrorAfterIdentity(f, identity, "sampling scan", err)
	}
	if err := validateTraceFileIdentityAfterRead(f, identity, "sampling scan"); err != nil {
		return nil, err
	}
	return idx, nil
}
