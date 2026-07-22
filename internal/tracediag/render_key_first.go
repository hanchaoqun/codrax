package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// detailRenderPolicy is deliberately an exact type+Go-field registry.  A
// Summary string, JSON key substring, or other noisy vocabulary must never
// decide that a fact is load-bearing. Fields registered here have one typed
// key-first renderer below and are therefore skipped by the generic detail
// walker to avoid publishing two rulers for the same dimension.
type detailRenderPolicy struct {
	skipped map[reflect.Type]map[string]bool
}

var nonEventDetailPolicy = detailRenderPolicy{skipped: map[reflect.Type]map[string]bool{
	reflect.TypeOf(tracequery.TimelineResult{}): {
		"HeadState": true, "IntegrityFailure": true, "Caveats": true,
	},
	reflect.TypeOf(tracequery.WindowStats{}): {
		"SchedulerHeadCoverage": true, "Caveats": true,
	},
	reflect.TypeOf(tracequery.TraceCounterQualitySummary{}): {
		"Rows": true, "ValidIdentityRows": true, "NumericRows": true,
		"InvalidRows": true, "NonNumericRows": true, "DerivedInvalidSeries": true,
		"TotalSeries": true, "TotalSeriesStatus": true, "PublishedSeries": true,
		"SuppressedSeries": true, "TruncatedSeries": true, "SeriesBudget": true,
		"SeriesBudgetExceeded": true, "OverflowRows": true, "BaselinePolicy": true,
		"UnitPolicy": true,
	},
	reflect.TypeOf(tracequery.PerfQualitySummary{}): {
		"CPUKnownCount": true, "CPUUnknownCount": true,
		"CallchainKnownCount": true, "CallchainUnknownCount": true,
		"InputIntegrityIssues": true, "ParserCaveats": true, "Caveats": true,
	},
	reflect.TypeOf(tracequery.StorageLatencySummary{}): {
		"PairedCount": true, "UnpairedStartCount": true, "UnpairedDoneCount": true,
		"AmbiguousCohortCount": true, "PairingSuppressedCount": true,
	},
	reflect.TypeOf(tracequery.InterruptActivity{}): {
		"PairedCount": true,
	},
	reflect.TypeOf(tracequery.WorkqueueActivity{}): {
		"PairedCount": true, "UnpairedStartCount": true, "UnpairedDoneCount": true,
		"AmbiguousCohortCount": true, "PairingSuppressedCount": true,
	},
	reflect.TypeOf(tracequery.DMAFenceActivity{}): {
		"PairedCount": true, "UnpairedStartCount": true, "UnpairedDoneCount": true,
		"AmbiguousCohortCount": true, "PairingSuppressedCount": true,
	},
	reflect.TypeOf(tracequery.RootCauseRankItem{}): {
		"GatedCapabilitySource": true, "GatedClusterTopology": true,
	},
	reflect.TypeOf(tracequery.WakeupCausalImpact{}): {
		"GatedCapabilitySource": true, "GatedClusterTopology": true,
	},
	reflect.TypeOf(tracequery.WakeupCausalAggregate{}): {
		"GatedAggregationCaliber": true, "GatedCapabilitySource": true,
		"GatedClusterTopology": true,
	},
	reflect.TypeOf(tracequery.SupplyFoldBasis{}): {
		"CapabilitySource": true, "CapabilitySplitAudit": true,
		"ClusterTopologySource": true,
	},

	// Closed diagnostic carriers. collectNonEventEngineDiagnostics reads these
	// exact fields before detail; skipping them here prevents nested duplicates.
	reflect.TypeOf(tracequery.SchedulerLatencyResult{}): {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.IPCGraphResult{}):         {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.ChainResult{}):            {"Caveats": true},
	reflect.TypeOf(tracequery.FramePipelineResult{}):    {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.FrameTimelineResult{}):    {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.CriticalBlockingResult{}): {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.RootCauseRankResult{}):    {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.FrameRootCauseBundle{}):   {"Caveats": true},
	reflect.TypeOf(tracequery.FrameTargetResolution{}):  {"Caveats": true},
	reflect.TypeOf(tracequery.InteractionStatsResult{}): {"Caveats": true, "Compactions": true},
	reflect.TypeOf(tracequery.PerfTimelineResult{}):     {"Caveats": true},
	reflect.TypeOf(tracequery.WindowSweepResult{}):      {"Caveats": true},
	reflect.TypeOf(tracequery.RecipeResult{}):           {"Caveats": true},
	reflect.TypeOf(tracequery.CPUOccupancyStats{}):      {"Caveats": true},
	reflect.TypeOf(tracequery.ProcessDomainCensus{}):    {"Caveats": true},
	reflect.TypeOf(tracequery.ComputeSupplyBalance{}):   {"Caveats": true},
	reflect.TypeOf(tracequery.BinderWaitSummary{}):      {"Caveats": true},
	reflect.TypeOf(tracequery.IPCEdge{}):                {"Caveats": true},
}}

func policySkipsDetailField(policy *detailRenderPolicy, typ reflect.Type, field string) bool {
	return policy != nil && policy.skipped[typ][field]
}

// orderedDetailFieldIndexes keeps compact structs/pointers ahead of slices,
// arrays and maps. This is a structural bulk rule, not a field-name heuristic;
// the schema pins below make a newly added field fail review until its
// key-first semantics have been adjudicated.
func orderedDetailFieldIndexes(v reflect.Value, policy *detailRenderPolicy) []int {
	if policy == nil {
		indexes := make([]int, v.NumField())
		for i := range indexes {
			indexes[i] = i
		}
		return indexes
	}
	regular := make([]int, 0, v.NumField())
	bulk := make([]int, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		switch v.Field(i).Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			bulk = append(bulk, i)
		default:
			regular = append(regular, i)
		}
	}
	return append(regular, bulk...)
}

type nonEventDiagnosticKind uint8

const (
	nonEventDiagnosticCaveat nonEventDiagnosticKind = iota + 1
	nonEventDiagnosticCompaction
)

type nonEventEngineDiagnostic struct {
	kind       nonEventDiagnosticKind
	source     string
	caveat     string
	compaction tracequery.ViewCompaction
}

func rawPerfCaptureCaveats(caveats []string) []string {
	return tracequery.RawPerfCaptureCompletenessCaveats(caveats)
}

