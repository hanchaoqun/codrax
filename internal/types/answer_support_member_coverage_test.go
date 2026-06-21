package types

import "testing"

func TestAnswerSupportMemberObligationStableHandoffKeys(t *testing.T) {
	ob := AnswerSupportMemberObligation{
		Label:    "StageAnalyze",
		Location: "internal/types/context.go:42",
		EquivalentLocations: []string{
			"internal/types/context.go:43",
			"internal/types/context.go:44",
		},
	}
	if got, want := ob.StableItemID(), "support-stageanalyze"; got != want {
		t.Fatalf("StableItemID = %q, want %q", got, want)
	}
	if got, want := ob.StableCitationKey(), "internal/types/context.go:42"; got != want {
		t.Fatalf("StableCitationKey = %q, want %q", got, want)
	}
}

func TestAnswerSupportMemberObligationStableItemIDFallsBackToLocation(t *testing.T) {
	ob := AnswerSupportMemberObligation{
		Location: "internal/analysis/criterion/eval.go:17",
	}
	if got, want := ob.StableItemID(), "support-internal_analysis_criterion_eval_go_17"; got != want {
		t.Fatalf("StableItemID fallback = %q, want %q", got, want)
	}
}

func TestPrincipalSupportMemberObligations_StripsAggregateCitationQualifier(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					EvidenceID:    "aggregate_fact:member_set:1:0",
					Text:          "Kind",
					Location:      "kind: internal/analysis/criterion/grammar.go:26",
					SurfaceTerms:  []string{"Kind"},
					LabelSurface:  ClaimLabelSurfaceDisplayLabel,
					MemberSurface: PrincipalMemberSurfaceDisplayLabel,
				},
				{
					EvidenceID:    "aggregate_fact:member_set:2:0",
					Text:          "Kind definition",
					Location:      "internal/analysis/criterion/grammar.go:26",
					Source:        "internal/analysis/criterion/grammar.go",
					LineStart:     26,
					ClaimForm:     ClaimDefinitionFact,
					AnchorSymbol:  "Kind",
					SurfaceTerms:  []string{"Kind"},
					LabelSurface:  ClaimLabelSurfaceSymbolLike,
					MemberSurface: PrincipalMemberSurfaceSymbolLike,
				},
			},
		}},
	}

	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) != 1 {
		t.Fatalf("same aggregate member/location should coalesce across claim lanes, got %+v", obligations)
	}
	if got, want := obligations[0].Source, "internal/analysis/criterion/grammar.go"; got != want {
		t.Fatalf("qualified citation source = %q, want %q", got, want)
	}
	if got, want := obligations[0].Location, "internal/analysis/criterion/grammar.go:26"; got != want {
		t.Fatalf("qualified citation location = %q, want %q", got, want)
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/analysis/criterion/grammar.go", Line: 26}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Text:        "| 名称 | 定义位置 |\n|---|---|\n| Kind | grammar.go:26 |",
			Items: []AnswerBlockItem{{
				ID:          "support-kind",
				Label:       "Kind",
				CitationRef: 0,
			}},
		}},
	}
	if missing := MissingPrincipalSupportMembers(doc, plan); len(missing) != 0 {
		t.Fatalf("real file:line citation should satisfy coalesced aggregate member, got %+v", missing)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsPrincipalSectionItems(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:          "Cart extend block",
				Location:      "cart/Cart.cj:30",
				Source:        "cart/Cart.cj",
				LineStart:     30,
				ClaimForm:     ClaimDefinitionFact,
				Subject:       "Cart",
				SurfaceTerms:  []string{"Cart"},
				LabelSurface:  ClaimLabelSurfaceSymbolLike,
				MemberSurface: PrincipalMemberSurfaceSymbolLike,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "cart/Cart.cj", Line: 30}},
		Blocks: []AnswerBlock{{
			ID:          "extend",
			Kind:        BlockSection,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				ID:          "cart",
				Label:       "Cart",
				Text:        "extend block",
				CitationRef: 0,
			}},
		}},
	}
	if missing := MissingPrincipalSupportMembers(doc, plan); len(missing) != 0 {
		t.Fatalf("principal section items should satisfy structured member coverage, got %+v", missing)
	}

	doc.Blocks[0].SurfaceRole = ""
	doc.Blocks[0].FacetIDs = nil
	if missing := MissingPrincipalSupportMembers(doc, plan); len(missing) != 1 {
		t.Fatalf("unannotated section items should not become principal carriers, got %+v", missing)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsBlockTextWithCitationSidecars(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					EvidenceID:    "aggregate_fact:member_set:0:0",
					Text:          "extend Cart",
					Location:      "cart/Cart.cj:30",
					Source:        "cart/Cart.cj",
					LineStart:     30,
					ClaimForm:     ClaimDefinitionFact,
					Subject:       "extend Cart",
					SurfaceTerms:  []string{"extend Cart", "Cart"},
					LabelSurface:  ClaimLabelSurfaceSymbolLike,
					MemberSurface: PrincipalMemberSurfaceSymbolLike,
				},
				{
					EvidenceID:    "aggregate_fact:member_set:1:0",
					Text:          "native_add",
					Location:      "bridge/Bridge.cj:6",
					Source:        "bridge/Bridge.cj",
					LineStart:     6,
					ClaimForm:     ClaimDefinitionFact,
					Subject:       "native_add",
					SurfaceTerms:  []string{"native_add"},
					LabelSurface:  ClaimLabelSurfaceSymbolLike,
					MemberSurface: PrincipalMemberSurfaceSymbolLike,
				},
			},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "cart/Cart.cj", Line: 30},
			{File: "bridge/Bridge.cj", Line: 6},
		},
		Blocks: []AnswerBlock{{
			ID:          "table",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Text:        "| 维度 | 符号名 | 文件路径 |\n|---|---|---|\n| extend 块 | extend Cart | cart/Cart.cj:30 |\n| foreign func 声明 | native_add | bridge/Bridge.cj:6 |",
			Items: []AnswerBlockItem{
				{ID: "cart-cite", CitationRef: 0},
				{ID: "native-cite", CitationRef: 1},
			},
		}},
	}
	if missing := MissingPrincipalSupportMembers(doc, plan); len(missing) != 0 {
		t.Fatalf("block text plus matching citation sidecars should satisfy support-member coverage, got %+v", missing)
	}

	doc.Blocks[0].Text = "| 维度 | 符号名 | 文件路径 |\n|---|---|---|\n| extend 块 | extend Cart | cart/Cart.cj:30 |"
	missing := MissingPrincipalSupportMembers(doc, plan)
	if len(missing) != 1 || missing[0].Label != "native_add" {
		t.Fatalf("citation sidecar without visible member text must still be missing, got %+v", missing)
	}
}
