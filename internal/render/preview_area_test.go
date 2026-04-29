package render

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderer_PreviewClear_NoOpWhenNoArea: the defensive Clear
// path the orchestrator emits at every Run exit must not panic /
// emit ghost output when no preview Area was ever opened (e.g.
// pipeline failed before finalize streamed).
func TestRenderer_PreviewClear_NoOpWhenNoArea(t *testing.T) {
	r := New(nil, false)
	r.handlePreviewClearLocked(Event{
		Kind:            EventLivePreviewClear,
		Stage:           types.StageFinalize,
		PreviewRejected: true,
	})
	// Just verify the handler returned cleanly; if previewArea
	// happens to be non-nil here something is wrong.
	if r.previewArea != nil {
		t.Errorf("previewArea must remain nil when Clear runs against empty state")
	}
}

// TestRenderer_PreviewChunk_NonTTYShortCircuit: when stdout is not
// a terminal, EventLivePreviewChunk must NOT open a pterm Area —
// it would otherwise leak preview text into the user's piped output
// (`codrax --request ... > file.txt`) corrupting the file. The
// renderer's previewIsTTY() short-circuit guards this. Test by
// invoking the handler directly with a Renderer constructed for
// non-TTY (forceColor=false on a test process where stdout is not a
// terminal — go test's stdout is a pipe).
func TestRenderer_PreviewChunk_NonTTYShortCircuit(t *testing.T) {
	r := New(nil, false)
	// In `go test` stdout is a pipe; previewIsTTY() returns false.
	r.handlePreviewChunkLocked(Event{
		Kind:        EventLivePreviewChunk,
		Stage:       types.StageFinalize,
		PreviewText: "hello world",
	})
	if r.previewArea != nil {
		t.Errorf("non-TTY path must NOT open a preview Area; got %v", r.previewArea)
	}
	if r.previewRound != 0 {
		t.Errorf("non-TTY path must not increment previewRound; got %d", r.previewRound)
	}
}

