package tracequery

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// durationOrderFamily names a typed elapsed-time state machine. A rollback in
// one family must not poison unrelated evidence (for example, a broken DMA
// fence lane cannot suppress scheduler or trace-mark durations).
type durationOrderFamily string

const (
	durationOrderTraceSpan    durationOrderFamily = "trace_span"
	durationOrderTraceTrack   durationOrderFamily = "trace_track_span"
	durationOrderTraceCounter durationOrderFamily = "trace_counter_delta"
	durationOrderIRQ          durationOrderFamily = "irq"
	durationOrderSoftIRQ      durationOrderFamily = "softirq"
	durationOrderIPI          durationOrderFamily = "ipi"
	durationOrderBinder       durationOrderFamily = "binder"
	durationOrderWorkqueue    durationOrderFamily = "workqueue"
	durationOrderDMAFence     durationOrderFamily = "dma_fence"
	durationOrderBlockIO      durationOrderFamily = "block_io"
	durationOrderStorage      durationOrderFamily = "storage_latency"
	durationOrderCPUFrequency durationOrderFamily = "cpu_frequency"
	durationOrderCPUFreqLimit durationOrderFamily = "cpu_frequency_limits"
)

const durationOrderFailureCap = 64

// durationOrderEventScanWitnessCap bounds the per-index memoized full-event
// scan (audit #24, 复核 F3): the 64-witness ledger cap was sized for
// per-query scan cost, which the once-memo removed. A larger memo cap keeps
// bounded memory (~200B/witness → ~200KB) while making the "first 64
// violations all sit outside the query window, so the in-window one collapses
// into a whole-family order_audit_truncated" false-close class 16× rarer.
// Overflow keeps the capped fail-close semantics.
const durationOrderEventScanWitnessCap = 1024

// durationPairingReplayEventBudget bounds every complete physical-topology
// replay and its temporary source/Line index. It deliberately matches the
// default in-memory Index event ceiling: ordinary indexes pay no new product
// regression, while callers that explicitly construct/raise a larger Index
// get a deterministic family fail-close instead of query-time unbounded maps.
const durationPairingReplayEventBudget = defaultTraceIndexMaxEvents

// durationOrderViolation is a precise physical-order regression on one typed
// duration lane. Lane is a bounded, deterministic digest for disclosure; the
// tracker itself always keys on the full, untruncated typed identity.
type durationOrderViolation struct {
	Family     durationOrderFamily
	Lane       string
	LaneKey    string
	Issue      string
	EventName  string
	Fields     []string
	PreviousTs float64
	CurrentTs  float64
	// TsUnknown marks a malformed endpoint whose own timestamp did not parse
	// (ENG audit #4b, §29.25 处置委托 2026-07-10): CurrentTs is then 0 and MUST
	// NOT be read as a time coordinate — window relevance and the per-pair
	// interval match treat such a violation conservatively (line-scoped).
	TsUnknown  bool
	Line       int
	SourcePath string
	// LocalLine is the artifact-local physical line, carried at composite
	// rebase time (audit #36, §29.25 处置委托 2026-07-10): Line is then a
	// virtual index-global coordinate and MUST NOT be read against the
	// physical SourcePath alone.
	LocalLine int
}

func (v *durationOrderViolation) reason() string {
	if v == nil {
		return ""
	}
	if v.Lane == "order_audit_truncated" {
		return fmt.Sprintf("duration_lane_timestamp_audit_truncated family=%s; overflow is fail-closed", v.Family)
	}
	if v.Issue == "endpoint_parse_incomplete" {
		ts := fmt.Sprintf("%.6f", v.CurrentTs)
		if v.TsUnknown {
			ts = "unknown"
		}
		reason := fmt.Sprintf("duration_endpoint_parse_incomplete family=%s event=%s missing_or_invalid=%s ts=%s line=%d",
			v.Family, v.EventName, strings.Join(v.Fields, ","), ts, v.Line)
		if v.SourcePath != "" {
			reason += fmt.Sprintf(" source=%s", v.SourcePath)
			reason += witnessLocalLineSuffix(v.Line, v.LocalLine)
		}
		return reason
	}
	reason := fmt.Sprintf("duration_lane_timestamp_regressed family=%s lane=%s previous_ts=%.6f current_ts=%.6f line=%d", v.Family, v.Lane, v.PreviousTs, v.CurrentTs, v.Line)
	if v.SourcePath != "" {
		reason += fmt.Sprintf(" source=%s", v.SourcePath)
		reason += witnessLocalLineSuffix(v.Line, v.LocalLine)
	}
	return reason
}

// witnessLocalLineSuffix renders the artifact-local coordinate whenever a
// witness line was rebased into virtual index-global space (audit #36):
// printing only "line=<virtual> source=<physical path>" invites fabricated
// file:line citations because that line does not exist at that offset in the
// named file. Coinciding coordinates (single-file or base-0 artifact) stay
// terse.
func witnessLocalLineSuffix(line, localLine int) string {
	if localLine <= 0 || localLine == line {
		return ""
	}
	return fmt.Sprintf(" local_line=%d", localLine)
}

type durationOrderLane struct {
	family durationOrderFamily
	key    string
}

type durationOrderPhase uint8

const (
	durationOrderSample durationOrderPhase = iota
	durationOrderOpen
	durationOrderClose
)

type durationOrderObservation struct {
	lane  durationOrderLane
	phase durationOrderPhase
	owner int
}

// durationOrderTrackerLaneBudget bounds the number of concurrently tracked
// lanes per typed family (perf audit #3/#23, §29.25 处置委托 2026-07-10): a
// hostile or degenerate capture minting a fresh counter/async identity per
// row must not grow tracker memory with capture-wide identity cardinality.
// DOMAIN SEMANTICS (复核 F4): this counter's domain is the ENTIRE scanned
// region (windowed builds audit the full physical prefix plus the window),
// unlike traceCounterSeriesBudget=8192 whose domain is the already
// window-clipped consumer event set. A GiB-scale capture can legitimately
// carry >8192 distinct counter identities file-wide, so the tracker budget is
// 8× the consumer budget; at ~300B/lane the worst case stays ~20MB/family.
// Overflow is fail-closed, never silent: the family is marked capped and
// every consumer sees the existing order_audit_truncated poison for it.
const durationOrderTrackerLaneBudget = 65536

