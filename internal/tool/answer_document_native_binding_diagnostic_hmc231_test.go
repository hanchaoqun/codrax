package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
