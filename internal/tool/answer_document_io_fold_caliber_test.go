package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMCIOFoldPublicNativeDurationIsNotRankingImpact(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := hmc081WriteTrace(t, string(body))
	bus, _, _ := hmc17NamedPathContext(t)
	var records []types.ObservationRecord
	var rankValue string
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.051})
		if !result.Success {
			t.Fatalf("public %s query failed: %s", view, result.Summary)
		}
		records = append(records, result.Observations...)
		for _, row := range result.Observations {
			if row.Predicate == "root_cause_background" && row.Subject == "backup-900" && row.Object == "io_latency" {
				rankValue = row.Value
			}
		}
	}
	// Native ordering remains capped inside the engine; the public copy now
	// carries a producer-identified measurement rather than that private cap.
	if rankValue != "47.000" {
		t.Fatalf("public background lane must retain the native measurement, got %q", rankValue)
	}
	ledger := types.ObservationLedger{Records: records}
	before, _ := json.Marshal(ledger)
	projection := types.CompileTraceCausalProjection(ledger)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		found, folded := false, false
		for _, row := range model.Background {
			if row.Node.Subject == "backup-900" {
				found = true
				if row.Node.ImpactMS != 47 {
					t.Errorf("background display lost the measured request: %.3f", row.Node.ImpactMS)
				}
				for _, peer := range row.IOFoldPeers {
					folded = folded || fmt.Sprintf("%.3f", peer.ImpactMS) == rankValue
					if peer.Caliber == runtimeTraceProjIOFoldRankImpact {
						t.Fatalf("calibrated native duration retained the legacy ranking label: %+v", peer)
					}
				}
			}
		}
		if !found {
			t.Fatal("public query set lost the background request")
		}
		// Equal original/rank measurements may now deduplicate before the IO
		// fold. Do not require a second visible instance of one physical fact.
		for name, rendered := range map[string]string{
			"tree":   runtimeTraceProjTreeFence(model, zh),
			"detail": runtimeTraceProjDetailFullText(model, zh),
		} {
			// Detail descriptions omit the primary duration unless they carry a
			// fold note; the tree always carries the primary measurement.
			if (name == "tree" || folded) && !strings.Contains(rendered, rankValue) {
				t.Fatalf("%s lost the measured background request: %s", name, rendered)
			}
			bad := "ranking impact·io_latency " + rankValue + "ms"
			want := "observed duration"
			if zh {
				bad = "排序影响·IO延迟（io_latency） " + rankValue + "ms"
				want = "观测计时"
			}
			if strings.Contains(rendered, bad) || (folded && !strings.Contains(rendered, want)) {
				t.Errorf("%s lost the producer's measurement caliber (zh=%t): %s", name, zh, rendered)
			}
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("display repair mutated source observations")
	}
}

