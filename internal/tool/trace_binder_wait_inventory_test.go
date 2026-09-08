package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1607BinderInventoryResult() tracequery.Result {
	i := &tracequery.TargetWindowBinderWaitInventory{
		Thread: tracequery.ThreadRef{PID: 100, Comm: "client"},
		Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
		Scope:  "indexed_target_verified_closed_waits", ScanStatus: "complete", OutputStatus: "incomplete",
		TargetSleepCount: 48, ConfirmedCount: 39, ConfirmedMs: 3.9,
		UnresolvedCandidateCount: 2, RemainingUnassociatedCount: 7, Emitted: 32,
		CausalAttributionStatus: "not_assessed",
	}
	for n := 0; n < 32; n++ {
		start := 10.01 + float64(n)*.001
		i.Occurrences = append(i.Occurrences, tracequery.TargetWindowBinderWaitOccurrence{
			Ordinal:  n + 1,
			Interval: tracequery.Interval{State: tracequery.StateSSleep, StartTs: start, EndTs: start + .0001, DurationMs: .1, StartLine: 10 + n*10, EndLine: 16 + n*10},
			Peer:     tracequery.ThreadRef{PID: 200, Comm: "server"}, ClosureStatus: "verified_reply_wakeup",
			RequestTransactionID: n*2 + 1, ReplyTransactionID: n*2 + 2,
			RequestSendLine: 9 + n*10, RequestReceiveLine: 11 + n*10,
			ReplySendLine: 13 + n*10, ReplyReceiveLine: 17 + n*10, ClosureLine: 15 + n*10,
			RequestSendTs: start - .0001, RequestReceiveTs: start + .00001,
			ReplySendTs: start + .00009, ReplyReceiveTs: start + .0002, ClosureTs: start + .0001,
		})
	}
	return tracequery.Result{View: "wakeup_chain", TargetWindowStates: &tracequery.TargetWindowStateAccount{
		Thread: i.Thread, Window: i.Window, LineStart: 1, LineEnd: 500,
		BinderWaitInventory: i,
	}}
}

func TestB1607BinderInventoryActualH1ToolPublication(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "wakeup_chain", "pid": 17267,
		"time_start": 13762.791708, "time_end": 13763.024898, "min_duration_ms": 1,
	})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("actual query failed: %v, %s", err, result.Summary)
	}
	sets, rows := 0, 0
	var old []types.ObservationRecord
	for _, r := range result.Observations {
		switch r.Predicate {
		case "target_binder_wait_inventory":
			sets++
			if r.ResultCount == nil || *r.ResultCount != 5 || r.Value != "3.094" || !strings.Contains(r.Summary, "unassociated=60") {
				t.Fatalf("actual H1 inventory missed sub-ms closed waits: %+v", r)
			}
		case "target_binder_wait_interval":
			rows++
			if len(r.SupportRefs) < 5 || r.SourceRef.ArtifactID == "" && types.RuntimeArtifactCaptureIdentityPath(r.SourceRef) == "" {
				t.Fatalf("endpoint/source references missing: %+v", r)
			}
		default:
			old = append(old, r)
		}
	}
	if sets != 1 || rows != 5 || !strings.Contains(result.Summary, "union=3.094ms") || !strings.Contains(result.Summary, "shows 4/5") {
		t.Fatalf("published context dropped full pre-cap inventory: sets=%d rows=%d summary=%s", sets, rows, result.Summary)
	}
	before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: old}))
	after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: result.Observations}))
	if string(before) != string(after) {
		t.Fatal("independent full inventory changed existing causal projection")
	}
}

func TestB1607BinderInventorySharedToolBoundaryKeepsTotalScopeAndNoRoot(t *testing.T) {
	r := b1607BinderInventoryResult()
	before, _ := json.Marshal(r)
	obs := traceQueryTypedObservations(r, "sample.ftrace", "payload", "raw", "binder-inventory", time.Unix(1, 0))
	var legacy []types.ObservationRecord
	sets, rows := 0, 0
	for _, o := range obs {
		switch o.Predicate {
		case "target_binder_wait_inventory":
			sets++
			if o.Value != "3.900" || o.ResultCount == nil || *o.ResultCount != 39 {
				t.Fatalf("inventory total replaced with emitted-prefix sum: %+v", o)
			}
		case "target_binder_wait_interval":
			rows++
		default:
			legacy = append(legacy, o)
			continue
		}
		if o.Role != types.AnswerAggregateRoleSupportingCoverage || len(o.RichNotes) != 1 ||
			!strings.HasPrefix(o.RichNotes[0], types.TraceNoteKeySelectedWindow+"=") {
			t.Fatalf("inventory must carry scope, never root rank or causal prices: %+v", o)
		}
	}
	if sets != 1 || rows != 32 {
		t.Fatalf("shared publication lost independent inventory: sets=%d rows=%d", sets, rows)
	}
	summary := traceQuerySummary(r, traceQueryParams{}, "sample.ftrace", "payload")
	for _, want := range []string{"39", "3.900ms", "unresolved=2", "unassociated=7", "shows 4/39", "retains 32/39"} {
		if !strings.Contains(summary, want) {
			t.Errorf("tool summary omitted %q", want)
		}
	}
	a, _ := json.Marshal(types.CompileTraceCausalProjection(types.ObservationLedger{Records: legacy}))
	b, _ := json.Marshal(types.CompileTraceCausalProjection(types.ObservationLedger{Records: obs}))
	after, _ := json.Marshal(r)
	if string(a) != string(b) || string(before) != string(after) {
		t.Fatal("supporting inventory changed causal projection or source result")
	}
	for _, p := range types.ProjectObservationPromptRecords(obs, nil, nil, types.DefaultObservationPromptProjectionOptions(100)) {
		if strings.HasSuffix(p.ID, "#target_binder_wait_inventory") {
			for _, want := range []string{"39", "3.900ms", "unresolved=2", "unassociated=7", "not all waits or roots"} {
				if !strings.Contains(p.Summary, want) {
					t.Errorf("180-char handoff lost %q: %s", want, p.Summary)
				}
			}
		}
	}
}
