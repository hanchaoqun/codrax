package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// NW-03 production-wiring pin: one background/context projection must not
// suppress the causal/enumeration authority block. The customer no_window
// shape had exactly this form: a count-only IO row made cluster non-empty and
// hid frame_causality=unproven.
func TestTraceProjectionNonEmptyBackgroundStillPublishesAuthority(t *testing.T) {
	ctx := traceAuthorityWiringContext("zh")
	records := traceQueryTypedObservations(
		countOnlyIOCaliberResult(), "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC(),
	)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Observations: records,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "absent",
			CausalConclusion:    "unproven",
		},
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: "incomplete",
			Boundaries: []types.ToolEnumerationBoundary{{
				Scope: "event_search", Dimension: "events", Emitted: 8,
			}},
		},
	})

	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	if !materializeRuntimeTraceCausalProjectionBlock(doc, ctx) {
		t.Fatal("non-empty context projection must materialize")
	}
	if projectionClusterBlock(doc.Blocks, runtimeTraceCausalProjectionBlockIDBase) == nil {
		t.Fatalf("background projection lead missing: %+v", doc.Blocks)
	}
	coverage := projectionClusterBlock(doc.Blocks, runtimeTraceCausalProjectionCoverageBlockID)
	if coverage == nil {
		t.Fatalf("non-empty projection swallowed authority block: %+v", doc.Blocks)
	}
	for _, want := range []string{
		"帧级因果尚未证明",
		"未找到可绑定到目标的帧或截止期证据",
		"枚举未完整",
		"事件检索的事件已展示 8 项，总数未知",
	} {
		if !strings.Contains(coverage.Text, want) {
			t.Fatalf("authority block missing %q:\n%s", want, coverage.Text)
		}
	}
}

func TestTraceProjectionReadsAuthorityFromSystemSupplementResults(t *testing.T) {
	ctx := traceAuthorityWiringContext("en")
	records := traceQueryTypedObservations(
		countOnlyIOCaliberResult(), "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC(),
	)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true, Observations: records,
	})
	ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views: []string{"frame_root_cause_bundle"},
	}, []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "unavailable",
			CausalConclusion:    "unproven",
		},
	}})

	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	if !materializeRuntimeTraceCausalProjectionBlock(doc, ctx) {
		t.Fatal("supplement authority fixture must materialize")
	}
	coverage := projectionClusterBlock(doc.Blocks, runtimeTraceCausalProjectionCoverageBlockID)
	if coverage == nil ||
		!strings.Contains(coverage.Text, "Frame-level causality is not yet proven") ||
		!strings.Contains(coverage.Text, "target-bound frame evidence is unavailable under the current evidence boundary") ||
		!strings.Contains(coverage.Text, "do not prove a specific frame-drop cause") {
		t.Fatalf("system-supplement authority did not reach EN production block: %+v", coverage)
	}
}

func traceAuthorityWiringContext(lang string) *types.BusContext {
	mutable := types.NewMutableState("trace authority wiring")
	return &types.BusContext{
		Language: lang,
		Mutable:  mutable,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentTrace,
				Scenario: types.ScenarioPerformanceBottleneck,
			},
			AnswerContract: types.AnswerContract{Language: lang},
		},
	}
}
