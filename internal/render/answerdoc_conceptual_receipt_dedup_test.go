package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1611BoundReceipt(t *testing.T) *types.AnswerConceptualTerminalResolutionReceipt {
	t.Helper()
	row := types.ConceptualTerminalResolutionRow{
		EvidenceID: "ev-terminal", TerminalCallable: "AuditLog.record", ExactOperation: "System.out.println",
		Source: "src/AuditLog.java:6",
		AllowedConclusions: []types.ConceptualTerminalResolutionConclusion{
			types.ConceptualTerminalResolutionDestinationSupported,
			types.ConceptualTerminalResolutionCurrentTerminalDiffers,
			types.ConceptualTerminalResolutionDestinationUnproven,
		},
	}
	receipt := &types.AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: row.EvidenceID, Conclusion: types.ConceptualTerminalResolutionCurrentTerminalDiffers,
	}
	if !types.BindConceptualTerminalResolutionReceipt(receipt, &types.ConceptualTerminalResolutionContract{Rows: []types.ConceptualTerminalResolutionRow{row}}) {
		t.Fatal("fixture failed the real typed receipt binding")
	}
	return receipt
}

func b1611Document(t *testing.T) *types.AnswerDocumentV2 {
	t.Helper()
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Model explanation.", ConceptualTerminalResolution: b1611BoundReceipt(t)},
		{ID: "diagram", Kind: types.BlockDiagram, Title: "Model diagram", Text: "Model diagram explanation.",
			Diagram:                      &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  A[AuditLog.record] --> B[System.out.println]"},
			ConceptualTerminalResolution: b1611BoundReceipt(t)},
	}}
}

func TestB1611RenderConceptualReceiptDeduplicatesOnlyFullTypedBinding(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name string
			edit func(*types.AnswerConceptualTerminalResolutionReceipt)
			want int
		}{
			{"same independent bound values", func(*types.AnswerConceptualTerminalResolutionReceipt) {}, 1},
			{"selected evidence differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) { r.EvidenceID = "ev-other" }, 2},
			{"bound evidence differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) { r.BoundRow.EvidenceID = "ev-other" }, 2},
			{"terminal differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.BoundRow.TerminalCallable = "OtherAuditLog.record"
			}, 2},
			{"operation differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.BoundRow.ExactOperation = "System.err.println"
			}, 2},
			{"source differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.BoundRow.Source = "other/src/AuditLog.java:6"
			}, 2},
			{"source case differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) { r.BoundRow.Source = "src/auditLog.java:6" }, 2},
			{"source line differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) { r.BoundRow.Source = "src/AuditLog.java:7" }, 2},
			{"conclusion differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.Conclusion = types.ConceptualTerminalResolutionDestinationUnproven
			}, 2},
			{"allowed conclusions differ", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.BoundRow.AllowedConclusions = r.BoundRow.AllowedConclusions[:2]
			}, 2},
			{"allowed conclusion order differs", func(r *types.AnswerConceptualTerminalResolutionReceipt) {
				r.BoundRow.AllowedConclusions[0], r.BoundRow.AllowedConclusions[1] = r.BoundRow.AllowedConclusions[1], r.BoundRow.AllowedConclusions[0]
			}, 2},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				doc, before := b1611Document(t), b1611Document(t)
				tc.edit(doc.Blocks[1].ConceptualTerminalResolution)
				tc.edit(before.Blocks[1].ConceptualTerminalResolution)
				out := RenderAnswerDocument(doc, lang)
				if got := strings.Count(out, b1611DisclosureHeading(lang)); got != tc.want {
					t.Errorf("system disclosure count=%d want=%d\n%s", got, tc.want, out)
				}
				if !reflect.DeepEqual(doc, before) || RenderAnswerDocument(doc, lang) != out {
					t.Fatal("rendering changed the model document, typed receipt, or repeat output")
				}
				for _, text := range []string{before.Blocks[0].Text, before.Blocks[1].Text, before.Blocks[1].Title, before.Blocks[1].Diagram.Body} {
					if !strings.Contains(out, text) {
						t.Fatalf("model-authored surface disappeared: %q\n%s", text, out)
					}
				}
				if strings.Index(out, b1611DisclosureHeading(lang)) > strings.Index(out, "**Model diagram**") {
					t.Fatal("first disclosure moved away from its original block")
				}
			})
		}
	}
}

