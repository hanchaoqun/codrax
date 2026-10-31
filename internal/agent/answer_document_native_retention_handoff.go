package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderAnswerDocNativeFactRetentionBoundary(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	status := ctx.Mutable.NativeFactRetentionStatus()
	if !status.CapacityOmitted {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(extractAnswerDocLang(ctx)), "zh") {
		return fmt.Sprintf("\n## 原生查询事实留存边界\n本次运行的原生事实留存容量已触及上限（%d项查询、%d编码字节；不是内存上限）。至少一项已查询原件未进入独立留存；最近一项来源=%q，查询=%q。此收据不是遗漏全集。历史结果中仍可能保有该查询；若成文上下文缺少所需原件，需按实际查询身份补查或明确缺证，不能称零结果、查询未执行或全部完成，也不能借另一窗口替代。已保留事实继续适用原来源、时间窗、完整性和因果权限，不因留存而升权。\n", status.QueryLimit, status.EncodedByteLimit, status.LastSource, status.LastQuery)
	}
	return fmt.Sprintf("\n## Native query retention boundary\nThis run reached the native-fact retention capacity (%d queries, %d encoded bytes; not a heap limit). At least one executed native result was not retained independently; last source=%q, query=%q. This bounded receipt is not a complete omission roster. History may still contain that result; if required native facts are absent from the writing context, query the actual identity again or state the evidence gap. Do not infer zero results, unexecuted queries, completion, or substitute another window. Retained facts keep their original source, scope, completeness and causal permissions.\n", status.QueryLimit, status.EncodedByteLimit, status.LastSource, status.LastQuery)
}
