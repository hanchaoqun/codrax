package tracequery

import (
	"context"
	"errors"
	"fmt"
)

// StreamTransactionHandoffs inspects the complete held source before matching.
// Query/display bounds never shrink the uniqueness or lifecycle universe.
func StreamTransactionHandoffs(ctx context.Context, path string, q Query) (Result, error) {
	return streamTransactionHandoffs(ctx, path, q, 65536, 32<<20)
}

func streamTransactionHandoffs(ctx context.Context, path string, q Query, recordLimit, byteLimit int) (Result, error) {
	if CanonicalViewName(q.View) != ViewTransactionHandoffs {
		return Result{}, fmt.Errorf("requires view=%s", ViewTransactionHandoffs)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	selection, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer selection.close()
	physical, source, err := resourceStackStreamSource(selection)
	if err != nil {
		return Result{}, err
	}
	var events []Event
	var conflicts []threadIncarnationConflict
	tracker := newThreadIncarnationTracker()
	bytes, keyReferences, overflow := 0, 0, false
	observe := func(ev Event) bool {
		if overflow {
			return true
		}
		for _, conflict := range tracker.observeAll(ev, 0) {
			if len(conflicts) >= recordLimit {
				overflow = true
				events = nil
				return true
			}
			conflicts = append(conflicts, conflict)
		}
		if len(tracker.seen) > recordLimit {
			overflow = true
			events = nil
			return true
		}
		if role, keys, _ := transactionProtocol(ev); role != "" {
			if len(keys) > recordLimit-keyReferences {
				overflow = true
				events = nil
				return true
			}
			keyReferences += len(keys)
			if len(events) >= recordLimit || len(ev.SpanName)+len(ev.Comm) > byteLimit-bytes {
				overflow = true
				events = nil
				return true
			}
			events = append(events, ev)
			bytes += len(ev.SpanName) + len(ev.Comm)
		}
		return true
	}
	idx, err := resourceStackScanSource(ctx, selection, physical, q.TraceFlavorHint, observe)
	if err != nil {
		return Result{}, err
	}
	source.LocalLineCount, source.EventCount = idx.LineCount, idx.ParsedKnown
	source.timestampOrder, source.clockRegressions = idx.TimestampOrder, idx.ClockRegressions
	idx.Path, idx.Size = selection.indexPath, selection.universe.totalBytes
	idx.TraceArtifacts = []TraceArtifactSource{source}
	idx.Events = events
	idx.threadIncarnationFailures = conflicts
	q = normalizeQuery(idx, q).WithRunContext(ctx)
	p := buildTransactionHandoffs(idx, q)
	if overflow {
		p = &TransactionHandoffsResult{Status: "unavailable", Reason: "complete_source_retention_limit", SourcePath: idx.Path, Window: RenderingCandidatesWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled}, TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope, Caveats: []string{"Full source scanned to EOF, but protocol or identity retention exceeded bounded capacity; no prefix handoff count or uniqueness is published."}}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := errors.Join(selection.validateIndex(idx), selection.validate(ctx)); err != nil {
		return Result{}, err
	}
	if err := selection.close(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewTransactionHandoffs, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines, EventCount: idx.ParsedKnown, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions, TraceArtifacts: idx.TraceArtifacts, TransactionHandoffs: p, Caveats: append(idx.Caveats, p.Caveats...)}, nil
}
