package types

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func b1604EnumerationEvidence() []EvidenceItem {
	var evidence []EvidenceItem
	for _, member := range []struct {
		name, base, path string
		line             int
	}{
		{"ConsoleSink", "Sink", "include/logx/console_sink.hpp", 8},
		{"FileSink", "Sink", "include/logx/file_sink.hpp", 10},
		{"RotatingSink", "FileSink", "include/logx/rotating_sink.hpp", 10},
	} {
		for i, item := range []EvidenceItem{
			{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, LineStart: member.line, Snippet: fmt.Sprintf("class %s : public %s {", member.name, member.base)},
			{Kind: EvidenceMechanism, AnchorKind: AnchorDefinition, LineStart: member.line - 1, Predicate: "documents", Object: member.name, Snippet: member.name + " documents " + member.name},
			{Kind: EvidenceRelationship, AnchorKind: AnchorDefinition, LineStart: member.line, Predicate: "inheritance", Object: member.base, Snippet: fmt.Sprintf("%s inheritance %s", member.name, member.base)},
			{Kind: EvidenceRelationship, AnchorKind: AnchorImport, AnchorSymbol: member.base, LineStart: 3, Predicate: "声明继承", Object: member.base, Snippet: fmt.Sprintf("#include \"%s.hpp\"", member.base)},
		} {
			item.ID = fmt.Sprintf("%s-%d", member.name, i)
			item.Source, item.Subject = member.path, member.name
			if item.AnchorSymbol == "" {
				item.AnchorSymbol = member.name
			}
			item.Scope, item.GroundingStatus, item.Origin = ScopeLine, GroundingGrounded, ClaimOriginCurrentRepo
			item.Producer = "explorer.emit_evidence"
			evidence = append(evidence, item)
		}
	}
	return append(evidence, EvidenceItem{
		ID: "base-definition", Kind: EvidenceMechanism, Scope: ScopeLine,
		Source: "include/logx/sink.hpp", LineStart: 8, AnchorKind: AnchorDefinition,
		AnchorSymbol: "Sink", Subject: "Sink", Snippet: "class Sink {",
		GroundingStatus: GroundingGrounded, Origin: ClaimOriginCurrentRepo, Producer: "explorer.emit_evidence",
	})
}

func b1604RelationEnumerationRequest() RequestModel {
	return RequestModel{Intent: IntentEnumerate, PredicateAxis: AxisImplement,
		Predicates:             SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
		CompletenessObligation: &CompletenessObligation{Required: true},
		SourceInventoryProfile: &SourceInventoryProfile{RequestedFields: []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation}},
	}
}

func TestB1604RelationCandidatesDoNotBecomeClosedMemberObligations(t *testing.T) {
	for _, transformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed_and_reversed=%t", transformed), func(t *testing.T) {
			rm := b1604RelationEnumerationRequest()
			evidence := b1604EnumerationEvidence()
			if transformed {
				rename := strings.NewReplacer("ConsoleSink", "TerminalOutput", "FileSink", "DiskOutput", "RotatingSink", "ArchiveOutput", "Sink", "Output")
				for i := range evidence {
					evidence[i].Subject = rename.Replace(evidence[i].Subject)
					evidence[i].Object = rename.Replace(evidence[i].Object)
					evidence[i].AnchorSymbol = rename.Replace(evidence[i].AnchorSymbol)
					evidence[i].Snippet = rename.Replace(evidence[i].Snippet)
					evidence[i].Source = "renamed/" + evidence[i].Source
				}
				for i, j := 0, len(evidence)-1; i < j; i, j = i+1, j-1 {
					evidence[i], evidence[j] = evidence[j], evidence[i]
				}
			}
			plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: rm}, nil, nil, nil, nil, evidence)
			support := BuildAnswerSupportPlan(rm, plan)
			if support == nil || support.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyEnrichmentOnly {
				t.Fatalf("unproven relation roster acquired a completeness obligation: %+v", support)
			}
			if obligations := PrincipalSupportMemberObligations(support); len(obligations) != 0 {
				t.Fatalf("definition candidates, including the base type, became mandatory members: %+v", obligations)
			}
			seen := map[string]bool{}
			for _, lane := range support.Lanes {
				for _, entry := range lane.Entries {
					seen[entry.EvidenceID] = true
				}
			}
			for _, id := range []string{"ConsoleSink-0", "FileSink-0", "RotatingSink-0", "base-definition"} {
				if !seen[id] {
					t.Errorf("valid local definition %s disappeared behind a hot-file boundary", id)
				}
			}
			lane := answerSupportLaneByKind(support, SupportLanePrincipalEvidence)
			if lane == nil || !strings.Contains(lane.Guidance, "not a proved relation-member set") {
				t.Fatalf("prompt must explain the exact membership boundary, not simply hide warnings: %+v", lane)
			}
		})
	}
}

