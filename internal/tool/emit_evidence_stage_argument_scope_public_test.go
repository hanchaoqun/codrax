package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Read real fixture bytes through the public tool, and verify the stage
// provider against copies of the current checkout's own declarations. The
// tiny search index describes the same fixture, not a guessed stage handoff.
func stageArgumentScopePublicContext(t *testing.T, callee string, carrier bool) (*types.BusContext, map[string]any) {
	t.Helper()
	argument := "types.StageAnalyze"
	if carrier {
		argument = "o.busCtx, " + argument
	}
	line := fmt.Sprintf(" %s(%s)", callee, argument)
	source := "package pipeline\ntype Orchestrator struct { busCtx *types.BusContext }\nfunc (o *Orchestrator) Run() {\n" + line + "\n}\n"
	bus := b1685ReadSources(t, map[string]string{"flow.go": source})
	for _, file := range []string{types.ReadModePipelineStageBindingFile, types.ReadModePipelineEnumsFile} {
		data, err := os.ReadFile(filepath.Join("../..", file))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(bus.RepoRoot, file)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	bus.AnalysisIR = flowOperationCompletionContext(nil).AnalysisIR
	bus.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true}
	for _, identity := range []string{"Analyzer", "Explorer", "Extractor", "Finalizer"} {
		bus.AnalysisIR.RequestModel.DiagramHint.Participants = append(bus.AnalysisIR.RequestModel.DiagramHint.Participants,
			types.DiagramParticipantHint{Identity: identity, Role: types.DiagramParticipantIncidentRequired})
	}
	if carrier {
		bus.AnalysisIR.RequestModel.DiagramHint.Participants = append(bus.AnalysisIR.RequestModel.DiagramHint.Participants,
			types.DiagramParticipantHint{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired})
	}
	if authority, ok := stageauthority.LoadReadMode(bus.RepoRoot); !ok || len(authority.Precedence) != 3 {
		t.Fatal("premise: real checkout stage declaration provider is unavailable")
	}
	fi := &repotypes.FileInfo{RelPath: "flow.go", Language: repotypes.LangGo, Package: "pipeline",
		Symbols: []repotypes.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 2, EndLine: 2},
			{Name: "Run", Kind: "method", Receiver: "Orchestrator", Line: 3, EndLine: 5},
		},
		Relations: []repotypes.Relation{{Kind: "call", File: "flow.go", Line: 4,
			FromEP:     repotypes.RelationEndpoint{Name: "Run", Receiver: "Orchestrator", File: "flow.go", Line: 4},
			ToEP:       repotypes.RelationEndpoint{Name: callee, File: "flow.go", Line: 4},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call"}},
	}
	bus.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{"flow.go": fi}))
	return bus, map[string]any{"scope": "line", "evidence_kind": "relationship", "source": "flow.go",
		"line_start": 4, "anchor_kind": "call", "anchor_symbol": callee,
		"subject": "Orchestrator.Run", "predicate": "calls", "object": callee,
		"snippet": line, "summary": "The selected exact source call."}
}

