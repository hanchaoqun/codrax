package recommend

import "github.com/hanchaoqun/codrax/internal/types"

func init() { Register("node", &nodeRecommender{}) }

type nodeRecommender struct{}

func (n *nodeRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagDepsMissing, types.DiagRunnerMissing:
		return n.installDeps(d, env)
	case types.DiagBuildToolchainMissing:
		return n.toolchainMissing(d, env)
	case types.DiagConfigMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyDocsLink,
			Command:  "create package.json first via !npm init -y",
			Priority: 1, DiagRef: d,
		}}
	}
	return nil
}

func (n *nodeRecommender) installDeps(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	if env == nil {
		return nil
	}
	var out []types.Recommendation
	// Lockfile-driven preferred: npm ci > pnpm > yarn > bun.
	switch {
	case env.HasProjectFile("package-lock.json"):
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!npm ci",
			Why:      "package-lock.json 存在;ci 命令最快且确定性",
			NeedsNetwork: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		})
	case env.HasProjectFile("pnpm-lock.yaml"):
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!pnpm install --frozen-lockfile",
			Why:      "pnpm-lock.yaml 存在", NeedsNetwork: true,
			EstimatedSec: 30, Priority: 1, DiagRef: d,
		})
	case env.HasProjectFile("yarn.lock"):
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!yarn install --frozen-lockfile",
			Why:      "yarn.lock 存在", NeedsNetwork: true,
			EstimatedSec: 30, Priority: 1, DiagRef: d,
		})
	case env.HasProjectFile("bun.lockb"):
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!bun install",
			Why:      "bun.lockb 存在(bun 项目)",
			NeedsNetwork: true, EstimatedSec: 5,
			Priority: 1, DiagRef: d,
		})
	default:
		out = append(out, types.Recommendation{
			Strategy: types.StrategyProjectInstall,
			Command:  "!npm install",
			Why:      "无 lockfile,通用 npm install",
			NeedsNetwork: true, EstimatedSec: 60,
			Priority: 5, DiagRef: d,
		})
	}
	return out
}

func (n *nodeRecommender) toolchainMissing(d types.Diagnosis, env *types.EnvFacts) []types.Recommendation {
	if env == nil {
		return nil
	}
	var out []types.Recommendation
	switch env.OSFamily {
	case "debian":
		out = append(out,
			types.Recommendation{
				Strategy: types.StrategyToolchainBootstrap,
				Command:  "!curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt install -y nodejs",
				Why:      "NodeSource 提供更新版 Node(系统包通常版本陈旧)",
				NeedsNetwork: true, NeedsSudo: true,
				EstimatedSec: 120, Priority: 1, DiagRef: d,
				Caveats: []string{"需要 sudo;脚本执行 apt"},
			},
			types.Recommendation{
				Strategy: types.StrategyGlobalInstall,
				Command:  "!sudo apt install -y nodejs npm",
				Why:      "系统仓自带版本(可能过旧)",
				NeedsNetwork: true, NeedsSudo: true,
				EstimatedSec: 30, Priority: 5, DiagRef: d,
			})
	case "darwin":
		if env.HasPkgManager("brew") {
			out = append(out, types.Recommendation{
				Strategy: types.StrategyGlobalInstall,
				Command:  "!brew install node",
				Why:      "macOS Homebrew", NeedsNetwork: true,
				EstimatedSec: 90, Priority: 1, DiagRef: d,
			})
		}
	case "windows":
		out = append(out, types.Recommendation{
			Strategy: types.StrategyDocsLink,
			Command:  "see https://nodejs.org/ or `winget install OpenJS.NodeJS`",
			Why:      "Windows 推荐用 winget 或官方安装器",
			Priority: 1, DiagRef: d,
		})
	}
	out = append(out, types.Recommendation{
		Strategy: types.StrategyToolchainBootstrap,
		Command:  "!curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash && nvm install --lts",
		Why:      "nvm 跨平台可控版本(适合多项目环境)",
		NeedsNetwork: true, EstimatedSec: 120,
		Priority: 10, DiagRef: d,
		Caveats: []string{"需要重启 shell 生效"},
	})
	return out
}
