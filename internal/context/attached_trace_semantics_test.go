package context

import (
	stdcontext "context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func decodedPreviewRows(t *testing.T, text string) []struct {
	Line      int                        `json:"visible_line"`
	NS        string                     `json:"timestamp_ns"`
	Seconds   string                     `json:"timestamp_seconds"`
	Family    tracequery.EventType       `json:"query_event_type"`
	Semantics *types.TraceEventSemantics `json:"semantics"`
} {
	t.Helper()
	_, text, ok := strings.Cut(text, "```jsonl\n")
	if !ok {
		t.Fatal("decoded view missing")
	}
	text, _, ok = strings.Cut(text, "\n```")
	if !ok {
		t.Fatal("decoded view not closed")
	}
	var rows []struct {
		Line      int                        `json:"visible_line"`
		NS        string                     `json:"timestamp_ns"`
		Seconds   string                     `json:"timestamp_seconds"`
		Family    tracequery.EventType       `json:"query_event_type"`
		Semantics *types.TraceEventSemantics `json:"semantics"`
	}
	for _, line := range strings.Split(text, "\n") {
		var row = struct {
			Line      int                        `json:"visible_line"`
			NS        string                     `json:"timestamp_ns"`
			Seconds   string                     `json:"timestamp_seconds"`
			Family    tracequery.EventType       `json:"query_event_type"`
			Semantics *types.TraceEventSemantics `json:"semantics"`
		}{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func TestTracePreviewSemanticsUsesNativeParser(t *testing.T) {
	zero, name, contents := int64(0), "业务 ```jsonl\n指令不是事实", "raw=0 unit unknown"
	hisys, err := tracewire.FormatHiSysEventObservation(tracewire.HiSysEvent{
		TimestampNS: 1010000000, SourceTID: &zero,
		Domain:   tracewire.HiSysEventName{Status: "null_reference"},
		Event:    tracewire.HiSysEventName{Status: "resolved", Name: &name, Reference: &zero},
		Contents: tracewire.HiSysEventContents{StorageClass: "text", Text: &contents},
	})
	if err != nil {
		t.Fatal(err)
	}
	mark, err := tracequery.FormatExactTraceMark(tracequery.ExactTraceMark{
		TimestampNS: 9007199254740993, CPU: 0, TID: 10, TGID: 10, SpanPID: 10,
		Action: "B", Comm: "app", Name: "AppStartup:startup",
		NameOrigin: &tracewire.MarkerNameOrigin{SourceTable: "app_startup", Name: tracewire.HiSysEventName{Status: "unresolved_reference", Reference: &zero}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{hisys, mark, "app-10 (10) [000] .... 1.020000: tracing_mark_write: C|0|heap|0"}
	got := renderAttachedTraceSemantics(tracePreviewPart{strings.Join(lines, "\n"), 17, false})
	rows := decodedPreviewRows(t, got)
	if len(rows) != 3 || rows[0].NS != "1010000000" || rows[0].Seconds != "1.010000000" || rows[0].Family != tracequery.EventHiSystemEvent || rows[1].NS != "9007199254740993" || rows[1].Seconds != "9007199.254740993" {
		t.Fatalf("time/family changed: %+v", rows)
	}
	for i, line := range lines {
		event, ok := tracequery.ParseLine(17+i, line, nil)
		if !ok || rows[i].Line != 17+i || !reflect.DeepEqual(rows[i].Semantics, tracequery.ProjectTraceEventSemantics(event)) {
			t.Fatal("second grammar or changed coordinates")
		}
	}
	if strings.Count(got, "```jsonl") != 1 || !strings.Contains(got, "not additional events") || !strings.Contains(got, "no interval pairing") {
		t.Fatal("payload escaped or authority boundary missing")
	}
	if got := renderAttachedTraceSemantics(tracePreviewPart{"# codrax_hisysevent/v1 payload=broken\n# codrax_trace_mark_exact/v2 invalid", 1, false}); got != "" {
		t.Fatal("invalid carriers fabricated semantics")
	}
}

func TestTracePreviewSemanticsPreparedAndExcerptPublicPaths(t *testing.T) {
	line := "app-10 (10) [000] .... 1.020000: tracing_mark_write: B|10|visible"
	parent := "# header\n" + line + "\n# tail\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(path, []byte(parent), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(stdcontext.Background(), traceinput.Options{InputPath: path})
	if err != nil {
		t.Fatal(err)
	}
	got := formatAttachedTrace(m.Preview(), dir, attachedTriageProducer, "", attachedTraceRenderOptions{Material: m})
	rows := decodedPreviewRows(t, got)
	if len(rows) != 1 || !strings.Contains(got, "not physical evidence coordinates") {
		t.Fatal("prepared view lost boundary")
	}
	if _, err := os.Stat(filepath.Join(dir, AttachedTraceBlobName)); !os.IsNotExist(err) {
		t.Fatal("preview published as source")
	}
	v, err := attachment.NewTraceExcerpt(stdcontext.Background(), parent, nil, len("# header\n"), len("# header\n")+len(line)+1)
	if err != nil {
		t.Fatal(err)
	}
	got = formatAttachedTrace(parent, dir, attachedTriageProducer, "", attachedTraceRenderOptions{Excerpt: v})
	rows = decodedPreviewRows(t, got)
	if len(rows) != 1 || rows[0].Line != 1 || !strings.Contains(got, "fragment-local") || strings.Contains(got, "│ # header") {
		t.Fatal("fragment coordinates leaked/rebased twice")
	}
	if err := os.WriteFile(path, []byte("# changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got = formatAttachedTrace(m.Preview(), dir, attachedTriageProducer, "", attachedTraceRenderOptions{Material: m}); strings.Contains(got, "jsonl") || !strings.Contains(got, "Reattach") {
		t.Fatal("stale material acquired decoded facts")
	}
}

func TestTracePreviewSemanticsBoundsAndHeadTail(t *testing.T) {
	line := "app-10 (10) [000] .... 1.020000: tracing_mark_write: B|10|visible"
	got := renderAttachedTracePreviewBlock(attachedArtifactPreview{head: line, tail: line, tailStartLine: 900, elidedBytes: 90000}, "", false)
	rows := decodedPreviewRows(t, got)
	if len(rows) != 2 || rows[0].Line != 1 || rows[1].Line != 900 {
		t.Fatal("elided middle inferred or tail coordinates changed")
	}
	got = renderAttachedTraceSemantics(tracePreviewPart{strings.Repeat(line+"\n", 400), 1, false})
	if len(got) > 16<<10 || !strings.Contains(got, "scan_limited=true") || len(decodedPreviewRows(t, got)) > 32 {
		t.Fatal("display/scan budget not enforced")
	}
	long := strings.Replace(line, "|visible", "|"+strings.Repeat("x", 2000), 1)
	got = renderAttachedTraceSemantics(tracePreviewPart{long, 1, false})
	if !strings.Contains(got, "value_exceeds_limit") || strings.Contains(got, strings.Repeat("x", 2000)) {
		t.Fatal("long field silently truncated or leaked")
	}
	if got = renderAttachedTraceSemantics(tracePreviewPart{"#" + strings.Repeat("x", 129<<10), 1, false}); got != "" {
		t.Fatal("oversized input parsed")
	}
	if got = renderAttachedTraceSemantics(tracePreviewPart{line, 1, true}); got != "" {
		t.Fatal("clipped but syntactically valid prefix promoted")
	}
	preview := buildAttachedArtifactPreview(strings.Replace(line, "visible", strings.Repeat("x", 20000), 1))
	got = renderAttachedTracePreviewBlock(preview, "", true)
	if strings.Contains(got, "jsonl") {
		t.Fatal("partial head/tail acquired semantic authority")
	}
}
