package tracequery

// causal_token_registry.go — SINGLE SOURCE OF TRUTH for the semantic lane of
// every causal type token (root-cause rank rows, critical-blocking rows,
// blocking-kind refinements, and the delivery/occupancy observation
// predicates). RN-16 (§7.9, docs/design/customer_dead_session_audit_20260703.md).
//
// ── Ruling anchor (READ BEFORE CHANGING ANY LANE) ────────────────────────────
//
// User adjudication 2026-07-04 (§7.4 CMP-10 demand/supply separation, restated
// for RN-14/RN-15): "runnable 等待 = 调度压力(需求积压);'算力'只留
// compute_supply 交付口径" — a runnable wait is DEMAND-side scheduling
// pressure and must never be published or worded as compute supply; the
// compute_supply family is reserved for the AGGREGATE delivery-side ledger
// (supply ratio / low-frequency loss / idle mismatch / core-limited). This
// separation was violated once in production (RN-15: the same 2.661/2.908ms
// runnable waits double-published as type=compute_supply with a per-thread
// subject) — this registry plus its construction guard exists so nobody can
// re-violate it silently.
//
// §7.5 final ruling (2026-07-04, item CLOSED): the supply_pressure WIRE TOKEN
// is retained verbatim; its demand-backlog semantics are owned by the display
// layer (runtimeTraceSupplyPressureDisplayLabel in internal/tool). Do not
// migrate the token; anyone re-proposing a rename reads §7.5 first.
//
// ── Change protocol ──────────────────────────────────────────────────────────
//
//  1. Read §7.4 / §7.5 / §7.9 of the ledger above before moving a token
//     between lanes or changing Additivity/Subject.
//  2. Any registry edit MUST be mirrored in the golden snapshot
//     (causal_token_registry_golden_test.go) — the diff is the review
//     surface, exactly like the 69-kind analysis golden.
//  3. New producer tokens: adding a construction site without a registry
//     entry panics under `go test` (assertCausalTokenRow) — register the
//     token, pick its lane per the rulings, then update the golden.
//  4. Display labels are NOT migrated here: the zh label stays owned by the
//     internal/tool helpers referenced in LabelZhRef; a tool-side pin keeps
//     that column and the helpers in lockstep.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// CausalTokenLane is the semantic lane a causal token belongs to. Only the
// SchedulingDemand / ComputeDelivery split is ruling-locked (§7.4); the other
// lanes are descriptive taxonomy so every token has exactly one home.
type CausalTokenLane string

const (
	// CausalLaneSchedulingDemand — demand-side backlog: the thread wanted a
	// CPU and queued for it (runnable waits, run-queue depth, priority
	// competition). Display wording: 调度压力/需求积压 — NEVER 算力 (§7.4).
	CausalLaneSchedulingDemand CausalTokenLane = "scheduling_demand"
	// CausalLaneComputeDelivery — supply side: how much compute the CPUs
	// actually delivered (frequency-weighted supply, low-frequency loss,
	// frequency caps). The ONLY lane whose display wording may use 算力 (§7.4).
	CausalLaneComputeDelivery CausalTokenLane = "compute_delivery"
	// CausalLaneWakeupChain — waiting to be signalled by a peer (sleep before
	// wakeup, missing wakeup edges, binder/IPC waits).
	CausalLaneWakeupChain CausalTokenLane = "wakeup_chain"
	// CausalLaneIOBlocking — device/filesystem waits and IO pressure.
	CausalLaneIOBlocking CausalTokenLane = "io_blocking"
	// CausalLaneIRQAggregate — interrupt/IPI activity aggregated per window.
	CausalLaneIRQAggregate CausalTokenLane = "irq_aggregate"
	// CausalLaneLockContention — monitor/lock holder-waiter contention.
	CausalLaneLockContention CausalTokenLane = "lock_contention"
	// CausalLaneCPUWork — the subject itself consumed CPU (running spans,
	// deterministic work classes such as JIT/verify/shader compilation).
	CausalLaneCPUWork CausalTokenLane = "cpu_work"
	// CausalLaneMemoryPressure — reclaim / page-fault / GC / page-cache churn.
	CausalLaneMemoryPressure CausalTokenLane = "memory_pressure"
	// CausalLaneDiagnostic — data-quality and corroboration rows (trace gaps,
	// unclassifiable states, churn clusters, kernel accounting echoes).
	CausalLaneDiagnostic CausalTokenLane = "diagnostic"
)

