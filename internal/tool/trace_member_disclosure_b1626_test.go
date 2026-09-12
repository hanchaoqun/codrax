package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceSupplementMemberDisclosureB1626(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, reason := range []string{types.TraceSupplementReasonDurationBudgetExceeded, types.TraceSupplementReasonCanceledByCaller, types.TraceSupplementReasonExecutionFailed} {
			meta := &types.SystemTraceSupplementMeta{DurationBudgetS: 30, MemberWindows: []types.SystemTraceSupplementWindowMeta{
				{WindowStart: 2, WindowEnd: 2.02, Views: []string{"window_stats"}, ViewValueObservations: []int{1}},
				{WindowStart: 3, WindowEnd: 3.03, SkipReason: reason, SkippedViews: []string{"window_stats"}},
			}}
			text := runtimeTraceSupplementDisclosureText(meta, zh)
			for _, want := range []string{"2.000000..2.020000", "3.000000..3.030000"} {
				if !strings.Contains(text, want) {
					t.Errorf("zh=%v reason=%s lost %s: %s", zh, reason, want, text)
				}
			}
			if strings.Contains(text, "0.000000..0.000000") || strings.Contains(text, reason) {
				t.Fatalf("fabricated ruler/internal enum: %s", text)
			}
		}
		meta := &types.SystemTraceSupplementMeta{}
		for i := 0; i < 10; i++ {
			meta.MemberWindows = append(meta.MemberWindows, types.SystemTraceSupplementWindowMeta{WindowStart: float64(i + 1), WindowEnd: float64(i+1) + .02, SkipReason: types.TraceSupplementReasonDurationBudgetExceeded})
		}
		text := runtimeTraceSupplementDisclosureText(meta, zh)
		if !strings.Contains(text, "8.000000..8.020000") || strings.Contains(text, "9.000000..9.020000") {
			t.Fatalf("member display cap: %s", text)
		}
		want := "2 additional windows"
		if zh {
			want = "另有 2 个窗口"
		}
		if !strings.Contains(text, want) {
			t.Fatalf("missing explicit remainder %s: %s", want, text)
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState("request"), Language: "en"}
		if zh {
			ctx.Language = "zh"
		}
		ctx.Mutable.SetSystemTraceSupplement(*meta, nil)
		doc := &types.AnswerDocumentV2{Caveats: []string{"model caveat"}}
		if !materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx) || len(doc.Caveats) != 2 || doc.Caveats[0] != "model caveat" {
			t.Fatal(fmt.Sprintf("publication lost skipped-only members/model caveat: %+v", doc))
		}
	}
}

// Final display/persistence, not just helper text: model blocks and the typed
// supplement remain byte-identical while partial work has an honest tail.
func TestB1626MemberDisclosureActualPersistPartialDuplicateAndLite(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, shape := range []string{"partial_caller", "partial_execution", "duplicate", "lite_once"} {
			t.Run(lang+"/"+shape, func(t *testing.T) {
				first := types.SystemTraceSupplementWindowMeta{WindowStart: 2, WindowEnd: 2.02,
					Views: []string{"root_cause_rank"}, ViewValueObservations: []int{2},
					ViewObservationFamilies: []types.TraceSupplementViewFamilyCensus{{RootCauseRows: 2}}}
				second := types.SystemTraceSupplementWindowMeta{WindowStart: 3, WindowEnd: 3.03, SkipReason: types.TraceSupplementReasonFamiliesPresent}
				meta := types.SystemTraceSupplementMeta{TargetPID: 200, DurationBudgetS: 20,
					Views: first.Views, ViewValueObservations: first.ViewValueObservations, ViewObservationFamilies: first.ViewObservationFamilies}
				want := ""
				switch shape {
				case "partial_caller":
					first.SkipReason, first.SkippedViews = types.TraceSupplementReasonCanceledByCaller, []string{"critical_blocking_calls"}
					want = "remaining queries did not start after this run was canceled"
					if lang == "zh" {
						want = "本次运行取消后未启动剩余查询"
					}
				case "partial_execution":
					first.SkipReason, first.SkippedViews = types.TraceSupplementReasonExecutionFailed, []string{"critical_blocking_calls"}
					want = "other supplementary queries failed"
					if lang == "zh" {
						want = "另有补采查询失败"
					}
				case "duplicate":
					second = first
					want = "not another execution"
					if lang == "zh" {
						want = "非重复执行"
					}
				case "lite_once":
					meta.CensusLite, meta.CensusLitePattern = true, "vsync"
					want = "lightweight whole-trace VSync/frame-pacing generator census"
					if lang == "zh" {
						want = "全 trace 补跑 VSync/帧节拍发生器轻量普查"
					}
				}
				meta.MemberWindows = []types.SystemTraceSupplementWindowMeta{first, second}
				before, _ := json.Marshal(meta)
				ctx := &types.BusContext{Mutable: types.NewMutableState("member disclosure"), Language: lang}
				ctx.Mutable.SetSystemTraceSupplement(meta, nil)
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Caveats: []string{"model-owned caveat"},
					Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "模型正文不变。 Model statement unchanged."}}}
				wire, err := modelOwnedAnswerBlockWire(doc)
				if err != nil {
					t.Fatal(err)
				}
				for pass := 0; pass < 2; pass++ {
					result, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1, 0))
					if err != nil || !result.Success {
						t.Fatalf("actual persistence: %v %+v", err, result)
					}
					stored := ctx.Mutable.AnswerDocumentV2()
					if err := requireModelOwnedAnswerBlockWirePreserved(wire, stored); err != nil {
						t.Fatal(err)
					}
					text := render.RenderAnswerDocument(stored, lang)
					if strings.Count(text, want) != 1 {
						t.Fatalf("missing/repeated honest member disposition %q: %s", want, text)
					}
					if strings.Contains(text, "0.000000..0.000000") || strings.Contains(text, types.TraceSupplementReasonCanceledByCaller) || strings.Contains(text, types.TraceSupplementReasonExecutionFailed) {
						t.Fatalf("internal enum or fabricated window leaked: %s", text)
					}
					if len(stored.Caveats) != 2 || stored.Caveats[0] != "model-owned caveat" {
						t.Fatalf("system disclosure changed model caveat or duplicated itself: %+v", stored.Caveats)
					}
				}
				after, _ := json.Marshal(ctx.Mutable.SystemTraceSupplementMeta())
				if string(before) != string(after) {
					t.Fatal("display/persistence rewrote the measurement metadata")
				}
			})
		}
	}
}
