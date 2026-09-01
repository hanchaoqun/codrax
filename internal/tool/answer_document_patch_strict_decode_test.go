package tool

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAnswerDocumentPatchMisplacedHintUsesLiveAtomicOperationPath(t *testing.T) {
	raw := json.RawMessage(`{
		"diagram_edge_edits":[{
			"action":"add",
			"addition_ref":"ra1",
			"from_node":"A",
			"to_node":"B"
		}]
	}`)
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"diagram_edge_edits":{
				"type":"array",
				"items":{"oneOf":[{
					"type":"object",
					"properties":{
						"action":{"const":"add"},
						"addition_ref":{"const":"ra1"},
						"edge":{"type":"object","properties":{
							"from_node":{"type":"string"},
							"to_node":{"type":"string"}
						}}
					}
				}]}
			}
		}
	}`)
	err := errors.New(`json: unknown field "from_node"`)
	hints := answerDocumentPatchMisplacedHintsForSchema(err, raw, schema)
	got := RemapStrictDecodeErrorWithRaw(err, hints, raw).Error()
	for _, want := range []string{
		`diagram_edge_edits[i].edge.from_node`,
		`NOT inside diagram_edge_edits[i]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("live patch relocation missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "blocks[i].edge_anchors") || strings.Contains(got, "claim_uses") {
		t.Fatalf("patch retry was redirected to the full-emit grammar: %s", got)
	}
}

func TestAnswerDocumentPatchMisplacedHintFailsClosedAcrossOperationOwners(t *testing.T) {
	raw := json.RawMessage(`{
		"diagram_edge_edits":[{"from_node":"A"}],
		"replace_blocks":[{"claim_uses":[{"from_node":"B"}]}]
	}`)
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"diagram_edge_edits":{"items":{"properties":{"edge":{"properties":{"from_node":{"type":"string"}}}}}},
			"replace_blocks":{"items":{"properties":{"edge_anchors":{"items":{"properties":{"from_node":{"type":"string"}}}}}}}
		}
	}`)
	err := errors.New(`json: unknown field "from_node"`)
	hints := answerDocumentPatchMisplacedHintsForSchema(err, raw, schema)
	got := RemapStrictDecodeErrorWithRaw(err, hints, raw).Error()
	if !strings.Contains(got, "blocks[i].edge_anchors[j].from_node") {
		t.Fatalf("ambiguous owners must retain the prior conservative hint: %s", got)
	}
}

func TestAnswerDocumentPatchMisplacedHintRequiresPublishedSameOperationPath(t *testing.T) {
	raw := json.RawMessage(`{"diagram_edge_edits":[{"from_node":"A"}]}`)
	schema := json.RawMessage(`{"type":"object","properties":{"unchanged_block_ids":{"type":"array"}}}`)
	err := errors.New(`json: unknown field "from_node"`)
	hints := answerDocumentPatchMisplacedHintsForSchema(err, raw, schema)
	got := RemapStrictDecodeErrorWithRaw(err, hints, raw).Error()
	if !strings.Contains(got, "blocks[i].edge_anchors[j].from_node") {
		t.Fatalf("inactive patch operation must not invent a current path: %s", got)
	}
}