// CausalTokenAdditivity classifies what a token's ms/value field IS, i.e.
// what arithmetic is legal on it across rows.
type CausalTokenAdditivity string

const (
	// CausalAdditivityWallClockPerThread — per-subject wall clock. Never
	// summable across rows/layers (墙钟不可加和, v3 projection ruling).
	CausalAdditivityWallClockPerThread CausalTokenAdditivity = "wall_clock_per_thread"
	// CausalAdditivityCrossThreadCPUms — thread·ms / cpu·ms summed across
	// threads/CPUs. NOT wall clock: must never anchor the projection bar
	// scale nor enter any cross-row Σ face; cross-window comparison only
	// after CMP-9 normalization (value / window length).
	CausalAdditivityCrossThreadCPUms CausalTokenAdditivity = "cross_thread_cpu_ms"
	// CausalAdditivityCount — counts or count-derived advisory scalars (not a
	// physical duration even when printed with an ms suffix).
	CausalAdditivityCount CausalTokenAdditivity = "count"
)

// CausalTokenSubjectKind constrains what subject a token's rows may carry.
type CausalTokenSubjectKind string

const (
	// CausalSubjectPerThread — rows carry a concrete thread subject.
	CausalSubjectPerThread CausalTokenSubjectKind = "per_thread"
	// CausalSubjectAggregateOnly — the subject IS the window/CPU-scoped
	// metric; a concrete thread subject (pid>0 or non-empty comm) is a
	// registry violation (panic under test, WARN in prod).
	CausalSubjectAggregateOnly CausalTokenSubjectKind = "aggregate_only"
	// CausalSubjectEither — aggregate metric that may borrow a representative
	// thread (e.g. io_pressure surfacing the top file-IO thread).
	CausalSubjectEither CausalTokenSubjectKind = "either"
)

// LabelZhRef values: the internal/tool helper that owns the token's Chinese
// display label. Labels are deliberately NOT migrated into this registry —
// a tool-side pin (internal/tool/semantic_ruling_pins_test.go) keeps this
// column and the helpers in lockstep.
const (
	// CausalZhLabelRefRootCauseType — internal/tool/answer_document_mutation_
	// runtime_typelabels.go :: runtimeTraceRootCauseTypeZHLabel.
	CausalZhLabelRefRootCauseType = "runtimeTraceRootCauseTypeZHLabel"
	// CausalZhLabelRefSupplyPressure — internal/tool/answer_document_mutation_
	// runtime_typelabels.go :: runtimeTraceSupplyPressureDisplayLabel (§7.5:
	// wire token retained, demand-backlog wording owned by the display layer).
	CausalZhLabelRefSupplyPressure = "runtimeTraceSupplyPressureDisplayLabel"
	// Empty LabelZhRef = the token renders verbatim (no zh label helper).
)

// CausalTokenSpec is one registry row.
type CausalTokenSpec struct {
	Lane       CausalTokenLane
	Additivity CausalTokenAdditivity
	Subject    CausalTokenSubjectKind
	// RowToken: the token may be constructed as a root-cause rank item or a
	// critical-blocking candidate row. false = observation-predicate or
	// blocking-kind refinement only — constructing a causal ROW with it is a
	// registry violation.
	RowToken bool
	// LabelZhRef names the internal/tool helper owning the zh display label
	// ("" = verbatim). See the CausalZhLabelRef* constants.
	LabelZhRef string
}

