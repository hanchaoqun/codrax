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
	maxBlockSectorCount    = int64(1<<32 - 1)
	// BlockIOWaitCaliberIssueToComplete is the physical request-residence
	// ruler.  It deliberately does not claim that the issuing task stayed
	// scheduler-blocked for the whole interval.
	BlockIOWaitCaliberIssueToComplete    = "block_rq_issue_to_complete"
	BlockIOWaitCaliberBIOQueueToComplete = "block_bio_queue_to_complete"
	// BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked is the response-
	// impact ruler. It is minted only by an exact request completion which
	// directly wakes the issuing thread after a proven S/D switch-out.
	BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked = "completion_closed_issuer_blocked"
)

func blockIORequestResidenceCaliber(family string) string {
	if family == blockEndpointFamilyBIO {
		return BlockIOWaitCaliberBIOQueueToComplete
	}
	return BlockIOWaitCaliberIssueToComplete
}

func blockIOIssuerWaitState(state ThreadState) bool {
	switch state {
	case StateSSleep, StateDSleep:
		return true
	default:
		return false
	}
}

// blockLatencyEndpoint is the single closed-set admission gate for elapsed
// block latency. EventType intentionally stays wire-compatible and therefore
// groups insert/getrq/bio rows; the exact raw event name is the precise signal
// that decides whether a row is an endpoint and which family it belongs to.
// Name-less hand-built Events retain the historical rq compatibility lane.
func blockLatencyEndpoint(ev Event) (family string, phase blockEndpointPhase, ok bool) {
	if profile, endpoint := pairingEndpointProfileForName(ev.Name); endpoint && profile.Family == PairingEndpointBlock {
		if profile.Phase == PairingEndpointStart {
			return profile.SemanticBase, blockEndpointStart, true
		}
		return profile.SemanticBase, blockEndpointDone, true
	}
	switch strings.ToLower(strings.TrimSpace(ev.Name)) {
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
// value and selects one exact event-family profile. In particular, an RQ issue
// must carry its uint32 byte count while an RQ completion must not; BIO rows use
// their simple profile. This prevents a malformed body from borrowing another
// event family's grammar and minting an elapsed-latency endpoint.
func parseBlockRequestValidated(rawType, fields string) (dev, op string, sector, length int64, valid bool) {
	dev, op, sector, length, keyKnown, admitted := parseBlockRequestFingerprint(rawType, fields)
	if !keyKnown || !admitted {
		return dev, op, 0, 0, false
	}
	return dev, op, sector, length, true
}

// parseBlockRequestFingerprint separates a syntactically complete hard key
// from semantic payload admission. For example `R ... + 0` names one exact
// lane but is not a real elapsed request; it must quarantine that lane without
// deleting a valid flush on the same physical source.
func parseBlockRequestFingerprint(rawType, fields string) (dev, op string, sector, length int64, keyKnown, admitted bool) {
	trimmed := strings.TrimSpace(fields)
	var devRaw, opRaw, sectorRaw, lengthRaw string
	nonKeyAdmitted := true
	profile, endpoint := pairingEndpointProfileForName(rawType)
	rawType = strings.TrimSpace(rawType)
	switch {
	case endpoint && profile.Family == PairingEndpointBlock && profile.SemanticBase == blockEndpointFamilyRQ && profile.Phase == PairingEndpointStart:
		m := blockRQIssueRE.FindStringSubmatch(trimmed)
		if len(m) != 6 {
			return "", "", 0, 0, false, false
		}
		nonKeyAdmitted = blockUnsignedFits(m[3], 32)
		devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[4], m[5]
	case endpoint && profile.Family == PairingEndpointBlock && profile.SemanticBase == blockEndpointFamilyRQ && profile.Phase == PairingEndpointDone:
		m := blockRQCompleteRE.FindStringSubmatch(trimmed)
		if len(m) != 6 {
			return "", "", 0, 0, false, false
		}
		nonKeyAdmitted = blockSignedFits(m[5], 32)
		devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[3], m[4]
	case endpoint && profile.Family == PairingEndpointBlock && profile.SemanticBase == blockEndpointFamilyBIO && profile.Phase == PairingEndpointStart:
		m := blockBioQueueRE.FindStringSubmatch(trimmed)
		if len(m) != 5 {
			return "", "", 0, 0, false, false
		}
		devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[3], m[4]
	case endpoint && profile.Family == PairingEndpointBlock && profile.SemanticBase == blockEndpointFamilyBIO && profile.Phase == PairingEndpointDone:
		m := blockBioCompleteRE.FindStringSubmatch(trimmed)
		if len(m) != 6 {
			return "", "", 0, 0, false, false
		}
		nonKeyAdmitted = blockSignedFits(m[5], 32)
		devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[3], m[4]
	case rawType == "block_rq_insert":
		// RQ insert is inventory-only. Prefer the modern RQ profile, but keep
		// the historical simple formatter shape without admitting either as
		// a latency endpoint.
		if m := blockRQIssueRE.FindStringSubmatch(trimmed); len(m) == 6 && blockUnsignedFits(m[3], 32) {
			devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[4], m[5]
		} else if m := blockSimpleLegacyRE.FindStringSubmatch(trimmed); len(m) == 5 {
			devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[3], m[4]
		} else {
			return "", "", 0, 0, false, false
		}
	case rawType == "block_getrq":
		m := blockSimpleLegacyRE.FindStringSubmatch(trimmed)
		if len(m) != 5 {
			return "", "", 0, 0, false, false
		}
		devRaw, opRaw, sectorRaw, lengthRaw = m[1], m[2], m[3], m[4]
	default:
		return "", "", 0, 0, false, false
	}
	sector, sectorErr := strconv.ParseInt(sectorRaw, 10, 64)
	length, lengthErr := strconv.ParseInt(lengthRaw, 10, 64)
	dev, devOK := canonicalBlockDevice(devRaw)
	op = strings.TrimSpace(opRaw)
	keyKnown = sectorErr == nil && lengthErr == nil && devOK && blockDeviceIdentifiesRequest(dev) && validBlockOperationToken(op) && sector >= 0 && length >= 0 && length <= maxBlockSectorCount
	if !keyKnown {
		return dev, op, 0, 0, false, false
	}
	admitted = nonKeyAdmitted && (length > 0 || blockOperationAllowsZeroLength(op, sector, length))
	return dev, op, sector, length, true, admitted
}

func blockUnsignedFits(raw string, bits int) bool {
	_, err := strconv.ParseUint(raw, 10, bits)
	return err == nil
}

func blockSignedFits(raw string, bits int) bool {
	_, err := strconv.ParseInt(raw, 10, bits)
	return err == nil
}

func blockRequestIdentityValid(ev Event) bool {
	blk := ev.BlockIOFields
	if blk == nil {
		return false
	}
	if blk.IdentityParsed {
		dev, devOK := canonicalBlockDevice(blk.Dev)
		return blk.IdentityValid && devOK && blockDeviceIdentifiesRequest(dev) && validBlockOperationToken(blk.Op) && blk.Sector >= 0 &&
			((blk.Len > 0 && blk.Len <= maxBlockSectorCount) || blockOperationAllowsZeroLength(blk.Op, blk.Sector, blk.Len))
	}
	// Compatibility lane for manually constructed Events. A zero sector is
	// accepted because its presence cannot be represented in the old shape.
	dev, devOK := canonicalBlockDevice(blk.Dev)
	return devOK && blockDeviceIdentifiesRequest(dev) && validBlockOperationToken(blk.Op) && blk.Sector >= 0 &&
		((blk.Len > 0 && blk.Len <= maxBlockSectorCount) || blockOperationAllowsZeroLength(blk.Op, blk.Sector, blk.Len))
}

func canonicalBlockDevice(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	separator := ","
	if strings.Count(raw, ",") == 0 && strings.Count(raw, ":") == 1 {
		separator = ":"
	} else if strings.Count(raw, ",") != 1 || strings.Count(raw, ":") != 0 {
		return "", false
	}
	parts := strings.Split(raw, separator)
	if len(parts) != 2 || !blockDecimalDigits(parts[0]) || !blockDecimalDigits(parts[1]) {
		return "", false
	}
	major, majorErr := strconv.ParseUint(parts[0], 10, 32)
	minor, minorErr := strconv.ParseUint(parts[1], 10, 32)
	if majorErr != nil || minorErr != nil || major > 0xfff || minor > 0xfffff {
		return "", false
	}
	return fmt.Sprintf("%d,%d", major, minor), true
}

func blockDecimalDigits(raw string) bool {
	if raw == "" {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	return true
}

// Linux publishes dev_t==0 as the exact unnamed/null-device sentinel when a
// request has no rq_disk. Preserve it in the event inventory, but never let
// unrelated anonymous requests share it as a physical latency identity.
func blockDeviceIdentifiesRequest(dev string) bool {
	return dev != "" && dev != "0,0"
}

func validBlockOperationToken(op string) bool {
	if op == "" || op != strings.TrimSpace(op) || len(op) > 32 {
		return false
	}
	for _, r := range op {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
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
	dev, _ := canonicalBlockDevice(blk.Dev)
	return blockRequestIdentity{
		Family: family,
		Dev:    dev,
		Op:     blk.Op,
		Sector: blk.Sector,
		Len:    blk.Len,
	}, true
}

func (id blockRequestIdentity) laneKey() string {
	return encodePairingKey(
		id.Family,
		id.Dev,
		id.Op,
		strconv.FormatInt(id.Sector, 10),
		strconv.FormatInt(id.Len, 10),
	)
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

func blockPairingKey(idx *Index, ev Event) (string, string, bool) {
	source, ok := tracePairingSourceIdentity(idx, ev)
	if !ok {
		return "", "", false
	}
	verdict := fingerprintPairingEvent(ev)
	if verdict.Family == PairingEndpointBlock && verdict.PayloadAdmitted && verdict.EmitterAdmitted && verdict.KeyKnown {
		key, keyOK := verdict.LaneKey(source)
		if keyOK {
			return key, source, true
		}
	}
	return "", "", false
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
	census    []IOLatencySummary
	summaries []StorageLatencySummary
	caveats   []string
}

// stampBlockIOCompletionWakeups adds the strict block-completion → issuing
// thread wake credential to exact-paired requests. The join key includes the
// physical artifact and both directed endpoint PIDs; the issuer must have
// switched into a non-runnable state after issue and before completion; and
// the last eligible completion before a wake is the batch release point in
// that directed physical-line cohort. Each wake is consumed once. Earlier
// completions remain request-wait observations but do not each borrow the one
// wake. This prevents one sched_wakeup from blessing a burst of concurrent
// requests while retaining the real final-completion→wake edge.
func stampBlockIOCompletionWakeups(idx *Index, latencies []IOLatencySummary, pidAllowed func(int) bool) {
	if idx == nil || len(latencies) == 0 {
		return
	}
	type directedKey struct {
		source             string
		wakerPID, wakeePID int
	}
	type issuerKey struct {
		source string
		pid    int
	}
	candidates := make(map[directedKey][]int)
	for i := range latencies {
		io := &latencies[i]
		if strings.TrimSpace(io.SourcePath) == "" || io.IssueThread.PID <= 0 || io.CompleteThread.PID <= 0 || io.CompleteLine <= 0 || io.CompleteTs <= io.IssueTs {
			continue
		}
		if pidAllowed != nil && (!pidAllowed(io.IssueThread.PID) || !pidAllowed(io.CompleteThread.PID)) {
			continue
		}
		key := directedKey{source: io.SourcePath, wakerPID: io.CompleteThread.PID, wakeePID: io.IssueThread.PID}
		candidates[key] = append(candidates[key], i)
	}
	for key := range candidates {
		sort.SliceStable(candidates[key], func(i, j int) bool {
			left, right := latencies[candidates[key][i]], latencies[candidates[key][j]]
			if left.CompleteLine != right.CompleteLine {
				return left.CompleteLine < right.CompleteLine
			}
			return left.CompleteTs < right.CompleteTs
		})
	}
	wakeups := make(map[directedKey][]Event)
	blockedSwitches := make(map[issuerKey][]Event)
	issuerTimelines := make(map[issuerKey][]Event)
	for _, ev := range idx.Events {
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			continue
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if pidAllowed == nil || (pidAllowed(ev.PID) && pidAllowed(ev.WakeePID)) {
				key := directedKey{source: source, wakerPID: ev.PID, wakeePID: ev.WakeePID}
				wakeups[key] = append(wakeups[key], ev)
				issuerTimelines[issuerKey{source: source, pid: ev.WakeePID}] = append(issuerTimelines[issuerKey{source: source, pid: ev.WakeePID}], ev)
			}
		case EventSchedSwitch:
			state := stateFromPrevState(ev.PrevState)
			if ev.PrevPID > 0 && blockIOIssuerWaitState(state) && (pidAllowed == nil || pidAllowed(ev.PrevPID)) {
				key := issuerKey{source: source, pid: ev.PrevPID}
				blockedSwitches[key] = append(blockedSwitches[key], ev)
			}
			if ev.PrevPID > 0 && (pidAllowed == nil || pidAllowed(ev.PrevPID)) {
				key := issuerKey{source: source, pid: ev.PrevPID}
				issuerTimelines[key] = append(issuerTimelines[key], ev)
			}
			if ev.NextPID > 0 && ev.NextPID != ev.PrevPID && (pidAllowed == nil || pidAllowed(ev.NextPID)) {
				key := issuerKey{source: source, pid: ev.NextPID}
				issuerTimelines[key] = append(issuerTimelines[key], ev)
			}
		}
	}
	for key := range blockedSwitches {
		sort.SliceStable(blockedSwitches[key], func(i, j int) bool {
			if blockedSwitches[key][i].Line != blockedSwitches[key][j].Line {
				return blockedSwitches[key][i].Line < blockedSwitches[key][j].Line
			}
			return blockedSwitches[key][i].Ts < blockedSwitches[key][j].Ts
		})
	}
	for key := range issuerTimelines {
		sort.SliceStable(issuerTimelines[key], func(i, j int) bool {
			if issuerTimelines[key][i].Line != issuerTimelines[key][j].Line {
				return issuerTimelines[key][i].Line < issuerTimelines[key][j].Line
			}
			return issuerTimelines[key][i].Ts < issuerTimelines[key][j].Ts
		})
	}
	issuerBlockForRequest := func(io IOLatencySummary, wake Event) (Event, bool) {
		rows := blockedSwitches[issuerKey{source: io.SourcePath, pid: io.IssueThread.PID}]
		start := sort.Search(len(rows), func(i int) bool { return rows[i].Line > io.IssueLine })
		end := sort.Search(len(rows), func(i int) bool { return rows[i].Line >= io.CompleteLine })
		if start >= end {
			return Event{}, false
		}
		// blockedSwitches already contains only S/D exits, so the last row in
		// the request's line interval is the only candidate that can still be
		// open at completion. Binary-searching both bounds avoids rescanning a
		// busy issuer's full scheduler history for every request.
		blocked := rows[end-1]
		if blocked.Ts < io.IssueTs || blocked.Ts > io.CompleteTs {
			return Event{}, false
		}
		// The selected switch-out must still be the open wait closed by this
		// completion wake. A prior wake or a switch back into the issuer means
		// the request only overlaps a different wait and cannot own its wall time.
		timeline := issuerTimelines[issuerKey{source: io.SourcePath, pid: io.IssueThread.PID}]
		at := sort.Search(len(timeline), func(i int) bool { return timeline[i].Line > blocked.Line })
		for ; at < len(timeline) && timeline[at].Line < wake.Line; at++ {
			ev := timeline[at]
			switch ev.Type {
			case EventSchedWakeup, EventSchedWaking:
				if ev.WakeePID == io.IssueThread.PID {
					return Event{}, false
				}
			case EventSchedSwitch:
				if ev.NextPID == io.IssueThread.PID {
					return Event{}, false
				}
			}
		}
		return blocked, true
	}
	for key, rows := range wakeups {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Line != rows[j].Line {
				return rows[i].Line < rows[j].Line
			}
			return rows[i].Ts < rows[j].Ts
		})
		candidateRows := candidates[key]
		cursor := 0
		previousWakeLine := 0
		for _, wake := range rows {
			eligible := -1
			var eligibleBlocked Event
			for cursor < len(candidateRows) && latencies[candidateRows[cursor]].CompleteLine <= previousWakeLine {
				cursor++
			}
			for j := cursor; j < len(candidateRows); j++ {
				io := latencies[candidateRows[j]]
				if io.CompleteLine >= wake.Line {
					break
				}
				if wake.Ts >= io.CompleteTs && wake.Ts <= io.CompleteTs+rspaIOCompletionClosureTolS {
					if blocked, ok := issuerBlockForRequest(io, wake); ok {
						eligible = candidateRows[j]
						eligibleBlocked = blocked
					}
				}
			}
			if eligible >= 0 {
				io := &latencies[eligible]
				io.CompletionWokeIssuer = true
				io.IssuerBlockedStartTs = eligibleBlocked.Ts
				io.IssuerBlockedEndTs = wake.Ts
				io.IssuerBlockedMs = (wake.Ts - eligibleBlocked.Ts) * 1000
				io.IssuerBlockedLine = eligibleBlocked.Line
				io.IssuerBlockedState = string(stateFromPrevState(eligibleBlocked.PrevState))
				io.CausalWaitCaliber = BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked
				io.WakeupTs = wake.Ts
				io.WakeupLine = wake.Line
			}
			previousWakeLine = wake.Line
		}
	}
}

// blockPairingReplayIndexes returns the complete endpoint topology in the
// only order that can prove an elapsed pair: physical source, then physical
// Line within that source. Composite indexes are canonically timestamp-sorted,
// so walking idx.Events directly can move a regressed endpoint across another
// member of its cohort. Query bounds deliberately do not participate here;
// they gate accounting after the cohort has been adjudicated.
func blockPairingReplayIndexes(idx *Index) []int {
	if idx == nil {
		return nil
	}
	bySource := map[string][]int{}
	for eventIndex, ev := range idx.Events {
		if _, _, endpoint := blockLatencyEndpoint(ev); !endpoint {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			// The completeness pre-audit fail-closes unresolved provenance
			// before this replay can publish anything.
			continue
		}
		bySource[source] = append(bySource[source], eventIndex)
	}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	replay := make([]int, 0)
	for _, source := range sources {
		indexes := bySource[source]
		sort.SliceStable(indexes, func(i, j int) bool {
			left, right := idx.Events[indexes[i]], idx.Events[indexes[j]]
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Ts < right.Ts
		})
		replay = append(replay, indexes...)
	}
	return replay
}

// computeBlockIOLatencies pairs only exact endpoint families and never assumes
// FIFO ordering for repeated coarse identities. Once a lane reaches depth two,
// its whole cohort is ambiguous and emits no duration; the lane recovers only
// after depth returns to zero.
func computeBlockIOLatencies(idx *Index, q Query, max int, providedIntegrity ...*durationPairingIntegrity) blockPairingResult {
	if idx == nil {
		return blockPairingResult{}
	}
	integrity := selectedDurationPairingIntegrity(idx, q, durationOrderBlockIO, providedIntegrity)
	for _, ev := range idx.Events {
		if !pairingReplayAuditEvent(ev, q) {
			continue
		}
		if _, _, endpoint := blockLatencyEndpoint(ev); !endpoint {
			continue
		}
		verdict := fingerprintPairingEvent(ev)
		if verdict.Family != PairingEndpointBlock || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			integrity.rejectEvent(idx, ev, verdict)
			continue
		}
		if _, sourceOK := tracePairingSourceIdentity(idx, ev); !sourceOK {
			integrity.rejectEvent(idx, ev, verdict)
		}
	}
	if integrity.familyGlobal {
		caveats := integrity.caveats("io_latencies/storage_latency_by_layer(block)")
		if integrity.unresolvedSources > 0 {
			caveats = append(caveats, fmt.Sprintf("block_io_pairing_provenance_unresolved=true; rows=%d; endpoints without exactly one physical source artifact were excluded", integrity.unresolvedSources))
		}
		return blockPairingResult{caveats: caveats}
	}
	lanes := map[string]*blockPairingLane{}
	accs := map[string]*blockPairingAccumulator{}
	var out blockPairingResult
	invalidIdentity := 0
	unresolvedSourceRows := 0
	for _, eventIndex := range blockPairingReplayIndexes(idx) {
		ev := idx.Events[eventIndex]
		family, phase, endpoint := blockLatencyEndpoint(ev)
		if !endpoint {
			continue
		}
		laneKey, source, keyOK := blockPairingKey(idx, ev)
		if !keyOK {
			if pairingEventInsideQuery(ev, q) {
				if _, sourceOK := tracePairingSourceIdentity(idx, ev); sourceOK {
					invalidIdentity++
				} else {
					unresolvedSourceRows++
				}
			}
			continue
		}
		if integrity.poisonedSources[source] {
			continue
		}
		if integrity.poisonedLanes[laneKey] {
			continue
		}
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
	// Keep the complete exact-pair ledger before applying the public display
	// cap.  Downstream causal/rank consumers read census, never the Top-N view.
	out.census = out.latencies
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
	out.caveats = append(out.caveats, integrity.caveats("io_latencies/storage_latency_by_layer(block)")...)
	return out
}

func blockPairingAccumulatorFor(accs map[string]*blockPairingAccumulator, source, family string, ev Event) *blockPairingAccumulator {
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	key := encodePairingKey(source, family, blk.Dev, blk.Op)
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
		EndpointFamily: lane.family,
		Dev:            startBlk.Dev,
		Op:             startBlk.Op,
		Sector:         startBlk.Sector,
		Len:            startBlk.Len,
		IssueThread:    threadRefFromEvent(start),
		CompleteThread: threadRefFromEvent(done),
		IssueTs:        start.Ts,
		CompleteTs:     done.Ts,
		DurationMs:     durationMs,
		WaitCaliber:    blockIORequestResidenceCaliber(lane.family),
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
