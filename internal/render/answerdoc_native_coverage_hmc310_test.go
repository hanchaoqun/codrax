package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC310RenderNativeCoverageScopeAndSafeFallback(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "d", Kind: types.BlockDiagram,
		Diagram:               &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: "sequenceDiagram\nparticipant A as role<br/>thread-7\nparticipant B as thread-8\nA->>B: wakeup"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "role", Status: types.DiagramParticipantBoundaryUnproven}, {Participant: "source-owner", Status: types.DiagramParticipantBoundaryUnproven}},
	}}}
	doc.Blocks[0].BindNativeDiagramCoverage([]string{"role"}, 1)
	for _, tc := range []struct{ lang, mapped, ordinary string }{{"zh-CN", "业务身份对应尚未核实", "未证关系边界：** "}, {"en", "Business-role mapping remains unverified", "Unproven relation boundaries:** "}} {
		text := RenderAnswerDocument(doc, tc.lang)
		if !strings.Contains(text, tc.mapped) || !strings.Contains(text, tc.ordinary+"`source-owner`") || strings.Contains(text, tc.ordinary+"`role`") {
			t.Fatalf("disclosures merged: %s", text)
		}
		if !strings.Contains(text, "role<br/>thread-7") || strings.Count(text, "participant ") != 2 {
			t.Fatalf("renderer changed thread lanes: %s", text)
		}
	}
	raw, _ := json.Marshal(doc)
	var restored types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if text := RenderAnswerDocument(&restored, "zh-CN"); strings.Contains(text, "业务身份对应尚未核实") || !strings.Contains(text, "未证关系边界") {
		t.Fatal("JSON-only output claimed validation")
	}
	doc.Blocks[0].Diagram.Body += "\nB->>A: changed"
	if text := RenderAnswerDocument(doc, "zh-CN"); strings.Contains(text, "业务身份对应尚未核实") {
		t.Fatal("changed graph reused receipt")
	}
}
