package repl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// handleEnvCmd dispatches /env subcommands. Recognised forms:
//
//   /env              — alias for /env show
//   /env show         — render the cached EnvFacts snapshot
//   /env probe        — re-run the probe and refresh the cache
//   /env explain      — LLM-powered conversational explanation of the
//                       current diagnosis (only when stderr is in
//                       recent context)
//   /env cache list   — show entries in the disk cache
//   /env cache clear  — wipe the disk cache
func (r *REPL) handleEnvCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/env"))
	if rest == "" {
		rest = "show"
	}
	fields := strings.Fields(rest)
	switch fields[0] {
	case "show":
		r.envShow()
	case "probe":
		r.envProbe(true)
	case "explain":
		r.envExplain(strings.Join(fields[1:], " "))
	case "cache":
		if len(fields) < 2 {
			r.info("/env cache <list|clear>")
			return
		}
		switch fields[1] {
		case "list":
			r.envCacheList()
		case "clear":
			r.envCacheClear()
		default:
			r.info("/env cache <list|clear>")
		}
	default:
		r.info("/env subcommands: show, probe, explain [stderr], cache list, cache clear")
	}
}

// envShow renders the cached EnvFacts. Lazily probes when no
// cache exists.
func (r *REPL) envShow() {
	facts := r.envFacts
	if facts == nil {
		r.envProbe(false)
		facts = r.envFacts
	}
	if facts == nil {
		r.warn("env probe unavailable\n")
		return
	}
	out := r.renderEnvFacts(facts)
	r.renderBordered(out)
}

func (r *REPL) envProbe(announce bool) {
	facts := env.Probe(env.ProbeOptions{
		RepoRoot:     r.repoRoot,
		ProbeNetwork: r.envSettings.ProbeNetwork,
	})
	r.envFacts = facts
	if announce {
		r.info(fmt.Sprintf("environment re-probed in %dms", facts.ProbeDurationMs))
		out := r.renderEnvFacts(facts)
		r.renderBordered(out)
	}
}

// envExplain consumes a stderr excerpt (or pulls last shell turn)
// and produces a conversational diagnosis explanation. Falls back
// to the standard render path when LLM is unwired.
func (r *REPL) envExplain(stderrArg string) {
	if r.envFacts == nil {
		r.envProbe(false)
	}
	if r.envFacts == nil {
		r.warn("env probe unavailable\n")
		return
	}
	stderr := stderrArg
	if stderr == "" && r.store != nil {
		stderr = latestShellTurnResponse(r.store.Recent())
	}
	if strings.TrimSpace(stderr) == "" {
		r.warn("no stderr supplied; usage: /env explain <stderr text>\n")
		return
	}
	d := env.Diagnose(stderr, "", r.envFacts)
	recs := env.Recommend(d, r.envFacts, r.envSettings)
	out := env.RenderRecommendations(d, recs, r.language)
	r.renderBordered(out)
}

