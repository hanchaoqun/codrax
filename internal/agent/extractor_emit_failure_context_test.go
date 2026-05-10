// Package agent — extractor_emit_failure_context_test.go (2026-05-10).
//
// Fix C of the post-sweep optimization: when the extractor soft-
// stops AFTER a failed emit_X call earlier in the same dispatch,
// the soft-stop hint should prepend the rejection reason so the
// LLM addresses the rejection AND the missing emit in one
// response. Without this, the 2026-05-10 sweep digest s7b
// dispatch-2 pattern recurs — generic "you stopped without
// emitting" leaves the LLM re-emitting the same wrong shape.
package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLatestExtractorEmitFailureContext_NoFailures_Empty(t *testing.T) {
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "found 12 hits"},
		{ToolName: "emit_evidence", Success: true, Summary: "ok"},
	})
	if got != "" {
		t.Errorf("no failed emit_* results: expected empty prefix; got %q", got)
	}
}

func TestLatestExtractorEmitFailureContext_NonEmitFailureIgnored(t *testing.T) {
	// A failed read_file or grep is not an emit-class failure;
	// shouldn't trigger the prefix.
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{ToolName: "read_file", Success: false, Summary: "no such file"},
	})
	if got != "" {
		t.Errorf("non-emit failure: expected empty prefix; got %q", got)
	}
}

func TestLatestExtractorEmitFailureContext_ReturnsLatestEmitFailure(t *testing.T) {
	// Two failed emit_* calls — the function must return the
	// LATEST (closest to the LLM's current state).
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{ToolName: "emit_answer_document", Success: false, Summary: "first failure"},
		{ToolName: "grep", Success: true, Summary: "ok"},
		{ToolName: "emit_answer_symbol", Success: false, Summary: "second failure detail"},
	})
	if !strings.Contains(got, "emit_answer_symbol") {
		t.Errorf("expected latest emit failure (emit_answer_symbol); got %q", got)
	}
	if !strings.Contains(got, "second failure detail") {
		t.Errorf("expected latest summary text; got %q", got)
	}
	if strings.Contains(got, "first failure") {
		t.Errorf("must return only the latest, not earlier failures; got %q", got)
	}
}

func TestLatestExtractorEmitFailureContext_FirstLineOnly(t *testing.T) {
	// emit_answer_symbol rejection summaries can be multi-line; the
	// prefix must take only the first line so the hint stays
	// readable.
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{
			ToolName: "emit_answer_symbol",
			Success:  false,
			Summary:  "items[0] (25): name \"25\" is not corroborated\nADDITIONAL VERBOSE\nMORE LINES HERE",
		},
	})
	if strings.Contains(got, "ADDITIONAL VERBOSE") {
		t.Errorf("multi-line summary leaked beyond first line; got %q", got)
	}
	if !strings.Contains(got, "name \"25\" is not corroborated") {
		t.Errorf("first line missing from prefix; got %q", got)
	}
}

func TestLatestExtractorEmitFailureContext_LongSummaryTruncated(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{ToolName: "emit_answer_symbol", Success: false, Summary: long},
	})
	if !strings.Contains(got, "…") {
		t.Errorf("long summary should be truncated with ellipsis; got %q", got)
	}
	// The 240-char cap plus ellipsis plus surrounding prose:
	// keep total < 400 so the hint body remains the dominant content.
	if len(got) > 400 {
		t.Errorf("prefix too long after truncation: len=%d, want < 400", len(got))
	}
}

func TestLatestExtractorEmitFailureContext_PrefixIsActionable(t *testing.T) {
	// The prefix should explicitly tell the LLM to address the
	// rejection — generic "your last call failed" without a verb
	// makes the LLM re-emit the same shape.
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{
			ToolName: "emit_answer_symbol",
			Success:  false,
			Summary:  "items[0] (25): name not corroborated",
		},
	})
	if !strings.Contains(got, "Address that rejection") {
		t.Errorf("prefix must instruct LLM to address the rejection; got %q", got)
	}
}

func TestLatestExtractorEmitFailureContext_R6Audit(t *testing.T) {
	// The prefix is LLM-facing; must not leak internal pipeline
	// vocabulary.
	got := latestExtractorEmitFailureContext([]types.ToolResult{
		{ToolName: "emit_answer_symbol", Success: false, Summary: "rejection text"},
	})
	for _, banned := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"BusContext", "MutableState", "AnswerDocumentV2", "AnswerBlock",
		"L1", "L2", "L3", "L4",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("internal term %q leaked into prefix: %q", banned, got)
		}
	}
}
