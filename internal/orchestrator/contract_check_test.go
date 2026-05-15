package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// contract_check_test.go locks the orchestrator's contract-checker
// glue: citation extraction, the violation-rendering helper, and the
// fail-loud answer-augmentation pattern. The contract checker logic
// itself has its own tests under internal/analysis/contract.

func TestExtractCitations_BasicFileLine(t *testing.T) {
	text := `The handler is dispatched in internal/agent/explorer.go:1234 and
returns through internal/agent/finalizer.go:567.`
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 2 {
		t.Fatalf("want 2 citations, got %d: %+v", len(cits), cits)
	}
	if cits[0].File != "internal/agent/explorer.go" || cits[0].Line != 1234 {
		t.Errorf("cit[0]: %+v", cits[0])
	}
	if cits[1].File != "internal/agent/finalizer.go" || cits[1].Line != 567 {
		t.Errorf("cit[1]: %+v", cits[1])
	}
}

func TestExtractCitations_RangeForm(t *testing.T) {
	text := "see file.go:100-150 for the full block."
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Fatalf("want 1, got %d", len(cits))
	}
	if cits[0].File != "file.go" || cits[0].Line != 100 {
		t.Errorf("file/line: %+v", cits[0])
	}
	if len(cits[0].Lines) != 2 || cits[0].Lines[0] != 100 || cits[0].Lines[1] != 150 {
		t.Errorf("Lines: %+v", cits[0].Lines)
	}
}

func TestExtractCitations_DeduplicatesIdenticalRefs(t *testing.T) {
	text := "x.go:5 ... x.go:5 again ... x.go:5 once more"
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Errorf("want 1 dedup'd citation, got %d", len(cits))
	}
}

func TestExtractCitations_IgnoresProseColons(t *testing.T) {
	// "step 3:" and "10:30" must not register as citations because
	// neither has a `path.ext:line` shape.
	text := "Step 3: at 10:30 we observed nothing."
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 0 {
		t.Errorf("prose colons should not match; got %+v", cits)
	}
}

func TestExtractCitations_BareFileWithExtension(t *testing.T) {
	// README.md:1 is a legitimate citation (no slash but real
	// extension + line number).
	text := "see README.md:1 for context"
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Fatalf("want 1, got %d: %+v", len(cits), cits)
	}
	if cits[0].File != "README.md" || cits[0].Line != 1 {
		t.Errorf("got %+v", cits[0])
	}
}

func TestExtractCitations_EmptyText(t *testing.T) {
	if cits := extractCitationsFromAnswer(""); cits != nil {
		t.Errorf("empty text should yield nil, got %+v", cits)
	}
}

func TestRunContractCheck_EmptyContractAlwaysPasses(t *testing.T) {
	out := &agent.StageOutput{FinalAnswer: "anything goes"}
	res := runContractCheck(out, types.AnswerContract{}, nil, nil)
	if !res.Passed {
		t.Errorf("empty contract should pass; got %+v", res)
	}
}

func TestRunContractCheck_NilOutputPasses(t *testing.T) {
	res := runContractCheck(nil, types.AnswerContract{}, nil, nil)
	if !res.Passed {
		t.Errorf("nil output should pass (caller decides separately); got %+v", res)
	}
}

// V1 shape-text-heuristic check (checkShape) has been retired per
// docs/migration/answer_shape_retirement.md; the V2 block oracles in
// contract_check_block.go own the read-mode block contract on the
// typed AnswerDocumentV2 carrier, not on rendered prose. The
// TestRunContractCheck_FailingShape coverage moves to V2 block-level
// tests in contract_check_block_test.go.

