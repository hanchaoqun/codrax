package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Q5-A P1-2 ② / P2-2: an evidence item whose Source is a trace_query BLOB
// named by basename (e.g. "trace_query-ab12cd34.txt") must soft-route into
// the external-observation lane, exactly like the full ".codrax/blob/..."
// spelling already does. Without the shared blob-registry consult, the
// bare basename slips RuntimeArtifactPathKind's extension shapes and would
// be sent to the current-source grounder. This shares the same matcher
// builtin.go / extractor.go / ground.go use — one signal, four surfaces.
func TestEmitEvidence_TraceQueryBlobBasenameStaysExternalObservation(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Register a real blob ref (basename "trace_query-ab12cd34.txt").
	ctx.Mutable.RegisterTraceQueryBlobRef("/anchor/.codrax/blob/20260704-1/trace_query-ab12cd34.txt")

	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "android.haitong-56023",
				"source": "trace_query-ab12cd34.txt",
				"line_start": 42,
				"summary": "trace_query identified the on-chain runnable blocker",
				"anchor_kind": "text_reference",
				"anchor_symbol": "android.haitong-56023"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := ctx.Mutable.EmittedEvidence(); len(got) != 0 {
		t.Fatalf("blob-basename rows must not enter the current-source evidence buffer: %+v", got)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("blob-basename source should publish the external-observation typed repair, got %+v", res.Repair)
	}
	if strings.Contains(res.Summary, "→ ungrounded") || strings.Contains(res.Summary, "Current actionable repair targets") {
		t.Fatalf("blob-basename rows must not render as ungrounded source evidence:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "external observation") &&
		(res.Repair == nil || !strings.Contains(res.Repair.Hint, "external observation")) {
		t.Fatalf("blob-basename item should be described as an external observation; summary=%s repair=%+v", res.Summary, res.Repair)
	}
}

// Control: an UNREGISTERED basename that merely looks like a source file
// keeps flowing through the ordinary grounding path (no over-broad skip).
func TestEmitEvidence_UnregisteredBasenameNotBlobLane(t *testing.T) {
	if got := emitEvidenceExternalObservationSourceKindForContext(newEmitCtx(), "helper.go"); got != "" {
		t.Fatalf("an ordinary .go source must not be classed as external observation, got %q", got)
	}
}
