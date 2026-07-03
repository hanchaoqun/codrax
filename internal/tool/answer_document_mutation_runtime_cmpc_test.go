package tool

// CMP-C adversarial-review pins (2026-07-04):
//
//   F3 — 单边未采样引导: comparison predicate ∧ EXACTLY one compiled projection
//        ∧ preflight census ≥2 logical trace captures (the SAME F1
//        capture-identity helper the skill-tier directive gate consumes) →
//        the next-step list gains ONE single-sided sampling row that names
//        the unsampled capture when exactly one remains. No comparison
//        overview table, no fabricated comparison data. Two projections keep
//        the existing comparison rows instead; a single-trace session shows
//        neither.
//
//   F5 downstream — 算力供给 comparison column: when EVERY artifact of the
//        comparison overview carries the typed compute_supply_balance ledger
//        observation (ClaimKey "compute_supply_balance", trace_query
//        producer, rich notes supply_ratio= / idle_mismatch_ms= / window_ms=
//        — the CMP-8/CMP-10 cross-lane contract), the table gains the
//        supply column with per-artifact normalized values; any missing or
//        unparseable side drops the WHOLE column (never estimated).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func cmpcPreflight(sources ...string) types.RuntimeArtifactPreflightProfile {
	artifacts := make([]types.RuntimeArtifactPreflightArtifact, 0, len(sources))
	for _, source := range sources {
		artifacts = append(artifacts, types.RuntimeArtifactPreflightArtifact{
			Kind: "trace", Source: source, Carrier: "request_path",
		})
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts:                artifacts,
	})
}