func TestStageArgumentScopePublicAncillaryConsumersDoNotBlockCompletion(t *testing.T) {
	for _, callee := range []string{"softTransportRetryHintForStage", "string", "renamedDiagnosticConsumer"} {
		t.Run(callee, func(t *testing.T) {
			bus, item := stageArgumentScopePublicContext(t, callee, false)
			requestBefore := *bus.AnalysisIR
			result := b1685Emit(t, bus, item)
			rows := bus.Mutable.EmittedEvidence()
			if len(rows) != 1 || !rows[0].IsCitable() || rows[0].AnchorKind != types.AnchorCall {
				t.Fatalf("premise: original call must be accepted without creating an argument edge: %+v", rows)
			}
			if result.Repair == nil || !strings.Contains(result.Repair.Hint, `subject="types.StageAnalyze"`) {
				t.Fatalf("the exact argument must remain available as a factual candidate: %+v", result.Repair)
			}
			completion, err := (&EmitInvestigationComplete{}).Execute(bus, flowOperationCompletionParams(t))
			if err != nil {
				t.Fatal(err)
			}
			if pending := pendingBlockingEmitEvidenceItemValidationRepair(bus); pending != nil ||
				(completion.Repair != nil && completion.Repair.Code == types.ToolRepairCodeEvidenceItemValidation) {
				t.Fatalf("stage identity/order authority cannot require every ancillary consumer as a handoff: pending=%+v completion=%+v", pending, completion)
			}
			if result.Repair.Metadata["repair_status"] != types.ToolRepairStatusAdvisory ||
				result.Repair.Metadata["completion_blocking"] == "true" ||
				result.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey] != "" {
				t.Fatalf("optional stage argument leaked into the durable required-relation contract: %+v", result.Repair)
			}
			if !reflect.DeepEqual(requestBefore, *bus.AnalysisIR) || !reflect.DeepEqual(rows, bus.Mutable.EmittedEvidence()) {
				t.Fatal("completion rewrote the request or minted argument evidence")
			}
		})
	}
}

func TestStageArgumentScopePublicCarrierStillRequiresItsExactHandoff(t *testing.T) {
	bus, item := stageArgumentScopePublicContext(t, "deliver", true)
	result := b1685Emit(t, bus, item)
	obligations, ok := decodeEmitEvidenceRelationRepairObligations(result.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
	if !ok || len(obligations) != 1 || obligations[0].Subject != "o.busCtx" || obligations[0].Object != "deliver" {
		t.Fatalf("only the declared incident-required carrier can be required: %+v", result.Repair)
	}
	if pendingBlockingEmitEvidenceItemValidationRepair(bus) == nil {
		t.Fatal("dropping ancillary stage debt must not weaken the carrier obligation")
	}
	completion, err := (&EmitInvestigationComplete{}).Execute(bus, flowOperationCompletionParams(t))
	if err != nil || completion.Repair == nil || completion.Repair.Code != types.ToolRepairCodeEvidenceItemValidation {
		t.Fatalf("public completion must retain the exact carrier repair: %v %+v", err, completion)
	}
	b1685Emit(t, bus, b1685ArgumentItem("flow.go", 4, "o.busCtx", "deliver"))
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(bus); pending != nil {
		t.Fatalf("authored carrier handoff should discharge the only required debt: %+v", pending)
	}
}

func TestStageArgumentScopePublicSelectedArgumentStillNeedsSourceProof(t *testing.T) {
	for _, argument := range []string{"types.StageAnalyze", "types.StageFinalize"} {
		t.Run(argument, func(t *testing.T) {
			bus, _ := stageArgumentScopePublicContext(t, "observe", false)
			b1685Emit(t, bus, b1685ArgumentItem("flow.go", 4, argument, "observe"))
			proved := false
			for _, row := range bus.Mutable.EmittedEvidence() {
				proved = proved || (row.IsCitable() && row.Kind == types.EvidenceRelationship &&
					row.AnchorKind == types.AnchorArgument && row.Subject == argument && row.Object == "observe")
			}
			if proved != (argument == "types.StageAnalyze") {
				t.Fatalf("optional candidate status must not waive model-selected exact argument proof: %+v", bus.Mutable.EmittedEvidence())
			}
		})
	}
}

func TestStageArgumentScopeRepairMergesPreserveIndependentDuties(t *testing.T) {
	for _, firstRequired := range []bool{false, true} {
		for _, secondRequired := range []bool{false, true} {
			t.Run(fmt.Sprintf("first=%t/second=%t", firstRequired, secondRequired), func(t *testing.T) {
				makeRepair := func(required bool, argument string) *types.ToolRepair {
					return buildEmitEvidenceArgumentFlowRepair([]emitEvidenceArgumentFlowRepair{{
						argument: argument, receiver: "consumer", source: "flow.go", line: 4, advisoryOnly: !required,
					}})
				}
				merged := mergeEmitEvidenceRelationEndpointRepairs(makeRepair(firstRequired, "first"), makeRepair(secondRequired, "second"))
				if (merged.Metadata["completion_blocking"] == "true") != (firstRequired || secondRequired) {
					t.Fatalf("merge invented or lost a completion obligation: %+v", merged)
				}
				obligations, _ := decodeEmitEvidenceRelationRepairObligations(merged.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
				for _, row := range obligations {
					if (row.Subject == "first" && !firstRequired) || (row.Subject == "second" && !secondRequired) {
						t.Fatalf("optional relation became durable required debt: %+v", obligations)
					}
				}
				validation := buildEmitEvidenceItemValidationRepair([]string{"optional rejected sibling"}, []string{"items[2]"}, false)
				merged = mergeEmitEvidenceValidationRepairs(validation, makeRepair(secondRequired, "second"))
				if (merged.Metadata["completion_blocking"] == "true") != secondRequired {
					t.Fatalf("optional schema rejection plus argument guidance invented a hard gate: %+v", merged)
				}
			})
		}
	}
}

func TestStageArgumentScopePublicCannotEraseUnclassifiedLegacyDebt(t *testing.T) {
	bus, item := stageArgumentScopePublicContext(t, "observe", false)
	legacy := buildEmitEvidenceArgumentFlowRepair([]emitEvidenceArgumentFlowRepair{{
		argument: "types.StageAnalyze", receiver: "observe", source: "flow.go", line: 4,
	}})
	bus.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "emit_evidence", Success: true, Repair: legacy})
	b1685Emit(t, bus, item)
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(bus); pending == nil {
		t.Fatal("new optional guidance cannot erase an old durable obligation with no distinguishing authority basis")
	}
	// An explicit, source-verified model relation can still discharge it.
	b1685Emit(t, bus, b1685ArgumentItem("flow.go", 4, "types.StageAnalyze", "observe"))
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(bus); pending != nil {
		t.Fatalf("exact authored proof must still discharge legacy debt: %+v", pending)
	}
}

