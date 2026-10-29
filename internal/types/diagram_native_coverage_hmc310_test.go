package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func nativeCoverageBlockHMC310() AnswerBlock {
	return AnswerBlock{ID: "d", Kind: BlockDiagram,
		Diagram:               &AnswerDiagramBlock{Kind: DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\nparticipant a as role<br/>thread-7\nparticipant b as worker-8\na->>b: wakeup"},
		EdgeAnchors:           []DiagramEdgeAnchor{{FromNode: "a", ToNode: "b", FromIdentity: "instance-7", ToIdentity: "instance-8", RelationKind: DiagramRelWakeup}},
		ParticipantBoundaries: []DiagramParticipantBoundary{{Participant: "role", Status: DiagramParticipantBoundaryUnproven}},
	}
}

func TestHMC310NativeCoverageReceiptMutationAndSerialization(t *testing.T) {
	for name, mutate := range map[string]func(*AnswerBlock){
		"body":            func(b *AnswerBlock) { b.Diagram.Body += "\nNote over a: changed" },
		"kind":            func(b *AnswerBlock) { b.Diagram.Kind = DiagramFlow },
		"id":              func(b *AnswerBlock) { b.ID = "other" },
		"language":        func(b *AnswerBlock) { b.Diagram.Language = "text" },
		"endpoint":        func(b *AnswerBlock) { b.EdgeAnchors[0].ToIdentity = "other-instance" },
		"direction":       func(b *AnswerBlock) { b.EdgeAnchors[0].FromNode, b.EdgeAnchors[0].ToNode = "b", "a" },
		"boundary":        func(b *AnswerBlock) { b.ParticipantBoundaries[0].Participant = "other-role" },
		"boundary_status": func(b *AnswerBlock) { b.ParticipantBoundaries[0].Status = DiagramParticipantBoundaryUnknown },
		"scope":           func(b *AnswerBlock) { b.RequestedRelationScope = DiagramRelationScopePartialUnproven },
		"selection": func(b *AnswerBlock) {
			b.Diagram.RuntimeParticipantsV1 = []RuntimeDiagramParticipantSelection{{NodeID: "a", NativeParticipant: "other"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := nativeCoverageBlockHMC310()
			b.BindNativeDiagramCoverage([]string{"role"}, 1)
			mutate(&b)
			if p, n := b.NativeDiagramCoverage(); len(p) != 0 || n != 0 {
				t.Fatalf("changed generation retained receipt: %v %d", p, n)
			}
		})
	}
	b := nativeCoverageBlockHMC310()
	input := []string{"role"}
	b.BindNativeDiagramCoverage(input, 1)
	input[0] = "caller mutation"
	roles, count := b.NativeDiagramCoverage()
	if len(roles) != 1 || roles[0] != "role" || count != 1 {
		t.Fatal("input alias changed private receipt")
	}
	roles[0] = "returned mutation"
	if p, _ := b.NativeDiagramCoverage(); p[0] != "role" {
		t.Fatal("getter exposed private slice")
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "nativeDiagramCoverage") || strings.Contains(string(raw), "unmapped") {
		t.Fatal("private receipt leaked")
	}
	var restored AnswerBlock
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if p, n := restored.NativeDiagramCoverage(); len(p) != 0 || n != 0 {
		t.Fatal("JSON restored display receipt")
	}
	if err := json.Unmarshal([]byte(`{"nativeDiagramCoverage":{"unmapped":["role"],"relations":99}}`), &restored); err != nil {
		t.Fatal(err)
	}
	if p, n := restored.NativeDiagramCoverage(); len(p) != 0 || n != 0 {
		t.Fatal("model JSON forged a receipt")
	}
	cloned := cloneAnswerDocumentV2(&AnswerDocumentV2{Blocks: []AnswerBlock{b}})
	if p, n := cloned.Blocks[0].NativeDiagramCoverage(); len(p) != 1 || n != 1 {
		t.Fatal("in-memory clone lost validated receipt")
	}
	cloned.Blocks[0].nativeDiagramCoverage.unmapped[0] = "clone mutation"
	if p, _ := b.NativeDiagramCoverage(); p[0] != "role" {
		t.Fatal("mutable private receipt shared by clone")
	}
}

func TestHMC310NativeCoverageReceiptRejectsInvalidClaims(t *testing.T) {
	for _, tc := range []struct {
		roles []string
		count int
	}{
		{nil, 1}, {[]string{"role"}, 0}, {[]string{"role"}, -1}, {[]string{"other"}, 1}, {[]string{"role", "role"}, 1},
	} {
		b := nativeCoverageBlockHMC310()
		b.BindNativeDiagramCoverage(tc.roles, tc.count)
		if p, n := b.NativeDiagramCoverage(); len(p) != 0 || n != 0 {
			t.Fatalf("invalid display claim accepted: %+v", tc)
		}
	}
}
