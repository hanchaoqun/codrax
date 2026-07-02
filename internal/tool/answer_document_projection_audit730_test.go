package tool

// §7.30 customer-report audit v2 — renderer-side pins (docs/design/
// trace_layered_root_cause_methodology_audit_20260701.md §7.30):
//   裁定1/2 — aggregate-metric rows and unattachable unknown-thread rows demote
//     from the on-chain tree to the background-pressure stanza, rendered under
//     their metric semantic name / 未定位线程 instead of on-chain placeholders;
//   裁定3 (rendering half) — the flat-fallback header names the typed cause
//     (missing_wakeup vs. recommended-but-not-run wakeup-chain drilldown);
//   裁定4 — every bar row carries a localized dominant-state attribution tag.
// ZH and EN are pinned symmetrically.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func audit730Bus(lang string) *types.BusContext {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	if lang != "" {
		bus.Language = lang
		bus.AnalysisIR.AnswerContract.Language = lang
	}
	return bus
}

func audit730Render(t *testing.T, bus *types.BusContext, obs []types.ObservationRecord, lang string) string {
	t.Helper()
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "observed."}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	renderLang := lang
	if renderLang == "" {
		renderLang = "zh"
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), renderLang)
}

func audit730ChainObs() []types.ObservationRecord {
	return []types.ObservationRecord{
		projV3Obs("root-binder", "root_cause_primary", "root_cause_primary:binder",
			"binder-42", "sleep_wait", "12.000", 12.0, 100, 200,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=s_sleep"),
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "binder-42 -> app-1",
		},
	}
}

// audit730AggregateObs is the berlin complaint-8 shape: a window-scoped
// io_pressure aggregate (typed subject_kind) plus a truly unattributed
// unknown-thread row, both stamped on_chain by the data layer but with no
// attachable relation.
func audit730AggregateObs() []types.ObservationRecord {
	agg := projV3Obs("agg-io", "root_cause_secondary", "root_cause_secondary:io_pressure",
		"unknown-thread", "io_pressure", "20.000", 20.0, 300, 400,
		"rank=3", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"subject_kind=aggregate_metric")
	unknown := projV3Obs("hop-unknown", "wakeup_causal_impact", "wakeup_causal_impact:unknown",
		"unknown-thread", "cpu_pressure", "8.000", 8.0, 500, 600,
		"chain_relevance=on_chain", "causality=on_wakeup_chain")
	return append(audit730ChainObs(), agg, unknown)
}

func TestTraceProjection730AggregateAndUnknownDemoteToBackgroundZH(t *testing.T) {
	bus := audit730Bus("")
	md := audit730Render(t, bus, audit730AggregateObs(), "")

	// 裁定1: the on-chain placeholders disappear from the on-chain surface.
	for _, banned := range []string{"链上·深度未解析", "on-chain·影响点未解析", "未解析线程", "unknown-thread"} {
		if strings.Contains(md, banned) {
			t.Fatalf("demoted rows must not render on-chain placeholder %q:\n%s", banned, md)
		}
	}
	// 裁定2: the aggregate row renders under its metric semantic name, the truly
	// unattributed row as 未定位线程 — both inside the background stanza.
	bgIdx := strings.Index(md, "▒ 背景压力")
	if bgIdx < 0 {
		t.Fatalf("demoted rows must produce the background stanza:\n%s", md)
	}
	for _, want := range []string{"窗口IO压力(聚合)", "未定位线程"} {
		idx := strings.Index(md, want)
		if idx < 0 {
			t.Fatalf("background stanza missing demoted row %q:\n%s", want, md)
		}
		if idx < bgIdx {
			t.Fatalf("demoted row %q rendered before the background stanza (still on-chain):\n%s", want, md)
		}
	}
	// The real chain stays on-chain and anchored.
	for _, want := range []string{"🎯 app-1", "💤 binder-42"} {
		if !strings.Contains(md, want) {
			t.Fatalf("real chain must stay on the tree %q:\n%s", want, md)
		}
	}
	// Detail table: demoted rows carry background layer cells, not primary ones.
	if !strings.Contains(md, "| ▒ 背景 | 背景 · 支撑参考 | 窗口IO压力(聚合) |") {
		t.Fatalf("aggregate detail row must be background-labeled:\n%s", md)
	}
}

