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
	durationOrderWorkqueue    durationOrderFamily = "workqueue"
	durationOrderDMAFence     durationOrderFamily = "dma_fence"
	durationOrderBlockIO      durationOrderFamily = "block_io"
	durationOrderStorage      durationOrderFamily = "storage_latency"
	durationOrderCPUFrequency durationOrderFamily = "cpu_frequency"
	durationOrderCPUFreqLimit durationOrderFamily = "cpu_frequency_limits"
)

const durationOrderFailureCap = 64

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
}

func (v *durationOrderViolation) reason() string {
	if v == nil {
		return ""
	}
	if v.Lane == "order_audit_truncated" {
		return fmt.Sprintf("duration_lane_timestamp_audit_truncated family=%s; overflow is fail-closed", v.Family)
	}
	if v.Issue == "endpoint_parse_incomplete" {
		reason := fmt.Sprintf("duration_endpoint_parse_incomplete family=%s event=%s missing_or_invalid=%s ts=%.6f line=%d",
			v.Family, v.EventName, strings.Join(v.Fields, ","), v.CurrentTs, v.Line)
		if v.SourcePath != "" {
			reason += fmt.Sprintf(" source=%s", v.SourcePath)
		}
		return reason
	}
	reason := fmt.Sprintf("duration_lane_timestamp_regressed family=%s lane=%s previous_ts=%.6f current_ts=%.6f line=%d", v.Family, v.Lane, v.PreviousTs, v.CurrentTs, v.Line)
	if v.SourcePath != "" {
		reason += fmt.Sprintf(" source=%s", v.SourcePath)
	}
	return reason
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

// durationOrderTracker follows physical input order. Open/close lanes are
// forgotten when their typed state machine closes, bounding workqueue/DMA/span
// memory by concurrent opens rather than capture-wide identity cardinality.
// Sample lanes (IRQ bursts and counters) retain one timestamp per exact lane.
type durationOrderTracker struct {
	last   map[durationOrderLane]float64
	depth  map[durationOrderLane]int
	owners map[durationOrderLane]int
	// traceSpanOwners mirrors the open-order stack for trace spans only. Async
	// S/F lanes are keyed by payload pid/name/cookie and may legitimately hold
	// starts emitted by several row PIDs; a last-writer owner cannot safely
	// decide which lifecycle/malformed reset touches the shared lane.
	traceSpanOwners map[durationOrderLane][]int
}

func newDurationOrderTracker() *durationOrderTracker {
	return &durationOrderTracker{
		last:            map[durationOrderLane]float64{},
		depth:           map[durationOrderLane]int{},
		owners:          map[durationOrderLane]int{},
		traceSpanOwners: map[durationOrderLane][]int{},
	}
}

func (t *durationOrderTracker) clearLane(lane durationOrderLane) {
	delete(t.last, lane)
	delete(t.depth, lane)
	delete(t.owners, lane)
	delete(t.traceSpanOwners, lane)
}

func (t *durationOrderTracker) resetPID(pid int) {
	if t == nil || pid <= 0 {
		return
	}
	for lane, owner := range t.owners {
		if owner == pid {
			t.clearLane(lane)
		}
	}
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
	for lane := range t.last {
		if lane.family == family {
			t.clearLane(lane)
		}
	}
}

func (t *durationOrderTracker) resetTraceMarkPID(pid int) {
	if t == nil || pid < 0 {
		return
	}
	for lane, owners := range t.traceSpanOwners {
		if lane.family != durationOrderTraceSpan {
			continue
		}
		for _, owner := range owners {
			if owner == pid {
				t.clearLane(lane)
				break
			}
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
		t.resetFamily(failure.Family)
		return []durationOrderViolation{*failure}
	}
	observations := durationOrderObservations(ev)
	if len(observations) == 0 {
		return nil
	}
	out := make([]durationOrderViolation, 0, len(observations))
	for _, observation := range observations {
		lane := observation.lane
		previous, seen := t.last[lane]
		// A close without a typed open cannot mint elapsed time, so it also
		// cannot establish a lane predecessor for a future pair.
		if observation.phase == durationOrderClose && t.depth[lane] == 0 {
			continue
		}
		if seen && ev.Ts < previous {
			out = append(out, durationOrderViolation{
				Family: lane.family, Lane: durationOrderLaneLabel(lane), LaneKey: lane.key,
				PreviousTs: previous, CurrentTs: ev.Ts, Line: ev.Line,
			})
		}
		switch observation.phase {
		case durationOrderOpen:
			t.depth[lane]++
			t.last[lane] = ev.Ts
			if lane.family == durationOrderTraceSpan {
				t.traceSpanOwners[lane] = append(t.traceSpanOwners[lane], observation.owner)
			} else if observation.owner > 0 {
				t.owners[lane] = observation.owner
			}
		case durationOrderClose:
			if lane.family == durationOrderTraceSpan {
				owners := t.traceSpanOwners[lane]
				if len(owners) > 0 {
					t.traceSpanOwners[lane] = owners[:len(owners)-1]
				}
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
				t.owners[lane] = observation.owner
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
		_, endpointPhase, endpoint := blockLatencyEndpoint(ev)
		if !endpoint {
			// block_rq_insert/block_getrq are inventory rows, not elapsed-time
			// endpoints. The exact raw name gate also prevents rq/bio cross-family
			// closes despite their shared wire-compatible EventType.
			return nil
		}
		key := blockKey(ev)
		if key == "" {
			return nil
		}
		phase := durationOrderOpen
		if endpointPhase == blockEndpointDone {
			phase = durationOrderClose
		}
		return []durationOrderObservation{{lane: lane(durationOrderBlockIO, key), phase: phase, owner: ev.PID}}
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
		work, function := workqueueExactEndpointFields(ev)
		base, phaseName := workqueueBaseAndPhase(ev.Name)
		phase, ok := durationPairPhase(phaseName)
		if !ok || len(workqueueEndpointMissingFields(ev, work)) > 0 {
			return nil
		}
		// Function is metadata, not cross-version hard identity: older kernel
		// execute_end rows may expose only the stable work struct pointer.
		_ = function
		key := strings.Join([]string{fmt.Sprintf("%d", ev.PID), work, base}, "\x00")
		return []durationOrderObservation{{lane: lane(durationOrderWorkqueue, key), phase: phase, owner: ev.PID}}
	case EventDMAFence:
		driver, timeline, context, seqno := dmaFenceExactEndpointFields(ev)
		base, phaseName := dmaFenceBaseAndPhase(ev.Name)
		phase, ok := durationPairPhase(phaseName)
		if !ok || len(dmaFenceEndpointMissingFields(ev, driver, timeline, context, seqno)) > 0 {
			return nil
		}
		key := strings.Join([]string{fmt.Sprintf("%d", ev.PID), driver, timeline, context, seqno, base}, "\x00")
		return []durationOrderObservation{{lane: lane(durationOrderDMAFence, key), phase: phase, owner: ev.PID}}
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
		identity, phaseName, endpoint := genericStorageEndpoint(ev)
		phase, ok := durationPairPhase(phaseName)
		if !endpoint || !ok {
			return nil
		}
		return []durationOrderObservation{{lane: lane(durationOrderStorage, identity.laneKey()), phase: phase, owner: ev.PID}}
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
	"tracing_mark_write", ": print:", "workqueue_", "dma_fence",
	"irq_handler_", "softirq_", "ipi_", "sched_wakeup_new:", "sched_switch:",
	"cpu_frequency:", "cpu_frequency_limits:",
	"block_rq_issue:", "block_rq_complete:", "block_bio_queue:", "block_bio_complete:", "ufshcd_", "mmc_", "scsi_", "i2c_", "smbus_", "bio_", "ebpf_bio",
	"f2fs_", "hmfs_", "android_fs_", "ext4_", "erofs_", "z_erofs_", "filesystem", "file_system", "ebpf_file", "file_check_and_advance_wb_err", "filemap_set_wb_err",
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
	if limit <= 0 || limit > durationOrderFailureCap {
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
	return out, capped
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
	// lazily. Composite indexes are canonically sorted, but their child-local
	// physical failures were copied into the private fields above; scanning the
	// sorted slice can only add a cross-artifact canonical-lane failure.
	failures, capped := durationOrderFailuresFromEvents(idx.Events, q, durationOrderFailureCap)
	for i := range failures {
		failure := failures[i]
		if _, exists := out[failure.Family]; !exists {
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
	// lazily; composite children already contributed their physical poison to
	// the private failure ledger before the canonical merge sort.
	if idx.TimestampOrder != TraceTimestampOrderMonotonic {
		failures, capped := durationOrderFailuresFromEvents(idx.Events, q, durationOrderFailureCap)
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
