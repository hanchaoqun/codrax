package render

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerRenderedSurfacesFollowOwnershipNotTitlesOrPositions(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, heading := range []string{"主要时间占用", "Trace 因果投影", "renamed system chapter"} {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
				{ID: "front", Kind: types.BlockSection, Title: heading, Text: "system-only-marker", SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
				{ID: "model", Kind: types.BlockSection, Title: "Trace 因果投影", Text: "model-owned-before"},
				{ID: "middle", Kind: types.BlockSection, Text: "system-middle-marker", SystemGeneratedKind: types.AnswerSystemGeneratedEvidenceSupplement},
				{ID: "patched-summary", Kind: types.BlockSummary, Text: "model-owned-after"},
			}, Citations: []types.Citation{{File: "citation-only.go", Line: 1}}, Snippets: []types.CodeSnippet{{File: "snippet.go", StartLine: 1, Code: "snippet-only-marker"}}}
			before := types.CaptureSystemGeneratedBlockKinds(doc)
			s := RenderAnswerDocumentWithAttachmentSurfaces(doc, []types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentText, Body: "recovery-only-marker"}}, lang)
			if s.Answer != RenderAnswerDocumentWithAttachments(doc, []types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentText, Body: "recovery-only-marker"}}, lang) {
				t.Fatal("audit changed the answer")
			}
			for _, text := range []string{"model-owned-before", "model-owned-after", "Trace 因果投影"} {
				if !strings.Contains(s.Primary, text) || !strings.Contains(s.Principal, text) {
					t.Fatalf("lost model text %q: %+v", text, s)
				}
			}
			for _, text := range []string{"system-only-marker", "system-middle-marker", "recovery-only-marker"} {
				if !strings.Contains(s.Answer, text) || strings.Contains(s.Primary+s.Principal, text) {
					t.Fatalf("ownership leak %q: %+v", text, s)
				}
			}
			for _, text := range []string{"citation-only.go", "snippet-only-marker"} {
				if strings.Contains(s.Primary, text) || !strings.Contains(s.Principal, text) {
					t.Fatalf("appendix scope %q: %+v", text, s)
				}
			}
			if !reflect.DeepEqual(before, types.CaptureSystemGeneratedBlockKinds(doc)) {
				t.Fatal("mutated private ownership")
			}
		}
	}
}

func TestAnswerRenderedSurfacesDoNotResurrectDeduplicatedContent(t *testing.T) {
	items := []types.AnswerBlockItem{{Text: "only-in-system-table"}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "model-section", Kind: types.BlockSection, Text: "model text", Items: items},
		{ID: "system-table", Kind: types.BlockTable, Items: items, SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
	}}
	s := RenderAnswerDocumentSurfaces(doc, "zh")
	if !strings.Contains(s.Answer, items[0].Text) || strings.Contains(s.Primary, items[0].Text) {
		t.Fatalf("filtered re-render resurrected hidden content: %+v", s)
	}
}

func TestAnswerRenderedSurfacesJSONCannotMintOwnership(t *testing.T) {
	var doc types.AnswerDocumentV2
	if err := json.Unmarshal([]byte(`{"blocks":[{"id":"runtime_trace_causal_projection","kind":"section","title":"Trace 因果投影","text":"model statement","system_generated_kind":"runtime_trace"}]}`), &doc); err != nil {
		t.Fatal(err)
	}
	if s := RenderAnswerDocumentSurfaces(&doc, "zh"); !strings.Contains(s.Primary, "model statement") {
		t.Fatalf("model JSON hid its statement: %+v", s)
	}
}

func TestAnswerRenderedSurfacesMixedDocumentCaveatsCannotProveModelExplanation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks:                []types.AnswerBlock{{ID: "model", Kind: types.BlockCaveat, Text: "model-owned-caveat"}},
		Caveats:               []string{"system-added-measurement-47ms", "unattributed-document-caveat"},
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{{Role: types.EvidenceDiagramRoleConfig, Label: "unattributed-role-receipt"}},
	}
	for _, lang := range []string{"zh", "en"} {
		s := RenderAnswerDocumentSurfaces(doc, lang)
		for _, want := range []string{"system-added-measurement-47ms", "unattributed-document-caveat", "unattributed-role-receipt"} {
			if !strings.Contains(s.Answer, want) || strings.Contains(s.Primary+s.Principal, want) {
				t.Fatalf("mixed document-level metadata borrowed model authority: %q: %+v", want, s)
			}
		}
		if !strings.Contains(s.Primary, "model-owned-caveat") {
			t.Fatal("an actual model caveat block was excluded")
		}
	}
}
