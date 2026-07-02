package tool

// Presentation v3 end-to-end goldens (docs/design/trace_projection_presentation_
// v3_20260702.md §8): two sample shapes rendered through the REAL pipeline
// (observations → aggregation → projection → blocks → markdown).
//   - Berlin shape: multi-layer chain + window anchor (window mode) — pins the
//     target-anchored tree, four edge kinds, window bars/%, coverage
//     subtraction, ⚠/⛔ inline markers and the file-grouped evidence roster.
//   - Aweme shape: the REAL customer duplication mess (dual-view running fact,
//     cross-predicate io_latency twins, unknown-thread background flooding, a
//     contradictory inherited attribution) in fallback (no-window) mode — pins
//     the aggregation → rendering contract end to end.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func projV3Obs(id, predicate, claimKey, subject, object, value string, impact float64, lineStart, lineEnd int, notes ...string) types.ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: predicate, ClaimKey: claimKey,
		Subject: subject, Object: object, Value: value, Unit: "ms", Confidence: 0.87,
		SupportRefs: []string{fmt.Sprintf("berlin.systrace:%d-%d", lineStart, lineEnd)},
		Span:        types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:   append(base, notes...),
	}
}

func TestTraceProjectionV3GoldenBerlinShape(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	binder := projV3Obs("root-binder", "root_cause_primary", "root_cause_primary:binder",
		"binder:42591_4-42712", "sleep_wait", "38.400", 38.4, 104233, 104391,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"effective_impact_ms=36.100", "actual_impact_ms=52.700", "dominant_state=s_sleep")
	renderSvc := projV3Obs("root-render", "root_cause_primary", "root_cause_primary:render",
		"RenderService-3021", "compute_supply", "21.300", 21.3, 88120, 88544,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2",
		"effective_impact_ms=27.900", "actual_impact_ms=27.900", "dominant_state=running")
	renderSvc.RichNotes[1] = "cumulative_impact_ms=27.900"
	obs := []types.ObservationRecord{
		{
			ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
			Subject: "oney.hmn.berlin-42591", Object: "frame",
			Span:      types.ObservationSpan{StartTs: 738291.402, EndTs: 738291.466},
			RichNotes: []string{"window_source=query_window"},
		},
		binder,
		renderSvc,
		projV3Obs("hop-disp", "wakeup_causal_impact", "wakeup_causal_impact:disp",
			"DispatchQueue-771", "runnable_delay", "9.800", 9.8, 76001, 76310,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=3",
			"actual_impact_ms=12.100", "dominant_state=runnable"),
		projV3Obs("hop-io", "wakeup_causal_impact", "wakeup_causal_impact:io",
			"IOWorker-8842", "sleep_wait", "6.200", 6.2, 61220, 61377,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=4", "dominant_state=s_sleep"),
		{
			ID: "undrill-io", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "missing_wakeup", ClaimKey: "root_evidence:missing_wakeup",
			Subject: "IOWorker-8842", Object: "sleep_wait", Value: "6.200", Unit: "ms", Confidence: 0.8,
			SupportRefs: []string{"berlin.systrace:61220-61377"},
			Span:        types.ObservationSpan{LineStart: 61220, LineEnd: 61377},
			RichNotes: []string{"impact_ms=6.200", "cumulative_impact_ms=6.200",
				"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=4", "dominant_state=s_sleep"},
		},
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "IOWorker-8842 -> DispatchQueue-771 -> RenderService-3021 -> binder:42591_4-42712 -> oney.hmn.berlin-42591",
		},
		{
			ID: "sem-verify", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "trace_semantic_span", ClaimKey: "trace_semantic_span:class_verification",
			Subject: "RenderService-3021", Object: "class_verification", Value: "4.700", Unit: "ms", Confidence: 0.82,
			SupportRefs: []string{"berlin.systrace:88200-88300"},
			Span:        types.ObservationSpan{LineStart: 88200, LineEnd: 88300},
			RichNotes: []string{"span_name=VerifyClass com.example.render.Pipeline", "semantic_class=class_verification",
				"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2",
				"effective_impact_ms=4.700", "actual_impact_ms=4.700"},
		},
		projV3Obs("adj-gc", "root_cause_context", "root_cause_context:adjacent",
			"HeapTaskDaemon-42610", "gc_pause", "3.900", 3.9, 90100, 90200,
			"chain_relevance=adjacent", "causality=adjacent_to_chain", "dominant_state=running"),
		projV3Obs("bg-codec", "root_cause_background", "root_cause_background:codec",
			"CodecLooper-17604", "background_io_pressure", "18.000", 18.0, 91000, 91100,
			"chain_relevance=background", "causality=background", "dominant_state=d_state"),
		projV3Obs("bg-kswapd", "root_cause_background", "root_cause_background:kswapd",
			"kswapd0-87", "memory_reclaim", "7.300", 7.3, 92000, 92100,
			"chain_relevance=background", "causality=background", "dominant_state=running"),
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "样例摘要。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")

	// Fact-only conclusion + window anchor + coverage subtraction.
	for _, want := range []string{
		"主根因: binder:42591_4-42712 sleep_wait 38.400ms(占窗60%),下钻到 RenderService-3021",
		"关注窗口 738291.402s → 738291.466s,共 64.000ms",
		"on-chain 已归因 38.400ms/60%,未归因残差 25.600ms/40%",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("berlin golden missing lead fact %q:\n%s", want, md)
		}
	}
	// Target-anchored tree: root, four edge kinds, real branches, inline markers.
	// §7.30.2 C4b B4: rows keep the leading state tag, typed ⚠/⛔ markers and
	// the [E#] reference; overflowing secondary tags elide to "…" — the detail
	// table below stays the lossless surface (链上累计 / span host live there).
	for _, want := range []string{
		"🎯 oney.hmn.berlin-42591 ‹用户关注线程›",
		"满格=窗口64.000ms",
		"└─下钻─ 💤 binder:42591_4-42712 · sleep_wait",
		"├─唤醒─ ⚙ RenderService-3021 · compute_supply",
		"└─唤醒─ ⏳ DispatchQueue-771 · runnable_delay",
		"└─唤醒─ 💤 IOWorker-8842 · sleep_wait",
		"└─语义─ ✦ VerifyClass com.example.render.Pipeline",
		"睡眠等待 · … · ⚠跨窗(实际52.700ms) · [E1]",
		"睡眠等待 · … · ⛔无匹配唤醒·链止 · [E4(+1)]",
		"语义优化span·class_verification · … · [E5]",
		"运行占用",
		"可运行等待",
		"◇ 邻近链",
		"▒ 背景压力",
		"▒▒▒░░░░░░░",
		" 60%",
		" 33%",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("berlin golden missing tree surface %q:\n%s", want, md)
		}
	}
	// The undrillable twin merges into the hop row (evidence roster keeps both).
	if !strings.Contains(md, "(+1)") {
		t.Fatalf("merged undrillable twin should surface as an evidence (+n) tag:\n%s", md)
	}
	// Lossless detail: full quad with ⚠, per-row relation, confidence tiers.
	// The chain total (27.900ms) and the semantic host thread elided from the
	// tree rows stay recoverable here.
	for _, want := range []string{
		"| 深度1 | 主根因 · 主要关注 | binder:42591_4-42712 / sleep_wait | 下钻 ▸ oney.hmn.berlin-42591 | sleep / 等待唤醒 | 38.400ms | 38.400ms | 36.100ms | 52.700ms ⚠ |",
		"| 深度2 | 主根因 · 主要关注 | RenderService-3021 / compute_supply | 唤醒 ▸ binder:42591_4-42712 | running / CPU执行 | 21.300ms | 27.900ms | 27.900ms | 27.900ms |",
		"RenderService-3021 / VerifyClass com.example.render.Pipe",
		"语义span ▸ binder:42591_4-42712",
		"语义优化span·class_verification",
		"IOWorker-8842 / sleep_wait ⛔",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("berlin golden missing detail row %q:\n%s", want, md)
		}
	}
	// File-grouped evidence roster.
	if !strings.Contains(md, "全部证据位于 `berlin.systrace`") || !strings.Contains(md, ":104233-104391") {
		t.Fatalf("berlin golden should group evidence by file:\n%s", md)
	}
	// Zero mermaid; single text fence tree.
	if strings.Contains(md, "```mermaid") || strings.Count(md, "```text") != 1 {
		t.Fatalf("v3 must render exactly one monospace tree and zero mermaid:\n%s", md)
	}
}

