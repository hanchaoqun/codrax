package tool

import (
	"strings"
	"testing"

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
