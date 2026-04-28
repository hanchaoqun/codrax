package env

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderRecommendations is the bilingual user-facing surface for
// the env_recommend pipeline. Takes the diagnosis + the sorted
// recommendation list + lang and returns text to embed in
// FailureSummary / ToolResult.Summary.
//
// Empty recs → returns the diagnostic alone with a "no
// recommendation available" footer; never produces empty output.
func RenderRecommendations(d types.Diagnosis, recs []types.Recommendation, lang string) string {
	if isZh(lang) {
		return renderZh(d, recs)
	}
	return renderEn(d, recs)
}

func renderZh(d types.Diagnosis, recs []types.Recommendation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✗ 环境诊断:%s", diagKindZh(d.Kind))
	if d.Tool != "" {
		fmt.Fprintf(&b, "(%s)", d.Tool)
	}
	if d.Runner != "" {
		fmt.Fprintf(&b, " / runner=%s", d.Runner)
	}
	b.WriteString("\n")
	if d.Source != "" {
		fmt.Fprintf(&b, "信号: %s\n", d.Source)
	}
	if len(recs) == 0 {
		b.WriteString("\n暂无可执行的安装命令。这通常表示自定义/小众环境;请手动安装或查看相应工具的官方文档。\n")
		return b.String()
	}
	b.WriteString("\n推荐:\n")
	for i, r := range recs {
		fmt.Fprintf(&b, "  %d) %s", i+1, r.Command)
		hints := []string{}
		if r.EstimatedSec > 0 {
			hints = append(hints, fmt.Sprintf("~%ds", r.EstimatedSec))
		}
		hints = append(hints, strategyZh(r.Strategy))
		if r.NeedsSudo {
			hints = append(hints, "需 sudo")
		}
		if !r.NeedsNetwork {
			hints = append(hints, "无需网络")
		}
		if len(hints) > 0 {
			fmt.Fprintf(&b, "  # %s", strings.Join(hints, ", "))
		}
		b.WriteString("\n")
		if r.Why != "" {
			fmt.Fprintf(&b, "     原因: %s\n", r.Why)
		}
		for _, c := range r.Caveats {
			fmt.Fprintf(&b, "     ⚠ %s\n", c)
		}
	}
	b.WriteString("\n复制以 ! 开头的命令到 codrax 提示符回车执行;或在主仓终端直接跑(去掉 !)。\n")
	return b.String()
}

func renderEn(d types.Diagnosis, recs []types.Recommendation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✗ Environment diagnosis: %s", diagKindEn(d.Kind))
	if d.Tool != "" {
		fmt.Fprintf(&b, " (%s)", d.Tool)
	}
	if d.Runner != "" {
		fmt.Fprintf(&b, " / runner=%s", d.Runner)
	}
	b.WriteString("\n")
	if d.Source != "" {
		fmt.Fprintf(&b, "Signal: %s\n", d.Source)
	}
	if len(recs) == 0 {
		b.WriteString("\nNo actionable install command available for this combination. " +
			"Likely a custom or niche environment; please install manually or consult the tool's official docs.\n")
		return b.String()
	}
	b.WriteString("\nRecommendations:\n")
	for i, r := range recs {
		fmt.Fprintf(&b, "  %d) %s", i+1, r.Command)
		hints := []string{}
		if r.EstimatedSec > 0 {
			hints = append(hints, fmt.Sprintf("~%ds", r.EstimatedSec))
		}
		hints = append(hints, strategyEn(r.Strategy))
		if r.NeedsSudo {
			hints = append(hints, "sudo")
		}
		if !r.NeedsNetwork {
			hints = append(hints, "offline-safe")
		}
		if len(hints) > 0 {
			fmt.Fprintf(&b, "  # %s", strings.Join(hints, ", "))
		}
		b.WriteString("\n")
		if r.Why != "" {
			fmt.Fprintf(&b, "     why: %s\n", r.Why)
		}
		for _, c := range r.Caveats {
			fmt.Fprintf(&b, "     ⚠ %s\n", c)
		}
	}
	b.WriteString("\nCopy commands starting with ! into the codrax prompt and press enter; or run them in your main repo terminal (drop the !).\n")
	return b.String()
}

func diagKindZh(k types.DiagKind) string {
	switch k {
	case types.DiagRunnerMissing:
		return "测试 runner 二进制缺失"
	case types.DiagDepsMissing:
		return "项目依赖未装"
	case types.DiagBuildToolchainMissing:
		return "编译/运行工具链缺失"
	case types.DiagSystemLibMissing:
		return "系统库 / 头文件缺失"
	case types.DiagConfigMissing:
		return "项目元数据缺失"
	case types.DiagGitNotInstalled:
		return "git 未安装"
	case types.DiagGitRepoNotInitialized:
		return "目标目录非 git 仓"
	case types.DiagGitRepoNoCommits:
		return "git 仓无 commits"
	}
	return string(k)
}

func diagKindEn(k types.DiagKind) string {
	switch k {
	case types.DiagRunnerMissing:
		return "test runner binary missing"
	case types.DiagDepsMissing:
		return "project dependencies not installed"
	case types.DiagBuildToolchainMissing:
		return "build / runtime toolchain missing"
	case types.DiagSystemLibMissing:
		return "system library / header missing"
	case types.DiagConfigMissing:
		return "project manifest missing"
	case types.DiagGitNotInstalled:
		return "git not installed"
	case types.DiagGitRepoNotInitialized:
		return "target directory is not a git repository"
	case types.DiagGitRepoNoCommits:
		return "git repo has no commits"
	}
	return string(k)
}

func strategyZh(s types.InstallStrategy) string {
	switch s {
	case types.StrategyVenvInstall:
		return "项目隔离"
	case types.StrategyProjectInstall:
		return "项目目录内"
	case types.StrategyUserInstall:
		return "用户级"
	case types.StrategyToolchainBootstrap:
		return "用户级工具链 bootstrap"
	case types.StrategyGlobalInstall:
		return "全局安装"
	case types.StrategyDocsLink:
		return "需手动查阅文档"
	}
	return ""
}

func strategyEn(s types.InstallStrategy) string {
	switch s {
	case types.StrategyVenvInstall:
		return "project-isolated"
	case types.StrategyProjectInstall:
		return "project-local"
	case types.StrategyUserInstall:
		return "user-level"
	case types.StrategyToolchainBootstrap:
		return "user-level toolchain bootstrap"
	case types.StrategyGlobalInstall:
		return "global install"
	case types.StrategyDocsLink:
		return "manual docs only"
	}
	return ""
}

func isZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}