func TestHMCIOFoldCarriesSelectedValueSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		node    types.TraceCausalProjectionNode
		value   float64
		caliber runtimeTraceProjIOFoldCaliber
	}{
		{"request", types.TraceCausalProjectionNode{Predicate: "io_latency", TypeToken: "io_latency", ImpactMS: 47, CumulativeImpactMS: 90}, 47, runtimeTraceProjIOFoldFamilyCaliber},
		{"rank", types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "io_latency", ImpactMS: 17.85, CumulativeImpactMS: 47}, 17.85, runtimeTraceProjIOFoldRankImpact},
		{"native_rank_duration", types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "io_latency", ImpactMS: 47, CumulativeImpactMS: 47, RankValueCaliber: types.TraceRankValueCaliberNativeDuration}, 47, runtimeTraceProjIOFoldNativeDuration},
		{"unknown_rank_caliber", types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "io_latency", ImpactMS: 17.85, RankValueCaliber: "guessed_duration"}, 17.85, runtimeTraceProjIOFoldRankImpact},
		{"cumulative", types.TraceCausalProjectionNode{Predicate: "root_cause_adjacent", TypeToken: "io_latency", CumulativeImpactMS: 47, EffectiveImpactMS: 31, ActualImpactMS: 12}, 47, runtimeTraceProjIOFoldCumulative},
		{"effective", types.TraceCausalProjectionNode{Predicate: "root_cause_primary", TypeToken: "io_wait", EffectiveImpactMS: 31, ActualImpactMS: 12}, 31, runtimeTraceProjIOFoldEffective},
		{"actual", types.TraceCausalProjectionNode{Predicate: "root_cause_primary", TypeToken: "io_wait", ActualImpactMS: 12}, 12, runtimeTraceProjIOFoldActual},
		{"composite", types.TraceCausalProjectionNode{Predicate: "root_cause_primary", TypeToken: "block_io_by_inode", ImpactMS: 9}, 9, runtimeTraceProjIOFoldFamilyCaliber},
		{"count", types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "page_cache_churn", ImpactMS: 7}, 7, runtimeTraceProjIOFoldFamilyCaliber},
		{"count_ignores_duration_marker", types.TraceCausalProjectionNode{Predicate: "root_cause_background", TypeToken: "page_cache_churn", ImpactMS: 7, RankValueCaliber: types.TraceRankValueCaliberNativeDuration}, 7, runtimeTraceProjIOFoldFamilyCaliber},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, zh := range []bool{true, false} {
				peer := runtimeTraceProjNewIOFoldPeer(tc.node, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				if peer.ImpactMS != tc.value || peer.Caliber != tc.caliber {
					t.Fatalf("value/source changed: %+v, want %v/%v", peer, tc.value, tc.caliber)
				}
				peer.EvidenceTag = "E9"
				note := runtimeTraceProjIOFoldNoteText([]runtimeTraceProjIOFoldPeer{peer}, zh)
				if !strings.Contains(note, "[E9]") || !strings.Contains(note, fmt.Sprintf("%.3f", tc.value)) {
					t.Fatalf("lost value/evidence: %s", note)
				}
				if tc.name == "composite" || strings.HasPrefix(tc.name, "count") {
					if strings.Contains(note, "ms") || !strings.Contains(note, map[bool]string{true: "非墙钟", false: "not wall clock"}[zh]) {
						t.Fatalf("non-duration family relabeled as duration: %s", note)
					}
				}
				if tc.caliber == runtimeTraceProjIOFoldRankImpact && !zh && strings.Contains(note, "also measured") {
					t.Fatalf("ranking impact is not another measurement: %s", note)
				}
				if tc.caliber == runtimeTraceProjIOFoldNativeDuration {
					want := map[bool]string{true: "观测计时", false: "observed duration"}[zh]
					if !strings.Contains(note, want) || strings.Contains(note, "非实测") || strings.Contains(note, "not measured") {
						t.Fatalf("producer-calibrated duration has contradictory wording: %s", note)
					}
				}
			}
		})
	}
}

func TestHMCIOFoldSameTokenDifferentRulersStayDistinct(t *testing.T) {
	for _, lane := range []string{"on_chain", "adjacent", "background"} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/zh=%t", lane, zh), func(t *testing.T) {
				main := revisit76IONode("physical", "worker-9", "io_latency", 80, 100, 200)
				measured := revisit76IONode("measured", "worker-9", "io_latency", 60, 105, 195)
				weighted := revisit76IONode("rank", "worker-9", "io_latency", 20, 110, 190)
				measured.Predicate = "io_latency"
				weighted.Predicate = "root_cause_background"
				for _, node := range []*types.TraceCausalProjectionNode{&main, &measured, &weighted} {
					node.ChainRelevance = lane
				}
				projection := types.TraceCausalProjection{WakeupPath: []string{"worker-9", "main-1"}}
				switch lane {
				case "on_chain":
					weighted.Predicate = "root_cause_primary"
					projection.OnChainCauses = []types.TraceCausalProjectionNode{main, measured, weighted}
				case "adjacent":
					weighted.Predicate = "root_cause_adjacent"
					projection.AdjacentCauses = []types.TraceCausalProjectionNode{main, measured, weighted}
				case "background":
					projection.BackgroundCauses = []types.TraceCausalProjectionNode{main, measured, weighted}
				}
				before, _ := json.Marshal(projection)
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				rows := append(append(append(append([]runtimeTraceProjTreeRow(nil), model.SelfRows...), model.TreeRows...), model.Adjacent...), model.Background...)
				found := false
				for _, row := range rows {
					if len(row.IOFoldPeers) < 2 {
						continue
					}
					found = true
					note := runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh)
					want := "ranking impact"
					if zh {
						want = "排序影响"
					}
					if !strings.Contains(note, want) || strings.Contains(note, "60.000/20.000ms") || !strings.Contains(note, "60.000ms") || !strings.Contains(note, "20.000ms") {
						t.Errorf("same type must not merge measurement and impact rulers: %s", note)
					}
				}
				if !found {
					t.Fatal("fixture did not exercise both folded peers")
				}
				after, _ := json.Marshal(projection)
				if string(before) != string(after) {
					t.Fatal("rendering changed projection authority")
				}
			})
		}
	}
}
