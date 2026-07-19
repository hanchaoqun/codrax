package tracediag

// evalcase_dh_scripts_test.go — EVALCASE-DH batch, 零 LLM tracediag 配置看护:
// the five shipped collection scripts (DH-J3 双窗分化 / DH-IO2 churn /
// DH-R1 runnable 风暴 / DH-C2 单窗多 target / XA-V1 微窗) parse under the
// strict loader, keep their step shapes, and run END TO END against the two
// committed real fixtures with every step succeeding and the case's
// dispositive report tokens in place (mining ledgers evalcase_donghu_mining.md
// / evalcase_xa_cmp_mining.md; expectations re-collected at HEAD 1ada2c49f).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func evalcaseScriptPath(name string) string {
	return filepath.Join("..", "..", "examples", "tracediag", name)
}

func evalcaseFixturePath(name string) string {
	return filepath.Join("..", "..", "eval", "fixtures", "real_traces", name)
}

// Shape pins: the five scripts stay statically-windowed v1 collections with
// the exact step rosters the eval cases were designed around.
func TestEvalcaseDHScriptShapes(t *testing.T) {
	type stepWant struct {
		view   string
		pid    int
		window string
	}
	cases := []struct {
		script string
		steps  []stepWant
	}{
		{"collect_dh_j3_dual_window.yaml", []stepWant{
			{"window_stats", 17267, "13762.937400..13762.973600"},
			{"root_cause_rank", 17267, "13762.937400..13762.973600"},
			{"window_stats", 17267, "13762.813789..13762.937367"},
			{"root_cause_rank", 17267, "13762.813789..13762.937367"},
		}},
		{"collect_dh_io2_churn.yaml", []stepWant{
			{"window_stats", 24711, "13763.005000..13763.024898"},
			{"root_cause_rank", 24711, "13763.005000..13763.024898"},
			{"event_search", 0, "13763.005000..13763.024898"},
		}},
		{"collect_dh_r1_runnable.yaml", []stepWant{
			{"window_stats", 9503, "13763.005000..13763.024898"},
			{"root_cause_rank", 9503, "13763.005000..13763.024898"},
			{"root_cause_rank", 24711, "13763.005000..13763.024898"},
		}},
		{"collect_dh_c2_multi_target.yaml", []stepWant{
			{"window_stats", 17585, "13762.980500..13762.985000"},
			{"root_cause_rank", 17585, "13762.980500..13762.985000"},
			{"root_cause_rank", 18130, "13762.980500..13762.985000"},
			{"root_cause_rank", 17457, "13762.980500..13762.985000"},
		}},
		{"collect_xa_v1_micro_window.yaml", []stepWant{
			{"window_stats", 59566, "34579.453000..34579.497500"},
			{"root_cause_rank", 59566, "34579.453000..34579.497500"},
		}},
	}
	for _, tc := range cases {
		script, err := LoadScript(evalcaseScriptPath(tc.script))
		if err != nil {
			t.Errorf("%s must parse without overrides (statically windowed): %v", tc.script, err)
			continue
		}
		if script.Version != 1 || len(script.Steps) != len(tc.steps) {
			t.Errorf("%s shape drifted: version=%d steps=%d want %d", tc.script, script.Version, len(script.Steps), len(tc.steps))
			continue
		}
		for i, want := range tc.steps {
			step := script.Steps[i]
			if step.View != want.view || step.PID != want.pid || step.Window != want.window {
				t.Errorf("%s step %d drifted: view=%s pid=%d window=%q want %+v", tc.script, i, step.View, step.PID, step.Window, want)
			}
		}
	}
	// The churn script's raw-row step is deliberately UNSCOPED (memory events
	// are a global lane; a thread filter would manufacture a false zero
	// census) and pattern-anchored on the raw event name.
	churn, err := LoadScript(evalcaseScriptPath("collect_dh_io2_churn.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	raw := churn.Steps[2]
	if raw.PID != 0 || raw.Thread != "" || raw.Pattern != "mm_filemap_delete" {
		t.Errorf("churn raw step drifted: %+v", raw)
	}
}

func evalcaseRunScript(t *testing.T, script, trace string) string {
	t.Helper()
	if _, err := os.Stat(evalcaseFixturePath(trace)); err != nil {
		t.Skipf("real fixture not present: %v", err)
	}
	var buf bytes.Buffer
	failed, err := Run(nil, Options{
		ScriptPath: evalcaseScriptPath(script),
		TracePath:  evalcaseFixturePath(trace),
		Version:    "evalcase-test",
		BuildTime:  "evalcase-test",
		Now:        func() time.Time { return time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC) },
	}, &buf)
	if err != nil {
		t.Fatalf("%s: Run: %v", script, err)
	}
	if failed != 0 {
		t.Fatalf("%s: %d failed steps\n%s", script, failed, buf.String())
	}
	return buf.String()
}

func evalcaseWantTokens(t *testing.T, script, report string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(report, want) {
			t.Errorf("%s: report missing %q", script, want)
		}
	}
}

