package recommend

import "github.com/hanchaoqun/codrax/internal/types"

// This file packs the remaining 9 runners' Recommenders. Each is a
// thin per-language switch on Diagnosis.Kind. Splitting them into
// separate files would inflate the package count without code-org
// value — they share the same shape.

func init() {
	Register("rust", &rustRecommender{})
	Register("go", &goRecommender{})
	Register("ruby", &rubyRecommender{})
	Register("java", &javaRecommender{})
	Register("swift", &swiftRecommender{})
	Register("cmake", &cmakeRecommender{})
	Register("meson", &mesonRecommender{})
	Register("make", &makeRecommender{})
	Register("hvigor", &hvigorRecommender{})
	Register("cjpm", &cjpmRecommender{})
}

// ── Rust ───────────────────────────────────────────────────────
type rustRecommender struct{}

func (rustRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagDepsMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyProjectInstall,
			Command:  "!cargo fetch",
			Why:      "cargo 自动从 Cargo.toml 解析依赖",
			NeedsNetwork: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		}}
	case types.DiagBuildToolchainMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyToolchainBootstrap,
			Command:  "!curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y",
			Why:      "rustup 是官方推荐安装路径(用户级,无需 sudo)",
			NeedsNetwork: true, EstimatedSec: 120,
			Priority: 1, DiagRef: d,
			Caveats: []string{"需要重启 shell 或 source $HOME/.cargo/env"},
		}}
	case types.DiagConfigMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyDocsLink,
			Command:  "create Cargo.toml first via !cargo init",
			Priority: 1, DiagRef: d,
		}}
	}
	return nil
}

// ── Go ─────────────────────────────────────────────────────────
type goRecommender struct{}

func (goRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagDepsMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyProjectInstall,
			Command:  "!go mod download",
			Why:      "go.mod 已存在,下载所有依赖",
			NeedsNetwork: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		}}
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil {
			switch env.OSFamily {
			case "debian":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo apt install -y golang-go",
					Why:      "Debian/Ubuntu(版本可能较旧)",
					NeedsNetwork: true, NeedsSudo: true,
					EstimatedSec: 60, Priority: 3, DiagRef: d,
				})
			case "darwin":
				if env.HasPkgManager("brew") {
					out = append(out, types.Recommendation{
						Strategy: types.StrategyGlobalInstall,
						Command:  "!brew install go",
						Why:      "macOS Homebrew",
						NeedsNetwork: true, EstimatedSec: 60,
						Priority: 1, DiagRef: d,
					})
				}
			}
		}
		out = append(out, types.Recommendation{
			Strategy: types.StrategyDocsLink,
			Command:  "see https://go.dev/dl/",
			Why:      "Go 官方下载(含 Windows MSI 和 macOS pkg)",
			Priority: 99, DiagRef: d,
		})
		return out
	case types.DiagConfigMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyProjectInstall,
			Command:  "!go mod init <module-name>",
			Why:      "Go 项目需要 go.mod",
			Priority: 1, DiagRef: d,
		}}
	}
	return nil
}

// ── Ruby ───────────────────────────────────────────────────────
type rubyRecommender struct{}

