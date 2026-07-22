package tool

// trace_note_keys_emit_pin_test.go — NKR producer-side pin: a full-coverage
// synthetic tracequery.Result runs through traceQueryTypedObservations (the
// ONE entry point that renders rich notes) and every emitted "key=value" note
// key must be registered in types/trace_note_keys.go. This is the dynamic
// half of the registry contract:
//
//   - emitted ⊆ registry — a typo'd or unregistered NEW producer key fails
//     here instead of riding wire silence into a silently-skipped consumer
//     gate (§7.4 / F-2 failure class);
//   - contract-tier coverage — every registered consumer-parsed key (anchor /
//     hard / soft carriers) must actually be emitted by the fixture, so the
//     fixture cannot rot into checking nothing.
//
// NOTE: key literals in THIS file are deliberate verbatim pins (wire-format
// double-write) — do not replace them with the constants.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// traceNoteKeysEmitFixtureOverflowImpacts pads the causal-impact list past the
// typed family row cap with simple ON-CHAIN rows, so the PTS fold record and
// its folded_* contract keys are exercised (PTV5 #68).
func traceNoteKeysEmitFixtureOverflowImpacts(first tracequery.WakeupCausalImpact) []tracequery.WakeupCausalImpact {
	out := []tracequery.WakeupCausalImpact{first, {
		Thread:           tracequery.ThreadRef{Comm: "plain-sleep", PID: 398},
		Window:           tracequery.TimeWindow{StartTs: 1.0, EndTs: 1.2},
		ChainDepth:       1,
		OnChain:          true,
		DominantState:    string(tracequery.StateSSleep),
		DominantImpactMs: 2,
		TotalMs:          2,
		SleepMs:          2,
		LineStart:        1, LineEnd: 2, Summary: "plain non-periodic sleep hop",
	}, {
		Thread:           tracequery.ThreadRef{Comm: "gated", PID: 399},
		Window:           tracequery.TimeWindow{StartTs: 1.0, EndTs: 1.2},
		ChainDepth:       1,
		OnChain:          true,
		DominantState:    string(tracequery.StateRunnable),
		DominantImpactMs: 2,
		TotalMs:          2,
		RunnableMs:       1,
		RunningMs:        1,
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 2, PriorityRelationArtifactSources: []string{"artifact:0"},
		PriorityInversionCandidate: true, PriorityInversionGatedMs: 2,
		GatedRunnableMs: 1, GatedRunningDeficitMs: 1, GatedCapabilitySource: tracequery.CoreCapabilitySourceFreqOnly,
		GatedClusterTopology: tracequery.CoreCapabilityTopologyComovement,
		// DISPHYG-3 件7: the gated reason twin key stays exercised (a
		// freq_only source is the only shape that carries it — S1 discipline).
		GatedCapabilityFreqOnlyReason: tracequery.CoreCapabilityFreqOnlyReasonSingleCluster,
		LineStart:                     3, LineEnd: 4, Summary: "valid gated runnable/running inversion fixture",
	}}
	for i := 0; i < traceQueryWidthTypedFamilyRowCap()+2; i++ {
		out = append(out, tracequery.WakeupCausalImpact{
			Thread:           tracequery.ThreadRef{Comm: fmt.Sprintf("ovf%d", i), PID: 400 + i},
			Window:           tracequery.TimeWindow{StartTs: 1.0, EndTs: 2.0},
			ChainDepth:       2,
			OnChain:          true,
			DominantState:    string(tracequery.StateSSleep),
			DominantImpactMs: 1.5 + float64(i)*0.01, TotalMs: 2,
			LineStart: 100 + i, LineEnd: 100 + i,
			Summary: "overflow hop",
		})
	}
	// DIAG A1 (§28.11-3(a)): two overflow members tying the fold MAX to the µs
	// exercise the same_value_members contract key (the huadong_79 E23 shape).
	for i, comm := range []string{"tievictim", "tietwin"} {
		out = append(out, tracequery.WakeupCausalImpact{
			Thread:           tracequery.ThreadRef{Comm: comm, PID: 500 + i},
			Window:           tracequery.TimeWindow{StartTs: 1.0, EndTs: 2.0},
			ChainDepth:       2,
			OnChain:          true,
			DominantState:    string(tracequery.StateSSleep),
			DominantImpactMs: 14.272, TotalMs: 15,
			LineStart: 200 + 10*i, LineEnd: 205 + 10*i,
			Summary: "overflow hop (µs tie)",
		})
	}
	return out
}

