package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC323ProcessProfilePublicTeachingPreservesIOMarkerKnowledge(t *testing.T) {
	original, err := os.ReadFile("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, marker string
		known        int
		state        tracequery.ThreadState
		d, io        float64
	}{
		{"known_false", "0", 1, tracequery.StateDSleep, 4, 0},
		{"known_true", "1", 1, tracequery.StateIOWait, 0, 4},
		{"malformed", "unknown", 0, tracequery.StateDSleep, 4, 0},
		{"absent", "", 0, tracequery.StateDSleep, 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Replace(string(original), "iowait=1", "iowait="+tc.marker, 1)
			if tc.marker == "" {
				var lines []string
				for _, line := range strings.Split(text, "\n") {
					if !strings.Contains(line, "sched_blocked_reason:") {
						lines = append(lines, line)
					}
				}
				text = strings.Join(lines, "\n")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.ftrace")
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("show process threads")}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "pid": 10, "time_start": 1, "time_end": 1.02})
			result, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("public process query: %v %+v", err, result)
			}
			found := false
			for _, record := range result.Observations {
				profile, valid := DecodeTraceProcessProfile(record)
				if !valid {
					continue
				}
				if profile.TGID != 10 || profile.ThreadCount != 3 || profile.UnavailableThreads != 1 {
					t.Fatalf("native identity or census changed: %+v", profile)
				}
				for _, row := range profile.Threads {
					if row.Thread.PID != 11 {
						continue
					}
					found = true
					v := row.States.Values
					if v == nil || fmt.Sprintf("%.3f/%.3f/%.3f", v.RunningMs, v.DStateMs, v.IOWaitMs) != fmt.Sprintf("15.000/%.3f/%.3f", tc.d, tc.io) || len(row.SleepGroups) != 1 {
						t.Fatalf("native scheduler partition changed: %+v", row)
					}
					group := row.SleepGroups[0]
					if group.State != tc.state || group.IntervalCount != 1 || group.IOFlagKnownIntervals != tc.known || fmt.Sprintf("%.3f/%.3f", group.DurationMs, group.IOFlagPositiveMs) != fmt.Sprintf("4.000/%.3f", tc.io) {
						t.Fatalf("known-zero and unknown marker groups were conflated: %+v", group)
					}
				}
			}
			if !found {
				t.Fatal("public handoff lost the selected process's worker")
			}
			var schema struct {
				Properties map[string]struct{ Description string }
			}
			if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
				t.Fatal(err)
			}
			teaching := schema.Properties["view"].Description
			if !strings.Contains(teaching, "D without an IO-wait mark (known-zero and unknown markers)") || !strings.Contains(teaching, "Do not describe an unknown marker as non-IO") {
				t.Fatalf("public tool teaching contradicts native marker knowledge: %s", teaching)
			}
		})
	}
}

func TestHMC323CapabilityPublicTeachingKeepsResidualDUnknown(t *testing.T) {
	out := capabilityPublicCall(t, nil, `{"view":"window_stats","detail":true}`)
	for _, raw := range out["metrics"].([]any) {
		metric := raw.(map[string]any)
		if metric["id"] != "scheduler_states" {
			continue
		}
		text, _ := json.Marshal(metric["limitations"])
		for _, want := range []string{
			"d_state_ms is the D lane without an IO-wait mark (known-zero and unknown markers), not the physical D total",
			"An unknown marker is not a known non-IO classification",
			"IO markers do not identify a device",
			"uncovered time stays unknown",
		} {
			if !strings.Contains(string(text), want) {
				t.Errorf("public scheduler contract lost %q: %s", want, text)
			}
		}
		return
	}
	t.Fatal("public catalog lost scheduler_states")
}
