package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestTraceQueryTypedObservationsCoverTypedProductBeyondSummaryCaps pins the
// A1 contract: the typed result product is published as ObservationRecord
// rows attached to the ToolResult, including evidence-pack facts beyond the
// prose preview's 16-fact cap, and every row keeps runtime-artifact origin.
func TestTraceQueryTypedObservationsCoverTypedProductBeyondSummaryCaps(t *testing.T) {
	facts := make([]tracequery.EvidenceFact, 0, 20)
	for i := 0; i < 20; i++ {
		facts = append(facts, tracequery.EvidenceFact{
			Subject:    fmt.Sprintf("thread-%d", i),
			Predicate:  "slept",
			Object:     "futex_wait",
			Summary:    fmt.Sprintf("fact %d", i),
			LineStart:  100 + i,
			LineEnd:    101 + i,
			Confidence: 0.9,
		})
	}
	result := tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "/traces/app.systrace",
		TimeStart:  1.0,
		TimeEnd:    2.0,
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "primary", Type: "binder_wait",
					Thread:   tracequery.ThreadRef{Comm: "app", PID: 20},
					ImpactMs: 12.5, Score: 0.91, Confidence: 0.88,
					LineStart: 10, LineEnd: 20, Source: "wakeup_chain",
					Summary: "binder reply stalled the frame",
				},
				{
					Rank: 2, Type: "runnable_wait",
					Thread:   tracequery.ThreadRef{Comm: "app", PID: 20},
					ImpactMs: 4.5, Confidence: 0.6,
				},
			},
		},
		WakeupChain: &tracequery.ChainResult{
			RootEvidence: []tracequery.RootEvidence{{
				Type:       "long_sleep",
				Thread:     tracequery.ThreadRef{Comm: "app", PID: 20},
				DurationMs: 9.25,
				LineStart:  30, LineEnd: 31,
				Summary:    "slept on futex",
				Confidence: 0.8,
			}},
		},
		CriticalBlocking: &tracequery.CriticalBlockingResult{
			Items: []tracequery.CriticalBlockingCandidate{{
				Type:       "futex",
				Thread:     tracequery.ThreadRef{Comm: "app", PID: 20},
				Peer:       tracequery.ThreadRef{Comm: "worker", PID: 21},
				DurationMs: 7.5, LineStart: 40, LineEnd: 41,
				Confidence: 0.7, Summary: "futex hold",
			}},
		},
		WindowStats: &tracequery.WindowStats{
			StateChurn: []tracequery.ThreadStateChurnSummary{{
				Thread:           tracequery.ThreadRef{Comm: "app", PID: 20},
				DominantState:    "runnable",
				DominantImpactMs: 5.5, TotalMs: 9.0,
				FragmentCount: 12, StateSwitches: 24,
				LineStart: 50, LineEnd: 60, Confidence: 0.66,
				RunnableCPU: 1, RunnableCPUKnown: true,
				TopCompetitor: "rival-30", TopCompetitorRunningMs: 4.0,
				NextStep: "inspect rival-30 on same CPU cpu=1 for CPU pressure/time-slice competition, then validate wake_latency with sched_wakeup",
				Summary:  "fragmented runnable churn",
			}},
			FileIOByInode: []tracequery.FileIOSummary{{
				Inode: "0x478e5", Dev: "253:7", EntryName: "db.sqlite",
				Operation: "read", Bytes: 4096,
				TotalLatencyMs: 3.5, LineStart: 70, LineEnd: 71,
				Summary: "inode read burst",
			}},
		},
		EvidencePack: facts,
	}
	rows := traceQueryTypedObservations(result, "attached_trace", "/blobs/trace-query-result-abcd1234.json", "/blobs/trace_query-eeff.txt", "", time.Now())

	wantRows := 2 + 1 + 1 + 1 + 1 + len(facts)
	if len(rows) != wantRows {
		t.Fatalf("expected %d typed rows, got %d", wantRows, len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
			t.Fatalf("row %s origin %q is not runtime_artifact", row.ID, row.Origin)
		}
		if row.SourceRef.Kind != types.ObservationSourceRuntimeArtifact {
			t.Fatalf("row %s source kind %q is not runtime_artifact", row.ID, row.SourceRef.Kind)
		}
		if row.SourceRef.ArtifactID != "attached_trace" {
			t.Fatalf("row %s artifact id %q", row.ID, row.SourceRef.ArtifactID)
		}
		if !strings.HasPrefix(row.ID, "trace_query:trace-query-result-abcd1234.json#") {
			t.Fatalf("row ID %q is not payload-anchored", row.ID)
		}
		if seen[row.ID] {
			t.Fatalf("duplicate typed row ID %q", row.ID)
		}
		seen[row.ID] = true
	}

	var rootCause *types.ObservationRecord
	for i := range rows {
		if strings.HasSuffix(rows[i].ID, "#root_cause_rank:1") {
			rootCause = &rows[i]
			break
		}
	}
	if rootCause == nil {
		t.Fatalf("missing primary root-cause row: %v", seen)
	}
	if rootCause.Role != types.AnswerAggregateRolePrincipalAnswer ||
		rootCause.GroundingPolicy != types.ClaimGroundingHard ||
		rootCause.ProvenanceLane != types.ObservationProvenanceObservedDirectCause {
		t.Fatalf("primary root-cause classification drifted: %+v", rootCause)
	}
	if rootCause.Predicate != "root_cause_primary" || rootCause.Object != "binder_wait" ||
		rootCause.Value != "12.500" || rootCause.Unit != "ms" {
		t.Fatalf("primary root-cause fields drifted: %+v", rootCause)
	}
	var churnRow *types.ObservationRecord
	for i := range rows {
		if strings.Contains(rows[i].ID, "#state_churn:1") {
			churnRow = &rows[i]
			break
		}
	}
	if churnRow == nil {
		t.Fatalf("missing state churn row: %v", seen)
	}
	notes := strings.Join(churnRow.RichNotes, "\n")
	for _, want := range []string{
		"runnable_cpu=1",
		"top_competitor=rival-30",
		"top_competitor_running=4.000",
		"next_step=inspect rival-30 on same CPU cpu=1",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("state churn notes missing %q: %+v", want, churnRow.RichNotes)
		}
	}

	// Facts beyond the 16-fact prose preview cap must survive.
	for _, ordinal := range []int{17, 18, 19, 20} {
		id := fmt.Sprintf("trace_query:trace-query-result-abcd1234.json#evidence_fact:%d", ordinal)
		if !seen[id] {
			t.Fatalf("evidence fact beyond prose cap missing: %s", id)
		}
	}
}

