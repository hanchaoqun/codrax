package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retry_state_test.go — R14-c3+c4 lock tests.

// TestPopulateRetryState_RoundTrip pins the basic flow:
//   1. orchestrator builds RetryState from prev emit + violations
//   2. SetRetryState writes to MutableState
//   3. RetryState() reads back the same pointer
//   4. RetryStateSummary captures block-level + item-level annotations
func TestPopulateRetryState_RoundTrip(t *testing.T) {
	mut := &types.MutableState{}
	mut.SetAnswerDocumentV2(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
			{File: "y.go", Line: 20},
		},
		Blocks: []types.AnswerBlock{
			{
				ID: "s1", Kind: types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "hello world",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "ev1"},
				},
			},
			{
				ID: "list1", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "A", CitationRef: 0,
						ClaimUse: &types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge}},
					{ID: "i2", Label: "B", CitationRef: 1},
				},
			},
		},
	})
	res := contract.Result{
		Passed: false,
		Violations: []contract.Violation{
			{Kind: types.ViolPrincipalClaimUseMissing, Detail: `principal block id="s1" kind=summary has no claim_use; family requires one of [definition_fact]`},
			{Kind: types.ViolFacetUncovered, Detail: `required facet "diagram_spine"`},
			{Kind: types.ViolRichnessRegression, Detail: "telemetry only"},
		},
	}

	populateRetryState(mut, res, 0)
	rs := mut.RetryState()
	if rs == nil {
		t.Fatal("RetryState must be set after populateRetryState")
	}
	if rs.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", rs.Attempt)
	}
	if !rs.HasContent() {
		t.Error("HasContent must be true with violations + prev emit")
	}

	// Prev emit summary
	if rs.PrevEmitSummary.CitationsCount != 2 {
		t.Errorf("CitationsCount = %d, want 2", rs.PrevEmitSummary.CitationsCount)
	}
	if got := len(rs.PrevEmitSummary.BlockSummaries); got != 2 {
		t.Fatalf("BlockSummaries len = %d, want 2", got)
	}
	bs0 := rs.PrevEmitSummary.BlockSummaries[0]
	if bs0.ID != "s1" {
		t.Errorf("block[0].ID = %q, want s1", bs0.ID)
	}
	if !bs0.HasClaimUse {
		t.Error("s1 had block-level claim_use; HasClaimUse must be true")
	}
	if bs0.ClaimForm != types.ClaimDefinitionFact {
		t.Errorf("s1 ClaimForm = %q, want definition_fact", bs0.ClaimForm)
	}
	if got := bs0.FacetIDs; len(got) != 1 || got[0] != "current_code_path" {
		t.Errorf("s1 FacetIDs = %v, want [current_code_path]", got)
	}

	bs1 := rs.PrevEmitSummary.BlockSummaries[1]
	if bs1.ItemsWithClaimUse != 1 {
		t.Errorf("list1 ItemsWithClaimUse = %d, want 1", bs1.ItemsWithClaimUse)
	}
	if bs1.ItemsWithCitation != 2 {
		t.Errorf("list1 ItemsWithCitation = %d, want 2", bs1.ItemsWithCitation)
	}

	// PrevEmitJSON populated
	if len(rs.PrevEmitJSON) == 0 {
		t.Error("PrevEmitJSON must be non-empty after marshal")
	}

	// Active violations
	if got := len(rs.ActiveViolations); got != 3 {
		t.Fatalf("ActiveViolations len = %d, want 3", got)
	}
	// Severities and layers populated correctly
	for _, sv := range rs.ActiveViolations {
		if !sv.Severity.IsValid() {
			t.Errorf("violation %q severity invalid: %q", sv.Kind, sv.Severity)
		}
		if sv.Layer == "" {
			t.Errorf("violation %q has empty Layer", sv.Kind)
		}
	}

	// Specific severity checks
	if rs.ActiveViolations[0].Severity != types.SeverityCritical {
		t.Errorf("PrincipalClaimUseMissing severity = %q, want critical",
			rs.ActiveViolations[0].Severity)
	}
	// ViolRichnessRegression must be soft regardless of strict
	if rs.ActiveViolations[2].Severity != types.SeveritySoft {
		t.Errorf("RichnessRegression severity = %q, want soft",
			rs.ActiveViolations[2].Severity)
	}
}

