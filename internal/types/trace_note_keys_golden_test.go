package types

// trace_note_keys_golden_test.go — NKR golden snapshot of the FULL rich-note
// key registry (key|family|carrier). The diff of this file is the review
// surface for ANY registry change, exactly like the causal-token registry
// golden. If this test fails you either (a) added/renamed/removed a key —
// update the golden IN THE SAME COMMIT and walk the trace_note_keys.go
// change protocol, or (b) typo'd a registry row — fix the row, not the
// golden.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var traceNoteKeyGoldenRows = []string{
	"actual_d_state|state|soft_consumer",
	"actual_impact|impact|hard_consumer",
	"actual_impact_ms|impact|hard_consumer",
	"actual_io_wait|state|soft_consumer",
	"actual_runnable|state|soft_consumer",
	"actual_running|state|soft_consumer",
	"actual_sleep|state|soft_consumer",
	"actual_total|impact|soft_consumer",
	"actual_total_ms|impact|soft_consumer",
	"actual_window|anchor_window|soft_consumer",
	"adds|io|display_only",
	"advisory_pretriage|ledger_marker|soft_consumer",
	"allowed_core_classes|cpu_load|display_only",
	"allowed_cpus|cpu_load|display_only",
	"also_starved|occupancy|display_only",
	"avg_latency|io|display_only",
	"block_dev|io|display_only",
	"block_max|io|display_only",
	"blocking_candidate|blocking|display_only",
	"blocking_kind|blocking|hard_consumer",
	"bytes|io|display_only",
	"callstack|io|display_only",
	"candidate_count|causal_rank|display_only",
	"capacity_truncated|causal_rank|hard_consumer",
	"category|plugin|display_only",
	"causality|causal_rank|hard_consumer",
	"chain_depth|causal_rank|hard_consumer",
	"chain_relevance|causal_rank|hard_consumer",
	"chain_required|causal_rank|hard_consumer",
	"churn|io|display_only",
	"clock_set_rate|supply_pressure|display_only",
	"completions|io|display_only",
	"constraint|cpu_load|display_only",
	"context|dma_fence|display_only",
	"core_class|cpu_load|display_only",
	"core_classes|cpu_load|display_only",
	"core_limited_cpu_ms|compute_supply|display_only",
	"count|io|display_only",
	"coverage_mode|causal_rank|display_only",
	"cpu|cpu_load|display_only",
	"cpu_count|compute_supply|display_only",
	"cpus|cpu_load|display_only",
	"cpuset|cpu_load|display_only",
	"cumulative_impact_ms|impact|hard_consumer",
	"d_state|state|soft_consumer",
	"ddr|supply_pressure|display_only",
	"delay|sched_accounting|display_only",
	"deletes|io|display_only",
	"delivered_cpu_ms|compute_supply|display_only",
	"depth|causal_rank|hard_consumer",
	"detected_period_ms|periodic|hard_consumer",
	"deterministic_runtime_query_present|ledger_marker|soft_consumer",
	"dev|io|display_only",
	"domain|plugin|display_only",
	"dominant_state|state|hard_consumer",
	"driver|dma_fence|display_only",
	"dso|perf|soft_consumer",
	"duration|impact|display_only",
	"edge_count|causal_rank|display_only",
	"edges|chain_path|display_only",
	"effective_impact|impact|hard_consumer",
	"effective_impact_ms|impact|hard_consumer",
	"event|io|display_only",
	"example|io|display_only",
	"file_bytes|io|display_only",
	"file_events|io|display_only",
	"flags|blocking|display_only",
	"fold_basis|supply_fold|hard_consumer",
	"fold_cluster_lane_caveat|supply_fold|display_only",
	"fold_fmax|supply_fold|display_only",
	"fold_fmax_finding|supply_fold|display_only",
	"fragments|state|soft_consumer",
	"freq|cpu_load|display_only",
	"function|workqueue|display_only",
	"gated_runnable|gating|hard_consumer",
	"gated_running_deficit|gating|hard_consumer",
	"high_prio|cpu_load|display_only",
	"high_prio_overlap|cpu_load|display_only",
	"high_prio_running|cpu_load|display_only",
	"holder_site|blocking|hard_consumer",
	"idle_mismatch_ms|compute_supply|soft_consumer",
	"impact|impact|hard_consumer",
	"impact_ms|impact|hard_consumer",
	"inode|io|display_only",
	"io_wait|state|soft_consumer",
	"iowait_blocked|io|display_only",
	"kind|cpu_load|display_only",
	"l3|supply_pressure|display_only",
	"latency|chain_path|display_only",
	"lateness_ms|periodic|hard_consumer",
	"layer|io|display_only",
	"legacy_summary_fallback|ledger_marker|display_only",
	"line|io|display_only",
	"low_freq_cpus|supply_pressure|display_only",
	"low_freq_loss_cpu_ms|compute_supply|display_only",
	"max|interrupt|display_only",
	"max_delay|sched_accounting|display_only",
	"max_latency|io|display_only",
	"max_runtime|sched_accounting|display_only",
	"max_segment|state|soft_consumer",
	"metric|plugin|display_only",
	"migrations|cpu_load|display_only",
	"name|io|display_only",
	"nearest_block_thread|io|display_only",
	"nearest_chain_thread|causal_rank|display_only",
	"nearest_chain_window|anchor_window|soft_consumer",
	"next_step|guidance|soft_consumer",
	"next_step_kind|guidance|soft_consumer",
	"nodes|chain_path|display_only",
	"not_answer_grade|ledger_marker|display_only",
	"observed_core_class|cpu_load|display_only",
	"observed_cpu|cpu_load|display_only",
	"occupier_1|occupancy|hard_consumer",
	"occupier_2|occupancy|hard_consumer",
	"occupier_3|occupancy|hard_consumer",
	"occurrence_windows|anchor_window|soft_consumer",
	"occurrences|causal_rank|display_only",
	"offsets|io|display_only",
	"oneway|blocking|display_only",
	"op|io|display_only",
	"other_cpu_idle|cpu_load|display_only",
	"overlap|causal_rank|display_only",
	"p95_segment|state|soft_consumer",
	"page_cache_churn|io|display_only",
	"paired|io|display_only",
	"path|chain_path|soft_consumer",
	"peer|blocking|hard_consumer",
	"peer_state_d_state|blocking|display_only",
	"peer_state_dominant|blocking|display_only",
	"peer_state_fragments|blocking|display_only",
	"peer_state_io_wait|blocking|display_only",
	"peer_state_runnable|blocking|display_only",
	"peer_state_running|blocking|display_only",
	"peer_state_sleep|blocking|display_only",
	"peer_state_total|blocking|display_only",
	"percent|perf|display_only",
	"perf_context|perf|display_only",
	"perf_contexts|perf|display_only",
	"perf_quality|perf|soft_consumer",
	"perf_quality_caveats|perf|soft_consumer",
	"periodic_source|periodic|hard_consumer",
	"policy|cpu_load|display_only",
	"pressure_density|occupancy|display_only",
	"prio|cpu_load|display_only",
	"priority|cpu_load|display_only",
	"priority_inversion_candidate|gating|display_only",
	"priority_inversion_edges|gating|display_only",
	"priority_inversion_gated|gating|display_only",
	"priority_relation|gating|display_only",
	"process|cpu_load|display_only",
	"projected_impact|impact|display_only",
	"projected_impact_ms|impact|display_only",
	"projected_total|impact|display_only",
	"projected_total_ms|impact|display_only",
	"rank|causal_rank|hard_consumer",
	"rank_impact|impact|display_only",
	"recommended_sections|causal_rank|display_only",
	"recommended_views|causal_rank|soft_consumer",
	"recursive|causal_rank|soft_consumer",
	"ret|io|display_only",
	"runnable|state|hard_consumer",
	"runnable_cpu|guidance|soft_consumer",
	"running|state|soft_consumer",
	"runtime|sched_accounting|display_only",
	"same_cpu_busy|cpu_load|display_only",
	"same_cpu_idle|cpu_load|display_only",
	"sample_weight|perf|display_only",
	"samples|perf|display_only",
	"score|causal_rank|display_only",
	"selected_frame_id|causal_rank|display_only",
	"selected_name|causal_rank|display_only",
	"selected_phase|causal_rank|display_only",
	"selected_role|causal_rank|display_only",
	"selected_window|anchor_window|anchor_window",
	"semantic_class|span|hard_consumer",
	"seqno|dma_fence|display_only",
	"signal|io|display_only",
	"significant|causal_rank|soft_consumer",
	"sleep|state|soft_consumer",
	"source|causal_rank|soft_consumer",
	"span_category|span|hard_consumer",
	"span_kind|span|hard_consumer",
	"span_name|span|hard_consumer",
	"span_subcategory|span|hard_consumer",
	"starved_runnable_ms|occupancy|display_only",
	"state|state|display_only",
	"storage_max|io|display_only",
	"subject_kind|causal_rank|hard_consumer",
	"supply_fold_deficit_ms|supply_fold|hard_consumer",
	"supply_fold_ideal_ms|supply_fold|hard_consumer",
	"supply_ratio|compute_supply|soft_consumer",
	"switches|state|soft_consumer",
	"symbol|perf|display_only",
	"symbolization_status|perf|display_only",
	"sync_like|blocking|display_only",
	"target|chain_path|display_only",
	"target_cpus|interrupt|display_only",
	"target_impact|impact|display_only",
	"target_impact_ms|impact|display_only",
	"target_mask|interrupt|display_only",
	"target_prio|cpu_load|display_only",
	"target_priority|cpu_load|display_only",
	"thermal|supply_pressure|display_only",
	"thread|cpu_load|display_only",
	"threads|cpu_load|display_only",
	"throughput|supply_pressure|display_only",
	"tier|causal_rank|hard_consumer",
	"timeline|dma_fence|display_only",
	"top_background_process|cpu_load|display_only",
	"top_background_threads|cpu_load|display_only",
	"top_competitor|guidance|soft_consumer",
	"top_competitor_overlap|guidance|display_only",
	"top_competitor_running|guidance|display_only",
	"top_dev|io|display_only",
	"top_inode|io|display_only",
	"top_name|io|display_only",
	"top_thread|cpu_load|display_only",
	"top_thread_ms|cpu_load|display_only",
	"total|impact|soft_consumer",
	"total_latency|io|display_only",
	"type|causal_rank|hard_consumer",
	"unpaired_done|io|display_only",
	"unpaired_start|io|display_only",
	"value|plugin|display_only",
	"vector|interrupt|display_only",
	"verdict|cpu_load|display_only",
	"waiters|blocking|hard_consumer",
	"wakee_priority|chain_path|display_only",
	"waker_priority|chain_path|display_only",
	"wakeup_ts|chain_path|display_only",
	"weight_unit|perf|display_only",
	"window|anchor_window|anchor_window",
	"window_ms|anchor_window|soft_consumer",
	"window_proportion|anchor_window|display_only",
	"window_source|anchor_window|anchor_window",
	"work|workqueue|display_only",
}