func (rubyRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagRunnerMissing:
		return []types.Recommendation{
			{
				Strategy: types.StrategyUserInstall,
				Command:  "!gem install --user-install bundler",
				Why:      "用户级 Bundler(无需 sudo)",
				NeedsNetwork: true, EstimatedSec: 15,
				Priority: 1, DiagRef: d,
				Caveats: []string{"~/.local/share/gem/ruby/<ver>/bin 需要在 PATH 中"},
			},
			{
				Strategy: types.StrategyGlobalInstall,
				Command:  "!sudo gem install bundler",
				Why:      "全局 Bundler",
				NeedsNetwork: true, NeedsSudo: true, EstimatedSec: 15,
				Priority: 5, DiagRef: d,
			},
		}
	case types.DiagDepsMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyProjectInstall,
			Command:  "!bundle install --path vendor/bundle",
			Why:      "项目隔离的 gem 安装到 vendor/bundle",
			NeedsNetwork: true, EstimatedSec: 60,
			Priority: 1, DiagRef: d,
		}}
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil {
			switch env.OSFamily {
			case "debian":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo apt install -y ruby ruby-dev",
					Why:      "Debian/Ubuntu", NeedsNetwork: true,
					NeedsSudo: true, EstimatedSec: 60,
					Priority: 1, DiagRef: d,
				})
			case "darwin":
				if env.HasPkgManager("brew") {
					out = append(out, types.Recommendation{
						Strategy: types.StrategyGlobalInstall,
						Command:  "!brew install ruby",
						Why: "macOS Homebrew", NeedsNetwork: true,
						EstimatedSec: 90, Priority: 1, DiagRef: d,
					})
				}
			}
		}
		out = append(out, types.Recommendation{
			Strategy: types.StrategyToolchainBootstrap,
			Command:  "!curl -sSL https://get.rvm.io | bash -s stable && rvm install ruby --latest",
			Why:      "rvm 跨平台用户级 Ruby 多版本管理",
			NeedsNetwork: true, EstimatedSec: 300,
			Priority: 10, DiagRef: d,
		})
		return out
	}
	return nil
}

// ── Java ───────────────────────────────────────────────────────
type javaRecommender struct{}

func (javaRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagDepsMissing:
		var out []types.Recommendation
		if env != nil && env.HasProjectFile("pom.xml") {
			out = append(out, types.Recommendation{
				Strategy: types.StrategyProjectInstall,
				Command:  "!mvn dependency:resolve",
				Why:      "Maven 项目;拉所有依赖",
				NeedsNetwork: true, EstimatedSec: 120,
				Priority: 1, DiagRef: d,
			})
		}
		if env != nil && (env.HasProjectFile("build.gradle") || env.HasProjectFile("build.gradle.kts")) {
			cmd := "!gradle dependencies"
			if len(env.Javas) > 0 && env.Javas[0].HasGradleWrapper {
				cmd = "!./gradlew dependencies"
			}
			out = append(out, types.Recommendation{
				Strategy: types.StrategyProjectInstall, Command: cmd,
				Why: "Gradle 项目", NeedsNetwork: true,
				EstimatedSec: 90, Priority: 1, DiagRef: d,
			})
		}
		return out
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil {
			switch env.OSFamily {
			case "debian":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo apt install -y default-jdk maven",
					Why:      "Debian/Ubuntu(default-jdk 选当前 LTS)",
					NeedsNetwork: true, NeedsSudo: true,
					EstimatedSec: 120, Priority: 1, DiagRef: d,
				})
			case "darwin":
				if env.HasPkgManager("brew") {
					out = append(out, types.Recommendation{
						Strategy: types.StrategyGlobalInstall,
						Command:  "!brew install openjdk maven",
						Why: "macOS Homebrew", NeedsNetwork: true,
						EstimatedSec: 120, Priority: 1, DiagRef: d,
					})
				}
			}
		}
		out = append(out, types.Recommendation{
			Strategy: types.StrategyToolchainBootstrap,
			Command:  "!curl -s https://get.sdkman.io | bash && source ~/.sdkman/bin/sdkman-init.sh && sdk install java",
			Why:      "SDKMAN! 用户级 JDK 多版本管理",
			NeedsNetwork: true, EstimatedSec: 300,
			Priority: 10, DiagRef: d,
		})
		return out
	}
	return nil
}

// ── Swift ──────────────────────────────────────────────────────
type swiftRecommender struct{}

