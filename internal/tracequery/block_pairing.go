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

type blockRequestIdentity struct {
	Family string
	Dev    string
	Op     string
	Sector int64
	Len    int64
}

func blockIdentity(ev Event) (blockRequestIdentity, bool) {
	family, _, ok := blockLatencyEndpoint(ev)
	if !ok || !blockRequestIdentityValid(ev) {
		return blockRequestIdentity{}, false
	}
	blk := ev.BlockIOFields
	return blockRequestIdentity{
		Family: family,
		Dev:    blk.Dev,
		Op:     blk.Op,
		Sector: blk.Sector,
		Len:    blk.Len,
	}, true
}

func (id blockRequestIdentity) laneKey() string {
	return strings.Join([]string{
		id.Family,
		id.Dev,
		id.Op,
		strconv.FormatInt(id.Sector, 10),
		strconv.FormatInt(id.Len, 10),
	}, "\x00")
}

// tracePairingSourceIdentity maps the index-global event coordinate back to
// exactly one physical artifact. A populated provenance ledger that cannot
// resolve the row fails closed. Path-less hand-built indexes use a stable
// compatibility sentinel and are never confused with production artifacts.
func tracePairingSourceIdentity(idx *Index, ev Event) (string, bool) {
	if idx == nil {
		return "", false
	}
	// Allocation-free single-line resolution (perf audit #25): identical
	// verdicts to ResolveArtifactSpans(ev.Line, ev.Line) with len(spans)==1.
	if i, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, ev.Line); ok {
		if strings.TrimSpace(idx.TraceArtifacts[i].SourcePath) != "" {
			return idx.TraceArtifacts[i].SourcePath, true
		}
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
	identity, ok := blockIdentity(ev)
	if !ok {
		return ""
	}
	return identity.laneKey()
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
	cohort pairingCohortState
	source string
	family string
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
		// Line scopes retain their historical exact-row semantics.  Time
		// scopes replay the complete available prefix/suffix so an already
		// closed pre-window pair cannot survive as false carry-in and an
		// interval crossing either boundary can be adjudicated as one cohort.
		if (q.LineStart > 0 || q.LineEnd > 0) && !eventLineInWindow(ev, q) {
			continue
		}
		family, phase, endpoint := blockLatencyEndpoint(ev)
		if !endpoint {
			continue
		}
		identity := blockKey(ev)
		if identity == "" {
			if pairingEventInsideQuery(ev, q) {
				invalidIdentity++
			}
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			if pairingEventInsideQuery(ev, q) {
				unresolvedSourceRows++
			}
			continue
		}
		laneKey := source + "\x00" + identity
		lane := lanes[laneKey]
		if lane == nil {
			lane = &blockPairingLane{source: source, family: family}
			lanes[laneKey] = lane
		}
		var transition pairingCohortTransition
		switch phase {
		case blockEndpointStart:
			transition = lane.cohort.observeStart(ev)
		case blockEndpointDone:
			transition = lane.cohort.observeDone(ev)
		}
		accountBlockPairingTransition(&out, accs, lane, transition, q)
		// A closed cohort left its zero state behind; drop the lane so map
		// residency tracks CONCURRENT opens, not distinct identities seen
		// (perf audit #25). A later same-identity start recreates an
		// identical fresh lane.
		if lane.cohort.depth == 0 {
			delete(lanes, laneKey)
		}
	}

	for _, lane := range lanes {
		transition := lane.cohort.finishEOF()
		if !pairingOpenCohortIntersectsIndex(transition.first, idx, q) {
			continue
		}
		accountBlockOpenTransition(accs, lane, transition)
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

func blockPairingAccumulatorFor(accs map[string]*blockPairingAccumulator, source, family string, ev Event) *blockPairingAccumulator {
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	key := strings.Join([]string{source, family, blk.Dev, blk.Op}, "\x00")
	if acc := accs[key]; acc != nil {
		return acc
	}
	acc := &blockPairingAccumulator{item: StorageLatencySummary{
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
	accs[key] = acc
	return acc
}

func observeBlockPairingEnvelope(acc *blockPairingAccumulator, first, last Event) {
	if acc == nil {
		return
	}
	for _, ev := range []Event{first, last} {
		applyLineRange(&acc.item.LineStart, &acc.item.LineEnd, ev.Line)
		if acc.item.StartTs == 0 || ev.Ts < acc.item.StartTs {
			acc.item.StartTs = ev.Ts
		}
		if ev.Ts > acc.item.EndTs {
			acc.item.EndTs = ev.Ts
		}
	}
}

func accountBlockPairingTransition(out *blockPairingResult, accs map[string]*blockPairingAccumulator, lane *blockPairingLane, transition pairingCohortTransition, q Query) {
	if lane == nil {
		return
	}
	if transition.unpairedDone {
		if !pairingEventInsideQuery(transition.last, q) {
			return
		}
		acc := blockPairingAccumulatorFor(accs, lane.source, lane.family, transition.last)
		acc.item.Count++
		acc.item.UnpairedDoneCount++
		observeBlockPairingEnvelope(acc, transition.last, transition.last)
		return
	}
	if !transition.cohortClosed || !pairingIntervalIntersectsQuery(transition.first, transition.last, q) {
		return
	}
	acc := blockPairingAccumulatorFor(accs, lane.source, lane.family, transition.first)
	observeBlockPairingEnvelope(acc, transition.first, transition.last)
	acc.item.Count += transition.cohortStarts
	if transition.ambiguous {
		acc.item.AmbiguousCohortCount++
		acc.item.PairingSuppressedCount += transition.cohortStarts
		return
	}
	start, done := transition.pairStart, transition.last
	if done.Ts < start.Ts {
		acc.item.PairingSuppressedCount++
		return
	}
	durationMs := (done.Ts - start.Ts) * 1000
	startBlk := start.BlockIOFields
	if startBlk == nil {
		startBlk = &BlockIOFields{}
	}
	out.latencies = append(out.latencies, IOLatencySummary{
		SourcePath:     lane.source,
		Dev:            startBlk.Dev,
		Op:             startBlk.Op,
		Sector:         startBlk.Sector,
		Len:            startBlk.Len,
		IssueThread:    threadRefFromEvent(start),
		CompleteThread: threadRefFromEvent(done),
		IssueTs:        start.Ts,
		CompleteTs:     done.Ts,
		DurationMs:     durationMs,
		IssueLine:      start.Line,
		CompleteLine:   done.Line,
	})
	acc.item.PairedCount++
	acc.totalLatencyMs += durationMs
	if durationMs > acc.item.MaxLatencyMs {
		acc.item.MaxLatencyMs = durationMs
	}
	acc.item.Bytes += startBlk.Len * 512
}

func accountBlockOpenTransition(accs map[string]*blockPairingAccumulator, lane *blockPairingLane, transition pairingCohortTransition) {
	if lane == nil || !transition.cohortClosed || transition.cohortStarts == 0 {
		return
	}
	acc := blockPairingAccumulatorFor(accs, lane.source, lane.family, transition.first)
	observeBlockPairingEnvelope(acc, transition.first, transition.last)
	acc.item.Count += transition.cohortStarts
	if transition.ambiguous {
		acc.item.AmbiguousCohortCount++
		acc.item.PairingSuppressedCount += transition.cohortStarts
		return
	}
	acc.item.UnpairedStartCount++
}

func pairingOpenCohortIntersectsIndex(first Event, idx *Index, q Query) bool {
	if first.Line == 0 || idx == nil {
		return false
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		if q.LineEnd > 0 && first.Line > q.LineEnd {
			return false
		}
		return idx.LineCount == 0 || q.LineStart == 0 || idx.LineCount >= q.LineStart
	}
	if queryBoundedTimeEnd(q) && first.Ts > q.TimeEnd {
		return false
	}
	return !queryBoundedTimeStart(q) || idx.LastTs == 0 || idx.LastTs >= q.TimeStart
}

func storageLatencySummaryText(item StorageLatencySummary) string {
	return fmt.Sprintf("layer=%s event=%s dev=%s op=%s count=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d max_latency=%.3fms",
		item.Layer, item.Event, item.Dev, item.Operation, item.Count, item.PairedCount,
		item.UnpairedStartCount, item.UnpairedDoneCount, item.AmbiguousCohortCount,
		item.PairingSuppressedCount, item.MaxLatencyMs)
}