// causalTokenRegistry — the exhaustive token universe. Sources (grep-audited
// 2026-07-04): rootCauseItem call sites + causalImpactRootType /
// aggregateRootCauseType / stateChurnRootCauseType / traceSpanSemanticWorkClass
// + chain RootEvidence producers + buildCriticalBlockingCallsFromStats +
// lock_contention blocking kinds + the runnable_occupancy (RN-1) and
// compute_supply_balance (CMP-10) observation predicates.
var causalTokenRegistry = map[string]CausalTokenSpec{
	// ── scheduling demand (调度压力/需求积压 — §7.4 ruling-locked) ──────────
	"runnable_wait":                    {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"fragmented_runnable_wait":         {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"scheduler_latency":                {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"priority_inversion_candidate":     {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"priority_inversion_runnable_wait": {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"cpu_affinity_or_cpuset":           {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	// cpu_pressure / supply_pressure: Σ runnable backlog across threads —
	// demand side despite the "supply" name (§7.4; §7.5 keeps the wire token).
	"cpu_pressure":    {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"supply_pressure": {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefSupplyPressure},
	// runnable_occupancy (RN-1): subject-keyed observation predicate — the
	// subject's runnable wall clock; occupier notes carry cpu·ms detail.
	"runnable_occupancy": {Lane: CausalLaneSchedulingDemand, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: false, LabelZhRef: ""},

	// ── compute delivery (算力交付 — §7.4 ruling-locked; the only lane whose
	// display wording may use 算力) ─────────────────────────────────────────
	// compute_supply: aggregate delivery-side ledger ONLY — RN-15 bans any
	// concrete per-thread subject (per-thread runnable waits ride the
	// scheduling-demand lane; per-thread low-frequency verdicts ride
	// low_frequency).
	"compute_supply":         {Lane: CausalLaneComputeDelivery, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"compute_supply_balance": {Lane: CausalLaneComputeDelivery, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: false, LabelZhRef: ""},
	"low_frequency":          {Lane: CausalLaneComputeDelivery, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"cpu_frequency_limit":    {Lane: CausalLaneComputeDelivery, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	// supply_fold_deficit (VS-2 §7.10): the per-node supply-fold accounting
	// riding wakeup_causal_impact / wakeup_causal_aggregate / root_cause
	// observations as typed notes (supply_fold_deficit_ms= / _ideal_ms= /
	// fold_basis=). Distinction from the compute_supply aggregate token
	// (§7.10 (5)): this IS the delivery lane's legal PER-THREAD form — a
	// wall-clock DECOMPOSITION of the subject thread's OWN running time
	// (running − big-cluster-fmax folded ideal, lower bound), never a
	// cross-thread sum — so the RN-15 aggregate-only rule does not apply.
	// Observation-note token only, never a rank row.
	"supply_fold_deficit": {Lane: CausalLaneComputeDelivery, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: false, LabelZhRef: ""},

	// ── wakeup chain (等待被唤醒/依赖对端) ──────────────────────────────────
	"sleep_wait":            {Lane: CausalLaneWakeupChain, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"fragmented_sleep_wait": {Lane: CausalLaneWakeupChain, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"missing_wakeup":        {Lane: CausalLaneWakeupChain, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
	"binder_wait":           {Lane: CausalLaneWakeupChain, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},

	// ── IO blocking ────────────────────────────────────────────────────────
	"io_wait":                       {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"d_state_or_io_wait":            {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"fragmented_d_state_or_io_wait": {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"io_latency":                    {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"io_burst_episode":              {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectEither, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"block_io_by_inode":             {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectEither, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	// file_io_hot_inode: advisory scalar derived from event counts + bytes
	// (fileIOAdvisoryImpactMs) — count-class, not a physical duration.
	"file_io_hot_inode": {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityCount, Subject: CausalSubjectEither, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"blocked_reason":    {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityCount, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
	// io_pressure: window-scoped pressure score in the CMP-3 cross-thread
	// display set; may borrow the top file-IO thread as representative.
	"io_pressure": {Lane: CausalLaneIOBlocking, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectEither, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},

	// ── CPU work (自身执行;wording 执行/算力 covers the consuming side and is
	// whitelisted where the user-adjudicated display cells already use it) ──
	"running":            {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"fragmented_running": {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"trace_span":         {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"jit_compile":        {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"class_verification": {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"shader_compile":     {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"runtime_compile":    {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"workqueue_activity": {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"dma_fence_activity": {Lane: CausalLaneCPUWork, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},

	// ── IRQ aggregate ──────────────────────────────────────────────────────
	"irq_burst":    {Lane: CausalLaneIRQAggregate, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"irq_activity": {Lane: CausalLaneIRQAggregate, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"ipi_activity": {Lane: CausalLaneIRQAggregate, Additivity: CausalAdditivityCrossThreadCPUms, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},

	// ── lock contention ────────────────────────────────────────────────────
	"blocking_span": {Lane: CausalLaneLockContention, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
	// monitor_contention / lock_contention are BlockingKind refinements on
	// blocking_span rows (§7.30.3 D1) — never row tokens themselves.
	"monitor_contention": {Lane: CausalLaneLockContention, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: false, LabelZhRef: ""},
	"lock_contention":    {Lane: CausalLaneLockContention, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: false, LabelZhRef: ""},

	// ── memory pressure ────────────────────────────────────────────────────
	// page_cache_churn impact is churn-count derived — count-class advisory.
	"page_cache_churn":  {Lane: CausalLaneMemoryPressure, Additivity: CausalAdditivityCount, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	"memory_reclaim":    {Lane: CausalLaneMemoryPressure, Additivity: CausalAdditivityCount, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: ""},
	"memory_page_fault": {Lane: CausalLaneMemoryPressure, Additivity: CausalAdditivityCount, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: ""},
	"memory_gc":         {Lane: CausalLaneMemoryPressure, Additivity: CausalAdditivityCount, Subject: CausalSubjectAggregateOnly, RowToken: true, LabelZhRef: ""},

	// ── diagnostic ─────────────────────────────────────────────────────────
	"trace_gap":     {Lane: CausalLaneDiagnostic, Additivity: CausalAdditivityCount, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
	"unknown_state": {Lane: CausalLaneDiagnostic, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
	"state_churn":   {Lane: CausalLaneDiagnostic, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: CausalZhLabelRefRootCauseType},
	// sched_stat_accounting corroborates scheduler intervals (discounted,
	// "not double-counted") — diagnostic echo, not an independent duration.
	"sched_stat_accounting": {Lane: CausalLaneDiagnostic, Additivity: CausalAdditivityWallClockPerThread, Subject: CausalSubjectPerThread, RowToken: true, LabelZhRef: ""},
}

// causalRegistryCrossThreadRowExceptions: RowToken tokens whose Additivity is
// cross_thread_cpu_ms but which are deliberately ABSENT from the two consumer
// sets (engine rootCauseAggregateMetricTypes + display
// runtimeTraceProjCrossThreadAggregateType) because production never emits
// them as aggregate rows today: the RN-15 guard kills the per-thread
// compute_supply face and computeSupplySummaries never builds a threadless
// row (the aggregate survivor is a pinned future lane, see
// compute_supply_runnable_rn15_test.go). Producing such rows for real
// requires adding the token to BOTH consumer sets and deleting it here — the
// interlock pins in semantic_ruling_pins_test.go force that conversation.
var causalRegistryCrossThreadRowExceptions = map[string]bool{
	"compute_supply": true,
}

// CausalTokenSpecFor returns the registry row for token (exact match on the
// canonical lowercase token — never a substring heuristic).
func CausalTokenSpecFor(token string) (CausalTokenSpec, bool) {
	spec, ok := causalTokenRegistry[token]
	return spec, ok
}

// CausalTokenUniverse returns every registered token, sorted.
func CausalTokenUniverse() []string {
	out := make([]string, 0, len(causalTokenRegistry))
	for token := range causalTokenRegistry {
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

// CausalTokenCrossThreadRowException reports whether token is on the
// documented exception list above.
func CausalTokenCrossThreadRowException(token string) bool {
	return causalRegistryCrossThreadRowExceptions[token]
}

// assertCausalTokenRow is the construction-funnel guard (RN-16 layer 1):
// every root-cause rank item and critical-blocking candidate passes through
// it. Under `go test` a violation panics at the construction site; in
// production it degrades to a WARN log line (precise signals only — the guard
// reads the typed token and the typed thread subject, never prose).
func assertCausalTokenRow(token string, thread ThreadRef, face string) {
	spec, ok := causalTokenRegistry[token]
	if !ok {
		causalTokenViolation(fmt.Sprintf("unregistered causal token %q constructed on the %s face — register it in internal/tracequery/causal_token_registry.go (read the header rulings first)", token, face))
		return
	}
	if !spec.RowToken {
		causalTokenViolation(fmt.Sprintf("causal token %q is observation/kind-only (RowToken=false) but was constructed as a %s row", token, face))
		return
	}
	if spec.Subject == CausalSubjectAggregateOnly && (thread.PID > 0 || strings.TrimSpace(thread.Comm) != "") {
		causalTokenViolation(fmt.Sprintf("causal token %q is subject=aggregate_only but was constructed with concrete thread subject %s on the %s face — §7.4 demand/supply separation (RN-15 shape)", token, threadLabel(thread), face))
	}
}

func causalTokenViolation(msg string) {
	msg = "tracequery: causal-token registry violation: " + msg
	if testing.Testing() {
		panic(msg)
	}
	logging.Warning("%s", msg)
}
