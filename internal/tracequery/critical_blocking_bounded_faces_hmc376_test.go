package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type hmc376FaceSpan struct {
	comm       string
	tid        int
	name       string
	durationUs int
}

func hmc376FaceIndex(t *testing.T, spans []hmc376FaceSpan) *Index {
	t.Helper()
	type event struct {
		us, order int
		text      string
	}
	stamp := func(us int) string { return fmt.Sprintf("%d.%06d", 50+us/1000000, us%1000000) }
	var events []event
	for i, s := range spans {
		begin, end := 2000+i*3, 2000+i*3+s.durationUs
		events = append(events,
			event{begin, i*2 + 1, fmt.Sprintf("%16s-%d   (    700) [002] ....  %s: tracing_mark_write: B|700|%s", s.comm, s.tid, stamp(begin), s.name)},
			event{end, i*2 + 2, fmt.Sprintf("%16s-%d   (    700) [002] ....  %s: tracing_mark_write: E|700", s.comm, s.tid, stamp(end))})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].us != events[j].us {
			return events[i].us < events[j].us
		}
		return events[i].order < events[j].order
	})
	var trace strings.Builder
	trace.WriteString("# tracer: nop\n")
	trace.WriteString("          anchor-700   (    700) [000] ....  50.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=anchor next_pid=700 next_prio=120\n")
	for _, e := range events {
		trace.WriteString(e.text + "\n")
	}
	trace.WriteString("          anchor-700   (    700) [000] ....  53.000000: sched_switch: prev_comm=anchor prev_pid=700 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120\n")
	path := filepath.Join(t.TempDir(), "faces.ftrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func hmc376BusyWindowSpans(targetWait bool) []hmc376FaceSpan {
	var spans []hmc376FaceSpan
	for i := 0; i < 8; i++ {
		spans = append(spans, hmc376FaceSpan{fmt.Sprintf("gen%d", i), 1200 + i, fmt.Sprintf("worker lock wait %d", i), 300000 - i*1000})
	}
	for i := 0; i < 16; i++ {
		spans = append(spans, hmc376FaceSpan{fmt.Sprintf("ver%d", i), 1300 + i, fmt.Sprintf("VerifyClass com.example.Lock%d", i), 30000 - i*500})
	}
	if targetWait {
		spans = append(spans, hmc376FaceSpan{"target", 900, "GC: Wait For Completion Alloc", 100000})
	}
	return spans
}

// The target's own recorded wait joins the list on top of the rows the list held
// without the vocabulary.
func TestHMC376OnChainVocabularyRowIsAdditionalToTheBlockingList(t *testing.T) {
	query := Query{View: "critical_blocking_calls", PID: 900, TimeStart: 50, TimeEnd: 53}
	without := Run(hmc376FaceIndex(t, hmc376BusyWindowSpans(false)), query)
	with := Run(hmc376FaceIndex(t, hmc376BusyWindowSpans(true)), query)
	if without.CriticalBlocking == nil || with.CriticalBlocking == nil {
		t.Fatal("the view must publish its list")
	}
	published := map[string]bool{}
	targetWait := false
	for _, item := range with.CriticalBlocking.Items {
		published[item.Type+"/"+threadLabel(item.Thread)] = true
		targetWait = targetWait || (item.Thread.PID == 900 && item.ChainRelevance == "on_chain" && item.seatedByVocabularyOnly)
	}
	if !targetWait {
		t.Fatalf("the target's wait must hold an additional seat: %+v", with.CriticalBlocking.Items)
	}
	for _, item := range without.CriticalBlocking.Items {
		if !published[item.Type+"/"+threadLabel(item.Thread)] {
			t.Fatalf("%s %s lost its seat to the target's wait", item.Type, threadLabel(item.Thread))
		}
	}
}

func TestHMC376OnChainVocabularyRowIsAdditionalToTheEvidenceFacts(t *testing.T) {
	var items []CriticalBlockingCandidate
	for i := 0; i < 16; i++ {
		items = append(items, CriticalBlockingCandidate{Summary: fmt.Sprintf("row%d", i)})
	}
	onChain := CriticalBlockingCandidate{Summary: "target wait", ChainRelevance: "on_chain", seatedByVocabularyOnly: true}
	facts := evidenceFromCriticalBlocking(CriticalBlockingResult{Items: append([]CriticalBlockingCandidate{onChain}, items...)})
	if len(facts) != 17 || facts[16].Summary != "row15" {
		t.Fatalf("the last row published without the vocabulary must keep its fact, got %d facts", len(facts))
	}
	offChain := onChain
	offChain.ChainRelevance = "background"
	if facts := evidenceFromCriticalBlocking(CriticalBlockingResult{Items: append(items, offChain)}); len(facts) != 16 {
		t.Fatalf("an off-chain row is not additional, got %d facts", len(facts))
	}
}

