package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryAnswerPreEmitAuthority_ProjectsCandidateUniverseGap(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "packages",
		Value:   "2",
		Members: []string{"alpha", "beta"},
	}}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, facts)
	if !auth.Active || !auth.Blocking {
		t.Fatalf("authority should expose the existing blocking gap, got %+v", auth)
	}
	if !auth.BestUniverseGap.Blocking ||
		auth.BestUniverseGap.Role != types.AnswerCandidateRolePackage ||
		len(auth.BestUniverseGap.Missing) != 1 ||
		auth.BestUniverseGap.Missing[0].Name != "gamma" {
		t.Fatalf("best universe gap did not preserve precise sensor result: %+v", auth.BestUniverseGap)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "candidate_universe_blocking") {
		t.Fatalf("reason codes should include candidate universe blocker, got %+v", auth.ReasonCodes)
	}
	if !auth.Snapshot.Active {
		t.Fatalf("authority should carry the shared snapshot for downstream consumers: %+v", auth.Snapshot)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_PreCompleteUsesSamePreciseCoverageView(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
			Confidence:        0.95,
		},
	}}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "packages",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"alpha", "beta"},
		SupportRefs: []string{"alpha: src/alpha:1", "beta: src/beta:1"},
	}}

	authority := BuildSourceInventoryAnswerPreEmitAuthority(ctx, facts)
	gap := sourceInventoryResolvedCompletionPreciseCoverageGap(ctx, facts)
	if !authority.BestUniverseGap.Blocking || !gap.Blocking {
		t.Fatalf("both pre-complete and pre-emit authority should see the exact typed gap: auth=%+v gap=%+v", authority.BestUniverseGap, gap)
	}
	if authority.BestUniverseGap.Scope != gap.Scope ||
		authority.BestUniverseGap.Role != gap.Role ||
		len(authority.BestUniverseGap.Missing) != len(gap.Missing) {
		t.Fatalf("pre-complete gap must be projected from the same authority view: auth=%+v gap=%+v",
			authority.BestUniverseGap, gap)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_BlocksOnSharedCompletionView(t *testing.T) {
	auth := SourceInventoryAnswerPreEmitAuthority{
		View: types.SourceInventoryAnswerAuthorityView{
			CanBlockCompletion: true,
			FollowupDebt: types.SourceInventoryFollowupDebt{
				Active:           true,
				ReasonCode:       types.SourceInventoryFollowupDebtMissingSourceClass,
				MissingClasses:   []types.SourcePathRole{types.SourcePathRoleFixture},
				CoveredClasses:   []types.SourcePathRole{types.SourcePathRoleThirdParty},
				MissingLanguages: []string{"cangjie"},
				Query: types.SourceInventoryLensQuery{
					Path:          ".",
					Scopes:        []string{"eval/fixtures/testdata/cangjie_minimal/bridge"},
					Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					IncludeCounts: true,
					TopN:          24,
				},
				Roles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			},
		},
	}
	auth.Blocking = sourceInventoryAnswerPreEmitBlocking(auth)
	auth.ReasonCodes = sourceInventoryAnswerPreEmitReasonCodes(auth)
	if !auth.Blocking {
		t.Fatalf("pre-emit authority must not drop shared view blocking state, got %+v", auth)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "view:block_completion") {
		t.Fatalf("reason codes should include the shared view blocker, got %+v", auth.ReasonCodes)
	}

	auth.View.CanBlockCompletion = false
	auth.View.CanOnlyCaveat = true
	auth.Blocking = sourceInventoryAnswerPreEmitBlocking(auth)
	auth.ReasonCodes = sourceInventoryAnswerPreEmitReasonCodes(auth)
	if auth.Blocking {
		t.Fatalf("caveat-only source inventory debt must not become a pre-emit hard blocker, got %+v", auth)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "view:caveat_only") {
		t.Fatalf("reason codes should preserve caveat-only state for status/handoff, got %+v", auth.ReasonCodes)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_ProjectsExactAbsenceDebt(t *testing.T) {
	mut := types.NewMutableState("source inventory absence")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleProduction,
			Count:    2,
			Complete: false,
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
		}},
	}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, nil)
	if !auth.Active || !auth.Blocking || !auth.ExactAbsenceBlocking {
		t.Fatalf("authority should expose exact absence proof debt, got %+v", auth)
	}
	if !strings.Contains(auth.ExactAbsenceSummary, "source_classes=") ||
		!strings.Contains(auth.ExactAbsenceSummary, "principal_roles=function") {
		t.Fatalf("absence summary should preserve typed proof surface, got %q", auth.ExactAbsenceSummary)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "exact_absence_requires_inventory_proof") {
		t.Fatalf("reason codes should include absence blocker, got %+v", auth.ReasonCodes)
	}
	summary, blocked := preCheckSourceInventoryExactAbsenceBound(ctx)
	if !blocked || summary != auth.ExactAbsenceSummary {
		t.Fatalf("pre-emit absence gate should consume the authority view: summary=%q blocked=%v auth=%q",
			summary, blocked, auth.ExactAbsenceSummary)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_ProjectsEnumerationDisplayCoverage(t *testing.T) {
	mut := types.NewMutableState("列出 package")
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "packages",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"alpha", "beta"},
		SupportRefs: []string{"alpha: src/alpha.go:3", "beta: src/beta.go:7"},
	}}
	mut.SetInvestigationAggregateFacts(facts)
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "src/alpha.go", Line: 3, Quote: "package alpha"},
			{File: "src/beta.go", Line: 7, Quote: "package beta"},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "packages",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{ID: "alpha", Label: "alpha", Text: "src/alpha.go:3", CitationRef: 0},
				{ID: "beta", Label: "beta", Text: "src/beta.go:7", CitationRef: 1},
			},
		}},
	}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, facts, doc)
	if auth.EnumerationSetCount != 1 || auth.EnumerationRowCount != 2 {
		t.Fatalf("authority should project accepted enumeration rows, got sets=%d rows=%d auth=%+v",
			auth.EnumerationSetCount, auth.EnumerationRowCount, auth)
	}
	if !auth.EnumerationCoverage.Complete() {
		t.Fatalf("authority should project visible row coverage, got %+v", auth.EnumerationCoverage)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "accepted_enumeration_rows_visible") {
		t.Fatalf("reason codes should include visible enumeration coverage, got %+v", auth.ReasonCodes)
	}
}

