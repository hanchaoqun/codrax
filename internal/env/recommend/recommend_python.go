package recommend

import "github.com/hanchaoqun/codrax/internal/types"

func init() { Register("python", &pythonRecommender{}) }

type pythonRecommender struct{}

func (p *pythonRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagRunnerMissing:
		return p.runnerMissing(d, env)
	case types.DiagDepsMissing:
		return p.depsMissing(d, env)
	case types.DiagBuildToolchainMissing:
		return p.toolchainMissing(d, env)
	case types.DiagConfigMissing:
		return p.configMissing(d, env)
	}
	return nil
}

func (p *pythonRecommender) runnerMissing(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	var out []types.Recommendation
	// Candidate 1: project-isolated venv (top priority, safest).
	if env != nil && env.HasPython() {
		out = append(out, types.Recommendation{
			Strategy:     types.StrategyVenvInstall,
			Command:      "!python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report",
			Why:          "项目隔离;PATH 已有 python3,pyproject 风格项目首选",
			NeedsNetwork: true,
			EstimatedSec: 30,
			Priority:     1,
			DiagRef:      d,
		})
	}
	// Candidate 2: pip --user.
	if env != nil && env.HasPip() {
		out = append(out, types.Recommendation{
			Strategy:     types.StrategyUserInstall,
			Command:      "!pip install --user pytest pytest-json-report",
			Why:          "已有 pip;装到 ~/.local 用户目录",
			NeedsNetwork: true,
			EstimatedSec: 10,
			Caveats:      []string{"确保 ~/.local/bin 在 PATH 中"},
			Priority:     2,
			DiagRef:      d,
		})
	}
	// Candidate 3: poetry / hatch / uv (if their manifest is
	// detected).
	if env != nil {
		switch {
		case env.HasPkgManager("poetry") && env.HasProjectFile("pyproject.toml"):
			out = append(out, types.Recommendation{
				Strategy:     types.StrategyProjectInstall,
				Command:      "!poetry add --group dev pytest pytest-json-report",
				Why:          "项目用 poetry 管理依赖",
				NeedsNetwork: true,
				EstimatedSec: 30,
				Priority:     1,
				DiagRef:      d,
			})
		case env.HasPkgManager("uv") && env.HasProjectFile("pyproject.toml"):
			out = append(out, types.Recommendation{
				Strategy:     types.StrategyProjectInstall,
				Command:      "!uv add --dev pytest pytest-json-report",
				Why:          "项目用 uv 管理依赖(快且支持 lockfile)",
				NeedsNetwork: true,
				EstimatedSec: 5,
				Priority:     1,
				DiagRef:      d,
			})
		case env.HasPkgManager("hatch"):
			out = append(out, types.Recommendation{
				Strategy:     types.StrategyProjectInstall,
				Command:      "!hatch env create && hatch shell",
				Why:          "项目用 hatch 管理环境",
				NeedsNetwork: true,
				EstimatedSec: 60,
				Priority:     1,
				DiagRef:      d,
			})
		}
	}
	return out
}

func (p *pythonRecommender) depsMissing(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	var out []types.Recommendation
	if env == nil {
		return out
	}
	if env.HasProjectFile("requirements.txt") {
		out = append(out, types.Recommendation{
			Strategy: types.StrategyVenvInstall,
			Command:  "!python3 -m venv .venv && .venv/bin/pip install -r requirements.txt",
			Why:      "requirements.txt 存在;装到项目 .venv",
			NeedsNetwork: true, EstimatedSec: 60,
			Priority: 1, DiagRef: d,
		})
	}
	if env.HasPkgManager("poetry") && env.HasProjectFile("pyproject.toml") {
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!poetry install",
			Why:      "项目用 poetry",
			NeedsNetwork: true, EstimatedSec: 60,
			Priority: 1, DiagRef: d,
		})
	}
	if env.HasPkgManager("uv") && env.HasProjectFile("pyproject.toml") {
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!uv sync",
			Why:      "项目用 uv",
			NeedsNetwork: true, EstimatedSec: 10,
			Priority: 1, DiagRef: d,
		})
	}
	return out
}

func (p *pythonRecommender) toolchainMissing(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	if env == nil {
		return nil
	}
	var out []types.Recommendation
	switch env.OSFamily {
	case "debian":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo apt install -y python3 python3-venv python3-pip",
			Why:      "Debian/Ubuntu 系发行版的 python3 安装路径",
			NeedsNetwork: true, NeedsSudo: true, EstimatedSec: 60,
			Priority: 1, DiagRef: d,
			Caveats: []string{"需要 sudo;影响所有用户"},
		})
	case "rhel":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo dnf install -y python3 python3-pip",
			Why:      "RHEL/Fedora 系发行版", NeedsNetwork: true, NeedsSudo: true,
			EstimatedSec: 60, Priority: 1, DiagRef: d,
			Caveats: []string{"需要 sudo"},
		})
	case "darwin":
		if env.HasPkgManager("brew") {
			out = append(out, types.Recommendation{
				Strategy: types.StrategyGlobalInstall,
				Command:  "!brew install python",
				Why:      "macOS Homebrew", NeedsNetwork: true,
				EstimatedSec: 60, Priority: 1, DiagRef: d,
			})
		}
	case "alpine":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo apk add --no-cache python3 py3-pip",
			Why:      "Alpine 包名", NeedsNetwork: true, NeedsSudo: true,
			EstimatedSec: 30, Priority: 1, DiagRef: d,
		})
	case "arch":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyGlobalInstall,
			Command:  "!sudo pacman -S --noconfirm python python-pip",
			Why:      "Arch Linux", NeedsNetwork: true, NeedsSudo: true,
			EstimatedSec: 30, Priority: 1, DiagRef: d,
		})
	}
	// Always offer DocsLink as ultimate fallback
	out = append(out, types.Recommendation{
		Strategy: types.StrategyDocsLink,
		Command:  "see https://www.python.org/downloads/",
		Why:      "Python 官方下载页面",
		Priority: 99, DiagRef: d,
	})
	return out
}

func (p *pythonRecommender) configMissing(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	return []types.Recommendation{{
		Strategy: types.StrategyDocsLink,
		Command:  "create pyproject.toml or setup.py first",
		Why:      "Python 项目至少需要一个 manifest(pyproject.toml 优先)",
		Priority: 1, DiagRef: d,
	}}
}
