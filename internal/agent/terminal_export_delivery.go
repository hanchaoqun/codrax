package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// FinalizeTraceCatalogExports is a deterministic agent boundary, not a model
// tool. The host calls it only after the final answer has been selected and
// all exploration has joined. It publishes accepted requests exactly once;
// it neither performs missing queries nor changes the model's interpretation.
func FinalizeTraceCatalogExports(ctx *types.AgentContext, answer, lang string) string {
	if ctx == nil || ctx.PipelineExports == nil || strings.TrimSpace(answer) == "" {
		return answer
	}
	requests, err := tool.MaterializePendingTraceCatalogExports(types.ToolBusContext(ctx, types.AgentFinalizer))
	if err != nil {
		logging.Warning("[agent/terminal_export] materialization: %v", err)
	}
	for _, request := range requests {
		data, _ := json.Marshal(request)
		logging.Info("[agent/terminal_export] request=%s", data)
		if request.Delivery != nil && request.Delivery.Receipt.PhysicalSuccess {
			data, _ := json.Marshal(request.Delivery.Receipt)
			logging.Info("[agent/terminal_export] receipt=%s", data)
		}
	}
	component := renderPublishedAnswerExportDeliveries(ctx, lang)
	zh := strings.HasPrefix(strings.ToLower(lang), "zh")
	var failures strings.Builder
	for _, request := range requests {
		if request.Status == "published" {
			continue
		}
		// Render the accepted name as literal metadata, not Markdown links.
		// Missing files have no download link and the target itself is unchanged.
		target := terminalExportTargetLiteral(request.Target)
		if zh {
			fmt.Fprintf(&failures, "\n- 证据包 %s 未完成交付（%s）；没有新增有效下载链接。", target, request.Status)
		} else {
			fmt.Fprintf(&failures, "\n- Evidence package %s was not delivered (%s); no new verified download link.", target, request.Status)
		}
	}
	if err != nil && len(requests) == 0 {
		if zh {
			failures.WriteString("\n证据包交付未通过校验；没有新增有效下载链接。")
		} else {
			failures.WriteString("\nEvidence-package delivery could not be verified; no new verified download link.")
		}
	}
	component = strings.TrimSpace(component + "\n" + failures.String())
	if component == "" {
		return answer
	}
	return answer + "\n\n" + component
}

func terminalExportTargetLiteral(target string) string {
	// Choose a code-span delimiter longer than any run inside the name.
	// Padding preserves leading/trailing backticks under CommonMark rules;
	// valid export names cannot contain leading/trailing whitespace or newlines.
	longest, run := 0, 0
	for _, r := range target {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	delimiter := strings.Repeat("`", longest+1)
	return delimiter + " " + target + " " + delimiter
}
