package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type blockEndpointPhase uint8

const (
	blockEndpointStart blockEndpointPhase = iota + 1
	blockEndpointDone
)

const (
	blockEndpointFamilyRQ  = "block_rq"
	blockEndpointFamilyBIO = "block_bio"
)

// blockLatencyEndpoint is the single closed-set admission gate for elapsed
// block latency. EventType intentionally stays wire-compatible and therefore
// groups insert/getrq/bio rows; the exact raw event name is the precise signal
// that decides whether a row is an endpoint and which family it belongs to.
// Name-less hand-built Events retain the historical rq compatibility lane.
func blockLatencyEndpoint(ev Event) (family string, phase blockEndpointPhase, ok bool) {
	switch strings.ToLower(strings.TrimSpace(ev.Name)) {
	case "block_rq_issue":
		return blockEndpointFamilyRQ, blockEndpointStart, true
	case "block_rq_complete":
		return blockEndpointFamilyRQ, blockEndpointDone, true
	case "block_bio_queue":
		return blockEndpointFamilyBIO, blockEndpointStart, true
	case "block_bio_complete":
		return blockEndpointFamilyBIO, blockEndpointDone, true
	case "block_rq_insert", "block_getrq":
		return "", 0, false
	case "":
		// Compatibility for package/external callers that construct Event by
		// hand. Production ParseLine always sets Name and cannot enter here.
		switch ev.Type {
		case EventBlockIssue:
			return blockEndpointFamilyRQ, blockEndpointStart, true
		case EventBlockComplete:
			return blockEndpointFamilyRQ, blockEndpointDone, true
		}
	}
	return "", 0, false
}

// parseBlockRequestValidated preserves presence independently from the numeric
// value. ParseInt is deliberately strict: an overflowed decimal cannot become
// sector 0 or len 0. Sector zero is legal; length must be positive.
func parseBlockRequestValidated(fields string) (dev, op string, sector, length int64, valid bool) {
	trimmed := strings.TrimSpace(fields)
	m := blockRequestRE.FindStringSubmatch(trimmed)
	if len(m) != 5 {
		m = blockSimpleRE.FindStringSubmatch(trimmed)
	}
	if len(m) != 5 {
		return "", "", 0, 0, false
	}
	sector, sectorErr := strconv.ParseInt(m[3], 10, 64)
	length, lengthErr := strconv.ParseInt(m[4], 10, 64)
	dev = strings.TrimSpace(m[1])
	op = strings.TrimSpace(m[2])
	valid = sectorErr == nil && lengthErr == nil && dev != "" && op != "" && sector >= 0 &&
		((length > 0 && length <= (1<<63-1)/512) || blockOperationAllowsZeroLength(op, sector, length))
	if !valid {
		return dev, op, 0, 0, false
	}
	return dev, op, sector, length, true
}

func blockRequestIdentityValid(ev Event) bool {
	blk := ev.BlockIOFields
	if blk == nil {
		return false
	}
	if blk.IdentityParsed {
		return blk.IdentityValid && blk.Dev != "" && blk.Op != "" && blk.Sector >= 0 &&
			((blk.Len > 0 && blk.Len <= (1<<63-1)/512) || blockOperationAllowsZeroLength(blk.Op, blk.Sector, blk.Len))
	}
	// Compatibility lane for manually constructed Events. A zero sector is
	// accepted because its presence cannot be represented in the old shape.
	return blk.Dev != "" && blk.Op != "" && blk.Sector >= 0 &&
		((blk.Len > 0 && blk.Len <= (1<<63-1)/512) || blockOperationAllowsZeroLength(blk.Op, blk.Sector, blk.Len))
}

// blockOperationAllowsZeroLength admits the kernel's exact flush request
// shape observed in production (`FS ... 0 + 0`). A flush carries no data
// sectors but its issue/complete interval is still real elapsed latency.
// Ordinary read/write/discard rows keep the positive-length requirement;
// broad prefix/substring matching would let malformed operations mint a key.
func blockOperationAllowsZeroLength(op string, sector, length int64) bool {
	if sector != 0 || length != 0 {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(op)) {
	case "F", "FS":
		return true
	default:
		return false
	}
}

