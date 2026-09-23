package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const schedulerConcurrencyPublicTrace = "idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n" +
	"idle-0 (0) [001] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=400 next_prio=120\n" +
	"other-400 (400) [001] .... 1.002000: sched_wakeup: comm=worker pid=300 prio=120 target_cpu=0\n" +
	"app-100 (100) [000] .... 1.004000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=300 next_prio=120\n" +
	"other-400 (400) [001] .... 1.005000: sched_switch: prev_comm=other prev_pid=400 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
	"worker-300 (300) [000] .... 1.006000: sched_switch: prev_comm=worker prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=app next_pid=100 next_prio=120\n" +
	"app-100 (100) [000] .... 1.008000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
	"app-100 (100) [000] .... 1.010000: tracing_mark_write: I|100|window_end\n"

func TestSchedulerConcurrencyPublicToolProjection(t *testing.T) {
	result, stats, path := schedulerConcurrencyToolQuery(t, schedulerConcurrencyPublicTrace, nil)
	if stats == nil || stats.Window == nil || stats.Window.StartTs != 1 || stats.Window.EndTs != 1.01 || len(stats.Groups) != 2 {
		t.Fatalf("native population prerequisite: %+v", stats)
	}
	for _, group := range stats.Groups {
		if group.SourcePath != path || group.Values == nil {
			t.Fatalf("physical measured group prerequisite: %+v", group)
		}
		peak, mean, busy, area := 1, .2, 2.0, 2.0
		if group.State == "running" {
			peak, mean, busy, area = 2, 1.2, 8, 12
		}
		v := group.Values
		if v.PeakThreads != peak || math.Abs(v.MeanThreads-mean) > 1e-7 || math.Abs(v.BusyMs-busy) > 1e-7 || math.Abs(v.ThreadMs-area) > 1e-7 {
			t.Fatalf("wrong native population %s: %+v", group.State, v)
		}
		var rows []types.ObservationRecord
		for _, r := range result.Observations {
			if r.Predicate == "scheduler_concurrency" && r.Subject == group.State {
				rows = append(rows, r)
			}
		}
		if len(rows) != 1 {
			t.Fatalf("state %s has %d typed publications", group.State, len(rows))
		}
		r := rows[0]
		if r.Role != types.AnswerAggregateRoleSupportingCoverage || r.SourceRef.Path != path || r.SourceRef.QueryScopeID == "" || r.Value != fmt.Sprint(peak) || r.Unit != "threads" {
			t.Fatalf("publication authority/source/value drift: %+v", r)
		}
		projection := types.ProjectObservationPromptRecords(rows, nil, nil, types.SemanticReviewObservationPromptProjectionOptions(1))
		if len(projection) != 1 || len(projection[0].Notes) > 6 {
			t.Fatalf("compact note budget changed: %+v", projection)
		}
		for name, surface := range map[string]string{"summary": result.Summary, "typed": r.Summary, "compact": projection[0].Summary} {
			for _, want := range []string{fmt.Sprintf("峰值=%d", peak), fmt.Sprintf("全窗平均=%.9g", mean), fmt.Sprintf("线程时间合计=%.9g thread·ms", area), "已确认闭合区间", "不是目标等待或根因"} {
				if !strings.Contains(surface, want) {
					t.Errorf("%s %s lost %q: %s", name, group.State, want, surface)
				}
			}
		}
		compact := strings.Join(projection[0].Notes, " ")
		for _, want := range []string{"state=" + group.State, "1.000000..1.010000", "accepted_closed_intervals", "all_positive_tids", "source_conflict=", "seconds:threads"} {
			if !strings.Contains(compact, want) {
				t.Errorf("compact source/state/window/coverage lost %q: %s", want, compact)
			}
		}
		for _, note := range r.RichNotes {
			key, _, _ := strings.Cut(note, "=")
			if _, exists := types.TraceNoteKeyLookup(key); !exists {
				t.Errorf("unregistered note %q", key)
			}
		}
	}
}

func TestSchedulerConcurrencyPublicUnavailableAndAbsent(t *testing.T) {
	t.Run("no_scheduler_observation", func(t *testing.T) {
		result, stats, _ := schedulerConcurrencyToolQuery(t, "io-40 (40) [003] .... 1.003000: tracing_mark_write: I|40|checkpoint\n", nil)
		if stats != nil {
			t.Fatalf("absent scheduler became a measurement: %+v", stats)
		}
		for _, r := range result.Observations {
			if strings.HasPrefix(r.Predicate, "scheduler_concurrency") {
				t.Fatalf("absent scheduler gained an observation: %+v", r)
			}
		}
	})
	t.Run("line_bounds", func(t *testing.T) {
		result, stats, _ := schedulerConcurrencyToolQuery(t, schedulerConcurrencyPublicTrace, map[string]any{"line_start": 1, "line_end": 8})
		if stats == nil || stats.WindowUnavailableReason == "" {
			t.Fatalf("line denominator was invented: %+v", stats)
		}
		for _, want := range []string{"原始行范围：1..8", "行范围优先，未建立时间分母", "未证明采集完整"} {
			if !strings.Contains(result.Summary, want) {
				t.Errorf("first-level public summary omitted %q: %s", want, result.Summary)
			}
		}
		for _, r := range result.Observations {
			if r.Predicate == "scheduler_concurrency" && (r.Value != "" || !strings.Contains(r.Summary, "不能按零处理")) {
				t.Fatalf("line-selected unknown became measured zero: %+v", r)
			}
		}
	})
	t.Run("open_tail", func(t *testing.T) {
		body := "idle-0 (0) [000] .... 1.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n" +
			"app-100 (100) [000] .... 1.010000: tracing_mark_write: I|100|window_end\n"
		result, stats, _ := schedulerConcurrencyToolQuery(t, body, nil)
		if stats == nil || stats.Coverage.OpenEndedIntervals == 0 {
			t.Fatalf("open tail vanished from coverage: %+v", stats)
		}
		found := false
		for _, r := range result.Observations {
			if r.Predicate == "scheduler_concurrency_coverage" {
				found = strings.Contains(strings.Join(r.RichNotes, " "), fmt.Sprintf("open=%d", stats.Coverage.OpenEndedIntervals))
			}
			if r.Predicate == "scheduler_concurrency" && r.Value != "" {
				t.Fatalf("unclosed running tail became a measured count: %+v", r)
			}
		}
		if !found {
			t.Fatal("typed context omitted open-tail diagnostic")
		}
	})
}

func schedulerConcurrencyToolQuery(t *testing.T, body string, extra map[string]any) (types.ToolResult, *tracequery.SchedulerConcurrencyStats, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scheduler.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 1.01}
	for key, value := range extra {
		params[key] = value
	}
	encoded, _ := json.Marshal(params)
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("Describe scheduler resource measurements")}, encoded)
	if err != nil || !result.Success {
		t.Fatalf("public query failed: %v %+v", err, result)
	}
	payloadPath := result.RawRef
	for _, r := range result.Observations {
		if r.SourceRef.PayloadRef != "" {
			payloadPath = r.SourceRef.PayloadRef
			break
		}
	}
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(payload, &native); err != nil || native.WindowStats == nil {
		t.Fatalf("query JSON missing window stats: %v", err)
	}
	original, err := os.ReadFile(path)
	if err != nil || string(original) != body {
		t.Fatal("public query changed original trace")
	}
	return result, native.WindowStats.SchedulerConcurrency, physical
}
