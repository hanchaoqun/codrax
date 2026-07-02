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
// keeps a readable name — the floor guarantees at least 20 display cells —
// and every bar row still starts its bar at ONE shared column.
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
	// B1 readability floor: the old whole-label truncation rendered this deep
	// name as "WifiHandlerThre-1109…"-or-shorter fragments; the name budget
	// floor keeps at least 20 display cells of it.
	if !strings.Contains(fence, "WifiHandlerThre-110") {
		t.Fatalf("deep row name lost its readability floor:\n%s", fence)
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
	if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
		t.Fatalf("row width %d exceeds the %d cap:\n%s", w, runtimeTraceProjTreeRowMaxWidth, line)
	}
	if !strings.Contains(line, "[E5(+2)]") {
		t.Fatalf("E# evidence reference must survive tag elision:\n%s", line)
	}
	// §7.30.3 D3: the gated-composite inversion row leads with the dedicated
	// inversion-impact tag instead of a single-state claim.
	if !strings.Contains(line, "反转影响") {
		t.Fatalf("primary state tag must survive tag elision:\n%s", line)
	}
	if !strings.Contains(line, " · … · ") {
		t.Fatalf("elided secondary tags must leave the … marker:\n%s", line)
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
	if strings.Contains(shortLine, "…") {
		t.Fatalf("lean-tag rows must not elide tags:\n%s", shortLine)
	}
	if !strings.Contains(shortLine, "[E32]") {
		t.Fatalf("lean-tag row lost its evidence tag:\n%s", shortLine)
	}

	// Typed ⚠/⛔ markers are load-bearing and must survive elision alongside
	// the primary tag and the E# reference.
	warn := row
	warn.Node.ActualImpactMS = 17.935
	warn.Node.EffectiveImpactMS = 0
	warnLine := runtimeTraceProjTreeRowLine(warn, runtimeTraceProjTreeLabelWidth, 199.992, true, true)
	if !strings.Contains(warnLine, "⚠跨窗(实际17.935ms)") {
		t.Fatalf("cross-window ⚠ marker must survive elision:\n%s", warnLine)
	}
	if !strings.Contains(warnLine, "[E5(+2)]") || !strings.Contains(warnLine, "…") {
		t.Fatalf("elided warn row must keep E# and the elision marker:\n%s", warnLine)
	}
}
