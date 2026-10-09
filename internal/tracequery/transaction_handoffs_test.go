package tracequery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func transactionResult(t *testing.T, body string, q Query) *TransactionHandoffsResult {
	t.Helper()
	idx := renderingFixture(t, body)
	q.View = ViewTransactionHandoffs
	r, err := StreamTransactionHandoffs(context.Background(), idx.Path, q)
	if err != nil || r.TransactionHandoffs == nil || !ValidTransactionHandoffs(*r.TransactionHandoffs) {
		t.Fatalf("invalid public stream: %v %+v", err, r.TransactionHandoffs)
	}
	if r.RootCauseRank != nil || r.FrameTimeline != nil || r.WakeupChain != nil {
		t.Fatal("protocol match acquired causal authority")
	}
	want := Run(idx, q)
	if !reflect.DeepEqual(r.TransactionHandoffs, want.TransactionHandoffs) {
		t.Fatalf("stream/index differ\nstream=%+v\nindexed=%+v", r.TransactionHandoffs, want.TransactionHandoffs)
	}
	return r.TransactionHandoffs
}

func TestTransactionHandoffsBranchWindowAndMultiplicity(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	p := transactionResult(t, string(body), Query{TimeStart: 1, TimeEnd: 1.05})
	if p.TotalKeys != 7 || p.WindowSubmissionEvents != 5 || p.WindowConsumptionEvents != 4 {
		t.Fatalf("wrong event populations: %+v", p)
	}
	wants := map[string]string{"101/7": "observed_unique_protocol_match", "102/8": "observed_unique_protocol_match", "101/9": "ambiguous", "101/10": "missing_consumption", "103/11": "missing_submission", "101/12": "observed_unique_protocol_match", "999/13": "identity_unverified"}
	for _, h := range p.Handoffs {
		key := fmt.Sprintf("%d/%s", h.TID, h.Sequence)
		if h.Status != wants[key] {
			t.Errorf("%s: %+v", key, h)
		}
		if key == "101/7" && (h.WindowSubmissions != 0 || h.Submissions[0].InWindow) {
			t.Fatal("window-external origin counted")
		}
		if key == "101/12" && (h.WindowConsumptions != 0 || h.Consumptions[0].InWindow) {
			t.Fatal("right boundary counted")
		}
	}
	trim := transactionResult(t, string(body), Query{TimeStart: 1, TimeEnd: 1.05, Limit: 1})
	if trim.TotalKeys != p.TotalKeys || trim.OmittedKeys != 6 || trim.WindowSubmissionEvents != 5 || trim.WindowConsumptionEvents != 4 {
		t.Fatal("display changed universe")
	}
}

func transactionPairBody(tid, tgid int, seq string) string {
	return renderingMarker("app", tid, tgid, 1.001, "H:MarshRSTransactionData transactionFlag:[101,"+seq+"]") + renderingMarker("rs", 201, 200, 1.02, "H:RSMainThread::ProcessCommandUni [101,"+seq+"]")
}

func TestTransactionHandoffsIdentityAndOrdering(t *testing.T) {
	base := transactionPairBody(101, 100, "9007199254740993")
	for _, tc := range []struct{ name, body, status string }{
		{"precise_sequence", base, "observed_unique_protocol_match"},
		{"unknown_owner", transactionPairBody(101, -1, "7"), "identity_unverified"},
		{"wrong_emitter", transactionPairBody(102, 100, "7"), "identity_unverified"},
		{"lifecycle", strings.Replace(base, "rs-201", "parent-300 (300) [000] .... 1.010000000: sched_wakeup_new: comm=app pid=101 prio=120 target_cpu=0\nrs-201", 1), "identity_unverified"},
		{"reverse_time", strings.Replace(base, "1.020000000", "1.000000000", 1), "order_unverified"},
		{"physical_reorder", strings.Split(base, "\n")[1] + "\n" + strings.Split(base, "\n")[0] + "\n", "observed_unique_protocol_match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := transactionResult(t, tc.body, Query{TimeStart: 1, TimeEnd: 1.05})
			if len(p.Handoffs) != 1 || p.Handoffs[0].Status != tc.status {
				t.Fatalf("identity/order: %+v", p)
			}
		})
	}
}

func TestTransactionHandoffsWholeSourceBeforeDisplay(t *testing.T) {
	body := transactionPairBody(101, 100, "7")
	for i := 0; i < 12; i++ {
		body += renderingMarker("app", 101, 100, 2+float64(i), "H:MarshRSTransactionData transactionFlag:[101,7]")
	}
	p := transactionResult(t, body, Query{TimeStart: 1, TimeEnd: 1.05, Limit: 1})
	h := p.Handoffs[0]
	if h.Status != "ambiguous" || h.SubmissionCount != 13 || h.OmittedSubmissions != 9 || h.WindowSubmissions != 1 {
		t.Fatalf("truncated uniqueness: %+v", h)
	}
}

