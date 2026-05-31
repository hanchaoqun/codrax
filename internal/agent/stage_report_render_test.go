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
		nil, evidence, chains, symbols, findings, files, false)
	second := renderExplorerStageReport("mechanism", "step_list",
		nil, evidence, chains, symbols, findings, files, false)

	if first != second {
		t.Fatalf("renderer is non-deterministic.\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Reorder the read-files slice and confirm we still get the same
	// output — files are sorted internally.
	shuffled := []string{files[1], files[0]}
	third := renderExplorerStageReport("mechanism", "step_list",
		nil, evidence, chains, symbols, findings, shuffled, false)
	if third != first {
		t.Fatalf("renderer is order-sensitive on read files.\nstable:\n%s\nshuffled:\n%s", first, third)
	}
}

func TestRenderExplorerStageReport_EmptyInputs(t *testing.T) {
	got := renderExplorerStageReport("", "", nil, nil, nil, nil, nil, nil, false)
	if got == "" {
		t.Fatal("renderer returned empty string on empty inputs; expected structured skeleton")
	}
	for _, want := range []string{
		"## Investigation Summary",
		"question_kind: -",
		"question_family: -",
		"evidence_items: 0",
		"external_observations: 0",
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

func TestRenderExplorerStageReport_RendersExternalObservationsSeparately(t *testing.T) {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{
		MCPResponses: []types.MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call",
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Success:     true,
			Observations: []types.MCPTypedObservation{
				{ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12, Summary: "pid=4242 event=sched_wakeup waker=helper"},
				{ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, Summary: "pid=4242 event=sched_switch prev_state=S"},
			},
		}},
	})

	got := renderExplorerStageReport("", "", nil, nil, nil, nil, nil, nil, false, ledger.Records...)
	for _, want := range []string{
		"evidence_items: 0",
		"external_observations: 2",
		"## External Observations",
		"`evidence_items: 0` only means no current-source EvidenceItem rows were emitted",
		"[mcp_resource] mcp://fixture/trace/sleep-wakeup:7 — pid=4242 event=sched_switch prev_state=S",
		"[mcp_resource] mcp://fixture/trace/sleep-wakeup:12 — pid=4242 event=sched_wakeup waker=helper",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("external observation report missing %q.\noutput:\n%s", want, got)
		}
	}
	if strings.Contains(got, "## Primary Evidence") {
		t.Fatalf("MCP observations must not be rendered as current-source primary evidence:\n%s", got)
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
			Kind:    types.EvidenceConcrete,
			Subject: "explorerEvaluator", Predicate: "method", Object: "X",
			Source: "internal/agent/explorer.go", LineStart: 100,
		},
		{
			Kind:    types.EvidenceMechanism,
			Subject: "subExplorerEvaluator", Predicate: "method", Object: "Y",
			Source: "internal/agent/sub_explorer.go", LineStart: 191,
		},
	}
	got := renderExplorerStageReport("mechanism", "step_list",
		nil, evidence, nil, nil, nil, nil, false)

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

func TestFormatEvidenceLineForReport_UngroundedTagPreserved(t *testing.T) {
	// Post 2026-04-17 redesign: GroundingStatus replaces the legacy
	// "/ungrounded" Producer suffix. formatEvidenceLineForReport
	// surfaces ungrounded items with a trailing [UNGROUNDED] tag so
	// callers that render lower-trust evidence directly still keep
	// the trust-level marker.
	ev := types.EvidenceItem{
		Kind:    types.EvidenceConcrete,
		Subject: "X", Predicate: "Y", Object: "Z",
		Source: "internal/agent/foo.go", LineStart: 10,
		GroundingStatus: types.GroundingUngrounded,
	}
	got := formatEvidenceLineForReport(ev, nil)
	if !strings.Contains(got, "[UNGROUNDED]") {
		t.Errorf("ungrounded marker not preserved in render:\n%s", got)
	}
}

func TestFormatEvidenceLineForReport_RecoveredTagPreserved(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:    types.EvidenceConcrete,
		Subject: "X", Predicate: "Y", Object: "Z",
		Source: "internal/agent/foo.go", LineStart: 42,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierFQNameSameFile,
	}
	got := formatEvidenceLineForReport(ev, nil)
	// Session-8: recovered tag still rendered, with new "read_file
	// before citing" guidance; LineStart (42) stripped so downstream
	// cannot pick it up.
	if !strings.Contains(got, "[recovered") {
		t.Errorf("recovered marker missing in render:\n%s", got)
	}
	if strings.Contains(got, ":42") {
		t.Errorf("recovered LineStart must be stripped at cross-stage boundary:\n%s", got)
	}
	if !strings.Contains(got, "read_file before citing") {
		t.Errorf("recovered tag must nudge the LLM to read_file:\n%s", got)
	}
}

func TestFormatEvidenceLineForReport_PrefersDeterministicSurfaceOverSummary(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:         types.EvidenceConditional,
		AnchorSymbol: "logBundle",
		Predicate:    "guards",
		Condition:    "logBundle != nil",
		Source:       "internal/agent/analyzer.go",
		LineStart:    1042,
		Summary:      "第 1042 行只检查 logBundle != nil，未检查 logBundle.Meta，因此后续一定会因为 Meta 为 nil 而 panic。",
	}
	got := formatEvidenceLineForReport(ev, nil)
	if !strings.Contains(got, "logBundle guards IF logBundle != nil") {
		t.Fatalf("report line should keep typed surface, got:\n%s", got)
	}
	if strings.Contains(got, "未检查 logBundle.Meta") || strings.Contains(got, "一定会因为 Meta 为 nil") {
		t.Fatalf("report line leaked unsupported summary prose, got:\n%s", got)
	}
}