func traceNoteKeysEmitFixtureResult() tracequery.Result {
	oneway := false
	syncLike := true
	blockingCandidate := true
	window := tracequery.TimeWindow{StartTs: 1.0, EndTs: 2.0}
	basis := &tracequery.SupplyFoldBasis{
		KnownMs: 5, UnknownMs: 1,
		FmaxKHz: 2400000, FmaxSource: tracequery.SupplyFoldFmaxSourceLimit,
		LimitThrottled: true, TraceObservedMaxKHz: 2800000,
		ClusterLaneName: "lane0", ClusterLaneMaxKHz: 999, ClusterLaneDivergent: true,
		// CFR (#75 簇共频): exercises the fold_cluster_freq_reuse emission.
		ClusterFreqReuse: []tracequery.SupplyFoldClusterReuse{{CPU: 3, DonorCPU: 4}},
		// CAP (§26 C3): exercises the fold_capability emission; the demoted
		// reference class (复核 F1) exercises fold_reference_class (the note
		// only emits on a non-big basis).
		CapabilitySource: tracequery.CoreCapabilitySourceDefault,
		ReferenceClass:   "small",
		// CAP-2 (§28.4/§28.5): exercises fold_cluster_topology + the
		// fold_rail_basis audit note; THERM exercises thermal_cap_khz.
		ClusterTopologySource: tracequery.CoreCapabilityTopologyKeyedRail,
		RailFamily:            "m3_c#_freq",
		RailGoverned:          []tracequery.SupplyFoldRailGoverned{{CPU: 12, Rail: "m3_c3_freq"}},
		ThermalCapKHz:         1850000,
		// CR-3 件⑥ F-10 (2026-07-12): exercises the thermal_cap_witnessed
		// contract key.
		ThermalCapWitnessed: true,
	}
	// CLUSTER-FIX-2 件1 (S1): the freq_only twin basis exercises the
	// fold_capability_freq_only_reason contract key (the engine mints the
	// reason iff the caliber is freq_only, so the fixture mirrors that shape).
	freqOnlyBasis := &tracequery.SupplyFoldBasis{
		KnownMs: 5, UnknownMs: 1,
		FmaxKHz: 2189000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
		CapabilitySource:         tracequery.CoreCapabilitySourceFreqOnly,
		CapabilityFreqOnlyReason: tracequery.CoreCapabilityFreqOnlyReasonSingleCluster,
	}
	impact := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Window: window,
		ActualWindow: tracequery.TimeWindow{StartTs: 0.9, EndTs: 2.1},
		ChainDepth:   1, ChainBranch: 1, OnChain: true, DominantState: string(tracequery.StateSSleep),
		DominantImpactMs: 4, ProjectedImpactMs: 4, TotalMs: 5, ProjectedTotalMs: 5,
		ActualImpactMs: 6, ActualTotalMs: 7, TargetBlockedMs: 3,
		FragmentCount: 2, StateSwitches: 3, MaxSegmentMs: 2, P95SegmentMs: 1,
		RunningMs: 1, RunnableMs: 1, SleepMs: 2, DStateMs: 1, IOWaitMs: 1,
		ActualRunningMs: 1, ActualRunnableMs: 1, ActualSleepMs: 2, ActualDStateMs: 1, ActualIOWaitMs: 1,
		Priority: 10, PriorityClass: "cfs", PrioritySource: "closed_range_stable", PriorityArtifactSource: "artifact:1",
		TargetPriority: 20, TargetPriorityClass: "cfs", TargetPrioritySource: "closed_range_stable", TargetPriorityArtifactSource: "artifact:0",
		PriorityRelation: "waker_higher", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 0, PriorityRelationUnknownOrNonLowerMs: 5,
		PriorityRelationArtifactSources: []string{"artifact:0", "artifact:1"},
		NextStep:                        "inspect the waker", NextStepKind: "wakeup_chain",
		PeriodicSource: true, DetectedPeriodMs: 16.6, LatenessMs: 0.5, EffectivePeriodicImpactMs: 0.5,
		SupplyFoldBasis: basis, SupplyFoldDeficitMs: 1, SupplyFoldIdealMs: 4,
		LineStart: 5, LineEnd: 9, Summary: "dep slept before wakeup",
	}
	occurrence := tracequery.WakeupCausalOccurrence{
		Window: tracequery.TimeWindow{StartTs: 1.01, EndTs: 1.02}, DominantState: string(tracequery.StateSSleep),
		DominantImpactMs: 2, TotalMs: 2, LineStart: 6, LineEnd: 7,
	}
	stats := &tracequery.WindowStats{
		Window: window,
		TopRunning: []tracequery.ThreadDuration{{
			Thread: tracequery.ThreadRef{Comm: "worker", PID: 30}, DurationMs: 12,
			CPU: 2, CoreClass: "big", Frequency: 1800000, StartTs: 1.1, EndTs: 1.3,
			Priority: 10, PriorityClass: "cfs", LineStart: 11, LineEnd: 12,
		}},
		RunnableTop: []tracequery.ThreadDuration{
			{Thread: tracequery.ThreadRef{Comm: "starved", PID: 31}, DurationMs: 400, CPU: 1, StartTs: 1.0, EndTs: 1.9, LineStart: 13, LineEnd: 14},
			{Thread: tracequery.ThreadRef{Comm: "starved2", PID: 32}, DurationMs: 200, CPU: 2, StartTs: 1.0, EndTs: 1.8, LineStart: 15, LineEnd: 16},
		},
		SleepTop:  []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{Comm: "sleeper", PID: 33}, DurationMs: 9, LineStart: 17, LineEnd: 18}},
		DStateTop: []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{Comm: "dwait", PID: 34}, DurationMs: 8, LineStart: 19, LineEnd: 20}},
		IOWaitTop: []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{Comm: "iowait", PID: 35}, DurationMs: 7, LineStart: 21, LineEnd: 22}},
		// 件1 census 根修 (2026-07-13): the pid-keyed per-caller census pair
		// (blocked_reason_census + its caller-overflow note).
		BlockedReasonCensus: []tracequery.BlockedReasonPIDCensus{{
			Thread: tracequery.ThreadRef{Comm: "dwait", PID: 34},
			Count:  5,
			Callers: []tracequery.BlockedReasonCensusCaller{
				{Caller: "dma_fence_default_wait", Count: 4, DelayTotalMs: 3.2},
				{Caller: "hmfs_read", Count: 1},
			},
			CallerOverflow: 1,
		}},
		CPUOccupancy: &tracequery.CPUOccupancyStats{
			WindowMs: 1000,
			TopThreads: []tracequery.CPUOccupancyThread{
				{Thread: tracequery.ThreadRef{Comm: "hog1", PID: 41}, RunningMs: 500},
				{Thread: tracequery.ThreadRef{Comm: "hog2", PID: 42}, RunningMs: 300},
				{Thread: tracequery.ThreadRef{Comm: "hog3", PID: 43}, RunningMs: 200},
				{Thread: tracequery.ThreadRef{Comm: "hog4", PID: 44}, RunningMs: 100},
			},
		},
		ThreadCPULoad: []tracequery.ThreadCPULoadSummary{{
			Thread: tracequery.ThreadRef{Comm: "loader", PID: 50}, RunningMs: 5, RunnableWaitMs: 3,
			HighPriorityRunningMs: 2, CPU: 3, CoreClass: "mid", Frequency: 1200000,
			Priority: 15, PriorityClass: "cfs", LineStart: 23, LineEnd: 24,
		}},
		CPUConstraints: []tracequery.CPUConstraintSummary{{
			Thread: tracequery.ThreadRef{Comm: "bound", PID: 51}, Kind: "cpuset",
			Policy: "SCHED_NORMAL", CPUSet: "background", AllowedCPUs: []int{0, 1},
			AllowedCoreClasses: []string{"small"}, ObservedCPU: 1, ObservedCPUKnown: true,
			ObservedCoreClass: "small", MigrationCount: 2, RunnableWaitMs: 4, OtherCPUIdleMs: 2,
			StartTs: 1.0, EndTs: 1.5, LineStart: 25, LineEnd: 26, Summary: "cpuset bound",
		}},
		RunnableContext: []tracequery.RunnableContextSummary{{
			Thread: tracequery.ThreadRef{Comm: "queued", PID: 52}, RunnableWaitMs: 6, CPU: 2,
			CoreClass: "big", Frequency: 2000000, Priority: 12, PriorityClass: "cfs",
			SameCPUBusyMs: 5, SameCPUIdleMs: 1, OtherCPUIdleMs: 3,
			HighPriorityRunningMs: 2, HighPriorityRunningOverlapMs: 1,
			TopBackgroundThreads: []tracequery.ThreadCPULoadSummary{{Thread: tracequery.ThreadRef{Comm: "bg", PID: 53}, RunningMs: 2}},
			TopBackgroundProcess: &tracequery.ProcessCPULoadSummary{Process: tracequery.ThreadRef{Comm: "bgproc", PID: 54}, RunningMs: 2},
			CPUConstraint:        &tracequery.CPUConstraintSummary{AllowedCPUs: []int{0, 1}, CPUSet: "bg", Policy: "SCHED_NORMAL"},
			Verdict:              "cpu_competition", Confidence: 0.7, LineStart: 27, LineEnd: 28, Summary: "queued behind bg",
		}},
		ProcessCPULoad: []tracequery.ProcessCPULoadSummary{{
			Process: tracequery.ThreadRef{Comm: "app", PID: 60}, ThreadCount: 4, RunningMs: 9,
			RunnableWaitMs: 3, HighPriorityRunningMs: 1, TopThread: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
			TopThreadMs: 5, CPUs: []int{2, 3}, CoreClasses: []string{"big"}, LineStart: 29, LineEnd: 30, Summary: "app load",
		}},
		StateChurn: []tracequery.ThreadStateChurnSummary{{
			Thread: tracequery.ThreadRef{Comm: "churny", PID: 70}, DominantState: string(tracequery.StateRunnable),
			TotalMs: 20, DominantImpactMs: 9, RunningMs: 4, RunnableMs: 9, SleepMs: 3, DStateMs: 2, IOWaitMs: 2,
			FragmentCount: 12, StateSwitches: 24, MaxSegmentMs: 3, P95SegmentMs: 2,
			RunnableCPU: 2, RunnableCPUKnown: true, TopCompetitor: "rival-71",
			TopCompetitorOverlapMs: 4, TopCompetitorRunningMs: 6,
			NextStep: "check same-cpu competition", NextStepKind: "cpu_competition",
			LineStart: 31, LineEnd: 32, Confidence: 0.7, Summary: "state_cluster churny fragmented",
		}},
		StateDrilldownPlan: []tracequery.StateDrilldownStep{{
			Rank: 1, Thread: tracequery.ThreadRef{Comm: "drill", PID: 80}, State: string(tracequery.StateRunnable),
			ImpactMs: 8, TotalMs: 10, RankImpactMs: 9, WindowProportion: 0.4, Significant: true,
			Source: "state_churn", RecommendedViews: []string{"scheduler_latency_stats"},
			ChainRequired: true, Recursive: true, StartTs: 1.2, EndTs: 1.4, LineStart: 33, LineEnd: 34, Summary: "drill runnable",
		}},
		FileIOByInode: []tracequery.FileIOSummary{{
			Inode: "0x1", Dev: "253,0", EntryName: "data.db", Operation: "read",
			Thread: tracequery.ThreadRef{Comm: "io", PID: 90}, Count: 3, CompletionCount: 3, Bytes: 4096,
			TotalLatencyMs: 2, MaxLatencyMs: 1, Ret: 4096, MinOffset: 0, MaxOffset: 8192,
			Example: "f2fs_dataread_start", LineStart: 35, LineEnd: 36,
		}},
		PageCacheByInode: []tracequery.PageCacheSummary{{
			Inode: "0x2", Dev: "253,0", Thread: tracequery.ThreadRef{Comm: "pc", PID: 91},
			Adds: 5, Deletes: 2, Churn: 7, Bytes: 8192, MinOffset: 0, MaxOffset: 4096, LineStart: 37, LineEnd: 38,
		}},
		// INODE (§28.6): exercises the top_io_inode observation family so its
		// note keys (reads/writes/top_threads/groups_total + the reused io
		// keys) stay inside the registry contract.
		TopIOInodes: &tracequery.TopIOInodeStats{
			Groups: []tracequery.TopIOInodeSummary{{
				Dev: "253,0", Inode: "0x1", EntryName: "data.db",
				Count: 13, FileIOCount: 3, CompletionCount: 3, ReadCount: 2, WriteCount: 1,
				Bytes: 4096, PageCacheAdds: 5, PageCacheDeletes: 2, PageCacheChurn: 7,
				MaxLatencyMs: 1, ThreadCount: 2,
				TopThreadLatencies: []tracequery.TopIOInodeThreadLatency{
					{Thread: tracequery.ThreadRef{Comm: "io", PID: 90}, TotalLatencyMs: 2, Count: 6},
				},
				LineStart: 35, LineEnd: 38, StartTs: 1.0, EndTs: 1.9,
				Summary: "inode=0x1 dev=253,0 events=13",
			}},
			TotalGroups:        2,
			UnidentifiedEvents: 1,
		},
		StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
			Layer: "block", Event: "block_rq", Dev: "253,0", Operation: "W",
			Thread: tracequery.ThreadRef{Comm: "st", PID: 92}, Count: 4, PairedCount: 3,
			UnpairedStartCount: 1, UnpairedDoneCount: 0, MaxLatencyMs: 5, AvgLatencyMs: 2,
			Bytes: 16384, Example: "block_rq_issue", LineStart: 39, LineEnd: 40,
		}},
		IOPressureSummary: &tracequery.IOPressureSummary{
			Signal: "io_pressure", Score: 0.8, BlockMaxLatencyMs: 5, StorageMaxLatencyMs: 4,
			FileIOBytes: 4096, FileIOEvents: 3, PageCacheChurn: 7, IOWaitBlockedCount: 2, DStateMs: 3,
			TopInode: "0x1", TopDev: "253,0", TopEntryName: "data.db", LineStart: 41, LineEnd: 42, Summary: "io pressure",
		},
		IOBurstEpisodes: []tracequery.IOBurstEpisodeSummary{{
			Thread: tracequery.ThreadRef{Comm: "burst", PID: 93}, ChainRelevance: "adjacent",
			DominantSignal: "d_state", DStateMs: 3, IOWaitMs: 2, BlockMaxLatencyMs: 5, StorageMaxLatencyMs: 4,
			TopInode: "0x1", TopDev: "253,0", TopEntryName: "data.db", FileIOBytes: 4096, PageCacheChurn: 7,
			OverlapMs: 1, NearestChainThread: tracequery.ThreadRef{Comm: "dep", PID: 21},
			DurationMs: 6, StartTs: 1.1, EndTs: 1.2, LineStart: 43, LineEnd: 44, Confidence: 0.7, Summary: "io burst",
		}},
		BlockIOByInode: []tracequery.BlockIOByInodeSummary{{
			Inode: "0x1", Dev: "253,0", EntryName: "data.db", Thread: tracequery.ThreadRef{Comm: "bio", PID: 94},
			BlockDev: "253,0", Operation: "W", FileIOBytes: 4096, PageCacheChurn: 7,
			BlockMaxLatencyMs: 5, StorageMaxLatencyMs: 4, NearestBlockThread: tracequery.ThreadRef{Comm: "kblockd", PID: 95},
			LineStart: 45, LineEnd: 46, Summary: "block io by inode",
		}},
		IRQActivity: []tracequery.InterruptActivity{{
			Kind: "irq", CPU: 0, CoreClass: "small", Vector: 11, Name: "dwc3", Count: 9,
			PairedCount: 9, ActiveMs: 2, MaxActiveMs: 1, TargetMask: "0f", TargetCPUs: []int{0, 1},
			StartTs: 1.0, EndTs: 1.9, LineStart: 47, LineEnd: 48, Summary: "irq activity",
		}},
		SchedStatAccounting: []tracequery.SchedStatSummary{{
			Thread: tracequery.ThreadRef{Comm: "acct", PID: 96}, Kind: "wait", Count: 5,
			TotalDelayMs: 4, MaxDelayMs: 2, TotalRuntimeMs: 6, MaxRuntimeMs: 3,
			StartTs: 1.0, EndTs: 1.8, LineStart: 49, LineEnd: 50, Summary: "sched stat",
		}},
		WorkqueueActivity: []tracequery.WorkqueueActivity{{
			Thread: tracequery.ThreadRef{Comm: "kworker", PID: 97}, Work: "0xabc", Function: "flush_work",
			Count: 3, PairedCount: 3, DurationMs: 2, MaxLatencyMs: 1,
			StartTs: 1.0, EndTs: 1.5, LineStart: 51, LineEnd: 52, Summary: "workqueue",
		}},
		DMAFenceActivity: []tracequery.DMAFenceActivity{{
			Thread: tracequery.ThreadRef{Comm: "gpu", PID: 98}, Driver: "kgsl", Timeline: "gfx",
			Context: "7", Seqno: "42", Count: 4, PairedCount: 4, WaitMs: 2, MaxWaitMs: 1,
			StartTs: 1.0, EndTs: 1.6, LineStart: 53, LineEnd: 54, Summary: "dma fence",
		}},
		SupplyPressureSummary: &tracequery.SupplyPressureSummary{
			Signal: "supply_pressure", CPUPressureMs: 30, RunnableWaitMs: 25, HighPriorityRunningMs: 5,
			LowFrequencyCPUs: []int{0, 1}, ClockSetRateCount: 2, ThermalEventCount: 1,
			DDREventCount: 1, L3EventCount: 1, ThroughputEventCount: 1,
			WindowMs: 1000, PressureDensity: 0.03, LineStart: 55, LineEnd: 56, Summary: "supply pressure",
		},
		ComputeSupplyBalance: &tracequery.ComputeSupplyBalance{
			WindowMs: 1000, CPUCount: 8, NominalCapacityMs: 8000, DeliveredComputeMs: 6000,
			SupplyRatio: 0.75, LowFrequencyLossMs: 900, IdleMismatchMs: 700, CoreLimitedMs: 400,
			Summary: "supply balance",
		},
		BIOResources: []tracequery.RuntimeResourceSummary{{
			Operation: "bio_write", Path: "/data/app.db", Thread: tracequery.ThreadRef{Comm: "bio", PID: 94},
			Count: 2, TotalLatencyMs: 3, MaxLatencyMs: 2, Bytes: 4096, Line: 57, Callstack: "submit_bio",
		}},
		TraceSpans: []tracequery.TraceSpanSummary{{
			Thread: tracequery.ThreadRef{Comm: "jit", PID: 99}, Kind: "sync", Name: "JIT compiling foo",
			Category: "runtime", Subcategory: "jit", SemanticClass: "jit_compile",
			StartTs: 1.05, EndTs: 1.15, DurationMs: 100, StartLine: 58, EndLine: 59,
		}},
		PerfSamples: &tracequery.PerfContext{
			SampleCount: 3, TotalPeriod: 9000,
			// Caveats non-empty on purpose: perf_quality_caveats is a
			// soft-consumer contract key (evaluator metric-check supplement
			// parses it back), so the coverage ratchet below requires the
			// fixture to actually emit it.
			Quality: &tracequery.PerfQualitySummary{CPUKnownCount: 3, Caveats: []string{"cpu_unknown"}},
			TopSymbols: []tracequery.PerfHotspot{{
				Symbol: "Logger::flush", DSO: "liblog.so", Event: "cpu-cycles", WeightUnit: "period",
				Source: "systrace", SymbolizationStatus: "symbolized", SampleCount: 3, Period: 9000,
				Percent: 42.5, Threads: []tracequery.ThreadRef{{Comm: "logger", PID: 100}}, LineStart: 60, LineEnd: 61,
			}},
		},
		AbilityEvents: []tracequery.TracePluginSummary{{
			Kind: "ability", Domain: "AAFwk", EventName: "AbilityStart", Metric: "count",
			Value: "1", Category: "lifecycle", Thread: tracequery.ThreadRef{Comm: "ability", PID: 101}, Count: 1, Line: 62,
		}},
	}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{
			{SourcePath: "/trace/primary.ftrace", CausalCompatible: true},
			{SourcePath: "/trace/secondary.ftrace", CausalCompatible: true},
		},
		View:       "frame_root_cause_bundle",
		SourcePath: "/traces/full.systrace",
		TimeStart:  1.0,
		TimeEnd:    2.0,
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{
			TargetResolution: &tracequery.FrameTargetResolution{
				Target: tracequery.ThreadRef{Comm: "app:ui", PID: 61}, Source: "frame_timeline",
				Confidence: 0.9, Window: window, WindowSource: "target_resolution",
				SelectedFrame: &tracequery.FrameTargetCandidate{
					Thread: tracequery.ThreadRef{Comm: "app:ui", PID: 61}, Role: "ui", Phase: "actual",
					Name: "frame", FrameID: "77", Window: window, StartLine: 1, EndLine: 2,
				},
				Candidates: []tracequery.FrameTargetCandidate{{FrameID: "77"}},
			},
			// §29.27② (COV-4): the target_window_states record exercises the
			// five per-state keys + deterministic_running on the wire.
			TargetWindowStates: &tracequery.TargetWindowStateAccount{
				Thread: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				Window: window, WindowMs: 1000,
				RunningMs: 700, RunnableMs: 100, SleepMs: 150, DStateMs: 30, IOWaitMs: 20,
				SleepIOWaitMs: 40, TotalMs: 1000, DeterministicRunningMs: 120,
				// ANSWERFACE-1 件2 (§29.140 G6): the boundary-fold disclosure
				// quartet on the wire (head prefix carried from a recovered
				// pre-window state + tail suffix flushed without a closing
				// event).
				HeadCarryMs: 5, HeadCarryState: "sleep",
				TailOpenMs: 7, TailOpenState: "running",
				FragmentCount: 5, LineStart: 1, LineEnd: 9,
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: window,
			// XLANE-3 件1 (§29.104.2 定谳③, 2026-07-16): the rank board
			// identity triple's target/params halves — exercises the
			// rank_board_target / rank_board_params_fingerprint contract keys.
			Target:                 tracequery.ThreadRef{Comm: "app:ui", PID: 61, TGID: 60},
			BoardParamsFingerprint: "0a1b2c3d",
			// SPANVIS-1 (2026-07-19): one advisory mention family — exercises
			// the business_span_* contract keys (name/count/total/max/lines/
			// basis/hidden/omitted).
			BusinessSpanMentions: &tracequery.BusinessSpanMentionResult{
				Families: []tracequery.BusinessSpanMention{{
					Thread:      tracequery.ThreadRef{Comm: "app:ui", PID: 61, TGID: 60},
					Name:        "Lock contention on a monitor lock (owner tid: 62020)",
					Count:       3,
					TotalMs:     0.612,
					MaxSingleMs: 0.303,
					StartLine:   5, EndLine: 9,
					OnChainBasis: tracequery.BusinessSpanMentionBasisSelf,
					HiddenCount:  3,
				}},
				OmittedFamilies: 2,
			},
			// PARTSPLIT-1 (§29.150④, 2026-07-19): one R4-refusal side-channel
			// record — exercises the gated_composite_edge_account /
			// gated_composite_edge_seat_published contract keys (the item-face
			// pre/post/anchor pair rides the refused seat item below).
			GatedCompositeEdgeShareDisclosures: []tracequery.GatedCompositeEdgeShareDisclosure{{
				Thread:     tracequery.ThreadRef{Comm: "inv-binder", PID: 118},
				PreMs:      13.959,
				PostMs:     0.020,
				AccountMs:  13.979,
				BoundaryTs: 2.000456,
				Via:        tracequery.HostWakeupEdgeAnchorViaDirect,
				LineStart:  114, LineEnd: 115,
			}},
			// RULER2-1 (§29.150②, 2026-07-19): the self runnable two-ruler
			// accounting side-channel record — exercises the six
			// self_two_ruler_* contract keys (per-ruler effs/ranks +
			// same-ruler subtotals; NO cross-ruler total key exists).
			SelfRunnableTwoRuler: &tracequery.SelfRunnableTwoRulerAccounting{
				Thread:         tracequery.ThreadRef{Comm: "app:ui", PID: 61, TGID: 60},
				WallSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 2, EffMs: 3.0}, {Rank: 5, EffMs: 1.0}},
				EdgeSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 4, EffMs: 2.0}},
				WallSubtotalMs: 4.0,
				EdgeSubtotalMs: 2.0,
				LineStart:      3, LineEnd: 4,
			},
			// SELFRUN-DISC (§29.192① (b), 2026-07-21): the self supply-fold
			// 「量不了」 absence disclosure side-channel record — exercises the
			// two self_running_fold_unmeasured_* contract keys (running ==
			// unknown by the KnownMs==0 fold identity).
			SelfRunningFoldUnmeasured: &tracequery.SelfRunningFoldUnmeasuredDisclosure{
				Thread:    tracequery.ThreadRef{Comm: "app:ui", PID: 61, TGID: 60},
				RunningMs: 7.25,
				UnknownMs: 7.25,
				LineStart: 3, LineEnd: 4,
			},
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "runnable_wait", SubjectKind: "thread",
				// CR-3 件③ P11 (2026-07-12): TGID + resolved process comm —
				// exercises the tgid / process_comm contract keys.
				Thread:      tracequery.ThreadRef{Comm: "app:ui", PID: 61, TGID: 60},
				ProcessComm: "app",
				ImpactMs:    12, ProjectedImpactMs: 12, CumulativeImpactMs: 14,
				EffectiveImpactMs: 14, TargetImpactMs: 10, ActualImpactMs: 15, ActualTotalMs: 16,
				ActualStartTs: 0.9, ActualEndTs: 2.05, Score: 0.9, Confidence: 0.85,
				LineStart: 3, LineEnd: 4, Source: "wakeup_chain", Causality: "on_wakeup_chain",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 1,
				DominantState: string(tracequery.StateRunnable),
				RunningMs:     2, RunnableMs: 9, SleepMs: 1, DStateMs: 1, IOWaitMs: 1,
				// DSTATE-REFINE arm a (件③, 2026-07-12): exercises the
				// dstate_all_noniowait + blocked_reason_caller contract keys.
				DStateAllNonIOProven: true, BlockedReasonCaller: "dma_fence_default_wait",
				GatedRunnableMs: 9, GatedRunningDeficitMs: 2,
				PriorityRelationCaliber:       "closed_range_stable",
				PriorityRelationProvenLowerMs: 11, PriorityRelationUnknownOrNonLowerMs: 3,
				PriorityRelationArtifactSources: []string{"artifact:0", "artifact:1"},
				OverlapMs:                       3, EdgeCount: 2,
				NearestChainThread: tracequery.ThreadRef{Comm: "dep", PID: 21},
				NearestChainWindow: tracequery.TimeWindow{StartTs: 1.01, EndTs: 1.02},
				SpanName:           "JIT compiling foo", SpanKind: "sync", SpanCategory: "runtime",
				SpanSubcategory: "jit", SemanticClass: "jit_compile",
				PeriodicSource: true, DetectedPeriodMs: 16.6, LatenessMs: 0.4,
				// CLUSTER-FIX-2 件1: this row rides the freq_only twin basis so
				// the fold_capability_freq_only_reason contract key is exercised
				// (the first fixture row keeps the default_table basis for the
				// judged-verdict keys).
				SupplyFoldBasis: freqOnlyBasis, SupplyFoldDeficitMs: 1, SupplyFoldIdealMs: 4,
				OccurrenceWindows: []tracequery.WakeupCausalOccurrence{occurrence},
				// P0-E2a: lock-lane rank rows publish the holder-resolution origin.
				BlockingKind: "monitor_contention", BlockingPeer: tracequery.ThreadRef{Comm: "holder", PID: 102},
				HolderSource: tracequery.CounterpartSourceContentionPayload, OwnerTidRaw: 0,
				// BLK §15.C: the resolved rank lock row's subject is the holder.
				SubjectIsLockHolder: true,
				// SYM-2 §24.17 R2: the self runnable row's typed below-RT
				// preemption disclosure — exercises the
				// runnable_below_rt_preempted contract key.
				SubjectIsAnalysisTarget: true, RunnableBelowRTPreempted: true,
				Summary: "runnable wait dominated the frame",
			}, {
				// BLK-2 P1/P2: holder-subject blocking_span rank row whose
				// waiter-subject critical_blocking twin (same physical span key,
				// lines 65-66) folds into it — exercises the twin-port lane's
				// contract key (lock_twin_folded) and the re-keyed
				// subject_state_* / subject_chain_* display families.
				Rank: 2, Tier: "secondary", Type: "blocking_span",
				Thread:   tracequery.ThreadRef{Comm: "lockholder", PID: 103},
				ImpactMs: 6, CumulativeImpactMs: 6, EffectiveImpactMs: 6,
				Score: 0.5, Confidence: 0.7,
				// CR-3 件② P10 (2026-07-12): the unconsumed blocked_reason
				// residual pair — exercises blocked_reason_window_count /
				// blocked_reason_window_caller (this row consumed no caller).
				BlockedReasonWindowCount: 3, BlockedReasonWindowCaller: "gpu_fence_wait",
				// §29.50.5 (v5 P1 批 件②, 2026-07-13): the proof-partition
				// honest-remainder marker — exercises
				// dstate_cause_unproven_remainder.
				DStateCauseUnprovenRemainder: true,
				LineStart:                    65, LineEnd: 66,
				Source:    "window_stats.trace_spans.lock_contention",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
				BlockingKind: "monitor_contention",
				BlockingPeer: tracequery.ThreadRef{Comm: "lockwaiter", PID: 104},
				HolderSite:   "Bar.baz(Bar.java:7)",
				// BLOCKFROM (§27.4 G13): exercises the blocking_from_site emission.
				BlockingFromSite:    "Waiter.enter(Waiter.java:9)",
				HolderSource:        tracequery.CounterpartSourceContentionPayload,
				SubjectIsLockHolder: true,
				Summary:             "lock holder lockholder-103 blocked lockwaiter-104",
			}, {
				// DCS E1b/E6 (ledger §23.1): non-chain semantic compile span
				// rank row — exercises the background_rank contract key (typed
				// non-on-chain board position, emitted on semantic rows only).
				Rank: 3, Tier: "tertiary", BackgroundRank: 1, Type: "shader_compile",
				Thread:   tracequery.ThreadRef{Comm: "RenderThread", PID: 105},
				ImpactMs: 4, ProjectedImpactMs: 4, CumulativeImpactMs: 4,
				EffectiveImpactMs: 4, Score: 3.2, Confidence: 0.8,
				LineStart: 70, LineEnd: 71,
				Source:    "window_stats.trace_spans.semantic",
				Causality: "background", ChainRelevance: "background",
				SpanName: "shader_compile warmup", SpanKind: "sync",
				SpanCategory: "shader_compile", SpanSubcategory: "shader",
				SemanticClass: "shader_compile",
				Summary:       "shader compilation span overlapped selected non-chain interval",
			}, {
				// SELF-SEM (§29.61.1, RANK-U Stage 1, 2026-07-13): the analysis
				// target's own deterministic semantic span on the typed self
				// basis — exercises the on_chain_basis contract key (and the
				// honest self_deterministic causality token beside
				// chain_relevance=on_chain).
				Rank: 5, Tier: "tertiary", Type: "class_verification",
				Thread:   tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				ImpactMs: 3.2, ProjectedImpactMs: 3.2, CumulativeImpactMs: 3.2,
				EffectiveImpactMs: 3.2, Score: 2.6, Confidence: 0.82,
				LineStart: 72, LineEnd: 73,
				Source:    "window_stats.trace_spans.semantic",
				Causality: "self_deterministic", ChainRelevance: "on_chain",
				OnChainBasis: tracequery.RootCauseOnChainBasisSelfDeterministicSpan,
				SpanName:     "VerifyClass com.example.Foo", SpanKind: "sync",
				SpanCategory: "runtime_verification", SpanSubcategory: "class_verification",
				SemanticClass: "class_verification",
				Summary:       "class verification span on the analysis target's own thread (self on-chain basis, no wakeup-edge claim)",
			}, {
				// R3-IMPL (§29.88.1, 2026-07-15): a NON-target host's semantic
				// span seated by the host's own in-window wakeup edge toward
				// the target — exercises the host_wakeup_edge_anchor_ts/-via
				// contract keys (SCAN-3 positive sentinel shape: tieba 61839
				// VerifyClass 0.285ms entirely before the 34579.496810 裸边).
				Rank: 10, Tier: "tertiary", Type: "class_verification",
				Thread:   tracequery.ThreadRef{Comm: "T7@ZeusThreadPo", PID: 113},
				ImpactMs: 0.285, ProjectedImpactMs: 0.285, CumulativeImpactMs: 0.285,
				EffectiveImpactMs: 0.285, Score: 0.23, Confidence: 0.82,
				LineStart: 108, LineEnd: 109,
				Source:    "window_stats.trace_spans.semantic",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
				OnChainBasis:            tracequery.RootCauseOnChainBasisHostWakeupEdge,
				HostWakeupEdgeAnchorTs:  34579.496810,
				HostWakeupEdgeAnchorVia: tracequery.HostWakeupEdgeAnchorViaDirect,
				SpanName:                "VerifyClass com.example.Bar", SpanKind: "sync",
				SpanCategory: "runtime_verification", SpanSubcategory: "class_verification",
				SemanticClass: "class_verification",
				Summary:       "class verification span anchored before the host's own in-window wakeup edge toward the analysis target (edge=credential)",
			}, {
				// RCM §24.7.1/§24.10 (2026-07-08): engine same-(thread,type)
				// family-merged rank row — exercises the member_* contract keys
				// (isolated family lane, never folded_*) plus the typed
				// inode/dev distinguishing keys promoted out of Summary prose.
				// F4 (对抗复核收尾): honest OVERLAP shape — member_sum_ms is
				// emitted only when the published value sits BELOW the raw
				// member Σ (here the max_overlap_fallback member MAX 1.136 <
				// Σ 1.598), matching the producer convention.
				Rank: 4, Tier: "tertiary", Type: "block_io_by_inode",
				Thread:   tracequery.ThreadRef{Comm: "RxComputationT", PID: 106},
				ImpactMs: 1.136, ProjectedImpactMs: 1.136, CumulativeImpactMs: 1.136,
				EffectiveImpactMs: 1.136, Score: 1.0, Confidence: 0.76,
				LineStart: 80, LineEnd: 90,
				Source:    "window_stats.block_io_by_inode",
				Causality: "background", ChainRelevance: "background",
				MemberCount: 2, MemberMaxMs: 1.136, MemberMinMs: 0.462,
				MemberSumMs: 1.598, MemberFoldCaliber: tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback,
				MemberRoster: []string{"inode=286395 dev=254:2 1.136ms", "inode=300123 dev=254:2 0.462ms"},
				Dev:          "254:2", Inode: "286395",
				// G1 跨车道对账 (§27.2-G1, 2026-07-09): family-side canonical
				// identity — exercises the rank_family_key contract key.
				RankFamilyKey: "io_latency|pid:106|background|1.000000..2.000000", AbsorbedChainRows: 1,
				Summary: "block IO family merged across two inodes on one thread",
			}, {
				// XLANE-2 件1 (2026-07-17): a SEMANTIC family-merged rank row —
				// exercises the member_line_ranges contract key (complete
				// per-member line ranges, minted all-or-nothing on the semantic
				// family lane only).
				Rank: 5, Tier: "tertiary", Type: "class_verification",
				Thread:   tracequery.ThreadRef{Comm: "app.main", PID: 112},
				ImpactMs: 1.402, ProjectedImpactMs: 1.402, CumulativeImpactMs: 1.402,
				EffectiveImpactMs: 1.402, Score: 0.9, Confidence: 0.7,
				LineStart: 120, LineEnd: 131,
				Source:    "window_stats.trace_spans.semantic",
				Causality: tracequery.RootCauseCausalitySelfDeterministic, ChainRelevance: "on_chain",
				OnChainBasis: tracequery.RootCauseOnChainBasisSelfDeterministicSpan,
				SpanName:     "VerifyClass com.example.Foo", SpanKind: "sync",
				SpanCategory: "runtime_verification", SpanSubcategory: "class_verification",
				SemanticClass: "class_verification",
				MemberCount:   2, MemberMaxMs: 0.912, MemberMinMs: 0.490,
				MemberFoldCaliber: tracequery.RootCauseMemberFoldCaliberSumDisjoint,
				MemberRoster:      []string{"VerifyClass com.example.Foo 0.912ms", "VerifyClass com.example.Baz 0.490ms"},
				MemberLineRanges:  []string{"120..124", "126..131"},
				// SPANTOP-1 件1 (§29.131): the complete per-member wall-clock
				// list rides beside the line ranges — exercises the
				// member_wall_ms contract key (Σ == 1.402 == the published
				// value: the display µs identity holds on this fixture).
				MemberWallMs: []string{"0.912", "0.490"},
				Summary:      "class verification family x2 on the analysis target's own thread",
			}, {
				// XLANE-2 件2 (2026-07-17): the self running supply-fold deficit
				// seat carrying the semantic-overlap disclosure roster —
				// exercises the self_gap_semantic_overlaps contract key.
				Rank: 6, Tier: "tertiary", Type: "running",
				Thread:   tracequery.ThreadRef{Comm: "app.main", PID: 112},
				ImpactMs: 70.0, ProjectedImpactMs: 70.0, CumulativeImpactMs: 70.0,
				EffectiveImpactMs: 20.0, Score: 0.8, Confidence: 0.86,
				LineStart: 1, LineEnd: 500,
				Source:    "thread_timeline.self_running_fold",
				Causality: tracequery.RootCauseCausalitySelfWallClock, ChainRelevance: "on_chain",
				OnChainBasis:  tracequery.RootCauseOnChainBasisSelfWallClockInterval,
				DominantState: "running", RunningMs: 70.0,
				SupplyFoldDeficitMs: 20.0, SupplyFoldIdealMs: 50.0,
				SubjectIsAnalysisTarget: true,
				SelfGapSemanticOverlaps: []tracequery.RootCauseSelfGapSemanticOverlap{
					{OverlapMs: 1.402, LineStart: 120, LineEnd: 131},
				},
				// AXIOM-V2 件1/件2 (2026-07-18): the registry fix-direction
				// attribute and one symmetric cross-direction overlap entry
				// (partner = the class_verification family seat above) —
				// exercises the fix_direction / cross_direction_overlaps
				// contract keys.
				FixDirection: "frequency_thermal",
				CrossDirectionOverlaps: []tracequery.RootCauseCrossDirectionOverlap{
					{OverlapMs: 1.402, LineStart: 120, LineEnd: 131,
						Direction: "self_workload",
						Basis:     tracequery.RootCauseDirectionBasisSemanticMembers},
				},
				// ELIM-V2 (2026-07-18): the 件3 conservation violation finding
				// — exercises the direction_conservation_excess contract key
				// (now compile-parsed for the ◎ 守恒尾行).
				DirectionConservationExcess: &tracequery.RootCauseDirectionConservation{
					Direction: "frequency_thermal", SumMs: 260.0, WindowMs: 200.0, SeatCount: 2,
				},
				Summary: "self running supply-fold deficit seat with a semantic-overlap disclosure",
			}, {
				// RSPA §29.61.10a/b (2026-07-14): the ◇ remainder half of a
				// re-anchored runnable window seat — exercises the
				// chain_anchored / chain_anchor_full /
				// chain_anchor_remainder_seat contract keys (donghu witness:
				// census-full 31.191 = 1.759 anchored + 29.432 remainder).
				Rank: 6, Tier: "tertiary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "JankManager", PID: 109},
				ImpactMs: 29.432, ProjectedImpactMs: 29.432, CumulativeImpactMs: 29.432,
				EffectiveImpactMs: 29.432, Score: 0.4, Confidence: 0.8,
				LineStart: 100, LineEnd: 101,
				Source:    "window_stats",
				Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
				DominantState: string(tracequery.StateRunnable), RunnableMs: 29.432,
				ChainAnchoredMs: 1.759, ChainAnchorFullMs: 31.191, ChainAnchorRemainderSeat: true,
				Summary: "runnable remainder outside its wakeup-dependency windows (no chain credential for these segments)",
			}, {
				// RNB-1 (§29.88 R2, 2026-07-14): the case-A' ownership-divergent
				// remainder seat — exercises chain_anchor_ownership_divergent /
				// chain_anchor_chain_lane / chain_anchor_census (donghu keva-1
				// witness shape: census 2.283 = 2.266 anchored + 0.017 remainder,
				// chain lane's own Σ 2.181 diverging).
				Rank: 8, Tier: "tertiary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "keva-1", PID: 111},
				ImpactMs: 0.017, ProjectedImpactMs: 0.017, CumulativeImpactMs: 0.017,
				EffectiveImpactMs: 0.017, Score: 0.1, Confidence: 0.8,
				LineStart: 104, LineEnd: 105,
				Source:    "window_stats",
				Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
				DominantState: string(tracequery.StateRunnable), RunnableMs: 0.017,
				ChainAnchoredMs: 2.266, ChainAnchorFullMs: 2.283, ChainAnchorRemainderSeat: true,
				ChainAnchorOwnershipDivergent: true, ChainAnchorChainLaneMs: 2.181, ChainAnchorCensusMs: 2.266,
				Summary: "runnable remainder with anchored-ownership divergence (chain seat publishes its own account)",
			}, {
				// RNB-1 R4 (§29.88.2, 2026-07-14): the whole-seat lane-demoted
				// affinity satellite — exercises chain_credential_lane_demoted.
				Rank: 9, Tier: "tertiary", Type: "cpu_affinity_or_cpuset",
				Thread:   tracequery.ThreadRef{Comm: "logd.writer", PID: 112},
				ImpactMs: 47.678, ProjectedImpactMs: 47.678, CumulativeImpactMs: 47.678,
				EffectiveImpactMs: 47.678, Score: 0.2, Confidence: 0.72,
				LineStart: 106, LineEnd: 107,
				Source:    "window_stats.cpu_constraints",
				Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
				DominantState: string(tracequery.StateRunnable), RunnableMs: 47.678,
				ChainCredentialLaneDemoted: true,
				// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): the judgment
				// payload quintet — exercises the cpu_constraint_* contract
				// keys on the same affinity seat that carries them in
				// production.
				CPUConstraintKind:         "sched_switch_next_info",
				CPUConstraintCPUSet:       "background",
				CPUConstraintPolicy:       "next_info affinity=ffb group=2 restricted=true",
				CPUConstraintAllowedCPUs:  []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11},
				CPUConstraintExcludedCPUs: []int{2, 12, 13},
				// R5a (§29.88.4 场景② 按核档, RNB-4 2026-07-15): the tier-
				// exclusion proof pair — the donghu mask=ffb shape's values
				// (allowed max tier 2270000 < global tier 2750000).
				CPUConstraintAllowedMaxTierKHz: 2270000,
				CPUConstraintGlobalMaxTierKHz:  2750000,
				Summary:                        "affinity satellite demoted whole to the adjacent lane (no per-row chain credential)",
			}, {
				// XLANE-1 件1 (§29.104.2, 2026-07-15): the fully-anchored
				// scheduler_latency satellite whose anchored share is
				// represented by the chain-lane seat — exercises the
				// chain_anchor_represented_by_chain_seat contract key
				// (runnable2 witness E11 shape: 23.471 full, values untouched
				// on the ◇ lane).
				Rank: 10, Tier: "tertiary", Type: "scheduler_latency",
				Thread:   tracequery.ThreadRef{Comm: "shadowhook-task", PID: 113},
				ImpactMs: 23.471, ProjectedImpactMs: 23.471, CumulativeImpactMs: 23.471,
				EffectiveImpactMs: 23.471, Score: 0.2, Confidence: 0.7,
				LineStart: 108, LineEnd: 109,
				Source:    "scheduler_latency_stats",
				Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
				DominantState: string(tracequery.StateRunnable), RunnableMs: 23.471,
				ChainAnchorRepresentedByChainSeat: true,
				Summary:                           "fully anchored satellite; anchored share represented by the chain seat (diagnostic projection rides the adjacent lane whole)",
			}, {
				// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the residual
				// (B) aggregate seat after the gated-share split — exercises
				// the gated_share_claimed / gated_share_full /
				// gated_share_claim_seats contract keys (identity: claimed
				// 10.000 + residual 5.000 == full 15.000).
				Rank: 11, Tier: "tertiary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "dep-worker", PID: 116},
				ImpactMs: 5.0, ProjectedImpactMs: 5.0, CumulativeImpactMs: 30.0,
				EffectiveImpactMs: 5.0, Score: 0.25, Confidence: 0.82,
				LineStart: 110, LineEnd: 111,
				Source:    "wakeup_chain.aggregated_impacts",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
				DominantState: string(tracequery.StateRunnable), RunnableMs: 5.0,
				GatedShareClaimedMs: 10.0, GatedShareFullMs: 15.0,
				GatedShareClaimSeats: []string{"300..340"},
				Summary:              "runnable aggregate residual 5.000ms after the interval-accounting split",
			}, {
				// LEVELMERGE-1 件2: the demoted A constituent row — exercises
				// the gated_share_constituent_seat contract key (adjacent
				// lane, value = the claimed share, points at the inversion
				// seat).
				Rank: 0, Tier: "tertiary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "dep-worker", PID: 116},
				ImpactMs: 10.0, ProjectedImpactMs: 10.0, CumulativeImpactMs: 10.0,
				EffectiveImpactMs: 10.0, Score: 0.2, Confidence: 0.82,
				LineStart: 110, LineEnd: 111,
				Source:    "wakeup_chain.aggregated_impacts",
				Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", ChainDepth: 1,
				DominantState: string(tracequery.StateRunnable), RunnableMs: 10.0,
				GatedShareClaimedMs: 10.0, GatedShareFullMs: 15.0,
				GatedShareConstituentSeat: true,
				GatedShareClaimSeats:      []string{"300..340"},
				Summary:                   "runnable share 10.000ms already attributed through the priority-inversion seat gated composite (constituent share only)",
			}, {
				// LEVELMERGE-1 件2: the fail-open disclosure arm — exercises
				// the gated_share_overlap contract key (partial typed
				// inventory; published value untouched, 裁定④ clause).
				Rank: 12, Tier: "tertiary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "dep-partial", PID: 117},
				ImpactMs: 8.0, ProjectedImpactMs: 8.0, CumulativeImpactMs: 12.0,
				EffectiveImpactMs: 8.0, Score: 0.22, Confidence: 0.82,
				LineStart: 112, LineEnd: 113,
				Source:    "wakeup_chain.aggregated_impacts",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
				DominantState: string(tracequery.StateRunnable), RunnableMs: 8.0,
				GatedShareOverlapDisclosureMs: 2.5,
				GatedShareClaimSeats:          []string{"350..360"},
				Summary:                       "8.000ms scheduling-demand account; 2.500ms overlaps the priority-inversion seat branch window (typed inventory incomplete, no value split)",
			}, {
				// PARTSPLIT-1 (§29.150④, 2026-07-19): the R4-mirror-refused
				// gated composite seat — exercises the item-face
				// gated_composite_edge_pre_share / _post_share / _anchor_ts /
				// _anchor_via contract keys (X+Y == RunnableMs, the atomic
				// refusal record; value/lane/ordinal untouched).
				Rank: 0, Tier: "tertiary", Type: "priority_inversion_runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "inv-binder", PID: 118},
				ImpactMs: 13.979, ProjectedImpactMs: 13.979, CumulativeImpactMs: 13.979,
				EffectiveImpactMs: 0.796, Score: 0.2, Confidence: 0.76,
				LineStart: 114, LineEnd: 115,
				Source:    "window_stats",
				Causality: "background", ChainRelevance: "background",
				DominantState: string(tracequery.StateRunnable), RunnableMs: 13.979,
				GatedRunnableMs:               0.796,
				GatedCompositeEdgePreShareMs:  13.959,
				GatedCompositeEdgePostShareMs: 0.020,
				GatedCompositeEdgeAnchorTs:    2.000456,
				GatedCompositeEdgeAnchorVia:   tracequery.HostWakeupEdgeAnchorViaDirect,
				Summary:                       "priority-inversion runnable account 13.979ms; R4-mirror refused the edge-boundary conversion (13.959 pre + 0.020 post), whole seat on its home lane",
			}, {
				// RSPA M-IO (§29.61.10c): the io_latency completion-closure
				// credential — exercises the resource_completion_closure
				// contract key.
				Rank: 7, Tier: "tertiary", Type: "io_latency",
				Thread:   tracequery.ThreadRef{Comm: "irq/143-ufs", PID: 110},
				ImpactMs: 1.4, ProjectedImpactMs: 1.4, CumulativeImpactMs: 1.4,
				EffectiveImpactMs: 1.4, Score: 0.3, Confidence: 0.8,
				LineStart: 102, LineEnd: 103,
				Source:    "window_stats",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 2,
				ResourceCompletionClosure: true,
				Summary:                   "io completion thread woke an anchored D/IO wait of a chain thread inside the IO's lifetime",
			}, {
				// G2/G9 (§27.2/§28.1, 2026-07-09): data-blind-spot rank row —
				// demoted tier=data_gap with Rank=0 (no board seat; the emit
				// face must NOT backfill an ordinal for a tier-carrying row)
				// and the typed trace_gap_kind criterion — exercises the
				// trace_gap_kind emission.
				Rank: 0, Tier: "data_gap", Type: "trace_gap",
				Thread:     tracequery.ThreadRef{Comm: "ghost", PID: 107},
				Confidence: 0.6,
				LineStart:  95, LineEnd: 96,
				// 复核 P3-7: the end-to-end minted form carries the ADJACENT
				// chain lane (the blind-spot subject is a chain node — engine
				// -probed on the gapKindTraceNoEligibleWait fixture), so the
				// fixture mirrors the real wire shape. 复核 P3-5: summary is
				// the narrowed precise form (all intervals below the floor).
				Source: "wakeup_chain", Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
				TraceGapKind: tracequery.TraceGapKindNoEligibleWait,
				Summary:      "scheduler intervals exist in the aligned window but all sit below min_duration_ms (no eligible wait candidate)",
			}, {
				// 复核 P3-1: a tier-carrying Rank=0 row with NO summary — the
				// record Summary fallback must render the no-seat form, never
				// a fabricated "#0" ordinal.
				Rank: 0, Tier: "data_gap", Type: "trace_gap",
				Thread:     tracequery.ThreadRef{Comm: "ghost2", PID: 108},
				Confidence: 0.6,
				LineStart:  97, LineEnd: 98,
				Source:         "wakeup_chain",
				Causality:      "adjacent_to_wakeup_chain",
				ChainRelevance: "adjacent",
				TraceGapKind:   tracequery.TraceGapKindNoSchedData,
			}},
		},
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
			Window: window,
			// P0-E CHAIN-PATH (ledger §22.1): branch-stamped nodes/edges so
			// the per-branch path record (branch=/branches= contract keys)
			// and the impact rows' chain_branch key are exercised.
			Nodes: []tracequery.ChainNode{
				{ID: "n1", Thread: tracequery.ThreadRef{Comm: "app:ui", PID: 61}, Branch: 1, Depth: 0, Impact: &tracequery.WakeupCausalImpact{ChainDepth: 0, ChainBranch: 1}},
				{ID: "n2", Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Branch: 1, Depth: 1, Impact: &tracequery.WakeupCausalImpact{ChainDepth: 1, ChainBranch: 1}},
			},
			Edges: []tracequery.WakeupEdge{{
				From: "n2", To: "n1", Branch: 1,
				Waker:    tracequery.ThreadRef{Comm: "dep", PID: 21},
				Wakee:    tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				WakeupTs: 1.024, WakeupLine: 22, LatencyMs: 14,
				WakerPriority: 10, WakerPriorityClass: "cfs",
				WakeePriority: 20, WakeePriorityClass: "cfs",
				WakerPrioritySource: "closed_range_stable", WakeePrioritySource: "native_exact",
				WakerPriorityArtifactSource: "artifact:0", WakeePriorityArtifactSource: "artifact:0",
				WakeePriorityAuthority: "exact_at_point",
				PriorityRelation:       "waker_higher", PriorityRelationCaliber: "closed_range_stable",
				PriorityInversionCandidate: true,
			}},
			// WAKE-CENSUS (§29.58) / WAKE-CENSUS-D 2A (§29.58.4): the per-pair
			// window-total census with a non-zero pair-cap overflow and the
			// typed exit split, so all seven wakeup_edge_census_* contract
			// keys are exercised by the emit pin.
			WakeupEdgeCensus: []tracequery.WakeupEdgeCensusPair{{
				Waker: tracequery.ThreadRef{Comm: "dep", PID: 21},
				Wakee: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				Count: 3, SleepExitCount: 1, DExitCount: 1, OtherExitCount: 1,
				FirstTs: 1.024, LastTs: 1.9,
			}},
			WakeupEdgeCensusOverflowPairs: 1,
			WakeupEdgeCensusOverflowEdges: 2,
			// PTV5 PTS (#68): the impact list overflows the per-family wire cap
			// so the fold record (folded_rows/folded_min_ms/folded_max_ms/
			// folded_subjects contract keys) is exercised by the fixture.
			CausalImpacts: traceNoteKeysEmitFixtureOverflowImpacts(impact),
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
				Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Path: "dep -> app:ui",
				ChainDepth: 1, ChainBranch: 1, OccurrenceCount: 3, DominantState: string(tracequery.StateSSleep),
				DominantImpactMs: 6, ProjectedImpactMs: 6, TotalMs: 7, ProjectedTotalMs: 7,
				ActualImpactMs: 8, ActualTotalMs: 9, RunningMs: 1, RunnableMs: 1, SleepMs: 4,
				DStateMs: 1, IOWaitMs: 1, ActualRunningMs: 1, ActualRunnableMs: 1, ActualSleepMs: 4,
				ActualDStateMs: 1, ActualIOWaitMs: 1, TargetBlockedMs: 5, FragmentCount: 3,
				StateSwitches: 5, MaxSegmentMs: 2, FirstTs: 1.0, LastTs: 1.9, ActualFirstTs: 0.9,
				ActualLastTs: 2.0, LineStart: 5, LineEnd: 9, PriorityRelation: "waker_higher",
				PriorityRelationCaliber:       "closed_range_stable",
				PriorityRelationProvenLowerMs: 2, PriorityRelationUnknownOrNonLowerMs: 5,
				PriorityRelationArtifactSources: []string{"artifact:0", "artifact:1"},
				PriorityInversion:               true, OccurrenceWindows: []tracequery.WakeupCausalOccurrence{occurrence},
				PeriodicSource: true, DetectedPeriodMs: 16.6, LatenessMs: 1.2, EffectivePeriodicImpactMs: 2.2,
				SupplyFoldBasis: basis, SupplyFoldDeficitMs: 1, SupplyFoldIdealMs: 4,
				Summary: "aggregated dep sleeps",
			}},
			// PTS-2 (#69): the engine-level aggregate top-8 fold member rides
			// the fixture so the aggregate fold record (same folded_* contract
			// keys) is exercised by the emit pin too.
			AggregatedImpactsFold: &tracequery.WakeupCausalAggregateFold{
				Groups: 3, MinImpactMs: 0.5, MaxImpactMs: 2.5,
				Subjects:  []string{"ovfa-500", "ovfb-501"},
				LineStart: 120, LineEnd: 140, FirstTs: 1.1, LastTs: 1.8,
			},
			RootEvidence: []tracequery.RootEvidence{{
				Type: "long_sleep", Thread: tracequery.ThreadRef{Comm: "dep", PID: 21},
				DurationMs: 5, LineStart: 5, LineEnd: 6, Summary: "root sleep", Confidence: 0.8,
			}},
		},
		CriticalBlocking: &tracequery.CriticalBlockingResult{
			Window: window,
			Items: []tracequery.CriticalBlockingCandidate{{
				Type: "monitor_contention", Thread: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				Peer: tracequery.ThreadRef{Comm: "holder", PID: 102}, BlockingKind: "monitor_contention",
				HolderSite: "Foo.bar(Foo.java:42)", Waiters: 2,
				// BLOCKFROM (§27.4 G13): waiter-side blocking call site —
				// exercises the blocking_from_site emission on this face too.
				BlockingFromSite: "App.enter(App.java:11)",
				// P0-E2a display-tier keys — exercised so the emit pin covers them.
				// LOCKNS-FIX 修补 件A (2026-07-16): the typed presence verdict
				// rides beside the raw tid — exercises the owner_tid_presence
				// contract key (the G1 collision shape).
				HolderSource: tracequery.CounterpartSourceWakeupEdge, PeerSource: tracequery.CounterpartSourceWakeupEdge,
				OwnerTidRaw: 987654, OwnerTidPresence: tracequery.OwnerTidPresenceCollision,
				WaitObject: "monitor of Foo",
				// P0-E 锁车道修2 keys — exercised so the emit pin covers them.
				// G10 (§27.4/§28.1, 2026-07-09): the engine mints the witness in
				// Chinese (§22.2.1 词条尺子; number/line formats preserved).
				HolderHandoff:           []string{"WorkerA", "WorkerB"},
				HolderSelfContradiction: "推断持有者 holder-102 自身在同一 payload 持有者 tid 987654 上排队 5.000ms(本段共 6.000ms;行 65-66)",
				// G10-EN 根修 (QH2-A, 2026-07-14): the typed component
				// quintet rides beside the zh string — exercises the five
				// holder_self_contradiction_* contract keys.
				HolderSelfContradictionParts: &types.TraceHolderSelfContradictionWitness{
					Holder: "holder-102", OwnerTid: 987654,
					QueuedMs: 5, SpanMs: 6, LineStart: 65, LineEnd: 66,
				},
				PeerState: &tracequery.ThreadStateBreakdown{
					DominantState: string(tracequery.StateRunning), TotalMs: 6, RunningMs: 4,
					RunnableMs: 1, SleepMs: 1, DStateMs: 1, IOWaitMs: 1, FragmentCount: 2,
				},
				// A1 bounded continuation (§12.3-5) — exercised so the emit pin
				// covers the peer_chain_* display-tier keys (state + direct 1-hop
				// blocker + its always-inferred source + presumptive flag).
				PeerChain: &tracequery.PeerChainStep{
					Peer:                tracequery.ThreadRef{Comm: "holder", PID: 102},
					State:               &tracequery.ThreadStateBreakdown{DominantState: string(tracequery.StateSSleep), TotalMs: 6, SleepMs: 5},
					DirectBlocker:       tracequery.ThreadRef{Comm: "upstream", PID: 130},
					DirectBlockerState:  string(tracequery.StateRunning),
					DirectBlockerSource: tracequery.CounterpartSourceWakeupEdge,
					Presumptive:         true, Confidence: 0.62, Summary: "continuation off holder",
				},
				Flags: "0x10", Oneway: &oneway, SyncLike: &syncLike, BlockingCandidate: &blockingCandidate,
				ChainRelevance: "on_chain", OverlapMs: 2, EdgeCount: 1,
				NearestChainThread: tracequery.ThreadRef{Comm: "dep", PID: 21},
				DurationMs:         4, StartTs: 1.2, EndTs: 1.25, LineStart: 63, LineEnd: 64,
				Confidence: 0.8, Summary: "lock held by holder",
			}, {
				// BLK-2 P1/P2: waiter-subject twin of the rank-2 blocking_span
				// above (same physical span key: kind + lines 65-66 + unordered
				// {103,104} pair) — suppressed by the §15.C ① fold, so its
				// display families ride the rank record re-keyed
				// (subject_state_* / subject_chain_*) plus the lock_twin_folded
				// contract witness.
				Type: "blocking_span", Thread: tracequery.ThreadRef{Comm: "lockwaiter", PID: 104},
				Peer:         tracequery.ThreadRef{Comm: "lockholder", PID: 103},
				BlockingKind: "monitor_contention", HolderSite: "Bar.baz(Bar.java:7)",
				Waiters: 1, WaitObject: "monitor of Bar",
				PeerState: &tracequery.ThreadStateBreakdown{
					DominantState: string(tracequery.StateRunning), TotalMs: 6, RunningMs: 3,
					RunnableMs: 1, SleepMs: 1, DStateMs: 0.5, IOWaitMs: 0.5, FragmentCount: 2,
				},
				PeerChain: &tracequery.PeerChainStep{
					Peer:                tracequery.ThreadRef{Comm: "lockholder", PID: 103},
					State:               &tracequery.ThreadStateBreakdown{DominantState: string(tracequery.StateRunning), TotalMs: 6, RunningMs: 6},
					DirectBlocker:       tracequery.ThreadRef{Comm: "upstream2", PID: 131},
					DirectBlockerState:  string(tracequery.StateSSleep),
					DirectBlockerSource: tracequery.CounterpartSourceWakeupEdge,
					Presumptive:         true, Confidence: 0.6, Summary: "continuation off rank-2 holder",
				},
				DurationMs: 6, StartTs: 1.3, EndTs: 1.36, LineStart: 65, LineEnd: 66,
				Confidence: 0.7, Summary: "monitor contention with owner lockholder",
			}, {
				// XERR1-FIX 件1/件3 (§29.104.3/.4, 2026-07-15): converged
				// payload-less blocking_span row — exercises the
				// blocking_value_basis / blocking_wait_segment_ms /
				// blocking_wait_sleep_ms / blocking_span_envelope_ms and the
				// budget trio contract keys (the customer E1 shape: envelope
				// 199.992 dressed as 阻塞等待 while running 54%).
				Type: "blocking_span", Thread: tracequery.ThreadRef{Comm: "spanwaiter", PID: 105},
				Peer:               tracequery.ThreadRef{Comm: "lastwaker", PID: 107},
				PeerSource:         tracequery.CounterpartSourceWakeupEdge,
				WaitObject:         "H:traversal",
				BlockingValueBasis: tracequery.BlockingValueBasisWaitSegments,
				WaitSegmentMs:      2.5, WaitSleepMs: 1.5, WaitDStateMs: 0.75, WaitIOWaitMs: 0.25,
				SpanEnvelopeMs:     6.0,
				WaitBudgetExceeded: true, WaitBudgetNonRunningMs: 2.5, WaitBudgetRunningMs: 3.5,
				DurationMs: 2.5, StartTs: 1.42, EndTs: 1.426, LineStart: 84, LineEnd: 85,
				Confidence: 0.72, Summary: "blocking-like trace span converged to wait segments",
			}, {
				// XERR1-FIX 修补 件F (冷读 P3-3, 2026-07-16): converged
				// payload-less row whose waiter account did NOT tile the span
				// window — exercises the blocking_wait_coverage_partial /
				// blocking_wait_account_covered_ms contract keys (its own row:
				// the 件3 budget marker and the coverage marker are mutually
				// exclusive by construction — the budget requires full
				// coverage, the marker IS the complement).
				Type: "blocking_span", Thread: tracequery.ThreadRef{Comm: "gapwaiter", PID: 108},
				Peer:               tracequery.ThreadRef{Comm: "lastwaker", PID: 107},
				PeerSource:         tracequery.CounterpartSourceWakeupEdge,
				WaitObject:         "H:traversal partial",
				BlockingValueBasis: tracequery.BlockingValueBasisWaitSegments,
				WaitSegmentMs:      1.2, WaitSleepMs: 1.2,
				SpanEnvelopeMs:      5.0,
				WaitCoveragePartial: true, WaitAccountCoveredMs: 1.5,
				DurationMs: 1.2, StartTs: 1.43, EndTs: 1.435, LineStart: 86, LineEnd: 87,
				Confidence: 0.72, Summary: "blocking-like trace span converged to a proven lower bound",
			}, {
				// LOCKNS-FIX 件6 (§29.104.12, 2026-07-16): rung-② ns-span
				// derived holder with the ②×③ identity-unification
				// declaration — exercises the holder_ns_unification contract
				// key (display_only→hard_consumer, OM-10 关账).
				Type: "blocking_span", Thread: tracequery.ThreadRef{Comm: "nswaiter", PID: 109},
				Peer:                tracequery.ThreadRef{Comm: "nsworker", PID: 110},
				BlockingKind:        "lock_contention",
				HolderSource:        tracequery.CounterpartSourceNsSpanDerivation,
				OwnerTidRaw:         62020,
				HolderNsUnification: "owner_ns_tid=62020 host=nsworker-110 lanes=ns_span_derivation+wakeup_edge",
				HolderHostProcess:   "tgid=59566 ns_pid=60194 level=process",
				DurationMs:          0.5, StartTs: 1.44, EndTs: 1.4405, LineStart: 88, LineEnd: 89,
				Confidence: 0.7, Summary: "ns-span derived holder with two-lane unification",
			}, {
				// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): unknown
				// owner-vocabulary form that fail-opened to the payload-less
				// lane — exercises the blocking_owner_key_unregistered
				// contract key.
				Type: "blocking_span", Thread: tracequery.ThreadRef{Comm: "vendorwaiter", PID: 111},
				WaitObject:           "lock owner=5 style=xyz",
				OwnerKeyUnregistered: true,
				BlockingValueBasis:   tracequery.BlockingValueBasisSpanEnvelope,
				SpanEnvelopeMs:       0.8,
				DurationMs:           0.8, StartTs: 1.45, EndTs: 1.4508, LineStart: 90, LineEnd: 91,
				Confidence: 0.72, Summary: "unregistered owner form rides the payload-less lane",
			}, {
				// HULL-CRED (§29.104 终判③, 2026-07-17): the per-segment-
				// proven demoted D-state view row — exercises the
				// chain_credential_segments + chain_credential_segment_disjoint
				// contract keys (production emission shape: the demote marker,
				// its segment-inventory proof and the R4 lane marker travel on
				// one row; hull [1.460,1.480] intersected the anchor windows
				// while both real segments lay in the hull gap).
				Type: "d_state_or_io_wait", Thread: tracequery.ThreadRef{Comm: "gapblock", PID: 114},
				ChainRelevance:                 "adjacent",
				ChainCredentialLaneDemoted:     true,
				ChainCredentialSegmentDisjoint: true,
				ChainCredentialSegments:        []string{"1.460000..1.462000", "1.478000..1.480000"},
				DurationMs:                     4.0, LineStart: 92, LineEnd: 93,
				Confidence: 0.80, Summary: "gapblock spent 4.000ms in non-IO D-state wait",
			}, {
				// HULL-CRED (§29.104 终判③, 2026-07-17): the envelope-tier
				// conservative keep-⛓ io_wait view row — exercises the
				// chain_credential_envelope_level contract key (segment
				// inventory absent, lane retained, honest word only).
				Type: "io_wait", Thread: tracequery.ThreadRef{Comm: "envio", PID: 115},
				ChainRelevance:               "on_chain",
				OverlapMs:                    1.0,
				EdgeCount:                    1,
				ChainCredentialEnvelopeLevel: true,
				DurationMs:                   2.0, LineStart: 94, LineEnd: 95,
				Confidence: 0.84, Summary: "envio spent 2.000ms in scheduler IO wait",
			}, {
				// ONCHAIN-FIX-2 件3 (Q6, 2026-07-18): the truncated lower-bound
				// prefix keep-⛓ D-state view row — exercises the
				// chain_credential_segments_truncated contract key (production
				// emission shape: the published inventory is the beyond-cap
				// group's checked prefix, ≥1 prefix segment intersected an
				// anchor window, the marker rides beside the inventory on the
				// on-chain lane — 「实际锚定不小于此值」).
				Type: "d_state_or_io_wait", Thread: tracequery.ThreadRef{Comm: "lowbound", PID: 117},
				ChainRelevance:                   "on_chain",
				EdgeCount:                        1,
				ChainCredentialSegments:          []string{"1.500000..1.502000", "1.520000..1.522000"},
				ChainCredentialSegmentsTruncated: true,
				DurationMs:                       80.0, LineStart: 98, LineEnd: 99,
				Confidence: 0.80, Summary: "lowbound spent 80.000ms in non-IO D-state wait",
			}, {
				// ONCHAIN-FIX-1 件1 (2026-07-18): the interval-less identity-
				// inheritance fail-open keep-⛓ D-state view row — exercises the
				// chain_identity_inheritance contract key (production emission
				// shape: no StartTs/EndTs on the wire, overlap honestly absent
				// — the retired pre-fix form fabricated it from the node-window
				// wall clock — and the admission marker rides the on-chain lane).
				Type: "d_state_or_io_wait", Thread: tracequery.ThreadRef{Comm: "idinherit", PID: 116},
				ChainRelevance:           "on_chain",
				EdgeCount:                1,
				ChainIdentityInheritance: true,
				DurationMs:               3.0, LineStart: 96, LineEnd: 97,
				Confidence: 0.80, Summary: "idinherit spent 3.000ms in non-IO D-state wait",
			}, {
				// G1 跨车道对账 (§27.2-G1, 2026-07-09): engine-absorbed
				// io_latency row — exercises the absorbed_by_rank_family /
				// absorbed_into contract keys (the row still publishes, 观测
				// 照发不删; the key value mirrors the family row's
				// rank_family_key above).
				Type: "io_latency", Thread: tracequery.ThreadRef{Comm: "RxComputationT", PID: 106},
				Peer:                 tracequery.ThreadRef{Comm: "udk-irq-0", PID: 73},
				AbsorbedByRankFamily: true,
				AbsorbedIntoFamily:   "io_latency|pid:106|background|1.000000..2.000000",
				DurationMs:           1.136, StartTs: 1.4, EndTs: 1.41, LineStart: 80, LineEnd: 82,
				Confidence: 0.86, Summary: "block IO 254:2 R sector=286395 len=8 took 1.136ms",
			}},
		},
		WindowStats: stats,
		Compactions: []tracequery.ViewCompaction{{}},
	}
}

