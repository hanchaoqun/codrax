package hitraceconv

import (
	"context"
	"fmt"
	"os"
)

type traceDBSystraceExport struct {
	Artifact          Artifact
	Coverage          []TraceDBCoverage
	TraceCoverage     []TraceDBCoverage
	EventsWritten     int
	OutputBytes       int64
	FirstTimestampSec float64
	LastTimestampSec  float64
}

func exportTraceDBToSystrace(ctx context.Context, dbPath, output string) (traceDBSystraceExport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureOutputDoesNotExist(output); err != nil {
		return traceDBSystraceExport{}, err
	}
	tdb, err := openTraceDB(ctx, dbPath)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	defer tdb.close()

	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	sinkClosed := false
	defer func() {
		if !sinkClosed {
			sink.cleanup()
		}
	}()

	var coverage []TraceDBCoverage
	schedulerCoverage, err := exportTraceDBSchedulerFamilies(ctx, tdb, sink)
	coverage = append(coverage, schedulerCoverage...)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	extendedCoverage, err := exportTraceDBExtendedFamilies(ctx, tdb, sink)
	coverage = append(coverage, extendedCoverage...)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	if sink.stats.RowsAccepted == 0 {
		coverage = append(coverage, sink.stats.coverage())
		sink.cleanup()
		sinkClosed = true
		return traceDBSystraceExport{Coverage: coverage}, nil
	}

	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	stats, writeErr := sink.writeTo(ctx, out)
	sinkClosed = true
	closeErr := out.Close()
	if writeErr != nil {
		_ = os.Remove(output)
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(output)
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, closeErr
	}
	info, err := os.Stat(output)
	if err != nil {
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	coverage = append(coverage, stats.coverage())
	result := traceDBSystraceExport{
		Artifact: Artifact{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     info.Size(),
			Converter: traceStreamerConverter,
			Caveats:   []string{"generated from trace_streamer SQLite DB rows"},
		},
		Coverage:          coverage,
		EventsWritten:     stats.RowsWritten,
		OutputBytes:       info.Size(),
		FirstTimestampSec: float64(stats.FirstTSNS) / 1e9,
		LastTimestampSec:  float64(stats.LastTSNS) / 1e9,
	}
	result.TraceCoverage = append(result.TraceCoverage, validateSystraceWithTraceQuery(ctx, output))
	if result.EventsWritten == 0 {
		return result, fmt.Errorf("trace DB sorter accepted rows but wrote none")
	}
	return result, nil
}
