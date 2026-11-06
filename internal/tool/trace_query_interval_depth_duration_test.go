package tool

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC360PublicIntervalDepthDuration(t *testing.T) {
	// Exercise the preview that reaches the model, not the unbounded tool body.
	inline, head, tail := MaxInlineBytes, PreviewHeadBytesValue(), PreviewTailBytesValue()
	SetBlobLimits(2400, 2400, 300)
	t.Cleanup(func() { SetBlobLimits(inline, head, tail) })
	t.Run("closed_scheduler_intervals", func(t *testing.T) {
		result, stats, _ := schedulerConcurrencyToolQuery(t, schedulerConcurrencyPublicTrace, nil)
		for _, g := range stats.Groups {
			peak, peakMS, zeroMS, percent := 1, 2.0, 8.0, "20"
			if g.State == "running" {
				peak, peakMS, zeroMS, percent = 2, 4, 2, "40"
			}
			checkPublicIntervalDepthDuration(t, result, g, "scheduler_concurrency", g.State, peak, peakMS, zeroMS, percent)
		}
	})
	t.Run("paired_request_residence", func(t *testing.T) {
		const body = "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
			"io-40 (40) [003] .... 1.003000: block_rq_issue: 8,0 R 4096 () 16 + 8 [io]\n" +
			"irq-2 (2) [003] .... 1.005000: block_rq_complete: 8,0 R () 16 + 8 [0]\n" +
			"irq-2 (2) [003] .... 1.007000: block_rq_complete: 8,0 R () 8 + 8 [0]\n"
		result, stats, _, _ := ioInFlightPrecisionPublicQuery(t, body, 1, 1.01, 0, 0)
		checkPublicIntervalDepthDuration(t, result, stats.Groups[0], "io_inflight", "block", 2, 2, 4, "20")
	})
}

func checkPublicIntervalDepthDuration(t *testing.T, result types.ToolResult, group any, predicate, subject string, peak int, peakMS, zeroMS float64, percent string) {
	t.Helper()
	want := "峰值持续时间"
	if !strings.Contains(result.Summary, want) {
		t.Errorf("first tool preview did not supply %q", want)
	}
	body, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	// Use the public JSON shape so the failure is observable on the old code.
	var native struct {
		Depth *struct {
			WindowMS           float64 `json:"window_ms"`
			ZeroContributionMS float64 `json:"zero_contribution_ms"`
			PeakDepth          int     `json:"peak_depth"`
			PeakDurationMS     float64 `json:"peak_duration_ms"`
		} `json:"depth_duration"`
	}
	if err := json.Unmarshal(body, &native); err != nil {
		t.Fatal(err)
	}
	if d := native.Depth; d == nil || d.PeakDepth != peak || math.Abs(d.PeakDurationMS-peakMS) > 1e-8 ||
		math.Abs(d.ZeroContributionMS-zeroMS) > 1e-8 || math.Abs(d.WindowMS-10) > 1e-8 {
		t.Errorf("native depth accounting is missing or used the wrong population: %+v", native.Depth)
	}
	for _, row := range result.Observations {
		if row.Predicate != predicate || row.Subject != subject {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(row)
		if !ok || p.Source.PayloadRef == "" || row.Role != types.AnswerAggregateRoleSupportingCoverage ||
			!p.Source.QueryWindowKnown || p.Source.QueryWindowStartTs != 1 || p.Source.QueryWindowEndTs != 1.01 {
			t.Fatalf("the existing source/window-scoped noncausal receipt changed: %+v", row)
		}
		compact := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, types.SemanticReviewObservationPromptProjectionOptions(1))
		for _, table := range p.Tables {
			if table.View != types.RuntimeMeasurementSummary {
				continue
			}
			for surface, value := range map[string]string{"tool": result.Summary, "handoff": compact[0].Summary, "table": strings.Join(table.Notes, " ")} {
				if !strings.Contains(value, want) || !strings.Contains(value, "全窗 "+percent+"%") || !strings.Contains(value, "零贡献") {
					t.Errorf("%s omitted independent peak duration or its full-window basis", surface)
				}
			}
			return
		}
		t.Fatal("source-bound summary table missing")
	}
	t.Fatal("source-bound observation missing")
}