func TestRunContractCheck_CitationGap_SoftDegrade(t *testing.T) {
	// Contract requires 3 file_line citations; answer has 1.
	// Soft degradation: 1 > 0 → pass.
	out := &agent.StageOutput{
		FinalAnswer: "- foo\n- bar\nSee internal/agent/explorer.go:42",
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if !res.Passed {
		t.Errorf("1 citation with min=3 should soft-pass; got violations: %+v", res.Violations)
	}
}

func TestRunContractCheck_CitationGap_ZeroRejects(t *testing.T) {
	// Contract requires 3 file_line citations; answer has 0 → reject.
	out := &agent.StageOutput{
		FinalAnswer: "- foo\n- bar\nNo citations at all",
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if res.Passed {
		t.Error("0 citations should hard-reject")
	}
}

func TestRunContractCheck_PassingCase(t *testing.T) {
	out := &agent.StageOutput{
		FinalAnswer: `- ` + "`" + `Foo` + "`" + ` at internal/a.go:1
- ` + "`" + `Bar` + "`" + ` at internal/b.go:2`,
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if !res.Passed {
		t.Errorf("expected pass, got violations: %+v", res.Violations)
	}
}

func TestRunExternalArtifactTypedCoverageCheck_DiagnosticRequiresTypedCarrier(t *testing.T) {
	mut := types.NewMutableState("test")
	bundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "index out of bounds: index=5, size=3",
		}},
	}
	mut.SetLogTriage(bundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: bundle,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
			},
		},
	}

	missing := runExternalArtifactTypedCoverageCheck(ctx, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "diagnostic summary",
		}},
	})
	if len(missing) != 1 || missing[0].Kind != types.ViolExternalArtifactUnderdecoded {
		t.Fatalf("diagnostic artifact answer should require typed observed-artifact carrier, got %+v", missing)
	}
	if strings.Contains(missing[0].Detail, "index out of bounds") ||
		strings.Contains(missing[0].Repair, "index out of bounds") {
		t.Fatalf("typed carrier repair must not echo runtime message text, got %+v", missing[0])
	}

	present := runExternalArtifactTypedCoverageCheck(ctx, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:       "observed",
			Kind:     types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetObservedArtifactFact)},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimExternalObservation,
				FacetID:   string(types.FacetObservedArtifactFact),
			}},
		}},
	})
	if len(present) != 0 {
		t.Fatalf("typed observed-artifact carrier should satisfy gate, got %+v", present)
	}
}

func TestRunExternalArtifactTypedCoverageCheck_NonDiagnosticTreatsArtifactAsContext(t *testing.T) {
	mut := types.NewMutableState("test")
	bundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "index out of bounds: index=5, size=3",
		}},
	}
	mut.SetLogTriage(bundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentExplain,
				Scenario:  types.ScenarioArchitectureExplain,
				LogTriage: bundle,
			},
		},
	}

	got := runExternalArtifactTypedCoverageCheck(ctx, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary}},
	})
	if len(got) != 0 {
		t.Fatalf("non-diagnostic artifact question must not require observed-artifact carrier, got %+v", got)
	}
}

func TestRunExternalArtifactTypedCoverageCheck_PerfTraceUsesSameCarrier(t *testing.T) {
	mut := types.NewMutableState("test")
	perf := &types.PerfBundle{
		Stalls: []types.PerfStall{{Symbol: "renderFrame", DurationMs: 120}},
	}
	mut.SetPerfTrace(perf)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				PerfTrace: perf,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
			},
		},
	}

	got := runExternalArtifactTypedCoverageCheck(ctx, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
		}},
	})
	if len(got) != 1 || got[0].Kind != types.ViolExternalArtifactUnderdecoded {
		t.Fatalf("perf diagnostic artifact should use the same typed carrier gate, got %+v", got)
	}
}

func TestRenderViolations_Ordering(t *testing.T) {
	res := contract.Result{
		Passed: false,
		Violations: []contract.Violation{
			{Kind: contract.ViolFamilyMismatch, Detail: "no bullets"},
			{Kind: contract.ViolCitation, Detail: "0/3 citations", Repair: "add 3 anchors"},
		},
	}
	got := renderViolations(res)
	if !strings.Contains(got, "no bullets") || !strings.Contains(got, "0/3 citations") {
		t.Errorf("missing details: %q", got)
	}
	if !strings.Contains(got, "add 3 anchors") {
		t.Errorf("repair missing: %q", got)
	}
	if !strings.Contains(got, ";") {
		t.Errorf("expected joiner; got %q", got)
	}
}

func TestRenderViolations_EmptyOnPassed(t *testing.T) {
	if got := renderViolations(contract.Result{Passed: true}); got != "" {
		t.Errorf("passed result should yield empty; got %q", got)
	}
}

// TestFormatViolationsForLogger_FailLoudPattern verifies the digest
// formatter (renamed from appendViolationsToAnswer in B4-F3,
// 2026-05-04). The function now returns the digest string only;
// the channel decision (user panel vs operator log) lives in
// orchestrator.go::applyContractViolations.
func TestFormatViolationsForLogger_FailLoudPattern(t *testing.T) {
	res := contract.Result{Passed: false, Violations: []contract.Violation{
		{Kind: contract.ViolFamilyMismatch, Detail: "wrong"},
	}}
	got := formatViolationsForLogger(res)
	if !strings.HasPrefix(got, "· answer-contract validation exhausted") {
		t.Errorf("want digest prefix; got %q", got)
	}
	if !strings.Contains(got, "wrong") {
		t.Errorf("want violation detail in digest; got %q", got)
	}
}

