package tracequery

// rank_item_wire_m18_test.go — RANKDIS-M18 (§29.104.16.1 M18, user ruling
// §29.104.17 ② 2026-07-16) wire pins: composite-score rank rows publish their
// value slots under the *_score JSON keys, never the ms-semantic keys; every
// other row and every physical wall-clock slot is byte-identical to the
// pre-M18 shape. The state-drilldown ranking-only composite weight left its
// ms-suit key the same way (rank_impact_ms → rank_impact_score).
//
// MUTATION self-check: deleting the RootCauseRankItem.MarshalJSON fork (or
// removing io_pressure/block_io_by_inode from
// causalTokenCompositeValueWireRows) reds the composite arms; widening the
// wire set reds the wall-clock byte-identity arm; reverting the
// StateDrilldownStep tag reds the drilldown pin.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCompositeScoreRowsAreCompositeValueWireRows pins the registry subset
// relation: every ⌗ caliber-side composite token is also a composite-value
// WIRE token (the converse is deliberately false — io_pressure re-keys its
// wire without the un-ruled ordinal/tier demotion; 排序零动).
func TestCompositeScoreRowsAreCompositeValueWireRows(t *testing.T) {
	for token := range causalTokenCompositeScoreRows {
		if !causalTokenCompositeValueWireRows[token] {
			t.Fatalf("composite-score token %q must be a composite-value wire token (M18 subset pin)", token)
		}
	}
	if !CausalTokenCompositeValueWire("io_pressure") {
		t.Fatalf("io_pressure is an M18 composite-value wire token")
	}
	if CausalTokenCaliberSideClass("io_pressure") != CausalCaliberSideNone {
		t.Fatalf("io_pressure must NOT join the caliber-side class arm (seat/tier lanes are 排序零动 in M18); a demotion needs its own ruling")
	}
}

func TestRankItemCompositeWireKeyFork(t *testing.T) {
	composite := RootCauseRankItem{
		Type:               "io_pressure",
		ImpactMs:           61.540,
		ProjectedImpactMs:  61.540,
		CumulativeImpactMs: 61.540,
		EffectiveImpactMs:  61.540,
		TargetImpactMs:     1.500,
		ActualImpactMs:     2.500,
		Score:              43.078,
		Confidence:         0.70,
		Source:             "window_stats.io_pressure_summary",
		Summary:            "io pressure signal=io_activity score=61.540",
	}
	payload, err := json.Marshal(composite)
	if err != nil {
		t.Fatalf("marshal composite: %v", err)
	}
	text := string(payload)
	for _, want := range []string{
		`"activity_index":61.54`,
		// Physical wall-clock ledgers keep the ms suit on every row shape
		// (QH2-A 口径分离).
		`"target_impact_ms":1.5`,
		`"actual_impact_ms":2.5`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("composite wire must carry %s, got:\n%s", want, text)
		}
	}
	for _, deny := range []string{`"impact_ms"`, `"projected_impact_ms"`, `"cumulative_impact_ms"`, `"effective_impact_ms"`, `"impact_score"`, `"projected_impact_score"`, `"cumulative_impact_score"`, `"effective_impact_score"`, `"score"`} {
		if strings.Contains(text, deny) {
			t.Fatalf("composite wire must not publish the ms-semantic key %s:\n%s", deny, text)
		}
	}

	// block_io_by_inode rides the same wire fork.
	composite.Type = "block_io_by_inode"
	payload, err = json.Marshal(composite)
	if err != nil {
		t.Fatalf("marshal block_io composite: %v", err)
	}
	if !strings.Contains(string(payload), `"effective_impact_score":61.54`) || strings.Contains(string(payload), `"effective_impact_ms"`) {
		t.Fatalf("block_io_by_inode must fork the wire keys too:\n%s", payload)
	}

	// Zero slots are omitted on the composite shape exactly like the legacy
	// omitempty behavior.
	zero := RootCauseRankItem{Type: "io_pressure", CumulativeImpactMs: 3.000}
	payload, err = json.Marshal(zero)
	if err != nil {
		t.Fatalf("marshal zero-slot composite: %v", err)
	}
	if strings.Contains(string(payload), "impact_score\":0") || strings.Contains(string(payload), `"effective_impact_score"`) {
		t.Fatalf("zero composite slots must be omitted:\n%s", payload)
	}
	if !strings.Contains(string(payload), `"activity_index":3`) {
		t.Fatalf("positive IO activity index must publish:\n%s", payload)
	}
}

// TestRankItemWallClockWireBytesUntouched pins the non-composite shape to the
// exact pre-M18 alias bytes (field set, order, keys): the MarshalJSON fork
// must be invisible off the wire set.
func TestRankItemWallClockWireBytesUntouched(t *testing.T) {
	wall := RootCauseRankItem{
		Type:               "long_sleep",
		ImpactMs:           4.000,
		ProjectedImpactMs:  4.000,
		CumulativeImpactMs: 4.000,
		EffectiveImpactMs:  4.000,
		Score:              3.200,
		Confidence:         0.80,
	}
	got, err := json.Marshal(wall)
	if err != nil {
		t.Fatalf("marshal wall row: %v", err)
	}
	want, err := json.Marshal(rootCauseRankItemWireAlias(wall))
	if err != nil {
		t.Fatalf("marshal alias: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("non-composite rows must marshal byte-identically to the legacy shape:\n got %s\nwant %s", got, want)
	}
	for _, wantKey := range []string{`"impact_ms":4`, `"cumulative_impact_ms":4`, `"effective_impact_ms":4`, `"projected_impact_ms":4`} {
		if !strings.Contains(string(got), wantKey) {
			t.Fatalf("wall-clock wire must keep %s:\n%s", wantKey, got)
		}
	}
	if strings.Contains(string(got), "_score\"") && strings.Contains(string(got), "impact") {
		t.Fatalf("wall-clock rows must not mint composite keys:\n%s", got)
	}
}

// TestStateDrilldownStepWireKeyRankImpactScore pins the drilldown composite
// weight's wire key (§7.30 S1 witness family): the ranking-only composite
// left the ms-semantic slot; ImpactMs/TotalMs stay physical ms keys.
func TestStateDrilldownStepWireKeyRankImpactScore(t *testing.T) {
	step := StateDrilldownStep{
		Rank:         1,
		State:        string(StateRunnable),
		ImpactMs:     9.000,
		TotalMs:      20.000,
		RankImpactMs: 13.500,
		Source:       "state_churn",
	}
	payload, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal drilldown step: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, `"rank_impact_score":13.5`) {
		t.Fatalf("the composite weight must publish as rank_impact_score:\n%s", text)
	}
	if strings.Contains(text, "rank_impact_ms") {
		t.Fatalf("the retired rank_impact_ms key must not appear:\n%s", text)
	}
	if !strings.Contains(text, `"impact_ms":9`) || !strings.Contains(text, `"total_ms":20`) {
		t.Fatalf("physical drilldown slots keep their ms keys:\n%s", text)
	}
}
