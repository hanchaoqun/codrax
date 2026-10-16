package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func exportedAnswerContextHMC251(t *testing.T) (*types.AgentContext, tracecatalog.ExportReceipt) {
	t.Helper()
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	catalogs := ctx.Mutable.TraceCatalogs()
	if len(catalogs) != 1 {
		t.Fatalf("current catalogs: %d", len(catalogs))
	}
	args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": catalogs[0].ID(), "export": map[string]string{"target": "capture-review"}})
	result, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("real export: %v %s", err, result.Summary)
	}
	var receipt tracecatalog.ExportReceipt
	if err := json.Unmarshal([]byte(result.Summary), &receipt); err != nil || !receipt.PhysicalSuccess {
		t.Fatalf("receipt: %v %+v", err, receipt)
	}
	return ctx, receipt
}

func TestHMC251ExportReceiptRecoveryAndCurrentAuthority(t *testing.T) {
	for _, mode := range []string{"raw", "text_recovery", "rerender", "revoked", "json", "package_changed", "source_changed", "execution_changed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, receipt := exportedAnswerContextHMC251(t)
			model := "当前记录需按原始来源分别解读。"
			var rendered string
			switch mode {
			case "revoked":
				ctx.RepoRoot, ctx.UserPinnedFiles = t.TempDir(), nil
			case "json":
				wire, _ := json.Marshal(receipt)
				ctx.Mutable = types.NewMutableState("export")
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{Success: true, Summary: string(wire)}}})
			case "package_changed":
				if err := os.WriteFile(receipt.ReportPath, []byte("external edit"), 0600); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				path := filepath.Join(ctx.RepoRoot, "captures", "capture.data")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "execution_changed":
				args := json.RawMessage(`{"source":"path","path":"captures/capture.data","view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}`)
				out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
				if err != nil || !out.Success {
					t.Fatalf("new real attempt: %v %s", err, out.Summary)
				}
			}
			if mode == "rerender" {
				rendered = RenderAnswerDocumentWithLastMileSupplements(ctx, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: model}}}, nil, "zh")
			} else {
				if mode == "text_recovery" {
					wire, _ := json.Marshal(map[string]any{"blocks": []any{map[string]string{"id": "model", "kind": "summary", "text": model}}})
					model = string(wire)
				}
				out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: model}}, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				rendered = out.FinalAnswer
			}
			hasLink := strings.Contains(rendered, receipt.ManifestPath)
			wantLink := mode != "revoked" && mode != "json" && mode != "package_changed"
			if hasLink != wantLink {
				t.Fatalf("%s delivery link=%v want=%v: %s", mode, hasLink, wantLink, rendered)
			}
			if mode == "package_changed" && !strings.Contains(rendered, "文件复核未通过") {
				t.Fatal("invalid publication silently disappeared")
			}
			if (mode == "source_changed" || mode == "execution_changed") && (!strings.Contains(rendered, "不代表当前执行完成") || strings.Contains(renderAnswerExportDeliveries(ctx, "zh"), "与当前登记执行一致")) {
				t.Fatal("frozen package promoted to current completion")
			}
		})
	}
}

func TestHMC251ExportReceiptActualFinalizerDelivery(t *testing.T) {
	ctx, receipt := exportedAnswerContextHMC251(t)
	const prose = "仅解释当前采集记录；空查询不是线程不存在的证明。"
	args, _ := json.Marshal(map[string]any{"blocks": []any{map[string]string{"id": "interpretation", "kind": "summary", "text": prose}}})
	r, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
	if err != nil || !r.Success {
		t.Fatalf("emit: %v %s", err, r.Summary)
	}
	final, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{receipt.ManifestPath, receipt.ReportPath} {
		if !strings.Contains(final.FinalAnswer, path) {
			t.Errorf("actual final answer lost verified delivery %s", path)
		}
	}
	if !strings.Contains(dependencyObservationMessages(t, ctx), receipt.ManifestPath) {
		t.Error("actual finalizer context lost verified delivery")
	}
	surfaces := ctx.Mutable.AnswerRenderedSurfaces()
	if !strings.Contains(surfaces.Primary, prose) || strings.Contains(surfaces.Primary, receipt.ManifestPath) || strings.Contains(surfaces.Principal, receipt.ManifestPath) {
		t.Fatal("system delivery changed attribution of model-owned answer")
	}
}