// cmpcSingleSidedBus is the F3 shape: comparison predicate, TWO trace
// captures in the preflight, but only artifact A's records in the ledger.
func cmpcSingleSidedBus() *types.BusContext {
	bus := compareProjBus(true)
	var obs []types.ObservationRecord
	for _, record := range compareProjTwoTraceObs() {
		if record.SourceRef.Path == "7.0B30SP22_7315.systrace" {
			obs = append(obs, record)
		}
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	bus.RuntimeArtifactPreflight = cmpcPreflight("7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace")
	return bus
}

func TestRuntimeTraceNextStepSingleSidedSamplingHintNamesUnsampledCapture(t *testing.T) {
	bus := cmpcSingleSidedBus()
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 1 {
		t.Fatalf("single-sided comparison shape must emit exactly the sampling hint: %+v", items)
	}
	if items[0].Text != "另一份 trace 尚未采样:对 6.0B138_3900.sys.systrace 重复同口径采样(同窗/同视图)后再对比" {
		t.Fatalf("the hint must name the ONE unsampled capture (census minus projected artifact): %q", items[0].Text)
	}
	if items[0].Label != "下一步" || items[0].CitationRef != -1 {
		t.Fatalf("the hint must reuse the next-step item shape: %+v", items[0])
	}
	// The comparison rows belong to the ≥2-projection form only.
	for _, item := range items {
		if strings.Contains(item.Text, "对比两 trace") {
			t.Fatalf("single-sided shape must not emit the two-projection comparison rows: %+v", items)
		}
	}
}

func TestRuntimeTraceNextStepSingleSidedSamplingHintEnglishSurface(t *testing.T) {
	bus := cmpcSingleSidedBus()
	bus.AnalysisIR.AnswerContract.Language = "en"
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 1 ||
		items[0].Text != "The other trace is not sampled yet: repeat the same-caliber sampling (same window/same views) on 6.0B138_3900.sys.systrace, then compare" {
		t.Fatalf("EN single-sided hint mismatch: %+v", items)
	}
}

func TestRuntimeTraceNextStepSingleSidedSamplingHintGenericOnMultipleRemaining(t *testing.T) {
	bus := cmpcSingleSidedBus()
	bus.RuntimeArtifactPreflight = cmpcPreflight(
		"7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace", "5.0B77_1200.systrace")
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 1 ||
		items[0].Text != "另一份 trace 尚未采样:对其余未采样的 trace 工件重复同口径采样(同窗/同视图)后再对比" {
		t.Fatalf(">1 unsampled captures must use the generic phrase, never a guessed name: %+v", items)
	}
}

func TestRuntimeTraceNextStepSingleSidedSamplingHintAbsentOnTwoProjections(t *testing.T) {
	// Both artifacts sampled → the existing comparison rows lead and the
	// single-sided hint must NOT appear (the two forms are mutually
	// exclusive on the compiled partition count).
	bus := compareProjBus(true)
	bus.RuntimeArtifactPreflight = cmpcPreflight("7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace")
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	if len(items) != 2 || !strings.Contains(items[0].Text, "对比两 trace") {
		t.Fatalf("two-projection shape must keep the comparison rows: %+v", items)
	}
	for _, item := range items {
		if strings.Contains(item.Text, "尚未采样") {
			t.Fatalf("two-projection shape must not emit the single-sided hint: %+v", items)
		}
	}
}

func TestRuntimeTraceNextStepSingleSidedSamplingHintAbsentOnSingleTraceSession(t *testing.T) {
	// Comparison predicate but the preflight census knows only ONE capture
	// (including its sibling spellings) → no hint.
	bus := cmpcSingleSidedBus()
	for name, profile := range map[string]types.RuntimeArtifactPreflightProfile{
		"one capture":       cmpcPreflight("7.0B30SP22_7315.systrace"),
		"sibling spellings": cmpcPreflight("7.0B30SP22_7315.systrace", "7.0B30SP22_7315.perftrace"),
		"empty preflight":   {},
	} {
		bus.RuntimeArtifactPreflight = profile
		items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
		for _, item := range items {
			if strings.Contains(item.Text, "尚未采样") {
				t.Fatalf("%s: single-trace session must not emit the sampling hint: %+v", name, items)
			}
		}
	}
}

func TestRuntimeTraceSingleSidedShapeMaterializesHintWithoutCompareTable(t *testing.T) {
	got := compareProjApply(t, cmpcSingleSidedBus())
	if projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare") != nil {
		t.Fatalf("single-sided shape must not fabricate the comparison overview table")
	}
	var next *types.AnswerBlock
	for i := range got.Blocks {
		if got.Blocks[i].ID == "next_steps" {
			next = &got.Blocks[i]
			break
		}
	}
	if next == nil {
		t.Fatalf("single-sided shape must materialize the next-step block: %+v", got.Blocks)
	}
	found := false
	for _, item := range next.Items {
		if strings.Contains(item.Text, "另一份 trace 尚未采样") &&
			strings.Contains(item.Text, "6.0B138_3900.sys.systrace") {
			found = true
		}
	}
	if !found {
		t.Fatalf("materialized next-step block must carry the named sampling hint: %+v", next.Items)
	}
}

// --- F5 downstream: 算力供给 comparison column ------------------------------

// cmpcSupplyObs is a compute_supply_balance ledger observation per the
// CMP-8/CMP-10 cross-lane contract (ClaimKey compute_supply_balance, producer
// trace_query, typed rich notes).
func cmpcSupplyObs(id, artifact string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "compute_supply_balance", ClaimKey: "compute_supply_balance",
		Subject: "cpu-supply", Value: "0", Unit: "ms",
		Span: types.ObservationSpan{LineStart: 1, LineEnd: 2},
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			Path:         artifact,
			ArtifactKind: "trace",
		},
		SupportRefs: []string{fmt.Sprintf("%s:1-2", artifact)},
		RichNotes:   notes,
	}
}

func cmpcSupplyNotes(ratio, delivered, lowFreq, idle, coreLimited, window float64, cpus int) []string {
	return []string{
		fmt.Sprintf("supply_ratio=%.3f", ratio),
		fmt.Sprintf("delivered_cpu_ms=%.3f", delivered),
		fmt.Sprintf("low_freq_loss_cpu_ms=%.3f", lowFreq),
		fmt.Sprintf("idle_mismatch_ms=%.3f", idle),
		fmt.Sprintf("core_limited_cpu_ms=%.3f", coreLimited),
		fmt.Sprintf("window_ms=%.3f", window),
		fmt.Sprintf("cpu_count=%d", cpus),
	}
}