// durationOrderTracker follows physical input order. Open/close topology is
// forgotten when a state machine closes. The four hard pairing families also
// retain one bounded last-endpoint timestamp per exact lane, because sequential
// identity reuse must not hide a cross-cohort physical clock rollback.
type durationOrderTracker struct {
	last                 map[durationOrderLane]float64
	depth                map[durationOrderLane]int
	owners               map[durationOrderLane]int
	pairingLast          map[durationOrderLane]float64
	pairingLanes         map[durationOrderFamily]map[durationOrderLane]struct{}
	pairingHistoryCapped map[durationOrderFamily]bool
	pairingHistoryBudget int
	// traceSpanOwners mirrors the open-order stack for trace spans only. Async
	// S/F lanes are keyed by payload pid/name/cookie and may legitimately hold
	// starts emitted by several row PIDs; a last-writer owner cannot safely
	// decide which lifecycle/malformed reset touches the shared lane.
	traceSpanOwners map[durationOrderLane][]int
	// Inverse indexes keep every reset amortized O(affected lanes) instead of
	// a full-table scan per lifecycle/malformed row (perf audit #23):
	// familyLanes mirrors the exact domain of t.last per family, ownerLanes
	// mirrors t.owners, and spanOwnerLanes counts per-pid stack membership in
	// traceSpanOwners.
	familyLanes    map[durationOrderFamily]map[durationOrderLane]struct{}
	ownerLanes     map[int]map[durationOrderLane]struct{}
	spanOwnerLanes map[int]map[durationOrderLane]int
	capped         map[durationOrderFamily]bool
}

func newDurationOrderTracker() *durationOrderTracker {
	return &durationOrderTracker{
		last:                 map[durationOrderLane]float64{},
		depth:                map[durationOrderLane]int{},
		owners:               map[durationOrderLane]int{},
		pairingLast:          map[durationOrderLane]float64{},
		pairingLanes:         map[durationOrderFamily]map[durationOrderLane]struct{}{},
		pairingHistoryCapped: map[durationOrderFamily]bool{},
		pairingHistoryBudget: durationOrderTrackerLaneBudget,
		traceSpanOwners:      map[durationOrderLane][]int{},
		familyLanes:          map[durationOrderFamily]map[durationOrderLane]struct{}{},
		ownerLanes:           map[int]map[durationOrderLane]struct{}{},
		spanOwnerLanes:       map[int]map[durationOrderLane]int{},
		capped:               map[durationOrderFamily]bool{},
	}
}

func durationFamilyUsesHardPairingLane(family durationOrderFamily) bool {
	return pairingEndpointFamilyForDuration(family) != ""
}

func (t *durationOrderTracker) observePairingTimestamp(lane durationOrderLane, ts float64) (previous float64, seen, admitted bool) {
	if t == nil || !durationFamilyUsesHardPairingLane(lane.family) {
		return 0, false, true
	}
	previous, seen = t.pairingLast[lane]
	if !seen {
		set := t.pairingLanes[lane.family]
		if set == nil {
			set = map[durationOrderLane]struct{}{}
			t.pairingLanes[lane.family] = set
		}
		if len(set) >= t.pairingHistoryBudget {
			t.pairingHistoryCapped[lane.family] = true
			return 0, false, false
		}
		set[lane] = struct{}{}
	}
	t.pairingLast[lane] = ts
	return previous, seen, true
}

func (t *durationOrderTracker) trackLane(lane durationOrderLane) {
	set := t.familyLanes[lane.family]
	if set == nil {
		set = map[durationOrderLane]struct{}{}
		t.familyLanes[lane.family] = set
	}
	set[lane] = struct{}{}
}

func (t *durationOrderTracker) setOwner(lane durationOrderLane, owner int) {
	if previous, ok := t.owners[lane]; ok {
		if previous == owner {
			return
		}
		if set := t.ownerLanes[previous]; set != nil {
			delete(set, lane)
			if len(set) == 0 {
				delete(t.ownerLanes, previous)
			}
		}
	}
	t.owners[lane] = owner
	set := t.ownerLanes[owner]
	if set == nil {
		set = map[durationOrderLane]struct{}{}
		t.ownerLanes[owner] = set
	}
	set[lane] = struct{}{}
}

func (t *durationOrderTracker) pushSpanOwner(lane durationOrderLane, owner int) {
	t.traceSpanOwners[lane] = append(t.traceSpanOwners[lane], owner)
	byLane := t.spanOwnerLanes[owner]
	if byLane == nil {
		byLane = map[durationOrderLane]int{}
		t.spanOwnerLanes[owner] = byLane
	}
	byLane[lane]++
}

func (t *durationOrderTracker) dropSpanOwner(lane durationOrderLane, owner int) {
	byLane := t.spanOwnerLanes[owner]
	if byLane == nil {
		return
	}
	if byLane[lane] <= 1 {
		delete(byLane, lane)
		if len(byLane) == 0 {
			delete(t.spanOwnerLanes, owner)
		}
		return
	}
	byLane[lane]--
}

func (t *durationOrderTracker) popSpanOwner(lane durationOrderLane) {
	owners := t.traceSpanOwners[lane]
	if len(owners) == 0 {
		return
	}
	owner := owners[len(owners)-1]
	t.traceSpanOwners[lane] = owners[:len(owners)-1]
	t.dropSpanOwner(lane, owner)
}

func (t *durationOrderTracker) clearLane(lane durationOrderLane) {
	if owner, ok := t.owners[lane]; ok {
		if set := t.ownerLanes[owner]; set != nil {
			delete(set, lane)
			if len(set) == 0 {
				delete(t.ownerLanes, owner)
			}
		}
	}
	for _, owner := range t.traceSpanOwners[lane] {
		t.dropSpanOwner(lane, owner)
	}
	if set := t.familyLanes[lane.family]; set != nil {
		delete(set, lane)
		if len(set) == 0 {
			delete(t.familyLanes, lane.family)
		}
	}
	delete(t.last, lane)
	delete(t.depth, lane)
	delete(t.owners, lane)
	delete(t.traceSpanOwners, lane)
}

func (t *durationOrderTracker) clearLaneSet(lanes map[durationOrderLane]struct{}) {
	if len(lanes) == 0 {
		return
	}
	// clearLane mutates the inverse sets being iterated; collect first.
	collected := make([]durationOrderLane, 0, len(lanes))
	for lane := range lanes {
		collected = append(collected, lane)
	}
	for _, lane := range collected {
		t.clearLane(lane)
	}
}

func (t *durationOrderTracker) resetPID(pid int) {
	if t == nil || pid <= 0 {
		return
	}
	t.clearLaneSet(t.ownerLanes[pid])
	// Trace async identities do not contain the row emitter and can be shared
	// by several emitters. Membership in the open-owner stack is the precise
	// signal; when present, clear the whole audit lane conservatively because a
	// partial depth/timestamp predecessor would no longer describe its pairing
	// state.
	t.resetTraceMarkPID(pid)
}

func (t *durationOrderTracker) resetFamily(family durationOrderFamily) {
	if t == nil || family == "" {
		return
	}
	t.clearLaneSet(t.familyLanes[family])
}

