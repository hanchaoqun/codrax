package tool

// answer_document_mutation_runtime_c4_test.go — §7.30.2 C4 (2026-07-02).
// C4a: deterministic semantic optimization points (class_verify / jit_compile /
// shader_compile / runtime_compile spans) surface as an UNCONDITIONAL system
// typed block in the answer body, ZH/EN symmetric, absent when no span exists.
// C4b: tree layout fixes pinned against the real customer render shapes from
// quest_list.txt lines 141-260 — B1 (deep-row names keep a readable floor and
// bars keep one aligned start column per level) and B4 (a tag-heavy row stays
// under the total row-width cap while the primary tag and E# reference survive).

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

func traceSemanticSpanObservation(id, host, class, spanName, value string, lineStart, lineEnd int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "trace_semantic_span",
		ClaimKey:        "trace_semantic_span:" + class,
		Subject:         host,
		Object:          class,
		Value:           value,
		Unit:            "ms",
		SupportRefs:     []string{fmt.Sprintf("app.systrace:%d-%d", lineStart, lineEnd)},
		Span:            types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes: []string{
			"span_name=" + spanName,
			"semantic_class=" + class,
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		},
		Confidence: 0.82,
	}
}

func semanticOptimizationFixtureBus(lang string) *types.BusContext {
	bus := newBusForMutationTest()
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	if lang != "" {
		bus.Language = lang
		ir.AnswerContract = types.AnswerContract{Language: lang}
	}
	bus.AnalysisIR = ir
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-threadpool", "threadpool-400", "io_wait", "11.000", "11.000", 1),
			traceSemanticSpanObservation("semantic-verify", "threadpool-400", "class_verification", "VerifyClass com.example.Foo", "2.000", 8100, 8200),
			traceSemanticSpanObservation("semantic-jit", "jitpool-500", "jit_compile", "JIT compiling boolean android.net.Foo", "5.232", 9100, 9200),
		},
	}}
	return bus
}

// C4a happy path (ZH): the block renders unconditionally from typed
// SemanticSpans with name / host thread / effective cost / E# evidence, and
// the E# tags stay consistent with the projection cluster's evidence index.
func TestApplyAndPersistMutation_MaterializesDeterministicOptimizationBlock(t *testing.T) {
	bus := semanticOptimizationFixtureBus("")
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "threadpool-400 IO wait dominated the window."},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply failed: err=%v res=%+v", err, res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	block := projectionClusterBlock(got.Blocks, "runtime_trace_semantic_optimizations")
	if block == nil || block.Kind != types.BlockTable {
		t.Fatalf("missing deterministic optimization table block: %+v", got.Blocks)
	}
	if block.Title != "确定性优化点" {
		t.Fatalf("ZH title mismatch: %q", block.Title)
	}
	for _, want := range []string{"优化点", "类别", "宿主线程", "有效成本", "证据"} {
		if !stringSliceContains(block.Columns, want) {
			t.Fatalf("optimization block missing column %q: %+v", want, block.Columns)
		}
	}
	if len(block.Items) != 2 {
		t.Fatalf("expected one row per semantic span, got %+v", block.Items)
	}
	assertProjectionRowsCitationFree(t, block)
	flat := ""
	for _, item := range block.Items {
		flat += strings.Join(item.Cells, " | ") + "\n"
	}
	for _, want := range []string{
		"VerifyClass com.example.Foo",
		"class_verification",
		"threadpool-400",
		"2.000ms",
		"JIT compiling boolean android.net.Foo",
		"jit_compile",
		"jitpool-500",
		"5.232ms",
	} {
		if !strings.Contains(flat, want) {
			t.Fatalf("optimization rows missing %q:\n%s", want, flat)
		}
	}
	// E# consistency: every row's evidence tag must appear (bracketed) inside
	// the projection cluster, which shares the same deterministic index.
	clusterText := projectionClusterText(got.Blocks)
	for _, item := range block.Items {
		tag := item.Cells[len(item.Cells)-1]
		if !strings.HasPrefix(tag, "E") {
			t.Fatalf("evidence cell should carry an E# tag, got %q", tag)
		}
		if !strings.Contains(clusterText, "["+tag+"]") {
			t.Fatalf("evidence tag %q not found in projection cluster:\n%s", tag, clusterText)
		}
	}
}

// C4a EN symmetry: same block, English title/columns.
func TestApplyAndPersistMutation_MaterializesDeterministicOptimizationBlockInEnglish(t *testing.T) {
	bus := semanticOptimizationFixtureBus("en")
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "threadpool-400 IO wait dominated the window."},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply failed: err=%v res=%+v", err, res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	block := projectionClusterBlock(got.Blocks, "runtime_trace_semantic_optimizations")
	if block == nil || block.Kind != types.BlockTable {
		t.Fatalf("missing deterministic optimization table block: %+v", got.Blocks)
	}
	if block.Title != "Deterministic Optimization Points" {
		t.Fatalf("EN title mismatch: %q", block.Title)
	}
	for _, want := range []string{"Optimization point", "Class", "Host thread", "Effective cost", "Evidence"} {
		if !stringSliceContains(block.Columns, want) {
			t.Fatalf("optimization block missing EN column %q: %+v", want, block.Columns)
		}
	}
	if len(block.Items) != 2 {
		t.Fatalf("expected one row per semantic span, got %+v", block.Items)
	}
}