func TestTransactionHandoffsRejectPartialUniversesAndFilters(t *testing.T) {
	idx := renderingFixture(t, transactionPairBody(101, 100, "7"))
	q := Query{View: ViewTransactionHandoffs, TimeStart: 1, TimeEnd: 1.05}
	for _, change := range []func(*Query){func(q *Query) { q.LineStart = 1 }, func(q *Query) { q.Pattern = "x" }, func(q *Query) { q.Patterns = []string{"x"} }, func(q *Query) { q.EventNames = []string{"print"} }, func(q *Query) { q.EventTypes = []EventType{EventTraceMark} }, func(q *Query) { q.TraceMarkActions = []TraceMarkAction{"B"} }, func(q *Query) { q.EventFieldFilters = []EventFieldFilter{{}} }} {
		copy := q
		change(&copy)
		r, err := StreamTransactionHandoffs(context.Background(), idx.Path, copy)
		if err != nil || r.TransactionHandoffs.Status != "unavailable" {
			t.Fatalf("filter ignored %+v %v", copy, err)
		}
	}
	for _, limits := range [][2]int{{1, 1 << 20}, {100, 8}} {
		r, err := streamTransactionHandoffs(context.Background(), idx.Path, q, limits[0], limits[1])
		if err != nil || r.TransactionHandoffs.Status != "unavailable" || r.TransactionHandoffs.TotalKeys != 0 || r.ScannedLineCount != idx.LineCount {
			t.Fatalf("prefix became population: %+v %v", r, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := StreamTransactionHandoffs(ctx, idx.Path, q)
	if !errors.Is(err, context.Canceled) || r.TransactionHandoffs != nil {
		t.Fatal("canceled prefix published")
	}
	idx.Windowed = true
	if Run(idx, q).TransactionHandoffs.Status != "unavailable" {
		t.Fatal("window index certified unique")
	}
}

func TestTransactionHandoffsBundleSources(t *testing.T) {
	for _, shape := range []string{"identity", "multi", "affine", "stale"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			child, bundle := filepath.Join(dir, "events.systrace"), filepath.Join(dir, "capture.tracebundle.json")
			body := transactionPairBody(101, 100, "7")
			os.WriteFile(child, []byte(body), 0600)
			manifest := `{"systrace":"events.systrace"}`
			if shape == "multi" {
				os.WriteFile(filepath.Join(dir, "other.systrace"), []byte(body), 0600)
				manifest = `{"systrace":"events.systrace","artifacts":[{"type":"systrace","path":"other.systrace"}]}`
			}
			if shape == "affine" {
				manifest = `{"systrace":"events.systrace","perf_clock_alignments":[{"artifact_path":"events.systrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":1,"slope":1,"calibrated":true}]}`
			}
			writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
			if shape == "stale" {
				os.WriteFile(child, []byte(body+"# changed\n"), 0600)
			}
			r, err := StreamTransactionHandoffs(context.Background(), bundle, Query{View: ViewTransactionHandoffs, TimeStart: 1, TimeEnd: 1.05})
			if shape != "identity" {
				if err == nil || r.TransactionHandoffs != nil {
					t.Fatal("unproven source admitted")
				}
				return
			}
			if err != nil || !ValidTransactionHandoffs(*r.TransactionHandoffs) {
				t.Fatalf("identity source failed %v %+v", err, r)
			}
			h := r.TransactionHandoffs.Handoffs[0]
			if h.Submissions[0].SourcePath != canonicalTraceIndexPath(child) || h.Submissions[0].SourceLine != 1 || r.SourcePath != canonicalTraceIndexPath(bundle) {
				t.Fatalf("lost physical source %+v", h)
			}
		})
	}
}

func TestTransactionHandoffsNavigationFromProtocolOnly(t *testing.T) {
	for _, body := range []string{transactionPairBody(101, 100, "7"), renderingMarker("MarshRSTransactionData", 101, 100, 1.001, "ordinary")} {
		idx := renderingFixture(t, body)
		p := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.05})
		got := strings.Contains(strings.Join(p.Caveats, "\n"), "transaction_handoffs can")
		if got != strings.Contains(body, "transactionFlag:") {
			t.Fatal("soft navigation is not grounded in precise protocol")
		}
	}
}

func TestTransactionHandoffsValidateEventPopulations(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	p := transactionResult(t, string(body), Query{TimeStart: 1, TimeEnd: 1.05})
	for _, change := range []func(*TransactionHandoffsResult){func(p *TransactionHandoffsResult) { p.WindowConsumptionEvents++ }, func(p *TransactionHandoffsResult) { p.WindowSubmissionEvents++ }, func(p *TransactionHandoffsResult) { p.Handoffs[0].WindowSubmissions = 0 }, func(p *TransactionHandoffsResult) { p.Handoffs[0].Submissions[0].InWindow = false }} {
		copy := *p
		copy.Handoffs = append([]TransactionHandoff(nil), p.Handoffs...)
		for i := range copy.Handoffs {
			copy.Handoffs[i].Submissions = append([]TransactionEndpoint(nil), p.Handoffs[i].Submissions...)
		}
		change(&copy)
		if ValidTransactionHandoffs(copy) {
			t.Fatal("forged population admitted")
		}
	}
}

func TestTransactionHandoffsBranchBudgetNoPrefix(t *testing.T) {
	body := renderingMarker("app", 101, 100, 1.001, "H:MarshRSTransactionData transactionFlag:[101,7]") + renderingMarker("rs", 201, 200, 1.02, "H:RSMainThread::ProcessCommandUni [101,7] [101,8] [101,9]")
	idx := renderingFixture(t, body)
	r, err := streamTransactionHandoffs(context.Background(), idx.Path, Query{View: ViewTransactionHandoffs, TimeStart: 1, TimeEnd: 1.05}, 3, 1<<20)
	if err != nil || r.TransactionHandoffs.Status != "unavailable" || r.TransactionHandoffs.TotalKeys != 0 || r.ScannedLineCount != idx.LineCount {
		t.Fatalf("large fanout published prefix: %v %+v", err, r)
	}
}