func TestTraceProjection730AggregateAndUnknownDemoteToBackgroundEN(t *testing.T) {
	bus := audit730Bus("en")
	md := audit730Render(t, bus, audit730AggregateObs(), "en")

	for _, banned := range []string{"on-chain · depth unresolved", "on-chain · impact point unresolved", "unresolved thread", "unknown-thread"} {
		if strings.Contains(md, banned) {
			t.Fatalf("demoted rows must not render on-chain placeholder %q:\n%s", banned, md)
		}
	}
	bgIdx := strings.Index(md, "▒ Background pressure")
	if bgIdx < 0 {
		t.Fatalf("demoted rows must produce the background stanza:\n%s", md)
	}
	for _, want := range []string{"window IO pressure (aggregate)", "unattributed thread"} {
		idx := strings.Index(md, want)
		if idx < 0 {
			t.Fatalf("background stanza missing demoted row %q:\n%s", want, md)
		}
		if idx < bgIdx {
			t.Fatalf("demoted row %q rendered before the background stanza (still on-chain):\n%s", want, md)
		}
	}
	if !strings.Contains(md, "🎯 app-1") {
		t.Fatalf("real chain must stay anchored:\n%s", md)
	}
}

func TestTraceProjection730FlatFallbackNamesMissingWakeupZH(t *testing.T) {
	bus := audit730Bus("")
	obs := []types.ObservationRecord{
		projV3Obs("root-sleep", "root_cause_primary", "root_cause_primary:app",
			"app-1", "sleep_wait", "9.000", 9.0, 10, 20,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"),
		{
			ID: "undrill", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "missing_wakeup", ClaimKey: "root_evidence:missing_wakeup",
			Subject: "app-1", Object: "sleep_wait", Value: "9.000", Unit: "ms", Confidence: 0.8,
			Span:      types.ObservationSpan{LineStart: 10, LineEnd: 20},
			RichNotes: []string{"impact_ms=9.000", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"},
		},
	}
	md := audit730Render(t, bus, obs, "")
	if !strings.Contains(md, "睡眠区间在所选窗口内无 sched_wakeup 记录,唤醒链无法上溯——按层级平铺展示") {
		t.Fatalf("flat fallback must name the missing_wakeup cause:\n%s", md)
	}
	if strings.Contains(md, "唤醒链路径未解析") {
		t.Fatalf("opaque fallback wording must not render when a typed cause exists:\n%s", md)
	}
}

func TestTraceProjection730FlatFallbackNamesDrilldownNotRunZH(t *testing.T) {
	bus := audit730Bus("")
	obs := []types.ObservationRecord{
		projV3Obs("root-sleep", "root_cause_primary", "root_cause_primary:app",
			"app-1", "sleep_wait", "9.000", 9.0, 10, 20,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"),
		{
			ID: "drill-reco", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "state_drilldown", ClaimKey: "state_drilldown:app-1:s_sleep",
			Subject: "app-1", Object: "s_sleep", Value: "9.000", Unit: "ms", Confidence: 0.74,
			RichNotes: []string{"state=s_sleep", "impact=9.000ms", "source=state_churn",
				"recommended_views=wakeup_chain", "chain_required=true", "recursive=true"},
		},
	}
	md := audit730Render(t, bus, obs, "")
	if !strings.Contains(md, "本轮未执行唤醒链下钻,建议 trace_query view=wakeup_chain——按层级平铺展示") {
		t.Fatalf("flat fallback must name the recommended-not-run cause:\n%s", md)
	}
	if strings.Contains(md, "唤醒链路径未解析") {
		t.Fatalf("opaque fallback wording must not render when a typed cause exists:\n%s", md)
	}
}

func TestTraceProjection730FlatFallbackTwoCausesEN(t *testing.T) {
	bus := audit730Bus("en")
	obs := []types.ObservationRecord{
		projV3Obs("root-sleep", "root_cause_primary", "root_cause_primary:app",
			"app-1", "sleep_wait", "9.000", 9.0, 10, 20,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"),
		{
			ID: "drill-reco", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "state_drilldown", ClaimKey: "state_drilldown:app-1:s_sleep",
			Subject: "app-1", Object: "s_sleep", Value: "9.000", Unit: "ms", Confidence: 0.74,
			RichNotes: []string{"state=s_sleep", "chain_required=true"},
		},
	}
	md := audit730Render(t, bus, obs, "en")
	if !strings.Contains(md, "wakeup-chain drilldown was not run this round — recommend trace_query view=wakeup_chain; layers rendered flat") {
		t.Fatalf("EN flat fallback must name the recommended-not-run cause:\n%s", md)
	}
	if strings.Contains(md, "wakeup path unresolved") {
		t.Fatalf("opaque EN fallback wording must not render when a typed cause exists:\n%s", md)
	}
}

func TestTraceProjection730BarStateAttributionZH(t *testing.T) {
	bus := audit730Bus("")
	obs := []types.ObservationRecord{
		projV3Obs("root-run", "root_cause_primary", "root_cause_primary:worker",
			"worker-2", "compute_supply", "7.000", 7.0, 30, 40,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		projV3Obs("hop-runnable", "wakeup_causal_impact", "wakeup_causal_impact:disp",
			"disp-3", "runnable_delay", "4.000", 4.0, 50, 60,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2", "dominant_state=runnable"),
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "disp-3 -> worker-2 -> app-1",
		},
	}
	md := audit730Render(t, bus, obs, "")
	// 裁定4: bar rows carry the localized dominant-state tag; the legend explains
	// the tag family next to the bar-scale note.
	for _, want := range []string{"运行占用", "可运行等待", "时长条后的状态标签(睡眠等待/可运行等待/运行占用/IO阻塞/D状态)"} {
		if !strings.Contains(md, want) {
			t.Fatalf("bar state attribution missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "bar满格") {
		t.Fatalf("mixed-language bar legend must not render:\n%s", md)
	}
}

func TestTraceProjection730EvidenceLocatorPrefersTimeWindow(t *testing.T) {
	bus := audit730Bus("")
	obs := []types.ObservationRecord{
		{
			ID: "root-io", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:io",
			Subject: "worker-2", Object: "io_wait", Value: "11.000", Unit: "ms", Confidence: 0.9,
			SupportRefs: []string{`D:\temp\南海\xiongqing\berlin.systrace:824646-1624260`},
			Span: types.ObservationSpan{
				LineStart: 824646, LineEnd: 1624260,
				StartTs: 6793222.031, EndTs: 6793225.370,
			},
			RichNotes: []string{"rank=1", "tier=primary", "impact_ms=11.000", "cumulative_impact_ms=11.000",
				"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"},
		},
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "worker-2 -> app-1",
		},
	}
	md := audit730Render(t, bus, obs, "")
	// 裁定6: basename + the node's own time window; the 800k-line range and the
	// raw customer path stay in the raw trace_query record only.
	if !strings.Contains(md, "berlin.systrace [6793222.031–6793225.370s]") {
		t.Fatalf("evidence locator must prefer basename + time window:\n%s", md)
	}
	for _, banned := range []string{`D:\temp`, ":824646-1624260", "xiongqing"} {
		if strings.Contains(md, banned) {
			t.Fatalf("raw customer locator %q must not render:\n%s", banned, md)
		}
	}
	if !strings.Contains(md, "完整定位见原始 trace_query 记录") {
		t.Fatalf("shortened locator must point back to the raw record:\n%s", md)
	}
}

// Flat mode (no wakeup-path trunk): every named row is depthless by
// construction, so the per-row "depth unresolved" / "impact point unresolved"
// placeholders must not spam the detail table — the flat-cause header and the
// causal-position column already carry that information. A typed depth still
// renders, just without the "(detached)" qualifier.
func TestTraceProjection730FlatModeSuppressesDepthlessPlaceholders(t *testing.T) {
	obs := func() []types.ObservationRecord {
		return []types.ObservationRecord{
			projV3Obs("root-sleep", "root_cause_primary", "root_cause_primary:app",
				"app-1", "sleep_wait", "9.000", 9.0, 10, 20,
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"),
			projV3Obs("hop-worker", "wakeup_causal_impact", "wakeup_causal_impact:worker",
				"worker-2", "runnable_delay", "4.000", 4.0, 30, 40,
				"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2", "dominant_state=runnable"),
		}
	}
	zhMD := audit730Render(t, audit730Bus(""), obs(), "")
	for _, banned := range []string{"链上·深度未解析", "on-chain·影响点未解析", "深度2(未接入链)"} {
		if strings.Contains(zhMD, banned) {
			t.Fatalf("flat mode must not render per-row placeholder %q:\n%s", banned, zhMD)
		}
	}
	if !strings.Contains(zhMD, "| 深度2 |") {
		t.Fatalf("flat mode must keep the plain typed depth cell:\n%s", zhMD)
	}
	enMD := audit730Render(t, audit730Bus("en"), obs(), "en")
	for _, banned := range []string{"on-chain · depth unresolved", "on-chain · impact point unresolved", "depth 2 (detached)"} {
		if strings.Contains(enMD, banned) {
			t.Fatalf("EN flat mode must not render per-row placeholder %q:\n%s", banned, enMD)
		}
	}
	if !strings.Contains(enMD, "| depth 2 |") {
		t.Fatalf("EN flat mode must keep the plain typed depth cell:\n%s", enMD)
	}
}

// 裁定1 follow-up: a row demoted to the background stanza must never be named
// as the lead "primary root cause" — when EVERY primary candidate is demoted,
// the lead says so explicitly and points at the background-pressure stanza.
func TestTraceProjection730AllPrimariesDemotedLeadPointsAtBackground(t *testing.T) {
	obs := func() []types.ObservationRecord {
		return []types.ObservationRecord{
			projV3Obs("agg-io", "root_cause_primary", "root_cause_primary:io_pressure",
				"unknown-thread", "io_pressure", "20.000", 20.0, 300, 400,
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
				"subject_kind=aggregate_metric"),
			projV3Obs("hop-unknown", "wakeup_causal_impact", "wakeup_causal_impact:unknown",
				"unknown-thread", "cpu_pressure", "8.000", 8.0, 500, 600,
				"chain_relevance=on_chain", "causality=on_wakeup_chain"),
		}
	}
	zhMD := audit730Render(t, audit730Bus(""), obs(), "")
	if !strings.Contains(zhMD, "**主根因:** 窗口内未定位到链上主根因,见背景压力段。") {
		t.Fatalf("all-demoted lead must point at the background stanza:\n%s", zhMD)
	}
	if strings.Contains(zhMD, "**主根因:** 窗口IO压力(聚合)") {
		t.Fatalf("lead must not name a background-demoted row as primary root cause:\n%s", zhMD)
	}
	enMD := audit730Render(t, audit730Bus("en"), obs(), "en")
	if !strings.Contains(enMD, "no on-chain primary root cause was located in the window — see the background-pressure stanza") {
		t.Fatalf("EN all-demoted lead must point at the background stanza:\n%s", enMD)
	}
}

func TestTraceProjection730BarStateAttributionEN(t *testing.T) {
	bus := audit730Bus("en")
	obs := []types.ObservationRecord{
		projV3Obs("root-run", "root_cause_primary", "root_cause_primary:worker",
			"worker-2", "compute_supply", "7.000", 7.0, 30, 40,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "worker-2 -> app-1",
		},
	}
	md := audit730Render(t, bus, obs, "en")
	for _, want := range []string{"running ·", "sleep wait / runnable wait / running / IO wait / D-state"} {
		if !strings.Contains(md, want) {
			t.Fatalf("EN bar state attribution missing %q:\n%s", want, md)
		}
	}
}