func (t *durationOrderTracker) resetTraceMarkPID(pid int) {
	if t == nil || pid < 0 {
		return
	}
	if byLane := t.spanOwnerLanes[pid]; len(byLane) > 0 {
		collected := make([]durationOrderLane, 0, len(byLane))
		for lane := range byLane {
			collected = append(collected, lane)
		}
		for _, lane := range collected {
			t.clearLane(lane)
		}
	}
	// pid 0 is not retained in owners, but the synchronous key is still exact.
	syncLane := durationOrderLane{family: durationOrderTraceSpan, key: fmt.Sprintf("sync\x00%d", pid)}
	t.clearLane(syncLane)
}

func (t *durationOrderTracker) observeAll(ev Event) []durationOrderViolation {
	if t == nil {
		return nil
	}
	if traceMarkEventMalformed(ev) {
		// The malformed row is inventory, not an endpoint observation. Reset the
		// known emitter's open lanes so a later valid close cannot attach to
		// pre-corruption state; a later valid begin starts a fresh recoverable lane.
		t.resetTraceMarkPID(ev.PID)
		return nil
	}
	if pid, reset := schedulerLifecycleResetPID(ev); reset {
		t.resetPID(pid)
		return nil
	}
	if failure := durationEndpointValidationFailureFromEvent(ev); failure != nil {
		// A malformed duration endpoint makes the affected family unavailable
		// for the current query. Reset its open state as well, so a later window
		// cannot attach a valid close to a pre-corruption start.
		if failure.LaneKey != "" {
			t.clearLane(durationOrderLane{family: failure.Family, key: failure.LaneKey})
		} else {
			t.resetFamily(failure.Family)
		}
		return []durationOrderViolation{*failure}
	}
	observations := durationOrderObservations(ev)
	if len(observations) == 0 {
		return nil
	}
	out := make([]durationOrderViolation, 0, len(observations))
	for _, observation := range observations {
		lane := observation.lane
		pairingPrevious, pairingSeen, pairingAdmitted := t.observePairingTimestamp(lane, ev.Ts)
		if !pairingAdmitted {
			continue
		}
		if pairingSeen && ev.Ts < pairingPrevious {
			out = append(out, durationOrderViolation{
				Family: lane.family, Lane: durationOrderLaneLabel(lane), LaneKey: lane.key,
				EventName: ev.Name, PreviousTs: pairingPrevious, CurrentTs: ev.Ts, Line: ev.Line,
			})
		}
		previous, seen := t.last[lane]
		// A close without a typed open cannot mint elapsed time, so it also
		// cannot establish a lane predecessor for a future pair.
		if observation.phase == durationOrderClose && t.depth[lane] == 0 {
			continue
		}
		if !seen {
			// Only open/sample phases can mint a NEW lane (a close on an
			// unseen lane bailed above). Past the per-family budget the lane
			// is dropped and the family fail-closes via the capped mark —
			// bounded memory, never a silently-missing audit.
			if len(t.familyLanes[lane.family]) >= durationOrderTrackerLaneBudget {
				t.capped[lane.family] = true
				continue
			}
			t.trackLane(lane)
		}
		if seen && !durationFamilyUsesHardPairingLane(lane.family) && ev.Ts < previous {
			out = append(out, durationOrderViolation{
				Family: lane.family, Lane: durationOrderLaneLabel(lane), LaneKey: lane.key,
				EventName: ev.Name, PreviousTs: previous, CurrentTs: ev.Ts, Line: ev.Line,
			})
		}
		switch observation.phase {
		case durationOrderOpen:
			t.depth[lane]++
			t.last[lane] = ev.Ts
			if lane.family == durationOrderTraceSpan {
				t.pushSpanOwner(lane, observation.owner)
			} else if observation.owner > 0 {
				t.setOwner(lane, observation.owner)
			}
		case durationOrderClose:
			if lane.family == durationOrderTraceSpan {
				t.popSpanOwner(lane)
			}
			if t.depth[lane] <= 1 {
				t.clearLane(lane)
			} else {
				t.depth[lane]--
				t.last[lane] = ev.Ts
			}
		default:
			t.last[lane] = ev.Ts
			if observation.owner > 0 {
				t.setOwner(lane, observation.owner)
			}
		}
	}
	return out
}

func durationOrderObservations(ev Event) []durationOrderObservation {
	lane := func(family durationOrderFamily, key string) durationOrderLane {
		return durationOrderLane{family: family, key: key}
	}
	switch ev.Type {
	case EventBlockIssue, EventBlockComplete:
		verdict := fingerprintPairingEvent(ev)
		if !verdict.Recognized || verdict.Family != PairingEndpointBlock || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			// block_rq_insert/block_getrq are inventory rows, not elapsed-time
			// endpoints. The exact raw name gate also prevents rq/bio cross-family
			// closes despite their shared wire-compatible EventType.
			return nil
		}
		phase := durationOrderOpen
		if verdict.Phase == PairingEndpointDone {
			phase = durationOrderClose
		}
		return []durationOrderObservation{{lane: lane(durationOrderBlockIO, verdict.SemanticKey), phase: phase, owner: ev.PID}}
	case EventTraceMark:
		switch ev.SpanAction {
		case "B":
			return []durationOrderObservation{{lane: lane(durationOrderTraceSpan, fmt.Sprintf("sync\x00%d", ev.PID)), phase: durationOrderOpen, owner: ev.PID}}
		case "E":
			return []durationOrderObservation{{lane: lane(durationOrderTraceSpan, fmt.Sprintf("sync\x00%d", ev.PID)), phase: durationOrderClose, owner: ev.PID}}
		case "S", "F":
			key := traceAsyncSpanKey(ev)
			if key == "" {
				return nil
			}
			phase := durationOrderOpen
			if ev.SpanAction == "F" {
				phase = durationOrderClose
			}
			return []durationOrderObservation{{lane: lane(durationOrderTraceSpan, "async\x00"+key), phase: phase, owner: ev.PID}}
		case "C":
			// Counter identity comes from the C| payload, not the row-header
			// writer: one process/global counter may be emitted by several
			// threads, while one writer may publish counters for several payload
			// owners. Only finite numeric rows feed counter_deltas. Do not attach
			// a scheduler-lifecycle owner: payload pids may live in a container
			// namespace, so a host sched_wakeup_new with the same integer is not
			// precise authority to reset this lane.
			key, ok := traceCounterTypedLaneKey(ev)
			if !ok {
				return nil
			}
			return []durationOrderObservation{{lane: lane(durationOrderTraceCounter, key), phase: durationOrderSample}}
		case "G", "H":
			key := traceTrackWireKey(ev)
			if key == "" {
				return nil
			}
			phase := durationOrderOpen
			if ev.SpanAction == "H" {
				phase = durationOrderClose
			}
			return []durationOrderObservation{{lane: lane(durationOrderTraceTrack, key), phase: phase, owner: ev.SpanPID}}
		}
	case EventWorkqueue:
		verdict := fingerprintPairingEvent(ev)
		phase, ok := durationPairPhase(string(verdict.Phase))
		if !verdict.Recognized || verdict.Family != PairingEndpointWorkqueue || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted || !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderWorkqueue, verdict.SemanticKey), phase: phase, owner: ev.PID}}
	case EventDMAFence:
		verdict := fingerprintPairingEvent(ev)
		phase, ok := durationPairPhase(string(verdict.Phase))
		if !verdict.Recognized || verdict.Family != PairingEndpointDMAFence || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted || !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderDMAFence, verdict.SemanticKey), phase: phase, owner: ev.PID}}
	case EventIRQ, EventSoftIRQ, EventIPI:
		family := durationOrderIRQ
		if ev.Type == EventSoftIRQ {
			family = durationOrderSoftIRQ
		} else if ev.Type == EventIPI {
			family = durationOrderIPI
		}
		_, phase := interruptBaseAndPhase(ev)
		// Raise rows are instantaneous inventory. They neither open a duration
		// lane nor govern entry/exit pairing/order.
		if phase != "entry" && phase != "exit" {
			return nil
		}
		key, ok := interruptLaneKey(ev)
		if !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(family, key), phase: durationOrderSample}}
	case EventStorage, EventFilesystem:
		verdict := fingerprintPairingEvent(ev)
		phase, ok := durationPairPhase(string(verdict.Phase))
		if !verdict.Recognized || verdict.Family != PairingEndpointStorage || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted || !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderStorage, verdict.SemanticKey), phase: phase, owner: ev.PID}}
	case EventCPUFrequency:
		// Only the exact per-CPU samples consumed by residency, scheduling-
		// segment weighting and the supply fold participate. Reclassified
		// clock_set_rate rows are corroboration-only and must not poison this
		// hard lane.
		if !isPerCPUFrequencySample(ev) {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderCPUFrequency, strconv.Itoa(eventCPUForStats(ev))), phase: durationOrderSample}}
	case EventCPUFrequencyLimit:
		// Match the policy-ceiling consumer admission exactly. Zero/absent Max
		// rows remain display inventory and do not govern elapsed state.
		cpu, ok := isPerCPULimitSample(ev)
		if !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderCPUFreqLimit, strconv.Itoa(cpu)), phase: durationOrderSample}}
	}
	return nil
}

