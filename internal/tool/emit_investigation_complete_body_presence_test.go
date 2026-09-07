package tool

import (
	"fmt"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func callablePresenceContext(t *testing.T, presence repotypes.CallableBodyPresence) (*types.BusContext, []types.EvidenceItem) {
	graph, fi := mechanismSemanticDescentFixture()
	fi.Symbols[1].BodyPresence = presence
	ctx := mechanismSemanticDescentContext(t, graph, 12, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "routing", Entities: []string{"rewrite"}, EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")}},
		{Summary: "configuration", Entities: []string{"config"}},
	}
	evidence := []types.EvidenceItem{{
		ID: "call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Render", Object: "rewrite", AnchorSymbol: "rewrite",
		Source: fi.RelPath, LineStart: 3, Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
	}}
	return ctx, evidence
}

func TestCallableBodyPresenceOnlyPrecisePositiveCreatesBodyDebt(t *testing.T) {
	for _, presence := range []repotypes.CallableBodyPresence{repotypes.CallableBodyPresent, repotypes.CallableBodyAbsent, repotypes.CallableBodyUnknown, "future"} {
		t.Run(string(presence), func(t *testing.T) {
			ctx, evidence := callablePresenceContext(t, presence)
			got := preCompleteContractCheckWithEvidence(ctx, "", evidence)
			if presence == repotypes.CallableBodyPresent {
				if !strings.Contains(got, "implementation-body evidence") {
					t.Fatalf("real body debt silently removed: %q", got)
				}
			} else {
				if strings.Contains(got, "implementation-body evidence") || len(ctx.Mutable.EvidenceClosure().PendingReads()) > 0 {
					t.Fatalf("non-positive body became a hard read/emit obligation: %q", got)
				}
				note := callableBodyInspectionAdvisory(ctx, nil, evidence)
				want := "body presence is not established"
				if presence == repotypes.CallableBodyAbsent {
					want = "declaration without a local body"
				}
				if !strings.Contains(note, want) || !strings.Contains(note, "non-blocking") {
					t.Fatalf("honest soft boundary missing: %q", note)
				}
			}
		})
	}
}

func TestCallableBodyPresenceSingleLineSignatureDoesNotBecomeBodyEvidence(t *testing.T) {
	ctx, evidence := callablePresenceContext(t, repotypes.CallableBodyAbsent)
	fi := ctx.Mutable.SearchGraph().(*repotypes.Graph).Files[0]
	sym := &fi.Symbols[1]
	sym.EndLine = sym.Line
	definition := types.EvidenceItem{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorDefinition, AnchorSymbol: sym.Name, Source: fi.RelPath, LineStart: sym.Line,
		Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded}
	evidence = append(evidence, definition)
	if requestedSubTopicCallableHasBodyEvidence(evidence, fi.RelPath, sym) {
		t.Fatal("signature definition was upgraded into implementation evidence")
	}
	sym.BodyPresence = repotypes.CallableBodyPresent
	sym.BodyStartLine, sym.BodyEndLine = sym.Line, sym.Line
	if !requestedSubTopicCallableHasBodyEvidence(evidence, fi.RelPath, sym) {
		t.Fatal("parser-proven one-line real body no longer permits inspected source coverage")
	}
	if types.ClaimFormOf(definition) != types.ClaimDefinitionFact {
		t.Fatal("body inspection changed definition semantics")
	}
	// A multi-line implementation still requires at least one actual body line.
	sym.EndLine = sym.Line + 2
	sym.BodyEndLine = sym.EndLine
	if requestedSubTopicCallableHasBodyEvidence(evidence, fi.RelPath, sym) {
		t.Fatal("real multi-line body was waived by a definition-site-only row")
	}
}

func TestCallableBodyPresenceDoesNotSelectAnImplementationThroughDeclarationFiltering(t *testing.T) {
	ctx, evidence := callablePresenceContext(t, repotypes.CallableBodyAbsent)
	graph := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	fi := graph.Files[0]
	implementation := fi.Symbols[1]
	implementation.Line, implementation.EndLine, implementation.BodyPresence = 20, 22, repotypes.CallableBodyPresent
	implementation.BodyStartLine, implementation.BodyEndLine = 20, 22
	fi.Symbols = append(fi.Symbols, implementation)
	if _, _, _, ok := requestedSubTopicUniqueCallable(graph, "rewrite"); ok {
		t.Fatal("declaration and implementation were silently collapsed by body presence")
	}
	if got := requestedSubTopicCallableBodyDebts(ctx.AnalysisIR.RequestModel.SubTopics, graph, evidence); len(got) != 0 {
		t.Fatalf("ambiguous symbol acquired body debt: %+v", got)
	}
}

func TestCallableBodyPresenceUnknownSemanticDescentIsNotForcedRead(t *testing.T) {
	for _, presence := range []repotypes.CallableBodyPresence{repotypes.CallableBodyAbsent, repotypes.CallableBodyUnknown} {
		ctx, evidence := callablePresenceContext(t, presence)
		fi := ctx.Mutable.SearchGraph().(*repotypes.Graph).Files[0]
		sym := &fi.Symbols[1]
		ctx.Mutable.EvidenceClosure().SetReadRanges(map[string][]types.LineRange{fi.RelPath: {{Start: sym.Line, End: sym.Line}}})
		evidence = append(evidence, types.EvidenceItem{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorDefinition, AnchorSymbol: sym.Name, Source: fi.RelPath, LineStart: sym.Line, Scope: types.ScopeLine,
			Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded})
		facts := []types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet, Members: []string{"rewrite"}, MemberNotes: []string{"behavior to investigate"}, SupportRefs: []string{"src/pipeline.go:6"}}}
		if n := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), facts, evidence); n != 0 {
			t.Fatalf("unproven body created %d forced reads", n)
		}
	}
}

