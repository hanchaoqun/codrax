package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const traceDBPostvalidationCoverageTable = "tracequery_build_index"

const (
	traceDBPostvalidationCanceled          = "tracequery_postvalidation_canceled"
	traceDBPostvalidationCountMismatch     = "tracequery_postvalidation_count_mismatch"
	traceDBPostvalidationGenerationInvalid = "tracequery_postvalidation_generation_invalid"
	traceDBPostvalidationHeaderInvalid     = "tracequery_postvalidation_header_invalid"
	traceDBPostvalidationParsePanic        = "tracequery_postvalidation_parse_panic"
	traceDBPostvalidationScanFailed        = "tracequery_postvalidation_scan_failed"
	traceDBPostvalidationClockRegression   = "tracequery_postvalidation_clock_regression"
	traceDBPostvalidationUnknownOwnedRow   = "tracequery_postvalidation_unknown_owned_row"
	traceDBPostvalidationUnparsedOwnedRow  = "tracequery_postvalidation_unparsed_owned_row"
	traceDBPostvalidationZeroRows          = "tracequery_postvalidation_zero_rows"
)

func newTraceDBPostvalidationCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "trace_cross_validation",
		Table:  traceDBPostvalidationCoverageTable,
		Role:   "tracequery_cross_validation",
		Found:  true,
	}
}

// validateSealedSystraceWithTraceQuery is the converter-owned query-ready
// admission gate. It consumes the exact held generation, never reopens the
// public path, and keeps the tracequery parser's event memory at O(1).
//
// The SQL writer always prefixes its rows with the fixed standard systrace
// header. Header bytes are therefore checked separately on the same handle;
// only those exact comment rows may account for UnparsedLines.
func validateSealedSystraceWithTraceQuery(ctx context.Context, source *sealedConversionFile, displayPath string, expectedRows int) (coverage TraceDBCoverage, resultErr error) {
	start := time.Now()
	defer func() {
		traceDBSetCoverageElapsed(&coverage, start)
	}()
	coverage = newTraceDBPostvalidationCoverage()
	if ctx == nil {
		ctx = context.Background()
	}
	fail := func(reason string, cause ...error) error {
		coverage.Error = reason
		var underlying error
		if len(cause) > 0 {
			underlying = cause[0]
		}
		return &traceDBOutputInvariantError{Reason: reason, Cause: underlying}
	}
	if expectedRows <= 0 {
		return coverage, fail(traceDBPostvalidationZeroRows)
	}
	if source == nil {
		coverage.Found = false
		return coverage, fail(traceDBPostvalidationGenerationInvalid)
	}
	if err := ctx.Err(); err != nil {
		coverage.Error = traceDBPostvalidationCanceled
		return coverage, err
	}
	if err := source.Validate(); err != nil {
		coverage.Found = false
		return coverage, fail(traceDBPostvalidationGenerationInvalid, err)
	}

	headerLines := strings.Count(systraceHeader, "\n")
	callbackCount := 0
	callbackOverflow := false
	parsedHeaderRow := false
	var idx *tracequery.Index
	operationReason := ""
	operationErr := source.withOpenFile(func(file *os.File) error {
		if err := ctx.Err(); err != nil {
			operationReason = traceDBPostvalidationCanceled
			return err
		}
		header := make([]byte, len(systraceHeader))
		n, err := file.ReadAt(header, 0)
		if err != nil || n != len(header) || !bytes.Equal(header, []byte(systraceHeader)) {
			operationReason = traceDBPostvalidationHeaderInvalid
			if err != nil {
				return err
			}
			return errors.New("generated systrace header bytes differ from the fixed writer contract")
		}
		idx, err = tracequery.StreamScanHeldFile(
			ctx,
			file,
			displayPath,
			tracequery.TraceFlavorAuto,
			maxTraceDBSystraceLineBytes,
			func(event tracequery.Event) bool {
				if event.Line <= headerLines {
					parsedHeaderRow = true
					return false
				}
				if callbackCount == math.MaxInt {
					callbackOverflow = true
					return false
				}
				callbackCount++
				return true
			},
		)
		if err != nil && operationReason == "" {
			operationReason = traceDBPostvalidationScanFailed
		}
		return err
	})
	if operationErr != nil {
		if operationReason == traceDBPostvalidationCanceled || errors.Is(operationErr, context.Canceled) || errors.Is(operationErr, context.DeadlineExceeded) {
			coverage.Error = traceDBPostvalidationCanceled
			return coverage, operationErr
		}
		if operationReason == "" {
			operationReason = traceDBPostvalidationScanFailed
		}
		return coverage, fail(operationReason, operationErr)
	}
	if err := source.Validate(); err != nil {
		coverage.Found = false
		return coverage, fail(traceDBPostvalidationGenerationInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		coverage.Error = traceDBPostvalidationCanceled
		return coverage, err
	}
	if idx == nil {
		return coverage, fail(traceDBPostvalidationScanFailed)
	}
	if parsedHeaderRow {
		return coverage, fail(traceDBPostvalidationHeaderInvalid)
	}

	coverage.RowsRead = idx.ScannedLineCount
	coverage.RowsEmitted = callbackCount
	coverage.ColumnsPresent = append(coverage.ColumnsPresent,
		fmt.Sprintf("header_lines=%d", headerLines),
		fmt.Sprintf("expected_rows=%d", expectedRows),
		fmt.Sprintf("parsed_known=%d", idx.ParsedKnown),
		fmt.Sprintf("callback_count=%d", callbackCount),
	)
	if idx.FirstTs != 0 || idx.LastTs != 0 {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent,
			fmt.Sprintf("first_ts=%.6f", idx.FirstTs),
			fmt.Sprintf("last_ts=%.6f", idx.LastTs),
		)
	}
	if idx.TraceFlavor != "" {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent, "trace_flavor="+string(idx.TraceFlavor))
	}

	if idx.ParseLinePanics != 0 {
		return coverage, fail(traceDBPostvalidationParsePanic)
	}
	if idx.ClockRegressions != 0 {
		return coverage, fail(traceDBPostvalidationClockRegression)
	}
	if idx.UnparsedLines != headerLines {
		return coverage, fail(traceDBPostvalidationUnparsedOwnedRow)
	}
	if callbackCount != idx.ParsedKnown {
		return coverage, fail(traceDBPostvalidationUnknownOwnedRow)
	}
	if callbackOverflow || expectedRows > math.MaxInt-headerLines {
		return coverage, fail(traceDBPostvalidationCountMismatch)
	}
	expectedLines := headerLines + expectedRows
	if idx.Size != source.Size() || len(idx.Events) != 0 || idx.LineCount != expectedLines ||
		idx.ScannedLineCount != expectedLines || idx.ParsedKnown != expectedRows || callbackCount != expectedRows {
		return coverage, fail(traceDBPostvalidationCountMismatch)
	}
	return coverage, nil
}
