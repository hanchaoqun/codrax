package context

import (
	stdcontext "context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceExcerptPromptPreservesLocalGuttersAndNeverWritesBlob(t *testing.T) {
	parent := "header\nfirst\nsecond\nother\n"
	v, err := attachment.NewTraceExcerpt(stdcontext.Background(), parent, nil, 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	got := formatAttachedTrace(parent, dir, attachedTriageProducer, "", attachedTraceRenderOptions{Excerpt: v})
	for _, want := range []string{"fragment", "lines 2-3", "not physical source-file lines", "1│ first", "2│ second", "fragment-local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "│other") || strings.Contains(got, "│header") {
		t.Fatal("leaked other segment")
	}
	if _, err := os.Stat(filepath.Join(dir, AttachedTraceBlobName)); !os.IsNotExist(err) {
		t.Fatalf("wrote blob: %v", err)
	}
	got = formatAttachedTrace(parent+"changed", dir, attachedTriageProducer, "", attachedTraceRenderOptions{Excerpt: v})
	if !strings.Contains(got, "Reattach") || strings.Contains(got, "```text") {
		t.Fatalf("stale view exposed: %s", got)
	}
}

func TestPerfExtractionCoverageAndScopeReachStructuredPrompt(t *testing.T) {
	bundle := &types.PerfBundle{Coverage: 1, ExtractionCoverage: &types.PerfExtractionCoverage{ParentPreviewBytes: 1000, Segments: 5, Attempted: 2, Succeeded: 1, Failed: 1, Skipped: 1, Unattempted: 2, ExtractedPreviewBytes: 100}, Observations: []types.PerfObservation{{Authority: types.PerfObservationAuthorityDeterministicValidator, Subject: "time", SourceScope: &types.PerfObservationSourceScope{ByteStart: 100, ByteEnd: 200, LineStart: 5, LineEnd: 9, LineCoordinates: "parent_preview"}, LineStart: 5, LineEnd: 9}}}
	got := formatPerfTriageStructured(bundle, nil)
	for _, want := range []string{"100/1000 preview bytes", "1 failed, 1 skipped, 2 unattempted", "not physical source-file lines", "neither establishes full-source scan completeness"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
}