// tracePairingSourceIdentity maps the index-global event coordinate back to
// exactly one physical artifact. A populated provenance ledger that cannot
// resolve the row fails closed. Path-less hand-built indexes use a stable
// compatibility sentinel and are never confused with production artifacts.
func tracePairingSourceIdentity(idx *Index, ev Event) (string, bool) {
	if idx == nil {
		return "", false
	}
	spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
	if len(spans) == 1 && strings.TrimSpace(spans[0].SourcePath) != "" {
		return spans[0].SourcePath, true
	}
	if len(idx.TraceArtifacts) > 0 {
		return "", false
	}
	if strings.TrimSpace(idx.Path) != "" {
		return idx.Path, true
	}
	return "<compat-index>", true
}

func blockKey(ev Event) string {
	family, _, ok := blockLatencyEndpoint(ev)
	if !ok || !blockRequestIdentityValid(ev) {
		return ""
	}
	blk := ev.BlockIOFields
	return fmt.Sprintf("%s/%s/%s/%d/%d", family, blk.Dev, blk.Op, blk.Sector, blk.Len)
}

func blockPairingKey(idx *Index, ev Event) (string, string, bool) {
	identity := blockKey(ev)
	if identity == "" {
		return "", "", false
	}
	source, ok := tracePairingSourceIdentity(idx, ev)
	if !ok {
		return "", "", false
	}
	return source + "\x00" + identity, source, true
}

type blockPairingLane struct {
	depth        int
	ambiguous    bool
	cohortStarts int
	start        Event
	acc          *blockPairingAccumulator
}

type blockPairingAccumulator struct {
	item           StorageLatencySummary
	totalLatencyMs float64
}

type blockPairingResult struct {
	latencies []IOLatencySummary
	summaries []StorageLatencySummary
	caveats   []string
}

