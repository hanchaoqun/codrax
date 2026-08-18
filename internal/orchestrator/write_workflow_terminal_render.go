package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderWriteWorkflowTerminalStatus publishes the final workflow verdict after
// every batch-local apply/verify card. A passed ChangeReport is only a local
// observation; the workflow may still finish unverified when typed proof or
// impact obligations remain open. Rendering from WriteWorkflowRun.Completion
// keeps that terminal authority visible without inspecting or rewriting prior
// result prose.
func renderWriteWorkflowTerminalStatus(run types.WriteWorkflowRun, lang string) string {
	run = types.NormalizeWriteWorkflowRun(run)
	if run.Status != types.WriteWorkflowRunComplete || run.Completion == nil {
		return ""
	}
	zh := isLangZh(lang)
	switch run.Completion.Verdict {
	case types.WriteWorkflowCompletionVerified:
		if zh {
			return "\n\n## 最终交付状态：已验证\n\n所有批次均已完成验证。\n"
		}
		return "\n\n## Final delivery status: verified\n\nAll batches completed recorded verification.\n"
	case types.WriteWorkflowCompletionUnverified:
		details := writeWorkflowTerminalUnverifiedDetails(run, zh)
		if zh {
			return "\n\n## 最终交付状态：未完全验证\n\n交付内容已保留，但最终不能标记为“已验证”。" + details +
				"这不是已确认的代码失败；合并前应补齐相应验证。\n"
		}
		return "\n\n## Final delivery status: unverified\n\nThe delivery artifacts were preserved, but the final workflow cannot be marked verified. " + details +
			"This is not a confirmed code failure; complete the missing verification before merging.\n"
	case types.WriteWorkflowCompletionAcceptedFailed:
		if zh {
			return "\n\n## 最终交付状态：已接受验证失败\n\n验证失败已被明确接受；合并前必须人工审查结构化报告中的失败证据。\n"
		}
		return "\n\n## Final delivery status: verification failure accepted\n\nThe verification failure was explicitly accepted; review the failure evidence in the structured report before merging.\n"
	default:
		return ""
	}
}

func writeWorkflowTerminalUnverifiedDetails(run types.WriteWorkflowRun, zh bool) string {
	var rows []string
	for _, batch := range run.Batches {
		if batch.Completion == nil || batch.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
			continue
		}
		batchID := strings.TrimSpace(batch.ID)
		if batchID == "" {
			batchID = "batch"
		}
		reason := writeWorkflowTerminalReasonLabel(batch.Completion.ReasonCode, zh)
		if zh {
			rows = append(rows, fmt.Sprintf("`%s`（%s）", batchID, reason))
		} else {
			rows = append(rows, fmt.Sprintf("`%s` (%s)", batchID, reason))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, writeWorkflowTerminalReasonLabel(run.Completion.ReasonCode, zh))
	}
	reasonClass := writeWorkflowTerminalUnverifiedReasonClass(run)
	if zh {
		explanation := "本地验证能力或证明覆盖不完整。"
		switch reasonClass {
		case "proof_incomplete":
			explanation = "测试结果可能已经通过，但声明的行为或影响证明仍未完全闭合。"
		case "no_tests":
			explanation = "没有测试实际执行。"
		case "runner_unavailable":
			explanation = "本地测试运行器、依赖或结果解析能力不可用。"
		}
		return "未闭合批次：" + strings.Join(rows, "、") + "；" + explanation
	}
	explanation := "Local verification capability or proof coverage is incomplete. "
	switch reasonClass {
	case "proof_incomplete":
		explanation = "Tests may have passed, but the declared behavior or impact proof is still incomplete. "
	case "no_tests":
		explanation = "No tests actually ran. "
	case "runner_unavailable":
		explanation = "The local test runner, dependencies, or result parser were unavailable. "
	}
	return "Open batch(es): " + strings.Join(rows, ", ") + ". " + explanation
}

// writeWorkflowTerminalReasonLabel keeps workflow reason codes in typed audit
// artifacts while presenting only their reader-facing meaning in the terminal
// status card. Unknown future codes fail closed to a generic explanation rather
// than leaking a protocol token into user-facing prose.
func writeWorkflowTerminalReasonLabel(reasonCode string, zh bool) string {
	code := strings.TrimSpace(reasonCode)
	if zh {
		switch code {
		case "production_verification_source_static_only":
			return "生产验证目前只有静态证据"
		case "verification_proof_incomplete":
			return "行为或影响证明尚未闭合"
		case "runner_missing":
			return "本地测试运行器不可用"
		case "parser_error":
			return "测试结果无法可靠解析"
		case "verification_incomplete":
			return "本地验证未完整执行"
		case "no_tests":
			return "没有测试实际执行"
		case "tests_failed", "verification_failed":
			return "测试或验证失败"
		default:
			return "验证未闭合，具体原因见结构化报告"
		}
	}
	switch code {
	case "production_verification_source_static_only":
		return "production verification currently has static evidence only"
	case "verification_proof_incomplete":
		return "behavior or impact proof remains incomplete"
	case "runner_missing":
		return "the local test runner was unavailable"
	case "parser_error":
		return "the test result could not be parsed reliably"
	case "verification_incomplete":
		return "local verification did not complete"
	case "no_tests":
		return "no tests actually ran"
	case "tests_failed", "verification_failed":
		return "tests or verification failed"
	default:
		return "verification remains incomplete; see the structured report for details"
	}
}

func writeWorkflowTerminalUnverifiedReasonClass(run types.WriteWorkflowRun) string {
	class := ""
	for _, batch := range run.Batches {
		if batch.Completion == nil || batch.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
			continue
		}
		switch strings.TrimSpace(batch.Completion.ReasonCode) {
		case "verification_proof_incomplete":
			return "proof_incomplete"
		case "runner_missing", "parser_error", "verification_incomplete":
			class = "runner_unavailable"
		case "no_tests":
			if class == "" {
				class = "no_tests"
			}
		}
	}
	return class
}
