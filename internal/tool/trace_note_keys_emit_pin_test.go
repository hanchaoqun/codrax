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
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

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
	}
	impact := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Window: window,
		ActualWindow: tracequery.TimeWindow{StartTs: 0.9, EndTs: 2.1},
		ChainDepth:   1, OnChain: true, DominantState: string(tracequery.StateSSleep),
		DominantImpactMs: 4, ProjectedImpactMs: 4, TotalMs: 5, ProjectedTotalMs: 5,
		ActualImpactMs: 6, ActualTotalMs: 7, TargetBlockedMs: 3,
		FragmentCount: 2, StateSwitches: 3, MaxSegmentMs: 2, P95SegmentMs: 1,
		RunningMs: 1, RunnableMs: 1, SleepMs: 2, DStateMs: 1, IOWaitMs: 1,
		ActualRunningMs: 1, ActualRunnableMs: 1, ActualSleepMs: 2, ActualDStateMs: 1, ActualIOWaitMs: 1,
		Priority: 10, PriorityClass: "cfs", TargetPriority: 20, TargetPriorityClass: "cfs",
		PriorityRelation: "waker_higher", PriorityInversionCandidate: true, PriorityInversionGatedMs: 1,
		GatedRunnableMs: 1, GatedRunningDeficitMs: 1,
		NextStep: "inspect the waker", NextStepKind: "wakeup_chain",
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
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: window,
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "runnable_wait", SubjectKind: "thread",
				Thread:   tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				ImpactMs: 12, ProjectedImpactMs: 12, CumulativeImpactMs: 14,
				EffectiveImpactMs: 14, TargetImpactMs: 10, ActualImpactMs: 15, ActualTotalMs: 16,
				ActualStartTs: 0.9, ActualEndTs: 2.05, Score: 0.9, Confidence: 0.85,
				LineStart: 3, LineEnd: 4, Source: "wakeup_chain", Causality: "on_wakeup_chain",
				ChainRelevance: "on_chain", ChainDepth: 1,
				DominantState: string(tracequery.StateRunnable),
				RunningMs:     2, RunnableMs: 9, SleepMs: 1, DStateMs: 1, IOWaitMs: 1,
				GatedRunnableMs: 9, GatedRunningDeficitMs: 2,
				OverlapMs: 3, EdgeCount: 2,
				NearestChainThread: tracequery.ThreadRef{Comm: "dep", PID: 21},
				NearestChainWindow: tracequery.TimeWindow{StartTs: 1.01, EndTs: 1.02},
				SpanName:           "JIT compiling foo", SpanKind: "sync", SpanCategory: "runtime",
				SpanSubcategory: "jit", SemanticClass: "jit_compile",
				PeriodicSource: true, DetectedPeriodMs: 16.6, LatenessMs: 0.4,
				SupplyFoldBasis: basis, SupplyFoldDeficitMs: 1, SupplyFoldIdealMs: 4,
				OccurrenceWindows: []tracequery.WakeupCausalOccurrence{occurrence},
				Summary:           "runnable wait dominated the frame",
			}},
		},
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "app:ui", PID: 61},
			Window: window,
			Nodes: []tracequery.ChainNode{
				{ID: "n1", Thread: tracequery.ThreadRef{Comm: "app:ui", PID: 61}, Impact: &tracequery.WakeupCausalImpact{ChainDepth: 0}},
				{ID: "n2", Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Impact: &tracequery.WakeupCausalImpact{ChainDepth: 1}},
			},
			Edges: []tracequery.WakeupEdge{{
				From: "n2", To: "n1",
				Waker:    tracequery.ThreadRef{Comm: "dep", PID: 21},
				Wakee:    tracequery.ThreadRef{Comm: "app:ui", PID: 61},
				WakeupTs: 1.024, WakeupLine: 22, LatencyMs: 14,
				WakerPriority: 10, WakerPriorityClass: "cfs",
				WakeePriority: 20, WakeePriorityClass: "cfs",
				PriorityRelation: "waker_higher", PriorityInversionCandidate: true,
			}},
			CausalImpacts: []tracequery.WakeupCausalImpact{impact},
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
				Thread: tracequery.ThreadRef{Comm: "dep", PID: 21}, Path: "dep -> app:ui",
				ChainDepth: 1, OccurrenceCount: 3, DominantState: string(tracequery.StateSSleep),
				DominantImpactMs: 6, ProjectedImpactMs: 6, TotalMs: 7, ProjectedTotalMs: 7,
				ActualImpactMs: 8, ActualTotalMs: 9, RunningMs: 1, RunnableMs: 1, SleepMs: 4,
				DStateMs: 1, IOWaitMs: 1, ActualRunningMs: 1, ActualRunnableMs: 1, ActualSleepMs: 4,
				ActualDStateMs: 1, ActualIOWaitMs: 1, TargetBlockedMs: 5, FragmentCount: 3,
				StateSwitches: 5, MaxSegmentMs: 2, FirstTs: 1.0, LastTs: 1.9, ActualFirstTs: 0.9,
				ActualLastTs: 2.0, LineStart: 5, LineEnd: 9, PriorityRelation: "waker_higher",
				PriorityInversion: true, OccurrenceWindows: []tracequery.WakeupCausalOccurrence{occurrence},
				PeriodicSource: true, DetectedPeriodMs: 16.6, LatenessMs: 1.2, EffectivePeriodicImpactMs: 2.2,
				SupplyFoldBasis: basis, SupplyFoldDeficitMs: 1, SupplyFoldIdealMs: 4,
				Summary: "aggregated dep sleeps",
			}},
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
				PeerState: &tracequery.ThreadStateBreakdown{
					DominantState: string(tracequery.StateRunning), TotalMs: 6, RunningMs: 4,
					RunnableMs: 1, SleepMs: 1, DStateMs: 1, IOWaitMs: 1, FragmentCount: 2,
				},
				Flags: "0x10", Oneway: &oneway, SyncLike: &syncLike, BlockingCandidate: &blockingCandidate,
				ChainRelevance: "on_chain", OverlapMs: 2, EdgeCount: 1,
				NearestChainThread: tracequery.ThreadRef{Comm: "dep", PID: 21},
				DurationMs:         4, StartTs: 1.2, EndTs: 1.25, LineStart: 63, LineEnd: 64,
				Confidence: 0.8, Summary: "lock held by holder",
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