// rawPerfCaptureKeyFirstLine compacts the engine's fixed machine-token caveat
// without interpreting any declared number. The full Result caveat remains in
// the JSON/tool payload; tracediag must keep one bounded, key-first customer
// line that preserves the exact-zero/not-reported/unknown distinction and the
// inventory analysis boundary. entry=i/n makes a general report-line cap
// explicit when a bundle legitimately contains several raw perf artifacts.
func rawPerfCaptureKeyFirstLine(caveat string, ordinal, total int) string {
	values := rawPerfCaptureCaveatValues(caveat)
	parts := []string{fmt.Sprintf("entry=%d/%d", ordinal, total)}
	appendAs := func(label, key string) {
		if value := values[key]; value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	if values["valid"] == "false" {
		appendAs("valid", "valid")
		appendAs("applicability", "applicability")
		appendAs("reason", "reason")
		appendAs("authority", "authority")
		appendAs("gate", "capture_hard_gate")
		appendAs("absence", "absence_policy")
		return "- key_first.perf_capture: " + clampToken(strings.Join(parts, " "))
	}
	appendAs("state", "capture_state")
	appendAs("ready", "query_ready")
	appendAs("issue", "capture_quality_issue")
	appendAs("clock", "effective_clock_evidence")
	if scope := values["census_scope"]; scope != "" {
		if scope == "observed_perf_record_stream" {
			scope = "record_stream_only"
		}
		parts = append(parts, "scope="+scope)
	}
	appendAs("device", "device_capture_completeness")
	compactRecord := func(value string) string {
		// The validator already proves physical=accepted+rejected. Publishing
		// a/r preserves the complete independent record census while avoiding
		// a redundant third uint64 that could force worst-case line truncation.
		var accepted, rejected string
		for _, field := range strings.Split(value, ",") {
			switch {
			case strings.HasPrefix(field, "accepted:"):
				accepted = "a" + strings.TrimPrefix(field, "accepted:")
			case strings.HasPrefix(field, "rejected:"):
				rejected = "r" + strings.TrimPrefix(field, "rejected:")
			}
		}
		if accepted == "" || rejected == "" {
			return ""
		}
		return strings.TrimPrefix(accepted, "a") + "/" + strings.TrimPrefix(rejected, "r")
	}
	appendRecordAndTotal := func(label, recordKey, totalLabel, totalKey string) {
		record := compactRecord(values[recordKey])
		totalValue := values[totalKey]
		switch {
		case record != "" && totalValue != "":
			parts = append(parts, label+"="+record+"|"+totalLabel+":"+totalValue)
		case record != "":
			parts = append(parts, label+"="+record)
		case totalValue != "":
			parts = append(parts, label+"="+totalLabel+":"+totalValue)
		}
	}
	parts = append(parts, "rec=a/r")
	if record := compactRecord(values["sample_records"]); record != "" {
		parts = append(parts, "s="+record)
	}
	appendRecordAndTotal("l", "lost_records", "events", "lost_events")
	appendRecordAndTotal("ls", "lost_sample_records", "samples", "lost_samples")
	appendRecordAndTotal("x", "aux_records", "bytes", "aux_bytes")
	appendAs("auth", "authority")
	appendAs("gate", "capture_hard_gate")
	if absence := values["absence_policy"]; absence != "" {
		if absence == "require_quality_caveat" {
			absence = "must_qualify"
		}
		parts = append(parts, "absence="+absence)
	}
	return "- key_first.perf_capture: " + clampToken(strings.Join(parts, " "))
}

func rawPerfCaptureCaveatValues(caveat string) map[string]string {
	values := make(map[string]string)
	for _, field := range strings.Fields(strings.TrimSpace(caveat)) {
		key, value, ok := strings.Cut(field, "=")
		if ok && key != "" && value != "" {
			values[key] = value
		}
	}
	return values
}

// rawPerfCaptureHeaderToken is the no-extra-line fallback for the minimum
// event-search budget. It intentionally carries only the decision-critical
// state, loss totals and scope; the full one-caveat-per-artifact roster
// remains available through trace_query/tool payloads.
func rawPerfCaptureHeaderToken(caveat string, ordinal, total int) string {
	values := rawPerfCaptureCaveatValues(caveat)
	parts := []string{fmt.Sprintf("entry=%d/%d", ordinal, total)}
	appendAs := func(label, key string) {
		if value := values[key]; value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	appendAs("valid", "valid")
	if values["valid"] == "false" {
		appendAs("reason", "reason")
		return clampToken(strings.Join(parts, ","))
	}
	appendAs("state", "capture_state")
	appendAs("ready", "query_ready")
	appendAs("issue", "capture_quality_issue")
	appendAs("lost_events", "lost_events")
	appendAs("lost_samples", "lost_samples")
	appendAs("aux_bytes", "aux_bytes")
	appendAs("scope", "census_scope")
	appendAs("device_complete", "device_capture_completeness")
	return clampToken(strings.Join(parts, ","))
}

func renderRawPerfCaptureKeyFirst(res *tracequery.Result, emit func(string)) {
	if res == nil {
		return
	}
	caveats := rawPerfCaptureCaveats(res.Caveats)
	for i, caveat := range caveats {
		emit(rawPerfCaptureKeyFirstLine(caveat, i+1, len(caveats)))
	}
}

func collectNonEventEngineDiagnostics(res *tracequery.Result) []nonEventEngineDiagnostic {
	if res == nil {
		return nil
	}
	var out []nonEventEngineDiagnostic
	seen := map[string]bool{}
	add := func(source string, caveats []string, compactions []tracequery.ViewCompaction) {
		for _, caveat := range caveats {
			if tracequery.IsRawPerfCaptureCompletenessCaveat(caveat) {
				continue
			}
			key := "c|" + caveat
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, nonEventEngineDiagnostic{kind: nonEventDiagnosticCaveat, source: source, caveat: caveat})
		}
		for _, compaction := range compactions {
			key := fmt.Sprintf("x|%s|%s|%d|%d|%s|%d", compaction.View, compaction.Dimension,
				compaction.Total, compaction.Emitted, formatSecondsToken(compaction.LastEmittedTs), compaction.LastEmittedLine)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, nonEventEngineDiagnostic{kind: nonEventDiagnosticCompaction, source: source, compaction: compaction})
		}
	}
	addPerf := func(source string, perf *tracequery.PerfContext) {
		if perf != nil && perf.Quality != nil {
			add(source+".quality", perf.Quality.Caveats, nil)
		}
		if perf != nil {
			for i := range perf.Cohorts {
				if perf.Cohorts[i].Quality != nil {
					add(fmt.Sprintf("%s.cohorts[%d].quality", source, i), perf.Cohorts[i].Quality.Caveats, nil)
				}
			}
		}
	}
	var addChain func(string, *tracequery.ChainResult)
	addChain = func(source string, chain *tracequery.ChainResult) {
		if chain == nil {
			return
		}
		add(source, chain.Caveats, nil)
		for i := range chain.BinderWaits {
			add(fmt.Sprintf("%s.binder_waits[%d]", source, i), chain.BinderWaits[i].Caveats, nil)
		}
	}
	addRank := func(source string, rank *tracequery.RootCauseRankResult) {
		if rank == nil {
			return
		}
		add(source, rank.Caveats, rank.Compactions)
		for i := range rank.Items {
			addPerf(fmt.Sprintf("%s.items[%d].perf_context", source, i), rank.Items[i].PerfContext)
			for j := range rank.Items[i].PerfContexts {
				addPerf(fmt.Sprintf("%s.items[%d].perf_contexts[%d]", source, i, j), rank.Items[i].PerfContexts[j].PerfContext)
			}
		}
	}
	addWindow := func(source string, stats *tracequery.WindowStats) {
		if stats == nil {
			return
		}
		add(source, stats.Caveats, nil)
		if stats.CPUOccupancy != nil {
			add(source+".cpu_occupancy", stats.CPUOccupancy.Caveats, nil)
		}
		if stats.ProcessDomainCensus != nil {
			add(source+".process_domain_census", stats.ProcessDomainCensus.Caveats, nil)
		}
		if stats.ComputeSupplyBalance != nil {
			add(source+".compute_supply_balance", stats.ComputeSupplyBalance.Caveats, nil)
		}
		addPerf(source+".perf_samples", stats.PerfSamples)
	}

	add("result", res.Caveats, res.Compactions)
	if res.Timeline != nil {
		add("timeline", res.Timeline.Caveats, nil)
	}
	addWindow("window_stats", res.WindowStats)
	if res.SchedulerLatency != nil {
		add("scheduler_latency_stats", res.SchedulerLatency.Caveats, res.SchedulerLatency.Compactions)
	}
	if res.IPCGraph != nil {
		add("ipc_graph", res.IPCGraph.Caveats, res.IPCGraph.Compactions)
		for i := range res.IPCGraph.Edges {
			add(fmt.Sprintf("ipc_graph.edges[%d]", i), res.IPCGraph.Edges[i].Caveats, nil)
		}
	}
	addChain("wakeup_chain", res.WakeupChain)
	if res.FramePipeline != nil {
		add("frame_pipeline", res.FramePipeline.Caveats, res.FramePipeline.Compactions)
	}
	if res.FrameTimeline != nil {
		add("frame_timeline", res.FrameTimeline.Caveats, res.FrameTimeline.Compactions)
	}
	if res.CriticalBlocking != nil {
		add("critical_blocking_calls", res.CriticalBlocking.Caveats, res.CriticalBlocking.Compactions)
	}
	addRank("root_cause_rank", res.RootCauseRank)
	if bundle := res.FrameRootCauseBundle; bundle != nil {
		add("frame_root_cause_bundle", bundle.Caveats, nil)
		if bundle.TargetResolution != nil {
			add("frame_root_cause_bundle.target_resolution", bundle.TargetResolution.Caveats, nil)
		}
		addChain("frame_root_cause_bundle.wakeup_chain", bundle.WakeupChain)
		addRank("frame_root_cause_bundle.root_cause_rank", bundle.RootCauseRank)
		if bundle.FrameTimeline != nil {
			add("frame_root_cause_bundle.frame_timeline", bundle.FrameTimeline.Caveats, bundle.FrameTimeline.Compactions)
		}
		if bundle.CriticalBlocking != nil {
			add("frame_root_cause_bundle.critical_blocking_calls", bundle.CriticalBlocking.Caveats, bundle.CriticalBlocking.Compactions)
		}
		addPerf("frame_root_cause_bundle.perf_samples", bundle.PerfSamples)
		addPerf("frame_root_cause_bundle.target_running_perf", bundle.TargetRunningPerf)
		addPerf("frame_root_cause_bundle.on_chain_perf", bundle.OnChainPerf)
		addPerf("frame_root_cause_bundle.binder_peer_perf", bundle.BinderPeerPerf)
		addPerf("frame_root_cause_bundle.same_cpu_competitor_perf", bundle.SameCPUCompetitorPerf)
	}
	if res.InteractionStats != nil {
		add("interaction_stats", res.InteractionStats.Caveats, res.InteractionStats.Compactions)
	}
	addPerf("perf_stats", res.PerfStats)
	if res.PerfTimeline != nil {
		add("perf_timeline", res.PerfTimeline.Caveats, nil)
	}
	if res.WindowSweep != nil {
		add("window_sweep", res.WindowSweep.Caveats, nil)
	}
	if res.Recipe != nil {
		add("recipe", res.Recipe.Caveats, nil)
	}
	return out
}

func countNonEventEngineDiagnostics(diagnostics []nonEventEngineDiagnostic) (caveats, compactions int) {
	for _, diagnostic := range diagnostics {
		if diagnostic.kind == nonEventDiagnosticCaveat {
			caveats++
		} else {
			compactions++
		}
	}
	return caveats, compactions
}

func renderNonEventEngineDiagnostics(diagnostics []nonEventEngineDiagnostic, emit func(string)) {
	for _, diagnostic := range diagnostics {
		switch diagnostic.kind {
		case nonEventDiagnosticCaveat:
			emit(fmt.Sprintf("- engine_caveat source=%s | %s", diagnostic.source, clampToken(diagnostic.caveat)))
		case nonEventDiagnosticCompaction:
			comp := diagnostic.compaction
			emit(fmt.Sprintf("- 引擎截断记录: source=%s view=%s dimension=%s total=%d emitted=%d last_ts=%s last_line=%d",
				diagnostic.source, comp.View, comp.Dimension, comp.Total, comp.Emitted,
				formatSecondsToken(comp.LastEmittedTs), comp.LastEmittedLine))
		}
	}
}