func durationPairPhase(phase string) (durationOrderPhase, bool) {
	switch phase {
	case "start", "entry":
		return durationOrderOpen, true
	case "done", "exit":
		return durationOrderClose, true
	default:
		return 0, false
	}
}

func durationOrderLaneLabel(lane durationOrderLane) string {
	sum := sha256.Sum256([]byte(string(lane.family) + "\x00" + lane.key))
	return fmt.Sprintf("%s:%x", lane.family, sum[:6])
}

var durationOrderRawTokens = [...]string{
	"tracing_mark_write", ": print:", "binder_transaction", "workqueue_", "dma_fence",
	"irq_handler_", "softirq_", "ipi_", "sched_wakeup_new:", "sched_switch:",
	"cpu_frequency:", "cpu_frequency_limits:",
	"block_rq_issue:", "block_rq_complete:", "block_bio_queue:", "block_bio_complete:", "ufshcd_", "mmc_", "scsi_", "i2c_", "smbus_", "bio_", "ebpf_bio",
	"f2fs_", "hmfs_", "android_fs_", "ext4_", "filesystem", "file_system", "ebpf_file",
}

func durationOrderRawCandidate(line string) bool {
	for _, token := range durationOrderRawTokens {
		if strings.Contains(line, token) {
			return true
		}
	}
	return false
}

func durationOrderFailuresFromEvents(events []Event, q Query, limit int) ([]durationOrderViolation, map[durationOrderFamily]bool) {
	// Explicit limits up to the memo cap are honored (the per-index event
	// scan passes durationOrderEventScanWitnessCap); everything else stays on
	// the 64-witness ledger cap.
	if limit <= 0 || limit > durationOrderEventScanWitnessCap {
		limit = durationOrderFailureCap
	}
	tracker := newDurationOrderTracker()
	out := make([]durationOrderViolation, 0, min(limit, 8))
	capped := map[durationOrderFamily]bool{}
	for _, ev := range events {
		for _, failure := range tracker.observeAll(ev) {
			if !durationOrderViolationRelevantToQuery(&failure, q) {
				continue
			}
			if len(out) >= limit {
				capped[failure.Family] = true
				continue
			}
			out = append(out, failure)
		}
	}
	// A lane-budget overflow means some lanes of that family were never
	// audited at all — fail-close the whole family, mirroring the witness-cap
	// semantics above.
	for family, isCapped := range tracker.capped {
		if isCapped {
			capped[family] = true
		}
	}
	if !durationEventSequenceMonotonic(events) {
		for family, isCapped := range tracker.pairingHistoryCapped {
			if isCapped {
				capped[family] = true
			}
		}
	}
	return out, capped
}

func durationEventSequenceMonotonic(events []Event) bool {
	for i := 1; i < len(events); i++ {
		if events[i].Ts < events[i-1].Ts {
			return false
		}
	}
	return true
}

func appendDurationOrderFailure(idx *Index, failure durationOrderViolation) {
	if idx == nil {
		return
	}
	if len(idx.durationOrderFailures) >= durationOrderFailureCap {
		if idx.durationOrderFailuresCapped == nil {
			idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
		}
		idx.durationOrderFailuresCapped[failure.Family] = true
		return
	}
	failure.Fields = append([]string(nil), failure.Fields...)
	idx.durationOrderFailures = append(idx.durationOrderFailures, failure)
}

func mergeDurationOrderFailures(idx *Index, failures []durationOrderViolation, capped map[durationOrderFamily]bool) {
	if idx == nil {
		return
	}
	for _, failure := range failures {
		duplicate := false
		for _, existing := range idx.durationOrderFailures {
			if existing.Family == failure.Family && existing.LaneKey == failure.LaneKey && existing.Line == failure.Line && existing.PreviousTs == failure.PreviousTs && existing.CurrentTs == failure.CurrentTs && existing.SourcePath == failure.SourcePath {
				duplicate = true
				break
			}
		}
		if !duplicate {
			appendDurationOrderFailure(idx, failure)
		}
	}
	if len(capped) > 0 {
		if idx.durationOrderFailuresCapped == nil {
			idx.durationOrderFailuresCapped = map[durationOrderFamily]bool{}
		}
		for family, value := range capped {
			if value {
				idx.durationOrderFailuresCapped[family] = true
			}
		}
	}
}

func cloneDurationOrderCapped(in map[durationOrderFamily]bool) map[durationOrderFamily]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[durationOrderFamily]bool, len(in))
	for family, capped := range in {
		if capped {
			out[family] = true
		}
	}
	return out
}

