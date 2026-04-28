package env

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// integration_test.go exercises the full env_recommend pipeline
// end-to-end: synthesised stderr → Diagnose → Recommend → Render.
// LLM is left disabled so the tests are hermetic; the assertion
// floor is "every supported runner produces non-empty output and
// names its docs URL". This is the commercial-usability invariant.

// runnerFixture is the per-runner test case: a representative
// command-not-found stderr line + the expected DiagKind +
// expected substring in the rendered output.
type runnerFixture struct {
	runner   string
	stderr   string
	wantKind types.DiagKind
	wantInOutput []string // bare substrings expected in rendered text
}

// commercial-fixtures cover the realistic stderr the operator's
// terminal emits when each runner is missing. The detector chain
// (system_lib > git > runner_missing > deps > toolchain > config)
// matches every entry to runner_missing — that's the most common
// "I just cloned the repo and tests fail" entry point.
var runnerFixtures = []runnerFixture{
	{
		runner:       "python",
		stderr:       "/bin/sh: pytest: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"python.org"},
	},
	{
		runner:       "node",
		stderr:       "bash: npm: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"nodejs.org"},
	},
	{
		runner:       "rust",
		stderr:       "/bin/sh: cargo: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"rust-lang.org"},
	},
	{
		runner:       "go",
		stderr:       "/bin/sh: go: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"go.dev"},
	},
	{
		runner:       "java",
		stderr:       "/bin/sh: mvn: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"adoptium.net"},
	},
	{
		runner:       "ruby",
		stderr:       "/bin/sh: bundle: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"ruby-lang.org"},
	},
	{
		runner:       "swift",
		stderr:       "/bin/sh: swift: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"swift.org"},
	},
	{
		runner:       "cmake",
		stderr:       "/bin/sh: ctest: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"cmake.org"},
	},
	{
		runner:       "meson",
		stderr:       "/bin/sh: meson: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"mesonbuild.com"},
	},
	{
		runner:       "make",
		stderr:       "/bin/sh: make: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"gnu.org/software/make"},
	},
	{
		runner:       "hvigor",
		stderr:       "/bin/sh: hvigorw: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"developer.harmonyos.com"},
	},
	{
		runner:       "cjpm",
		stderr:       "/bin/sh: cjpm: command not found\n",
		wantKind:     types.DiagRunnerMissing,
		wantInOutput: []string{"cangjie-lang.cn"},
	},
}

// TestIntegration_AllRunnersProduceRecommendation locks the
// commercial guarantee: for every supported runner, the
// "command-not-found" path produces a non-empty rendered block
// containing the runner's docs URL — even with LLM disabled.
// If a future change drops a runner from the docs map, this
// test fails per-subtest naming the dropped runner.
func TestIntegration_AllRunnersProduceRecommendation(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false // hermetic — no network

	envFacts := &types.EnvFacts{OSFamily: "debian", OS: "linux"}

	for _, f := range runnerFixtures {
		t.Run(f.runner, func(t *testing.T) {
			d := Diagnose(f.stderr, f.runner, envFacts)
			if d.Kind != f.wantKind {
				t.Errorf("Diagnose kind: want %s, got %s", f.wantKind, d.Kind)
			}
			recs := Recommend(d, envFacts, settings)
			if len(recs) == 0 {
				t.Fatalf("Recommend produced empty result for runner=%s — DocsLink fallback should always fire", f.runner)
			}
			out := RenderRecommendations(d, recs, "en")
			if strings.TrimSpace(out) == "" {
				t.Fatalf("RenderRecommendations produced empty text for runner=%s", f.runner)
			}
			for _, want := range f.wantInOutput {
				if !strings.Contains(out, want) {
					t.Errorf("rendered output missing expected substring %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}

// TestIntegration_RenderBilingual verifies the language switch
// works on the integrated output (en vs zh).
func TestIntegration_RenderBilingual(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false
	envFacts := &types.EnvFacts{OSFamily: "debian"}
	d := Diagnose("/bin/sh: pytest: not found\n", "python", envFacts)
	recs := Recommend(d, envFacts, settings)

	en := RenderRecommendations(d, recs, "en")
	zh := RenderRecommendations(d, recs, "zh")

	if !strings.Contains(en, "Environment diagnosis") {
		t.Errorf("EN output missing English header; got %q", en)
	}
	if !strings.Contains(zh, "环境诊断") {
		t.Errorf("ZH output missing Chinese header; got %q", zh)
	}
	if en == zh {
		t.Errorf("EN and ZH outputs should differ")
	}
}

// TestIntegration_GitNotInitializedHappyPath verifies the
// system-stable Stage 1 path end-to-end. No LLM, no docs URL —
// purely the deterministic git-init recommendation.
func TestIntegration_GitNotInitializedHappyPath(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false
	envFacts := &types.EnvFacts{
		OSFamily:     "debian",
		GitRepoState: "not_initialized",
	}
	d := Diagnose("", "", envFacts)
	if d.Kind != types.DiagGitRepoNotInitialized {
		t.Fatalf("expected DiagGitRepoNotInitialized; got %s", d.Kind)
	}
	recs := Recommend(d, envFacts, settings)
	out := RenderRecommendations(d, recs, "en")
	if !strings.Contains(out, "git init") {
		t.Errorf("rendered output should mention `git init`; got %q", out)
	}
}

// TestIntegration_DisabledMasterSwitchByteIdentity is the R5 floor
// at the integration level. Master switch off → Recommend yields
// nil → Renderer's "no actionable command" footer surfaces.
func TestIntegration_DisabledMasterSwitchByteIdentity(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.Enabled = false

	envFacts := &types.EnvFacts{OSFamily: "debian"}
	d := Diagnose("/bin/sh: pytest: not found\n", "python", envFacts)
	recs := Recommend(d, envFacts, settings)
	if len(recs) != 0 {
		t.Errorf("R5 violation: disabled master switch must produce 0 recommendations; got %d", len(recs))
	}
	out := RenderRecommendations(d, recs, "en")
	if !strings.Contains(out, "No actionable install command available") {
		t.Errorf("Renderer should surface generic 'no recommendation' on empty recs; got %q", out)
	}
}

// TestIntegration_NetworkUnreachableDemotion verifies the
// network-aware demotion fires end-to-end. With Network probed
// and unreachable, recommendations carrying NeedsNetwork=true
// get +100 priority and an extra Caveat.
func TestIntegration_NetworkUnreachableDemotion(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.RecommendGlobalInstall = true
	settings.LLMEnabled = false
	envFacts := &types.EnvFacts{
		OSFamily:     "debian",
		GitRepoState: "git_missing",
		Network: types.NetworkStatus{
			Reachable: false,
			ProbedAt:  mustParseTime("2026-04-28T12:00:00Z"),
		},
	}
	d := Diagnose("", "", envFacts)
	recs := Recommend(d, envFacts, settings)
	if len(recs) == 0 {
		t.Fatal("expected git install recommendations")
	}
	// At least one rec should carry the unreachable caveat.
	found := false
	for _, r := range recs {
		for _, c := range r.Caveats {
			if strings.Contains(c, "network appears unreachable") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected `network appears unreachable` caveat on any NeedsNetwork rec; got %+v", recs)
	}
}
