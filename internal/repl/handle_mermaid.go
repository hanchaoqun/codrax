package repl

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/render"
)

// handleMermaidCmd dispatches /mermaid subcommands. Surfaces the
// process-wide mermaid render counters maintained by
// internal/render/mermaid_stats.go so operators can see how often
// the LLM is emitting unsupported diagram kinds, how many narrow
// multi-byte runes the fallback table is rewriting, and whether
// the renderability gate (L4) is rejecting submissions.
//
// Recognised forms:
//
//   /mermaid              — alias for /mermaid stats
//   /mermaid stats        — show counters (process-lifetime totals)
//   /mermaid help         — usage line
//
// Counters reset only on process restart by design — the same
// rationale env_recommend uses (§21 observability): aggregating
// across the whole session shows the operator real frequency,
// reset-per-turn would lose that.
func (r *REPL) handleMermaidCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/mermaid"))
	if rest == "" {
		rest = "stats"
	}
	fields := strings.Fields(rest)
	switch fields[0] {
	case "stats":
		r.mermaidStats()
	case "help":
		r.info("/mermaid subcommands: stats")
	default:
		r.info("/mermaid subcommands: stats")
	}
}

// mermaidStats renders the current MermaidStats snapshot. Output
// language follows REPL.language (zh = Chinese, default = English).
func (r *REPL) mermaidStats() {
	s := render.GetMermaidStats()
	zh := r.language == "zh"

	var b strings.Builder
	if zh {
		b.WriteString("Mermaid 渲染计数（本进程累计）\n\n")
		fmt.Fprintf(&b, "  尝试渲染:    %d\n", s.Attempts)
		fmt.Fprintf(&b, "  渲染成功:    %d\n", s.Succeeded)
		fmt.Fprintf(&b, "  字符替换块数: %d (累计 %d 个字符)\n", s.FallbackSubstituted, s.FallbackRunes)
		fmt.Fprintf(&b, "  库拒绝/超限: %d\n", s.LibraryRejected)
		fmt.Fprintf(&b, "  闸门拦截:    %d\n", s.GateBlocked)
		if len(s.UnsupportedKind) > 0 {
			b.WriteString("\n  库不支持的图类（已留源 + 标 ⚠）:\n")
			for _, k := range s.SortedUnsupportedKinds() {
				fmt.Fprintf(&b, "    %-22s × %d\n", k, s.UnsupportedKind[k])
			}
		}
		if s.LastFailureReason != "" {
			fmt.Fprintf(&b, "\n  最近一次失败原因: %s\n", s.LastFailureReason)
		}
		if s.Attempts == 0 && s.GateBlocked == 0 && len(s.UnsupportedKind) == 0 {
			b.WriteString("\n  本会话尚无 Mermaid 渲染活动。\n")
		}
	} else {
		b.WriteString("Mermaid render counters (process lifetime)\n\n")
		fmt.Fprintf(&b, "  attempts:           %d\n", s.Attempts)
		fmt.Fprintf(&b, "  succeeded:          %d\n", s.Succeeded)
		fmt.Fprintf(&b, "  blocks w/ fallback: %d (%d runes total)\n", s.FallbackSubstituted, s.FallbackRunes)
		fmt.Fprintf(&b, "  library rejected:   %d\n", s.LibraryRejected)
		fmt.Fprintf(&b, "  gate blocked:       %d\n", s.GateBlocked)
		if len(s.UnsupportedKind) > 0 {
			b.WriteString("\n  Unsupported diagram kinds (kept as source + ⚠):\n")
			for _, k := range s.SortedUnsupportedKinds() {
				fmt.Fprintf(&b, "    %-22s × %d\n", k, s.UnsupportedKind[k])
			}
		}
		if s.LastFailureReason != "" {
			fmt.Fprintf(&b, "\n  last failure reason: %s\n", s.LastFailureReason)
		}
		if s.Attempts == 0 && s.GateBlocked == 0 && len(s.UnsupportedKind) == 0 {
			b.WriteString("\n  no mermaid render activity in this session.\n")
		}
	}
	r.renderBordered(b.String())
}
