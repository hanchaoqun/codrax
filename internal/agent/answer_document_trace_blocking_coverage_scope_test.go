package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1615BlockingCapacityRealProducerPromptScope(t *testing.T) {
	data, err := os.ReadFile("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trace.ftrace"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		start, end float64
		limit      int
		capacity   bool
	}{
		{"capacity_limited_result", 13762.791708, 13763.024898, 20, true},
		{"complete_result", 13762.8355, 13762.8375, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := json.Marshal(map[string]any{
				"source": "path", "path": "trace.ftrace", "view": "critical_blocking_calls",
				"pid": 17267, "thread": ".ugc.aweme.lite-17267",
				"time_start": tc.start, "time_end": tc.end, "max_depth": 4, "limit": tc.limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("real trace_query failed: %v; %s", err, result.Summary)
			}
			for _, lang := range []string{"zh", "en"} {
				t.Run(lang, func(t *testing.T) {
					ctx := tracePrincipalValueAuthorityTestContext(".ugc.aweme.lite-17267", 17267, result.Observations)
					ctx.Language = lang
					ctx.AnalysisIR.RequestModel.Language = lang
					ledger := answerDocObservationLedger(ctx)
					authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
					var binder *types.TraceBlockingWallClockAuthority
					for i := range authorities {
						if authorities[i].Type == "binder_wait" {
							binder = &authorities[i]
						}
					}
					wantStatus := "complete"
					if tc.capacity {
						wantStatus = "lower_bound_capacity_truncated"
					}
					if binder == nil || binder.CoverageStatus != wantStatus || len(binder.Occurrences) != 1 ||
						binder.Occurrences[0].StartTs != 13762.835861 || binder.Occurrences[0].EndTs != 13762.837270 {
						t.Fatalf("fixture must exercise actual typed capacity status and exact wait: %+v", binder)
					}
					model := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
						ID: "summary", Kind: types.BlockSummary,
						Text: "Model-owned conclusion; capture loss is not a prompt rewrite target.",
					}}}
					ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, model)
					before, _ := json.Marshal(result)
					projection := types.CompileTraceCausalProjectionSet(ledger)
					wantScope := "query-returned rows or chain-traversal result limits"
					wantBoundary := "not evidence of trace capture loss or capture completeness"
					if lang == "zh" {
						wantScope = "查询返回条目或链遍历结果的容量裁剪"
						wantBoundary = "不代表 Trace 采集缺失，也不据此证明采集完整"
					}
					for name, render := range map[string]func() string{
						"observation_handoff": func() string { return renderAnswerDocObservationLedger(ctx) },
						"principal_recap":     func() string { return renderAnswerDocTracePrincipalValueAuthority(ctx) },
						"finalizer_entry":     func() string { return (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil) },
					} {
						got := render()
						for _, want := range []string{wantScope, wantBoundary} {
							if strings.Contains(got, want) != tc.capacity {
								t.Errorf("%s capacity=%t must describe only typed result scope via %q", name, tc.capacity, want)
							}
						}
						if !strings.Contains(got, "coverage_status=`"+wantStatus+"`") {
							t.Errorf("%s lost original typed wire status %q", name, wantStatus)
						}
						if got != render() {
							t.Errorf("%s prompt rendering must be idempotent", name)
						}
					}
					after, _ := json.Marshal(result)
					if string(before) != string(after) || !reflect.DeepEqual(model, ctx.Mutable.AnswerDocumentV2()) ||
						!reflect.DeepEqual(ledger, answerDocObservationLedger(ctx)) ||
						!reflect.DeepEqual(projection, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))) ||
						!reflect.DeepEqual(authorities, types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)) {
						t.Fatal("prompt-only scope disclosure mutated source, model answer, ledger, projection or authority")
					}
				})
			}
		})
	}
}

func TestB1615BlockingCapacityScopeDoesNotRenameOtherCoverageReasons(t *testing.T) {
	for _, lang := range []bool{false, true} {
		for _, status := range []string{
			"", "complete", "unavailable", "unknown", "missing_wakeup", "missing_closure",
			"capacity_truncated", "lower_bound", "lower_bound_other_reason",
			"LOWER_BOUND_CAPACITY_TRUNCATED", " lower_bound_capacity_truncated",
		} {
			if got := traceBlockingCapacityScopeNote(status, lang); got != "" {
				t.Errorf("status %q must not acquire a capacity cause explanation: %s", status, got)
			}
		}
	}
}
