package tracequery

import (
	"context"
	"errors"
	"fmt"
)

func StreamMeasurements(ctx context.Context, path string, q Query) (Result, error) {
	return streamMeasurements(ctx, path, q, 65536, 32<<20)
}
func streamMeasurements(ctx context.Context, path string, q Query, recordLimit, byteLimit int) (Result, error) {
	if CanonicalViewName(q.View) != ViewMeasurements {
		return Result{}, fmt.Errorf("requires view=%s", ViewMeasurements)
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
	bytes, overflow := 0, false
	idx, err := resourceStackScanSource(ctx, selection, physical, q.TraceFlavorHint, func(ev Event) bool {
		if ev.Type == EventMeasureInterval && !overflow {
			if len(events) >= recordLimit || len(ev.FieldText) > byteLimit-bytes {
				overflow, events = true, nil
			} else {
				events = append(events, ev)
				bytes += len(ev.FieldText)
			}
		}
		return true
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
	p := buildMeasurements(idx, q)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if overflow {
		p = &MeasurementsResult{Status: "unavailable", SourcePath: idx.Path, Window: ProcessMeasurementsWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled}, Caveats: []string{MeasurementsTeaching, "complete_scan_retention_limit: source scanned to EOF; no partial measurement population published"}}
	}
	if err := errors.Join(selection.validateIndex(idx), selection.validate(ctx)); err != nil {
		return Result{}, err
	}
	if err := selection.close(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewMeasurements, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines, EventCount: idx.ParsedKnown, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions, TraceArtifacts: idx.TraceArtifacts, Measurements: p, Caveats: append(idx.Caveats, p.Caveats...)}, nil
}
