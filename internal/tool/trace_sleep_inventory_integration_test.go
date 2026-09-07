package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1607SleepInventoryIndex(t *testing.T) *tracequery.Index {
	t.Helper()
	text := `idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120
ui-100 (100) [000] .... 10.001000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=200 next_prio=120
worker-200 (200) [000] .... 10.001200: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=0
worker-200 (200) [000] .... 10.001400: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120
ui-100 (100) [000] .... 10.002000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=D ==> next_comm=worker next_pid=200 next_prio=120
worker-200 (200) [000] .... 10.003000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=0
worker-200 (200) [000] .... 10.003100: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120
ui-100 (100) [000] .... 10.004000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=200 next_prio=120
worker-200 (200) [000] .... 10.008000: sched_wakeup: comm=ui pid=100 prio=120 target_cpu=0
worker-200 (200) [000] .... 10.009000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=100 next_prio=120
ui-100 (100) [000] .... 10.010000: sched_switch: prev_comm=ui prev_pid=100 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120
`
	path := filepath.Join(t.TempDir(), "sleep.ftrace")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestB1607SleepInventoryPublishesOutsideChainSelectionWithoutRootAuthority(t *testing.T) {
	idx := b1607SleepInventoryIndex(t)
	for _, view := range []string{"wakeup_chain", "root_cause_rank", "frame_root_cause_bundle", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 100, TimeStart: 10, TimeEnd: 10.01, MinDurationMs: 3, MaxBranches: 1, MaxDepth: 1})
			obs := traceQueryTypedObservations(result, "sleep.ftrace", "payload", "raw", "sleep-test", time.Unix(1, 0))
			var old []types.ObservationRecord
			var set *types.ObservationRecord
			rows := 0
			for i := range obs {
				r := obs[i]
				if r.Predicate == "target_sleep_inventory" {
					set = &obs[i]
				}
				if r.Predicate == "target_sleep_interval" {
					rows++
				}
				if r.Predicate != "target_sleep_inventory" && r.Predicate != "target_sleep_interval" {
					old = append(old, r)
					continue
				}
				if r.Role != types.AnswerAggregateRoleSupportingCoverage {
					t.Fatalf("state census gained causal role: %+v", r)
				}
				for _, note := range r.RichNotes {
					if strings.HasPrefix(note, types.TraceNoteKeyRank+"=") || strings.HasPrefix(note, types.TraceNoteKeyEffectiveImpactMS+"=") {
						t.Fatalf("unproved state interval gained root price: %+v", r)
					}
				}
			}
			if set == nil || set.ResultCount == nil || *set.ResultCount != 3 || rows != 3 {
				t.Fatalf("full target sleep census lost short/plain S intervals: set=%+v rows=%d", set, rows)
			}
			if account := traceQueryTargetWindowStatesAccount(result); account == nil || len(account.WaitOccurrences) != 1 {
				t.Fatalf("plain S entered the older D/IO census: %+v", account)
			}
			if !strings.Contains(set.Summary, "5.200ms") || !strings.Contains(set.Summary, "not prove") {
				t.Fatalf("census lost measured extent or mechanism boundary: %s", set.Summary)
			}
			before, _ := json.Marshal(types.CompileTraceCausalProjection(types.ObservationLedger{Records: old}))
			after, _ := json.Marshal(types.CompileTraceCausalProjection(types.ObservationLedger{Records: obs}))
			if string(before) != string(after) {
				t.Fatal("independent state inventory changed the causal projection")
			}
			head := traceQuerySummary(result, traceQueryParams{}, "sleep.ftrace", "payload")
			if !strings.Contains(head, "Target sleep intervals") || !strings.Contains(head, "5.200ms") {
				t.Fatalf("tool head omitted independent state inventory: %s", head)
			}
		})
	}
}

func TestB1607SleepInventoryPromptKeepsHeadAmountAndAuthorityBoundary(t *testing.T) {
	result := tracequery.Run(b1607SleepInventoryIndex(t), tracequery.Query{View: "wakeup_chain", PID: 100, TimeStart: 10, TimeEnd: 10.01})
	account := result.TargetWindowStates
	if account == nil || account.SleepInventory == nil {
		t.Fatal("real query omitted inventory")
	}
	for _, status := range []string{"unknown", "recovered", "observed_in_index", "future_status"} {
		t.Run(status, func(t *testing.T) {
			account.SleepInventory.HeadState = &tracequery.TimelineHeadState{Status: status, BoundaryTs: 10, State: tracequery.StateSSleep, ActualStartTs: 9.5, SourceLine: 7}
			obs := traceQuerySleepInventoryObservations(account, types.ObservationSourceRef{}, "prompt", "")
			projected := types.ProjectObservationPromptRecords(obs, nil, nil, types.DefaultObservationPromptProjectionOptions(len(obs)))
			for _, p := range projected {
				if strings.HasSuffix(p.ID, "#target_sleep_inventory") {
					for _, want := range []string{"5.200ms", "window-head state", "10.000000s", "does not prove cause"} {
						if !strings.Contains(p.Summary, want) {
							t.Fatalf("compact model context lost %q: %s", want, p.Summary)
						}
					}
					if (status == "unknown" || status == "future_status") && !strings.Contains(p.Summary, "unknown") {
						t.Fatalf("unknown head disguised as complete: %s", p.Summary)
					}
				} else if !strings.Contains(p.Summary, "not proof of cause or completion") || !strings.Contains(p.Summary, "ms") {
					t.Fatalf("compact state row lost amount or noncausal boundary: %s", p.Summary)
				}
				if strings.Contains(p.Summary, "observed_in_index") || strings.Contains(p.Summary, "future_status") {
					t.Fatalf("internal status leaked into model wording: %s", p.Summary)
				}
			}
		})
	}
}

func TestB1607SleepInventoryPreviewAndPayloadCapsRemainExplicit(t *testing.T) {
	account := &tracequery.TargetWindowStateAccount{SleepInventory: &tracequery.TargetWindowSleepInventory{
		Total: 39, Emitted: 32, TotalMs: 7.8,
		Occurrences: make([]tracequery.TargetWindowSleepOccurrence, 32),
	}}
	for i := range account.SleepInventory.Occurrences {
		account.SleepInventory.Occurrences[i] = tracequery.TargetWindowSleepOccurrence{Ordinal: i + 1, Interval: tracequery.Interval{State: tracequery.StateSSleep, DurationMs: .2}}
	}
	var b strings.Builder
	writeTraceSleepInventoryPreview(&b, account, "payload-1")
	text := b.String()
	if strings.Count(text, "- State only") != 4 || !strings.Contains(text, "shows 4/39") || !strings.Contains(text, "retains 32/39") || !strings.Contains(text, "7.800ms") {
		t.Fatalf("preview silently lost cap/union semantics: %s", text)
	}
	rows := traceQuerySleepInventoryObservations(account, types.ObservationSourceRef{}, "cap", "")
	if len(rows) != 33 || rows[0].ResultCount == nil || *rows[0].ResultCount != 39 {
		t.Fatalf("payload truncated total to emitted rows: %+v", rows)
	}
}