// DH-J3 e2e: the SAME target's two windows answer differently — the
// intra-frame window is single-seat compute-bound on the big core while the
// inter-frame gap splits per-cpu across clusters, with the fscache witness
// lane present.
func TestEvalcaseDHJ3DualWindowEndToEnd(t *testing.T) {
	report := evalcaseRunScript(t, "collect_dh_j3_dual_window.yaml", "donghu.ftrace")
	evalcaseWantTokens(t, "dh_j3", report, []string{
		"结论: 全部 4 步骤成功。",
		// 帧内: single big-core seat at full frequency.
		"running=32.739ms runnable=0.239ms high_prio_running=32.739ms system_or_kernel_running=0.000ms cpu=12 prio=53/ohos_rt",
		"duration_ms=32.739 cpu=12 frequency=2750000",
		"low_frequency_cpus: [4, 5, 6, 7, 8, 9, 10, 11]",
		// 帧间: per-cpu cross-cluster split rows.
		"duration_ms=57.828 cpu=12",
		"duration_ms=28.477 cpu=4",
		// 帧间 gap window echo + the complete io pairing account (the mining
		// P2 fact: 167/167 pairs; the per-caller fscache census is pinned at
		// the engine face in TestEvalcaseDHJ2InterFrameGapMigration).
		"window=[13762.813789..13762.937367]",
		"block_issue_count=167 block_complete_count=167",
	})
}

// DH-IO2 e2e: churn identity + reclaim entity + raw delete rows.
func TestEvalcaseDHIO2ChurnEndToEnd(t *testing.T) {
	report := evalcaseRunScript(t, "collect_dh_io2_churn.yaml", "donghu.ftrace")
	evalcaseWantTokens(t, "dh_io2", report, []string{
		"结论: 全部 3 步骤成功。",
		"signal=page_cache_churn score=438.195",
		"page_cache_churn=2167",
		"top_inode=0x9903f",
		"sysmgr-reclaim0-9",
		"sh-19629 thread load running=19.402ms",
		"mm_filemap_delete",
	})
}

// DH-R1 e2e: the runnable storm's victims + the cpu·ms caliber words + the
// display-cap overflow disclosure (帽基不当全量).
func TestEvalcaseDHR1RunnableStormEndToEnd(t *testing.T) {
	report := evalcaseRunScript(t, "collect_dh_r1_runnable.yaml", "donghu.ftrace")
	evalcaseWantTokens(t, "dh_r1", report, []string{
		"结论: 全部 3 步骤成功。",
		"hilogcat-9503",
		"11.841",
		"top_runnable shows 8 of",
		"跨线程合计为 cpu·ms,不可当作墙钟耗时",
	})
}

// DH-C2 e2e: three targets in ONE window, each rank step runs on its own
// subject (LegoHandler's per-cpu running fragments stay per-cpu).
func TestEvalcaseDHC2MultiTargetEndToEnd(t *testing.T) {
	report := evalcaseRunScript(t, "collect_dh_c2_multi_target.yaml", "donghu.ftrace")
	evalcaseWantTokens(t, "dh_c2", report, []string{
		"结论: 全部 4 步骤成功。",
		"[步骤 2/4] label=lego_handler_rank view=root_cause_rank",
		"[步骤 3/4] label=transmit_thread_rank view=root_cause_rank",
		"[步骤 4/4] label=tp_io_rank view=root_cause_rank",
		"thread=LegoHandler-17585 duration_ms=1.624 cpu=1",
		"thread=LegoHandler-17585 duration_ms=0.563 cpu=2",
	})
}

// XA-V1 e2e: the sub-50ms micro-window disclosure + the tieba honesty trio
// (wakeup degradation advisory, freq-only donor rows).
func TestEvalcaseXAV1MicroWindowEndToEnd(t *testing.T) {
	report := evalcaseRunScript(t, "collect_xa_v1_micro_window.yaml", "donghu_tieba_frame.systrace")
	evalcaseWantTokens(t, "xa_v1", report, []string{
		"结论: 全部 2 步骤成功。",
		"selected_window_duration=44.500ms is a micro-window probe",
		"use this sub-50ms result only as local evidence",
		"wakeup_target_cpu_degraded=true total=752",
		"advisory only",
		"frequency_cluster_donor_source=freq_change_point_derived",
		"frequency_cluster_donor_cpu: 3",
	})
}
