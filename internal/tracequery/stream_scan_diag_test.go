package tracequery

// stream_scan_diag_test.go — TDIAG engine-half pins (DIAG batch, §28.13,
// real_trace_campaign_20260705.md, 2026-07-09).
//
// B1 CanonicalViewNames: the exported enumerator IS the capacity table (one
//    source — a rename/removal in the table moves the list in the same edit).
// B2 StreamScan: zero-second-parser parity with the indexed build (same
//    events, same order, same quality counters), early stop, and the
//    budget-freedom witness (a MaxEvents build denies while StreamScan
//    streams everything). 自设最强突变形: a StreamScan that silently
//    truncates at a budget reddens the budget-freedom pin.
// B3 semantic exports: thin wrappers agree with the engine classifier.
// B4 Index.UnparsedSamples: typed samples with the 帽 5 cap, rune-safe
//    byte-capped text, collected on windowed builds too.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- B1 ----------------------------------------------------------------------

func TestCanonicalViewNamesIsTheCapacityTable(t *testing.T) {
	names := CanonicalViewNames()
	want := make([]string, 0, len(viewCapacityTable))
	for name := range viewCapacityTable {
		want = append(want, name)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("exported enumerator must be the capacity table:\n got %v\nwant %v", names, want)
	}
	for _, name := range names {
		if CanonicalViewName(name) != name {
			t.Errorf("table key %q is not canonical", name)
		}
		capacity := ViewCapacityFor(name)
		if !capacity.HeavyView && capacity.DefaultLimit <= 0 {
			t.Errorf("view %q resolves to a zero capacity row", name)
		}
	}
	// Spot pins: the streaming channels and the rank views must be present —
	// an accidental table gutting cannot pass on shape alone.
	got := strings.Join(names, ",")
	for _, must := range []string{"event_search", "window_sweep", "root_cause_rank", "critical_blocking_calls", "evidence_pack"} {
		if !strings.Contains(","+got+",", ","+must+",") {
			t.Errorf("canonical view list lost %q: %v", must, names)
		}
	}
}

// --- B2 / B4 fixtures ----------------------------------------------------------

func diagScanTraceText() string {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "      worker-10   (   10) [000] .... 1.%06d: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=10 next_prio=%d\n", 10000+i*1000, 20+i)
	}
	b.WriteString("garbage line alpha\n")
	b.WriteString("垃圾行——中文不可解析样本" + strings.Repeat("宽", 200) + "\n") // >480 bytes, CJK
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "unparsed tail %d\n", i)
	}
	return b.String()
}