func TestStageArgumentScopePublicOptionalCandidateCannotMaskActualRepair(t *testing.T) {
	for _, schemaInvalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("schema_invalid=%t", schemaInvalid), func(t *testing.T) {
			bad := map[string]any{"scope": "line", "evidence_kind": "direct", "source": "missing.go",
				"line_start": 80, "anchor_kind": "definition", "anchor_symbol": "MissingDefinition",
				"subject": "MissingDefinition", "predicate": "defines", "object": "configuration",
				"summary": "A model-selected assertion requiring correction."}
			if schemaInvalid {
				bad["evidence_kind"], bad["anchor_kind"], bad["object"] = "relationship", "argument", ""
			}
			baseline, _ := stageArgumentScopePublicContext(t, "observe", false)
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{bad}})
			before, err := (&EmitEvidence{}).Execute(baseline, params)
			if err != nil {
				t.Fatal(err)
			}
			if before.Repair == nil || before.Repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired {
				t.Fatalf("premise: the independent sibling needs an actual repair: %+v", before)
			}
			bus, call := stageArgumentScopePublicContext(t, "observe", false)
			after := b1685Emit(t, bus, call, bad)
			if after.Repair == nil || after.Repair.Code != before.Repair.Code ||
				after.Repair.Metadata["repair_status"] != before.Repair.Metadata["repair_status"] ||
				after.Repair.Metadata["completion_blocking"] != before.Repair.Metadata["completion_blocking"] {
				t.Fatalf("optional candidate replaced or weakened independent repair: before=%+v after=%+v", before.Repair, after.Repair)
			}
			if !schemaInvalid && !reflect.DeepEqual(before.Repair.Targets, after.Repair.Targets) {
				t.Fatalf("grounding target identity changed: before=%+v after=%+v", before.Repair.Targets, after.Repair.Targets)
			}
			if !strings.Contains(after.Summary, "Optional exact source relation candidate") {
				t.Fatal("keeping the actual repair must not hide the optional factual argument candidate")
			}
		})
	}
}
