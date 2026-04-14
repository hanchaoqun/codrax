package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEnsureStructuredEvidence_MergesEmittedAndParsed pins the P1.1
// invariant: when the explorer's BusContext.Mutable carries items
// produced by the emit_evidence tool, ensureStructuredEvidence merges
// them with the markdown-parsed channel via mergeEvidenceItems so a
// single fact reported through both channels collapses to one item by
// StableEvidenceID, and items reported through only one channel
// survive verbatim.
func TestEnsureStructuredEvidence_MergesEmittedAndParsed(t *testing.T) {
	// Markdown-only fact: only the parsed channel sees it.
	// Tool-only fact: only the emitted buffer carries it.
	// Overlapping fact: both channels report identical content; must dedup.
	// Note shapes intentionally omit both the post-line colon and the
	// `line N` clause:
	//   - omitting the colon keeps parseEvidenceLine's object empty
	//     so the markdown and tool-channel shapes converge.
	//   - omitting `line N` keeps LineStart=0 so groundEvidenceItems'
	//     "skip when LineStart == 0" guard fires. Without that guard
	//     the grounder would notice the unit test has no read_file
	//     history and tag both items "/ungrounded", which (a) mutates
	//     Producer and (b) zeroes LineStart and rebuilds the ID — the
	//     test would still demonstrate dedup, but the canonical-ID
	//     assertion below would have to be stale-aware. Easier to
	//     just exercise the "no line cited" path, which is realistic:
	//     emit_evidence permits items without a line, and the
	//     finalizer's UNGROUNDED tag exists precisely so those still
	//     reach the synthesis stage.
	notes := []string{
		"## Evidence from [internal/agent/foo.go]\n" +
			"- [DIRECT] `MarkdownOnly`\n" +
			"- [DIRECT] `Shared`",
	}

	overlapID := types.StableEvidenceID(
		types.EvidenceDirect, "Shared", "direct", "", "",
		"internal/agent/foo.go", 0, 0,
	)

	toolOnly := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "ToolOnly",
		Predicate:  "direct",
		Source:     "internal/agent/bar.go",
		LineStart:  0,
		LineEnd:    0,
		Confidence: 0.78,
		Producer:   "explorer.emit_evidence",
	}
	toolOnly.ID = types.StableEvidenceID(
		toolOnly.Kind, toolOnly.Subject, toolOnly.Predicate, toolOnly.Object, toolOnly.Condition,
		toolOnly.Source, toolOnly.LineStart, toolOnly.LineEnd,
	)
	overlapping := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "Shared",
		Predicate:  "direct",
		Source:     "internal/agent/foo.go",
		LineStart:  0,
		LineEnd:    0,
		Confidence: 0.78,
		Producer:   "explorer.emit_evidence",
	}
	overlapping.ID = types.StableEvidenceID(
		overlapping.Kind, overlapping.Subject, overlapping.Predicate, overlapping.Object, overlapping.Condition,
		overlapping.Source, overlapping.LineStart, overlapping.LineEnd,
	)
	if overlapping.ID != overlapID {
		t.Fatalf("test setup wrong: overlap IDs do not match — %q vs %q", overlapping.ID, overlapID)
	}

	mut := types.NewMutableState(types.TaskList{})
	mut.AppendEvidence([]types.EvidenceItem{toolOnly, overlapping})

	// Reuse the singleton evaluator state by constructing one directly;
	// ensureStructuredEvidence does not require any other init when
	// there is no graph (the dataflow path is gated on hasGraph).
	e := &explorerEvaluator{
		investigationNotes: notes,
		userQuestion:       "what does Shared return",
	}
	ctx := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Mutable:   mut,
	}
	e.ensureStructuredEvidence(ctx, nil)

	got := e.structuredEvidence
	if len(got) != 3 {
		t.Fatalf("want 3 merged items (markdown-only, tool-only, overlap dedup), got %d:\n%v", len(got), summarizeIDs(got))
	}

	subjects := make(map[string]int)
	for _, it := range got {
		subjects[it.Subject]++
	}
	if subjects["MarkdownOnly"] != 1 {
		t.Errorf("markdown-only fact missing: %v", subjects)
	}
	if subjects["ToolOnly"] != 1 {
		t.Errorf("tool-only fact missing: %v", subjects)
	}
	if subjects["Shared"] != 1 {
		t.Errorf("overlap dedup failed; want 1 Shared, got %d", subjects["Shared"])
	}

	// Verify the overlap survived under one specific ID — proving the
	// dedup key really is StableEvidenceID and not the Producer label.
	var foundOverlap bool
	for _, it := range got {
		if it.ID == overlapID {
			foundOverlap = true
			break
		}
	}
	if !foundOverlap {
		t.Errorf("dedup did not preserve the canonical Shared ID %q", overlapID)
	}
}

