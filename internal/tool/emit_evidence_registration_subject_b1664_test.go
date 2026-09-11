package tool_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const b1664RegistrationSource = `mod py {
    use pyo3::prelude::*;

    #[pyfunction]
    fn tokenize_bytes(data: Vec<u8>) -> Vec<u8> {
        data
    }
    #[pymodule]
    fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {
        m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;
        Ok(())
    }
}
`

// Parse the real Rust declaration and read it through the public tool. Neither
// the function identity nor its already-observed source is fabricated in a
// grounding map. This is a parser/evidence test, not native Rust execution.
func b1664RegistrationBus(t *testing.T) *types.BusContext {
	t.Helper()
	repo := t.TempDir()
	const source = "bridge.rs"
	path := filepath.Join(repo, source)
	if err := os.WriteFile(path, []byte(b1664RegistrationSource), 0o600); err != nil {
		t.Fatal(err)
	}
	files := index.ParseFiles([]index.FileEntry{{RelPath: source, AbsPath: path, Language: rmtypes.LangRust, Size: int64(len(b1664RegistrationSource))}}, repo)
	if len(files) != 1 {
		t.Fatalf("actual Rust parser returned %d files", len(files))
	}
	declared := false
	for _, symbol := range files[0].Symbols {
		if symbol.Name == "PyModule" || symbol.Name == "Bound" {
			t.Fatalf("parameter types must not be extracted as declarations: %+v", symbol)
		}
		if symbol.Name == "_fastlex" && symbol.Parent == "py" && symbol.Kind == "function" && symbol.Line == 9 && symbol.EndLine == 12 && symbol.Arity == 1 {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("missing actual py::_fastlex declaration: %+v", files[0].Symbols)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("B1664 registration source roles"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, Predicates: types.SemanticPredicates{IsCrossComponent: true}, AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}}
	bus.Mutable.SetSearchGraph(index.BuildGraph(repo, files))
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"bridge.rs","offset":0,"limit":30}`))
	if err != nil || !read.Success || !strings.Contains(read.Summary, "fn _fastlex(m: &Bound<'_, PyModule>)") {
		t.Fatalf("actual source read failed: %v %+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	bus.Mutable.AppendDispatchToolResult(read)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{read}})
	return bus
}

func b1664RegistrationItem(subject, object, anchor string, line int, kind string) map[string]any {
	return map[string]any{
		"scope": "line", "evidence_kind": "registration", "subject": subject, "predicate": "registers", "object": object,
		"source": "bridge.rs", "line_start": line, "anchor_kind": kind, "anchor_symbol": anchor,
		"summary": "Model-authored registration explanation; not an endpoint proof.",
	}
}

func b1664EmitRegistration(t *testing.T, bus *types.BusContext, items ...map[string]any) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("public emit failed unexpectedly: %v %+v", err, result)
	}
	bus.Mutable.AppendDispatchToolResult(result)
	bus.EvidenceItems = append([]types.EvidenceItem(nil), bus.Mutable.EmittedEvidence()...)
	return result
}

func TestB1664PublicRegistrationParameterTypeIsNotBindingSubject(t *testing.T) {
	for _, subject := range []string{"py::PyModule", "PyModule"} {
		t.Run(subject, func(t *testing.T) {
			bus := b1664RegistrationBus(t)
			before, _ := json.Marshal(bus.Mutable.SearchGraph().(*rmtypes.Graph).FileIndex["bridge.rs"])
			result := b1664EmitRegistration(t, bus, b1664RegistrationItem(subject, "_fastlex", "_fastlex", 9, "definition"))
			preservedLead := false
			for _, item := range bus.EvidenceItems {
				if item.Producer == tool.EmitEvidenceProducer && item.Subject == subject && item.Object == "_fastlex" && item.IsCitable() && types.ClaimFormOf(item) == types.ClaimRegistrationEdge {
					t.Errorf("parameter type became verified registration subject at a different symbol's declaration: %+v; result=%s", item, result.Summary)
				}
				if item.Subject == subject && item.Object == "_fastlex" && item.Kind == types.EvidenceUnresolved &&
					item.GroundingStatus == types.GroundingUngrounded && item.Confidence == 0 &&
					item.Summary == "Model-authored registration explanation; not an endpoint proof." {
					preservedLead = true
				}
			}
			if !preservedLead {
				t.Fatalf("withdrawal must preserve the model's original endpoints and explanation as an unresolved lead: %+v", bus.EvidenceItems)
			}
			for _, purpose := range []types.TypedRelationPurpose{types.TypedRelationPurposePromptHint, types.TypedRelationPurposeCoverageGate} {
				rows := (types.EvidenceRelationCandidateSource{Items: bus.EvidenceItems}).TypedRelationCandidates(types.TypedRelationQuery{
					Kinds: []types.TypedRelationKind{types.TypedRelationRegisters}, Sources: []string{subject, "_fastlex"}, Purpose: purpose,
				})
				if len(rows) != 0 {
					t.Errorf("withdrawn registration returned through %s recipes: %+v", purpose, rows)
				}
			}
			plan := types.BuildAnswerSupportPlanForBusContext(bus)
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			for _, anchor := range types.BuildVisibleAnchorWhitelist(plan, view).Groundable {
				if (anchor.Symbol == "PyModule" || anchor.QualifiedSymbol == subject) && anchor.Kind == types.VisibleAnchorKindFunction && anchor.IsHighConfidenceTier() {
					t.Errorf("parameter type became a safe function-definition anchor: %+v", anchor)
				}
			}
			after, _ := json.Marshal(bus.Mutable.SearchGraph().(*rmtypes.Graph).FileIndex["bridge.rs"])
			if string(before) != string(after) {
				t.Fatal("evidence emission mutated the independently parsed source identities")
			}
		})
	}
}

func TestB1664PublicRegistrationAttachedContainerAndBindingStayDistinct(t *testing.T) {
	for _, mode := range []string{"attached_container", "actual_binding", "both"} {
		t.Run(mode, func(t *testing.T) {
			bus := b1664RegistrationBus(t)
			container := b1664RegistrationItem("pyo3 #[pymodule]", "_fastlex", "_fastlex", 8, "definition")
			binding := b1664RegistrationItem("m", "wrap_pyfunction!(tokenize_bytes, m)", "add_function", 10, "call")
			var items []map[string]any
			if mode != "actual_binding" {
				items = append(items, container)
			}
			if mode != "attached_container" {
				items = append(items, binding)
			}
			before, _ := json.Marshal(items)
			result := b1664EmitRegistration(t, bus, items...)
			if !result.Success {
				t.Fatalf("existing exact registration carrier was rejected: %+v", result)
			}
			for _, want := range items {
				found := false
				for _, got := range bus.EvidenceItems {
					if got.Producer == tool.EmitEvidenceProducer && got.Subject == want["subject"] && got.Object == want["object"] {
						found = got.IsCitable() && types.ClaimFormOf(got) == types.ClaimRegistrationEdge && got.Summary == want["summary"]
					}
				}
				if !found {
					t.Errorf("exact model-owned endpoint/summary carrier was lost: wanted=%+v got=%+v", want, bus.EvidenceItems)
				}
			}
			bindingDebt := result.Repair != nil && result.Repair.Metadata["repair_scope"] == "registration_binding_expression"
			if bindingDebt != (mode == "attached_container") {
				t.Fatalf("container disclosure must not substitute for the distinct actual binding: mode=%s repair=%+v", mode, result.Repair)
			}
			if bindingDebt {
				for _, exact := range []string{`line_start=10`, `anchor_symbol="add_function"`, `subject="m"`, `object="wrap_pyfunction!(tokenize_bytes, m)"`} {
					if !strings.Contains(result.Repair.Hint, exact) {
						t.Errorf("existing source-owned binding repair lost %s: %+v", exact, result.Repair)
					}
				}
			}
			after, _ := json.Marshal(items)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("public evidence call changed the model input")
			}
		})
	}
}
