package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P3 (2026-05-10) — structured Mechanical Fix Actions ===

// TestRenderRetryStructuredFixList_NilSafe — empty / nil rs returns empty.
func TestRenderRetryStructuredFixList_NilSafe(t *testing.T) {
	if got := renderRetryStructuredFixList(nil); got != "" {
		t.Errorf("nil rs: want empty, got %q", got)
	}
	if got := renderRetryStructuredFixList(&types.RetryState{}); got != "" {
		t.Errorf("empty rs: want empty, got %q", got)
	}
}

// TestRenderRetryStructuredFixList_BlockCoverage — typed missing-kind row.
func TestRenderRetryStructuredFixList_BlockCoverage(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:      types.ViolBlockCoverageMissing,
				Severity:  types.SeverityMedium,
				FieldPath: "blocks[].kind=ordered_list",
				Repair:    "emit at least 1 block of kind=ordered_list",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "Action: `add_block`") {
		t.Errorf("missing add_block action; got %q", got)
	}
	if !strings.Contains(got, "kind: `ordered_list`") {
		t.Errorf("missing kind=ordered_list arg; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_PrincipalClaimUse — typed set_field row.
func TestRenderRetryStructuredFixList_PrincipalClaimUse(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolPrincipalClaimUseMissing,
				Severity: types.SeverityHigh,
				BlockID:  "l1",
				Repair:   "emit claim_use on the block",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "Action: `set_field`") {
		t.Errorf("missing set_field action; got %q", got)
	}
	if !strings.Contains(got, `Target: `+"`blocks[id=\"l1\"].claim_uses`") {
		t.Errorf("missing per-block target; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_FacetUncovered — typed facet path row.
func TestRenderRetryStructuredFixList_FacetUncovered(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolFacetUncovered,
				Severity: types.SeverityMedium,
				Detail:   `required facet "current_code_path" (essential) is not covered: no block declared it`,
				Repair:   `declare facet_id="current_code_path" on at least one block`,
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "facet_ids") {
		t.Errorf("missing facet_ids target; got %q", got)
	}
	if !strings.Contains(got, "current_code_path") {
		t.Errorf("missing extracted facet kind; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_UncertaintyBlock — typed add_caveat_block row.
func TestRenderRetryStructuredFixList_UncertaintyBlock(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolUncertaintyBlockMissing,
				Severity: types.SeverityMedium,
				Repair:   "emit a caveat block disclosing the boundary",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "Action: `add_caveat_block`") {
		t.Errorf("missing add_caveat_block; got %q", got)
	}
	if !strings.Contains(got, "kind: `caveat`") {
		t.Errorf("missing caveat kind arg; got %q", got)
	}
	if kIdx, textIdx := strings.Index(got, "kind: `caveat`"), strings.Index(got, "text: `"); kIdx < 0 || textIdx < 0 || kIdx > textIdx {
		t.Errorf("structured args must render in deterministic kind-then-text order; got %q", got)
	}
}

func TestRenderRetryStructuredFixList_CurrentStatusVerdict(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolCurrentStatusVerdictMissing,
				Severity: types.SeverityMedium,
				BlockID:  "decision_current",
				Repair:   "emit a principal decision block whose text starts with exactly one of still_present, fixed, or not_enough_evidence",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "Action: `set_field`") {
		t.Errorf("missing set_field action; got %q", got)
	}
	if !strings.Contains(got, `Target: `+"`blocks[id=\"decision_current\"].text`") {
		t.Errorf("missing decision text target; got %q", got)
	}
	if !strings.Contains(got, "still_present | fixed | not_enough_evidence") {
		t.Errorf("missing bounded verdict prefixes; got %q", got)
	}
}

func TestRenderRetryStructuredFixList_LaneBlockKindMismatch(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolLaneBlockKindMismatch,
				Severity: types.SeverityMedium,
				BlockID:  "observed_steps",
				Repair:   "change the block kind to one of [summary caveat]",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, `Target: `+"`blocks[id=\"observed_steps\"].kind`") {
		t.Errorf("missing block kind target; got %q", got)
	}
	if !strings.Contains(got, "one of the block kinds named in the repair hint") {
		t.Errorf("missing value guidance; got %q", got)
	}
}

func TestRenderRetryStructuredFixList_MissingRequestedRoles(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolMissingRequestedRoleUndisclosed,
				Severity: types.SeverityMedium,
				Repair:   `set to [{role:"runtime", label:"CLI"}]`,
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if !strings.Contains(got, "Target: `missing_requested_roles[]`") {
		t.Errorf("missing missing_requested_roles target; got %q", got)
	}
	if !strings.Contains(got, `set to [{role:"runtime", label:"CLI"}]`) {
		t.Errorf("missing exact repair value guidance; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_UnknownKind_FallsThrough — subjective
// reviewer concerns (e.g. answer_semantic_underfilled) are NOT covered
// by P3; should NOT appear in the structured list and the section
// stays empty when it's the only violation.
func TestRenderRetryStructuredFixList_UnknownKind_FallsThrough(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:     types.ViolAnswerSemanticUnderfilled,
				Severity: types.SeverityMedium,
				Repair:   "address the reviewer concerns",
			},
		},
	}
	got := renderRetryStructuredFixList(rs)
	if got != "" {
		t.Errorf("unknown kind should fall through to prose; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_MixedKinds_RendersTypedOnly — known
// kinds render; unknowns silently dropped.
func TestRenderRetryStructuredFixList_MixedKinds_RendersTypedOnly(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolBlockCoverageMissing, FieldPath: "blocks[].kind=ordered_list", Severity: types.SeverityMedium},
			{Kind: types.ViolAnswerSemanticUnderfilled, Severity: types.SeverityMedium, Repair: "subjective"},
			{Kind: types.ViolUncertaintyBlockMissing, Severity: types.SeverityMedium},
		},
	}
	got := renderRetryStructuredFixList(rs)
	// Two typed rows; subjective dropped.
	if !strings.Contains(got, "1. Action: `add_block`") {
		t.Errorf("missing first typed row; got %q", got)
	}
	if !strings.Contains(got, "2. Action: `add_caveat_block`") {
		t.Errorf("missing second typed row; got %q", got)
	}
	// "subjective" should NOT appear (not in the structured list).
	if strings.Contains(got, "subjective") {
		t.Errorf("subjective Repair should not surface in structured list; got %q", got)
	}
}

// TestRenderRetryStructuredFixList_R6Audit pins the new prompt text
// for R6 (no internal vocab leaks) + R4 (no third natural language).
func TestRenderRetryStructuredFixList_R6Audit(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolBlockCoverageMissing, FieldPath: "blocks[].kind=ordered_list", Severity: types.SeverityMedium},
			{Kind: types.ViolPrincipalClaimUseMissing, BlockID: "l1", Severity: types.SeverityMedium},
			{Kind: types.ViolFacetUncovered, Detail: `required facet "x"`, Severity: types.SeverityMedium},
			{Kind: types.ViolUncertaintyBlockMissing, Severity: types.SeverityMedium},
		},
	}
	got := renderRetryStructuredFixList(rs)

	// R6: no internal Go type / metric vocab.
	for _, banned := range []string{
		"ScoredViolation", "ViolKind", "ViolationKind",
		"ClusterKey", "SuspectedRoot", "BuildRepairPlan",
		"FacetCoverage", "AnchoredCount", "DeclaredCount",
		"BlockRequirement", "RetryState", "RepairCluster",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("must not leak internal token %q; got %q", banned, got)
		}
	}

	// No third natural language tokens.
	for _, tok := range []string{"diese", "todos", "tutti", "cette", "это", "هذا"} {
		if strings.Contains(got, tok) {
			t.Errorf("must be EN only; banned %q in %q", tok, got)
		}
	}
}

// TestExtractMissingBlockKind_FieldPathFallback
func TestExtractMissingBlockKind_FieldPathFallback(t *testing.T) {
	sv := types.ScoredViolation{
		FieldPath: "blocks[].kind=ordered_list",
	}
	if got := extractMissingBlockKind(sv); got != "ordered_list" {
		t.Errorf("FieldPath path: want ordered_list, got %q", got)
	}
	sv = types.ScoredViolation{
		Detail: "required block kind=summary appears 0 times",
	}
	if got := extractMissingBlockKind(sv); got != "summary" {
		t.Errorf("Detail path: want summary, got %q", got)
	}
}

// TestExtractFacetIDFromDetail
func TestExtractFacetIDFromDetail(t *testing.T) {
	sv := types.ScoredViolation{
		Detail: `required facet "uncertainty_boundary" (essential) is not covered`,
	}
	if got := extractFacetIDFromDetail(sv); got != "uncertainty_boundary" {
		t.Errorf("want uncertainty_boundary, got %q", got)
	}
}