// TestEnsureStructuredEvidence_ToolOnlyIsCharted ensures that when
// the markdown channel produces nothing, the emit_evidence buffer
// alone still flows through ensureStructuredEvidence into
// e.structuredEvidence — the explorer must not require both channels.
func TestEnsureStructuredEvidence_ToolOnlyIsCharted(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:       types.EvidenceRegistration,
		Subject:    "Reg",
		Predicate:  "registers",
		Object:     "Foo",
		Source:     "internal/agent/bar.go",
		LineStart:  5,
		LineEnd:    5,
		Confidence: 0.78,
		Producer:   "explorer.emit_evidence",
	}})

	e := &explorerEvaluator{
		investigationNotes: nil,
		userQuestion:       "what does Reg register",
	}
	e.ensureStructuredEvidence(&types.AgentContext{Mutable: mut}, nil)

	if len(e.structuredEvidence) != 1 {
		t.Fatalf("want 1 item, got %d", len(e.structuredEvidence))
	}
	if e.structuredEvidence[0].Subject != "Reg" {
		t.Errorf("subject = %q", e.structuredEvidence[0].Subject)
	}
}

// TestEvidenceToolFlag_DefaultOn pins the default-on contract: the
// structured emit_evidence channel is enabled unless codrax.yaml
// explicitly flips it off. A regression here would silently push
// explorer runs back onto the markdown-only fallback.
func TestEvidenceToolFlag_DefaultOn(t *testing.T) {
	defer SetEvidenceToolMode("on")
	defer SetTwoTurnExplorerMode("on")

	// Isolate the direct flag from the two-turn escalation rule.
	SetTwoTurnExplorerMode("off")

	SetEvidenceToolMode("off")
	if EvidenceToolEnabled() {
		t.Fatal("explicit off should be off")
	}
	SetEvidenceToolMode("ON")
	if !EvidenceToolEnabled() {
		t.Fatal("on (any case) should enable the channel")
	}
	SetEvidenceToolMode("nonsense")
	if !EvidenceToolEnabled() {
		t.Fatal("unrecognised values should collapse to on (default-on)")
	}
	SetEvidenceToolMode("")
	if !EvidenceToolEnabled() {
		t.Fatal("empty string should collapse to on (default-on)")
	}
}

// TestPhase2EvidenceChannelInstructions_TeachesToolWhenEnabled covers
// the prompt-shape contract: when the flag is on, the phase-2 prompt
// fragment must mention emit_evidence and the markdown shape leader;
// when off, it must collapse to a colon so the prompt reads
// "extract ALL relevant facts as structured evidence:" verbatim.
func TestPhase2EvidenceChannelInstructions_TeachesToolWhenEnabled(t *testing.T) {
	defer SetEvidenceToolMode("on")
	defer SetTwoTurnExplorerMode("on")

	// Isolate from the two-turn escalation so explicit off on
	// evidence_tool_mode actually disables the channel.
	SetTwoTurnExplorerMode("off")

	SetEvidenceToolMode("off")
	off := phase2EvidenceChannelInstructions()
	if off != ":" {
		t.Errorf("off variant must be exactly ':' to keep current prompt verbatim, got %q", off)
	}

	SetEvidenceToolMode("on")
	on := phase2EvidenceChannelInstructions()
	for _, want := range []string{"emit_evidence", "items=", "REJECTED", "Fallback channel", "Markdown shape"} {
		if !strings.Contains(on, want) {
			t.Errorf("on variant must contain %q", want)
		}
	}
}

func summarizeIDs(items []types.EvidenceItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Subject+"@"+it.Source+" id="+it.ID)
	}
	return out
}
