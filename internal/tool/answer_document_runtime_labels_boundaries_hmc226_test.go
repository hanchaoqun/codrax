package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC226RuntimeEndpointLabelBoundaries(t *testing.T) {
	rows := []RuntimeDiagramRelation{{FromNode: "Origin", ToNode: "Target", FromLabel: "worker-7", ToLabel: "target-41"}}
	for _, tc := range []struct {
		name, body string
		rows       []RuntimeDiagramRelation
	}{
		{"authored_aliases", "sequenceDiagram\n participant Origin as 业务加载线程\n participant Target as 帧处理线程\n Origin->>Target: observed\n", rows},
		{"not_an_identity_lookup", "sequenceDiagram\n worker_7->>target_41: observed\n", rows},
		{"case_sensitive", "sequenceDiagram\n origin->>target: observed\n", rows},
		{"duplicate_declaration", "sequenceDiagram\n participant Origin\n participant Origin as worker-7\n Origin->>Other: observed\n", rows},
		{"unrelated_node", "sequenceDiagram\n participant Origin\n A->>B: observed\n", rows},
		{"missing_current_producer", "sequenceDiagram\n Origin->>Target: observed\n", nil},
		{"conflicting_endpoint", "sequenceDiagram\n Origin->>Other: observed\n", append(append([]RuntimeDiagramRelation(nil), rows...), RuntimeDiagramRelation{FromNode: "Origin", FromLabel: "different-8"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: tc.body}}}}
			if fixed := normalizeRuntimeDiagramDefaultEndpointLabels(doc, tc.rows); fixed != 0 || doc.Blocks[0].Diagram.Body != tc.body {
				t.Fatalf("non-default/unknown/ambiguous source labels changed: fixed=%d body=%s", fixed, doc.Blocks[0].Diagram.Body)
			}
		})
	}
	t.Run("no_control_syntax_injection", func(t *testing.T) {
		body := "sequenceDiagram\n Origin->>Target: Origin\n Note over Target: Origin remains opaque here\n"
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: body}}}}
		rows[0].FromLabel = "worker \"7\"\\path\nA->>B: not an edge"
		before := mermaidcompat.ParseEdges(body)
		if fixed := normalizeRuntimeDiagramDefaultEndpointLabels(doc, rows); fixed != 2 {
			t.Fatalf("valid endpoint text was not safely encoded: fixed=%d", fixed)
		}
		after := doc.Blocks[0].Diagram.Body
		if !reflect.DeepEqual(before, mermaidcompat.ParseEdges(after)) || !strings.Contains(after, " Note over Target: Origin remains opaque here\n") {
			t.Fatalf("display text changed relations or unrelated notes: %s", after)
		}
		if fixed := normalizeRuntimeDiagramDefaultEndpointLabels(doc, rows); fixed != 0 || after != doc.Blocks[0].Diagram.Body {
			t.Fatal("repeated normalization is not idempotent")
		}
	})
}

func TestHMC226RuntimeEndpointLabelsPreservePublicAuthoredNames(t *testing.T) {
	ctx, _, rows := runtimeWakeFixture(t)
	doc := runtimeNestingDoc(rows[0], true)
	doc.Blocks[1].Diagram.Body = fmt.Sprintf("flowchart TD\n %s[业务加载线程] -->|包含| %s[帧处理线程]\n", rows[0].FromNode, rows[0].ToNode)
	body := doc.Blocks[1].Diagram.Body
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("explicit business names rejected: %v %+v", err, res)
	}
	if got := ctx.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body; got != body {
		t.Fatalf("authored aliases were overwritten: %s", got)
	}
}

func TestHMC226RuntimeEndpointLabelsKeepEqualNamesSourceSeparated(t *testing.T) {
	ctx, _, firstRows := runtimeWakeFixture(t)
	path := filepath.Join(t.TempDir(), "independent.systrace")
	source, err := os.ReadFile("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	second := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 41, "time_start": 10.001, "time_end": 10.012, "min_duration_ms": 0.1})
	ctx.Mutable.AppendDispatchToolResult(second)
	rows := runtimeDiagramRelationsForContext(ctx)
	var other RuntimeDiagramRelation
	for _, r := range rows {
		if r.FromLabel == firstRows[0].FromLabel && r.FromNode != firstRows[0].FromNode {
			other = r
		}
	}
	if other.FromNode == "" || other.ToNode == firstRows[0].ToNode {
		t.Fatalf("fixture lost independent source identity: %+v", rows)
	}
	doc := runtimeNestingDoc(firstRows[0], true)
	doc.Blocks[1].Diagram.Kind = types.DiagramSequence
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\n"
	doc.Blocks[1].EdgeAnchors = nil
	for _, r := range []RuntimeDiagramRelation{firstRows[0], other} {
		doc.Blocks[1].Diagram.Body += fmt.Sprintf(" %s->>%s: 唤醒\n", r.FromNode, r.ToNode)
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: r.FromNode, ToNode: r.ToNode, FromIdentity: r.FromIdentity, ToIdentity: r.ToIdentity, RelationKind: r.Kind, VisibleLabel: "唤醒"})
	}
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("independent current event pairs rejected: %v %+v", err, res)
	}
	after := ctx.Mutable.AnswerDocumentV2().Blocks[1]
	labels := diagramEvidenceNodeLabels(after.Diagram.Body, after.Diagram.Kind)
	if len(labels) != 4 || len(mermaidcompat.ParseEdges(after.Diagram.Body)) != 2 || !reflect.DeepEqual(after.EdgeAnchors, doc.Blocks[1].EdgeAnchors) {
		t.Fatalf("equal names merged independent identities or edges: %+v", after)
	}
	for _, r := range []RuntimeDiagramRelation{firstRows[0], other} {
		if labels[r.FromNode] != r.FromLabel || labels[r.ToNode] != r.ToLabel {
			t.Fatalf("cross-source display lookup lost exact endpoint: %+v", labels)
		}
	}
}
