package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeBindingPublicPatchSchemaUsesCurrentLocalNodeRoster(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []any
	}{
		{"current", "sequenceDiagram\nparticipant App as 界面主线程\nparticipant app as worker\nparticipant Stale as One\nparticipant Stale as Two\n", []any{"App", "app"}},
		{"none", "sequenceDiagram\nApp->>Remote: observed\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "sequence", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: tc.body}}}}
			candidate := types.AnswerDiagramRelationRepairCandidate{BlockID: "sequence", RelationKind: types.DiagramRelWakeup,
				FromIdentity: "from-event", ToIdentity: "to-event", Source: "trace_query", FromNativeParticipant: "rt_thread_from", ToNativeParticipant: "rt_thread_to",
				FromNodeIDs: []string{"rt_thread_from"}, ToNodeIDs: []string{"rt_thread_to"}}
			lease := types.NewAnswerDiagramRelationRepairLease(base, nil, []types.AnswerDiagramRelationRepairCandidate{candidate})
			if lease == nil || len(lease.AllowedAdditions) != 1 {
				t.Fatal("missing exact current addition lease")
			}
			mu := types.NewMutableState("diagram repair")
			mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			mu.SetAnswerDiagramRelationRepairLease(lease)
			var root map[string]any
			if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mu}), &root); err != nil {
				t.Fatal(err)
			}
			item := root["properties"].(map[string]any)["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)
			branch := item["oneOf"].([]any)[0].(map[string]any)
			props := branch["properties"].(map[string]any)
			for _, side := range []string{"from", "to"} {
				field, node := side+"_native_participant", side+"_node"
				if _, ok := props[field]; ok != (tc.want != nil) {
					t.Fatalf("native binding availability disagrees with exact current roster: %v", props)
				}
				if tc.want == nil {
					continue
				}
				var got []any
				conditions, _ := branch["allOf"].([]any)
				for _, value := range conditions {
					r := value.(map[string]any)
					if !reflect.DeepEqual(r["if"].(map[string]any)["required"], []any{field}) {
						continue
					}
					edge := r["then"].(map[string]any)["properties"].(map[string]any)["edge"].(map[string]any)
					got = edge["properties"].(map[string]any)[node].(map[string]any)["enum"].([]any)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("%s binding must select the existing diagram node, not the native identity: got=%v want=%v", field, got, tc.want)
				}
			}
			if tc.want != nil {
				for _, from := range []string{"App", "app", "Stale", "rt_thread_from"} {
					edit := emitAnswerDiagramEdgeEdit{Action: "add", additionCandidate: &candidate, FromNativeParticipant: candidate.FromNativeParticipant,
						Edge: &types.DiagramEdgeAnchor{FromNode: from, ToNode: "new_unbound"}}
					if err := validateAtomicNativeParticipantBinding(&base.Blocks[0], edit, lease); (err == nil) != (from == "App" || from == "app") {
						t.Fatalf("execution and schema disagree for local %q: %v", from, err)
					}
				}
			}
		})
	}
}

func TestNativeBindingHMC231AliasDiagnosticIsExactAndScoped(t *testing.T) {
	for _, tc := range []struct {
		name, side                string
		kind                      types.DiagramKind
		declaration, nativeChoice bool
		duplicate                 bool
	}{
		{name: "source_alias", side: "from", kind: types.DiagramSequence, declaration: true, nativeChoice: true},
		{name: "target_alias", side: "to", kind: types.DiagramSequence, declaration: true, nativeChoice: true},
		{name: "no_native_choice", side: "from", kind: types.DiagramSequence, declaration: true},
		{name: "undeclared_alias", side: "from", kind: types.DiagramSequence, nativeChoice: true},
		{name: "ambiguous_declaration", side: "from", kind: types.DiagramSequence, declaration: true, nativeChoice: true, duplicate: true},
		{name: "flow_not_sequence", side: "from", kind: types.DiagramFlow, declaration: true, nativeChoice: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "sequenceDiagram\n"
			if tc.declaration {
				body += "participant Existing as readable business role\nNote over Existing: unchanged note\n"
			}
			if tc.duplicate {
				body += "participant Existing as another declaration\n"
			}
			if tc.kind == types.DiagramFlow {
				body = "flowchart LR\nExisting[readable business role]\n"
			}
			block := types.AnswerBlock{Diagram: &types.AnswerDiagramBlock{Kind: tc.kind, Body: body}}
			before := cloneAtomicDiagramPatchBlock(block)
			candidate := types.AnswerDiagramRelationRepairCandidate{AdditionRef: "ra-current", RelationKind: types.DiagramRelWakeup,
				FromIdentity: "event-from", ToIdentity: "event-to", FromNodeIDs: []string{"rt_thread_from"}, ToNodeIDs: []string{"rt_thread_to"}}
			if tc.nativeChoice {
				candidate.FromNativeParticipant, candidate.ToNativeParticipant = "rt_thread_from", "rt_thread_to"
			}
			edge := types.DiagramEdgeAnchor{FromIdentity: candidate.FromIdentity, ToIdentity: candidate.ToIdentity, FromNode: "rt_thread_from", ToNode: "rt_thread_to", RelationKind: candidate.RelationKind}
			if tc.side == "from" {
				edge.FromNode = "Existing"
			} else {
				edge.ToNode = "Existing"
			}
			err := validateAtomicDiagramAdditionEndpointBindings(&block, &edge, &candidate, nil)
			if err == nil || !strings.Contains(err.Error(), "is not a typed carrier") || !reflect.DeepEqual(before, block) {
				t.Fatalf("diagnostic must not authorize or mutate: %v %+v", err, block)
			}
			wantHint := tc.kind == types.DiagramSequence && tc.declaration && tc.nativeChoice && !tc.duplicate
			if wantHint {
				for _, want := range []string{tc.side + "_native_participant=\"rt_thread_" + tc.side + "\"", "reuse", "notes", "labels grant no identity"} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("missing executable scoped repair %q: %v", want, err)
					}
				}
			} else if strings.Contains(err.Error(), "native_participant=") {
				t.Fatalf("unavailable binding advertised: %v", err)
			}
		})
	}
}
