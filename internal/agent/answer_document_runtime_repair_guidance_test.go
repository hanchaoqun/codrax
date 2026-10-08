package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeDiagramPublicRepairDoesNotDemandSourceCall(t *testing.T) {
	for _, view := range []string{"thread_timeline", "wakeup_chain"} {
		t.Run(view, func(t *testing.T) {
			path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("画出观察到的唤醒关系"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow}}}
			raw, _ := json.Marshal(map[string]any{"path": path, "view": view, "pid": 200, "time_start": 5.0, "time_end": 5.041})
			query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !query.Success {
				t.Fatalf("query: %v %+v", err, query)
			}
			ctx.Mutable.AppendDispatchToolResult(query)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
			from, to := "A", "B"
			if view == "wakeup_chain" {
				rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
				if len(rows) == 0 {
					t.Fatal("fixture has no exact wakeup pair")
				}
				from, to = rows[0].FromNode, rows[0].ToNode
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "只描述所选窗口的观察事件。"},
				{ID: "diag", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant " + from + " as source\n participant " + to + " as target\n " + from + "->>" + to + ": 唤醒\n"}},
			}}
			raw, _ = json.Marshal(doc)
			res, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || res.Success {
				t.Fatalf("unanchored runtime arrow accepted: %v %+v", err, res)
			}
			hint, ok := answerDocDiagramRelationDeltaPatchHint(&res, false, false)
			if !ok || !strings.Contains(hint, "local typed relation mismatch") || strings.Contains(hint, "typed source relation mismatch") {
				t.Fatalf("runtime repair reclassified as source call: %s", hint)
			}
			if !strings.Contains(res.Summary, "does not turn runtime events into source calls") {
				t.Fatalf("tool rejection lacks namespace boundary: %s", res.Summary)
			}
			if view == "thread_timeline" && !strings.Contains(res.Summary, "only answer/patch tools") {
				t.Fatal("repair demands unavailable collection tools")
			}
			if view == "wakeup_chain" {
				delta, _, ok := parseAnswerDocDiagramRelationRepairDelta(&res)
				found := false
				for _, candidate := range delta.AllowedAdditions {
					found = found || candidate.RelationKind == types.DiagramRelWakeup
				}
				if !ok || !found {
					t.Fatalf("exact runtime repair credential disappeared: %s", hint)
				}
			}
		})
	}
}