func durationOrderViolationRelevantToQuery(v *durationOrderViolation, q Query) bool {
	if v == nil {
		return false
	}
	// Every hard elapsed-pairing consumer replays complete physical endpoint
	// topology for every query shape. This includes the parser-rejected raw-row
	// ledger: such a row has no Event for the consumer to rediscover. Cropping
	// its rollback/malformed barrier at a line/time boundary can rescue a false
	// pair, so the integrity witness must have exactly the same complete scope.
	if durationFamilyUsesHardPairingLane(v.Family) {
		return true
	}
	// Interrupt activity pairs only endpoints retained inside the selected
	// window; unlike trace-span/workqueue carry-in, an earlier malformed IRQ
	// endpoint cannot govern a later pair. Keep this poison scoped to the same
	// exact line/time selection instead of suppressing unrelated future windows.
	if v.Issue == "endpoint_parse_incomplete" {
		if q.LineStart > 0 && v.Line < q.LineStart {
			return false
		}
		// ENG audit #4b: an endpoint whose timestamp did not parse has no time
		// coordinate — its CurrentTs=0 must not let a windowed query scope the
		// poison out (the row may well sit inside the window).
		if !v.TsUnknown {
			if q.TimeStart > 0 && v.CurrentTs < q.TimeStart {
				return false
			}
			if q.TimeEnd > 0 && v.CurrentTs > q.TimeEnd {
				return false
			}
		}
	}
	if q.LineEnd > 0 && v.Line > q.LineEnd {
		return false
	}
	// A predecessor before LineStart may leave an open typed state that closes
	// inside the requested range, so a lower line bound cannot prove the
	// failure irrelevant. (The same carry-in concern applies to TimeStart.)
	// Like the scheduler audit, an earlier rollback may corrupt an open
	// state that closes inside the query. Only failures wholly after the
	// requested upper bound are provably irrelevant.
	if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && v.PreviousTs > q.TimeEnd && v.CurrentTs > q.TimeEnd {
		return false
	}
	return true
}

// ensureDurationOrderEventScan memoizes the full-event duration-order audit
// for non-monotonic indexes (perf audit #24; generationMetadataOnce house
// pattern). The tracker scan is query-independent — every observeAll runs
// over every event regardless of q — so the memo scans once with an empty
// query and each caller applies its own relevance post-filter. Cap semantics
// can only tighten: a violation the per-query scan would have surfaced is
// either inside the memo (then post-filtered in) or beyond the unfiltered
// witness cap, in which case its family carries the capped mark and every
// consumer fail-closes it via order_audit_truncated.
func ensureDurationOrderEventScan(idx *Index) ([]durationOrderViolation, map[durationOrderFamily]bool) {
	if idx == nil {
		return nil, nil
	}
	idx.durationOrderEventScanOnce.Do(func() {
		idx.durationOrderEventScanFailures, idx.durationOrderEventScanCapped = durationOrderFailuresFromEvents(idx.Events, Query{}, durationOrderEventScanWitnessCap)
	})
	return idx.durationOrderEventScanFailures, idx.durationOrderEventScanCapped
}

// durationPairingIntegrity is the source-scoped quarantine view for elapsed
// endpoint families. A failure with a known hard key invalidates only that
// physical artifact lane; an unknown key/owner with a known source closes only
// that source-family. Only unresolved provenance or bounded-audit overflow is
// the precise signal that requires whole-bundle family fail-close.
type durationPairingIntegrity struct {
	family             durationOrderFamily
	familyGlobal       bool
	topologyIncomplete bool
	poisonedLanes      map[string]bool
	laneSources        map[string]string
	laneSourceScopes   map[string]string
	poisonedSources    map[string]bool
	// poisonedSourceScopes is used only by the generic-storage integrity.
	// A malformed exact endpoint whose request key is unknowable still has an
	// exact layer/base scope; quarantine that source-local scope instead of
	// suppressing unrelated storage protocols in the same artifact.
	poisonedSourceScopes map[string]bool
	sourceScopeSources   map[string]string
	exactWitnesses       int
	sourceWitnesses      int
	globalWitnesses      int
	firstGlobalFailure   *durationOrderViolation
	firstExactFailure    *durationOrderViolation
	firstSourceFailure   *durationOrderViolation
	rejectedEndpointRows int
	unresolvedSources    int
	laneBudget           int
	budgetExceeded       bool
}

func newDurationPairingIntegrity(family durationOrderFamily) *durationPairingIntegrity {
	return newDurationPairingIntegrityWithBudget(family, durationOrderTrackerLaneBudget)

}

func newDurationPairingIntegrityWithBudget(family durationOrderFamily, budget int) *durationPairingIntegrity {
	if budget <= 0 || budget > durationOrderTrackerLaneBudget {
		budget = durationOrderTrackerLaneBudget
	}
	return &durationPairingIntegrity{
		family: family, poisonedLanes: map[string]bool{}, laneSources: map[string]string{}, laneSourceScopes: map[string]string{}, poisonedSources: map[string]bool{},
		poisonedSourceScopes: map[string]bool{}, sourceScopeSources: map[string]string{}, laneBudget: budget,
	}
}

func completePhysicalPairingTopology(idx *Index) bool {
	return idx != nil && (!idx.Windowed || idx.pairingTopologyComplete)
}

func failDurationPairingTopology(integrity *durationPairingIntegrity) {
	if integrity == nil {
		return
	}
	integrity.familyGlobal = true
	integrity.topologyIncomplete = true
}

func pairingEndpointFamilyForDuration(family durationOrderFamily) PairingEndpointFamily {
	switch family {
	case durationOrderBinder:
		return PairingEndpointBinder
	case durationOrderWorkqueue:
		return PairingEndpointWorkqueue
	case durationOrderDMAFence:
		return PairingEndpointDMAFence
	case durationOrderBlockIO:
		return PairingEndpointBlock
	case durationOrderStorage:
		return PairingEndpointStorage
	default:
		return ""
	}
}

func sourceScopedDurationOrderFailuresFromEvents(idx *Index, q Query, limit int) ([]durationOrderViolation, map[durationOrderFamily]bool) {
	if idx == nil {
		return nil, nil
	}
	bySource := map[string][]int{}
	for eventIndex, ev := range idx.Events {
		source, ok := tracePairingSourceIdentity(idx, ev)
		if !ok {
			continue
		}
		bySource[source] = append(bySource[source], eventIndex)
	}
	out := make([]durationOrderViolation, 0, 8)
	capped := map[durationOrderFamily]bool{}
	counts := map[durationOrderFamily]int{}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		eventIndexes := bySource[source]
		sort.SliceStable(eventIndexes, func(i, j int) bool {
			left, right := idx.Events[eventIndexes[i]], idx.Events[eventIndexes[j]]
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Ts < right.Ts
		})
		tracker := newDurationOrderTracker()
		monotonic := true
		previousTs := 0.0
		for position, eventIndex := range eventIndexes {
			ev := idx.Events[eventIndex]
			if position > 0 && ev.Ts < previousTs {
				monotonic = false
			}
			previousTs = ev.Ts
			for _, failure := range tracker.observeAll(ev) {
				if !durationFamilyUsesHardPairingLane(failure.Family) || !durationOrderViolationRelevantToQuery(&failure, q) {
					continue
				}
				if counts[failure.Family] >= limit {
					capped[failure.Family] = true
					continue
				}
				counts[failure.Family]++
				failure.SourcePath = source
				out = append(out, failure)
			}
		}
		for family, value := range tracker.capped {
			if value && durationFamilyUsesHardPairingLane(family) {
				capped[family] = true
			}
		}
		if !monotonic {
			for family, value := range tracker.pairingHistoryCapped {
				if value {
					capped[family] = true
				}
			}
		}
	}
	return out, capped
}

