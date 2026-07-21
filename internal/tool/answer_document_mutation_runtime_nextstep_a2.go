package tool

// answer_document_mutation_runtime_nextstep_a2.go — A2 件1 (§29.174 UX-13,
// 2026-07-21): the 「## 下一步」 direction-action lane.
//
// EVOLUTION RECORD: the per-record template lane
// (runtimeTraceNextStepFromObservationRecord + the fixed next_step_kind → ZH
// sentence table, the former runtime.go:7088 domain) is RETIRED — its rows
// were subject-less, value-less boilerplate (「排查相邻的调度与资源事件」/
// 「排查反复唤醒它的对端线程、binder等待、锁与条件变量等待」/「排查所依赖的
// 低优先级线程的调度延迟,以及同窗口内的 CPU 压力」, runnable_2:503-505) that
// repeated verbatim on every trace report. The list now synthesizes ONE
// concrete action per PUBLISHED ◎ fix-direction section: the section's
// direction word (tracefence.FixDirectionWord, single word source), the
// section's top-seat subject and the section head's 最大可消 value verbatim
// (%.3f — the same bytes the ◎ head prints; the selection reads the SAME
// runtimeTraceProjElimChainRosterFor authority as the fence, 单一值源).
// Directions without a published section emit nothing (无席方向不发); the
// action verbs are a fixed closed set per direction (词面单点 zh/EN). The
// engine-side next_step/next_step_kind wire notes stay published (wire
// compatibility); no display surface consumes them any more.

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceNextStepDirectionAction is one synthesized action row: the
// typed direction token plus the composed display text (typed key for the
// dedupe lane; the text is the display face).
type runtimeTraceNextStepDirectionAction struct {
	direction string
	subject   string
	value     string
	text      string
}

// runtimeTraceNextStepDirectionActions synthesizes the direction-action rows
// for every ACTIVE compiled projection of this ledger, in projection-set
// order; within one projection the rows follow the published ◎ section order
// (其他方向恒末,余按节内最大可消降序 — the unresolved tail section carries no
// direction and emits nothing).
func runtimeTraceNextStepDirectionActions(ctx *types.BusContext, zh bool) []runtimeTraceNextStepDirectionAction {
	if ctx == nil {
		return nil
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	focus := runtimeTraceProjUserFocusFromBusContext(ctx)
	var out []runtimeTraceNextStepDirectionAction
	for _, projection := range set.Projections {
		if !projection.Active() || !projection.RootCauseFamilyObserved {
			continue
		}
		evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
		evidence.flatChain = len(runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)) < 2
		model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
		runtimeTraceProjApplyUserFocus(&model, focus)
		roster := runtimeTraceProjElimChainRosterFor(model)
		for _, section := range runtimeTraceProjElimSectionsFor(roster.renderedChain) {
			if action, ok := runtimeTraceNextStepDirectionActionFor(section, zh); ok {
				out = append(out, action)
			}
		}
	}
	return out
}

// runtimeTraceNextStepDirectionActionFor composes one section's action row.
// ok=false on the unresolved tail section (no direction word — 不猜方向) and
// on a section whose top seat cannot be located (defensive; the section
// builder always seats ≥1 entry).
func runtimeTraceNextStepDirectionActionFor(section runtimeTraceProjElimSection, zh bool) (runtimeTraceNextStepDirectionAction, bool) {
	direction := strings.TrimSpace(section.direction)
	word, resolved := tracefence.FixDirectionWord(direction, zh)
	if !resolved || len(section.entries) == 0 {
		return runtimeTraceNextStepDirectionAction{}, false
	}
	top := section.entries[0]
	for _, entry := range section.entries[1:] {
		if entry.row.Node.EffectiveImpactMS > top.row.Node.EffectiveImpactMS {
			top = entry
		}
	}
	subject := strings.TrimSpace(runtimeTraceProjElimSubject(top.row, zh))
	if subject == "" {
		return runtimeTraceNextStepDirectionAction{}, false
	}
	value := fmt.Sprintf("%.3fms", section.maxEff)
	var body string
	switch direction {
	case "scheduling_supply":
		if zh {
			body = fmt.Sprintf("排查 %s 的就绪等待:同核竞争线程与 CPU 亲和性(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("investigate %s's ready waits: same-CPU contention and affinity (%s eliminable)", subject, value)
		}
	case "lock_priority":
		if zh {
			body = fmt.Sprintf("评估提升 %s 调度优先级或减少其唤醒往返依赖(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("consider raising %s's scheduling priority or cutting its wakeup round-trips (%s eliminable)", subject, value)
		}
	case "io_dependency":
		if zh {
			body = fmt.Sprintf("排查 %s 的 IO、内核不可中断与依赖等待来源(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("investigate %s's IO, kernel-uninterruptible and dependency wait sources (%s eliminable)", subject, value)
		}
	case "memory":
		if zh {
			body = fmt.Sprintf("排查 %s 的内存回收与缺页压力(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("investigate %s's memory-reclaim and page-fault pressure (%s eliminable)", subject, value)
		}
	case "frequency_thermal":
		if zh {
			body = fmt.Sprintf("评估解除 %s 的运行频点限制(升频/迁核)(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("consider lifting %s's running-frequency limits (boost / migrate) (%s eliminable)", subject, value)
		}
	case "self_workload":
		verb := fmt.Sprintf("评估削减 %s 的确定性工作(%s 可消)", subject, value)
		if !zh {
			verb = fmt.Sprintf("consider reducing %s's deterministic work (%s eliminable)", subject, value)
		}
		body = verb
	default:
		// A token inside FixDirectionWord's closed set but without an action
		// template would silently vanish — fail open with the generic
		// investigate form so a table extension is never a silent drop.
		if zh {
			body = fmt.Sprintf("排查 %s 的该方向等待来源(%s 可消)", subject, value)
		} else {
			body = fmt.Sprintf("investigate %s's wait sources on this direction (%s eliminable)", subject, value)
		}
	}
	text := word + "→" + body
	if !zh {
		text = word + " → " + body
	}
	return runtimeTraceNextStepDirectionAction{
		direction: direction,
		subject:   subject,
		value:     value,
		text:      text,
	}, true
}