// A name an operator pattern also classifies is published as classified work on
// every later face, exactly as it is at the seat bound.
func TestHMC376OperatorClassifiedVocabularySpanKeepsItsBoardRow(t *testing.T) {
	const wait = "GC: Wait For Completion Alloc"
	spans := []hmc376FaceSpan{{"opwait", 1100, wait, 300000}}
	for i := 0; i < 15; i++ {
		spans = append(spans, hmc376FaceSpan{fmt.Sprintf("ver%d", i), 1300 + i, fmt.Sprintf("VerifyClass com.example.Lock%d", i), 40000 - i*500})
	}
	onBoard := func() bool {
		res := Run(hmc376FaceIndex(t, spans), Query{View: "root_cause_rank", TimeStart: 50, TimeEnd: 53})
		if res.RootCauseRank == nil {
			t.Fatal("the view must publish its board")
		}
		for _, item := range res.RootCauseRank.Items {
			if item.Thread.PID == 1100 {
				return true
			}
		}
		return false
	}
	if !semanticSpanMintedByPlatformVocabulary(wait) || onBoard() {
		t.Fatal("control: without a pattern the off-chain vocabulary row yields its board seat")
	}
	SetSemanticSpanPatterns([]SemanticSpanPattern{{SemanticClass: "gc_pause", Contains: []string{"wait for completion"}}})
	t.Cleanup(func() { SetSemanticSpanPatterns(nil) })
	if semanticSpanMintedByPlatformVocabulary(wait) {
		t.Fatal("a name an operator pattern classifies is not classified by the vocabulary alone")
	}
	if !onBoard() {
		t.Fatal("the operator-classified row must keep its board row")
	}
}

func TestHMC376CategoryListSeatsVocabularyOnlyCategoriesLast(t *testing.T) {
	var spans []TraceSpanSummary
	for i := 0; i < 7; i++ {
		spans = append(spans, TraceSpanSummary{Name: fmt.Sprintf("work%d", i), Category: fmt.Sprintf("cat%d", i), Subcategory: "x", DurationMs: float64(100 - i), StartLine: i + 1})
	}
	spans = append(spans,
		TraceSpanSummary{Name: "VerifyClass a", Category: "runtime_verification", Subcategory: "class_verification", DurationMs: 1, StartLine: 20},
		TraceSpanSummary{Name: "GC: Wait For Completion Alloc", Category: "garbage_collection", Subcategory: "gc_pause", DurationMs: 500, StartLine: 21, seatedByVocabularyOnly: true})
	got := computeTraceMarkCategories(spans, 8)
	if len(got) != 8 {
		t.Fatalf("want 8 categories, got %+v", got)
	}
	for _, item := range got {
		if item.Category == "garbage_collection" {
			t.Fatalf("a category of vocabulary seats alone took a seat: %+v", got)
		}
	}
	if got[0].Category != "cat0" || got[7].Category != "runtime_verification" {
		t.Fatalf("seated categories keep their published order: %+v", got)
	}
	// A free seat is filled, and the row sorts by the time it published.
	got = computeTraceMarkCategories(spans, 9)
	if len(got) != 9 || got[0].Category != "garbage_collection" || got[0].TotalMs != 500 {
		t.Fatalf("a category takes a seat nobody else needs: %+v", got)
	}
}

func TestHMC376WindowEvidenceSeatsVocabularyOnlySpansLast(t *testing.T) {
	var stats WindowStats
	for i := 0; i < 8; i++ {
		stats.TraceSpans = append(stats.TraceSpans, TraceSpanSummary{Name: fmt.Sprintf("vocabulary%d", i), DurationMs: 500, seatedByVocabularyOnly: true})
	}
	for i := 0; i < 28; i++ {
		stats.TraceSpans = append(stats.TraceSpans, TraceSpanSummary{Name: fmt.Sprintf("published%02d", i), DurationMs: 10})
	}
	var text strings.Builder
	for _, fact := range evidenceFromStats(stats) {
		text.WriteString(fact.Summary + "\n")
	}
	if strings.Contains(text.String(), "vocabulary") || !strings.Contains(text.String(), "published27") {
		t.Fatalf("spans published without the vocabulary must fill the facts first:\n%s", text.String())
	}
}
