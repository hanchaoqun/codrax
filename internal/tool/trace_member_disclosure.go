package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The parent is one budgeted attempt, not an enclosing analysis window.
// Format each member's outcome independently. A display cap never clips the
// stored request, the completed observations, or the per-member audit record.
func runtimeTraceSupplementMemberDisclosure(meta *types.SystemTraceSupplementMeta, zh bool) string {
	const displayLimit = 8
	var parts []string
	first := map[[2]float64]int{}
	prefix := runtimeTraceSupplementDisclosurePrefixEN
	if zh {
		prefix = runtimeTraceSupplementDisclosurePrefixZH
	}
	for i, member := range meta.MemberWindows {
		if i >= displayLimit {
			break
		}
		leaf := &types.SystemTraceSupplementMeta{
			WindowStart: member.WindowStart, WindowEnd: member.WindowEnd,
			TargetPID: meta.TargetPID, TargetThread: meta.TargetThread,
			Views: member.Views, ViewValueObservations: member.ViewValueObservations,
			ViewObservationFamilies: member.ViewObservationFamilies,
			SkipReason:              member.SkipReason, SkippedViews: member.SkippedViews,
			CanceledViews: member.CanceledViews, WindowBudgetS: member.WindowBudgetS,
			DurationBudgetS: meta.DurationBudgetS,
		}
		text := runtimeTraceSupplementDisclosureText(leaf, zh)
		if text == "" {
			var reason string
			switch member.SkipReason {
			case types.TraceSupplementReasonDurationBudgetExceeded:
				if zh {
					reason = "全部时间窗共用的补采时限已用尽，未启动该窗剩余查询"
				} else {
					reason = "the shared time limit for all windows was exhausted before the remaining queries started"
				}
			case types.TraceSupplementReasonCanceledByCaller:
				if zh {
					reason = "本次运行已取消，未启动该窗剩余查询"
				} else {
					reason = "this run was canceled before the remaining queries started"
				}
			case types.TraceSupplementReasonExecutionFailed:
				if zh {
					reason = "补采查询失败，未获得完整结果"
				} else {
					reason = "the supplementary queries failed; no complete result was obtained"
				}
			case types.TraceSupplementReasonNoTypedTarget:
				if zh {
					reason = "尚未明确唯一目标线程，未执行该窗补采"
				} else {
					reason = "no unique target thread was established, so this window was not queried"
				}
			case types.TraceSupplementReasonColdBudgetExceeded:
				if zh {
					reason = "文件大小超过本次补采的首次读取预算，未执行该窗补采"
				} else {
					reason = "the file exceeds this attempt's initial-read budget, so this window was not queried"
				}
			default:
				// A no-op is not a coverage claim. Existing evidence remains in
				// its own account; this sentence claims no new query or result.
				if zh {
					reason = "本次未执行额外查询；已有证据及其覆盖范围见该窗分析"
				} else {
					reason = "no additional query ran; see this window's analysis for existing evidence and its coverage"
				}
			}
			text = fmt.Sprintf("%.6f..%.6f: %s", member.WindowStart, member.WindowEnd, reason)
		} else {
			text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
			if len(member.Views) > 0 && len(member.SkippedViews) > 0 {
				switch member.SkipReason {
				case types.TraceSupplementReasonExecutionFailed:
					if zh {
						text += "；另有补采查询失败，仅保留上述已完成结果"
					} else {
						text += "; other supplementary queries failed; only the completed results above were kept"
					}
				case types.TraceSupplementReasonCanceledByCaller:
					if zh {
						text += "；本次运行取消后未启动剩余查询，仅保留上述已完成结果"
					} else {
						text += "; remaining queries did not start after this run was canceled; completed results above were kept"
					}
				}
			}
		}
		key := [2]float64{member.WindowStart, member.WindowEnd}
		if prior, ok := first[key]; ok {
			if zh {
				text += fmt.Sprintf("（与第 %d 个时间窗共用同一次补采处理及状态，非重复执行）", prior)
			} else {
				text += fmt.Sprintf(" (shares the same supplementary-query attempt and status with window %d, not another execution)", prior)
			}
		} else {
			first[key] = i + 1
		}
		if zh {
			text = fmt.Sprintf("第 %d 个时间窗：%s", i+1, text)
		} else {
			text = fmt.Sprintf("Window %d: %s", i+1, text)
		}
		parts = append(parts, text)
	}
	if remaining := len(meta.MemberWindows) - displayLimit; remaining > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("另有 %d 个窗口的逐窗补采状态未在此展开；不能据此认定所有窗口均已完成分析", remaining))
		} else {
			parts = append(parts, fmt.Sprintf("%d additional windows are not expanded here; this does not establish complete analysis of all windows", remaining))
		}
	}
	if meta.CensusLite {
		lite := &types.SystemTraceSupplementMeta{CensusLite: true, CensusLitePattern: meta.CensusLitePattern}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(runtimeTraceSupplementDisclosureText(lite, zh), prefix)))
	}
	if len(parts) == 0 {
		return ""
	}
	return prefix + " " + strings.Join(parts, "\n")
}
