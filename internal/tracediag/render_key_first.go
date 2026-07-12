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
		"Caveats": true,
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

func collectNonEventEngineDiagnostics(res *tracequery.Result) []nonEventEngineDiagnostic {
	if res == nil {
		return nil
	}
	var out []nonEventEngineDiagnostic
	seen := map[string]bool{}
	add := func(source string, caveats []string, compactions []tracequery.ViewCompaction) {
		for _, caveat := range caveats {
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
		emit(fmt.Sprintf("- key_first.perf_quality %s: cpu_known=%d cpu_unknown=%d callchain_known=%d callchain_unknown=%d sources=%d symbolization_statuses=%d sample_kinds=%d weight_units=%d clocks=%d clock_confidences=%d callchain_statuses=%d caveats=%d",
			ref.path, quality.CPUKnownCount, quality.CPUUnknownCount,
			quality.CallchainKnownCount, quality.CallchainUnknownCount,
			len(quality.Sources), len(quality.SymbolizationStatuses), len(quality.SampleKinds),
			len(quality.WeightUnits), len(quality.Clocks), len(quality.ClockConfidences),
			len(quality.CallchainStatuses), len(quality.Caveats)))
	}
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
	reflect.TypeOf(tracequery.Result{}):                     "05dd81090fe34e19e78dcf43b1a2e1f109adefc0a0f99d3d5229318cc4fce312",
	reflect.TypeOf(tracequery.WindowStats{}):                "1347e623ef12e6024068e0cb1f97256ed747f364515f3aeaf0f0d35ca68d357e",
	reflect.TypeOf(tracequery.TimelineResult{}):             "ec28f82b56a2e1b64cdfde5e0b6a4769886b32df15dc7a99250ec0da16dacc3a",
	reflect.TypeOf(tracequery.TraceCounterQualitySummary{}): "e3bead6ff4a3c2e7f9d24487c5905f3594b219505afc106d95af9cfd9c552c2d",
	reflect.TypeOf(tracequery.PerfQualitySummary{}):         "72c447267958bb72db82ab1e807135761cbea3caf60bf09f040a8f451476972a",
	reflect.TypeOf(tracequery.StorageLatencySummary{}):      "0dd6c71d18f36308bc3771f2dd87270d3c02a194f0b3051ceaffc36a961a7559",
	reflect.TypeOf(tracequery.InterruptActivity{}):          "697433793ee39e4a426d249ed9b1559ea6a11d1ca76a569bb30fe9159f45617f",
	reflect.TypeOf(tracequery.WorkqueueActivity{}):          "ed0cdfade0931978ac0def62cbd7c55d226ec943a4e33a43154e3d09a6e3bb70",
	reflect.TypeOf(tracequery.DMAFenceActivity{}):           "c1094517e8c9f158eee1c47dceb51d7a20d6f686a4c07a839d5854e165ed1c1e",
	reflect.TypeOf(tracequery.RootCauseRankResult{}):        "148735ab082d8b42df67875bccfaa6043e50d7c0f99fada94957aeecfdb3b703",
	reflect.TypeOf(tracequery.RootCauseRankItem{}):          "3062374b24f82fa46cd5651e33c5a693585626f5b739ab0c3b2beb4ae0322872",
	// CR-1 P9 (§29.42 案1, 2026-07-12) schema review: ChainResult gained
	// PacingIdles ([]PacingIdleSummary, arm-c frame-pacing idle segments).
	// Key-first adjudication: a slice → structural bulk lane (same as
	// BinderWaits); no skipped fields (PacingIdleSummary carries no
	// Caveats/Compactions to dedupe — the write-off disclosure rides
	// ChainResult.Caveats, which collectNonEventEngineDiagnostics already
	// reads); no priority override needed.
	reflect.TypeOf(tracequery.ChainResult{}): "77bc23ed2ece8b6be7031a48a12833911e11f0cfb28b90d171580a6e5bf11244",
	// ENG-2 追修 + P3-4 (2026-07-12) schema review: PacingIdleSummary gained
	// EvidenceLineStart/End — the segment's causal-impact evidence span the
	// published row aligns to so the display same-fact fold engages by
	// construction; the raw SleepLine/WakeupLine pair stays the audit-honest
	// event locator. Key-first adjudication: two plain int coordinates, no
	// priority/bulk-lane change, no skipped fields.
	reflect.TypeOf(tracequery.PacingIdleSummary{}):     "d1cd02ccef0e5974f23ecc1be4a3f0bf72f7c35fc022dbc561479b56e35e8909",
	reflect.TypeOf(tracequery.WakeupCausalImpact{}):    "625697669c2daac29f2efc46a84d3d372c66924f64e98f82f1134f82762846eb",
	reflect.TypeOf(tracequery.WakeupCausalAggregate{}): "e4ed22c7d66ff5724b9e395de5fdb921f3d06fd66e70ff44fba6fcac40a14831",
	reflect.TypeOf(tracequery.SupplyFoldBasis{}):       "2812909ea9da6b3296c70229002e432dd9ef9153f8912fcdd3ba1012b34be9c4",
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