func renderNonEventKeyFirstSummaries(res *tracequery.Result, emit func(string)) {
	if res == nil {
		return
	}
	if timeline := res.Timeline; timeline != nil && (timeline.HeadState != nil || timeline.IntegrityFailure != "") {
		parts := []string{}
		if head := timeline.HeadState; head != nil {
			parts = append(parts,
				"head_status="+head.Status,
				"boundary_ts="+formatSecondsToken(head.BoundaryTs),
				"state="+string(head.State),
				"actual_start_ts="+formatSecondsToken(head.ActualStartTs),
				"source_line="+strconv.Itoa(head.SourceLine),
				"reason="+strconv.Quote(head.Reason),
			)
		}
		if timeline.IntegrityFailure != "" {
			parts = append(parts, "integrity_failure="+strconv.Quote(timeline.IntegrityFailure))
		}
		emit("- key_first.completeness timeline: " + clampToken(strings.Join(parts, " ")))
	}
	if stats := res.WindowStats; stats != nil {
		if coverage := stats.SchedulerHeadCoverage; coverage != nil {
			emit(fmt.Sprintf("- key_first.completeness window_stats: status=%s boundary_ts=%s missing_cpus=%d%s missing_threads=%d%s reason=%s",
				coverage.Status, formatSecondsToken(coverage.BoundaryTs), coverage.MissingCPUCount,
				formatIntRoster(coverage.MissingCPUs), coverage.MissingThreadCount,
				formatIntRoster(coverage.MissingThreadPIDs), strconv.Quote(clampToken(coverage.Reason))))
		}
		if quality := stats.CounterQuality; quality != nil {
			emit(fmt.Sprintf("- key_first.counter_quality window_stats: rows=%d valid_identity_rows=%d numeric_rows=%d invalid_rows=%d non_numeric_rows=%d derived_invalid_series=%d total_series=%d total_series_status=%s published_series=%d suppressed_series=%d truncated_series=%d series_budget=%d series_budget_exceeded=%t overflow_rows=%d baseline_policy=%s unit_policy=%s issues=%d",
				quality.Rows, quality.ValidIdentityRows, quality.NumericRows, quality.InvalidRows,
				quality.NonNumericRows, quality.DerivedInvalidSeries, quality.TotalSeries,
				quality.TotalSeriesStatus, quality.PublishedSeries, quality.SuppressedSeries,
				quality.TruncatedSeries, quality.SeriesBudget, quality.SeriesBudgetExceeded,
				quality.OverflowRows, quality.BaselinePolicy, quality.UnitPolicy, len(quality.Issues)))
		}
		if line := windowPairingSummary("window_stats", stats.StorageLatencyByLayer,
			stats.IRQActivity, stats.SoftIRQActivity, stats.IPIActivity,
			stats.WorkqueueActivity, stats.DMAFenceActivity); line != "" {
			emit(line)
		}
	}
	if bundle := res.FrameRootCauseBundle; bundle != nil {
		if line := windowPairingSummary("frame_root_cause_bundle", nil,
			bundle.IRQActivity, bundle.SoftIRQActivity, nil,
			bundle.WorkqueueActivity, bundle.DMAFenceActivity); line != "" {
			emit(line)
		}
	}
	if capability := collectCapabilityAudit(res); capability != nil {
		emit(clampToken(capability.render()))
	}
	renderPerfQualitySummaries(res, emit)
}

func formatIntRoster(values []int) string {
	if len(values) == 0 {
		return ""
	}
	const limit = 8
	shown := minInt(len(values), limit)
	parts := make([]string, shown)
	for i, value := range values[:shown] {
		parts[i] = strconv.Itoa(value)
	}
	return fmt.Sprintf("(shown=%d/%d)[%s]", shown, len(values), strings.Join(parts, ","))
}

type pairingTotals struct {
	groups, paired, unpairedStart, unpairedDone, ambiguous, suppressed int
}

func (p pairingTotals) token(full bool) string {
	if full {
		return fmt.Sprintf("groups=%d paired=%d unpaired_start=%d unpaired_done=%d ambiguous=%d suppressed=%d",
			p.groups, p.paired, p.unpairedStart, p.unpairedDone, p.ambiguous, p.suppressed)
	}
	return fmt.Sprintf("groups=%d paired=%d", p.groups, p.paired)
}

func windowPairingSummary(source string, storage []tracequery.StorageLatencySummary,
	irq, softIRQ, ipi []tracequery.InterruptActivity,
	workqueue []tracequery.WorkqueueActivity, dma []tracequery.DMAFenceActivity) string {
	parts := []string{}
	storageTotals := pairingTotals{groups: len(storage)}
	for _, item := range storage {
		storageTotals.paired += item.PairedCount
		storageTotals.unpairedStart += item.UnpairedStartCount
		storageTotals.unpairedDone += item.UnpairedDoneCount
		storageTotals.ambiguous += item.AmbiguousCohortCount
		storageTotals.suppressed += item.PairingSuppressedCount
	}
	if storageTotals.groups > 0 {
		parts = append(parts, "storage={"+storageTotals.token(true)+"}")
	}
	addInterrupt := func(name string, items []tracequery.InterruptActivity) {
		if len(items) == 0 {
			return
		}
		totals := pairingTotals{groups: len(items)}
		for _, item := range items {
			totals.paired += item.PairedCount
		}
		parts = append(parts, name+"={"+totals.token(false)+"}")
	}
	addInterrupt("irq", irq)
	addInterrupt("softirq", softIRQ)
	addInterrupt("ipi", ipi)
	workqueueTotals := pairingTotals{groups: len(workqueue)}
	for _, item := range workqueue {
		workqueueTotals.paired += item.PairedCount
		workqueueTotals.unpairedStart += item.UnpairedStartCount
		workqueueTotals.unpairedDone += item.UnpairedDoneCount
		workqueueTotals.ambiguous += item.AmbiguousCohortCount
		workqueueTotals.suppressed += item.PairingSuppressedCount
	}
	if workqueueTotals.groups > 0 {
		parts = append(parts, "workqueue={"+workqueueTotals.token(true)+"}")
	}
	dmaTotals := pairingTotals{groups: len(dma)}
	for _, item := range dma {
		dmaTotals.paired += item.PairedCount
		dmaTotals.unpairedStart += item.UnpairedStartCount
		dmaTotals.unpairedDone += item.UnpairedDoneCount
		dmaTotals.ambiguous += item.AmbiguousCohortCount
		dmaTotals.suppressed += item.PairingSuppressedCount
	}
	if dmaTotals.groups > 0 {
		parts = append(parts, "dma_fence={"+dmaTotals.token(true)+"}")
	}
	if len(parts) == 0 {
		return ""
	}
	return "- key_first.pairing " + source + ": " + clampToken(strings.Join(parts, " "))
}

func renderPerfQualitySummaries(res *tracequery.Result, emit func(string)) {
	type qualityRef struct {
		path    string
		quality *tracequery.PerfQualitySummary
	}
	refs := []qualityRef{}
	seen := map[*tracequery.PerfQualitySummary]bool{}
	add := func(path string, perf *tracequery.PerfContext) {
		if perf == nil || perf.Quality == nil || seen[perf.Quality] {
			return
		}
		seen[perf.Quality] = true
		refs = append(refs, qualityRef{path: path, quality: perf.Quality})
	}
	addRank := func(path string, rank *tracequery.RootCauseRankResult) {
		if rank == nil {
			return
		}
		addItems := func(prefix string, items []tracequery.RootCauseRankItem) {
			for i := range items {
				add(fmt.Sprintf("%s[%d].perf_context", prefix, i), items[i].PerfContext)
				for j := range items[i].PerfContexts {
					add(fmt.Sprintf("%s[%d].perf_contexts[%d]", prefix, i, j), items[i].PerfContexts[j].PerfContext)
				}
			}
		}
		addItems(path+".items", rank.Items)
		addItems(path+".absorbed_items", rank.AbsorbedItems)
	}
	add("perf_stats", res.PerfStats)
	if res.WindowStats != nil {
		add("window_stats.perf_samples", res.WindowStats.PerfSamples)
	}
	if bundle := res.FrameRootCauseBundle; bundle != nil {
		add("frame_root_cause_bundle.perf_samples", bundle.PerfSamples)
		add("frame_root_cause_bundle.target_running_perf", bundle.TargetRunningPerf)
		add("frame_root_cause_bundle.on_chain_perf", bundle.OnChainPerf)
		add("frame_root_cause_bundle.binder_peer_perf", bundle.BinderPeerPerf)
		add("frame_root_cause_bundle.same_cpu_competitor_perf", bundle.SameCPUCompetitorPerf)
		addRank("frame_root_cause_bundle.root_cause_rank", bundle.RootCauseRank)
	}
	addRank("root_cause_rank", res.RootCauseRank)
	const maxShown = 4
	shown := minInt(len(refs), maxShown)
	if len(refs) > 0 {
		emit(fmt.Sprintf("- key_first.perf_quality: shown=%d/%d", shown, len(refs)))
	}
	for _, ref := range refs[:shown] {
		quality := ref.quality
		emit(fmt.Sprintf("- key_first.perf_quality %s: cpu_known=%d cpu_unknown=%d callchain_known=%d callchain_unknown=%d sources=%d input_integrity_issues=%d input_integrity=%s parser_caveats=%d parser_caveat=%s symbolization_statuses=%d sample_kinds=%d weight_units=%d clocks=%d clock_confidences=%d callchain_statuses=%d caveats=%d",
			ref.path, quality.CPUKnownCount, quality.CPUUnknownCount,
			quality.CallchainKnownCount, quality.CallchainUnknownCount,
			len(quality.Sources), len(quality.InputIntegrityIssues), perfIntegrityValueToken(quality.InputIntegrityIssues, 4),
			len(quality.ParserCaveats), perfIntegrityValueToken(quality.ParserCaveats, 2),
			len(quality.SymbolizationStatuses), len(quality.SampleKinds),
			len(quality.WeightUnits), len(quality.Clocks), len(quality.ClockConfidences),
			len(quality.CallchainStatuses), len(quality.Caveats)))
	}
}

