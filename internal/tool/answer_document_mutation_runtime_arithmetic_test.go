package tool

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceArithmeticRelationCaveatRecomputesCustomerMismatch(t *testing.T) {
	const original = "累计约 1.0ms，占比 0.44%。8 段碎片合计约 0.817ms，总 CPU 占比仅 0.44%。"
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: original,
		}},
	}
	ctx := runtimeTraceArithmeticTestContext("complete", true)
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, ctx) {
		t.Fatal("expected arithmetic relation caveat")
	}
	if got := doc.Blocks[0].Text; got != original {
		t.Fatalf("model prose was rewritten:\n got: %q\nwant: %q", got, original)
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"0.817ms / 0.440%",
		"typed 窗长 227.367ms",
		"重算为 0.359%",
		"差值 0.081 个百分点",
		"统一容差 0.005",
		"completeness=complete",
		"正文保留未改写",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("arithmetic caveat missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "1.000ms / 0.440%") {
		t.Fatalf("correct rounded relation should not be flagged:\n%s", got)
	}
}

func TestRuntimeTraceArithmeticRelationCaveatDisclosesIncompleteNumerator(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "累计约 1.0ms，占比 0.44%。",
		}},
	}
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, runtimeTraceArithmeticTestContext("incomplete", true)) {
		t.Fatal("expected incomplete-enumeration caveat even though displayed arithmetic rounds correctly")
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"关系复算为 0.440%",
		"completeness=incomplete",
		"无法确认该分子是完整总量",
		"正文保留未改写",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("incomplete relation caveat missing %q:\n%s", want, got)
		}
	}
}

func TestRuntimeTraceArithmeticRelationCaveatFailsClosedWithoutUniqueWindow(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "碎片合计 0.817ms，占比 0.44%。",
		}},
	}
	ctx := runtimeTraceArithmeticTestContext("complete", false)
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, ctx) {
		t.Fatal("expected denominator-unavailable caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	if !strings.Contains(got, "typed 窗长无法唯一定位，关系未复算") ||
		!strings.Contains(got, "正文保留未改写") {
		t.Fatalf("denominator caveat = %q", got)
	}
}

func TestRuntimeTraceArithmeticRelationCaveatDeduplicatesRepeatedClaims(t *testing.T) {
	// no_window.txt 客户形:同一断言在正文出现两次,词面精度不同(18.76% 与
	// 18.760%)但解析值与 %.3f 渲染串完全相同 —— 修前逐匹配各发一条注,
	// 答案里出现两条 byte-identical 的「无法唯一定位」句。
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "timerfd_read 阻塞 42.668ms（占窗口 18.76%）。该调用阻塞 42.668ms，占整个窗口的 18.760%。",
		}},
	}
	ctx := runtimeTraceArithmeticTestContext("incomplete", false)
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, ctx) {
		t.Fatal("expected denominator-unavailable caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	const sentence = "正文 42.668ms / 18.760% 的 typed 窗长无法唯一定位"
	if n := strings.Count(got, sentence); n != 1 {
		t.Fatalf("repeated identical claim produced %d notes, want exactly 1:\n%s", n, got)
	}
}

func TestRuntimeTraceArithmeticRelationCaveatRequiresTypedTraceQuery(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "耗时 0.817ms，占比 0.44%。",
		}},
	}
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{ToolName: "grep", Success: true}}}
	if materializeRuntimeTraceArithmeticRelationCaveat(doc, ctx) {
		t.Fatalf("non-trace answer gained arithmetic caveat: %+v", doc.Caveats)
	}
}

func TestRuntimeTraceArithmeticRelationCaveatIsWiredThroughPersistInEnglish(t *testing.T) {
	const original = "Eight slices total 0.817ms, 0.44% of the selected window."
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: original,
		}},
	}
	ctx := runtimeTraceArithmeticTestContext("complete", true)
	ctx.Mutable = types.NewMutableState("trace arithmetic wiring")
	ctx.Language = "en"
	result, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("persist failed: err=%v result=%+v", err, result)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("persisted answer document missing")
	}
	if got.Blocks[0].Text != original {
		t.Fatalf("persist-time arithmetic check rewrote model prose: %q", got.Blocks[0].Text)
	}
	caveats := strings.Join(got.Caveats, "\n")
	for _, want := range []string{
		"Arithmetic relation check:",
		"model text 0.817ms / 0.440%",
		"recomputes to 0.359%",
		"typed 227.367ms window",
		"0.081 percentage-point difference",
		"model prose was retained unchanged",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("persisted English arithmetic caveat missing %q:\n%s", want, caveats)
		}
	}
}