func TestTraceNoteKeysEmittedSubsetOfRegistry(t *testing.T) {
	records := traceQueryTypedObservations(traceNoteKeysEmitFixtureResult(), "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("fixture produced no observation records — the emit pin is checking nothing")
	}
	emitted := map[string]bool{}
	for _, record := range records {
		for _, note := range record.RichNotes {
			key, _, ok := strings.Cut(note, "=")
			if !ok {
				t.Errorf("rich note without key=value shape on %s: %q", record.ID, note)
				continue
			}
			key = strings.TrimSpace(key)
			emitted[key] = true
			if !types.TraceNoteKeyRegistered(key) {
				t.Errorf("record %s emits UNREGISTERED rich-note key %q (note %q) — register it in types/trace_note_keys.go and walk the change protocol", record.ID, key, note)
			}
		}
	}

	// G2/G9 (§27.2/§28.1, 2026-07-09): the demoted blind-spot records — Rank=0
	// rows that CARRY a Tier — must emit no rank= ordinal (the legacy
	// positional backfill is reserved for identity-less rows with no Tier;
	// resurrecting an ordinal here would re-badge the row on the projection
	// face against the engine's no-board-seat signal, 三面同源) while the
	// typed tier and trace_gap_kind criterion ride the wire.
	var gapRecords []*types.ObservationRecord
	for i := range records {
		if records[i].Object == "trace_gap" {
			gapRecords = append(gapRecords, &records[i])
		}
	}
	if len(gapRecords) != 2 {
		t.Fatalf("fixture drifted: expected the two trace_gap rank observations, got %d", len(gapRecords))
	}
	sawKinds := map[string]bool{}
	for _, gapRecord := range gapRecords {
		sawTier := false
		for _, note := range gapRecord.RichNotes {
			if strings.HasPrefix(note, "rank=") {
				t.Fatalf("a tier-carrying Rank=0 row must not resurrect an ordinal on the note face, got %q", note)
			}
			if note == "tier=data_gap" {
				sawTier = true
			}
			if kind, ok := strings.CutPrefix(note, "trace_gap_kind="); ok {
				sawKinds[kind] = true
			}
		}
		if !sawTier {
			t.Fatalf("every blind-spot record must carry tier=data_gap, got %v", gapRecord.RichNotes)
		}
		// 复核 P3-2: a data blind spot is never a principal answer — the role
		// demotes UNCONDITIONALLY (foreground or not), the provenance follows,
		// and the typed root_cause_data_gap predicate identity is preserved
		// (never blurred into root_cause_background).
		if gapRecord.Role != types.AnswerAggregateRoleSupportingCoverage ||
			gapRecord.ProvenanceLane != types.ObservationProvenanceArtifactSpan {
			t.Fatalf("P3-2: blind-spot records must ride SupportingCoverage/ArtifactSpan, got role=%s lane=%s",
				gapRecord.Role, gapRecord.ProvenanceLane)
		}
		if gapRecord.Predicate != "root_cause_data_gap" || gapRecord.ClaimKey != "root_cause_data_gap" {
			t.Fatalf("P3-2: the typed data_gap predicate identity must survive the role demotion, got predicate=%q claim=%q",
				gapRecord.Predicate, gapRecord.ClaimKey)
		}
		// 复核 P3-1: the summary face never fabricates a "#0" seat.
		if strings.Contains(gapRecord.Summary, "#0") {
			t.Fatalf("P3-1: a Rank=0 row's summary must not print a #0 ordinal, got %q", gapRecord.Summary)
		}
	}
	if !sawKinds["no_eligible_wait"] || !sawKinds["no_sched_data"] {
		t.Fatalf("both typed criterion forms must ride the wire, got %v", sawKinds)
	}
	// 复核 P3-1: the summary-less blind-spot row exercises the fallback — the
	// no-seat wording replaces the ordinal form.
	if !strings.Contains(gapRecords[1].Summary, "(no rank seat)") {
		t.Fatalf("P3-1: the summary fallback for a Rank=0 tier-carrying row must render the no-seat form, got %q", gapRecords[1].Summary)
	}

	// Contract-tier coverage: every consumer-parsed key must actually be
	// emitted by this fixture, except the documented consumer-only aliases.
	consumerOnlyAliases := map[string]bool{
		// Legacy FirstFloat fallback alias; no current producer emits it.
		"effective_impact": true,
	}
	for _, row := range types.TraceNoteKeyRows() {
		if row.Carrier == types.TraceNoteCarrierDisplayOnly {
			continue
		}
		// Ledger-marker rows are emitted by the observation-ledger compile
		// (types package), never by traceQueryTypedObservations; their
		// emission sites are pinned by the types-side consumer pin (rule 4)
		// and exercised by observation_ledger_test.go.
		if row.Family == "ledger_marker" {
			continue
		}
		if consumerOnlyAliases[row.Key] {
			continue
		}
		if !emitted[row.Key] {
			t.Errorf("contract-tier key %q (%s/%s) not emitted by the fixture — extend the fixture so the registry contract stays exercised", row.Key, row.Family, row.Carrier)
		}
	}
}

// BLOCKFROM (§27.4 G13 配套, 2026-07-09): the waiter-side blocking call site
// rides BOTH typed-note faces — the lock rank row and the critical_blocking
// row — under the registered blocking_from_site key, verbatim, exactly like
// holder_site (the DISP-2 batch consumes this key for the 等待点 line).
func TestTraceNoteKeysBlockingFromSiteRidesBothLockFaces(t *testing.T) {
	records := traceQueryTypedObservations(traceNoteKeysEmitFixtureResult(), "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	want := map[string]string{
		// The rank-face fixture row (lockholder-103, blocking_span).
		"blocking_from_site=Waiter.enter(Waiter.java:9)": "",
		// The critical_blocking fixture row (app:ui-61, monitor_contention).
		"blocking_from_site=App.enter(App.java:11)": "",
	}
	for _, record := range records {
		for _, note := range record.RichNotes {
			if _, ok := want[note]; ok {
				want[note] = record.ID
			}
		}
	}
	for note, id := range want {
		if id == "" {
			t.Errorf("typed note %q must ride the wire on its face (rank/critical_blocking)", note)
		}
	}
}