func perfIntegrityValueToken(values []tracequery.PerfValueCount, max int) string {
	if len(values) == 0 {
		return "none"
	}
	if max <= 0 || max > len(values) {
		max = len(values)
	}
	parts := make([]string, 0, max+1)
	for _, value := range values[:max] {
		parts = append(parts, fmt.Sprintf("%s:%d", clampToken(value.Value), value.SampleCount))
	}
	if len(values) > max {
		parts = append(parts, fmt.Sprintf("omitted:%d", len(values)-max))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type capabilityAudit struct {
	carriers     map[string]bool
	sources      map[string]bool
	topologies   map[string]bool
	splitAudits  map[string]bool
	aggregations map[string]bool
}

func newCapabilityAudit() *capabilityAudit {
	return &capabilityAudit{
		carriers: map[string]bool{}, sources: map[string]bool{}, topologies: map[string]bool{},
		splitAudits: map[string]bool{}, aggregations: map[string]bool{},
	}
}

func (a *capabilityAudit) add(path, source, topology, split, aggregation string) {
	if source == "" && topology == "" && split == "" && aggregation == "" {
		return
	}
	a.carriers[path] = true
	if source != "" {
		a.sources[source] = true
	}
	if topology != "" {
		a.topologies[topology] = true
	}
	if split != "" {
		a.splitAudits[split] = true
	}
	if aggregation != "" {
		a.aggregations[aggregation] = true
	}
}

func (a *capabilityAudit) addBasis(path string, basis *tracequery.SupplyFoldBasis) {
	if basis == nil {
		return
	}
	a.add(path, basis.CapabilitySource, basis.ClusterTopologySource, basis.CapabilitySplitAudit, "")
}

func collectCapabilityAudit(res *tracequery.Result) *capabilityAudit {
	audit := newCapabilityAudit()
	seenRanks := map[*tracequery.RootCauseRankResult]bool{}
	seenChains := map[*tracequery.ChainResult]bool{}
	addRank := func(path string, rank *tracequery.RootCauseRankResult) {
		if rank == nil || seenRanks[rank] {
			return
		}
		seenRanks[rank] = true
		addItems := func(prefix string, items []tracequery.RootCauseRankItem) {
			for i := range items {
				itemPath := fmt.Sprintf("%s[%d]", prefix, i)
				audit.add(itemPath, items[i].GatedCapabilitySource, items[i].GatedClusterTopology, "", "")
				audit.addBasis(itemPath+".supply_fold_basis", items[i].SupplyFoldBasis)
			}
		}
		addItems(path+".items", rank.Items)
		addItems(path+".absorbed_items", rank.AbsorbedItems)
	}
	addChain := func(path string, chain *tracequery.ChainResult) {
		if chain == nil || seenChains[chain] {
			return
		}
		seenChains[chain] = true
		for i := range chain.CausalImpacts {
			itemPath := fmt.Sprintf("%s.causal_impacts[%d]", path, i)
			impact := &chain.CausalImpacts[i]
			audit.add(itemPath, impact.GatedCapabilitySource, impact.GatedClusterTopology, "", "")
			audit.addBasis(itemPath+".supply_fold_basis", impact.SupplyFoldBasis)
		}
		// ChainNode.Impact is the legacy/embedded carrier. Modern results also
		// publish CausalImpacts, so read nodes only as an exact typed fallback;
		// traversing both would double-count one projection in the carrier tally.
		if len(chain.CausalImpacts) == 0 {
			for i := range chain.Nodes {
				impact := chain.Nodes[i].Impact
				if impact == nil {
					continue
				}
				itemPath := fmt.Sprintf("%s.nodes[%d].impact", path, i)
				audit.add(itemPath, impact.GatedCapabilitySource, impact.GatedClusterTopology, "", "")
				audit.addBasis(itemPath+".supply_fold_basis", impact.SupplyFoldBasis)
			}
		}
		for i := range chain.AggregatedImpacts {
			itemPath := fmt.Sprintf("%s.aggregated_impacts[%d]", path, i)
			impact := &chain.AggregatedImpacts[i]
			audit.add(itemPath, impact.GatedCapabilitySource, impact.GatedClusterTopology, "", impact.GatedAggregationCaliber)
			audit.addBasis(itemPath+".supply_fold_basis", impact.SupplyFoldBasis)
		}
	}
	addRank("root_cause_rank", res.RootCauseRank)
	addChain("wakeup_chain", res.WakeupChain)
	if bundle := res.FrameRootCauseBundle; bundle != nil {
		addRank("frame_root_cause_bundle.root_cause_rank", bundle.RootCauseRank)
		addChain("frame_root_cause_bundle.wakeup_chain", bundle.WakeupChain)
	}
	if len(audit.carriers) == 0 {
		return nil
	}
	return audit
}

func (a *capabilityAudit) render() string {
	return fmt.Sprintf("- key_first.capability: carriers=%d capability_source=%s cluster_topology=%s capability_split_audit=%s gated_aggregation_caliber=%s",
		len(a.carriers), renderStringSet(a.sources, 8), renderStringSet(a.topologies, 8),
		renderStringSet(a.splitAudits, 3), renderStringSet(a.aggregations, 8))
}

func renderStringSet(values map[string]bool, limit int) string {
	tokens := make([]string, 0, len(values))
	for token := range values {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	shown := minInt(len(tokens), limit)
	quoted := make([]string, shown)
	for i := 0; i < shown; i++ {
		quoted[i] = strconv.Quote(clampToken(tokens[i]))
	}
	return fmt.Sprintf("shown=%d/%d[%s]", shown, len(tokens), strings.Join(quoted, ","))
}

// The policy is intentionally coupled to these schemas. A new exported JSON
// field in a load-bearing carrier must fail a mechanical pin so its priority,
// duplication and bulk-lane semantics are reviewed instead of inheriting an
// accidental declaration-order default.
var nonEventPrioritySchemaPins = map[reflect.Type]string{
	// §29.27② 常态发布 (SMR-1 修复轮 引擎件①, 2026-07-13): Result gained the
	// top-level TargetWindowStates slot (non-bundle runs only — the bundle
	// path keeps its own copy, so no run ever carries two). Generic detail
	// rendering (small typed account, no bulk lane, no dup channel) — same
	// treatment as the bundle copy; hash re-pinned after review.
	// SA-F2 (DISPATCH-IND 批4, 2026-07-14) schema review (R2' 第 7 处):
	// Result gained VsyncGeneratorCensus (event_search matched-rows caliber
	// — per-generator counts + authoritative period-print parse). Key-first
	// adjudication: small typed detail rows, no Caveats/Compactions channel
	// to dedupe, no bulk lane, no priority override — generic detail
	// rendering, CPUFrequencyCensus 同构; hash re-pinned after review.
	// SUPP-CANCEL (2026-07-14) schema review (R2' 第 7 处): Result gained
	// ViewCancellation — the typed in-view cooperative-cancellation record
	// (view/reason/scanned_units/discarded_faces). Key-first adjudication:
	// tracediag NEVER mints it by construction (this lane passes no run
	// context — nil carrier, never cancels, byte-identical outputs), so the
	// field is always nil here; if a future lane ever surfaces it, the small
	// typed record takes generic detail rendering — no bulk lane, no dup
	// channel, no priority override; hash re-pinned after review.
	// RANKDIS-EXT A1 (§29.104.16.1 M20, 2026-07-16) schema review (R2' 第 7
	// 处): WindowSweepHotspot.Rank's json tag renamed rank→density_rank
	// (word-face scope split — bare `rank` stays exclusive to the root-cause
	// board; field set/type unchanged). The Result hash sees only the
	// WindowSweep pointer's type name, so a NESTED tag rename is invisible
	// to this tripwire by construction (WAKE-CENSUS-D 2A precedent) —
	// comment-entry adjudication, hash unchanged (零重钉). Key-first
	// adjudication: pure wire word, no lane/priority/dup semantics change;
	// tracediag renders sweep hotspots through its own text face, which
	// already wears the scoped `- hotspot` prefix.
	reflect.TypeOf(tracequery.Result{}): "d4c8a7348fdc30e3ea34bdeea35a2359d88230342c09d5357e5c02654981abe8",
	// 修复轮二 件A (2026-07-13) schema review: WindowStats gained the
	// per-lane cap-overflow disclosure quartet
	// (DStateTopOverflowGroups/-Ms, IOWaitTopOverflowGroups/-Ms — scalar
	// disclosure lane beside the capped top lists; the family seats already
	// carry the full census account). Key-first adjudication: plain scalar
	// fields, no skipped fields, no priority override.
	// 件1 census 根修 (修复轮, 2026-07-13) schema review: WindowStats gained
	// BlockedReasonCensus (bounded per-pid per-caller 符号×count×Σms rows,
	// full-accumulator sourced) + BlockedReasonCensusOverflow (pid-cap
	// disclosure scalar). Key-first adjudication: small typed detail rows
	// with explicit overflow scalars — no bulk lane, no dup channel, no
	// priority override; hash re-pinned after review.
	// SA-F2 (DISPATCH-IND 批4, 2026-07-14) schema review (R2' 第 7 处):
	// WindowStats gained VsyncGeneratorCensus (window_population caliber —
	// per-generator event/wakeup counts + authoritative period-print parse;
	// population-wide, no pid predicate). Key-first adjudication: small
	// typed detail rows, no Caveats/Compactions channel to dedupe, no bulk
	// lane, no priority override — generic detail rendering, blocked_reason
	// census 同构; hash re-pinned after review.
	// RSPA (§29.61.10, 2026-07-14) schema review (R2' 第 7 处): WindowStats
	// gained RunnableTopOverflowGroups/-Ms — the runnable lane's cap-overflow
	// disclosure pair (件A 帽基当全量 fourth-instance mirror of the D/IO
	// quartet above). Key-first adjudication: plain scalar disclosure fields,
	// no skipped fields, no priority override; hash re-pinned after review.
	// RANKDIS-EXT A1 (§29.104.16, 2026-07-16) schema review (R2' 第 7 处):
	// StateDrilldownStep.Rank's json tag renamed rank→drill_rank (word-face
	// scope split, witness cust_span_vs_prio.txt — the drilldown ordinal
	// grep-collided with root-cause board seats; field set/type unchanged).
	// The WindowStats hash sees only the StateDrilldownPlan slice's type
	// name, so a NESTED tag rename is invisible to this tripwire by
	// construction (WAKE-CENSUS-D 2A precedent) — comment-entry
	// adjudication, hash unchanged (零重钉). Key-first adjudication: pure
	// wire word, no lane/priority/dup semantics change.
	reflect.TypeOf(tracequery.WindowStats{}):                "2b8831a2d60a240cd93fee91d1b2b61acce31ce63550a9c15c9af267ae080e66",
	reflect.TypeOf(tracequery.TimelineResult{}):             "ec28f82b56a2e1b64cdfde5e0b6a4769886b32df15dc7a99250ec0da16dacc3a",
	reflect.TypeOf(tracequery.TraceCounterQualitySummary{}): "e3bead6ff4a3c2e7f9d24487c5905f3594b219505afc106d95af9cfd9c552c2d",
	// PERF raw quality disclosure: ParserCaveats is rendered once in the
	// bounded key-first line (count + top witness) and skipped by detail.
	// PERF-AGGREGATE-EVENT-UNIT-CHECKED-ADD (2026-07-18) schema review:
	// WeightStatus discloses whether value counts share an exact cohort
	// denominator or are sample-count-only after mixed/overflow withdrawal.
	// It is a scalar key-first quality field; no bulk/duplication lane changes.
	reflect.TypeOf(tracequery.PerfQualitySummary{}):    "fdb13dffd367d3977d395372c51005ddb537109a7a264fb11bc9d27bde1c50b9",
	reflect.TypeOf(tracequery.StorageLatencySummary{}): "0dd6c71d18f36308bc3771f2dd87270d3c02a194f0b3051ceaffc36a961a7559",
	reflect.TypeOf(tracequery.InterruptActivity{}):     "697433793ee39e4a426d249ed9b1559ea6a11d1ca76a569bb30fe9159f45617f",
	reflect.TypeOf(tracequery.WorkqueueActivity{}):     "ed0cdfade0931978ac0def62cbd7c55d226ec943a4e33a43154e3d09a6e3bb70",
	reflect.TypeOf(tracequery.DMAFenceActivity{}):      "c1094517e8c9f158eee1c47dceb51d7a20d6f686a4c07a839d5854e165ed1c1e",
	// XLANE-3 件1 (§29.104.2 定谳③, 2026-07-16) schema review (R2' 第 7 处):
	// RootCauseRankResult gained BoardParamsFingerprint (string, the rank
	// BOARD identity triple's params half — 8-hex sha256 over the normalized
	// closed rank-knob set MaxDepth/MaxBranches/MinDurationMs/Limit, minted
	// once at the rank build entry; window and target are the triple's other
	// two components and travel on their existing fields). Key-first
	// adjudication: a result-level scalar identity disclosure (same lane as
	// Target/Window — generic detail rendering); no bulk lane, no dup
	// channel, no skipped fields, no priority override; hash re-pinned after
	// review.
	//
	// SPANVIS-1 (2026-07-19) schema review: RootCauseRankResult gained
	// BusinessSpanMentions (*BusinessSpanMentionResult — the pure-advisory
	// business-lens mention face: ≤3 on-chain (thread, verbatim span name)
	// families over the FULL span inventory with count/Σ/max-single/line
	// envelope/closed-set basis, plus the honest omitted-family counter).
	// Key-first adjudication: an advisory result-level disclosure list (no
	// seat/ordinal/gate consumer — generic detail rendering); no bulk lane,
	// no dup channel, no skipped fields, no priority override; hash
	// re-pinned after review.
	// PARTSPLIT-1 (§29.150④, 2026-07-19) schema review (R2' 第 7 处):
	// RootCauseRankResult gained GatedCompositeEdgeShareDisclosures
	// ([]GatedCompositeEdgeShareDisclosure — the R4-mirror refusal NON-SEAT
	// disclosure side channel: per refused gated composite seat the X/Y
	// bisection measures + runnable account (X+Y==account µs identity) +
	// boundary/via + seat-published honesty bit + line span; harvested from
	// pool ∪ published so a cap-dead refused seat still discloses). Key-first
	// adjudication: an advisory result-level disclosure list, SPANVIS
	// BusinessSpanMentions 同构 (no seat/ordinal/gate consumer — generic
	// detail rendering); no bulk lane, no dup channel, no skipped fields, no
	// priority override; hash re-pinned after review.
	// RULER2-1 (§29.150②, 2026-07-19) schema review (R2' 第 7 处):
	// RootCauseRankResult gained SelfRunnableTwoRuler
	// (*SelfRunnableTwoRulerAccounting — the target's own runnable seats
	// split across the two closed rulers: per-ruler (rank,eff) seat lists +
	// same-ruler subtotals (µs identity by construction) + the lead seat's
	// line span; deliberately NO cross-ruler total field — M3 禁混尺).
	// Key-first adjudication: an at-most-one advisory result-level record,
	// GatedCompositeEdgeShareDisclosures 同构 (display wording input only —
	// no seat/ordinal/gate consumer; generic detail rendering of the small
	// typed record); no bulk lane, no dup channel, no skipped fields, no
	// priority override; hash re-pinned after review.
	// SELFRUN-DISC (§29.192① (b), 2026-07-21) schema review (R2' 第 7 处):
	// RootCauseRankResult gained SelfRunningFoldUnmeasured
	// (*SelfRunningFoldUnmeasuredDisclosure — the self supply-fold 「量不了」
	// absence disclosure: the target ran in-window while the fold basis was
	// ENTIRELY unknown (KnownMs==0 ∧ UnknownMs>0, running==unknown by the
	// fold identity) + line span; minted only on the zero-seat path, 有席不发
	// / 真满频不发 by construction). Key-first adjudication: an at-most-one
	// advisory result-level record, SelfRunnableTwoRuler 同构 (display
	// wording input only — no seat/ordinal/gate consumer; generic detail
	// rendering of the small typed record: the collection lane honestly
	// shows the absence account); no bulk lane, no dup channel, no skipped
	// fields, no priority override; hash re-pinned after review.
	reflect.TypeOf(tracequery.RootCauseRankResult{}): "099bec8a3df2329b48b8fb46699792a93e878082c43039814f409009b11f66bc",
	// DSTATE-REFINE arm a (CAL-1 件③, 2026-07-12) schema review:
	// RootCauseRankItem gained DStateAllNonIOProven (bool, refined-D proof)
	// + BlockedReasonCaller (string, unanimous 等待对象 symbol). Key-first
	// adjudication: both are per-row wording inputs (scalar disclosure lane,
	// same as DominantState); no skipped fields.
	// CR-3 件②/件③ (2026-07-12) schema review: RootCauseRankItem gained
	// BlockedReasonWindowCount/-Caller (int/string, the unconsumed
	// sched_blocked_reason residual disclosure pair) + ProcessComm (string,
	// the owning-process comm beside Thread.TGID). Key-first adjudication:
	// all three are per-row identity/wording inputs (scalar disclosure
	// lane, same as BlockedReasonCaller); no skipped fields.
	// §29.50.5 证明分区 (v5 P1 批 件②, 2026-07-13) schema review:
	// RootCauseRankItem gained DStateCauseUnprovenRemainder (bool, the
	// honest-remainder D/IO seat marker beside sibling cause seats).
	// Key-first adjudication: per-row wording input (scalar disclosure
	// lane, same as DStateAllNonIOProven); no skipped fields.
	// SELF-SEM (§29.61.1, RANK-U Stage 1, 2026-07-13) schema review:
	// RootCauseRankItem gained OnChainBasis (string, closed set
	// {""|self_deterministic_span} — the typed on-chain proof basis beside
	// ChainRelevance). Key-first adjudication: per-row identity/wording input
	// (scalar disclosure lane, same as ChainRelevance); no skipped fields;
	// hash re-pinned after review.
	// SELF-ALL (§29.61.2, 2026-07-13) schema review (R2' 第 7 处): the
	// OnChainBasis closed set gained "self_wall_clock_interval" (the target's
	// own wall-clock seat basis; causality closed set gained
	// "self_wall_clock" alongside). VALUE-set growth on an existing pinned
	// field — no struct/field change, hash unchanged by construction; the
	// CriticalBlockingCandidate / IOBurstEpisodeSummary witness-feeder structs
	// gained their own OnChainBasis mirror (same scalar disclosure lane;
	// neither type is hash-pinned here — key-first renders fields
	// reflectively, no skipped-field table entry required).
	// RSPA (§29.61.10a/b/c, 2026-07-14) schema review (R2' 第 7 处):
	// RootCauseRankItem gained the re-anchoring bipartition trio
	// ChainAnchoredMs/ChainAnchorFullMs (float64, the 全窗=锚定+余段 same-
	// source split both halves carry) + ChainAnchorRemainderSeat (bool, the
	// ◇ remainder half marker) and ResourceCompletionClosure (bool, the
	// M-IO per-IO completion-closure credential). Key-first adjudication:
	// all four are per-row identity/wording disclosure inputs (scalar
	// disclosure lane, same as DStateCauseUnprovenRemainder); no skipped
	// fields; hash re-pinned after review.
	// RNB-1 (§29.88 R2/R4 user rulings, 2026-07-14) schema review (R2' 第 7
	// 处): RootCauseRankItem gained the case-A' ownership-divergence trio
	// ChainAnchorOwnershipDivergent (bool, the double-account relation
	// marker on a migrated ◇ remainder seat whose chain seat does not
	// provably hold the anchored account) + ChainAnchorChainLaneMs /
	// ChainAnchorCensusMs (float64, the two Σs of the typed double-account
	// disclosure — armed-tick face, one replay pins each diverging gate) and
	// ChainCredentialLaneDemoted (bool, the R4 whole-seat ◇ lane demotion
	// with values untouched: affinity satellite / inversion-retyped seat).
	// Key-first adjudication: all four are per-row identity/wording
	// disclosure inputs (scalar disclosure lane, same as
	// ChainAnchorRemainderSeat); no skipped fields; hash re-pinned after
	// review. The CriticalBlockingCandidate mirror gained
	// ChainCredentialLaneDemoted the same way (not hash-pinned here —
	// key-first renders fields reflectively).
	// XLANE-1 件1 (§29.104.1/§29.104.2, 2026-07-15) schema review (R2' 第 7
	// 处): RootCauseRankItem gained ChainAnchorRepresentedByChainSeat (bool,
	// the fully-anchored runnable-family satellite whole-seat ◇ demotion
	// whose anchored share is already represented by a physically
	// intersecting same-pid chain-lane runnable seat — values untouched;
	// honest word face forks from the R4 无链上凭证 form: this seat HAS
	// credential). Key-first adjudication: per-row identity/wording
	// disclosure input (scalar disclosure lane, same as
	// ChainCredentialLaneDemoted); no skipped fields; hash re-pinned after
	// review. No CriticalBlockingCandidate mirror — the marker mints only on
	// rank satellite rows.
	// XERR1-FIX 件1/件3 (§29.104.3/.4, 2026-07-15) schema review (R2' 第 7
	// 处): CriticalBlockingCandidate gained the payload-less blocking_span
	// value-convergence carriage — BlockingValueBasis (string, closed set
	// {wait_segments|span_envelope}), WaitSegmentMs/WaitSleepMs/WaitDStateMs/
	// WaitIOWaitMs (float64, the converged Σ and its decomposition),
	// SpanEnvelopeMs (float64, the preserved pre-convergence envelope) and
	// the budget-sanity trio WaitBudgetExceeded (bool) +
	// WaitBudgetNonRunningMs/WaitBudgetRunningMs (float64). Key-first
	// adjudication: all per-row identity/wording disclosure inputs (scalar
	// disclosure lane, same as ChainCredentialLaneDemoted); no skipped
	// fields; CriticalBlockingCandidate is not hash-pinned here (key-first
	// renders fields reflectively) and RootCauseRankItem is untouched (the
	// payload-less lane mints no rank seat; payload-typed rank rows carry the
	// budget trio via the twin-port NOTES, not struct fields) — hash
	// unchanged by construction. XCPU rider (§29.104.5): the runnable
	// segment cpu_continuity value set gained sched_in_migrated /
	// sched_in_stamped (VALUE-set growth on an existing field — the
	// SELF-ALL OnChainBasis precedent; no struct change).
	// XERR1-EXT 裁定⑤ (§29.104.17, 2026-07-16) schema review (R2' 第 7 处):
	// no struct change — the existing CriticalBlockingCandidate value-
	// convergence carriage (BlockingValueBasis / Wait*Ms / SpanEnvelopeMs /
	// coverage pair) now ALSO mints on payload-TYPED contention rows (their
	// DurationMs converges to the waiter's Σ(sleep+D+io) over the fold
	// value-winner interval; the whole-wait envelope moves to SpanEnvelopeMs).
	// Scope growth on existing fields only (the SELF-ALL VALUE-set-growth
	// precedent); RootCauseRankItem untouched (the rank face consumes the
	// basis lane via the twin-port NOTES, extended in the same batch) — hash
	// unchanged by construction; key-first renders the fields reflectively.
	// XERR1-FIX 修补 件F (冷读 P3-3, 2026-07-16) schema review (R2' 第 7 处):
	// CriticalBlockingCandidate gained the partial-coverage lower-bound
	// disclosure pair WaitCoveragePartial (bool) + WaitAccountCoveredMs
	// (float64) — the waiter's account did not tile span∩window, so the
	// converged wait_segments value is a proven lower bound. Key-first
	// adjudication: per-row wording disclosure inputs (scalar disclosure
	// lane, same as the 件3 budget trio); no skipped fields; not hash-pinned
	// here (key-first renders CriticalBlockingCandidate fields reflectively);
	// RootCauseRankItem untouched — hash unchanged by construction.
	// HULL-CRED (§29.104 终判③, 2026-07-17) schema review (R2' 第 7 处):
	// CriticalBlockingCandidate gained the keep-⛓ per-segment credential
	// trio — ChainCredentialSegments ([]string, the "start..end" evidence
	// segment inventory published on the two segment-adjudicated verdicts),
	// ChainCredentialSegmentDisjoint (bool, the all-segments-outside-anchor-
	// windows demote form beside ChainCredentialLaneDemoted) and
	// ChainCredentialEnvelopeLevel (bool, the conservative-keep honest-word
	// tier). Key-first adjudication: the two bools are per-row wording
	// disclosure inputs (scalar disclosure lane, same as
	// ChainCredentialLaneDemoted); the segment list is a small bounded
	// (cap 32) proof-carriage slice — generic bulk ordering applies, no dup
	// channel, no priority override; not hash-pinned here (key-first renders
	// CriticalBlockingCandidate fields reflectively); RootCauseRankItem
	// untouched — hash unchanged by construction.
	// G10-EN 根修 (QH2-A, 2026-07-14) schema review (R2' 第 7 处):
	// RootCauseRankItem gained HolderSelfContradictionParts
	// (*types.TraceHolderSelfContradictionWitness — the typed components of
	// the same-lock self-contradiction withdrawal witness, riding beside the
	// byte-frozen zh string so the zh/EN report lanes each word their own
	// sentence). Key-first adjudication: a small typed disclosure record
	// behind a pointer (nil when the guard never fired) — generic detail
	// rendering, PeerChain 同构; no bulk lane, no dup channel, no skipped
	// fields, no priority override; hash re-pinned after review. The
	// CriticalBlockingCandidate mirror gained the same field (not hash-pinned
	// here — key-first renders fields reflectively).
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15) schema review (R2' 第 7 处):
	// RootCauseRankItem gained the affinity/cpuset judgment-payload quintet
	// CPUConstraintKind/CPUConstraintCPUSet/CPUConstraintPolicy (string,
	// judgment-basis kind / group name / verbatim policy) +
	// CPUConstraintAllowedCPUs/CPUConstraintExcludedCPUs ([]int, allowed
	// union vs the in-window observed CPUs absent from it — the restriction
	// gate's own comparison; §29.88.4 R5a comparison-input reserve).
	// Key-first adjudication: per-row description/wording disclosure inputs
	// (scalar disclosure lane, same as DominantState; the two small int
	// slices render reflectively — no bulk lane, no dup channel, no skipped
	// fields, no priority override); hash re-pinned after review.
	// RNB-4 R5a (§29.88.4 场景② 按核档, 2026-07-15) schema review (R2' 第 7
	// 处): RootCauseRankItem gained the tier-exclusion proof pair
	// CPUConstraintAllowedMaxTierKHz/CPUConstraintGlobalMaxTierKHz (int kHz,
	// minted together only on proof — the obligatory 「绑核排除更大核档」
	// mention's inputs). Key-first adjudication: two scalar disclosure ints
	// (same lane as BlockedReasonWindowCount) — no bulk lane, no dup channel,
	// no skipped fields, no priority override; hash re-pinned after review.
	// R3-IMPL (§29.88.1 user ruling, 2026-07-15) schema review (R2' 第 7 处):
	// RootCauseRankItem gained the host-edge-anchoring disclosure pair
	// HostWakeupEdgeAnchorTs (float64, the latest in-window credential edge
	// timestamp — the bisection boundary of a host-edge-anchored semantic
	// seat, µs-verifiable against the raw wakeup line) +
	// HostWakeupEdgeAnchorVia (string, closed set direct / chain_hop /
	// direct+chain_hop — the typed edge inventory word). The OnChainBasis
	// closed set gained "host_wakeup_edge_pre_span" alongside (VALUE-set
	// growth on an existing pinned field, no struct change by itself).
	// Key-first adjudication: per-row identity/wording disclosure inputs
	// (scalar disclosure lane, same as ChainAnchorRemainderSeat); no bulk
	// lane, no dup channel, no skipped fields, no priority override; hash
	// re-pinned after review.
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15) schema review (R2' 第 7 处):
	// the ChainRelevance VALUE set gained "self_caliber_side" (the analysis
	// target's own count-equivalent rows — a non-channel ⌗ side-rail token
	// replacing their former "adjacent" proximity verdict). VALUE-set growth
	// on an existing pinned field, no struct change by itself (the R3-IMPL
	// OnChainBasis precedent); hash unchanged.
	// ONCHAIN-3c (2026-07-19) schema review (R2' 第 7 处): the OnChainBasis
	// VALUE set gained "host_wakeup_edge_pre_state" (bare-census-edge hosts'
	// runnable / D-IO state seats anchored by the R3 host-edge credential;
	// the HostWakeupEdgeAnchorTs/-Via pair now also rides those seats and
	// their ◇ remainder clones). VALUE-set growth on an existing pinned
	// field, no struct change; hash unchanged.
	// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16) schema review (R2' 第 7 处):
	// CriticalBlockingCandidate gained OwnerKeyUnregistered (bool — the span
	// speaks lock-owner vocabulary but matched no registered contention
	// morphology; fail-open marker driving the 「owner 未解析(形态未注册)」
	// disclosure only, never a gate). Key-first adjudication: per-row wording
	// disclosure input (scalar disclosure lane, same as WaitCoveragePartial);
	// no skipped fields; not hash-pinned here (key-first renders
	// CriticalBlockingCandidate fields reflectively); RootCauseRankItem
	// untouched (payload-less rows mint no lock rank seat) — hash unchanged
	// by construction.
	// LOCKNS-FIX 修补 件A (冷读 P2-F1+P3-F7, 2026-07-16) schema review (R2'
	// 第 7 处): RootCauseRankItem gained OwnerTidPresence (string, closed set
	// {absent|present_collision|present_comm_mismatch} — the typed presence
	// verdict of the payload owner tid on a rung-①-diverged row, minted from
	// the engine's existing determination bits; drives the detail 持有者来历
	// presence-clause fork only, absence fail-opens to the legacy sentence).
	// Key-first adjudication: per-row wording disclosure input (scalar
	// disclosure lane, same as HolderSource); no skipped fields; hash
	// re-pinned after review. The CriticalBlockingCandidate mirror gained the
	// same field (not hash-pinned here — key-first renders fields
	// reflectively).
	// TQ-PRIORITY-POINT-AUTHORITY (2026-07-17) schema review: the rank item
	// gained the closed relation caliber, proven-lower/unknown coverage
	// partition and sorted physical artifact-source roster. They are
	// load-bearing audit provenance and must remain visible through generic
	// detail rendering; the source slice keeps the structural bulk-last lane.
	// No duplicate key-first renderer exists, so no skipped fields are added.
	// XLANE-2 件1/件2 (§29.104.1/.2 定谳④ + 裁定④, 2026-07-17) schema review
	// (R2' 第 7 处): the rank item gained MemberLineRanges ([]string — the
	// semantic family seat's COMPLETE per-member trace line ranges, minted
	// all-or-nothing; the display 成员子集 subset-judgment input) and
	// SelfGapSemanticOverlaps ([]RootCauseSelfGapSemanticOverlap — the
	// self-gap seat's per-partner typed interval-intersection disclosure,
	// 其中X与语义席[E#]重叠 clause input). Key-first adjudication: bounded
	// typed disclosure rosters (audit-visible through generic detail
	// rendering, same lane as MemberRoster); no bulk lane, no dup channel,
	// no skipped fields, no priority override; hash re-pinned after review.
	// CPU-SCALAR-A (§29.104.23, 2026-07-18) schema review: the rank item
	// gained CPUConstraint* disclosure scalars/rosters describing the effective
	// cpuset/policy/allowed-vs-global max tier used by strict frequency/CAP
	// derivation. Key-first adjudication: exact provenance/decision inputs;
	// rosters remain visible through generic detail rendering, no duplicate
	// key-first lane, no skipped fields, hash re-pinned after review.
	// LEVELMERGE-1 件2 (方案 P 区间分账, user ruling 2026-07-18) schema
	// review (R2' 第 7 处): the rank item gained the gated-share split family
	// GatedShareClaimedMs/GatedShareFullMs (float64 — the A share and the
	// pre-split aggregate account; identity claimed + residual == full),
	// GatedShareConstituentSeat (bool — the demoted A constituent row on the
	// adjacent lane), GatedShareClaimSeats ([]string — the claiming inversion
	// seats' own line intervals, the [E#] cross-reference pointer input) and
	// GatedShareOverlapDisclosureMs (float64 — the fail-open 裁定④ disclosure
	// overlap with the published value untouched). Key-first adjudication:
	// per-row identity/wording disclosure inputs (scalar disclosure lane,
	// same as ChainAnchorRemainderSeat; the pointer roster is bounded and
	// audit-visible through generic detail rendering, same lane as
	// MemberLineRanges); no bulk lane, no dup channel, no skipped fields,
	// hash re-pinned after review.
	// SPANTOP-1 件1 (§29.131, 2026-07-18) schema review (R2' 第 7 处): the
	// rank item gained MemberWallMs ([]string — the semantic family seat's
	// COMPLETE per-member in-window wall-clock durations, "%.3f" member
	// order, minted all-or-nothing beside MemberLineRanges; the display
	// constituent top-3 sub-row input under its µs identity gate). Key-first
	// adjudication: bounded typed disclosure roster (audit-visible through
	// generic detail rendering, same lane as MemberLineRanges); no bulk
	// lane, no dup channel, no skipped fields, no priority override; hash
	// re-pinned after review.
	// AXIOM-V2 (user rulings 2026-07-18) schema review (R2' 第 7 处): the
	// rank item gained FixDirection (string — the registry repair-direction
	// attribute, verbatim from causalTokenFixDirections; attribute axis,
	// 序数芯片本体零动), CrossDirectionOverlaps
	// ([]RootCauseCrossDirectionOverlap — the 件2 symmetric cross-direction
	// overlap pair roster: exact interval-intersection wall clock + partner
	// line envelope/direction/basis, the display 互指句 input),
	// CrossDirectionOverlapUndisclosed ([]string — un-pointable pair partner
	// TYPE tokens, 宁漏勿假指 audit disclosure) and
	// DirectionConservationExcess (*RootCauseDirectionConservation — the 件3
	// per-(thread,direction) Σ>window violation finding; pure disclosure /
	// 立案素材, nil on every clean seat). Key-first adjudication: bounded
	// typed disclosure rosters + a small typed finding record behind a
	// pointer (SelfGapSemanticOverlaps / HolderSelfContradictionParts 同构 —
	// audit-visible through generic detail rendering); no bulk lane, no dup
	// channel, no skipped fields, no priority override; hash re-pinned after
	// review.
	// ONCHAIN-FIX-1 件1 (mint audit 命题2 不一致①, 2026-07-18) schema review
	// (R2' 第 7 处): the rank item gained ChainIdentityInheritance (bool — the
	// interval-less same-pid fail-open admission record: the row inherited
	// the on-chain lane from bare thread identity with no typed interval; the
	// fabricated whole-node-window overlap it replaces is retired, OverlapMs
	// stays honest zero and the 「成员继承(链窗级,无区间凭证)」 disclosure
	// word rides this bit). Key-first adjudication: a per-row wording/channel
	// disclosure input (scalar disclosure lane, same as
	// ChainCredentialLaneDemoted); no bulk lane, no dup channel, no skipped
	// fields, no priority override; hash re-pinned after review. The
	// CriticalBlockingCandidate mirror gained the same field (not hash-pinned
	// here — key-first renders fields reflectively).
	// ONCHAIN-FIX-2 (2026-07-18) schema review (R2' 第 7 处): the rank item
	// gained ChainCredentialEnvelopeLevel (bool — the rank-lane mirror of the
	// critical-side envelope-tier honest word: a hull-only keep-⛓ legacy-basis
	// row wears 「交集证明(包络级)」; 件1 包络泛化) plus the unexported
	// dioSegmentIntervals carrier (件4 — hash-invisible by construction).
	// The CriticalBlockingCandidate mirror gained
	// ChainCredentialSegmentsTruncated (bool — 件3 proven-lower-bound prefix
	// marker beside the published inventory; not hash-pinned here, key-first
	// renders CriticalBlockingCandidate fields reflectively). Key-first
	// adjudication: per-row wording disclosure inputs (scalar disclosure
	// lane, same as ChainCredentialLaneDemoted); no bulk lane, no dup
	// channel, no skipped fields, no priority override; hash re-pinned after
	// review.
	// PARTSPLIT-1 (§29.150④, 2026-07-19) schema review (R2' 第 7 处): the
	// rank item gained the GatedCompositeEdge* quartet
	// (GatedCompositeEdgePreShareMs/PostShareMs — float64 X/Y bisection
	// measures of the refused gated composite seat's runnable census account,
	// X+Y==RunnableMs to the µs; GatedCompositeEdgeAnchorTs/Via — the
	// credential boundary pair, dedicated so the R3 keep arms never read a
	// refused seat). Stamped atomically at the single R4-mirror refusal
	// site; disclosure/wording inputs only, every published value channel
	// untouched. Key-first adjudication: per-row wording disclosure inputs
	// (scalar disclosure lane, same as ChainAnchorRemainderSeat); no bulk
	// lane, no dup channel, no skipped fields, no priority override; hash
	// re-pinned after review.
	// DISPHYG-3 件7 (2026-07-20): +GatedCapabilityFreqOnlyReason — a scalar
	// wording-disclosure token (the gated freq_only reason twin), same lane
	// as GatedClusterTopology; hash re-pinned after review.
	// P3MEASURE-1 (§29.169, 2026-07-20) schema review (R2' 第 7 处): the rank
	// item gained the four P3M* silent-measurement fields
	// (P3MCounterfactualValidMs/InvalidMs — the counterfactual anchor-time
	// split, µs identity valid+invalid==anchor time; P3MEdgeWitnessedMs —
	// the structural edge-witness share, ≤ 席值; P3MDisposition — the closed
	// measurement-form token). Key-first adjudication: scalar AUDIT-ONLY
	// disclosure lane (display_only wire keys, advisory-only red line —
	// never a gate/ordinal/value input); audit-visible through this
	// deterministic generic rendering BY DESIGN (the SelfGapSemanticOverlaps
	// "audit-visible through generic detail rendering" precedent — the
	// zero-LLM diag dump IS the silent wire's audit home), while every
	// answer-pipeline user/model face is pinned byte-identical without them
	// (four-flagship A/B). No bulk lane, no dup channel, no skipped fields,
	// no priority override; hash re-pinned after review.
	// CHAINGUARD-1 件1 (§29.204.1, 2026-07-22) schema review (R2' 第 7 处):
	// the rank item gained ChainCredentialCensus — the closed chain-credential
	// census verdict (wakeup_anchored/target_self/interval_proven/
	// member_inherited/none; single engine mint point at the ordinal
	// publication tail, single tool emission helper). Key-first adjudication:
	// scalar wording/channel-disclosure token (same lane as OnChainBasis /
	// ChainIdentityInheritance — never a value/score/sort input; the display
	// chip maps it and the board second gate reads it); no bulk lane, no dup
	// channel, no skipped fields, no priority override; hash re-pinned after
	// review.
	reflect.TypeOf(tracequery.RootCauseRankItem{}): "748e960e0b3f1e5500215643d7197f3cfda63704d09163725c2bcbc83ff3ad5d",
	// CR-1 P9 (§29.42 案1, 2026-07-12) schema review: ChainResult gained
	// PacingIdles ([]PacingIdleSummary, arm-c frame-pacing idle segments).
	// Key-first adjudication: a slice → structural bulk lane (same as
	// BinderWaits); no skipped fields (PacingIdleSummary carries no
	// Caveats/Compactions to dedupe — the write-off disclosure rides
	// ChainResult.Caveats, which collectNonEventEngineDiagnostics already
	// reads); no priority override needed.
	// WAKE-CENSUS (§29.58, 修复轮 件1 2026-07-13) schema review: ChainResult
	// gained WakeupEdgeCensus (bounded per-(waker→wakee) count+first/last-ts
	// rows, FULL pre-cap edge-set sourced) + WakeupEdgeCensusOverflowPairs/
	// -Edges (pair-cap disclosure scalars). Key-first adjudication — the
	// WindowStats blocked_reason census precedent verbatim: small typed
	// detail rows with explicit overflow scalars — no bulk lane, no dup
	// channel, no priority override; hash re-pinned after review. 教训入注:
	// this pin is the R2' 第 7 处 for census-shaped engine fields — the
	// blocked_reason census batch walked it, the WAKE-CENSUS main batch
	// missed it until this tripwire fired (工作清单缺项, not a schema doubt).
	reflect.TypeOf(tracequery.ChainResult{}): "7acd830b8504baae094c3d2d8f12f7151abf47339dd68ec0bb214b1c316c5f40",
	// CHAIN-BUDGET (user ruling 2026-07-18) schema review (R2' 第 7 处):
	// ChainNode and WakeupEdge now pinned in their own right (the ChainResult
	// hash sees only the slices' type names — the WakeupEdgeCensusPair
	// precedent) and each gained SegmentOrdinal (int, json segment_ordinal
	// omitempty): the expansion-lane identity — 0 = guaranteed top-1 lane /
	// legacy (wire-stable absence), >= 2 = the value-order position of the
	// parent-node sleep segment a budget-gated extra expansion came from.
	// Key-first adjudication: a per-row scalar identity input (same lane as
	// Branch/Depth — generic detail rendering); no bulk lane, no dup channel,
	// no skipped fields, no priority override.
	reflect.TypeOf(tracequery.ChainNode{}):  "4860488d816c9a4ce6a24e7f2d9b3c2bdf998314dfdf80087ced1835673b0ede",
	reflect.TypeOf(tracequery.WakeupEdge{}): "84855212bf935a6dcc1920a5a9be54b5665e04f90404f301008283d147adc1a6",
	// WAKE-CENSUS-D 2A (§29.58.4, RANK-U Stage 1 commit B, 2026-07-13) schema
	// review: WakeupEdgeCensusPair now pinned in its own right (the ChainResult
	// hash sees only the slice's type name, so pair-level field growth was
	// invisible to the R2' 第 7 处 tripwire) and gained the typed exit-split
	// trio (SleepExitCount/DExitCount/OtherExitCount — the three columns
	// partition Count exactly; measurement-face counts, the D causal lane
	// stays with blocked_reason). Key-first adjudication: plain scalar
	// disclosure columns beside Count — no bulk lane, no dup channel, no
	// priority override.
	reflect.TypeOf(tracequery.WakeupEdgeCensusPair{}): "6cc9d001bd84c93e8cfee72488390af69b2bcb89e1a52c4df87b95fd81bcb839",
	// ENG-2 追修 + P3-4 (2026-07-12) schema review: PacingIdleSummary gained
	// EvidenceLineStart/End — the segment's causal-impact evidence span the
	// published row aligns to so the display same-fact fold engages by
	// construction; the raw SleepLine/WakeupLine pair stays the audit-honest
	// event locator. Key-first adjudication: two plain int coordinates, no
	// priority/bulk-lane change, no skipped fields.
	reflect.TypeOf(tracequery.PacingIdleSummary{}): "d1cd02ccef0e5974f23ecc1be4a3f0bf72f7c35fc022dbc561479b56e35e8909",
	// TQ-PRIORITY-POINT-AUTHORITY (2026-07-17) schema review: per-impact
	// priority/target source+artifact provenance and both impact/aggregate
	// relation caliber+coverage+artifact rosters are deterministic audit
	// fields. Generic detail rendering is their sole tracediag face; artifact
	// rosters remain bulk-last and no duplicate/skipped lane is introduced.
	// DISPHYG-3 件7 (2026-07-20): +GatedCapabilityFreqOnlyReason on both
	// faces (scalar disclosure token beside GatedClusterTopology); hashes
	// re-pinned after review.
	reflect.TypeOf(tracequery.WakeupCausalImpact{}):    "1975cd5e7cabf4a6783f8b3dd4bd8a7e2ba7017325ab36517ec3a903c4e22610",
	reflect.TypeOf(tracequery.WakeupCausalAggregate{}): "8c13f1dd669d8b2063fca83b44296b4196aa8edc622d79d7416a523a71d3fb06",
	// CR-3 件⑥ F-10 (2026-07-12) schema review: SupplyFoldBasis gained
	// ThermalCapWitnessed (bool, the cap's in-window limits/thermal event
	// witness — the 受热限压 vs 运行于(限压原因未见证) wording gate).
	// Key-first adjudication: a wording-input boolean beside its value
	// (same lane as ThermalCapClusterClass); no skipped fields.
	// CLUSTER-FIX-1 (user ruling 2026-07-18) schema review (R2' 第 7 处):
	// SupplyFoldBasis gained ClusterSampleBasis (string, closed set
	// {""|side_scan|window_carve} — the cluster-derivation sample-stream
	// basis token; the healthy full_index norm is disclosed by absence, the
	// ClusterTopologySource precedent) + ClusterFreqIntegrityDroppedCPUs
	// ([]int, sorted cpu_frequency lanes the order-integrity audit removed
	// from the derivation basis — the S4 silent cluster-count-understatement
	// side effect made auditable; judgment unchanged). Key-first
	// adjudication: the token is a scalar wording/audit disclosure input
	// (same lane as CapabilitySource but NOT rendered by capabilityAudit —
	// generic detail rendering, no skipped-field entry); the small int
	// roster takes the structural bulk-last lane reflectively — no dup
	// channel, no priority override; hash re-pinned after review. Re-pinned
	// again at the 2026-07-18 confluence merge (this batch × the strict CPU
	// scalar authority batch both evolved SupplyFoldBasis; merged struct
	// carries both sides' fields).
	// CLUSTER-FIX-2 (2026-07-20) schema review (R2' 第 7 处): SupplyFoldBasis
	// gained CapabilityFreqOnlyReason (string, closed CoreCapabilityFreqOnly
	// Reason* set — the typed freq_only cause token riding beside
	// CapabilitySplitAudit, S1) + ClusterLimitsAnchorMismatch ([]int, sorted
	// limits anchors sitting strictly inside a derived cluster — the C2
	// partition-consistency disclosure roster; membership consumption stays a
	// ruling candidate, S9). Key-first adjudication: the token is a scalar
	// wording/audit disclosure input (same lane as CapabilitySource /
	// ClusterSampleBasis — generic detail rendering, no skipped-field entry);
	// the small int roster takes the structural bulk-last lane reflectively
	// like ClusterFreqIntegrityDroppedCPUs — no dup channel, no priority
	// override; hash re-pinned after review.
	// CLUSTERTIE-1 (§29.197①, 2026-07-21): + CapabilityTieBreakAudit (string,
	// omitempty — the judged fmax tie-break chain disclosure
	// "labelLow↔labelHigh fmax=NkHz 破局链=chain(zh:XkHz vs YkHz)"). Key-first
	// adjudication: same adjudication category as CapabilitySplitAudit
	// (scalar wording/audit disclosure input, no gate) but a DIFFERENT render
	// lane — SplitAudit is a skipped field lifted into the key_first.capability
	// census row, while TieBreakAudit stays on generic detail rendering (no
	// skipped-field entry, no census column; the engine caveat already lifts
	// it single-point — 冷读 P3-2, census-column adoption deferred until a
	// customer replay shows the caveat face insufficient); hash re-pinned
	// after review.
	reflect.TypeOf(tracequery.SupplyFoldBasis{}): "f7122564054aa8c52a16ae2ab2e0eda4746ebe0e393ae9250f304209532b856a",
}

func detailSchemaFingerprint(typ reflect.Type) (fingerprint, schema string) {
	parts := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" || jsonExcluded(field) {
			continue
		}
		parts = append(parts, field.Name+"|"+field.Type.String()+"|"+field.Tag.Get("json"))
	}
	schema = strings.Join(parts, ";")
	sum := sha256.Sum256([]byte(schema))
	return hex.EncodeToString(sum[:]), schema
}
