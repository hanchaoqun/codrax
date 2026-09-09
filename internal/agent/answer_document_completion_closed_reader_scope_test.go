package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1630CompletionReaderContext(truncated bool, lang string) *types.AgentContext {
	ctx := boundedRuntimeReaderHandoffTestContext()
	ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
	a := ctx.Mutable.TurnAArtifacts()
	state := a.ToolResults[0].Observations[0]
	for i, bounds := range [][2]float64{
		{13762.796752, 13762.797534}, {13762.831788, 13762.832815},
		{13762.862763, 13762.864001}, {13762.872586, 13762.873923},
	} {
		r := types.ObservationRecord{
			ID: fmt.Sprintf("closed-io-%d", i), Origin: state.Origin,
			Producer: state.Producer, GroundingPolicy: state.GroundingPolicy, SourceRef: state.SourceRef,
			Subject: state.Subject, Predicate: "io_latency", Unit: "ms",
			Value: fmt.Sprintf("%.3f", (bounds[1]-bounds[0])*1000),
			Span:  types.ObservationSpan{StartTs: bounds[0], EndTs: bounds[1]},
			RichNotes: []string{
				"selected_window=13762.791708..13763.024898",
				"completion_woke_issuer=true", "causal_wait_caliber=completion_closed_issuer_blocked",
				"issuer_blocked_state=s_sleep", fmt.Sprintf("issuer_blocked=%.3f", (bounds[1]-bounds[0])*1000),
				fmt.Sprintf("issuer_blocked_start=%.6f", bounds[0]), fmt.Sprintf("issuer_blocked_end=%.6f", bounds[1]),
				fmt.Sprintf("capacity_truncated=%t", truncated),
			},
		}
		a.ToolResults[0].Observations = append(a.ToolResults[0].Observations, r)
	}
	ctx.Mutable.SetTurnAArtifacts(*a)
	return ctx
}

func TestB1630CompletionReaderActualFinalContextKeepsBlockingCoverage(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, truncated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/truncated=%t", lang, truncated), func(t *testing.T) {
				ctx := b1630CompletionReaderContext(truncated, lang)
				before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				ledger := answerDocObservationLedger(ctx)
				authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
				if len(authorities) != 1 || len(authorities[0].Occurrences) != 4 || fmt.Sprintf("%.3f", authorities[0].ObservedMS) != "4.384" {
					t.Fatalf("fixture must supply the original 4 measured waits / 4.384ms: %+v", authorities)
				}
				wantStatus := "complete"
				if truncated {
					wantStatus = "lower_bound_capacity_truncated"
				}
				if authorities[0].CoverageStatus != wantStatus {
					t.Fatalf("wrong source coverage premise: %+v", authorities[0])
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				prefix := "  - Separate completion-closed IO ruler ("
				if lang == "zh" {
					prefix = "  - 独立 IO 完成闭合口径（"
				}
				var line string
				for _, candidate := range strings.Split(prompt, "\n") {
					if strings.HasPrefix(candidate, prefix) {
						line = candidate
						break
					}
				}
				wants := []string{"complete coverage", "4 proved", "4.384 ms", "must neither be added"}
				if lang == "zh" {
					wants = []string{"覆盖完整", "等待 4 次", "4.384 毫秒", "不能相加"}
				}
				if truncated && lang == "zh" {
					wants = []string{"查询结果容量受限", "至少 4 次", "至少 4.384 毫秒", "全窗总次数和总量未知", "不代表 Trace 采集缺失", "不能相加"}
				} else if truncated {
					wants = []string{"query-result capacity", "at least 4", "at least 4.384 ms", "full-window totals are unknown", "does not imply missing trace capture", "must neither be added"}
				}
				for _, want := range wants {
					if !strings.Contains(line, want) {
						t.Errorf("real final-context IO line lost %q: %s", want, line)
					}
				}
				if strings.Contains(line, "窗口覆盖范围未知") || strings.Contains(line, "window coverage unknown") || strings.Contains(line, "lower_bound_capacity_truncated") {
					t.Errorf("blocking coverage was mistranslated or leaked: %s", line)
				}
				after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				if string(before) != string(after) {
					t.Fatal("prompt rendering changed source records, values, or model handoff")
				}
			})
		}
	}
}

func TestB1630CompletionReaderUnknownCoverageDoesNotBorrowStateOrCapacityStatus(t *testing.T) {
	for _, status := range []string{"", "unknown", "partial_unaccounted", "not_assessed", "future_blocking_status"} {
		for _, zh := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%s/zh=%t", status, zh), func(t *testing.T) {
				a := types.TraceBlockingWallClockAuthority{
					CoverageStatus: status, ObservedMS: 4.384,
					Occurrences: make([]types.TraceBlockingWallClockOccurrence, 4),
				}
				before, _ := json.Marshal(a)
				got := formatTraceCompletionClosedReaderFact(a, zh)
				wants := []string{"coverage completeness unconfirmed", "observed 4", "observed 4.384", "Full-window totals are not established", "must neither be added"}
				if zh {
					wants = []string{"覆盖完整性未确认", "已观测 4 次", "已观测 4.384", "全窗总次数和总量尚未确定", "不能相加"}
				}
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("unknown blocking coverage lost %q: %s", want, got)
					}
				}
				for _, forbidden := range []string{"（覆盖完整）", "(complete coverage)", "capacity", "容量", "capture", "采集", "at least", "至少"} {
					if strings.Contains(got, forbidden) {
						t.Errorf("unknown status borrowed a known coverage cause %q: %s", forbidden, got)
					}
				}
				after, _ := json.Marshal(a)
				if string(before) != string(after) || got != formatTraceCompletionClosedReaderFact(a, zh) {
					t.Fatal("display changed typed input or was not idempotent")
				}
			})
		}
	}
}

func TestB1630CompletionReaderCompleteSingleOutputRemainsByteEqual(t *testing.T) {
	a := types.TraceBlockingWallClockAuthority{CoverageStatus: "complete", ObservedMS: 1.25, Occurrences: make([]types.TraceBlockingWallClockOccurrence, 2)}
	for _, tc := range []struct {
		zh   bool
		want string
	}{
		{true, "  - 独立 IO 完成闭合口径（覆盖完整）：已证目标线程等待 2 次，区间并集 1.250 毫秒。它与调度器标记的 D/IO 等待是两把尺，不能相加或互相否定。\n"},
		{false, "  - Separate completion-closed IO ruler (complete coverage): 2 proved target-thread wait occurrence(s), interval union 1.250 ms. This and scheduler-marked D/IO wait are different rulers and must neither be added nor used to negate one another.\n"},
	} {
		if got := formatTraceCompletionClosedReaderFact(a, tc.zh); got != tc.want {
			t.Fatalf("complete account changed: %q, want %q", got, tc.want)
		}
	}
}
