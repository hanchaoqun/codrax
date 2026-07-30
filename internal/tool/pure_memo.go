package tool

// pure_memo.go — run-scoped pure-tool memo (EVALFIX-2B 类2, 2026-07-30).
//
// A tool call is "pure" when its output is a deterministic function of
// (artifact byte content, normalized effective params). Repeating such a
// call inside one task re-buys the full computation (for trace_query: a
// complete index rebuild + view run + blob store) for a result that is
// byte-identical by the DET-1 determinism pin. RunPureToolMemo lets a
// tool wrap exactly its honest pure core: a miss executes for real and
// stores the Success result on MutableState; a hit returns a verbatim
// copy of the stored result — same RawRef / payload refs / Observations /
// typed authority fields — so the observation ledger's existing
// SourceRef-keyed dedupe collapses duplicate observations with zero new
// dedup code.
//
// Design boundaries (ledger: docs/design/evalfix2_class_designs_20260730.md 类2):
//   - The purity boundary is DECLARED BY THE TOOL, never by the Registry:
//     only the tool knows which of its side-effect registries must keep
//     firing per call (trace_query's cursor + call-window recorders run
//     BEFORE its memo hook, so supplement window election sees every call).
//   - Only Success=true results are stored (retry semantics re-execute).
//   - The memo never refuses new work: any param variation is a new key
//     and a real execution. A hit returns the true answer, not a refusal.
//   - §29.21 proof lane: a hit re-publishes the SAME deterministic tool
//     witness (same refs, same producer rows); no model assertion is ever
//     minted into an observation by this path.
//   - Lifecycle is per-task (ResetTurnAArtifacts), forked/merged with the
//     same semantics as traceQueryPublishedBlobRefs.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// pureToolMemoEnabled is the operator kill switch. Default true; set
// codrax.yaml :: pipeline_pure_tool_memo_enabled: false to restore the
// always-recompute behavior. Same master-switch shape as lintEnabled.
var pureToolMemoEnabled = true

// PureToolMemoEnabled reports whether the run-scoped pure-tool memo is
// active. Producers (traceQueryMemoKey) consult this before minting a key
// so a disabled memo is byte-identical to a build without the feature.
func PureToolMemoEnabled() bool { return pureToolMemoEnabled }

// SetPureToolMemoEnabled installs the operator's yaml override. Called
// once from cmd/root.go's settings-resolution block alongside the other
// pipeline knobs.
func SetPureToolMemoEnabled(b bool) { pureToolMemoEnabled = b }

// PureToolMemoReuseDisclosure is the Summary-tail line appended to a memo
// hit (model-facing soft guidance only — no hard gate reads it). It keeps
// the [note: …] tool-result appendix convention.
func PureToolMemoReuseDisclosure(toolName string) string {
	return "[note: identical " + toolName + " call (same artifact + same effective params) already executed in this task; result reused verbatim — vary parameters to gather new information instead of re-issuing the same query]"
}

// RunPureToolMemo wraps a tool's declared pure core. On a miss it runs
// compute and stores a Success result; on a hit it returns a verbatim
// copy of the stored result decorated with exactly two things: the
// disclosure line appended to Summary and ReusedFromRunMemo=true. All
// refs, Observations (including their first-run ObservedAt — that is
// when the computation actually happened; the disclosure says so), and
// typed authority fields are preserved byte-for-byte.
func RunPureToolMemo(ctx *types.BusContext, toolName, memoKey string,
	compute func() (types.ToolResult, error)) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return compute()
	}
	if cached, ok := ctx.Mutable.ToolResultMemo(toolName, memoKey); ok {
		out := cached
		out.ReusedFromRunMemo = true
		if strings.TrimSpace(out.Summary) == "" {
			out.Summary = PureToolMemoReuseDisclosure(toolName)
		} else {
			out.Summary = out.Summary + "\n" + PureToolMemoReuseDisclosure(toolName)
		}
		logging.Info("[pure_memo] %s: identical call served from run memo (verbatim refs; no recomputation)", toolName)
		return out, nil
	}
	result, err := compute()
	if err == nil && result.Success {
		ctx.Mutable.StoreToolResultMemo(toolName, memoKey, result)
	}
	return result, err
}