// TestInferViolationLayer_KeyKindsMapped pins kind→layer routing.
func TestInferViolationLayer_KeyKindsMapped(t *testing.T) {
	cases := map[types.ViolationKind]string{
		types.ViolBlockCoverageMissing:     "v2_oracle",
		types.ViolPrincipalClaimUseMissing: "v2_oracle",
		types.ViolFacetUncovered:           "v2_oracle",
		types.ViolSelfContradiction:        "self_consistency",
		types.ViolExternalArtifactUnderdecoded: "external_artifact",
		types.ViolAuthorityOverreach:       "authority",
		types.ViolGhostAnchor:              "cgec",
		types.ViolPlanCritic:               "reviewer",
		types.ViolViewIntentMismatch:       "answer_oracle",
		types.ViolCitation:                 "contract_check",
	}
	for k, want := range cases {
		if got := inferViolationLayer(k); got != want {
			t.Errorf("layer for %q: got %q, want %q", k, got, want)
		}
	}
}

// TestExtractBlockIDFromDetail pins canonical detail-string parsing.
func TestExtractBlockIDFromDetail(t *testing.T) {
	cases := map[string]string{
		`principal block id="lifecycle" kind=section has no claim_use`: "lifecycle",
		`block id="s1" diagram kind mismatch`:                          "s1",
		`required facet "X"`:                                           "",
		``:                                                              "",
		`block id=lifecycle (no quotes)`:                               "",
	}
	for in, want := range cases {
		if got := extractBlockIDFromDetail(in); got != want {
			t.Errorf("extractBlockIDFromDetail(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestInferFieldPathFromKind pins specific kind → field path mapping.
func TestInferFieldPathFromKind(t *testing.T) {
	if got := inferFieldPathFromKind(types.ViolPrincipalClaimUseMissing,
		`principal block id="lifecycle" kind=section has no claim_use`); !strings.Contains(got, `id="lifecycle"`) {
		t.Errorf("field path missing block id: %q", got)
	}
	if got := inferFieldPathFromKind(types.ViolFacetUncovered, `required facet "X"`); !strings.Contains(got, "facet_ids") {
		t.Errorf("FacetUncovered field path = %q, want contains facet_ids", got)
	}
	if got := inferFieldPathFromKind(types.ViolRichnessRegression, "anything"); got != "" {
		t.Errorf("RichnessRegression must yield empty field path; got %q", got)
	}
}

// TestSummarizeAnswerDocV2ForRetry_NilSafe covers nil and empty
// edge cases.
func TestSummarizeAnswerDocV2ForRetry_NilSafe(t *testing.T) {
	if got := summarizeAnswerDocV2ForRetry(nil); got.CitationsCount != 0 {
		t.Errorf("nil doc must yield zero summary")
	}
	got := summarizeAnswerDocV2ForRetry(&types.AnswerDocumentV2{})
	if got.CitationsCount != 0 || len(got.BlockSummaries) != 0 {
		t.Errorf("empty doc must yield zero summary; got %+v", got)
	}
}

// TestTextHeadTail_TruncatesLargeText covers the head+tail clip.
func TestTextHeadTail_TruncatesLargeText(t *testing.T) {
	short := "short text"
	if got := textHeadTail(short, 100, 50); got != short {
		t.Errorf("short text must pass through; got %q", got)
	}
	long := strings.Repeat("a", 1000)
	got := textHeadTail(long, 100, 50)
	if !strings.Contains(got, "<truncated") {
		t.Errorf("long text must include truncation marker; got %q", got[:80])
	}
}
