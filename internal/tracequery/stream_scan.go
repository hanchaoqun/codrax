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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
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
	return streamScan(ctx, path, flavorHint, fn, false)
}

// StreamScanHeldFile streams one already-open, strongly identified regular
// file without reopening its pathname. It is the converter-owned publication
// validation lane: callers keep the generation handle alive, this function
// reuses StreamScan's single parser loop with O(1) event memory, and a
// per-physical-line byte limit fails before an unbounded line can be retained.
//
// Unlike StreamScan, this API deliberately does not discover siblings or
// inspect file.Name(). The caller owns the private parent-relative binding and
// must validate it before and after this call. The held generation itself is
// validated here with the strongest platform identity available.
func StreamScanHeldFile(ctx context.Context, file *os.File, displayPath string, flavorHint TraceFlavor, maxLineBytes int, fn func(Event) bool) (*Index, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if file == nil {
		return nil, fmt.Errorf("trace held stream scan: file is nil")
	}
	if fn == nil {
		return nil, fmt.Errorf("trace held stream scan: fn is nil")
	}
	displayPath = strings.TrimSpace(displayPath)
	if displayPath == "" {
		return nil, fmt.Errorf("trace held stream scan: display path is empty")
	}
	if maxLineBytes <= 0 || maxLineBytes > 16<<20 {
		return nil, fmt.Errorf("trace held stream scan: max line bytes must be in 1..%d", 16<<20)
	}
	openedIdentity, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, fmt.Errorf("trace held stream scan: capture source identity: %w", err)
	}
	if !openedIdentity.Strong() {
		return nil, fmt.Errorf("trace held stream scan: strong source identity is unavailable")
	}
	if !openedIdentity.Mode().IsRegular() || openedIdentity.Size() < 0 {
		return nil, fmt.Errorf("trace held stream scan: source is not a regular file")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("trace held stream scan: stat source: %w", err)
	}
	reader := io.NewSectionReader(file, 0, openedIdentity.Size())
	idx, scanErr := streamScanReader(ctx, displayPath, info, reader, flavorHint, fn, false, maxLineBytes, true)
	finalIdentity, identityErr := filegeneration.FromFile(file)
	if identityErr != nil {
		identityErr = fmt.Errorf("trace held stream scan: revalidate source identity: %w", identityErr)
	} else if !openedIdentity.SameVersion(finalIdentity) {
		identityErr = fmt.Errorf("trace held stream scan: source generation changed during validation")
	}
	if scanErr != nil {
		return nil, traceStreamScanJoin(scanErr, identityErr)
	}
	if identityErr != nil {
		return nil, identityErr
	}
	return idx, nil
}

// streamScanWithPairingAudit is the pairing-window discovery lane. It keeps
// StreamScan's single read/single parser, but also returns the existing
// bounded Block/Storage duration-order ledger on the metadata shell. Raw
// endpoints rejected by ParseLine must remain visible to discovery: otherwise
// deleting one physical marker could bridge two valid callbacks into a false
// collection window.
func streamScanWithPairingAudit(ctx context.Context, path string, flavorHint TraceFlavor, fn func(Event) bool) (*Index, error) {
	return streamScan(ctx, path, flavorHint, fn, true)
}