func TestSourceInventoryAnswerPreEmitAuthority_CarriesSharedAnswerAuthorityView(t *testing.T) {
	mut := types.NewMutableState("source inventory authority view")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"src"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Total:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "Serve",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "src/serve.cj",
				Line:          12,
				SupportRef:    "Serve: src/serve.cj:12",
				Language:      "cangjie",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.95,
			},
		}},
	}

	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, nil)
	if !auth.View.Active || len(auth.View.PrincipalRows) != 1 {
		t.Fatalf("pre-emit authority should carry the shared answer authority view, got %+v", auth.View)
	}
	if len(auth.View.CitationObligations) != 1 {
		t.Fatalf("view should expose citation obligations for pre-emit consumers, got %+v", auth.View.CitationObligations)
	}
	obligation := auth.View.CitationObligations[0]
	if obligation.Member != "Serve" || obligation.File != "src/serve.cj" || obligation.Line != 12 ||
		obligation.Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("unexpected citation obligation: %+v", obligation)
	}
	if !sourceInventoryAnswerPreEmitReasonCodeContains(auth, "view:citation_obligations") {
		t.Fatalf("pre-emit reason codes should include view citation obligations, got %+v", auth.ReasonCodes)
	}
}

func sourceInventoryAnswerPreEmitReasonCodeContains(auth SourceInventoryAnswerPreEmitAuthority, want string) bool {
	for _, got := range auth.ReasonCodes {
		if got == want {
			return true
		}
	}
	return false
}