func cmpcSupplyBus(t *testing.T, withB bool) *types.BusContext {
	t.Helper()
	bus := compareProjBus(true)
	obs := compareProjTwoTraceObs()
	obs = append(obs, cmpcSupplyObs("supply-a", "7.0B30SP22_7315.systrace",
		cmpcSupplyNotes(0.446, 90, 30, 20, 62, 101, 2)...))
	if withB {
		obs = append(obs, cmpcSupplyObs("supply-b", "6.0B138_3900.sys.systrace",
			cmpcSupplyNotes(0.812, 164, 5, 3.5, 12, 701, 4)...))
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	return bus
}

func TestTraceProjectionComparisonOverviewSupplyColumnOnDualObservations(t *testing.T) {
	got := compareProjApply(t, cmpcSupplyBus(t, true))
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil {
		t.Fatalf("comparison shape must render the overview table: %+v", got.Blocks)
	}
	supplyCol := -1
	for i, column := range compare.Columns {
		if column == "算力供给(归一化)" {
			supplyCol = i
		}
	}
	if supplyCol != 5 {
		t.Fatalf("dual observations must add the supply column before the window column: %v", compare.Columns)
	}
	if compare.Columns[len(compare.Columns)-1] != "投影窗" {
		t.Fatalf("the window column must stay last: %v", compare.Columns)
	}
	// 数值对位: per-artifact values sit in the SAME column, normalized caliber
	// with units — ratio as %, idle mismatch in ms plus its share of that
	// artifact's own supply window (20/101 → 19.8%, 3.5/701 → 0.5%).
	if got := compare.Items[0].Cells[supplyCol]; got != "供给率 44.6% · 闲置错配 20.000ms(占窗 19.8%)" {
		t.Fatalf("artifact A supply cell mismatch: %q", got)
	}
	if got := compare.Items[1].Cells[supplyCol]; got != "供给率 81.2% · 闲置错配 3.500ms(占窗 0.5%)" {
		t.Fatalf("artifact B supply cell mismatch: %q", got)
	}
	// Every data row keeps cells aligned with the widened column set.
	for _, item := range compare.Items[:2] {
		if len(item.Cells) != len(compare.Columns) {
			t.Fatalf("row width must match the widened columns: %v vs %v", item.Cells, compare.Columns)
		}
	}
}

func TestTraceProjectionComparisonOverviewSupplyColumnAbsentWhenOneSideMissing(t *testing.T) {
	got := compareProjApply(t, cmpcSupplyBus(t, false))
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil {
		t.Fatalf("comparison shape must render the overview table: %+v", got.Blocks)
	}
	joined := strings.Join(compare.Columns, " | ")
	if strings.Contains(joined, "算力供给") {
		t.Fatalf("a single-sided supply observation must not add the column (never estimated): %v", compare.Columns)
	}
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "供给率") {
			t.Fatalf("no supply values may leak into other cells: %+v", item.Cells)
		}
	}
	if len(compare.Columns) != 6 {
		t.Fatalf("without dual observations the table keeps the pre-F5 column set: %v", compare.Columns)
	}
}

func TestTraceProjectionComparisonOverviewSupplyColumnAbsentOnUnparseableNotes(t *testing.T) {
	bus := compareProjBus(true)
	obs := compareProjTwoTraceObs()
	obs = append(obs,
		cmpcSupplyObs("supply-a", "7.0B30SP22_7315.systrace",
			cmpcSupplyNotes(0.446, 90, 30, 20, 62, 101, 2)...),
		// Artifact B's observation exists but its required window_ms note is
		// corrupt → the whole column stays out; nothing is estimated.
		cmpcSupplyObs("supply-b", "6.0B138_3900.sys.systrace",
			"supply_ratio=0.812", "idle_mismatch_ms=3.500", "window_ms=n/a"),
	)
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	got := compareProjApply(t, bus)
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || strings.Contains(strings.Join(compare.Columns, " "), "算力供给") {
		t.Fatalf("unparseable required note must drop the whole supply column: %+v", compare)
	}
}