// C4a negative: no semantic span in the ledger → no block (never an empty
// placeholder table).
func TestApplyAndPersistMutation_NoSemanticSpansNoOptimizationBlock(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			traceProjectionObservation("root-threadpool", "threadpool-400", "io_wait", "11.000", "11.000", 1),
		},
	}}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "threadpool-400 IO wait dominated the window."},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply failed: err=%v res=%+v", err, res)
	}
	if projectionClusterBlock(bus.Mutable.AnswerDocumentV2().Blocks, "runtime_trace_semantic_optimizations") != nil {
		t.Fatalf("optimization block must not render without semantic spans")
	}
}

// C4b B1: a deep trunk row (the `│ … │ └─唤醒─` shape from quest_list.txt:158)
// keeps a readable name — the floor guarantees 20 display cells UNTIL the F-3
// label-column cap (§7.7 回访聚焦复核 2026-07-04) binds first; past the cap the
// name budget is column-cap − fixed (still a recognizable prefix, detail table
// lossless) so one deep row can no longer lift the whole fence over the NEW-10
// row cap — and every bar row still starts its bar at ONE shared column.
func TestRuntimeTraceProjTreeDeepRowNameFloorAndBarAlignment(t *testing.T) {
	deep := []bool{true, false, false, false, true, false, false}
	rows := []runtimeTraceProjTreeRow{
		{
			Node: types.TraceCausalProjectionNode{
				Subject: "binder:14214_1-17558", Object: "priority_inversion_candidate",
				ImpactMS: 5.388, CumulativeImpactMS: 5.997, StateKind: "runnable", ChainRelevance: "on_chain",
			},
			Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
			Depth: 1, HasData: true, EvidenceTag: "E5(+2)",
		},
		{
			Node: types.TraceCausalProjectionNode{
				Subject: "WifiHandlerThre-11098", Object: "priority_inversion_candidate",
				ImpactMS: 4.482, CumulativeImpactMS: 4.839, StateKind: "runnable", ChainRelevance: "on_chain",
			},
			Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
			Depth: len(deep), Ancestors: deep, Last: true, HasData: true, EvidenceTag: "E8",
		},
		{
			Node: types.TraceCausalProjectionNode{
				Subject: "WifiHandlerThre-11098", Object: "runnable",
				ImpactMS: 3.006, StateKind: "runnable", ChainRelevance: "on_chain",
			},
			Kind: runtimeTraceProjTreeRowCause, Edge: runtimeTraceProjTreeEdgeCause,
			Depth: len(deep) + 1, Ancestors: append(append([]bool{}, deep...), true),
			HasData: true, EvidenceTag: "E9",
		},
	}
	model := runtimeTraceProjTreeModel{
		Target:   "ease.cloudmusic-17487",
		TreeRows: rows,
		WindowMS: 199.992,
		TrunkLen: len(deep) + 1,
	}
	fence := runtimeTraceProjTreeFence(model, true)
	// B1 readability budget under the F-3 cap, PTV4 T2 form: the deep fixed
	// part leaves column-cap − fixed cells for the name, and the T2 middle cut
	// keeps the identity-bearing pid tail whole ("WifiH…-11098") instead of
	// the old tail fragment — the detail blocks keep the full name.
	if !strings.Contains(fence, "…-11098") {
		t.Fatalf("deep row name lost its T2 pid-tail identity:\n%s", fence)
	}
	if width := runtimeTraceProjTreeLabelColumn(model, true); width > runtimeTraceProjTreeLabelColumnMax {
		t.Fatalf("F-3: deep rows must not lift the shared column past the cap (%d > %d)", width, runtimeTraceProjTreeLabelColumnMax)
	}
	// Every deep-row fence line holds the row cap (subordinate detail lines
	// included — the T1 split keeps main lines lean by construction).
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "-11098") {
			continue
		}
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("F-3: deep-row fence line exceeds the row cap (%d cells):\n%s", w, line)
		}
	}
	var cols []int
	for _, line := range strings.Split(fence, "\n") {
		idx := strings.IndexAny(line, "█░▒")
		if idx < 0 {
			continue
		}
		cols = append(cols, runewidth.StringWidth(line[:idx]))
	}
	if len(cols) != len(rows) {
		t.Fatalf("expected %d bar rows, got %d:\n%s", len(rows), len(cols), fence)
	}
	for _, col := range cols[1:] {
		if col != cols[0] {
			t.Fatalf("bar start columns misaligned %v:\n%s", cols, fence)
		}
	}
}