func pairingReplayAuditEvent(_ Event, _ Query) bool {
	// Block/Storage consumers replay complete physical topology for both time
	// and line queries. Auditing a narrower slice would delete a malformed or
	// regressed endpoint and let selected rows bridge across it. Known keys
	// remain lane-local; unknown keys remain source-family fail-close.
	return true
}

func durationPairingFailureSource(idx *Index, failure durationOrderViolation) (string, bool) {
	source := strings.TrimSpace(failure.SourcePath)
	if source == "" {
		var ok bool
		source, ok = tracePairingSourceIdentity(idx, Event{Line: failure.Line})
		if !ok {
			return "", false
		}
	}
	if source == "order_audit_truncated" {
		return "", false
	}
	return source, true
}

func durationPairingFailureLane(idx *Index, failure durationOrderViolation) (string, string, bool) {
	pairingFamily := pairingEndpointFamilyForDuration(failure.Family)
	if pairingFamily == "" || failure.LaneKey == "" {
		return "", "", false
	}
	source, ok := durationPairingFailureSource(idx, failure)
	if !ok {
		return "", "", false
	}
	return encodePairingKey(source, string(pairingFamily), failure.LaneKey), source, true
}

func (integrity *durationPairingIntegrity) quarantineCount() int {
	if integrity == nil {
		return 0
	}
	return len(integrity.poisonedLanes) + len(integrity.poisonedSources) + len(integrity.poisonedSourceScopes)
}

func (integrity *durationPairingIntegrity) poisonLane(lane, source string) bool {
	if integrity == nil || lane == "" || source == "" || integrity.familyGlobal || integrity.poisonedSources[source] || integrity.poisonedLanes[lane] {
		return false
	}
	if integrity.quarantineCount() >= integrity.laneBudget {
		integrity.familyGlobal = true
		integrity.budgetExceeded = true
		integrity.globalWitnesses++
		return false
	}
	integrity.poisonedLanes[lane] = true
	integrity.laneSources[lane] = source
	return true
}

func (integrity *durationPairingIntegrity) poisonLaneForEvent(lane, source, eventName string) bool {
	if integrity == nil {
		return false
	}
	if scope, ok := genericStorageSourceScope(source, eventName); ok {
		if integrity.poisonedSourceScopes[scope] {
			return false
		}
		if !integrity.poisonLane(lane, source) {
			return false
		}
		integrity.laneSourceScopes[lane] = scope
		return true
	}
	return integrity.poisonLane(lane, source)
}

func (integrity *durationPairingIntegrity) poisonSource(source string) bool {
	if integrity == nil || source == "" || integrity.familyGlobal || integrity.poisonedSources[source] {
		return false
	}
	for lane, laneSource := range integrity.laneSources {
		if laneSource == source {
			delete(integrity.poisonedLanes, lane)
			delete(integrity.laneSources, lane)
			delete(integrity.laneSourceScopes, lane)
		}
	}
	for scope, scopeSource := range integrity.sourceScopeSources {
		if scopeSource == source {
			delete(integrity.poisonedSourceScopes, scope)
			delete(integrity.sourceScopeSources, scope)
		}
	}
	if integrity.quarantineCount() >= integrity.laneBudget {
		integrity.familyGlobal = true
		integrity.budgetExceeded = true
		integrity.globalWitnesses++
		return false
	}
	integrity.poisonedSources[source] = true
	return true
}

func (integrity *durationPairingIntegrity) poisonStorageSourceScope(source, eventName string) (handled, added bool) {
	if integrity == nil || integrity.family != durationOrderStorage {
		return false, false
	}
	scope, ok := genericStorageSourceScope(source, eventName)
	if !ok {
		return false, false
	}
	// Once the exact profile scope is known, repeated witnesses stay handled
	// by that same scope.  Falling through to poisonSource merely because the
	// scope was already present would widen the second malformed row into an
	// unrelated-protocol outage.
	if integrity.familyGlobal || integrity.poisonedSources[source] || integrity.poisonedSourceScopes[scope] {
		return true, false
	}
	// A source-scope witness dominates every exact lane already quarantined in
	// that scope. Remove the subordinate entries before applying the bounded
	// budget so lane->scope replacement consumes one slot rather than
	// spuriously overflowing at the same cardinality.
	for lane, laneScope := range integrity.laneSourceScopes {
		if laneScope == scope {
			delete(integrity.poisonedLanes, lane)
			delete(integrity.laneSources, lane)
			delete(integrity.laneSourceScopes, lane)
		}
	}
	if integrity.quarantineCount() >= integrity.laneBudget {
		integrity.familyGlobal = true
		integrity.budgetExceeded = true
		integrity.globalWitnesses++
		return true, false
	}
	integrity.poisonedSourceScopes[scope] = true
	integrity.sourceScopeSources[scope] = source
	return true, true
}

func (integrity *durationPairingIntegrity) sourcePoisoned(source, eventName string) bool {
	if integrity == nil {
		return false
	}
	if integrity.familyGlobal || integrity.poisonedSources[source] {
		return true
	}
	if integrity.family != durationOrderStorage {
		return false
	}
	scope, ok := genericStorageSourceScope(source, eventName)
	return ok && integrity.poisonedSourceScopes[scope]
}

func applyDurationPairingFailure(idx *Index, integrity *durationPairingIntegrity, failure durationOrderViolation) {
	if integrity == nil {
		return
	}
	lane, source, ok := durationPairingFailureLane(idx, failure)
	if ok {
		if integrity.poisonLaneForEvent(lane, source, failure.EventName) {
			integrity.exactWitnesses++
		}
		if integrity.firstExactFailure == nil {
			copy := failure
			integrity.firstExactFailure = &copy
		}
		return
	}
	if source, sourceOK := durationPairingFailureSource(idx, failure); sourceOK {
		if handled, added := integrity.poisonStorageSourceScope(source, failure.EventName); handled {
			if added {
				integrity.sourceWitnesses++
			}
			if integrity.firstSourceFailure == nil {
				copy := failure
				integrity.firstSourceFailure = &copy
			}
			return
		}
		if integrity.poisonSource(source) {
			integrity.sourceWitnesses++
		}
		if integrity.firstSourceFailure == nil {
			copy := failure
			integrity.firstSourceFailure = &copy
		}
		return
	}
	integrity.familyGlobal = true
	integrity.globalWitnesses++
	integrity.unresolvedSources++
	if integrity.firstGlobalFailure == nil {
		copy := failure
		integrity.firstGlobalFailure = &copy
	}
}