func TestRenderExplorerStageReport_PrimaryEvidenceExcludesNonCitableNoise(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind:    types.EvidenceUnresolved,
			Subject: "DynamicDispatch", Predicate: "unknown_effect", Object: "target unresolved",
			Source: "internal/agent/foo.go", LineStart: 10,
		},
		{
			Kind:    types.EvidenceTruncated,
			Subject: "candidate_files", Predicate: "truncated_to_budget", Object: "20",
		},
		{
			Kind:    types.EvidenceConcrete,
			Subject: "Ungrounded", Predicate: "returns", Object: "value",
			Source: "internal/agent/foo.go", LineStart: 12,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:    types.EvidenceConcrete,
			Subject: "Recovered", Predicate: "returns", Object: "value",
			Source: "internal/agent/foo.go", LineStart: 13,
			GroundingStatus: types.GroundingRecovered,
		},
		{
			Kind:    types.EvidenceDirect,
			Subject: "Grounded", Predicate: "returns", Object: "value",
			Source: "internal/agent/foo.go", LineStart: 14,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := renderExplorerStageReport("mechanism", "value",
		nil, evidence, nil, nil, nil, nil, false)
	if !strings.Contains(got, "evidence_items: 5") {
		t.Fatalf("summary should retain raw evidence count for auditability:\n%s", got)
	}
	if !strings.Contains(got, "Grounded returns value") {
		t.Fatalf("grounded evidence missing from Primary Evidence:\n%s", got)
	}
	for _, noise := range []string{"DynamicDispatch", "candidate_files", "Ungrounded", "Recovered"} {
		if strings.Contains(got, noise) {
			t.Fatalf("non-primary evidence %q leaked into Primary Evidence:\n%s", noise, got)
		}
	}
}

func TestRenderExplorerStageReport_NeutralizesExactResolutionNearbyContext(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	item := types.EvidenceItem{
		Kind:            types.EvidenceMechanism,
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "explains",
		Object:          "nearby precedence baseline",
		Summary:         "This item names explore_mid_loop_hint_budget only in explanatory context; do NOT repair this item.",
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}
	got := renderExplorerStageReport(
		"mechanism",
		"explanation",
		contract,
		[]types.EvidenceItem{item},
		[]types.AnswerChain{{Item: item, Score: 1.0, StrictOK: true}},
		nil,
		nil,
		nil,
		false,
	)
	if strings.Contains(got, "do NOT repair this item") {
		t.Fatalf("StageReport leaked operational repair prose:\n%s", got)
	}
	if strings.Contains(got, "explore_mid_loop_hint_budget") {
		t.Fatalf("StageReport leaked repeated exact target prose for nearby-context item:\n%s", got)
	}
	if !strings.Contains(got, "DefaultExploreHeuristics explains nearby precedence baseline") {
		t.Fatalf("StageReport lost the structural nearby-context claim:\n%s", got)
	}
}

// TestRenderExplorerStageReport_AnchorKindTag pins the Fix C
// contract: formatEvidenceLineForReport appends an anchor-kind
// suffix — "(call site)", "(definition)", etc. — after the cited
// location so downstream LLM consumers cannot confuse a call-site
// line with the caller's definition line. Regression target was the
// u3a run-1 failure (eval/results/u3a-20260422-171414/run-1) where
// the extractor read "[relationship] X calls Y — file:992" as "X is
// at 992", derived 10 wrong-line emit_answer_symbol items, and all
// finalizer citations failed grounding.
func TestRenderExplorerStageReport_AnchorKindTag(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind:         types.EvidenceRelationship,
			Subject:      "internal/agent/explorer.go:explorerEvaluator.focusedAnchorNeighborhood",
			Predicate:    "calls",
			Object:       "canonicalExplorerPath",
			Source:       "internal/agent/explorer.go",
			LineStart:    992,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "canonicalExplorerPath",
		},
		{
			Kind:         types.EvidenceConcrete,
			Subject:      "explorerEvaluator.ShouldStop",
			Predicate:    "defined",
			Source:       "internal/agent/explorer.go",
			LineStart:    2478,
			AnchorKind:   types.AnchorDefinition,
			AnchorSymbol: "ShouldStop",
		},
	}
	got := renderExplorerStageReport("mechanism", "step_list",
		nil, evidence, nil, nil, nil, nil, false)

	if !strings.Contains(got, "internal/agent/explorer.go:992 (call site)") {
		t.Errorf("call-site anchor tag missing; render:\n%s", got)
	}
	if !strings.Contains(got, "internal/agent/explorer.go:2478 (definition)") {
		t.Errorf("definition anchor tag missing; render:\n%s", got)
	}
}

func TestRenderExplorerStageReport_KindAgnostic(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "X", Source: "a.go", LineStart: 1},
	}
	mech := renderExplorerStageReport("mechanism", "step_list",
		nil, evidence, nil, nil, nil, nil, false)
	enum := renderExplorerStageReport("enumeration", "list_of_symbols",
		nil, evidence, nil, nil, nil, nil, true)

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
