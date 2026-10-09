package tracequery

import (
	"context"
	"errors"
	"fmt"
)

// StreamProcessMeasurements scans the full frozen source before selecting
// intervals. A time-centered event index would discard valid carry-in rows.
func StreamProcessMeasurements(ctx context.Context, path string, q Query) (Result, error) {
	return streamProcessMeasurements(ctx, path, q, 65536, 32<<20)
}

func StreamPreferredFrameRate(ctx context.Context, path string, q Query) (Result, error) {
	q.View = ViewPreferredFrameRate
	return streamProcessMeasurements(ctx, path, q, 65536, 32<<20)
}

func streamProcessMeasurements(ctx context.Context, path string, q Query, recordLimit, byteLimit int) (Result, error) {
	if CanonicalViewName(q.View) != ViewProcessMeasurements && CanonicalViewName(q.View) != ViewPreferredFrameRate {
		return Result{}, fmt.Errorf("process measurement stream requires view=%s", ViewProcessMeasurements)
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
		return Result{}, fmt.Errorf("process_measurements source selection: %w", err)
	}
	var events []Event
	bytes, overflow := 0, false
	idx, err := resourceStackScanSource(ctx, selection, physical, q.TraceFlavorHint, func(ev Event) bool {
		if ev.Type == EventProcessMeasureInterval && !overflow {
			if len(events) >= recordLimit || len(ev.FieldText) > byteLimit-bytes {
				overflow, events = true, nil
			} else {
				events = append(events, ev)
				bytes += len(ev.FieldText)
			}
		}
		return true // retention exhaustion is not EOF or a complete empty result
	})
	if err != nil {
		return Result{}, err
	}
	source.LocalLineCount, source.EventCount = idx.LineCount, idx.ParsedKnown
	source.timestampOrder, source.clockRegressions = idx.TimestampOrder, idx.ClockRegressions
	idx.Path, idx.Size, idx.Events = selection.indexPath, selection.universe.totalBytes, events
	idx.TraceArtifacts = []TraceArtifactSource{source}
	idx.Caveats = append(idx.Caveats, selection.caveats...)
	if selection.bundleSet {
		idx.Caveats = append(idx.Caveats, traceBundleCaveats(selection.bundle)...)
	}
	q = normalizeQuery(idx, q).WithRunContext(ctx)
	if q.View == ViewPreferredFrameRate {
		p := buildPreferredFrameRate(idx, q)
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if overflow {
			p = &PreferredFrameRateResult{Status: "unavailable", SourcePath: idx.Path, Window: ProcessMeasurementsWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}, TargetScope: q.TargetScope, TargetPID: q.PID, Caveats: []string{PreferredFrameRateTeaching, "complete_scan_retention_limit: no partial frequency population published"}}
		}
		if err := errors.Join(selection.validateIndex(idx), selection.validate(ctx)); err != nil {
			return Result{}, err
		}
		if err := selection.close(); err != nil {
			return Result{}, err
		}
		return Result{View: q.View, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines, EventCount: idx.ParsedKnown, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions, TraceArtifacts: idx.TraceArtifacts, PreferredFrameRate: p, Caveats: append(idx.Caveats, p.Caveats...)}, nil
	}
	p := buildProcessMeasurements(idx, q)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if overflow {
		p.Status = "unavailable"
		p.Rows = nil
		p.TotalRows, p.OmittedRows, p.UnpositionedRows = 0, 0, 0
		p.Caveats = append(p.Caveats, "complete_scan_retention_limit: source scanned to EOF; no partial process measurement inventory published")
	}
	if err := errors.Join(selection.validateIndex(idx), selection.validate(ctx)); err != nil {
		return Result{}, err
	}
	if err := selection.close(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewProcessMeasurements, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd,
		LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines, EventCount: idx.ParsedKnown,
		ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions, TraceArtifacts: idx.TraceArtifacts,
		ProcessMeasurements: p, Caveats: append(idx.Caveats, p.Caveats...)}, nil
}
