package recommend

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runnerDocsURL maps each supported runner to its canonical docs
// URL. Used by docsLinkFallback when neither LLM nor disk cache
// produced an actionable recommendation. Stable: these URLs change
// at the same cadence as the language ecosystems themselves
// (years). Update only when an upstream redirects.
//
// New runners SHOULD add an entry here so the offline / LLM-off
// fallback path remains useful.
var runnerDocsURL = map[string]string{
	"python": "https://www.python.org/downloads/",
	"node":   "https://nodejs.org/en/download",
	"rust":   "https://www.rust-lang.org/tools/install",
	"go":     "https://go.dev/dl/",
	"java":   "https://adoptium.net/",
	"ruby":   "https://www.ruby-lang.org/en/downloads/",
	"swift":  "https://swift.org/download/",
	"cmake":  "https://cmake.org/download/",
	"meson":  "https://mesonbuild.com/Getting-meson.html",
	"make":   "https://www.gnu.org/software/make/",
	"hvigor": "https://developer.harmonyos.com/cn/develop/deveco-studio",
	"cjpm":   "https://cangjie-lang.cn/download",
}

// docsLinkFallback produces a single DocsLink Recommendation when
// no other strategy produced output. The runner-to-URL map is the
// only piece of per-runner static knowledge the recommender
// retains under the LLM-first architecture; everything else
// (install commands, package names, OS-family branches) is
// delegated to the LLM via env_recommender.
//
// Returns nil when no URL is registered for the runner — caller
// surfaces a generic "no recommendation available" message via
// the Renderer.
func docsLinkFallback(d types.Diagnosis) []types.Recommendation {
	url, ok := runnerDocsURL[d.Runner]
	if !ok {
		return nil
	}
	why := docsLinkWhy(d.Kind, d.Runner)
	return []types.Recommendation{{
		Strategy: types.StrategyDocsLink,
		Command:  fmt.Sprintf("see %s", url),
		Why:      why,
		Priority: 99, DiagRef: d,
	}}
}

func docsLinkWhy(kind types.DiagKind, runner string) string {
	switch kind {
	case types.DiagRunnerMissing, types.DiagBuildToolchainMissing:
		return fmt.Sprintf("%s 工具链官方安装文档", runner)
	case types.DiagDepsMissing:
		return fmt.Sprintf("查阅 %s 项目的依赖安装步骤", runner)
	case types.DiagConfigMissing:
		return fmt.Sprintf("%s 项目 manifest / 项目结构说明", runner)
	}
	return fmt.Sprintf("%s 官方文档", runner)
}