func writeDiagScanTrace(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan.systrace")
	if err := os.WriteFile(path, []byte(diagScanTraceText()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- B2 ----------------------------------------------------------------------

func TestStreamScanParityWithIndexedBuild(t *testing.T) {
	path := writeDiagScanTrace(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var streamed []Event
	shell, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(ev Event) bool {
		streamed = append(streamed, ev)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed) != len(idx.Events) {
		t.Fatalf("event parity broken: streamed=%d indexed=%d", len(streamed), len(idx.Events))
	}
	for i := range streamed {
		if streamed[i].Line != idx.Events[i].Line || streamed[i].Ts != idx.Events[i].Ts || streamed[i].Type != idx.Events[i].Type {
			t.Fatalf("event %d diverges: stream=%+v index=%+v", i, streamed[i], idx.Events[i])
		}
	}
	if shell.Events != nil {
		t.Fatalf("the shell must accumulate nothing (Events nil), got %d", len(shell.Events))
	}
	// Quality-counter parity: the census keys its honesty lines on these.
	if shell.UnparsedLines != idx.UnparsedLines || shell.ParseLinePanics != idx.ParseLinePanics ||
		shell.ParsedKnown != idx.ParsedKnown || shell.LineCount != idx.LineCount ||
		shell.FirstTs != idx.FirstTs || shell.LastTs != idx.LastTs {
		t.Fatalf("quality counters diverge:\nshell %+v\nindex unparsed=%d panics=%d known=%d lines=%d first=%v last=%v",
			shell, idx.UnparsedLines, idx.ParseLinePanics, idx.ParsedKnown, idx.LineCount, idx.FirstTs, idx.LastTs)
	}
	// Typed sample parity (B4): both paths retain the same first-cap samples.
	if !reflect.DeepEqual(shell.UnparsedSamples, idx.UnparsedSamples) {
		t.Fatalf("typed samples diverge:\nshell %+v\nindex %+v", shell.UnparsedSamples, idx.UnparsedSamples)
	}
}

func TestStreamScanEarlyStop(t *testing.T) {
	path := writeDiagScanTrace(t)
	seen := 0
	_, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(Event) bool {
		seen++
		return seen < 3
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != 3 {
		t.Fatalf("fn returning false must stop the scan: saw %d events", seen)
	}
}

// TestStreamScanNotSubjectToIndexBudget is the budget-freedom witness (自设
// 最强突变形): the SAME trace denies an indexed build under a tiny MaxEvents
// budget, while StreamScan streams every event.
func TestStreamScanNotSubjectToIndexBudget(t *testing.T) {
	path := writeDiagScanTrace(t)
	_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{MaxEvents: 3})
	var limitErr *IndexEventLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("fixture must trip the index budget, got %v", err)
	}
	streamed := 0
	if _, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(Event) bool {
		streamed++
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if streamed != 12 {
		t.Fatalf("StreamScan must stream past the index budget: saw %d of 12", streamed)
	}
}

// --- B4 ----------------------------------------------------------------------

func TestIndexUnparsedSamplesTypedFace(t *testing.T) {
	path := writeDiagScanTrace(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.UnparsedSamples) != IndexUnparsedSampleCap {
		t.Fatalf("samples must cap at %d, got %d", IndexUnparsedSampleCap, len(idx.UnparsedSamples))
	}
	first := idx.UnparsedSamples[0]
	if first.Line != 13 || first.Text != "garbage line alpha" {
		t.Fatalf("first sample must be the first unparsed line verbatim: %+v", first)
	}
	// The 2MiB-class pathological line class: rune-safe byte cap, never a
	// mid-rune cut (G18 family). The cap sits deliberately above the
	// tracediag render clamp (480) so a cut sample still trips the render
	// marker.
	cjk := idx.UnparsedSamples[1]
	if len(cjk.Text) > indexUnparsedSampleTextBytes {
		t.Fatalf("sample text must be byte-capped: %d bytes", len(cjk.Text))
	}
	if len(cjk.Text) <= 480 {
		t.Fatalf("the capped CJK fixture must exceed the render clamp (parse cap > render cap): %d bytes", len(cjk.Text))
	}
	if !utf8.ValidString(cjk.Text) {
		t.Fatalf("sample truncation split a rune: %q", cjk.Text)
	}
	if !strings.HasPrefix(cjk.Text, "垃圾行——中文不可解析样本") {
		t.Fatalf("sample must keep the head of the line: %q", cjk.Text)
	}
	// The count stays the full authority beyond the sample cap.
	if idx.UnparsedLines != 8 {
		t.Fatalf("unparsed count must stay authoritative: %d", idx.UnparsedLines)
	}
}

// TestIndexUnparsedSamplesOnWindowedBuild: the old census second read had to
// honestly SKIP windowed indexes; the parse-site face collects there too.
func TestIndexUnparsedSamplesOnWindowedBuild(t *testing.T) {
	path := writeDiagScanTrace(t)
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 14, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatalf("fixture must produce a windowed index")
	}
	if len(idx.UnparsedSamples) == 0 {
		t.Fatalf("windowed builds must retain typed samples too")
	}
	if idx.UnparsedSamples[0].Line != 13 {
		t.Fatalf("windowed sample coordinates must be real line numbers: %+v", idx.UnparsedSamples[0])
	}
}

// --- B3 ----------------------------------------------------------------------

func TestSemanticExportsAgreeWithEngineClassifier(t *testing.T) {
	if got := TraceSpanSemanticClass("H:Texture upload(200 KiB)"); got != "texture_upload" {
		t.Fatalf("texture upload span must classify, got %q", got)
	}
	if got := TraceSpanSemanticClass("VerifyClass com.example.Foo"); got != "class_verification" {
		t.Fatalf("VerifyClass span must classify, got %q", got)
	}
	if got := TraceSpanSemanticClass("Choreographer#doFrame"); got != "" {
		t.Fatalf("plain frame span must not classify, got %q", got)
	}
	// Near miss: semantic vocabulary without a known pattern — and never both
	// (a classified span is not a near miss on the census face).
	if !TraceSpanNearMissesSemanticWork("GLES texture upload path") {
		t.Fatalf("texture+upload vocabulary off-pattern must near-miss")
	}
	if TraceSpanSemanticClass("GLES texture upload path") != "" {
		t.Fatalf("near-miss witness must stay unclassified")
	}
	if TraceSpanNearMissesSemanticWork("Choreographer#doFrame") {
		t.Fatalf("plain span must not near-miss")
	}
}
