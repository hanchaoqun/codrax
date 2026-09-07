package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B1577: the citation preview limit must not turn a same-file short-name
// ambiguity into a unique callable definition. These fixtures carry the
// producer-stamped owner authority actually used by the production matcher.
func TestCallableCitationCensusPrecedesDisplayLimit(t *testing.T) {
	for _, conflictFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_first=%t", conflictFirst), func(t *testing.T) {
			entries := citationCensusEntries("src/service.cj", "run", conflictFirst)
			got := renderAnswerDocCallChainCitationAuthority(citationCensusPlan(entries))
			line := citationCensusRow(t, got, "A.run")
			if !strings.Contains(line, "definition_status=`unproven`") || strings.Contains(line, "definition_ref=") {
				t.Fatalf("the short definition also matches B.run in the same file, even when B is outside the preview:\n%s", line)
			}
			if !strings.Contains(line, "callsite_refs=`src/service.cj:20`") ||
				!strings.Contains(line, "body_call_facts=`A.run -> Sink.consume @ src/service.cj:20`") {
				t.Fatalf("withholding an ambiguous definition must preserve the exact invocation facts:\n%s", line)
			}
			if gotCount := strings.Count(got, "- callable["); gotCount != maxAnswerDocCallableCitationRows {
				t.Fatalf("preview has %d rows, want display limit %d", gotCount, maxAnswerDocCallableCitationRows)
			}
			if !strings.Contains(got, "showing 12 of 13 observed callables") {
				t.Fatalf("bounded preview must disclose the full observed census:\n%s", got)
			}
		})
	}
}

func TestCallableCitationCensusKeepsSourceAndQualifiedIdentityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		conflictSource string
		definition     string
	}{
		{name: "other_file_does_not_make_local_definition_ambiguous", conflictSource: "src/other.cj", definition: "run"},
		{name: "qualified_definition_does_not_match_sibling_owner", conflictSource: "src/service.cj", definition: "A.run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, conflictFirst := range []bool{false, true} {
				got := renderAnswerDocCallChainCitationAuthority(citationCensusPlan(citationCensusEntries(tc.conflictSource, tc.definition, conflictFirst)))
				line := citationCensusRow(t, got, "A.run")
				if !strings.Contains(line, "definition_status=`proved`; definition_ref=`src/service.cj:10`; definition_evidence=`short-def`") {
					t.Fatalf("unrelated identities must not revoke a unique source-bound definition (conflict first=%t):\n%s", conflictFirst, line)
				}
			}
		})
	}
}

func TestCallableCitationCensusIsUsedByFinalizerInstruction(t *testing.T) {
	for _, conflictFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_first=%t", conflictFirst), func(t *testing.T) {
			entries := citationCensusEntries("src/service.cj", "run", conflictFirst)
			ctx := &types.AgentContext{
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
				}},
			}
			for _, entry := range entries {
				item := types.EvidenceItem{
					ID: entry.EvidenceID, Source: entry.Source, LineStart: entry.LineStart,
					Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
					Subject: entry.Subject, Object: entry.Object, OwnerSymbol: entry.OwnerSymbol,
					AnchorSymbol: entry.AnchorSymbol, Producer: entry.Producer,
				}
				if entry.ClaimForm == types.ClaimCallEdge {
					item.Kind, item.AnchorKind = types.EvidenceRelationship, types.AnchorCall
				} else {
					item.Kind, item.AnchorKind = types.EvidenceDirect, types.AnchorDefinition
				}
				ctx.EvidenceItems = append(ctx.EvidenceItems, item)
			}
			got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			line := citationCensusRow(t, got, "A.run")
			if !strings.Contains(line, "definition_status=`unproven`") || strings.Contains(line, "definition_ref=") {
				t.Fatalf("finalizer was taught a false unique definition after display truncation:\n%s", line)
			}
			if strings.Count(got, "- callable[") != maxAnswerDocCallableCitationRows ||
				!strings.Contains(got, "showing 12 of 13 observed callables") {
				t.Fatalf("finalizer did not receive a bounded, disclosed callable preview:\n%s", got)
			}
		})
	}
}

func citationCensusEntries(conflictSource, definition string, conflictFirst bool) []types.AnswerSupportEntry {
	call := func(id, caller, callee, source string, line int) types.AnswerSupportEntry {
		return types.AnswerSupportEntry{
			EvidenceID: id, ClaimForm: types.ClaimCallEdge, Subject: caller, Object: callee,
			OwnerSymbol: caller, Producer: types.EvidenceProducerExplorerEmitEvidence,
			Source: source, LineStart: line, Location: fmt.Sprintf("%s:%d", source, line),
		}
	}
	entries := []types.AnswerSupportEntry{call("a-call", "A.run", "Sink.consume", "src/service.cj", 20)}
	conflict := call("b-call", "B.run", "Sink.consume", conflictSource, 40)
	if conflictFirst {
		entries = append(entries, conflict)
	}
	for i := 0; i < 5; i++ {
		entries = append(entries, call(fmt.Sprintf("filler-%d", i), fmt.Sprintf("Helper%d.prepare", i),
			fmt.Sprintf("Target%d.consume", i), "src/service.cj", 21+i))
	}
	if !conflictFirst {
		entries = append(entries, conflict)
	}
	return append(entries, types.AnswerSupportEntry{
		EvidenceID: "short-def", ClaimForm: types.ClaimDefinitionFact, AnchorSymbol: definition,
		Source: "src/service.cj", LineStart: 10, Location: "src/service.cj:10",
	})
}

func citationCensusPlan(entries []types.AnswerSupportEntry) *types.AnswerSupportPlan {
	return &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath, Entries: entries,
	}}}
}

func citationCensusRow(t *testing.T, text, identity string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "- callable[") && strings.Contains(line, "identity=`"+identity+"`;") {
			return line
		}
	}
	t.Fatalf("missing callable row for %s:\n%s", identity, text)
	return ""
}