func TestTraceNoteKeyRegistryGolden(t *testing.T) {
	rows := TraceNoteKeyRows()
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, fmt.Sprintf("%s|%s|%s", row.Key, row.Family, row.Carrier))
	}
	// The golden list is maintained in key-sorted order (the same order the
	// registry renders); compare positionally so the golden file itself is
	// the canonical layout.
	want := append([]string(nil), traceNoteKeyGoldenRows...)
	if len(got) != len(want) {
		t.Fatalf("registry has %d rows, golden has %d rows\nregistry:\n%s\ngolden:\n%s",
			len(got), len(want), strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("registry/golden diverge at sorted row %d:\n  registry: %s\n  golden:   %s", i, got[i], want[i])
		}
	}
}

// TestTraceNoteKeyRegistryStructure lints the table itself: unique keys,
// wire-safe charset, occupier prefix/cap lockstep, and the anchor-window
// whitelist stays EXACTLY the three adjudicated carriers.
func TestTraceNoteKeyRegistryStructure(t *testing.T) {
	keyPattern := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	seen := map[string]bool{}
	var anchors []string
	for _, row := range TraceNoteKeyRows() {
		if seen[row.Key] {
			t.Errorf("duplicate registry key %q", row.Key)
		}
		seen[row.Key] = true
		if !keyPattern.MatchString(row.Key) {
			t.Errorf("registry key %q is not wire-safe (want ^[a-z][a-z0-9_]*$)", row.Key)
		}
		switch row.Carrier {
		case TraceNoteCarrierAnchorWindow, TraceNoteCarrierHardConsumer,
			TraceNoteCarrierSoftConsumer, TraceNoteCarrierDisplayOnly:
		default:
			t.Errorf("registry key %q has unknown carrier %q", row.Key, row.Carrier)
		}
		if row.Carrier == TraceNoteCarrierAnchorWindow {
			anchors = append(anchors, row.Key)
		}
		if strings.HasPrefix(row.Key, TraceNoteKeyActualPrefix) && row.Carrier == TraceNoteCarrierDisplayOnly {
			t.Errorf("registry key %q: actual_* keys are prefix-consumed dual-basis markers and can never be display_only", row.Key)
		}
	}
	sort.Strings(anchors)
	wantAnchors := []string{TraceNoteKeySelectedWindow, TraceNoteKeyWindow, TraceNoteKeyWindowSource}
	sort.Strings(wantAnchors)
	if strings.Join(anchors, ",") != strings.Join(wantAnchors, ",") {
		t.Errorf("anchor-window whitelist drifted: got %v want %v", anchors, wantAnchors)
	}
	// Occupier roster: prefix + the three registered ordinals stay in lockstep
	// with the producer cap of 3.
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("%s%d", TraceNoteKeyOccupierPrefix, i)
		if !TraceNoteKeyRegistered(key) {
			t.Errorf("occupier roster key %q missing from registry", key)
		}
	}
	if TraceNoteKeyRegistered(TraceNoteKeyOccupierPrefix + "4") {
		t.Errorf("occupier_4 registered but the producer roster cap is 3 — raise both in lockstep or neither")
	}
}