func TestTraceProjectionV3GoldenAwemeShapeAggregated(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	mk := func(id, predicate, claimKey, subject, object string, impact float64, lineStart, lineEnd int, notes ...string) types.ObservationRecord {
		r := projV3Obs(id, predicate, claimKey, subject, object, fmt.Sprintf("%.3f", impact), impact, lineStart, lineEnd, notes...)
		r.SupportRefs = []string{fmt.Sprintf("aweme.sys.ftrace:%d-%d", lineStart, lineEnd)}
		return r
	}
	dual := mk("E1", "root_cause_primary", "root_cause_primary:running", "#RxComputationT-16816", "running", 112.103, 45689, 79000,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"actual_impact_ms=59.050", "dominant_state=running")
	dual.RichNotes[0] = "impact_ms=58.919"
	dualTwin := mk("E12", "wakeup_causal_impact", "wakeup_causal_impact:running", "#RxComputationT-16816", "running", 58.919, 45689, 79000,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1", "actual_impact_ms=59.050", "dominant_state=running")
	blockIO := mk("E2", "root_cause_primary", "root_cause_primary:blockio", "#RxComputationT-16816", "block_io_by_inode", 1.136, 59099, 77000,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"effective_impact_ms=112.175")
	obs := []types.ObservationRecord{
		dual, dualTwin, blockIO,
		// io_latency instances + same-interval critical_blocking twins.
		mk("E3", "root_cause_primary", "root_cause_primary:io1", "#RxComputationT-16816", "io_latency", 0.568, 59938, 60100,
			"rank=5", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		mk("E14", "critical_blocking", "critical_blocking:io1", "#RxComputationT-16816", "udk-irq-10-90", 0.568, 59938, 60100,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		mk("E4", "root_cause_primary", "root_cause_primary:io2", "#RxComputationT-16816", "io_latency", 0.500, 63943, 64100,
			"rank=6", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		mk("E15", "critical_blocking", "critical_blocking:io2", "#RxComputationT-16816", "udk-irq-2-77", 0.500, 63943, 64100,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		mk("E5", "root_cause_primary", "root_cause_primary:io3", "#RxComputationT-16816", "io_latency", 0.499, 59809, 59900,
			"rank=7", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		// Target's own blocked-wait view.
		mk("E11", "critical_blocking", "critical_blocking:self", ".ugc.aweme.lite-16547", "s_sleep", 112.175, 45450, 79000,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2", "dominant_state=s_sleep"),
		// Background flooding on the unknown-thread sentinel.
		mk("E21", "root_cause_background", "root_cause_background:b1", "isplogcat-1764", "unknown-thread", 117.928, 45524, 45524,
			"chain_relevance=background", "causality=background"),
		mk("E23", "root_cause_background", "root_cause_background:b2", "VSyncGenerator-2290", "unknown-thread", 98.501, 53244, 81000,
			"chain_relevance=background", "causality=background"),
		mk("E24", "root_cause_background", "root_cause_background:b3", "#tp-io-2036-16781", "unknown-thread", 12.689, 46055, 50000,
			"chain_relevance=background", "causality=background"),
		mk("E25", "root_cause_background", "root_cause_background:b4", "CodecLooper-17604", "unknown-thread", 6.886, 60560, 65000,
			"chain_relevance=background", "causality=background"),
		mk("E26", "root_cause_background", "root_cause_background:b5", "ProcessEvent_t-17599", "unknown-thread", 4.355, 79382, 81000,
			"chain_relevance=background", "causality=background"),
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "#RxComputationT-16816 -> .ugc.aweme.lite-16547",
		},
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "客户案例摘要。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")

	// Fallback scale (no window anchor): declared, never fabricated — no
	// conclusion-line window share and no bar-column percentages.
	if !strings.Contains(md, "窗口起止未采集") || !strings.Contains(md, "满格=本批最大117.928ms") ||
		strings.Contains(md, "(占窗") {
		t.Fatalf("aweme golden must run in fallback scale without window percentages:\n%s", md)
	}
	// Target anchor + its own state line under the root.
	for _, want := range []string{"🎯 .ugc.aweme.lite-16547 ‹用户关注线程›", "💤 s_sleep 112.175"} {
		if !strings.Contains(md, want) {
			t.Fatalf("aweme golden missing target/self surface %q:\n%s", want, md)
		}
	}
	// R1 dual-view: ONE running row with per-layer 58.919 + chain total 112.103.
	// C4b: the tree row elides the chain-total tag under the width cap; the
	// lossless detail row (窗口投影 58.919ms | 链上累计 112.103ms) carries it.
	if strings.Count(md, "· running") > 2 { // tree row + detail row only
		t.Fatalf("dual-view running fact must render once per surface:\n%s", md)
	}
	for _, want := range []string{"⚙ #RxComputationT-16816 · running", "| 58.919ms | 112.103ms |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("aweme golden missing merged dual-view %q:\n%s", want, md)
		}
	}
	// R1+R2: io_latency renders as ONE ×3 aggregate with the udk-irq peers kept
	// (merged range + impact points live on the lossless detail row after C4b).
	for _, want := range []string{"×3(0.499–0.568ms)", "udk-irq-10-90"} {
		if !strings.Contains(md, want) {
			t.Fatalf("aweme golden missing io_latency aggregation %q:\n%s", want, md)
		}
	}
	if strings.Count(md, "io_latency") > 2 { // one tree row + one detail row
		t.Fatalf("io_latency must not flood the report after aggregation:\n%s", md)
	}
	// Contradictory inherited attribution is annotated, not shown bare.
	if !strings.Contains(md, "承自等待区间") {
		t.Fatalf("inherited attribution must be annotated:\n%s", md)
	}
	// R3: background keeps top-2 + one fold row; unknown-thread translated.
	for _, want := range []string{"isplogcat-1764", "VSyncGenerator-2290", "其余 3 项合并"} {
		if !strings.Contains(md, want) {
			t.Fatalf("aweme golden missing background fold %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "unknown-thread") {
		t.Fatalf("unknown-thread sentinel must render translated:\n%s", md)
	}
	if strings.Contains(md, "CodecLooper-17604 ·") {
		t.Fatalf("folded background rows must not render individually in the tree:\n%s", md)
	}
	if strings.Contains(md, "```mermaid") {
		t.Fatalf("v3 is zero-mermaid:\n%s", md)
	}
}
