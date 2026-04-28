package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEnvRecommendOutput_DisabledMasterSwitch is the R6 byte-
// identical fallback drift guard. When env_recommend_enabled is
// false, the integration seam MUST return empty string so
// run_tests falls through to the legacy hardcoded
// runnerInstallHint. Future refactors that accidentally couple
// env_recommend output to the disabled path will break this test.
func TestEnvRecommendOutput_DisabledMasterSwitch(t *testing.T) {
	ctx := &types.BusContext{
		EnvRecommendSettings: types.EnvRecommendSettings{Enabled: false},
		EnvFacts:             &types.EnvFacts{OS: "linux"},
		Language:             "zh",
	}
	out := envRecommendOutput(ctx, "python", "/bin/sh: pytest: not found\n")
	if out != "" {
		t.Errorf("R6 violation: env_recommend_enabled=false must return empty so legacy runnerInstallHint is used; got %q", out)
	}
}

// TestEnvRecommendOutput_NilFactsFallsBack covers the partial
// init path: ctx exists but EnvFacts is nil (pre-probe Run).
// Should return empty so legacy hint surfaces.
func TestEnvRecommendOutput_NilFactsFallsBack(t *testing.T) {
	ctx := &types.BusContext{
		EnvRecommendSettings: types.EnvRecommendSettings{Enabled: true},
		EnvFacts:             nil,
		Language:             "zh",
	}
	out := envRecommendOutput(ctx, "python", "stderr")
	if out != "" {
		t.Errorf("nil EnvFacts must return empty (legacy fallback); got %q", out)
	}
}

// TestUpdateFailureSummaryWithHint_PreservesPrefixOnHardcodedHint
// covers the legacy retention path: when env_recommend output is
// empty, the existing makeRunnerMissingReport prose passes
// through unchanged.
func TestUpdateFailureSummaryWithHint_PreservesPrefixOnHardcodedHint(t *testing.T) {
	original := `runner "python" 的主二进制 "pytest" 未在本环境安装(子进程 exit=1, 触发信号=pattern: No module named pytest) —— 在项目 venv 里装(`
	got := updateFailureSummaryWithHint(original, "")
	if got != original {
		t.Errorf("empty hint must pass through unchanged; got %q", got)
	}
}

// TestUpdateFailureSummaryWithHint_ReplacesLegacyMarker covers the
// substitution path: env_recommend produced a richer block,
// caller passes it as `hint`, the helper finds the legacy hint
// marker and replaces from there.
func TestUpdateFailureSummaryWithHint_ReplacesLegacyMarker(t *testing.T) {
	legacy := "pytest 未在本环境安装。请先安装;在项目 venv 里装 `pip install pytest`"
	envRec := "✗ 环境诊断: pytest 缺失\n推荐: !pip install pytest"
	got := updateFailureSummaryWithHint(legacy, envRec)
	if !strings.Contains(got, envRec) {
		t.Errorf("env_recommend block must be present; got %q", got)
	}
	if strings.Contains(got, "请先安装;") {
		t.Errorf("legacy marker should be cut after replacement; got %q", got)
	}
}

// TestUpdateFailureSummaryWithHint_Idempotent covers double-call
// safety: calling with the same hint twice doesn't accumulate.
func TestUpdateFailureSummaryWithHint_Idempotent(t *testing.T) {
	hint := "✗ 环境诊断: X"
	first := updateFailureSummaryWithHint("legacy summary", hint)
	second := updateFailureSummaryWithHint(first, hint)
	if first != second {
		t.Errorf("idempotency violated:\n first:  %q\n second: %q", first, second)
	}
}
