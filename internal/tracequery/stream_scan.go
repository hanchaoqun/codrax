package tracequery

// stream_scan.go — TDIAG B2 (§28.13, real_trace_campaign_20260705.md,
// 2026-07-09): the exported full-trace streaming event iterator. It reuses
// the SAME line loop + safeParseLine path as StreamEventSearch (zero second
// parser — anti-parallel-subsystem red line) and never materializes an event
// index, so it is NOT subject to the per-index event budget
// (IndexEventLimitError is structurally unreachable here): a whole-file
// format census over a GB trace streams in O(file) with O(1) memory.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// StreamScan streams every parsed event of the trace file to fn in file
// order. fn returning false stops the scan early (the returned shell then
// covers only the scanned prefix — its ScannedLineCount says how far).
//
// The returned *Index is a metadata SHELL, deliberately carrying NO events
// (Events stays nil — nothing accumulates, no budget applies): path/size,
// line + parse-quality counters (LineCount / ScannedLineCount /
// UnparsedLines / ParseLinePanics / ClockRegressions / ParsedKnown), the
// FirstTs/LastTs envelope, the flavor vote and the typed UnparsedSamples
// face (TDIAG B4) — exactly the quality surfaces the indexed build exposes,
// so streaming consumers (tracediag format census) keep every honesty
// disclosure without a second parser or a second read.
//
// flavorHint semantics: a concrete hint (non-auto) is applied to each event's
// priority classes via the same applyPriorityFlavor path StreamEventSearch
// uses; auto/empty delivers raw parsed events byte-identical to Index.Events
// (per-event flavor resolution needs the full-file vote, which a streaming
// callback cannot retro-apply — the shell's TraceFlavor carries the vote for
// callers that finish the scan).
func StreamScan(ctx context.Context, path string, flavorHint TraceFlavor, fn func(Event) bool) (*Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return nil, fmt.Errorf("trace stream scan: fn is nil")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trace path is empty")
	}
	path = canonicalTraceIndexPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	applyHint := flavorHint != "" && flavorHint != TraceFlavorAuto
	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	reader := bufio.NewReaderSize(f, 256*1024)
	lastParsedTs := 0.0
	stopped := false
	for lineNo := 1; !stopped; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			flavor.observeRawLine(trimmed)
			panicsBefore := idx.ParseLinePanics
			ev, ok := safeParseLine(lineNo, trimmed, intern, idx)
			if !ok {
				if trimmed != "" {
					if idx.ParseLinePanics == panicsBefore {
						idx.UnparsedLines++
					}
					idx.recordUnparsedSample(lineNo, trimmed)
				}
				goto nextLine
			}
			// Parse-quality counters mirror the indexed path (same discipline
			// as StreamEventSearch — the census consumers key honesty
			// disclosures on them).
			if prev := lastParsedTs; prev > 0 && ev.Ts > 0 && ev.Ts < prev {
				idx.ClockRegressions++
			}
			if ev.Ts > 0 {
				lastParsedTs = ev.Ts
			}
			if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
				idx.FirstTs = ev.Ts
			}
			if ev.Ts > idx.LastTs {
				idx.LastTs = ev.Ts
			}
			if ev.Type != EventUnknown {
				idx.ParsedKnown++
			}
			flavor.observeEvent(ev)
			if applyHint {
				ev = applyPriorityFlavor(ev, flavorHint)
			}
			if !fn(ev) {
				stopped = true
			}
		}
	nextLine:
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, readErr
		}
	}
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	return idx, nil
}
