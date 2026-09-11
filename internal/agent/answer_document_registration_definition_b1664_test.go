package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the actual finalizer entry over an already-published typed evidence
// snapshot. The public parser/read/emit producer is independently covered by
// tool.TestB1664PublicRegistrationParameterTypeIsNotBindingSubject. Keeping the
// snapshot here also protects old reports: prompt projection may not upgrade a
// registration endpoint or descriptive term to a declaration alias.
func TestB1664BuildInitialInstructionDefinitionGuideDoesNotMintRegistrationAliases(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := &types.AgentContext{
				Language: lang,
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Language: lang, Intent: types.IntentExplain,
					Predicates:    types.SemanticPredicates{IsCrossComponent: true},
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
				}},
				Mutable: types.NewMutableState("Explain the selected native binding."),
				EvidenceItems: []types.EvidenceItem{
					{
						ID: "registration-snapshot", Kind: types.EvidenceRegistration,
						Subject: "py::PyModule", Predicate: "registers", Object: "_fastlex",
						AnchorKind: types.AnchorDefinition, AnchorSymbol: "_fastlex",
						Source: "bridge.rs", LineStart: 9, LineEnd: 9, Scope: types.ScopeLine,
						Snippet:         "fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {",
						Summary:         "Model-owned explanation, preserved rather than re-authored.",
						SurfaceTerms:    []string{"PyModule", "py::PyModule", "BindingContext", "_fastlex"},
						Producer:        types.EvidenceProducerExplorerEmitEvidence,
						GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
					},
					{
						ID: "binding-call", Kind: types.EvidenceRegistration,
						Subject: "m", Predicate: "registers", Object: "wrap_pyfunction!(tokenize_bytes, m)",
						AnchorKind: types.AnchorCall, AnchorSymbol: "add_function",
						Source: "bridge.rs", LineStart: 10, LineEnd: 10, Scope: types.ScopeLine,
						Snippet:         "m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
						Summary:         "The exact model-selected receiver call remains available.",
						Producer:        types.EvidenceProducerExplorerEmitEvidence,
						GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
					},
				},
			}
			ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
				DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "The model owns this explanation and any diagram choice."}},
			})
			beforeEvidence, _ := json.Marshal(ctx.EvidenceItems)
			beforeModel, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			const marker = "### Preferred anchors — safe to cite\n\n"
			_, safe, found := strings.Cut(prompt, marker)
			if !found {
				t.Fatalf("actual finalizer did not publish the safe-anchor section:\n%s", prompt)
			}
			if end := strings.Index(safe, "\n##"); end >= 0 {
				safe = safe[:end]
			}
			if !strings.Contains(safe, "- `_fastlex` — `bridge.rs:9` · symbol") {
				t.Errorf("declaration anchor must remain visible with neutral symbol role, not an invented function kind:\n%s", safe)
			}
			for _, forbidden := range []string{"`PyModule`", "`py::PyModule`", "`BindingContext`"} {
				if strings.Contains(safe, forbidden) {
					t.Errorf("registration subject/description became a declaration identity or alias (%s):\n%s", forbidden, safe)
				}
			}
			if !strings.Contains(safe, "- `add_function` — `bridge.rs:10` · call_site") {
				t.Errorf("independent exact call-site capability was lost:\n%s", safe)
			}
			afterEvidence, _ := json.Marshal(ctx.EvidenceItems)
			afterModel, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			if string(beforeEvidence) != string(afterEvidence) || string(beforeModel) != string(afterModel) {
				t.Fatal("display projection changed the historical evidence or model-owned answer")
			}
		})
	}
}
