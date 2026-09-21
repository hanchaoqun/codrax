package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the audit snapshot in lockstep with the existing recovery-only display
// transform. Never derive ownership by parsing the recovered prose.
func sanitizeRecoveredAnswerSurfaces(ctx *types.AgentContext, answer, lang string, trim bool) string {
	transform := func(s string) string {
		if trim {
			s = strings.TrimSpace(s)
		}
		return render.SanitizeDegradedMermaidBlocks(s, lang)
	}
	out := transform(answer)
	if ctx != nil && ctx.Mutable != nil {
		if s := ctx.Mutable.AnswerRenderedSurfaces(); s != nil && s.Answer == answer {
			s.Answer, s.Primary, s.Principal = out, transform(s.Primary), transform(s.Principal)
			ctx.Mutable.SetAnswerRenderedSurfaces(*s)
		}
	}
	return out
}