// C4b B4: the tag-heavy primary row shape from quest_list.txt:150 (state +
// layer/priority + action + chain-cum + 影响点 + [E5(+2)]) must fit the total
// row-width cap by eliding secondary tags — the leading state tag and the E#
// evidence reference always survive, with the "…" marker in between.
func TestRuntimeTraceProjTreeRowWidthCapKeepsPrimaryTagAndEvidence(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Role:    types.TraceCausalRolePrimaryRootCause,
			Subject: "binder:14214_1-17558", Object: "priority_inversion_candidate",
			ImpactMS: 5.388, CumulativeImpactMS: 5.997,
			StateKind: "runnable", ChainRelevance: "on_chain",
			SecondaryObjects: []string{"priority_inversion_runnable_wait/runnable"},
		},
		Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
		Depth: 1, HasData: true, EvidenceTag: "E5(+2)",
	}
	line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 199.992, true, true)
	// PTV4 T1: the tag-heavy row SPLITS instead of eliding — every physical
	// line holds the cap, the main line keeps the essentials ([E#] included),
	// and every formerly-elided tag is now fully visible on a "· " subordinate
	// detail line. Nothing is "…"-elided any more (the DropOrder lane is
	// retired; assertion strength only grew).
	lines := strings.Split(line, "\n")
	for _, l := range lines {
		if w := runewidth.StringWidth(l); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("row line width %d exceeds the %d cap:\n%s", w, runtimeTraceProjTreeRowMaxWidth, l)
		}
	}
	if !strings.Contains(lines[0], "[E5(+2)]") {
		t.Fatalf("E# evidence reference must stay on the MAIN line:\n%s", line)
	}
	// §7.30.3 D3 + PTV6-C ruling B (#73): the gated-composite inversion row
	// carries the cause FULL word (name cell truncated → #12 puts it on the
	// first subordinate slot) — never a single-state claim, and never the
	// deleted 反转影响 shape word.
	if !strings.Contains(line, "优先级反转候选") {
		t.Fatalf("cause full word must survive the split:\n%s", line)
	}
	if strings.Contains(line, "反转影响") {
		t.Fatalf("deleted 反转影响 shape word resurfaced (PTV6-C ruling B):\n%s", line)
	}
	// T1 upgrade: the formerly-elided extras are all reachable in the fence.
	// PTV6-C #6: the 影响点 tokens speak the D4 中文（token） combined form.
	for _, want := range []string{"链上累计5.997ms", "影响点 可运行等待反转（priority_inversion_runnable_wait）/runnable"} {
		if !strings.Contains(line, want) {
			t.Fatalf("demoted tag %q must survive on a subordinate line:\n%s", want, line)
		}
	}
	if strings.Contains(line, " · … · ") {
		t.Fatalf("the elision marker is retired — no tag may be dropped:\n%s", line)
	}
	// A lean-tag row (background stanza rows skip the layer/action chips by
	// design) must render its full tag set — no gratuitous elision.
	short := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "isplogcat-1778", Object: "background_io_pressure",
			ImpactMS: 76.476, StateKind: "running", ChainRelevance: "background",
		},
		Kind:    runtimeTraceProjTreeRowBackground,
		HasData: true, EvidenceTag: "E32",
	}
	shortLine := runtimeTraceProjStanzaRowLine(short, runtimeTraceProjTreeLabelWidth, 199.992, true, true)
	// NEW-10: the 44-cell label budget may B1-truncate the NAME (that ellipsis
	// is fine — the detail table keeps the full name); the TAG segment must
	// still render complete, with no elision. PTV6-C #12: the truncated cause
	// word re-renders WHOLE as the first subordinate slot (全词保障), and the
	// main line keeps E#.
	if shortMain := strings.Split(shortLine, "\n")[0]; !strings.Contains(shortMain, "[E32]") {
		t.Fatalf("lean row must keep E# on the main line:\n%s", shortLine)
	}
	for _, want := range []string{"· background_io_pressure", "· running"} {
		if !strings.Contains(shortLine, want) {
			t.Fatalf("lean-tag rows must render their full tag set without elision (%q):\n%s", want, shortLine)
		}
	}
	if shortLines := strings.Split(shortLine, "\n"); len(shortLines) > 1 &&
		!strings.Contains(shortLines[1], "background_io_pressure") {
		t.Fatalf("#12 cause full word must lead the subordinate slots:\n%s", shortLine)
	}

	// Typed ⚠/⊘ markers are T1 Keep 记号: they sit on the MAIN line with their
	// data (⚠实际Xms — PTV4 T4 keeps the value, the 跨窗 semantics live in the
	// legend) and are never demoted or truncated.
	warn := row
	warn.Node.ActualImpactMS = 17.935
	warn.Node.EffectiveImpactMS = 0
	warnLine := runtimeTraceProjTreeRowLine(warn, runtimeTraceProjTreeLabelWidth, 199.992, true, true)
	warnMain := strings.Split(warnLine, "\n")[0]
	if !strings.Contains(warnMain, "⚠实际17.935ms") {
		t.Fatalf("cross-window ⚠ mark must stay on the main line with its actual value:\n%s", warnLine)
	}
	if !strings.Contains(warnMain, "[E5(+2)]") {
		t.Fatalf("warn row must keep E# on the main line:\n%s", warnLine)
	}
}
