package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderRuntimeTraceCPUIdentityGuide is shared soft teaching for every LLM
// stage that reads a typed trace attachment. It prevents ftrace task identity
// fields from being mistaken for CPU identity without inspecting request or
// model prose and without taking ownership of the answer conclusion.
func renderRuntimeTraceCPUIdentityGuide(ctx *types.AgentContext) string {
	if !agentContextHasRuntimeTraceCPUIdentityCarrier(ctx) {
		return ""
	}
	return "## Runtime Trace CPU Identity Syntax\n\n" +
		"- In an ftrace/systrace row such as `comm-tid (tgid) [CPU] ...`, `tid` and the parenthesized `tgid` are thread/process identity fields; namespace/host PID variants are identities too. None of those values is a CPU number.\n" +
		"- The bracketed `[NNN]` field is the event-row CPU. For a perf sample, an explicit `cpu=N` is sample CPU authority only when the typed row also says `cpu_known=true`.\n" +
		"- Do not infer a CPU migration by comparing PID/TID/TGID/namespace identity with a CPU value. A migration needs a typed migration event or compatible target rows on multiple CPUs. Keep the interpretation and final conclusion model-authored.\n\n"
}

func prependRuntimeTraceCPUIdentityGuide(ctx *types.AgentContext, base string) string {
	guide := strings.TrimRight(renderRuntimeTraceCPUIdentityGuide(ctx), "\n")
	if guide == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return guide
	}
	return guide + "\n\n" + base
}

func agentContextHasRuntimeTraceCPUIdentityCarrier(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" || ctx.PerfTrace != nil {
		return true
	}
	return ctx.Mutable != nil && ctx.Mutable.PerfTrace() != nil
}
