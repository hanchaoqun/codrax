package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC226DegradedNativeFactsActualRecovery(t *testing.T) {
	ctx, table := nativeDisplayBindingContext224(t, `{}`)
	const draft = "Original facts remain separate from this unaccepted interpretation."
	ctx.Mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "draft", Kind: types.BlockSummary, Text: draft}}})
	evaluator := &answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}
	out, err := evaluator.ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "Failed repair scratch must not ship."}}, nil, nil)
	if err != nil || !out.AnswerDegraded || !out.SkipAnswerChecks || out.DegradeReason != "answer_document_retry_state_recovered" {
		t.Fatalf("native facts changed recovery disposition: err=%v output=%+v", err, out)
	}
	if ctx.Mutable.AnswerDocumentV2() != nil || ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("native display promoted the rejected draft or completed investigation")
	}
	doc := ctx.Mutable.DegradedRecoveredAnswerDocumentV2()
	if doc == nil || doc.Blocks[0].Text != draft {
		t.Fatal("recovery lost or changed the model draft")
	}
	tables := 0
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		tables++
		if block.SystemGeneratedKind != types.AnswerSystemGeneratedNativeFacts || !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
			t.Fatal("recovery lost source-bound native values")
		}
	}
	if tables != 1 || !strings.Contains(out.FinalAnswer, "Native recorded facts") || !strings.Contains(out.FinalAnswer, "9007199254741001") || strings.Contains(out.FinalAnswer, "Failed repair scratch") {
		t.Fatalf("degraded output lost native table/disclosure or leaked repair scratch: %s", out.FinalAnswer)
	}
}

func TestHMC226DegradedNativeFactsDisclosureUsesReaderLanguage(t *testing.T) {
	for language, label := range map[string]string{"en": "Native recorded facts", "zh": "原始观测事实"} {
		footer := degradedDeterministicSectionsCaveat(language, []string{"native_facts"})
		if !strings.Contains(footer, label) || strings.Contains(footer, "native_facts") {
			t.Fatalf("%s native display disclosure lost its reader-facing label: %s", language, footer)
		}
	}
}
