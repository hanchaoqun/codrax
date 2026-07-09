package orchestrator

// contract_check_block_enum_external_observation_test.go — INODE 批复核收尾
// P2-1 pin (§28.6 ⑫ 二阶可达面, 2026-07-09).
//
// 如实定性: the §28.6 ⑫ change added ClaimExternalObservation to
// compileEnumeration's AcceptableClaimForms as a HINT-surface fix (the
// block-contract prompt's "claim_form must be one of" list and the
// claim-use-missing violation's repair text). That list has a SECOND
// consumer: answerBlockCitationRoleForms (contract_check_block.go) folds the
// view requirements' AcceptableClaimForms into the citation-role alignment
// allowed set, and ClaimExternalObservation carries
// CitationRoleIdentityKind=DisplaySurface — so enumeration-family principal
// list/table items whose visible label names a runtime-artifact
// (external-observation) evidence surface but cite a DIFFERENT evidence now
// produce ViolClaimFormUnsupported. That violation is SoftByDefault
// (violation_registry.go: SeverityMedium, soft, promotable) — a retry-hint
// lane, never a hard reject — and the direction is protective (a mis-cited
// trace-surface row gets a repair hint naming the right citation), so the
// reachable-face extension ships as-is and THIS pin fixes the behavior so a
// future change cannot silently widen or drop it.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func enumExternalObservationAlignmentFixture(t *testing.T) (*types.AnswerSemanticView, *types.MutableState) {
	t.Helper()
	// The REAL enumeration compile (not a hand-built view): the reachability
	// under test rides compileEnumeration's AcceptableClaimForms.
	view := types.BuildAnswerSemanticView(&types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Scenario: types.ScenarioGeneric,
		},
	}, nil)
	if view == nil || view.Family != types.QFEnumeration {
		t.Fatalf("fixture must compile the enumeration family view: %+v", view)
	}
	mut := types.NewMutableState("which inodes are IO-hot")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			// Runtime-artifact observation row: Origin=log projects to
			// ClaimExternalObservation (claim_form.go priority 1), whose
			// citation-role identity is the DisplaySurface "hot.db".
			ID:              "obs-inode",
			Kind:            types.EvidenceDirect,
			Origin:          types.ClaimOriginLog,
			Source:          "customer.systrace",
			LineStart:       4210,
			Subject:         "hot.db",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			// A source-side definition at a different location — the
			// mis-citation target.
			ID:              "def-reader",
			Kind:            types.EvidenceDirect,
			Source:          "internal/io/reader.go",
			LineStart:       88,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "readHotFile",
			AnchorSymbol:    "readHotFile",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	return view, mut
}

func enumExternalObservationDoc(citation types.Citation) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Citations: []types.Citation{citation},
		Blocks: []types.AnswerBlock{{
			ID:   "inode-list",
			Kind: types.BlockOrderedList,
			// The enumeration principal facet — the block inherits the view
			// requirement's AcceptableClaimForms through the facet share;
			// deliberately NO block-level ClaimUses, so the alignment forms
			// come ONLY from the view lane (the newly reachable arm).
			FacetIDs: []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "i1",
				Label:       "hot.db",
				Text:        "highest IO event count in the selected window",
				CitationRef: 0,
			}},
		}},
	}
}

// TestEnumerationExternalObservationCitationRoleAlignment pins the P2-1
// reachable face end-to-end: mis-cited trace-surface row → ONE soft
// ViolClaimFormUnsupported naming the observation role and the repair
// citation; correctly cited row → no violation; and without the view lane
// (nil view, no block claim_uses) the face is unreachable — proving the
// extension rides exactly compileEnumeration's AcceptableClaimForms.
func TestEnumerationExternalObservationCitationRoleAlignment(t *testing.T) {
	view, mut := enumExternalObservationAlignmentFixture(t)

	// Arm 1 — role mismatch: the visible label names the runtime-artifact
	// surface "hot.db" but the citation points at the source definition.
	misCited := enumExternalObservationDoc(types.Citation{File: "internal/io/reader.go", Line: 88})
	vs := validateCallChainItemCitationRoleAlignment(misCited, view, mut)
	if len(vs) != 1 {
		t.Fatalf("expected one citation-role violation on the mis-cited enumeration row, got %d: %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolClaimFormUnsupported {
		t.Fatalf("kind = %q, want %q", vs[0].Kind, types.ViolClaimFormUnsupported)
	}
	if !strings.Contains(vs[0].Detail, "external_observation") || !strings.Contains(vs[0].Detail, "hot.db") {
		t.Fatalf("violation should name the external-observation display role, got %+v", vs[0])
	}
	if !strings.Contains(vs[0].Repair, "customer.systrace:4210") {
		t.Fatalf("repair should point at the matching runtime-artifact evidence, got %+v", vs[0])
	}
	// Soft-lane classification stays pinned: this face may hint, never hard-reject.
	spec, ok := types.ViolKindSpecFor(types.ViolClaimFormUnsupported)
	if !ok || !spec.SoftByDefault {
		t.Fatalf("ViolClaimFormUnsupported must stay soft-by-default (retry-hint lane): %+v", spec)
	}

	// Arm 2 — correct citation: same row citing the observation itself.
	aligned := enumExternalObservationDoc(types.Citation{File: "customer.systrace", Line: 4210})
	if vs := validateCallChainItemCitationRoleAlignment(aligned, view, mut); len(vs) != 0 {
		t.Fatalf("correctly cited trace-surface row must pass, got %+v", vs)
	}

	// Arm 3 — reachability provenance: without the view lane (and with no
	// block claim_uses) the enumeration block contributes no alignment
	// forms, so the same mis-cited doc produces nothing. The face is opened
	// exactly by compileEnumeration's AcceptableClaimForms.
	if vs := validateCallChainItemCitationRoleAlignment(misCited, nil, mut); len(vs) != 0 {
		t.Fatalf("without the view's claim-form lane the face must be unreachable, got %+v", vs)
	}
}