func TestCallableBodyPresenceAdvisoryUsesCurrentSnapshotAndDoesNotOverwriteOtherNotes(t *testing.T) {
	ctx, evidence := callablePresenceContext(t, repotypes.CallableBodyUnknown)
	graph := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	fi := graph.Files[0]
	ctx.Mutable.AppendCompletionGateNote("existing-note")
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("callable%d", i)
		fi.Symbols = append(fi.Symbols, repotypes.Symbol{Name: name, Kind: "function", File: fi.RelPath, Line: 30 + i, EndLine: 30 + i})
		ctx.AnalysisIR.RequestModel.SubTopics[0].Entities = append(ctx.AnalysisIR.RequestModel.SubTopics[0].Entities, name)
		ctx.AnalysisIR.RequestModel.SubTopics[0].EntityProvenance = append(ctx.AnalysisIR.RequestModel.SubTopics[0].EntityProvenance, requestedSubTopicSymbolProvenance(name))
		row := evidence[0]
		row.ID, row.Object, row.AnchorSymbol = name, name, name
		evidence = append(evidence, row)
	}
	first := callableBodyInspectionAdvisory(ctx, nil, evidence)
	if !strings.Contains(first, "showing 4 of 7") || strings.Count(first, "body presence is not established") != 4 {
		t.Fatalf("advisory cap/full-census disclosure lost: %q", first)
	}
	if second := callableBodyInspectionAdvisory(ctx, nil, evidence); second != first {
		t.Fatal("identical snapshot accumulated advice")
	}
	for i := range fi.Symbols {
		fi.Symbols[i].BodyPresence = repotypes.CallableBodyPresent
		fi.Symbols[i].BodyStartLine, fi.Symbols[i].BodyEndLine = fi.Symbols[i].Line, fi.Symbols[i].EndLine
	}
	if got := callableBodyInspectionAdvisory(ctx, nil, evidence); got != "" {
		t.Fatalf("stale unknown survived precise upgrade: %q", got)
	}
	if got := ctx.Mutable.TakeCompletionGateNote(); got != "existing-note" {
		t.Fatalf("other gate note overwritten: %q", got)
	}
}

func TestCallableBodyPresenceMultilineSignatureIsNotBodyEvidence(t *testing.T) {
	ctx, _ := callablePresenceContext(t, repotypes.CallableBodyPresent)
	fi := ctx.Mutable.SearchGraph().(*repotypes.Graph).Files[0]
	sym := &fi.Symbols[1]
	// Explicit parser fixture: declaration starts at 6, parameters continue
	// through 8, the concrete block is 9..11. No line-count inference.
	sym.EndLine, sym.BodyStartLine, sym.BodyEndLine = 11, 9, 11
	row := types.EvidenceItem{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorDefinition, AnchorSymbol: sym.Name,
		Source: fi.RelPath, Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded}
	for _, line := range []int{6, 7, 8} {
		row.LineStart, row.LineEnd = line, line
		if requestedSubTopicCallableHasBodyEvidence([]types.EvidenceItem{row}, fi.RelPath, sym) {
			t.Fatalf("signature line %d became body evidence", line)
		}
	}
	for _, line := range []int{9, 10} {
		row.LineStart, row.LineEnd = line, line
		if !requestedSubTopicCallableHasBodyEvidence([]types.EvidenceItem{row}, fi.RelPath, sym) {
			t.Fatal("actual body line was lost")
		}
	}
}