func streamScan(ctx context.Context, path string, flavorHint TraceFlavor, fn func(Event) bool, pairingAudit bool) (*Index, error) {
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
	if tracePathRequiresCompositeIndex(path) {
		return nil, fmt.Errorf("stream_scan requires a single physical artifact; %s has a tracebundle or sibling artifact universe, so use an indexed composite view", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return nil, err
	}
	openedIdentity := traceFileIdentityFromInfo(openedInfo)
	if !openedIdentity.SameVersion(traceFileIdentityFromInfo(info)) {
		return nil, fmt.Errorf("trace source identity changed before stream_scan opened the artifact")
	}
	info = openedInfo
	idx, scanErr := streamScanReader(ctx, path, info, f, flavorHint, fn, pairingAudit, 0, false)
	if scanErr != nil {
		return nil, scanErr
	}
	if err := validateTraceFileIdentityAfterRead(f, openedIdentity, "stream_scan"); err != nil {
		return nil, err
	}
	return idx, nil
}

func streamScanReader(ctx context.Context, path string, info os.FileInfo, source io.Reader, flavorHint TraceFlavor, fn func(Event) bool, pairingAudit bool, maxLineBytes int, canonicalGeneratedLines bool) (*Index, error) {
	applyHint := flavorHint != "" && flavorHint != TraceFlavorAuto
	idx := &Index{Path: path, Size: info.Size(), ModTime: info.ModTime()}
	intern := newStringInterner()
	flavor := newFlavorVote(path)
	reader := bufio.NewReaderSize(source, 256*1024)
	lastParsedTs := 0.0
	stopped := false
	var durationAudit *durationOrderTracker
	if pairingAudit {
		durationAudit = newDurationOrderTracker()
	}
	var scan lineScan
	for lineNo := 1; !stopped; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, readErr := readStreamScanPhysicalLine(reader, maxLineBytes)
		if len(line) > 0 {
			if canonicalGeneratedLines {
				if line[len(line)-1] != '\n' || strings.IndexByte(line, '\r') >= 0 {
					return nil, fmt.Errorf("trace held stream scan: non-canonical generated line terminator at line %d", lineNo)
				}
			}
			idx.LineCount = lineNo
			idx.ScannedLineCount = lineNo
			trimmed := strings.TrimRight(line, "\r\n")
			scan.reset(lineNo, trimmed)
			var rawPairingFailure *durationOrderViolation
			pairingRawCandidate := pairingAudit && durationOrderRawCandidate(trimmed)
			if pairingRawCandidate {
				if failure := durationEndpointRawValidationFailureScan(&scan); failure != nil {
					if _, _, relevant := pairingDiscoveryFamilyForDuration(failure.Family); relevant {
						rawPairingFailure = failure
					}
				}
			}
			// Keep the streaming metadata shell on the same raw trace-mark
			// integrity contract as BuildIndex. This audit must run before
			// the parse: malformed endpoint rows with an invalid emitter,
			// CPU, or timestamp cannot materialize as Event, but they are still
			// authoritative fail-closed witnesses for B/E/S/F/G/H/N/I state
			// machines. traceMarkValidationFailureScan uses the precise
			// corrupted-row remnant gate, so ordinary unparsed prose that merely
			// quotes a mark payload remains an unparsed-quality observation,
			// never a pairing poison. The shared lineScan memo keeps this audit
			// plus the parse at ONE header match per line (perf audit #21).
			if failure := traceMarkValidationFailureScan(&scan); failure != nil {
				failure.SourcePath = path
				appendTraceMarkIntegrityFailure(idx, *failure)
			}
			if failure := blockedReasonValidationFailureScan(&scan); failure != nil {
				failure.SourcePath = path
				appendBlockedReasonIntegrityFailure(idx, *failure)
			}
			flavor.observeRawLine(trimmed)
			panicsBefore := idx.ParseLinePanics
			ev, ok := safeParseLineScan(&scan, intern, idx)
			if !ok {
				if pairingAudit {
					failure := rawPairingFailure
					if failure == nil && pairingRawCandidate {
						failure = durationEndpointRejectedRowFailureScan(&scan)
					}
					if failure != nil {
						if _, _, relevant := pairingDiscoveryFamilyForDuration(failure.Family); relevant {
							failure.SourcePath = path
							failure.Fields = uniqueSortedStrings(append(failure.Fields, "parser_rejected_row"))
							appendDurationOrderFailure(idx, *failure)
						}
					}
				}
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
			if pairingAudit {
				for _, failure := range durationAudit.observeAll(ev) {
					if _, _, relevant := pairingDiscoveryFamilyForDuration(failure.Family); !relevant || failure.Issue == "endpoint_parse_incomplete" {
						continue
					}
					failure.SourcePath = path
					appendDurationOrderFailure(idx, failure)
				}
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
	if pairingAudit {
		for family, capped := range durationAudit.capped {
			if _, _, relevant := pairingDiscoveryFamilyForDuration(family); capped && relevant {
				if idx.durationOrderFailuresCapped == nil {
					idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
				}
				idx.durationOrderFailuresCapped[family] = true
			}
		}
		if idx.ClockRegressions > 0 {
			for family, capped := range durationAudit.pairingHistoryCapped {
				if _, _, relevant := pairingDiscoveryFamilyForDuration(family); capped && relevant {
					if idx.durationOrderFailuresCapped == nil {
						idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
					}
					idx.durationOrderFailuresCapped[family] = true
				}
			}
		}
	}
	idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals = flavor.result()
	return idx, nil
}

func readStreamScanPhysicalLine(reader *bufio.Reader, maxLineBytes int) (string, error) {
	if maxLineBytes <= 0 {
		return reader.ReadString('\n')
	}
	line := make([]byte, 0, min(maxLineBytes+1, 256*1024))
	for {
		fragment, err := reader.ReadSlice('\n')
		contentBudget := maxLineBytes
		if len(fragment) > 0 && fragment[len(fragment)-1] == '\n' {
			contentBudget++
		}
		if len(line) > contentBudget-len(fragment) {
			return "", fmt.Errorf("trace held stream scan: physical line exceeds %d bytes", maxLineBytes)
		}
		line = append(line, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return string(line), err
	}
}

func traceStreamScanJoin(primary, secondary error) error {
	return errors.Join(primary, secondary)
}
