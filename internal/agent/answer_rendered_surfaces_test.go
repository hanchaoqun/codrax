package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerRenderedSurfacesActualParseAcceptedAndRecovered(t *testing.T) {
	for _, lane := range []string{"accepted", "recovered"} {
		t.Run(lane, func(t *testing.T) {
			m := types.NewMutableState("render audit")
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "system-front", Kind: types.BlockSection, Text: "system-only-business-name", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
				{ID: "summary", Kind: types.BlockSummary, Text: "model-owned-business-answer"},
				{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Body: "flowchart TD\nA -->"}},
			}}
			if lane == "accepted" {
				m.RewriteAcceptedAnswerDocumentV2(doc)
			} else {
				m.SetLastRejectedAnswerDocumentV2(doc)
			}
			e := &answerDocumentEvaluator{language: "zh"}
			out, err := e.ParseOutput(&types.AgentContext{Mutable: m}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			s := m.AnswerRenderedSurfaces()
			if s == nil || s.Answer != out.FinalAnswer || !strings.Contains(s.Primary, "model-owned-business-answer") || strings.Contains(s.Primary, "system-only-business-name") {
				t.Fatalf("%s parse lost final ownership: out=%+v scope=%+v", lane, out, s)
			}
			if lane == "recovered" && !out.AnswerDegraded {
				t.Fatal("recovery changed existing degradation disclosure")
			}
			m.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
			m.SetLastRejectedAnswerDocumentV2(nil)
			out, err = e.ParseOutput(&types.AgentContext{Mutable: m}, []llm.Message{{Role: "assistant", Content: "raw replacement, not a structured carrier"}}, nil, nil)
			if err != nil || out.FinalAnswer == "" {
				t.Fatalf("raw fallback lost: %v", err)
			}
			if m.AnswerRenderedSurfaces() != nil {
				t.Fatal("raw fallback borrowed previous structured ownership")
			}
		})
	}
}