func durationPairingIntegritiesForQuery(idx *Index, q Query) map[durationOrderFamily]*durationPairingIntegrity {
	families := []durationOrderFamily{durationOrderWorkqueue, durationOrderDMAFence, durationOrderBlockIO, durationOrderStorage}
	out := make(map[durationOrderFamily]*durationPairingIntegrity, len(families))
	for _, family := range families {
		out[family] = newDurationPairingIntegrity(family)
	}
	if idx == nil {
		return out
	}
	if !completePhysicalPairingTopology(idx) {
		for _, integrity := range out {
			failDurationPairingTopology(integrity)
		}
		return out
	}
	if applyDurationPairingReplayBudget(out, len(idx.Events)) {
		return out
	}
	for _, failure := range idx.durationOrderFailures {
		integrity := out[failure.Family]
		if integrity != nil && durationOrderViolationRelevantToQuery(&failure, q) {
			applyDurationPairingFailure(idx, integrity, failure)
		}
	}
	for family, capped := range idx.durationOrderFailuresCapped {
		if capped && out[family] != nil {
			out[family].familyGlobal = true
			out[family].globalWitnesses++
		}
	}
	if idx.TimestampOrder != TraceTimestampOrderMonotonic || idx.LineCount == 0 {
		failures, capped := sourceScopedDurationOrderFailuresFromEvents(idx, q, durationOrderEventScanWitnessCap)
		for _, failure := range failures {
			applyDurationPairingFailure(idx, out[failure.Family], failure)
		}
		for family, value := range capped {
			if value && out[family] != nil {
				out[family].familyGlobal = true
				out[family].globalWitnesses++
			}
		}
	}
	return out
}

func applyDurationPairingReplayBudget(integrities map[durationOrderFamily]*durationPairingIntegrity, eventCount int) bool {
	if eventCount >= 0 && eventCount <= durationPairingReplayEventBudget {
		return false
	}
	for _, integrity := range integrities {
		failDurationPairingReplayBudget(integrity)
	}
	return true
}

func failDurationPairingReplayBudget(integrity *durationPairingIntegrity) {
	if integrity == nil {
		return
	}
	integrity.familyGlobal = true
	if !integrity.budgetExceeded {
		integrity.globalWitnesses++
	}
	integrity.budgetExceeded = true
}

func durationPairingIntegrityForQuery(idx *Index, q Query, family durationOrderFamily) *durationPairingIntegrity {
	return durationPairingIntegritiesForQuery(idx, q)[family]
}

func selectedDurationPairingIntegrity(idx *Index, q Query, family durationOrderFamily, provided []*durationPairingIntegrity) *durationPairingIntegrity {
	if len(provided) > 0 && provided[0] != nil && provided[0].family == family {
		integrity := provided[0]
		if idx != nil && !completePhysicalPairingTopology(idx) {
			failDurationPairingTopology(integrity)
			return integrity
		}
		if idx != nil && len(idx.Events) > durationPairingReplayEventBudget {
			// Preserve the replay bound even for internal/direct callers that
			// supply an integrity object rather than the shared production batch.
			failDurationPairingReplayBudget(integrity)
		}
		return integrity
	}
	// Compatibility path for direct consumer tests and legacy internal callers.
	// The production ComputeWindowStats path supplies the shared batch result so
	// all four pairing families consume one source-aware scan.
	return durationPairingIntegrityForQuery(idx, q, family)
}

func (integrity *durationPairingIntegrity) rejectEvent(idx *Index, ev Event, verdict PairingEndpointVerdict) {
	if integrity == nil {
		return
	}
	integrity.rejectedEndpointRows++
	wantFamily := pairingEndpointFamilyForDuration(integrity.family)
	source, sourceOK := tracePairingSourceIdentity(idx, ev)
	if !sourceOK {
		integrity.familyGlobal = true
		integrity.globalWitnesses++
		integrity.unresolvedSources++
		return
	}
	if !verdict.Recognized || verdict.Family != wantFamily || !verdict.KeyKnown || !verdict.EmitterKnown {
		if handled, added := integrity.poisonStorageSourceScope(source, ev.Name); handled {
			if added {
				integrity.sourceWitnesses++
			}
			return
		}
		if integrity.poisonSource(source) {
			integrity.sourceWitnesses++
		}
		return
	}
	lane, laneOK := verdict.LaneKey(source)
	if !laneOK {
		integrity.familyGlobal = true
		integrity.globalWitnesses++
		return
	}
	if integrity.poisonLaneForEvent(lane, source, ev.Name) {
		integrity.exactWitnesses++
	}
}

func (integrity *durationPairingIntegrity) caveats(outputs string) []string {
	if integrity == nil {
		return nil
	}
	if integrity.topologyIncomplete {
		return []string{fmt.Sprintf("duration_pairing_fail_closed=true; family=%s windowed_pairing_topology_incomplete=true; omitted affected output family=%s because this cropped index has no complete physical endpoint topology proof; rebuild without windowed parsing or wait for a bounded pairing-topology sidecar", integrity.family, outputs)}
	}
	if integrity.familyGlobal {
		reason := fmt.Sprintf("family=%s unknown_or_unscoped_witnesses=%d rejected_endpoints=%d budget_exceeded=%t", integrity.family, integrity.globalWitnesses, integrity.rejectedEndpointRows, integrity.budgetExceeded)
		if integrity.firstGlobalFailure != nil {
			reason = integrity.firstGlobalFailure.reason()
		}
		return []string{fmt.Sprintf("duration_pairing_fail_closed=true; %s; omitted affected output family=%s because an endpoint key/source or bounded audit coverage was not provable", reason, outputs)}
	}
	var out []string
	if len(integrity.poisonedSources) > 0 || len(integrity.poisonedSourceScopes) > 0 {
		reason := "source-scoped endpoint key or owner was not provable"
		if integrity.firstSourceFailure != nil {
			reason = integrity.firstSourceFailure.reason()
		}
		out = append(out, fmt.Sprintf("duration_pairing_source_fail_closed=true; family=%s sources=%d source_scopes=%d witnesses=%d; %s; omitted only affected physical-source families/scopes from output family=%s", integrity.family, len(integrity.poisonedSources), len(integrity.poisonedSourceScopes), integrity.sourceWitnesses, reason, outputs))
	}
	if len(integrity.poisonedLanes) == 0 {
		return out
	}
	reason := "known hard lane carried a rejected or non-monotonic endpoint"
	if integrity.firstExactFailure != nil {
		reason = integrity.firstExactFailure.reason()
	}
	return append(out, fmt.Sprintf("duration_pairing_exact_lane_quarantined=true; family=%s lanes=%d witnesses=%d rejected_endpoints=%d; %s; omitted only affected source-scoped hard lanes from output family=%s", integrity.family, len(integrity.poisonedLanes), integrity.exactWitnesses, integrity.rejectedEndpointRows, reason, outputs))
}