func (swiftRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagDepsMissing:
		return []types.Recommendation{{
			Strategy: types.StrategyProjectInstall,
			Command:  "!swift package resolve",
			Why:      "Swift Package Manager 自动解析依赖",
			NeedsNetwork: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		}}
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil && env.OSFamily == "darwin" {
			out = append(out, types.Recommendation{
				Strategy: types.StrategyDocsLink,
				Command:  "open Mac App Store and install Xcode (provides Swift toolchain)",
				Why:      "macOS 官方 Swift 来自 Xcode", Priority: 1, DiagRef: d,
			})
		}
		out = append(out, types.Recommendation{
			Strategy: types.StrategyDocsLink,
			Command:  "see https://swift.org/download/",
			Why:      "Linux/Windows 用 Swift 官方分发",
			Priority: 99, DiagRef: d,
		})
		return out
	}
	return nil
}

// ── CMake ──────────────────────────────────────────────────────
type cmakeRecommender struct{}

func (cmakeRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil {
			switch env.OSFamily {
			case "debian":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo apt install -y cmake ninja-build",
					Why:      "Debian/Ubuntu;cmake + ninja(generator)",
					NeedsNetwork: true, NeedsSudo: true,
					EstimatedSec: 60, Priority: 1, DiagRef: d,
				})
			case "darwin":
				if env.HasPkgManager("brew") {
					out = append(out, types.Recommendation{
						Strategy: types.StrategyGlobalInstall,
						Command:  "!brew install cmake ninja",
						Why: "macOS Homebrew", NeedsNetwork: true,
						EstimatedSec: 60, Priority: 1, DiagRef: d,
					})
				}
			}
		}
		return out
	}
	return nil
}

// ── Meson ──────────────────────────────────────────────────────
type mesonRecommender struct{}

func (mesonRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagBuildToolchainMissing:
		return []types.Recommendation{{
			Strategy:     types.StrategyUserInstall,
			Command:      "!pip install --user meson ninja",
			Why:          "Meson 通过 pip 安装(纯 Python)",
			NeedsNetwork: true, EstimatedSec: 30,
			Priority: 1, DiagRef: d,
		}}
	}
	return nil
}

// ── Make ───────────────────────────────────────────────────────
type makeRecommender struct{}

func (makeRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	switch d.Kind {
	case types.DiagBuildToolchainMissing:
		var out []types.Recommendation
		if env != nil {
			switch env.OSFamily {
			case "debian":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo apt install -y build-essential",
					Why:      "Debian/Ubuntu;build-essential 含 make + gcc",
					NeedsNetwork: true, NeedsSudo: true,
					EstimatedSec: 120, Priority: 1, DiagRef: d,
				})
			case "darwin":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyDocsLink,
					Command:  "!xcode-select --install",
					Why:      "macOS Command Line Tools 含 make",
					Priority: 1, DiagRef: d,
				})
			case "rhel":
				out = append(out, types.Recommendation{
					Strategy: types.StrategyGlobalInstall,
					Command:  "!sudo dnf groupinstall -y \"Development Tools\"",
					Why:      "RHEL/Fedora 工具组合",
					NeedsNetwork: true, NeedsSudo: true,
					EstimatedSec: 120, Priority: 1, DiagRef: d,
				})
			}
		}
		return out
	}
	return nil
}

// ── hvigor (HarmonyOS) ─────────────────────────────────────────
type hvigorRecommender struct{}

func (hvigorRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	return []types.Recommendation{{
		Strategy: types.StrategyDocsLink,
		Command:  "see HarmonyOS DevEco Studio: https://developer.harmonyos.com/cn/develop/deveco-studio",
		Why:      "DevEco Studio 提供完整 hvigor / ohpm 工具链",
		Priority: 1, DiagRef: d,
		Caveats: []string{"hvigor 不支持单独安装,必须配合 DevEco Studio"},
	}}
}

// ── cjpm (Cangjie) ─────────────────────────────────────────────
type cjpmRecommender struct{}

func (cjpmRecommender) Recommend(d types.Diagnosis, env *types.EnvFacts, _ types.EnvRecommendSettings) []types.Recommendation {
	return []types.Recommendation{{
		Strategy: types.StrategyDocsLink,
		Command:  "see Cangjie SDK: https://cangjie-lang.cn/download",
		Why:      "Cangjie 工具链下载页面(含 cjpm)",
		Priority: 1, DiagRef: d,
	}}
}
