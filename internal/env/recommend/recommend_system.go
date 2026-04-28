package recommend

import "github.com/hanchaoqun/codrax/internal/types"

// systemRecommender (registered under name "") handles the
// runner-agnostic diagnoses: git state, system libs, etc. It runs
// on every Recommend call alongside the per-runner recommender.
func init() { Register("", &systemRecommender{}) }

type systemRecommender struct{}

func (s *systemRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagGitNotInstalled:
		return s.gitNotInstalled(d, env)
	case types.DiagGitRepoNotInitialized:
		return s.gitNotInitialized(d, env)
	case types.DiagGitRepoNoCommits:
		return s.gitNoCommits(d, env)
	case types.DiagSystemLibMissing:
		return s.systemLib(d, env)
	}
	return nil
}

func (s *systemRecommender) gitNotInstalled(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	if env == nil {
		return nil
	}
	var out []types.Recommendation
	switch env.OSFamily {
	case "debian":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo apt install -y git",
			Why:      "Debian/Ubuntu", NeedsNetwork: true,
			NeedsSudo: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		})
	case "rhel":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo dnf install -y git",
			Why:      "RHEL/Fedora", NeedsNetwork: true,
			NeedsSudo: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		})
	case "darwin":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyDocsLink,
			Command:  "!xcode-select --install",
			Why:      "macOS Command Line Tools 含 git",
			Priority: 1, DiagRef: d,
		})
	case "windows":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyDocsLink,
			Command:  "see https://git-scm.com/download/win",
			Why:      "Git for Windows 官方安装器",
			Priority: 1, DiagRef: d,
		})
	}
	return out
}

func (s *systemRecommender) gitNotInitialized(d types.Diagnosis, _ *types.EnvFacts) []types.Recommendation {
	return []types.Recommendation{
		{
			Strategy: types.StrategyProjectInstall,
			Command:  "!git init && git add -A && git commit -m \"initial commit\"",
			Why:      "在当前仓初始化 git;codrax 写模式需要 git 仓",
			EstimatedSec: 2, Priority: 1, DiagRef: d,
		},
		{
			Strategy: types.StrategyDocsLink,
			Command:  "use codrax built-in: --auto-init-repo (CLI) / write_auto_init_repo: true (yaml) / REPL prompt on /approve",
			Why:      "codrax 自带三层授权门(单次/持久化/交互),自动 scaffold + 初始化 commit",
			Priority: 5, DiagRef: d,
		},
	}
}

func (s *systemRecommender) gitNoCommits(d types.Diagnosis, _ *types.EnvFacts) []types.Recommendation {
	return []types.Recommendation{{
		Strategy: types.StrategyProjectInstall,
		Command:  "!git commit --allow-empty -m \"initial commit\"",
		Why:      "已有 .git/ 但 HEAD 无 commit;创建一个空首次 commit",
		EstimatedSec: 1, Priority: 1, DiagRef: d,
	}}
}

func (s *systemRecommender) systemLib(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	if env == nil {
		return nil
	}
	// Stage 1 deterministic translation table for the most common
	// libs across the most common distros. Long-tail goes to LLM
	// (Stage 3).
	libToPkg := map[string]map[string]string{
		"ssl": {
			"debian": "libssl-dev", "rhel": "openssl-devel",
			"alpine": "openssl-dev", "arch": "openssl",
		},
		"crypto": {
			"debian": "libssl-dev", "rhel": "openssl-devel",
			"alpine": "openssl-dev", "arch": "openssl",
		},
		"z": {
			"debian": "zlib1g-dev", "rhel": "zlib-devel",
			"alpine": "zlib-dev", "arch": "zlib",
		},
		"xml2": {
			"debian": "libxml2-dev", "rhel": "libxml2-devel",
			"alpine": "libxml2-dev", "arch": "libxml2",
		},
		"curl": {
			"debian": "libcurl4-openssl-dev", "rhel": "libcurl-devel",
			"alpine": "curl-dev", "arch": "curl",
		},
		"gl": {
			"debian": "libgl1-mesa-dev", "rhel": "mesa-libGL-devel",
			"arch": "mesa",
		},
		"opencv": {
			"debian": "libopencv-dev", "rhel": "opencv-devel",
			"arch": "opencv",
		},
	}
	pkgs, ok := libToPkg[d.Tool]
	if !ok {
		// Fallback: derive `libX-dev` style guess
		guess := "lib" + d.Tool + "-dev"
		switch env.OSFamily {
		case "debian", "alpine":
			return []types.Recommendation{{
				Strategy: types.StrategyGlobalInstall,
				Command:  installCmd(env.OSFamily, guess),
				Why:      "未在内置表中 — 启发式猜测包名 lib<lib>-dev",
				NeedsNetwork: true, NeedsSudo: true,
				EstimatedSec: 30, Priority: 5, DiagRef: d,
				Caveats: []string{"启发式猜测,可能不准;LLM 兜底会更精确"},
			}}
		}
		return nil
	}
	pkg := pkgs[env.OSFamily]
	if pkg == "" {
		return nil
	}
	return []types.Recommendation{{
		Strategy: types.StrategyGlobalInstall,
		Command:  installCmd(env.OSFamily, pkg),
		Why:      "已知库名 → 发行版包名映射",
		NeedsNetwork: true, NeedsSudo: true, EstimatedSec: 30,
		Priority: 1, DiagRef: d,
	}}
}

// installCmd returns the canonical "install package" command for
// the given OS family.
func installCmd(family, pkg string) string {
	switch family {
	case "debian":
		return "!sudo apt install -y " + pkg
	case "rhel":
		return "!sudo dnf install -y " + pkg
	case "alpine":
		return "!sudo apk add --no-cache " + pkg
	case "arch":
		return "!sudo pacman -S --noconfirm " + pkg
	case "darwin":
		return "!brew install " + pkg
	}
	return "(unknown package manager) — install " + pkg + " manually"
}