// TestTraceQueryTypedObservationsScopeSuffixKeepsMultiWindowIDsDistinct pins
// the auto-window candidate path: one ToolResult carrying several bounded
// child runs publishes each child's rows under a distinct ID namespace.
func TestTraceQueryTypedObservationsScopeSuffixKeepsMultiWindowIDsDistinct(t *testing.T) {
	result := tracequery.Result{
		View:       "frame_window",
		SourcePath: "/traces/app.systrace",
		EvidencePack: []tracequery.EvidenceFact{{
			Subject: "app-20", Predicate: "ran", Summary: "fact", LineStart: 1,
		}},
	}
	a := traceQueryTypedObservations(result, "path", "/blobs/p.json", "", "w1", time.Now())
	b := traceQueryTypedObservations(result, "path", "/blobs/p.json", "", "w2", time.Now())
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one row per window, got %d/%d", len(a), len(b))
	}
	if a[0].ID == b[0].ID {
		t.Fatalf("window-scoped IDs collide: %q", a[0].ID)
	}
	if !strings.Contains(a[0].ID, ":w1#") || !strings.Contains(b[0].ID, ":w2#") {
		t.Fatalf("window scope suffix missing: %q / %q", a[0].ID, b[0].ID)
	}
}

// TestTraceQueryExecuteAttachesTypedObservations runs the real Execute path on
// a small fixture and pins that the returned ToolResult carries typed rows of
// runtime-artifact origin alongside the prose Summary.
func TestTraceQueryExecuteAttachesTypedObservations(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "sample.systrace",
		"view":       "evidence_pack",
		"pid":        20,
		"time_start": 1.0,
		"time_end":   1.1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if len(res.Observations) == 0 {
		t.Fatalf("expected typed observations on the tool result; summary:\n%s", res.Summary)
	}
	for _, row := range res.Observations {
		if row.Origin != types.AnswerEvidenceOriginRuntimeArtifact {
			t.Fatalf("typed row %s origin %q is not runtime_artifact", row.ID, row.Origin)
		}
		if strings.TrimSpace(row.ID) == "" {
			t.Fatalf("typed row missing ID: %+v", row)
		}
	}
}
