package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func renderTraceFindingShortRootCause(ctx *types.AgentContext, lang string) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	return renderTraceFindingShortRootCauseValue(ctx.Mutable.TraceFinding(), lang)
}

func renderTraceFindingShortRootCauseValue(finding *types.TraceFindingV1, lang string) string {
	if finding == nil {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	if finding.PrimaryCause == nil {
		reason := "available evidence is insufficient to identify a root cause"
		title := "## Short root cause"
		if finding.Unresolved != nil && strings.TrimSpace(finding.Unresolved.Reason) != "" {
			reason = compactFindingText(finding.Unresolved.Reason, 140)
		}
		if zh {
			title = "## 简短根因"
			if finding.Unresolved == nil || strings.TrimSpace(finding.Unresolved.Reason) == "" {
				reason = "现有证据不足，暂时无法确定根因"
			}
			return title + "\n\n**未确定**：" + reason + "。"
		}
		return title + "\n\n**Unresolved:** " + reason + "."
	}

	primary := *finding.PrimaryCause
	label := traceFindingCauseLabel(primary.Token.Token, zh)
	subject := traceFindingRoleLabel(primary.SubjectRole, zh)
	phase := traceFindingPhaseLabel(primary.Phase, zh)
	status := "Strongest supported candidate"
	title := "## Short root cause"
	if primary.Status == types.TraceCausalProven {
		status = "Root cause"
	}
	if zh {
		title = "## 简短根因"
		status = "最强根因候选"
		if primary.Status == types.TraceCausalProven {
			status = "根因"
		}
	}
	parts := []string{label}
	if subject != "" {
		parts = append(parts, subject)
	}
	if phase != "" {
		parts = append(parts, phase)
	}
	line := fmt.Sprintf("**%s：%s**", status, strings.Join(parts, " · "))
	if !zh {
		line = fmt.Sprintf("**%s: %s**", status, strings.Join(parts, " · "))
	}
	if primary.Magnitude != nil && primary.Magnitude.Value > 0 {
		line += fmt.Sprintf("（%.3f %s）", primary.Magnitude.Value, primary.Magnitude.Unit)
		if !zh {
			line = strings.TrimSuffix(line, fmt.Sprintf("（%.3f %s）", primary.Magnitude.Value, primary.Magnitude.Unit)) +
				fmt.Sprintf(" (%.3f %s)", primary.Magnitude.Value, primary.Magnitude.Unit)
		}
	}
	if len(finding.Contributors) > 0 {
		labels := make([]string, 0, 3)
		for _, contributor := range finding.Contributors {
			labels = append(labels, traceFindingCauseLabel(contributor.Token.Token, zh))
			if len(labels) == 3 {
				break
			}
		}
		if zh {
			line += "\n\n次要因素：" + strings.Join(labels, "、") + "。"
		} else {
			line += "\n\nContributors: " + strings.Join(labels, ", ") + "."
		}
	}
	return title + "\n\n" + line
}

func prependAnswerSupplement(prose, supplement string) string {
	supplement = strings.TrimSpace(supplement)
	if supplement == "" || strings.Contains(prose, supplement) {
		return prose
	}
	if strings.TrimSpace(prose) == "" {
		return supplement
	}
	return supplement + "\n\n" + prose
}

func traceFindingCauseLabel(token string, zh bool) string {
	token = strings.TrimSpace(token)
	if zh {
		if label := tool.TraceRootCauseTypeZHLabel(token); label != "" {
			return label + "（" + token + "）"
		}
		return token
	}
	return strings.ReplaceAll(token, "_", " ")
}

func traceFindingRoleLabel(role string, zh bool) string {
	if !zh {
		return strings.ReplaceAll(strings.TrimSpace(role), "_", " ")
	}
	switch strings.TrimSpace(role) {
	case "target_thread":
		return "目标线程"
	case "upstream_dependency":
		return "上游依赖线程"
	case "lock_holder":
		return "锁持有线程"
	case "aggregate_metric":
		return "系统级指标"
	case "causal_worker":
		return "关联工作线程"
	default:
		return ""
	}
}

func traceFindingPhaseLabel(phase string, zh bool) string {
	phase = strings.TrimSpace(phase)
	if phase == "" || phase == "unknown" {
		return ""
	}
	if zh && phase == "pre_wakeup_dependency" {
		return "唤醒前依赖阶段"
	}
	return strings.ReplaceAll(phase, "_", " ")
}

func compactFindingText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
