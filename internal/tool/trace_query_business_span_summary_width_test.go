package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBusinessSpanSchedulerSummaryWidthPublicBlob(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{strings.Repeat("MarkerAlpha", 2000), strings.Repeat("MarkerBeta", 2000)}
	trace := strings.NewReplacer("OpenDocument", names[0], "LoadDocumentIndex", names[1]).Replace(string(fixture))
	ctx, path := businessRefTestContext(t, trace)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": 1, "time_end": 1.051})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if !strings.HasSuffix(result.RawRef, ".txt") {
		t.Fatalf("fixture did not exercise the public StoreBlob preview: raw_ref=%q", result.RawRef)
	}
	raw, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= MaxInlineBytes || string(raw) == result.Summary {
		t.Fatal("expected actual head/tail offload at the unchanged blob budget")
	}
	for _, want := range []string{
		fmt.Sprintf("marker_state_account %q owner=app-main-100 interval=1.000000..1.050000 running=5.000ms runnable=1.000ms sleep=44.000ms", sanitizeForBanner(names[0])),
		fmt.Sprintf("marker_state_account %q owner=document-worker-200 interval=1.004500..1.044500 running=8.000ms runnable=1.000ms sleep=31.000ms", sanitizeForBanner(names[1])),
		"root_cause_rank_preview status=", "root_cause_rank_preview_row board_order=1 ",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("public StoreBlob preview lost %q (raw=%d bytes preview=%d bytes)", want, len(raw), len(result.Summary))
		}
	}
	for _, name := range names {
		foundNative, foundTyped := false, false
		for _, span := range payload.WindowStats.TraceSpans {
			if span.Name == name {
				foundNative = span.SchedulerStates.Matches(span.SourcePath, traceThreadLabel(span.Thread), span.StartTs, span.EndTs)
			}
		}
		for _, record := range result.Observations {
			if record.Predicate != types.TraceBusinessSpanPredicate || record.Object != name || record.ClaimKey != types.TraceBusinessSpanPredicate+":"+name {
				continue
			}
			for _, note := range record.RichNotes {
				if note == types.TraceNoteKeySpanName+"="+name {
					foundTyped = true
				}
			}
		}
		if !foundNative || !foundTyped {
			t.Errorf("display clipping changed the full marker identity: native=%v typed=%v name_bytes=%d", foundNative, foundTyped, len(name))
		}
	}
}

func TestBusinessSpanSchedulerSummaryWidthDoesNotWeakenIdentity(t *testing.T) {
	owner := tracequery.ThreadRef{PID: 100, Comm: strings.Repeat("w", toolBannerMaxValueLen)}
	source := "/capture/" + strings.Repeat("s", 500) + ".systrace"
	span := tracequery.TraceSpanSummary{
		Name: "marker\n\t" + strings.Repeat("业务标记", 100), Kind: "sync", Thread: owner,
		SourcePath: source, StartTs: 1, EndTs: 1.05, StartLine: 2, EndLine: 7,
		SchedulerStates: &tracequery.TraceSpanSchedulerStates{
			SourcePath: source, Thread: owner, Window: tracequery.TimeWindow{StartTs: 1, EndTs: 1.05}, Coverage: "unavailable",
		},
	}
	before, err := json.Marshal(span)
	if err != nil {
		t.Fatal(err)
	}
	var preview strings.Builder
	writeTraceBusinessSpanSchedulerPreview(&preview, []tracequery.TraceSpanSummary{span}, "payload.json")
	want := fmt.Sprintf("marker_state_account %q owner=%s interval=", sanitizeForBanner(span.Name), sanitizeForBanner(traceThreadLabel(owner)))
	if !strings.Contains(preview.String(), want) || !strings.Contains(preview.String(), "source="+traceQuerySourceBasename(source)+" lines=2-7") ||
		!strings.Contains(preview.String(), "unavailable, not zero") || strings.Count(preview.String(), "\n") != 1 || !utf8.ValidString(preview.String()) {
		t.Errorf("display values were not bounded single-line UTF-8: %q", preview.String())
	}
	if len(preview.String()) > 3*toolBannerMaxValueLen+300 {
		t.Errorf("marker/owner/source exceeded existing per-value budget: %d bytes", len(preview.String()))
	}
	var note tracequery.TraceSpanSchedulerStates
	if err := json.Unmarshal([]byte(traceQueryBusinessSpanSchedulerNote(span)), &note); err != nil || note.Thread != owner || note.SourcePath != source {
		t.Fatalf("display clipping changed the full typed scheduler note: %v", err)
	}
	after, _ := json.Marshal(span)
	if string(before) != string(after) {
		t.Fatal("preview changed native evidence")
	}
	// These two owners have the same bounded display but different full IDs.
	// The match must still reject the other owner before display sanitation.
	span.Thread.PID++
	if sanitizeForBanner(traceThreadLabel(span.Thread)) != sanitizeForBanner(traceThreadLabel(owner)) {
		t.Fatal("fixture does not exercise a display collision")
	}
	if got := traceQueryBusinessSpanSchedulerSummary(span); got != "" {
		t.Fatal("truncated display identity authorized a foreign scheduler account")
	}
}