func TestB1611RenderConceptualReceiptSingleOutputUnchanged(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"zh", "Model explanation.\n\n**概念目标核对**：当前已证终点操作为 `AuditLog.record` 调用 `System.out.println`（`src/AuditLog.java:6`）；模型判断当前实现终止于该精确操作，并未达到用户所述的概念目标\n"},
		{"en", "Model explanation.\n\n**Conceptual-destination check**: the grounded terminal operation is `AuditLog.record` calling `System.out.println` (`src/AuditLog.java:6`); the model concludes that the current implementation terminates at this exact operation and does not reach the requested conceptual destination\n"},
	} {
		doc := b1611Document(t)
		doc.Blocks = doc.Blocks[:1]
		if got := RenderAnswerDocument(doc, tc.lang); got != tc.want {
			t.Fatalf("%s single-disclosure bytes changed:\ngot  %q\nwant %q", tc.lang, got, tc.want)
		}
	}
}

func TestB1611RenderConceptualReceiptUnrenderedDoesNotConsumeSlot(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AnswerBlock)
	}{
		{"nil receipt", func(b *types.AnswerBlock) { b.ConceptualTerminalResolution = nil }},
		{"unbound receipt", func(b *types.AnswerBlock) { b.ConceptualTerminalResolution.Bound = false }},
		{"system generated block", func(b *types.AnswerBlock) { b.SystemGeneratedKind = types.AnswerSystemGeneratedRuntimeTrace }},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				doc, before := b1611Document(t), b1611Document(t)
				tc.edit(&doc.Blocks[0])
				tc.edit(&before.Blocks[0])
				out := RenderAnswerDocument(doc, lang)
				if strings.Count(out, b1611DisclosureHeading(lang)) != 1 ||
					strings.Index(out, b1611DisclosureHeading(lang)) < strings.Index(out, "**Model diagram**") {
					t.Fatalf("unrendered receipt consumed later disclosure slot:\n%s", out)
				}
				if !reflect.DeepEqual(doc, before) {
					t.Fatal("rendering mutated input")
				}
			})
		}
	}
}

func TestB1611RenderConceptualReceiptDoesNotDeduplicateModelOrTrace(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			makeDoc := func() *types.AnswerDocumentV2 {
				doc := b1611Document(t)
				var disclosure strings.Builder
				renderV2ConceptualTerminalResolutionReceipt(&disclosure, doc.Blocks[0], normalizeAnswerDocLang(lang))
				doc.Blocks[1].Text = strings.TrimSpace(disclosure.String())
				for i := range doc.Blocks {
					doc.Blocks[i].RuntimeWorkRelation = &types.AnswerRuntimeWorkRelationReceipt{
						ObservationID: "trace-work", Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven,
						BoundRow: types.RuntimeWorkRelationRow{ObservationID: "trace-work", WorkLabel: "Compile work", Subject: "worker-7", MeasuredDurationMS: 1.25, Credential: "host_direct_wakeup_edge"},
					}
				}
				doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "trace", Kind: types.BlockSection, Title: "Trace 因果投影", Text: "Unchanged exact trace facts.", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace})
				return doc
			}
			doc, before := makeDoc(), makeDoc()
			out := RenderAnswerDocument(doc, lang)
			if strings.Count(out, b1611DisclosureHeading(lang)) != 2 || !strings.Contains(out, before.Blocks[1].Text) {
				t.Fatalf("only duplicate system disclosure may disappear; model-authored identical sentence must remain:\n%s", out)
			}
			runtimeHeading := "**运行时工作关系判断**"
			if lang == "en" {
				runtimeHeading = "**Runtime-work relation conclusion**"
			}
			if strings.Count(out, runtimeHeading) != 2 || !strings.Contains(out, "## Trace 因果投影") || !strings.Contains(out, before.Blocks[2].Text) {
				t.Fatalf("code-receipt de-duplication affected runtime/Trace disclosure:\n%s", out)
			}
			if !reflect.DeepEqual(doc, before) || RenderAnswerDocument(doc, lang) != out {
				t.Fatal("render changed input or failed deterministic repeat")
			}
		})
	}
}

func b1611DisclosureHeading(lang string) string {
	if lang == "zh" {
		return "**概念目标核对**"
	}
	return "**Conceptual-destination check**"
}