func TestFormatViolationsForLogger_PassedReturnsEmpty(t *testing.T) {
	if got := formatViolationsForLogger(contract.Result{Passed: true}); got != "" {
		t.Errorf("passed: want empty digest; got %q", got)
	}
}

func TestFormatViolationsForLogger_NoViolationsReturnsEmpty(t *testing.T) {
	if got := formatViolationsForLogger(contract.Result{Passed: false}); got != "" {
		t.Errorf("no violations: want empty digest; got %q", got)
	}
}

// ── B6-F1 cross-citation single-locus oracle tests ─────────────────

// TestRunCrossCitationConflictOracleV2_FlagsDifferentLines covers the
// canonical case: same symbol in two block items with citations
// pointing at different lines >tolerance apart in the SAME file.
// Real-world example from m1a-... (extractor.go:114 vs :135).
func TestRunCrossCitationConflictOracleV2_FlagsDifferentLines(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "internal/agent/extractor.go", Line: 114},
			{File: "internal/agent/extractor.go", Line: 135},
		},
		Blocks: []types.AnswerBlock{
			{
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{Label: "TurnAArtifacts", Text: "first reference", CitationRef: 0},
					{Label: "TurnAArtifacts", Text: "second reference", CitationRef: 1},
				},
			},
		},
	}
	vs := runCrossCitationConflictOracleV2(doc)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolCrossCitationConflict {
		t.Errorf("kind = %q, want ViolCrossCitationConflict", vs[0].Kind)
	}
	if got := vs[0].ClusterKey; got != "label:TurnAArtifacts|root:answer_citations_locus" {
		t.Errorf("ClusterKey = %q, want typed symbol cluster key", got)
	}
	if !strings.Contains(vs[0].Detail, "TurnAArtifacts") {
		t.Errorf("Detail must name the conflicting symbol; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "extractor.go:114") || !strings.Contains(vs[0].Detail, "extractor.go:135") {
		t.Errorf("Detail must verbatim list both loci; got %q", vs[0].Detail)
	}
}

// TestRunCrossCitationConflictOracleV2_DifferentFilesFlags covers the
// "two different files" case (interface decl vs impl). Without
// distinct labels, this is also a conflict.
func TestRunCrossCitationConflictOracleV2_DifferentFilesFlags(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "iface.go", Line: 10},
			{File: "impl.go", Line: 10},
		},
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{
				{Label: "MyHandler", CitationRef: 0},
				{Label: "MyHandler", CitationRef: 1},
			},
		}},
	}
	if vs := runCrossCitationConflictOracleV2(doc); len(vs) != 1 {
		t.Errorf("want 1 violation for cross-file conflict; got %d (%+v)", len(vs), vs)
	}
}

// TestRunCrossCitationConflictOracleV2_AdjacentLinesPass confirms the
// 2-line tolerance — receiver-decl + signature lines are not a
// conflict.
func TestRunCrossCitationConflictOracleV2_AdjacentLinesPass(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "x.go", Line: 100},
			{File: "x.go", Line: 101},
		},
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{
				{Label: "F", CitationRef: 0},
				{Label: "F", CitationRef: 1},
			},
		}},
	}
	if vs := runCrossCitationConflictOracleV2(doc); len(vs) > 0 {
		t.Errorf("want no conflict on adjacent lines; got %+v", vs)
	}
}

// TestRunCrossCitationConflictOracleV2_DistinctNamesNoConflict
// confirms that two different symbols cited at different lines
// is the normal case — no conflict.
func TestRunCrossCitationConflictOracleV2_DistinctNamesNoConflict(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "x.go", Line: 200},
		},
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{
				{Label: "A", CitationRef: 0},
				{Label: "B", CitationRef: 1},
			},
		}},
	}
	if vs := runCrossCitationConflictOracleV2(doc); len(vs) > 0 {
		t.Errorf("distinct names must not conflict; got %+v", vs)
	}
}

