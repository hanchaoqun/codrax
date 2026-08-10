package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCallChainCitationAuthoritySeparatesCallsitesAndDefinitions(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "py-native", ClaimForm: types.ClaimCallEdge, Subject: "FastTokenizer.tokenize", Object: "_fastlex.tokenize_bytes", OwnerSymbol: "FastTokenizer.tokenize", Source: "tokenizer.py", Location: "tokenizer.py:21", Producer: types.EvidenceProducerExplorerEmitEvidence},
			{EvidenceID: "py-slow", ClaimForm: types.ClaimCallEdge, Subject: "FastTokenizer.tokenize", Object: "_tokenize_slow", OwnerSymbol: "FastTokenizer.tokenize", Source: "tokenizer.py", Location: "tokenizer.py:22", Producer: types.EvidenceProducerExplorerEmitEvidence},
			{EvidenceID: "rust-call", ClaimForm: types.ClaimCallEdge, Subject: "py::tokenize_bytes", Object: "super::tokenize_bytes", OwnerSymbol: "py::tokenize_bytes", Source: "lib.rs", Location: "lib.rs:42", Producer: types.EvidenceProducerExplorerEmitEvidence},
			{EvidenceID: "py-def", ClaimForm: types.ClaimDefinitionFact, AnchorSymbol: "tokenize", Source: "tokenizer.py", Location: "tokenizer.py:18"},
			{EvidenceID: "wrapper-def", ClaimForm: types.ClaimDefinitionFact, AnchorSymbol: "tokenize_bytes", Source: "lib.rs", Location: "lib.rs:40"},
		},
	}}}

	got := renderAnswerDocCallChainCitationAuthority(plan)
	for _, want := range []string{
		"identity=`FastTokenizer.tokenize`",
		"callsite_refs=`tokenizer.py:21 | tokenizer.py:22`",
		"definition_status=`proved`; definition_ref=`tokenizer.py:18`",
		"identity=`py::tokenize_bytes`",
		"callsite_refs=`lib.rs:42`; definition_status=`proved`; definition_ref=`lib.rs:40`",
		"identity=`_tokenize_slow`",
		"definition_status=`unproven`",
		"identity=`super::tokenize_bytes`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("callable citation authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "definition_ref=`lib.rs:42`") || strings.Contains(got, "definition_ref=`tokenizer.py:22`") {
		t.Fatalf("call sites must never be relabeled as definition locations:\n%s", got)
	}
}

func TestCallChainCitationAuthorityFailsClosedOnSameTailAmbiguityAcrossLanguages(t *testing.T) {
	for _, source := range []string{"src/main.go", "src/main.ets", "src/main.cj"} {
		plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{
				{EvidenceID: "a-call", ClaimForm: types.ClaimCallEdge, Subject: "Entry", Object: "A.run", Source: source, Location: source + ":20"},
				{EvidenceID: "b-call", ClaimForm: types.ClaimCallEdge, Subject: "Entry", Object: "B.run", Source: source, Location: source + ":21"},
				{EvidenceID: "short-def", ClaimForm: types.ClaimDefinitionFact, AnchorSymbol: "run", Source: source, Location: source + ":30"},
			},
		}}}
		got := renderAnswerDocCallChainCitationAuthority(plan)
		if strings.Contains(got, "definition_ref=`"+source+":30`") {
			t.Fatalf("same-tail ambiguity in %s must not assign one short definition to either callable:\n%s", source, got)
		}
	}
}

func TestCallChainCitationAuthorityDoesNotTrustUnstampedOwner(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "call", ClaimForm: types.ClaimCallEdge, Subject: "pkg::run", Object: "pkg::next", OwnerSymbol: "pkg::run", Source: "src/main.rs", Location: "src/main.rs:20"},
			{EvidenceID: "def", ClaimForm: types.ClaimDefinitionFact, AnchorSymbol: "run", Source: "src/main.rs", Location: "src/main.rs:10"},
		},
	}}}
	got := renderAnswerDocCallChainCitationAuthority(plan)
	if !strings.Contains(got, "identity=`pkg::run`") || !strings.Contains(got, "definition_status=`unproven`") ||
		strings.Contains(got, "definition_ref=`src/main.rs:10`") {
		t.Fatalf("model-shaped owner without typed producer must not qualify a short definition:\n%s", got)
	}
}

func TestCallChainCitationAuthorityIsWiredIntoFinalizerPrompt(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "A.run", Object: "B.run", OwnerSymbol: "A.run", Source: "src/main.cj", LineStart: 20, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence},
			{ID: "def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "B.run", Source: "src/main.cj", LineStart: 30, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		},
	}
	got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"Callable role and citation authority", "identity=`B.run`", "definition_ref=`src/main.cj:30`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("finalizer prompt lost callable citation authority %q:\n%s", want, got)
		}
	}
}