func durationOrderFailuresForQuery(idx *Index, q Query) map[durationOrderFamily]*durationOrderViolation {
	out := map[durationOrderFamily]*durationOrderViolation{}
	if idx == nil {
		return out
	}
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		if _, exists := out[failure.Family]; !exists && durationOrderViolationRelevantToQuery(failure, q) {
			copy := *failure
			out[failure.Family] = &copy
		}
	}
	for family, capped := range idx.durationOrderFailuresCapped {
		if capped {
			if _, exists := out[family]; !exists {
				out[family] = &durationOrderViolation{Family: family, Lane: "order_audit_truncated", SourcePath: "order_audit_truncated"}
			}
		}
	}
	// A complete monotonic source proof dominates every per-lane audit. For a
	// canonical composite, child-local physical failures have already been
	// consumed above; the merged stream itself is monotonic by construction.
	// Avoid a second O(events) parse/KV pass on the overwhelmingly common path.
	if idx.TimestampOrder == TraceTimestampOrderMonotonic {
		return out
	}
	// Single-file full indexes retain physical event order and can be audited
	// lazily (memoized per index, filtered per query). Composite indexes are
	// canonically sorted, but their child-local physical failures were copied
	// into the private fields above; scanning the sorted slice can only add a
	// cross-artifact canonical-lane failure.
	failures, capped := ensureDurationOrderEventScan(idx)
	for i := range failures {
		failure := failures[i]
		if _, exists := out[failure.Family]; !exists && durationOrderViolationRelevantToQuery(&failure, q) {
			copy := failure
			out[failure.Family] = &copy
		}
	}
	for family, value := range capped {
		if value {
			if _, exists := out[family]; !exists {
				out[family] = &durationOrderViolation{Family: family, Lane: "order_audit_truncated", SourcePath: "order_audit_truncated"}
			}
		}
	}
	return out
}

func durationOrderFailureForFamily(idx *Index, q Query, family durationOrderFamily) *durationOrderViolation {
	return durationOrderFailuresForQuery(idx, q)[family]
}

func durationOrderFailClosedCaveat(failure *durationOrderViolation, outputs string) string {
	if failure == nil {
		return ""
	}
	suffix := "because its exact typed lane moved backwards in physical trace order"
	if failure.Issue == "endpoint_parse_incomplete" {
		suffix = "because a required endpoint identity was absent or malformed"
	}
	return "duration_pairing_fail_closed=true; " + failure.reason() + "; omitted affected output family=" + outputs + " " + suffix
}

// frequencyOrderIntegrity is the per-CPU fail-close view over the two
// frequency state families. A physical rollback on cpu N invalidates only
// cpu N; an audit-cap overflow has no provable subject set and therefore
// closes that whole family rather than silently admitting uninspected lanes.
type frequencyOrderIntegrity struct {
	frequency map[int]*durationOrderViolation
	limits    map[int]*durationOrderViolation
	freqAll   bool
	limitAll  bool
}

func frequencyOrderIntegrityForQuery(idx *Index, q Query) frequencyOrderIntegrity {
	out := frequencyOrderIntegrity{
		frequency: map[int]*durationOrderViolation{},
		limits:    map[int]*durationOrderViolation{},
	}
	if idx == nil {
		return out
	}
	add := func(failure durationOrderViolation) {
		if !durationOrderViolationRelevantToQuery(&failure, q) {
			return
		}
		cpu, err := strconv.Atoi(failure.LaneKey)
		if err != nil || cpu < 0 {
			return
		}
		copy := failure
		switch failure.Family {
		case durationOrderCPUFrequency:
			if _, exists := out.frequency[cpu]; !exists {
				out.frequency[cpu] = &copy
			}
		case durationOrderCPUFreqLimit:
			if _, exists := out.limits[cpu]; !exists {
				out.limits[cpu] = &copy
			}
		}
	}
	for _, failure := range idx.durationOrderFailures {
		add(failure)
	}
	out.freqAll = idx.durationOrderFailuresCapped[durationOrderCPUFrequency]
	out.limitAll = idx.durationOrderFailuresCapped[durationOrderCPUFreqLimit]

	// The common monotonic proof makes a second event scan redundant. For an
	// unknown/regressed single-file index, audit the retained physical order
	// lazily (memoized per index, filtered per query by add); composite
	// children already contributed their physical poison to the private
	// failure ledger before the canonical merge sort.
	if idx.TimestampOrder != TraceTimestampOrderMonotonic {
		failures, capped := ensureDurationOrderEventScan(idx)
		for _, failure := range failures {
			add(failure)
		}
		out.freqAll = out.freqAll || capped[durationOrderCPUFrequency]
		out.limitAll = out.limitAll || capped[durationOrderCPUFreqLimit]
	}
	return out
}

func (i frequencyOrderIntegrity) frequencyUnsafe(cpu int) bool {
	if i.freqAll {
		return true
	}
	_, unsafe := i.frequency[cpu]
	return unsafe
}

func (i frequencyOrderIntegrity) limitUnsafe(cpu int) bool {
	if i.limitAll {
		return true
	}
	_, unsafe := i.limits[cpu]
	return unsafe
}

func (i frequencyOrderIntegrity) caveats() []string {
	var out []string
	if i.freqAll || len(i.frequency) > 0 {
		out = append(out, frequencyOrderFailClosedCaveat(durationOrderCPUFrequency, i.frequency, i.freqAll,
			"frequency residency/weighting, cluster-frequency derivation, donor reuse and low-frequency judgments"))
	}
	if i.limitAll || len(i.limits) > 0 {
		out = append(out, frequencyOrderFailClosedCaveat(durationOrderCPUFreqLimit, i.limits, i.limitAll,
			"policy-ceiling timelines and frequency-limit-derived supply judgments"))
	}
	return out
}

func frequencyOrderFailClosedCaveat(family durationOrderFamily, failures map[int]*durationOrderViolation, all bool, outputs string) string {
	cpus := make([]int, 0, len(failures))
	for cpu := range failures {
		cpus = append(cpus, cpu)
	}
	sort.Ints(cpus)
	scope := fmt.Sprintf("affected_cpus=%v", cpus)
	if all {
		scope = "affected_cpus=all(audit_truncated)"
	}
	reason := ""
	if len(cpus) > 0 {
		reason = "; " + failures[cpus[0]].reason()
	}
	return fmt.Sprintf("frequency_timeline_fail_closed=true; family=%s %s%s; omitted affected CPU lanes from %s because physical sample order is not monotonic", family, scope, reason, outputs)
}