func TestB1604EvidenceSequenceAuthorityIsNotCollectionMembership(t *testing.T) {
	irByFamily := map[QuestionFamily]*AnalysisIR{
		QFRootCauseTrace: irForRootCauseTrace(), QFCallChain: irForCallChain(), QFEnumeration: irForEnumeration(),
		QFConfigPrecedence: irForConfigPrecedence(), QFRoleLookup: irForRoleLookup(), QFArchitecture: irForArchitecture(),
		QFComparison: irForComparison(), QFGeneric: irForGeneric(),
	}
	var sameFile []EvidenceItem
	for i := 0; i < 3; i++ {
		sameFile = append(sameFile, EvidenceItem{ID: fmt.Sprintf("call-%d", i), Kind: EvidenceDirect, Source: "src/flow.go", LineStart: 10 + i,
			AnchorKind: AnchorCall, AnchorSymbol: fmt.Sprintf("Step%d", i), GroundingStatus: GroundingGrounded})
	}
	for _, family := range AllQuestionFamilies() {
		t.Run(string(family), func(t *testing.T) {
			ir := irByFamily[family]
			if ir == nil || ResolveQuestionFamily(ir.RequestModel) != family {
				t.Fatalf("missing accurate family fixture %s", family)
			}
			plan := &AnswerSurfacePlan{}
			ApplyEvidenceStepBackbone(plan, ir, sameFile)
			want := 0
			if family == QFCallChain || family == QFRootCauseTrace {
				want = 3
			}
			if len(plan.StepBackbone) != want {
				t.Fatalf("family=%s generic evidence backbone has %d rows, want %d", family, len(plan.StepBackbone), want)
			}
		})
	}
}

func TestB1604AcceptedSymbolSlateRetainsExactCoverageWithoutCandidateExpansion(t *testing.T) {
	rm := b1604RelationEnumerationRequest()
	ir := &AnalysisIR{RequestModel: rm}
	symbols := []AnswerSymbol{{Name: "ConsoleSink", File: "include/logx/console_sink.hpp", Line: 8, Kind: KindType}, {Name: "FileSink", File: "include/logx/file_sink.hpp", Line: 10, Kind: KindType}}
	for _, claim := range []CompletenessClaim{CompletenessComplete, CompletenessLowerBound} {
		t.Run(string(claim), func(t *testing.T) {
			mutable := NewMutableState("")
			mutable.SetEmittedAnswerSymbolsWithOrigin(symbols, claim, AnswerSymbolSelectionExplicitItems)
			plan := BuildAnswerSurfacePlan(ir, mutable, nil, nil, nil, b1604EnumerationEvidence())
			if !reflect.DeepEqual(plan.StepBackbone, compileStepSurfaceAnchors(symbols)) {
				t.Fatalf("generic candidates expanded or replaced the accepted symbol slate: %+v", plan.StepBackbone)
			}
			support := BuildAnswerSupportPlan(rm, plan)
			if support.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyRequired || len(PrincipalSupportMemberObligations(support)) != 2 {
				t.Fatalf("accepted exact symbols lost their own member obligations: %+v", support)
			}
			for _, change := range []string{"same", "subset", "identity", "location", "expand", "source"} {
				t.Run(change, func(t *testing.T) {
					reapplied := cloneAnswerSurfacePlan(plan)
					input := append([]AnswerSymbol(nil), symbols...)
					switch change {
					case "subset":
						input = input[:1]
					case "identity":
						input[0].Name = "OtherSink"
					case "location":
						input[0].Line++
					case "source":
						input[0].File = "other/console_sink.hpp"
					case "expand":
						input = append(input, AnswerSymbol{Name: "RotatingSink", File: "include/logx/rotating_sink.hpp", Line: 10, Kind: KindType})
					}
					ApplyAnswerSymbolStepBackbone(reapplied, ir, input, claim)
					got := BuildAnswerSupportPlan(rm, reapplied)
					want := PrincipalMemberCoveragePolicyEnrichmentOnly
					if change == "same" || change == "subset" {
						want = PrincipalMemberCoveragePolicyRequired
					}
					if got.PrincipalMemberCoverage != want {
						t.Fatalf("%s reapplied slate gained/lost accepted identity authority: %s want %s", change, got.PrincipalMemberCoverage, want)
					}
				})
			}
		})
	}
	// A context-projected, automatically synthesized slate has the same
	// shape, but was never accepted through the symbol tool's mutable buffer.
	plan := BuildAnswerSurfacePlan(ir, nil, nil, nil, nil, b1604EnumerationEvidence())
	ApplyAnswerSymbolStepBackbone(plan, ir, symbols, CompletenessComplete)
	if got := BuildAnswerSupportPlan(rm, plan); got.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyEnrichmentOnly {
		t.Fatalf("zero provenance inferred symbol authority from its name/shape: %+v", got)
	}
}

func TestB1604MarkedRelationSetKeepsRequiredMembership(t *testing.T) {
	rm := b1604RelationEnumerationRequest()
	mutable := NewMutableState("")
	mutable.SetInvestigationAggregateFacts([]AnswerAggregateFact{{
		Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer, Label: "verified implementers", Value: "3",
		Members:     []string{"ConsoleSink", "FileSink", "RotatingSink"},
		SupportRefs: []string{"ConsoleSink @ include/logx/console_sink.hpp:8", "FileSink @ include/logx/file_sink.hpp:10", "RotatingSink @ include/logx/rotating_sink.hpp:10"},
		Provenance:  TypedRelationPrincipalMemberSetAggregateProvenance,
	}})
	mutable.RetainInvestigationAggregateFacts()
	plan := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: rm}, mutable, nil, nil, nil, b1604EnumerationEvidence())
	support := BuildAnswerSupportPlan(rm, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if support.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyRequired || len(obligations) != 3 {
		t.Fatalf("proved relation roster lost its exact member contract: %+v / %+v", support, obligations)
	}
	for _, ob := range obligations {
		if ob.Label == "Sink" {
			t.Fatalf("base type expanded the proved relation set: %+v", obligations)
		}
	}
}
