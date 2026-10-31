package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This component is added only after the model answer is selected. It does not
// enter the model's primary/principal surfaces or mint diagnosis/completion.
func finalizeTraceCatalogRepresentatives(ctx *types.AgentContext, answer, lang string) string {
	if ctx == nil || strings.TrimSpace(answer) == "" {
		return answer
	}
	rows := tool.MaterializeTraceCatalogRepresentativeDeliveries(types.ToolBusContext(ctx, types.AgentFinalizer))
	if len(rows) == 0 {
		return answer
	}
	zh := strings.HasPrefix(strings.ToLower(lang), "zh")
	var b strings.Builder
	if zh {
		b.WriteString("### 系统交付：抽样检查记录\n\n")
	} else {
		b.WriteString("### System delivery: representative check records\n\n")
	}
	for _, row := range rows {
		data, _ := json.Marshal(row)
		// Operator-only typed receipt: the audit need not parse model prose or
		// read a mutable latest alias to find the delivered generation.
		logging.Info("[agent/terminal_representatives] receipt=%s", data)
		if row.Status != "published" || row.ExecutionCoverage == nil || row.RepresentativeCoverage == nil {
			if zh {
				fmt.Fprintf(&b, "- 抽样检查记录未完成交付（%s）；没有新增有效下载链接。\n", row.Status)
			} else {
				fmt.Fprintf(&b, "- Representative check records were not delivered (%s); no new verified download link.\n", row.Status)
			}
			continue
		}
		e, r := row.ExecutionCoverage, row.RepresentativeCoverage
		if zh {
			fmt.Fprintf(&b, "- 同一代检查记录：[执行索引](<%s>) · [代表与未选清单](<%s>)。登记计划 %d 项、额外查询 %d 项；实际尝试 %d 次，涉及 %d 条查询记录。规格组 %d 组，选中 %d 项，预算未选 %d 项。\n", exportDeliveryLink(row.IndexPath), exportDeliveryLink(row.SelectionPath), e.DeclaredPlans+e.ResumedPlans, e.ObservedQueries, e.Attempts, e.QueriesWithAttempts, r.Clusters, r.Selected, r.BudgetNotSelected)
		} else {
			fmt.Fprintf(&b, "- Same-generation records: [Execution index](<%s>) · [Selection snapshot](<%s>). Registered plans: %d; additional queries: %d; actual attempts: %d across %d query records. Specification groups: %d; selected: %d; budget-unselected: %d.\n", exportDeliveryLink(row.IndexPath), exportDeliveryLink(row.SelectionPath), e.DeclaredPlans+e.ResumedPlans, e.ObservedQueries, e.Attempts, e.QueriesWithAttempts, r.Clusters, r.Selected, r.BudgetNotSelected)
		}
		fmt.Fprintf(&b, "  SHA-256 (index): `%s`; SHA-256 (selection): `%s`.\n", row.IndexSHA256, row.SelectionSHA256)
	}
	if zh {
		b.WriteString("\n记录保留成功、已查空、失败与未执行的区别；已交付记录不表示全部材料已查询、分析已完成或存在共同根因。")
	} else {
		b.WriteString("\nThe records keep success, empty, failure and unexecuted work distinct. Delivering them does not prove all materials were queried, the analysis is complete, or a common cause exists.")
	}
	return answer + "\n\n" + b.String()
}
