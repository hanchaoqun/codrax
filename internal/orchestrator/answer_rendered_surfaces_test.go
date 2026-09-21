package orchestrator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinalAnswerRenderedSurfacesBindActualRenderAndCaveats(t *testing.T) {
	m := types.NewMutableState("audit")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: m, Language: "zh"}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "system", Kind: types.BlockSection, Text: "system-only", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
		{ID: "model", Kind: types.BlockSummary, Text: "current model answer"},
	}}
	m.RewriteAcceptedAnswerDocumentV2(doc)
	if o.finalAnswerRenderedSurfaces("current model answer") != nil {
		t.Fatal("unrendered document minted an audit")
	}
	answer := agent.RenderAnswerDocumentWithLastMileSupplements(&types.AgentContext{Mutable: m}, doc, nil, "zh")
	answer = o.appendRegisteredAnswerCaveatBullet(answer, "system disclosure")
	s := o.finalAnswerRenderedSurfaces(answer)
	if s == nil || s.Answer != answer || !strings.Contains(s.Primary, "current model answer") || strings.Contains(s.Primary, "system-only") || strings.Contains(s.Primary, "system disclosure") {
		t.Fatalf("wrong final audit: %+v", s)
	}
	if o.finalAnswerRenderedSurfaces(answer+"raw replacement") != nil {
		t.Fatal("fallback borrowed previous render")
	}
	// A same-text later document with different ownership is not what rendered.
	doc.Blocks[0].SystemGeneratedKind = types.AnswerSystemGeneratedBlockUnknown
	m.RewriteAcceptedAnswerDocumentV2(doc)
	if got := o.finalAnswerRenderedSurfaces(answer); got == nil || strings.Contains(got.Primary, "system-only") {
		t.Fatal("readback of later doc changed render ownership")
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "patch", Kind: types.BlockSummary, Text: "new patch summary"})
	patched := o.renderFinalAnswerWithLastMileSupplements(doc, nil)
	if o.finalAnswerRenderedSurfaces(answer) != nil {
		t.Fatal("old rendered bytes accepted after rerender")
	}
	if got := o.finalAnswerRenderedSurfaces(patched); got == nil || !strings.Contains(got.Primary, "new patch summary") {
		t.Fatal("patch scope not bound")
	}
}

func TestRecordTaskFinalizePersistsOnlyBoundAnswerSurfaces(t *testing.T) {
	for _, replace := range []bool{false, true} {
		m := types.NewMutableState("audit")
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: m, Language: "zh"}, outputDumpDir: t.TempDir(), emit: func(render.Event) {}}
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "answer", Kind: types.BlockSummary, Text: "visible model answer"}}}
		answer := o.renderFinalAnswerWithLastMileSupplements(doc, nil)
		// The real scheduler always runs this final display transform, even
		// without advisories. It removes the renderer's trailing newline.
		answer = appendRuntimeDispatchAdvisoriesToAnswer(answer, nil, "zh")
		if replace {
			answer = "raw fallback replacement"
		}
		o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: answer})
		raw, err := os.ReadFile(outputdump.AnswerSurfacesPathForMarkdown(m.FinalAnswerMarkdownPath()))
		if err != nil {
			t.Fatal(err)
		}
		var artifact outputdump.AnswerSurfaceArtifact
		if err := json.Unmarshal(raw, &artifact); err != nil {
			t.Fatal(err)
		}
		if replace {
			if artifact.Status != "unavailable" || artifact.Primary != "" {
				t.Fatalf("stale audit shipped: %+v", artifact)
			}
		} else if artifact.Status != "available" || !strings.Contains(artifact.Primary, "visible model answer") {
			t.Fatalf("bound audit lost: %+v", artifact)
		}
		if m.Result() != answer {
			t.Fatal("audit changed final answer")
		}
	}
}

func TestFinalAnswerRenderedSurfacesFinalTrimPreservesStrictBinding(t *testing.T) {
	m := types.NewMutableState("audit")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: m, Language: "zh"}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "answer", Kind: types.BlockSummary, Text: "visible model answer"}}}
	answer := o.renderFinalAnswerWithLastMileSupplements(doc, nil)
	answer = o.appendRegisteredAnswerCaveatBullet(answer, "system disclosure")
	answer = appendRuntimeDispatchAdvisoriesToAnswer(answer, nil, "zh")
	s := o.finalAnswerRenderedSurfaces(answer)
	if s == nil || s.Answer != answer || strings.Contains(s.Primary, "system disclosure") {
		t.Fatalf("final trim lost ownership or mixed system disclosure: %+v", s)
	}
	if o.finalAnswerRenderedSurfaces(strings.Replace(answer, "visible model", "different model", 1)) != nil {
		t.Fatal("interior answer replacement borrowed the rendered ownership")
	}
	if o.finalAnswerRenderedSurfaces(answer+"\nunknown appendix") != nil {
		t.Fatal("untracked appendix borrowed the rendered ownership")
	}
}
