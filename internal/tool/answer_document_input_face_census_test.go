package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_input_face_census_test.go — V1-5 (§40.16 ①/③): a retired
// lane must retire on all THREE faces together (schema / teaching / decoder).
// The precise rule this census enforces: every top-level INPUT face of the
// two answer-document tools — the quarantine allowlist, the strict-decode wire
// struct, and every top-level field a ToolRepair names — must be a field the
// tool can PUBLISH to the model under some production-shaped activation.
// A key the decoder binds but no schema ever publishes is a dead lane whose
// stray presence can only be handled by the generic unknown-field quarantine,
// never by a dedicated hard reject (the trace_finding failEmit that this
// census was red on at HEAD).
//
// The activation roster is explicit: a future contract-gated field must be
// registered here (its activation added to the roster), never hand-waved.

// answerDocumentUnpublishedCompatFaces are the top-level input faces that are
// deliberately accepted WITHOUT being published by any schema activation.
// Registration requires the soft-lane rationale (§40.16 ①: an unpublished
// face may only ever be quarantined or bound softly — never a dedicated hard
// reject); a face that becomes published, or stops being an input face, is a
// stale entry and red. Key: "<tool>:<field>".
var answerDocumentUnpublishedCompatFaces = map[string]string{
	"emit_answer_document:document_model": "V2 discriminator on the wire struct; the executor stamps \"v2\" itself and never reads the model's value (no reject arm)",
	"emit_answer_document:caveats":        "legacy V2 document-level caveats: decoded and bound onto the document softly (loose-JSON tolerance + strict decode); the published twin is the patch tool's replace_caveats",
	"emit_answer_document:snippets":       "legacy V2 document-level code snippets: decoded and bound softly; the published twin is the patch tool's replace_snippets",
}

// answerDocumentPublishableSchemas lists, per tool, the schemas whose
// property keys form its publishable set: the canonical schema plus every
// production-shaped activation that projects an additional top-level field.
func answerDocumentPublishableSchemas(t *testing.T) map[string][]json.RawMessage {
	t.Helper()
	rootCause := &types.AgentContext{Mutable: &types.MutableState{}}
	rootCause.Mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	full := &EmitAnswerDocument{}
	patch := &EmitAnswerDocumentPatch{}
	return map[string][]json.RawMessage{
		full.Name(): {
			full.Parameters(),
			// trace root-cause selector: published only when the frozen typed
			// contract exposes selectable on-chain candidates.
			full.ParametersFor(rootCause),
		},
		patch.Name(): {
			patch.Parameters(),
			patch.ParametersFor(rootCause),
		},
	}
}

func schemaTopLevelProperties(t *testing.T, schemas []json.RawMessage) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, raw := range schemas {
		var root map[string]any
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatalf("schema is not JSON: %v", err)
		}
		properties, _ := root["properties"].(map[string]any)
		for key := range properties {
			out[key] = true
		}
	}
	return out
}