func TestRuntimeTraceArithmeticRelationCaveatElectsDenominatorAcrossWindows(t *testing.T) {
	// ARITH-DENOM(NW-WIN-TYPED 拆件):多窗 census 下按算术自洽唯一性判别分母。
	windows := []string{
		"selected_window=69326.832743749..69327.060110624", // 227.367ms(用户窗)
		"selected_window=69326.832743749..69326.875412",    // 42.668ms(phase 1)
		"selected_window=69326.875412..69327.060110624",    // 184.699ms(phase 2)
	}
	// 臂1(客户实形):18.76% 与三窗均不自洽(真值 18.768%)→ mismatch-vs-all。
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "timerfd_read 阻塞 42.668ms，占窗口 18.76%。",
		}},
	}
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, runtimeTraceArithmeticMultiWindowTestContext("incomplete", windows)) {
		t.Fatal("expected mismatch-vs-all caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"与全部 3 个 typed 窗长均不自洽",
		"最接近的窗长 227.367ms",
		"重算为 18.766%",
		"差值 0.006 个百分点超过统一容差 0.005",
		"正文保留未改写",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mismatch-vs-all caveat missing %q:\n%s", want, got)
		}
	}
	// 臂2:恰一窗自洽(21.334/42.668=50.001%)→ 分母判别披露(incomplete 才出注)。
	doc = &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "该阶段耗时 21.334ms，占比 50.0%。",
		}},
	}
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, runtimeTraceArithmeticMultiWindowTestContext("incomplete", windows[:2])) {
		t.Fatal("expected elected-denominator caveat")
	}
	got = strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"关系复算为 50.000%",
		"分母=2 个 typed 窗长中唯一算术自洽的 42.668ms",
		"completeness=incomplete",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("elected-denominator caveat missing %q:\n%s", want, got)
		}
	}
	// 臂2b:同形 completeness=complete → 自洽即静默(与单窗 checked 臂对称)。
	doc = &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "该阶段耗时 21.334ms，占比 50.0%。",
		}},
	}
	if materializeRuntimeTraceArithmeticRelationCaveat(doc, runtimeTraceArithmeticMultiWindowTestContext("complete", windows[:2])) {
		t.Fatalf("complete consistent relation must stay silent: %q", doc.Caveats)
	}
	// 臂3:多窗同时自洽(0.001ms/0.0%)→ 维持 unverified 词面。
	doc = &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "碎片仅 0.001ms，占比 0.0%。",
		}},
	}
	if !materializeRuntimeTraceArithmeticRelationCaveat(doc, runtimeTraceArithmeticMultiWindowTestContext("incomplete", windows[:2])) {
		t.Fatal("expected ambiguous-window caveat")
	}
	if got := strings.Join(doc.Caveats, "\n"); !strings.Contains(got, "typed 窗长无法唯一定位") {
		t.Fatalf("multi-consistent relation must keep the unverified wording: %s", got)
	}
}

func runtimeTraceArithmeticMultiWindowTestContext(completeness string, windows []string) *types.BusContext {
	result := types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: completeness,
		},
	}
	for i, window := range windows {
		result.Observations = append(result.Observations, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:customer#root_cause_rank:%d", i+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			Subject:         "worker",
			Object:          "runnable",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes:       []string{"tier=primary", window},
			Confidence:      0.8,
		})
	}
	return &types.BusContext{ToolResults: []types.ToolResult{result}}
}

func runtimeTraceArithmeticTestContext(completeness string, includeWindow bool) *types.BusContext {
	notes := []string{"tier=primary"}
	if includeWindow {
		notes = append(notes, "selected_window=69326.832743749..69327.060110624")
	}
	result := types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: completeness,
		},
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:customer#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			Subject:         "worker",
			Object:          "runnable",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes:       notes,
			Confidence:      0.8,
		}},
	}
	return &types.BusContext{ToolResults: []types.ToolResult{result}}
}
