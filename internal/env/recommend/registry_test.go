package recommend

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRecommend_Python_RunnerMissing_VenvFirst locks the priority:
// venv-install ranks above user-install ranks above all global
// strategies.
func TestRecommend_Python_RunnerMissing_VenvFirst(t *testing.T) {
	env := &types.EnvFacts{
		OSFamily: "debian",
		Pythons:  []types.PythonInterp{{Path: "/usr/bin/python3", Version: "3.11"}},
		PkgManagers: map[string]types.PkgManagerInfo{
			"pip": {Path: "/usr/bin/pip"}, "pip3": {Path: "/usr/bin/pip3"},
		},
		ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
	}
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	settings := types.DefaultEnvRecommendSettings()
	recs := Recommend(d, env, settings)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	if recs[0].Strategy != types.StrategyVenvInstall {
		t.Errorf("top-1 should be VenvInstall; got %v", recs[0].Strategy)
	}
	if !strings.Contains(recs[0].Command, "python3 -m venv") {
		t.Errorf("expected venv command in top-1; got %q", recs[0].Command)
	}
}

// TestRecommend_Node_LockfileSelectsManager covers the
// lockfile-driven selection: pnpm-lock.yaml present → pnpm
// install --frozen-lockfile preferred.
func TestRecommend_Node_LockfileSelectsManager(t *testing.T) {
	env := &types.EnvFacts{
		OSFamily: "darwin",
		Nodes:    []types.NodeInterp{{Path: "/usr/bin/node", PkgManagers: []string{"pnpm"}}},
		PkgManagers: map[string]types.PkgManagerInfo{
			"pnpm": {Path: "/usr/bin/pnpm"},
		},
		ProjectFiles: map[string]string{
			"package.json":    "/abs",
			"pnpm-lock.yaml": "/abs/pnpm-lock.yaml",
		},
	}
	d := types.Diagnosis{Kind: types.DiagDepsMissing, Runner: "node", Tool: "node_modules"}
	recs := Recommend(d, env, types.DefaultEnvRecommendSettings())
	if len(recs) == 0 {
		t.Fatal("expected at least one rec")
	}
	if !strings.Contains(recs[0].Command, "pnpm install --frozen-lockfile") {
		t.Errorf("top-1 should prefer pnpm; got %q", recs[0].Command)
	}
}

// TestRecommend_GlobalInstallFiltered verifies the master-switch
// filter: when RecommendGlobalInstall=false, all GlobalInstall
// candidates are stripped.
func TestRecommend_GlobalInstallFiltered(t *testing.T) {
	env := &types.EnvFacts{OSFamily: "debian"}
	d := types.Diagnosis{Kind: types.DiagBuildToolchainMissing, Runner: "ruby", Tool: "ruby"}
	settings := types.DefaultEnvRecommendSettings() // GlobalInstall=false default
	recs := Recommend(d, env, settings)
	for _, r := range recs {
		if r.Strategy == types.StrategyGlobalInstall {
			t.Errorf("GlobalInstall should be filtered out; got %+v", r)
		}
	}
	// With opt-in, global re-appears.
	settings.RecommendGlobalInstall = true
	recs = Recommend(d, env, settings)
	hasGlobal := false
	for _, r := range recs {
		if r.Strategy == types.StrategyGlobalInstall {
			hasGlobal = true
		}
	}
	if !hasGlobal {
		t.Errorf("expected at least one GlobalInstall when opted in; got %+v", recs)
	}
}

// TestRecommend_GitNotInitialized covers the git-state path:
// runner-agnostic, the systemRecommender produces the candidates.
func TestRecommend_GitNotInitialized(t *testing.T) {
	env := &types.EnvFacts{}
	d := types.Diagnosis{Kind: types.DiagGitRepoNotInitialized, Tool: "git"}
	recs := Recommend(d, env, types.DefaultEnvRecommendSettings())
	if len(recs) == 0 {
		t.Fatal("expected git init recommendations")
	}
	found := false
	for _, r := range recs {
		if strings.Contains(r.Command, "git init") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `git init` recommendation; got %+v", recs)
	}
}

// TestRecommend_DisabledMasterSwitch verifies env_recommend_enabled=false
// returns no recommendations regardless of diagnosis.
func TestRecommend_DisabledMasterSwitch(t *testing.T) {
	env := &types.EnvFacts{
		Pythons: []types.PythonInterp{{Path: "/usr/bin/python3"}},
	}
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	settings := types.DefaultEnvRecommendSettings()
	settings.Enabled = false
	recs := Recommend(d, env, settings)
	if len(recs) > 0 {
		t.Errorf("disabled master switch should produce no recs; got %d", len(recs))
	}
}

// TestRecommend_ResultCappedAt3 verifies the 3-recommendation cap.
func TestRecommend_ResultCappedAt3(t *testing.T) {
	env := &types.EnvFacts{
		OSFamily: "darwin",
		Pythons:  []types.PythonInterp{{Path: "/usr/bin/python3"}},
		PkgManagers: map[string]types.PkgManagerInfo{
			"pip":  {Path: "/usr/bin/pip"},
			"pip3": {Path: "/usr/bin/pip3"},
			"brew": {Path: "/usr/local/bin/brew"},
		},
		ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
	}
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	recs := Recommend(d, env, types.DefaultEnvRecommendSettings())
	if len(recs) > 3 {
		t.Errorf("results should cap at 3; got %d", len(recs))
	}
}