func TestAnswerDocumentTopLevelInputFacesArePublishable(t *testing.T) {
	published := answerDocumentPublishableSchemas(t)
	faces := []struct {
		tool    string
		profile answerDocumentFieldQuarantineProfile
		wire    any
	}{
		{tool: (&EmitAnswerDocument{}).Name(), profile: answerDocumentFullEmitQuarantineProfile, wire: emitAnswerDocumentV2Params{}},
		{tool: (&EmitAnswerDocumentPatch{}).Name(), profile: answerDocumentPatchQuarantineProfile, wire: emitAnswerDocumentPatchParams{}},
	}
	for _, face := range faces {
		t.Run(face.tool, func(t *testing.T) {
			schemas, ok := published[face.tool]
			if !ok || len(schemas) == 0 {
				t.Fatalf("no publishable schema roster for %s", face.tool)
			}
			publishable := schemaTopLevelProperties(t, schemas)
			if len(publishable) == 0 {
				t.Fatal("publishable set is empty — the census scan is broken and the tripwire vacuous")
			}
			inputFaces := map[string]string{}
			for key := range face.profile.TopLevelAllowed {
				inputFaces[key] = "quarantine allowlist"
			}
			for _, key := range jsonFieldNames(reflect.TypeOf(face.wire)) {
				if _, seen := inputFaces[key]; seen {
					inputFaces[key] = "quarantine allowlist + strict-decode wire struct"
				} else {
					inputFaces[key] = "strict-decode wire struct"
				}
			}
			keys := make([]string, 0, len(inputFaces))
			for key := range inputFaces {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				exemptionKey := face.tool + ":" + key
				rationale, exempted := answerDocumentUnpublishedCompatFaces[exemptionKey]
				switch {
				case publishable[key] && exempted:
					t.Errorf("%s: stale compat exemption for %q — the field is published now; drop the entry", face.tool, key)
				case !publishable[key] && !exempted:
					t.Errorf("%s: top-level input face %q (%s) is never published by any schema activation — a dead lane; retire the decoder/allowlist face with it, register its activation in answerDocumentPublishableSchemas, or (soft lanes only) register it in answerDocumentUnpublishedCompatFaces with its rationale", face.tool, key, inputFaces[key])
				case !publishable[key] && strings.TrimSpace(rationale) == "":
					t.Errorf("%s: compat exemption for %q carries no rationale", face.tool, key)
				}
			}
			for exemptionKey := range answerDocumentUnpublishedCompatFaces {
				if !strings.HasPrefix(exemptionKey, face.tool+":") {
					continue
				}
				if _, isFace := inputFaces[strings.TrimPrefix(exemptionKey, face.tool+":")]; !isFace {
					t.Errorf("stale compat exemption %q — no such top-level input face any more; drop the entry", exemptionKey)
				}
			}
		})
	}
}

// answerDocumentRepairFieldSourceFiles are the executor/runtime files whose
// ToolRepair.Fields literals name top-level answer-document fields.
var answerDocumentRepairFieldSourceFiles = map[string]string{
	"emit_answer_document_v2.go":          (&EmitAnswerDocument{}).Name(),
	"emit_answer_document_patch.go":       (&EmitAnswerDocumentPatch{}).Name(),
	"answer_document_mutation_runtime.go": "", // shared by both tools
	"final_answer_artifacts_mutation.go":  "", // shared by both tools
}

// TestAnswerDocumentRepairFieldsArePublishable is the second census arm:
// every top-level field a ToolRepair literal names (no `.` / `[` path
// segment) must be publishable by the tool the file serves (shared files:
// by either tool). A repair that steers the model toward an unpublished
// field would teach a lane the schema never offered.
func TestAnswerDocumentRepairFieldsArePublishable(t *testing.T) {
	published := answerDocumentPublishableSchemas(t)
	byTool := map[string]map[string]bool{}
	union := map[string]bool{}
	for tool, schemas := range published {
		byTool[tool] = schemaTopLevelProperties(t, schemas)
		for key := range byTool[tool] {
			union[key] = true
		}
	}
	fset := token.NewFileSet()
	literals := 0
	for file, tool := range answerDocumentRepairFieldSourceFiles {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		allowed := union
		if tool != "" {
			allowed = byTool[tool]
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if !isToolRepairLiteral(lit) {
				return true
			}
			for _, element := range lit.Elts {
				kv, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Fields" {
					continue
				}
				fields, ok := kv.Value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, item := range fields.Elts {
					basic, ok := item.(*ast.BasicLit)
					if !ok || basic.Kind != token.STRING {
						continue
					}
					name := strings.Trim(basic.Value, "`\"")
					if strings.ContainsAny(name, ".[") {
						continue
					}
					literals++
					if !allowed[name] {
						t.Errorf("%s:%s: ToolRepair.Fields names top-level field %q which no schema activation publishes", file, fset.Position(basic.Pos()), name)
					}
				}
			}
			return true
		})
	}
	if literals == 0 {
		t.Fatal("no ToolRepair.Fields top-level literals found — the census scan is broken and the tripwire vacuous")
	}
}

func isToolRepairLiteral(lit *ast.CompositeLit) bool {
	switch typ := lit.Type.(type) {
	case *ast.SelectorExpr:
		pkg, ok := typ.X.(*ast.Ident)
		return ok && pkg.Name == "types" && typ.Sel.Name == "ToolRepair"
	case *ast.Ident:
		return typ.Name == "ToolRepair"
	}
	return false
}