// computeBlockIOLatencies pairs only exact endpoint families and never assumes
// FIFO ordering for repeated coarse identities. Once a lane reaches depth two,
// its whole cohort is ambiguous and emits no duration; the lane recovers only
// after depth returns to zero.
func computeBlockIOLatencies(idx *Index, q Query, max int) blockPairingResult {
	if idx == nil {
		return blockPairingResult{}
	}
	lanes := map[string]*blockPairingLane{}
	accs := map[string]*blockPairingAccumulator{}
	var out blockPairingResult
	invalidIdentity := 0
	unresolvedSourceRows := 0
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			if idx.TimestampOrder.AllowsTimeEndEarlyStop() {
				break
			}
			continue
		}
		family, phase, endpoint := blockLatencyEndpoint(ev)
		if !endpoint {
			continue
		}
		if phase == blockEndpointDone && q.TimeStart > 0 && ev.Ts < q.TimeStart {
			continue
		}
		identity := blockKey(ev)
		if identity == "" {
			invalidIdentity++
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedSourceRows++
			continue
		}
		laneKey := source + "\x00" + identity
		blk := ev.BlockIOFields
		accKey := strings.Join([]string{source, family, blk.Dev, blk.Op}, "\x00")
		acc := accs[accKey]
		if acc == nil {
			acc = &blockPairingAccumulator{item: StorageLatencySummary{
				SourcePath: source,
				Layer:      "block",
				Event:      family,
				Dev:        blk.Dev,
				Operation:  blk.Op,
				Thread:     threadRefFromEvent(ev),
				LineStart:  ev.Line,
				LineEnd:    ev.Line,
				StartTs:    ev.Ts,
				EndTs:      ev.Ts,
				Example:    clampString(ev.FieldText, 160),
			}}
			accs[accKey] = acc
		}
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, ev.Line)
		if ev.Ts < acc.item.StartTs || acc.item.StartTs == 0 {
			acc.item.StartTs = ev.Ts
		}
		if ev.Ts > acc.item.EndTs {
			acc.item.EndTs = ev.Ts
		}
		lane := lanes[laneKey]
		if lane == nil {
			lane = &blockPairingLane{}
			lanes[laneKey] = lane
		}
		switch phase {
		case blockEndpointStart:
			acc.item.Count++
			if lane.depth == 0 {
				lane.depth = 1
				lane.cohortStarts = 1
				lane.start = ev
				lane.acc = acc
				continue
			}
			lane.depth++
			lane.cohortStarts++
			if !lane.ambiguous {
				lane.ambiguous = true
				lane.acc.item.AmbiguousCohortCount++
				lane.start = Event{}
			}
		case blockEndpointDone:
			if lane.depth == 0 {
				acc.item.Count++
				acc.item.UnpairedDoneCount++
				continue
			}
			if lane.ambiguous {
				lane.depth--
				if lane.depth == 0 {
					lane.acc.item.PairingSuppressedCount += lane.cohortStarts
					*lane = blockPairingLane{}
				}
				continue
			}
			start := lane.start
			startAcc := lane.acc
			*lane = blockPairingLane{}
			if ev.Ts < start.Ts {
				startAcc.item.PairingSuppressedCount++
				continue
			}
			durationMs := (ev.Ts - start.Ts) * 1000
			startBlk := start.BlockIOFields
			out.latencies = append(out.latencies, IOLatencySummary{
				SourcePath:     source,
				Dev:            startBlk.Dev,
				Op:             startBlk.Op,
				Sector:         startBlk.Sector,
				Len:            startBlk.Len,
				IssueThread:    threadRefFromEvent(start),
				CompleteThread: threadRefFromEvent(ev),
				IssueTs:        start.Ts,
				CompleteTs:     ev.Ts,
				DurationMs:     durationMs,
				IssueLine:      start.Line,
				CompleteLine:   ev.Line,
			})
			startAcc.item.PairedCount++
			startAcc.totalLatencyMs += durationMs
			if durationMs > startAcc.item.MaxLatencyMs {
				startAcc.item.MaxLatencyMs = durationMs
			}
			startAcc.item.Bytes += startBlk.Len * 512
		}
	}

	for _, lane := range lanes {
		if lane.depth == 0 || lane.acc == nil {
			continue
		}
		if lane.ambiguous {
			acc := lane.acc
			suppressed := lane.cohortStarts
			*lane = blockPairingLane{}
			acc.item.PairingSuppressedCount += suppressed
		} else {
			lane.acc.item.UnpairedStartCount++
			*lane = blockPairingLane{}
		}
	}

	sort.SliceStable(out.latencies, func(i, j int) bool {
		if out.latencies[i].DurationMs != out.latencies[j].DurationMs {
			return out.latencies[i].DurationMs > out.latencies[j].DurationMs
		}
		return out.latencies[i].IssueLine < out.latencies[j].IssueLine
	})
	if max > 0 && len(out.latencies) > max {
		out.latencies = out.latencies[:max]
	}

	var ambiguous, suppressed, unpairedStart, unpairedDone int
	for _, acc := range accs {
		if acc.item.PairedCount > 0 {
			acc.item.AvgLatencyMs = acc.totalLatencyMs / float64(acc.item.PairedCount)
		}
		acc.item.Summary = storageLatencySummaryText(acc.item)
		ambiguous += acc.item.AmbiguousCohortCount
		suppressed += acc.item.PairingSuppressedCount
		unpairedStart += acc.item.UnpairedStartCount
		unpairedDone += acc.item.UnpairedDoneCount
		out.summaries = append(out.summaries, acc.item)
	}
	if invalidIdentity > 0 {
		out.caveats = append(out.caveats, fmt.Sprintf("block_io_pairing_identity_invalid=true; rows=%d; missing/overflowed dev/op/sector/len were excluded from latency pairing", invalidIdentity))
	}
	if unresolvedSourceRows > 0 {
		out.caveats = append(out.caveats, fmt.Sprintf("block_io_pairing_provenance_unresolved=true; rows=%d; endpoints without exactly one physical source artifact were excluded", unresolvedSourceRows))
	}
	if ambiguous > 0 {
		out.caveats = append(out.caveats, fmt.Sprintf("block_io_pairing_ambiguous=true; cohorts=%d pairing_suppressed=%d; overlapping identical endpoint identities were withheld as whole cohorts instead of FIFO-guessed", ambiguous, suppressed))
	}
	if unpairedStart > 0 || unpairedDone > 0 {
		out.caveats = append(out.caveats, fmt.Sprintf("block_io_pairing_unpaired=true; unpaired_start=%d unpaired_done=%d; elapsed latency was emitted only for complete exact-family pairs", unpairedStart, unpairedDone))
	}
	return out
}

func storageLatencySummaryText(item StorageLatencySummary) string {
	return fmt.Sprintf("layer=%s event=%s dev=%s op=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d max_latency=%.3fms",
		item.Layer, item.Event, item.Dev, item.Operation, item.Count, item.PairedCount,
		item.UnpairedStartCount, item.UnpairedDoneCount, item.AmbiguousCohortCount,
		item.PairingSuppressedCount, item.MaxLatencyMs)
}
