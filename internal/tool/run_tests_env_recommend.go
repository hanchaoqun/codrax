package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/types"
)

// envRecommendOutput is the integration seam between run_tests and
// the env_recommend pipeline. Returns the rendered recommendation
// block, or empty string when env_recommend is disabled / unwired
// — caller falls through to the legacy runnerInstallHint string so
// behaviour is byte-identical with the master switch off.
//
// The integration is intentionally narrow: only the runner_missing
// branch in run_tests calls this. Other diagnoses (build_failure /
// timeout / OOM) keep their existing rendering. Future commits can
// extend the seam to cover more failure shapes.
func envRecommendOutput(ctx *types.BusContext, runner, stderr string) string {
	if ctx == nil || !ctx.EnvRecommendSettings.Enabled || ctx.EnvFacts == nil {
		return ""
	}
	d := env.Diagnose(stderr, runner, ctx.EnvFacts)
	if d.Kind == "" {
		return ""
	}
	recs := env.Recommend(d, ctx.EnvFacts, ctx.EnvRecommendSettings)
	out := env.RenderRecommendations(d, recs, ctx.Language)
	return strings.TrimSpace(out)
}

// updateFailureSummaryWithHint takes the existing FailureSummary
// (which usually contains a hardcoded install hint suffix from
// runnerInstallHint) and replaces the trailing hint with the
// env_recommend rendered block. When the FailureSummary is empty
// or doesn't contain a recognisable hint marker, prepends the
// hint with a separator. Idempotent: calling twice with the same
// hint is a no-op.
func updateFailureSummaryWithHint(summary, hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return summary
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return hint
	}
	// If the existing summary already contains the hint verbatim,
	// no-op. This is the idempotent path.
	if strings.Contains(summary, hint) {
		return summary
	}
	// Look for the legacy hint markers (zh: "请先安装"; en:
	// "install it before re-running") and replace from there.
	for _, marker := range []string{
		"请先安装;", "Install the runner's CLI binary",
		"install it before re-running",
	} {
		if i := strings.Index(summary, marker); i >= 0 {
			return strings.TrimSpace(summary[:i]) + "\n\n" + hint
		}
	}
	// No marker found — append.
	return summary + "\n\n" + hint
}