// renderEnvFacts produces a compact bilingual environment summary.
func (r *REPL) renderEnvFacts(f *types.EnvFacts) string {
	if f == nil {
		return "(env unavailable)"
	}
	var b strings.Builder
	zh := strings.ToLower(strings.TrimSpace(r.language)) != "en"
	if zh {
		fmt.Fprintf(&b, "## 环境画像\n\n")
		fmt.Fprintf(&b, "- 探测时间: %s, 耗时 %dms\n", f.ProbedAt.Format("2006-01-02 15:04:05"), f.ProbeDurationMs)
		fmt.Fprintf(&b, "- OS: %s / %s / %s\n", f.OS, f.Arch, f.OSVersion)
		fmt.Fprintf(&b, "- OSFamily: %s; Shell: %s\n", f.OSFamily, f.Shell)
		fmt.Fprintf(&b, "- 在容器内: %t\n", f.InsideContainer)
		fmt.Fprintf(&b, "- 网络可达: %t (代理: %t)\n", f.Network.Reachable, f.Network.ProxySet)
		fmt.Fprintf(&b, "- Git 仓状态: %s\n", f.GitRepoState)
	} else {
		fmt.Fprintf(&b, "## Environment Snapshot\n\n")
		fmt.Fprintf(&b, "- Probed at %s, took %dms\n", f.ProbedAt.Format("2006-01-02 15:04:05"), f.ProbeDurationMs)
		fmt.Fprintf(&b, "- OS: %s / %s / %s\n", f.OS, f.Arch, f.OSVersion)
		fmt.Fprintf(&b, "- OSFamily: %s; Shell: %s\n", f.OSFamily, f.Shell)
		fmt.Fprintf(&b, "- Inside container: %t\n", f.InsideContainer)
		fmt.Fprintf(&b, "- Network reachable: %t (proxy: %t)\n", f.Network.Reachable, f.Network.ProxySet)
		fmt.Fprintf(&b, "- Git repo state: %s\n", f.GitRepoState)
	}
	if len(f.Pythons) > 0 {
		b.WriteString("\n### Pythons\n")
		for _, p := range f.Pythons {
			fmt.Fprintf(&b, "  %s (%s) origin=%s pytest=%t json-report=%t\n",
				p.Path, p.Version, p.Origin, p.HasPytest, p.HasJsonReport)
		}
	}
	if len(f.Nodes) > 0 {
		b.WriteString("\n### Nodes\n")
		for _, n := range f.Nodes {
			fmt.Fprintf(&b, "  %s (%s) pkg-managers=%s\n",
				n.Path, n.Version, strings.Join(n.PkgManagers, ","))
		}
	}
	if len(f.Rusts) > 0 {
		b.WriteString("\n### Rust\n")
		for _, t := range f.Rusts {
			fmt.Fprintf(&b, "  rustc %s (origin=%s); cargo=%s\n",
				t.Version, t.Origin, t.CargoPath)
		}
	}
	if len(f.Javas) > 0 {
		b.WriteString("\n### Java\n")
		for _, t := range f.Javas {
			fmt.Fprintf(&b, "  javac=%s java_version=%s mvn=%s gradle=%s wrapper=%t\n",
				t.JavacPath, t.JavaVersion, t.MavenPath, t.GradlePath, t.HasGradleWrapper)
		}
	}
	if len(f.Rubies) > 0 {
		b.WriteString("\n### Ruby\n")
		for _, t := range f.Rubies {
			fmt.Fprintf(&b, "  %s (%s) bundler=%s\n", t.RubyPath, t.Version, t.BundlerPath)
		}
	}
	if len(f.PkgManagers) > 0 {
		b.WriteString("\n### Package managers\n  ")
		keys := make([]string, 0, len(f.PkgManagers))
		for k := range f.PkgManagers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(strings.Join(keys, ", "))
		b.WriteString("\n")
	}
	if len(f.ProjectFiles) > 0 {
		b.WriteString("\n### Project manifests\n  ")
		keys := make([]string, 0, len(f.ProjectFiles))
		for k := range f.ProjectFiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(strings.Join(keys, ", "))
		b.WriteString("\n")
	}
	return b.String()
}

func (r *REPL) envCacheList() {
	c := envCacheRef()
	if c == nil {
		r.info("env cache unavailable")
		return
	}
	entries := c.Snapshot()
	if len(entries) == 0 {
		r.info("env cache empty")
		return
	}
	r.info(fmt.Sprintf("%d entries at %s", len(entries), c.Path()))
	for _, e := range entries {
		r.info(fmt.Sprintf("  %s  source=%s  age=%s", e.Key, e.Source,
			e.CreatedAt.Format("2006-01-02 15:04")))
	}
}

func (r *REPL) envCacheClear() {
	c := envCacheRef()
	if c == nil {
		r.info("env cache unavailable")
		return
	}
	if err := c.Clear(); err != nil {
		r.warn("clear failed: %v\n", err)
		return
	}
	r.info("env cache cleared")
}

// latestShellTurnResponse scans the recent buffer in reverse and
// returns the Response of the newest `!`-prefixed shell-bang turn.
// Pre-this-helper the inline loop iterated forward and break'd on
// the FIRST match, which surfaced the OLDEST shell turn — the
// opposite of what /env explain wants. Returns "" when no shell
// turn exists.
//
// memory.Store.Recent() returns chronologically (oldest at slice
// index 0); the helper walks i := len(recent)-1; i >= 0; i-- so
// the first match is the most-recent shell turn.
func latestShellTurnResponse(recent []memory.Turn) string {
	for i := len(recent) - 1; i >= 0; i-- {
		t := recent[i]
		if strings.HasPrefix(t.Request, "!") {
			return t.Response
		}
	}
	return ""
}
