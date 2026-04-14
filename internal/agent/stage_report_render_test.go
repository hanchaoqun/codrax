package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// P1.2 — over-fitting audit notes for these tests:
//
//   - reverse test: TestRenderExplorerStageReport_Deterministic shuffles
//     a copy of the inputs and asserts byte equality; if rendering
//     ever depends on map iteration order or other non-determinism,
//     this test fails first.
//   - deletion test: TestRenderExplorerStageReport_EmptyInputs asserts
//     that removing every input still produces a structured skeleton
//     rather than an empty string. Honesty over silence.
//   - class test: TestRenderExplorerStageReport_NoSiblingProseLeak
//     asserts that no LLM-authored prose is read by the renderer at
//     all — the input is structured fields, so sibling-file `## Evidence
//     from <path>` headers cannot appear in the output regardless of
//     what the LLM wrote in its synthesis prose. This is the structural
//     replacement for F9 (`scrubSiblingEvidenceBlocks`): the leak it
//     prevented is now impossible by data-flow construction.
//   - no-bait test: zero hardcoded keywords or curated word lists in
//     the renderer; this test file's fixtures are real type instances
//     not regex inputs.
//   - no-contamination test: the same renderer is used for every
//     question kind / answer shape — TestRenderExplorerStageReport_KindAgnostic
//     pins that swapping question_kind does not change output beyond
//     the metadata line.

func TestRenderExplorerStageReport_Deterministic(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceConcrete, Subject: "explorerEvaluator",
			Predicate: "Name", Object: "explorer",
			Source: "internal/agent/explorer.go", LineStart: 100,
		},
		{
			Kind: types.EvidenceConditional, Subject: "subExplorerEvaluator",
			Predicate: "ContinuationPrompt", Object: "phase 1 push",
			Source: "internal/agent/sub_explorer.go", LineStart: 191,
		},
	}
	chains := []types.AnswerChain{
		{
			Item: types.EvidenceItem{
				Kind:    types.EvidenceDataflowPath,
				Summary: "RegisterExplorer binds NewExplorerAgent → Name() returns \"explorer\"",
			},
			Score:    1.0,
			StrictOK: true,
		},
	}
	symbols := []types.AnswerSymbol{
		{Name: "ExplorerAgent", File: "internal/agent/explorer.go", Line: 80},
	}
	findings := []types.FlowFindingDigest{
		{ID: "f1", Path: []string{"src", "sink"}},
	}
	files := []string{
		"internal/agent/explorer.go",
		"internal/agent/sub_explorer.go",
	}

	first := renderExplorerStageReport("mechanism", "step_list",
		evidence, chains, symbols, findings, files, false)
	second := renderExplorerStageReport("mechanism", "step_list",
		evidence, chains, symbols, findings, files, false)

	if first != second {
		t.Fatalf("renderer is non-deterministic.\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Reorder the read-files slice and confirm we still get the same
	// output — files are sorted internally.
	shuffled := []string{files[1], files[0]}
	third := renderExplorerStageReport("mechanism", "step_list",
		evidence, chains, symbols, findings, shuffled, false)
	if third != first {
		t.Fatalf("renderer is order-sensitive on read files.\nstable:\n%s\nshuffled:\n%s", first, third)
	}
}

func TestRenderExplorerStageReport_EmptyInputs(t *testing.T) {
	got := renderExplorerStageReport("", "", nil, nil, nil, nil, nil, false)
	if got == "" {
		t.Fatal("renderer returned empty string on empty inputs; expected structured skeleton")
	}
	for _, want := range []string{
		"## Investigation Summary",
		"question_kind: -",
		"answer_shape: -",
		"evidence_items: 0",
		"answer_chains: 0",
		"answer_symbols: 0",
		"flow_findings: 0",
		"files_read: 0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty render missing %q.\noutput:\n%s", want, got)
		}
	}
	// Sections without items must be entirely absent — no empty
	// header noise.
	for _, mustNot := range []string{
		"## Primary Evidence",
		"## Resolution Chains",
		"## Answer Symbols",
		"## Dataflow Findings",
		"## Files Read",
	} {
		if strings.Contains(got, mustNot) {
			t.Errorf("empty render contains spurious section %q.\noutput:\n%s", mustNot, got)
		}
	}
}

// TestRenderExplorerStageReport_NoSiblingProseLeak is the structural
// replacement for the deleted F9 (`scrubSiblingEvidenceBlocks`) tests.
//
// F9's job was to walk LLM-authored prose and strip `## Evidence from
// <sibling>` blocks. P1.2 makes that impossible by construction: the
// renderer reads ONLY structured types, never prose. So even if the
// explorer's `e.investigationNotes` contains such headers (and it
// regularly does — see the t3 sibling-drift cases), they cannot
// appear in the StageReport because the StageReport is built from
// EvidenceItems / AnswerChains / AnswerSymbols, not from notes.
//
// This test pins that property: we hand the renderer evidence items
// whose sources include both primary and sibling paths, and we
// confirm the render uses the structured path field rather than any
// `## Evidence from <path>` header markup.
func TestRenderExplorerStageReport_NoSiblingProseLeak(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceConcrete,
			Subject: "explorerEvaluator", Predicate: "method", Object: "X",
			Source: "internal/agent/explorer.go", LineStart: 100,
		},
		{
			Kind: types.EvidenceMechanism,
			Subject: "subExplorerEvaluator", Predicate: "method", Object: "Y",
			Source: "internal/agent/sub_explorer.go", LineStart: 191,
		},
	}
	got := renderExplorerStageReport("mechanism", "step_list",
		evidence, nil, nil, nil, nil, false)

	// The renderer never invokes the `## Evidence from <path>`
	// header convention — that was an LLM prose artifact. Sources
	// appear inline as `path:line` in the bullet.
	if strings.Contains(got, "## Evidence from") {
		t.Errorf("render leaked LLM prose header convention:\n%s", got)
	}
	if !strings.Contains(got, "internal/agent/explorer.go:100") {
		t.Errorf("render missing primary source location:\n%s", got)
	}
	if !strings.Contains(got, "internal/agent/sub_explorer.go:191") {
		t.Errorf("render missing sibling source location:\n%s", got)
	}
}

func TestRenderExplorerStageReport_UngroundedTagPreserved(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceConcrete,
			Subject: "X", Predicate: "Y", Object: "Z",
			Source: "internal/agent/foo.go", LineStart: 10,
			Producer: "explorer.llm/ungrounded",
		},
	}
	got := renderExplorerStageReport("mechanism", "value",
		evidence, nil, nil, nil, nil, false)
	if !strings.Contains(got, "[UNGROUNDED]") {
		t.Errorf("ungrounded marker not preserved in render:\n%s", got)
	}
}

func TestRenderExplorerStageReport_KindAgnostic(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "X", Source: "a.go", LineStart: 1},
	}
	mech := renderExplorerStageReport("mechanism", "step_list",
		evidence, nil, nil, nil, nil, false)
	enum := renderExplorerStageReport("enumeration", "list_of_symbols",
		evidence, nil, nil, nil, nil, true)

	// The Investigation Summary metadata differs (that is the entire
	// purpose), but every other section must be byte-equal.
	mechBody := stripInvestigationSummary(mech)
	enumBody := stripInvestigationSummary(enum)
	if mechBody != enumBody {
		t.Errorf("question_kind leaked beyond Investigation Summary.\nmech body:\n%s\nenum body:\n%s",
			mechBody, enumBody)
	}
}

func stripInvestigationSummary(s string) string {
	idx := strings.Index(s, "\n## ")
	if idx < 0 {
		return ""
	}
	return s[idx:]
}
