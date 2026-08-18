package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocTracePrincipalValueAuthority is the final, compact typed
// numeric handoff shown immediately before the submission checklist. The same
// target resolver and authority builders feed the post-finalize deterministic
// leading cards, so the model sees their principal values before composing its
// prose instead of learning them only after emit.
//
// This is deliberately soft guidance. It reads no raw request, model thinking,
// or draft answer text and does not reject or rewrite the answer.
func renderAnswerDocTracePrincipalValueAuthority(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	authorityRM := tool.RuntimeTraceAuthorityRequestModelForAgentContext(ctx)
	if authorityRM == nil {
		return ""
	}
	ledger := answerDocObservationLedger(ctx)
	projectionSet := types.CompileTraceCausalProjectionSet(ledger)
	stateAllowed := types.RuntimeTraceTargetStateMaterializationAllowed(authorityRM, projectionSet)
	waitAllowed := types.RuntimeTraceTargetWaitMaterializationAllowed(authorityRM, projectionSet)
	wakeupAllowed := types.RuntimeTraceWakeupEdgeMaterializationAllowed(authorityRM, projectionSet)
	if !stateAllowed && !waitAllowed && !wakeupAllowed {
		return ""
	}
	var states []types.TraceTargetStateScopeAuthority
	if stateAllowed {
		states = types.BuildTraceTargetStateScopeAuthorities(projectionSet)
	}
	var waits []types.TraceTargetWaitSummaryAuthority
	var blocking []types.TraceBlockingWallClockAuthority
	if waitAllowed {
		waits = types.BuildTraceTargetWaitSummaryAuthorities(ledger, authorityRM)
		blocking = types.BuildTraceBlockingWallClockAuthorities(ledger, authorityRM)
	}
	var wakeupEdges []types.TraceWakeupEdgeRoleAuthority
	if wakeupAllowed {
		wakeupEdges = types.BuildTraceWakeupEdgeRoleAuthorities(ledger, authorityRM)
	}
	stateRosterTruncated := stateAllowed && len(projectionSet.OmittedArtifactLabels) > 0
	if len(states) == 0 && len(waits) == 0 && len(blocking) == 0 && len(wakeupEdges) == 0 && !stateRosterTruncated {
		return ""
	}

	var b strings.Builder
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(extractAnswerDocLang(ctx))), "zh")
	b.WriteString("## Runtime Trace Principal Values — Final Typed Recap\n\n")
	b.WriteString("- Use these typed rows for the leading numeric conclusion. They are a compact recap of the same authority used by the deterministic answer lead; later blocked-reason records, IPC request counts, transport latency, capped exploration rows, per-CPU aggregate groups, or narrative estimates cannot replace their caliber.\n")
	b.WriteString("- `principal_state` is the selected-window authority for a target thread's running/runnable/sleep/D-state totals. A perf-triage `time_semantics` duration is the whole attachment's first-to-last timestamp extent; it is unit/provenance context only and must never replace a `principal_state` value or be emitted as the target's selected-window state total. If an earlier narrative or model-authored aggregate used the attachment extent for that purpose, keep the model's diagnosis but correct the numeric caliber from `principal_state`.\n")
	b.WriteString("- Within a `principal_state` row, `head_carry` or `tail_open` marked `already_included=true` is selected-window wall clock already contained in its named state and in `accounted_total`; never add it again, call it outside the selected window, or combine it with `unaccounted`. Only `unaccounted` is the separate uncovered remainder, and insufficient boundary evidence means its state is unknown. If an earlier model aggregate/completion note conflicts, use this final typed accounting while keeping the conclusion model-authored.\n")
	b.WriteString("- A complete target-wait row authorizes its exact occurrence count and wall-clock sum. A capacity-truncated blocking row authorizes only the displayed observed lower bound (`>=`); never turn it into an exact total, a unique/only occurrence, or a claim that every other request caused no blocking.\n\n")
	b.WriteString("- Complete target-wait rows below are compiled from the uncapped typed per-occurrence leaves. An earlier `target_wait_occurrence_prompt=status=incomplete,emitted=N,total=M` row describes only that compact prompt preview, not an incomplete engine roster. When this recap publishes `permission=exact_complete_rowset`, use its full count/sum/rows and never call the underlying roster incomplete or reconcile its preview-prefix sum with another measurement.\n\n")
	b.WriteString("- Target wait state kinds remain separate: `d_state_occurrences`, `io_wait_occurrences`, and `sleep_iowait_occurrences` are separately reported typed counts. Do not rename an `io_wait` row to D-state when the same authority reports `d_state_occurrences=0`.\n\n")
	if zh {
		b.WriteString("- 缺失边界：目标窗口内没有匹配的等待原因记录，只表示这份清单没有捕获到对应事件；不能据此把 S 状态判成主动休眠，不能证明 sleep/nanosleep 调用，也不能排除系统调用或外部依赖。只有独立的系统调用、业务片段、阻塞原因、唤醒或依赖关系证据才能命名具体等待机制。\n\n")
	} else {
		b.WriteString("- Absence boundary: no matching target-window wait-reason records means only that this roster captured no such occurrence. It does not classify an S interval as cooperative or voluntary sleep, prove a sleep/nanosleep syscall, rule out a syscall or dependency, or establish the wait mechanism. Name a mechanism only from separate syscall, span, blocked-reason, wakeup, or dependency evidence.\n\n")
	}
	b.WriteString("- `sched_blocked_reason.caller` is the kernel-reported wait call-site/symbol associated with that target interval. It can describe where the target blocked, but it is not a typed resource identity, lock owner, or holder thread. Only a separate typed holder/owner relation may authorize holder language.\n\n")
	b.WriteString("- Wakeup endpoint values are role-bound. Never swap a waker's priority/class/CPU with the wakee's values, and do not compare or strengthen the relation beyond its row-local caliber.\n\n")
	if zh {
		b.WriteString("- 不同 authority 行的数值差本身不是关系证据；除非另有 explicit typed relation 证明，不得把 record/occurrence/partition 的差值解释成窗口边界、重叠、精度误差或缺失闭合。\n\n")
	} else {
		b.WriteString("- A numeric delta between authority rows is not relation evidence. Unless a separate explicit typed relation proves it, do not explain record/occurrence/partition differences as window-boundary effects, overlap, precision drift, or missing closure.\n\n")
	}

	if stateRosterTruncated {
		fmt.Fprintf(&b,
			"- principal_state_roster_coverage: visible_accounts=%d; additional_artifact_partitions_omitted=%d; status=`capacity_truncated`; complete=false; omitted_state_accounts=`not_evaluated`\n",
			len(states),
			len(projectionSet.OmittedArtifactLabels),
		)
	}
	for _, state := range states {
		fmt.Fprintf(&b,
			"- principal_state: artifact=`%s`; target=`%s`; window=`%.6f..%.6f`; running=%.3fms; runnable=%.3fms; sleep=%.3fms; d_state=%.3fms; io_wait=%.3fms; accounted_total=%.3fms; window_ms=%.3fms; coverage_status=`%s`",
			state.ArtifactLabel,
			state.Subject,
			state.WindowStartTs,
			state.WindowEndTs,
			state.RunningMS,
			state.RunnableMS,
			state.SleepMS,
			state.DStateMS,
			state.IOWaitMS,
			state.TotalMS,
			state.WindowMS,
			state.CoverageStatus,
		)
		if state.UnaccountedMS > 0 {
			fmt.Fprintf(&b, "; unaccounted=%.3fms (typed boundary evidence is insufficient to assign this remainder to any state)", state.UnaccountedMS)
		}
		if state.HeadCarryMS > 0 && state.HeadCarryState != "" {
			fmt.Fprintf(&b, "; head_carry=%.3fms state=`%s` already_included=true", state.HeadCarryMS, state.HeadCarryState)
		}
		if state.TailOpenMS > 0 && state.TailOpenState != "" {
			fmt.Fprintf(&b, "; tail_open=%.3fms state=`%s` already_included=true", state.TailOpenMS, state.TailOpenState)
		}
		b.WriteByte('\n')
	}
	hasRequestedWaitPrincipal := false
	for _, wait := range waits {
		if wait.IsRequestedScopePrincipal() {
			hasRequestedWaitPrincipal = true
			break
		}
	}
	for _, wait := range waits {
		scopeRole := strings.TrimSpace(string(wait.RequestedScopeRole))
		if scopeRole == "" {
			scopeRole = "unclassified"
		}
		fmt.Fprintf(&b,
			"- principal_wait_occurrences: artifact=`%s`; target=`%s`; window=`%.6f..%.6f`; scope_role=`%s`; permission=`exact_complete_rowset`; occurrence_count=%d; d_state_occurrences=%d; io_wait_occurrences=%d; sleep_iowait_occurrences=%d; other_wait_occurrences=%d; wall_clock_sum=%.3fms",
			wait.ArtifactLabel,
			wait.Subject,
			wait.WindowStartTs,
			wait.WindowEndTs,
			scopeRole,
			wait.Count,
			wait.DStateOccurrences,
			wait.IOWaitOccurrences,
			wait.SleepIOWaitOccurrences,
			wait.OtherWaitOccurrences,
			wait.WallClockMS,
		)
		if len(wait.Callers) > 0 {
			fmt.Fprintf(&b, "; blocked_reason_callers=`%s`; caller_role=`kernel_reported_wait_callsite`; holder_authority=`not_provided_by_caller`", strings.Join(wait.Callers, "`, `"))
		}
		b.WriteString("; use this occurrence count rather than blocked_reason record count or aggregate-group count\n")
		if hasRequestedWaitPrincipal && !wait.IsRequestedScopePrincipal() {
			b.WriteString("  - scope_boundary=`supporting exploration window only; do not use this row's count, total, or occurrence roster as the answer for the requested artifact scope.`\n")
			continue
		}
		for _, occurrence := range wait.Occurrences {
			fmt.Fprintf(&b, "  - principal_occurrence=`%s`\n", occurrence.CanonicalLine())
		}
		if zh {
			fmt.Fprintf(&b,
				"  - principal_conclusion_zh=`%s 在 %.6f..%.6f 窗内确切发生 %d 次目标等待，目标等待墙钟合计 %.3fms",
				wait.Subject,
				wait.WindowStartTs,
				wait.WindowEndTs,
				wait.Count,
				wait.WallClockMS,
			)
			if len(wait.Callers) > 0 {
				fmt.Fprintf(&b, "，内核报告的等待调用点/符号为 %s（不据此推断资源持有者）", strings.Join(wait.Callers, "、"))
			}
			b.WriteString("。`\n")
		} else {
			fmt.Fprintf(&b,
				"  - principal_conclusion_en=`In %.6f..%.6f, %s has exactly %d target-wait occurrence(s), totaling %.3fms of target-wait wall clock",
				wait.WindowStartTs,
				wait.WindowEndTs,
				wait.Subject,
				wait.Count,
				wait.WallClockMS,
			)
			if len(wait.Callers) > 0 {
				fmt.Fprintf(&b, ", with kernel-reported wait call-site/symbol(s) %s (not holder identity)", strings.Join(wait.Callers, ", "))
			}
			b.WriteString(".`\n")
		}
	}
	for _, block := range blocking {
		permission := "bounded_observation"
		occurrenceToken := fmt.Sprintf("%d", len(block.Occurrences))
		wallClockToken := fmt.Sprintf("%.3fms", block.ObservedMS)
		if block.CoverageStatus == "complete" {
			permission = "exact_complete"
		} else if block.CoverageStatus == "lower_bound_capacity_truncated" {
			permission = "lower_bound_only"
			occurrenceToken = fmt.Sprintf(">=%d", len(block.Occurrences))
			wallClockToken = fmt.Sprintf(">=%.3fms", block.ObservedMS)
		}
		fmt.Fprintf(&b,
			"- principal_blocking: artifact=`%s`; target=`%s`; selected_window=`%s`; blocking_type=`%s`; permission=`%s`; observed_occurrences=%s; observed_wall_clock=%s; coverage_status=`%s`\n",
			block.ArtifactLabel,
			block.Subject,
			block.SelectedWindow,
			block.Type,
			permission,
			occurrenceToken,
			wallClockToken,
			block.CoverageStatus,
		)
		switch {
		case block.CoverageStatus == "complete" && zh:
			fmt.Fprintf(&b,
				"  - principal_conclusion_zh=`关于 %s，%s 在窗 %s 内确切发生 %d 次，目标阻塞墙钟合计 %.3fms。`\n",
				block.Type, block.Subject, block.SelectedWindow, len(block.Occurrences), block.ObservedMS)
		case block.CoverageStatus == "complete":
			fmt.Fprintf(&b,
				"  - principal_conclusion_en=`For %s, %s has exactly %d occurrence(s) in %s, totaling %.3fms of target blocking wall clock.`\n",
				block.Type, block.Subject, len(block.Occurrences), block.SelectedWindow, block.ObservedMS)
		case block.CoverageStatus == "lower_bound_capacity_truncated" && zh:
			fmt.Fprintf(&b,
				"  - principal_conclusion_zh=`关于 %s，当前只确认 %s 在窗 %s 内至少 %d 次、至少 %.3fms；由于覆盖被截断，全窗总次数和总量未知，不能表述为只有、唯一或总计，也不能断言其他请求没有阻塞。`\n",
				block.Type, block.Subject, block.SelectedWindow, len(block.Occurrences), block.ObservedMS)
		case block.CoverageStatus == "lower_bound_capacity_truncated":
			fmt.Fprintf(&b,
				"  - principal_conclusion_en=`For %s, the current evidence confirms at least %d occurrence(s) and at least %.3fms for %s in %s. Coverage is truncated, so the full-window count and total are unknown; do not say only/unique/total or claim that every other request caused no blocking.`\n",
				block.Type, len(block.Occurrences), block.ObservedMS, block.Subject, block.SelectedWindow)
		}
	}
	const wakeupEdgeLimit = 8
	wakeupEdgeCount := len(wakeupEdges)
	if wakeupEdgeCount > wakeupEdgeLimit {
		wakeupEdgeCount = wakeupEdgeLimit
	}
	for _, edge := range wakeupEdges[:wakeupEdgeCount] {
		fmt.Fprintf(&b,
			"- principal_wakeup_edge_roles: artifact=`%s`; scope=`%s`; waker=`%s`; wakee=`%s`",
			edge.ArtifactLabel, edge.Scope, edge.Waker, edge.Wakee,
		)
		if edge.WakeupTimestamp != "" {
			fmt.Fprintf(&b, "; wakeup_ts=%s", edge.WakeupTimestamp)
		}
		if edge.WakerPriority != "" {
			fmt.Fprintf(&b, "; waker_priority=`%s`", edge.WakerPriority)
			if edge.WakerPrioritySource != "" {
				fmt.Fprintf(&b, "; waker_priority_source=`%s`", edge.WakerPrioritySource)
			}
		}
		if edge.WakeePriority != "" {
			fmt.Fprintf(&b, "; wakee_priority=`%s`", edge.WakeePriority)
			if edge.WakeePrioritySource != "" {
				fmt.Fprintf(&b, "; wakee_priority_source=`%s`", edge.WakeePrioritySource)
			}
			if edge.WakeePriorityAuthority != "" {
				fmt.Fprintf(&b, "; wakee_priority_authority=`%s`", edge.WakeePriorityAuthority)
			}
		}
		if edge.WakerCPU != "" {
			fmt.Fprintf(&b, "; waker_cpu=%s", edge.WakerCPU)
		}
		if edge.WakeeTargetCPU != "" {
			fmt.Fprintf(&b, "; wakee_target_cpu=%s", edge.WakeeTargetCPU)
		}
		if edge.CPURelation != "" {
			fmt.Fprintf(&b, "; cpu_relation=`%s`", edge.CPURelation)
		}
		b.WriteString("; role_binding=`exact_row_local`; conclusion_owner=`model`\n")
	}
	if len(wakeupEdges) > wakeupEdgeCount {
		fmt.Fprintf(&b, "- principal_wakeup_edge_role_coverage: emitted=%d; total=%d; complete=false; omitted_rows=`not_evaluated_in_this_compact_recap`\n", wakeupEdgeCount, len(wakeupEdges))
	}
	b.WriteByte('\n')
	return b.String()
}
