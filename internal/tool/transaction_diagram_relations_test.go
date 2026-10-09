package tool

import (
	"encoding/json"
	"fmt"
	"github.com/hanchaoqun/codrax/internal/types"
	"path/filepath"
	"strings"
	"testing"
)

func transactionDiagramFixture(t *testing.T) (*types.BusContext, types.ToolResult, []RuntimeDiagramRelation) {
	t.Helper()
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	start, end := 1.0, 1.05
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Mutable: types.NewMutableState("application rendering transaction observations"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到1.05秒"}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 3 {
		t.Fatalf("expected exactly three observed matches, not ambiguous/missing/identity rows: %+v", rows)
	}
	return ctx, r, rows
}

func transactionDiagramDoc(rows []RuntimeDiagramRelation) *types.AnswerDocumentV2 {
	var body strings.Builder
	body.WriteString("flowchart TD\n")
	var anchors []types.DiagramEdgeAnchor
	for _, row := range rows {
		fmt.Fprintf(&body, "%s[%q] -->|协议记录对应| %s[%q]\n", row.FromNode, row.FromLabel, row.ToNode, row.ToLabel)
		anchors = append(anchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "协议记录对应"})
	}
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "仅当前已发布记录内可对应的提交与消费；不表示调用、等待或整帧根因。"}, {ID: "handoffs", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body.String()}, EdgeAnchors: anchors}}}
}

func TestTransactionHandoffsDiagramPublicEmitAndPost(t *testing.T) {
	ctx, r, rows := transactionDiagramFixture(t)
	consumerCounts := map[string]int{}
	for _, row := range rows {
		if row.Kind != types.DiagramRelObserve {
			t.Fatal("promoted to causal relation")
		}
		consumerCounts[row.ToIdentity]++
		if len(row.SupportRefs) != 2 {
			t.Fatal("physical source references lost")
		}
	}
	foundShared := false
	for _, n := range consumerCounts {
		foundShared = foundShared || n == 2
	}
	if !foundShared {
		t.Fatal("same consumption was split into serial instances")
	}
	doc := transactionDiagramDoc(rows)
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("exact protocol observation diagram rejected: %v %+v", err, res)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("emit did not publish")
	}
	if mismatches := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, got, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(mismatches) != 0 {
		t.Fatalf("post-validator differs: %+v", mismatches)
	}
	if r.TraceEvidenceAuthority != nil || !strings.Contains(got.Blocks[1].Diagram.Body, "窗外关联背景") {
		t.Fatal("root authority invented or endpoint window hidden")
	}
	if rows := RuntimeDiagramRelations(types.ObservationLedger{}, nil); len(rows) != 0 {
		t.Fatal("empty history fabricated matching endpoints")
	}
}

func TestTransactionHandoffsDiagramScopedNegativeMatrix(t *testing.T) {
	for _, name := range []string{"reverse", "call", "wakeup", "forged_identity", "other_query", "other_source", "wider_window", "conflicting_payload"} {
		t.Run(name, func(t *testing.T) {
			ctx, r, rows := transactionDiagramFixture(t)
			doc := transactionDiagramDoc(rows[:1])
			a := &doc.Blocks[1].EdgeAnchors[0]
			switch name {
			case "reverse":
				a.FromIdentity, a.ToIdentity = a.ToIdentity, a.FromIdentity
			case "call":
				a.RelationKind = types.DiagramRelCall
			case "wakeup":
				a.RelationKind = types.DiagramRelWakeup
			case "forged_identity":
				a.ToIdentity += "forged"
			default:
				copy := r
				copy.Observations = append([]types.ObservationRecord(nil), r.Observations...)
				for i := range copy.Observations {
					rec := &copy.Observations[i]
					if rec.Predicate != TraceTransactionHandoffsPredicate {
						continue
					}
					if name == "other_query" {
						rec.SourceRef.QueryScopeID += "other"
					}
					if name == "other_source" {
						rec.SourceRef.Path += "other"
					}
					if name == "wider_window" {
						rec.SourceRef.QueryWindowEndTs += 1
					}
					if name == "conflicting_payload" {
						rec.ID += "conflicting_retained_record"
						p, _ := DecodeTraceTransactionHandoffs(*rec)
						p.Caveats = append(p.Caveats, "different payload")
						data, _ := json.Marshal(p)
						rec.RichNotes = []string{types.TraceNoteKeyTransactionHandoffs + "=" + string(data)}
					}
				}
				ctx.Mutable = types.NewMutableState("different current observations")
				results := []types.ToolResult{copy}
				if name == "conflicting_payload" {
					results = append(results, r)
				}
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			}
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success {
				t.Fatalf("unsupported observation diagram admitted: %s %v %+v", name, err, res)
			}
		})
	}
}

func TestTransactionHandoffsDiagramDefaultEnvelopeLabel(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("all transactions")}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "transaction_handoffs"})
	r, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !r.Success {
		t.Fatalf("default query failed %v %+v", err, r)
	}
	rows := RuntimeDiagramRelations(types.ObservationLedger{Records: r.Observations}, nil)
	if len(rows) != 3 {
		t.Fatalf("default relations: %+v", rows)
	}
	for _, row := range rows {
		if !strings.Contains(row.ScopeLabel, "1.060000000]秒") {
			t.Fatal("inclusive default envelope mislabeled", row.ScopeLabel)
		}
	}
}
