package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The prompt has a bounded whole-record preview. The final system delivery
// below enumerates all private receipts; a preview is never a completion claim.
func renderAnswerExportDeliveryContext(ctx *types.AgentContext) string {
	rows, _ := tool.AuthorizedTraceCatalogDeliveries(types.ToolBusContext(ctx, types.AgentFinalizer))
	if len(rows) == 0 {
		return ""
	}
	const maxPromptReceipts = 16
	const maxPromptBytes = 16384
	var shown []json.RawMessage
	bytes := 2
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil || len(shown) >= maxPromptReceipts || bytes+len(data)+1 > maxPromptBytes {
			continue // Whole receipts only; never truncate paths or completion axes.
		}
		shown = append(shown, data)
		bytes += len(data) + 1
	}
	data, _ := json.Marshal(shown)
	return fmt.Sprintf("\n\n## Verified evidence-package deliveries\nThese are host-verified file delivery states, not diagnostic evidence or whole-question completion. The system appends their current delivery links; report.md is an execution record, not an analysis report. A valid frozen package with current_match=false describes an earlier execution revision. Shown=%d of %d complete receipt records.\n%s\n", len(shown), len(rows), data)
}

// File delivery is system-owned on normal, recovered and raw fallback paths.
// Do not change Primary/Principal or scan/rewrite the model's interpretation.
func renderAnswerExportDeliveries(ctx *types.AgentContext, lang string) string {
	rows, _ := tool.AuthorizedTraceCatalogDeliveries(types.ToolBusContext(ctx, types.AgentFinalizer))
	if len(rows) == 0 {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(lang), "zh")
	var b strings.Builder
	if zh {
		b.WriteString("### 系统交付：证据包\n\n")
	} else {
		b.WriteString("### System delivery: evidence packages\n\n")
	}
	for _, row := range rows {
		r := row.Receipt
		target := strings.NewReplacer("`", "\\`", "\n", " ", "\r", " ").Replace(r.Target)
		if !row.PackageValid {
			if zh {
				fmt.Fprintf(&b, "- `%s`：文件复核未通过，不能作为当前有效交付。\n", target)
			} else {
				fmt.Fprintf(&b, "- `%s`: file verification failed; not a valid current delivery.\n", target)
			}
			continue
		}
		state, material := "verified; matches current registered execution", "bound reference material is complete"
		if !row.CurrentMatch {
			state = "verified frozen package; does not match current execution"
		}
		if !r.MaterialComplete {
			material = "reference material is incomplete; see the manifest for omissions"
		}
		if zh {
			state, material = "已生成并复核；与当前登记执行一致", "引用材料完整"
			if !row.CurrentMatch {
				state = "已复核的冻结包；不代表当前执行完成"
			}
			if !r.MaterialComplete {
				material = "引用材料不完整，遗漏见清单"
			}
			fmt.Fprintf(&b, "- `%s`：%s；%s。\n  [执行记录](<%s>) · [证据清单](<%s>)。包内登记：计划 %d 项、额外查询 %d 项、尝试 %d 次。\n", target, state, material, exportDeliveryLink(r.ReportPath), exportDeliveryLink(r.ManifestPath), r.ExecutionCoverage.DeclaredPlans+r.ExecutionCoverage.ResumedPlans, r.ExecutionCoverage.ObservedQueries, r.ExecutionCoverage.Attempts)
		} else {
			fmt.Fprintf(&b, "- `%s`: %s; %s.\n  [Execution record](<%s>) · [Evidence manifest](<%s>). Recorded in package: %d plans, %d additional queries, %d attempts.\n", target, state, material, exportDeliveryLink(r.ReportPath), exportDeliveryLink(r.ManifestPath), r.ExecutionCoverage.DeclaredPlans+r.ExecutionCoverage.ResumedPlans, r.ExecutionCoverage.ObservedQueries, r.ExecutionCoverage.Attempts)
		}
	}
	if zh {
		b.WriteString("\n执行记录不是诊断报告；材料完整不表示整个用户问题已分析完成。")
	} else {
		b.WriteString("\nAn execution record is not a diagnosis; complete reference material does not prove the whole question was analyzed.")
	}
	return b.String()
}

func exportDeliveryLink(path string) string {
	return strings.NewReplacer("%", "%25", "<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D").Replace(path)
}