// TestRunCrossCitationConflictOracleV2_NilGuards exercises the early-
// return guards.
func TestRunCrossCitationConflictOracleV2_NilGuards(t *testing.T) {
	if vs := runCrossCitationConflictOracleV2(nil); vs != nil {
		t.Errorf("nil doc must yield nil; got %+v", vs)
	}
	if vs := runCrossCitationConflictOracleV2(&types.AnswerDocumentV2{}); vs != nil {
		t.Errorf("empty doc must yield nil; got %+v", vs)
	}
}

// ── R2.4 selection-fix tests (post_shape_residual_audit.md
// 2026-05-04) ──────────────────────────────────────────────────────

// TestRunCrossCitationConflictOracleV2_WordyLabelNoLongerFires pins
// the R2.4 fix: pre-fix, an item whose Label is wordy prose like
// "checkCoverage — 覆盖度检查" + a different sibling item with the
// same Label would group as the same symbol. R2.4 changes the
// identity key to require identifier shape, so wordy Labels are
// silently skipped (no false grouping → no false fire).
//
// This is the s1a r1 false-negative root: 15 items had distinct
// wordy Labels each pointing at gate.go:128. Pre-R2.4 behaviour
// would NOT fire (because Labels were distinct), but the fix
// keeps the same outcome on this specific shape via a more
// principled gating — and stops false fires on a hypothetical
// case where the LLM emits the same wordy prose twice.
func TestRunCrossCitationConflictOracleV2_WordyLabelNoLongerFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "internal/agent/extractor.go", Line: 114},
			{File: "internal/agent/extractor.go", Line: 200},
		},
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{Label: "checkCoverage — 覆盖度检查", CitationRef: 0},
				{Label: "checkCoverage — 覆盖度检查", CitationRef: 1},
			},
		}},
	}
	if vs := runCrossCitationConflictOracleV2(doc); len(vs) > 0 {
		t.Errorf("wordy Label must NOT group items; got false fire %+v", vs)
	}
}

// TestRunCrossCitationConflictOracleV2_ItemIDGroupingFires confirms
// the typed-signal path: items sharing the same ID become the
// identity key.
func TestRunCrossCitationConflictOracleV2_ItemIDGroupingFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "x.go", Line: 50},
			{File: "x.go", Line: 150},
		},
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "step1", Label: "wordy prose A", CitationRef: 0},
				{ID: "step1", Label: "wordy prose B", CitationRef: 1},
			},
		}},
	}
	if vs := runCrossCitationConflictOracleV2(doc); len(vs) != 1 {
		t.Errorf("ID-keyed grouping must fire once; got %d (%+v)", len(vs), vs)
	}
}

// TestLooksLikeIdentifierShape covers the helper edge cases.
func TestLooksLikeIdentifierShape(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"TurnAArtifacts", true},
		{"check_coverage", true},
		{"my.dotted.name", true},
		{"kebab-name", true},
		{"V2", true},
		{"", false},
		{"has space", false},
		{"中文标识符", false},
		{"label — extra prose", false},
		{"foo()", false},
	}
	for _, tc := range cases {
		if got := looksLikeIdentifierShape(tc.in); got != tc.want {
			t.Errorf("looksLikeIdentifierShape(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestCrossCitationIdentityKey_PriorityOrder pins the priority:
// ID > Label (ident-shape only).
func TestCrossCitationIdentityKey_PriorityOrder(t *testing.T) {
	// ID wins
	if got := crossCitationIdentityKey(types.AnswerBlockItem{
		ID: "iid", Label: "Lbl",
	}); got != "id:iid" {
		t.Errorf("ID priority broken: %q", got)
	}

	// No ID → Label (only when ident-shape)
	if got := crossCitationIdentityKey(types.AnswerBlockItem{Label: "Lbl"}); got != "label:Lbl" {
		t.Errorf("Label fallback broken: %q", got)
	}
	// Wordy Label → ""
	if got := crossCitationIdentityKey(types.AnswerBlockItem{
		Label: "wordy prose label",
	}); got != "" {
		t.Errorf("wordy Label must yield empty key; got %q", got)
	}
}

// TestIsJustifiedAbsenceAnswer_ShapeTieredGate pins the shape-tiered
// trust check. Shallow shapes (value / boolean / config_value) and
// EMPTY list_of_symbols pass on any single investigation tool. Only
// NON-empty list_of_symbols (behaviour-enumeration positive answers)
// require a content read — and those are not absence cases. Zero-tool
// runs never pass.
//
