package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public emitter and the production IR compiler before either
// prompt consumer. The small graph below is only navigation scaffolding for a
// real read_file result; it does not supply or replace request classification.
func TestB1691PublicAnalysisPreservesDownstreamAnswerShape(t *testing.T) {
	for _, multi := range []bool{true, false} {
		name := "single_role_literal"
		if multi {
			name = "call_chain_and_required_dimensions"
		}
		t.Run(name, func(t *testing.T) {
			objective := "Which single name is responsible for worker?"
			var payload map[string]any
			if err := json.Unmarshal([]byte(`{
				"intent":"explain","scenario":"architecture_explain","complexity":"moderate",
				"keywords":["worker","entry","flow"],"entities":["worker"],
				"question_kind":"mechanism","predicate_axis":"call",
				"answer_subject":{"kind":"generic","confidence":0.7},
				"intent_confidence":0.7,"complexity_confidence":0.7,"kind_confidence":0.7,
				"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":true,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
				"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.7},
				"answer_role_profile":{"is_role_binding_requested":true,"required_candidate_roles":["agent"],"source_quotes":["worker"],"confidence":0.9},
				"error_granularity_profile":{"is_granularity_question":false,"confidence":0.7},
				"runtime_artifact_scope_profile":{"requested_scope":"not_applicable","confidence":0.7},
				"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.7},
				"completeness_obligation":{"required":false,"source_quote":""},
				"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
				"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.7},
				"requested_answer_dimensions":{"is_dimensioned_answer":false,"confidence":0.7},
				"runtime_target_profile":{"declaration":"unspecified","confidence":0.7},
				"runtime_question_profile":{"scope":"unspecified","runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.7}
			}`), &payload); err != nil {
				t.Fatal(err)
			}
			if multi {
				objective = "Follow Engine.Begin through worker; explain dimension1 and dimension2."
				payload["intent"], payload["question_kind"] = "trace", "call_chain"
				payload["entities"] = []string{"Engine.Begin", "worker"}
				payload["predicates"].(map[string]any)["is_relational_lookup"] = false
				payload["call_chain_endpoints"] = map[string]any{"source": "Engine.Begin", "sink": "", "sink_mode": "discover_path"}
				payload["requested_answer_dimensions"] = map[string]any{
					"is_dimensioned_answer": true, "confidence": 0.95,
					"dimensions": []any{
						map[string]any{"index": 1, "label": "dimension1", "role": "relation_path", "source_quote": "dimension1", "required": true},
						map[string]any{"index": 2, "label": "dimension2", "role": "function_or_purpose", "source_quote": "dimension2", "required": true},
					},
				}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			originalRaw := append([]byte(nil), raw...)
			root := t.TempDir()
			ctx := &types.AgentContext{Stage: types.StageAnalyze, Objective: objective, RepoRoot: root, WorkDir: root, Language: "en", Mutable: types.NewMutableState(objective)}
			result, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
			if err != nil || !result.Success || ctx.Mutable.RequestModel() == nil {
				t.Fatalf("public emit must succeed before downstream assertions: err=%v result=%+v", err, result)
			}
			if !bytes.Equal(raw, originalRaw) {
				t.Fatal("emitter mutated input bytes")
			}
			ir, err := buildAnalysisIR(ctx)
			if err != nil {
				t.Fatalf("production buildAnalysisIR: %v", err)
			}
			ctx.AnalysisIR = ir
			beforeIR, _ := json.Marshal(ir)
			ctx.Stage = types.StageExplore
			explorer := &explorerEvaluator{}
			explorerPrompt := explorer.BuildInitialInstruction(ctx, nil)
			if explorerPrompt == "" {
				t.Fatal("explorer initialization must produce an instruction")
			}
			if multi {
				if ir.RequestModel.Predicates.IsScalarAnswer || ir.RequestModel.Predicates.IsRoleLocateLookup {
					t.Error("public whole-request shape became scalar before downstream prompting")
				}
				if ir.RequestModel.RequestedAnswerDimensions == nil || len(ir.RequestModel.RequestedAnswerDimensions.Dimensions) != 2 {
					t.Fatal("required dimensions did not survive public emitter and IR")
				}
				if !strings.Contains(explorerPrompt, "Call-edge Evidence Handoff") {
					t.Error("explorer lost call-chain operation-evidence guidance")
				}
			}
			if got := explorer.scalarSourceLiteralPrimaryReadMode(); got == multi {
				t.Errorf("downstream scalar surface selection=%t, want %t", got, !multi)
			}

			const source = "package sample\n\ntype Engine struct{}\nfunc (Engine) Begin() {\n worker()\n}\nfunc worker() {}\n"
			if err := os.WriteFile(filepath.Join(root, "entry.go"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			read, err := (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"path":"entry.go","line_offset":0,"limit":5}`))
			if err != nil || !read.Success || read.ReadCoverage == nil {
				t.Fatalf("real source read prerequisite failed: err=%v result=%+v", err, read)
			}
			explorer.searchResult = &keywordSearchResult{Graph: &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{
				"Engine.Begin": {{Name: "Engine.Begin", Kind: "function", File: "entry.go", Line: 4, EndLine: 6}},
			}}}
			explorer.phase, explorer.primaryReadSeen, explorer.primaryReadIter = 1, true, 0
			sig := explorer.postPrimaryReadMidLoopSignal(LoopObservation{Phase: PhaseMidLoop, Iteration: 0, LastToolResult: &read, AllToolResults: []types.ToolResult{read}})
			// The non-scalar request may legitimately need no immediate nudge;
			// do not require it to incur unrelated function-body reading debt.
			if !multi && !sig.HintRequested {
				t.Fatal("single-literal role lookup must retain its post-primary hint")
			}
			if got := strings.Contains(sig.Hint, "scalar source-literal lookup"); got == multi {
				t.Errorf("post-primary read wrongly selects scalar-only discipline=%t: %s", got, sig.Hint)
			}
			ctx.Stage = types.StageFinalize
			finalPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if got := strings.Contains(finalPrompt, "## Scalar Lookup Discipline"); got == multi {
				t.Errorf("finalizer wrongly selects scalar-only discipline=%t", got)
			}
			if multi {
				for _, want := range []string{"Dimension 1: dimension1", "Dimension 2: dimension2"} {
					if !strings.Contains(finalPrompt, want) {
						t.Errorf("finalizer lost independent requested dimension %q", want)
					}
				}
			}
			if !multi && !strings.Contains(finalPrompt, "answer with the located literal and its file:line first") {
				t.Error("legitimate one-name role lookup lost its focused finalizer instruction")
			}
			afterIR, _ := json.Marshal(ir)
			if !bytes.Equal(beforeIR, afterIR) {
				t.Fatal("prompt consumers mutated production AnalysisIR")
			}
		})
	}
}
