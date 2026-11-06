package tracequery

import (
	"math"
	"strings"
	"testing"
)

func TestHMC356IOActivityDurationOperandsAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   float64
		want string
	}{
		{"unaccounted", 0, "not_accounted"},
		{"invalid", -1, "not_accounted"},
		{"nan", math.NaN(), "not_accounted"},
		{"infinite", math.Inf(1), "not_accounted"},
		{"tiny positive", 0.0001, "0.0001ms"},
		{"ordinary positive", 3.25, "3.250ms"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IOActivityDurationText(tc.ms, "not_accounted"); got != tc.want {
				t.Fatalf("input %g published as %q, want %q", tc.ms, got, tc.want)
			}
		})
	}
	pressure := computeIOPressureSummary(WindowStats{
		IOWaitBlockedCount: 2,
		DStateTop:          []ThreadDuration{{DurationMs: 3.25}},
		IOWaitTop:          []ThreadDuration{{DurationMs: 1.5}},
		IOLatencies:        []IOLatencySummary{{DurationMs: 7.5}},
	})
	if pressure == nil || pressure.Score != 22.25 || pressure.DStateMs != 3.25 || pressure.IOWaitMs != 1.5 {
		t.Fatalf("positive state and request operands or score changed: %+v", pressure)
	}
	for _, want := range []string{"block_max=7.500ms", "storage_max=not_accounted", "iowait_blocked=2", "d_state=3.250ms", "io_wait=1.500ms"} {
		if !strings.Contains(pressure.Summary, want) {
			t.Errorf("independent operands lost %q: %s", want, pressure.Summary)
		}
	}
}

func TestIOPressureCountOnlyMarkersStayActivityOnly(t *testing.T) {
	stats := WindowStats{IOWaitBlockedCount: 36}
	pressure := computeIOPressureSummary(stats)
	if pressure == nil {
		t.Fatal("count-only iowait markers must remain visible as activity context")
	}
	if pressure.Signal != "blocked_reason_iowait_count_only" ||
		pressure.EvidenceQuality != IOPressureEvidenceQualityActivityMarkerOnly ||
		pressure.ScoreCaliber != IOPressureScoreCaliberCountWeightedActivityIndex ||
		pressure.Score != 180 {
		t.Fatalf("count-only IO caliber drifted: %+v", pressure)
	}
	if pressure.DStateMs != 0 || pressure.IOWaitMs != 0 ||
		pressure.BlockMaxLatencyMs != 0 || pressure.StorageMaxLatencyMs != 0 {
		t.Fatalf("count-only fixture accidentally gained wall-clock/latency proof: %+v", pressure)
	}
	for _, want := range []string{
		"io activity signal=blocked_reason_iowait_count_only",
		"activity_index=180.000",
		"evidence_quality=activity_marker_only",
		"score_caliber=count_weighted_activity_index",
		"d_state=not_accounted",
		"io_wait=not_accounted",
		"An unaccounted duration is not a measured zero",
		"score_breakdown=iowait_blocked_count(36)*5=180.000",
		"comparison_scope=same_score_caliber_capture_conditions_and_window_duration",
		"absolute_level=not_defined",
	} {
		if !strings.Contains(pressure.Summary, want) {
			t.Fatalf("pressure summary missing %q: %s", want, pressure.Summary)
		}
	}
	if strings.Contains(strings.ToLower(pressure.Summary), "high io pressure") {
		t.Fatalf("count-only activity must not claim high IO pressure: %s", pressure.Summary)
	}
}

func TestIOPressureCustomerCountOnlyScoreExplains4340WithoutLevelClaim(t *testing.T) {
	pressure := computeIOPressureSummary(WindowStats{IOWaitBlockedCount: 868})
	if pressure == nil || pressure.Score != 4340 {
		t.Fatalf("868 count-only markers must produce the observed 4340 activity index: %+v", pressure)
	}
	for _, want := range []string{
		"score_breakdown=iowait_blocked_count(868)*5=4340.000",
		"comparison_scope=same_score_caliber_capture_conditions_and_window_duration",
		"absolute_level=not_defined",
	} {
		if !strings.Contains(pressure.Summary, want) {
			t.Fatalf("customer score explanation missing %q: %s", want, pressure.Summary)
		}
	}
}

func TestIOPressureWallClockOrLatencyCorroborationUsesPositiveEvidenceLane(t *testing.T) {
	pressure := computeIOPressureSummary(WindowStats{
		IOWaitBlockedCount: 2,
		IOLatencies: []IOLatencySummary{{
			DurationMs: 7.5,
		}},
	})
	if pressure == nil ||
		pressure.Signal != "scheduler_iowait_with_storage_latency" ||
		pressure.EvidenceQuality != IOPressureEvidenceQualityWallClockOrLatencyCorroborated ||
		pressure.ScoreCaliber != IOPressureScoreCaliberCrossUnitActivityIndex ||
		pressure.Score != 17.5 {
		t.Fatalf("latency-corroborated IO pressure lane drifted: %+v", pressure)
	}
	for _, forbidden := range []string{"score_breakdown=", "absolute_level=not_defined"} {
		if strings.Contains(pressure.Summary, forbidden) {
			t.Fatalf("count-only explanation leaked into corroborated lane (%q): %s", forbidden, pressure.Summary)
		}
	}
}

func TestIOPressureRankContextCarriesExactCaliberAndConstituents(t *testing.T) {
	pressure := computeIOPressureSummary(WindowStats{IOWaitBlockedCount: 36})
	rank := buildRootCauseRankFrom(nil, Query{}, ChainResult{}, WindowStats{
		IOPressureSummary: pressure,
	})
	var got *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "io_pressure" {
			got = &rank.Items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("aggregate IO context row missing: %+v", rank.Items)
	}
	if got.Rank != 0 || got.Tier != RootCauseTierContextOnly ||
		got.Causality != "background" || got.ChainRelevance != "background" ||
		got.IOPressureSignal != "blocked_reason_iowait_count_only" ||
		got.IOPressureEvidenceQuality != IOPressureEvidenceQualityActivityMarkerOnly ||
		got.IOPressureScoreCaliber != IOPressureScoreCaliberCountWeightedActivityIndex ||
		got.IOPressureIOWaitBlockedCount != 36 ||
		got.DStateMs != 0 || got.IOWaitMs != 0 {
		t.Fatalf("rank context lost IO caliber/constituents: %+v", *got)
	}
}
