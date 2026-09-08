package tool

import "github.com/hanchaoqun/codrax/internal/types"

// Scope is presentation metadata, never a reason to remove a chain, clip an
// account, or change the model's selected causes. Ordinary elected windows
// retain their existing presentation; explicit requests get the shared ruler.
func runtimeTraceQueryScopePreface(text string, scope types.TraceQueryWindowScope, zh bool) string {
	if !scope.RequestedWindowKnown {
		return text
	}
	lang := "en"
	if zh {
		lang = "zh"
	}
	note := scope.Format(lang)
	if note == "" {
		return text
	}
	if text == "" {
		return note
	}
	return note + "\n\n" + text
}

func runtimeTraceQueryScopeTitleSuffix(scope types.TraceQueryWindowScope, zh bool) string {
	if !scope.IsSupportingExploration() {
		return ""
	}
	if zh {
		return "（补充查询范围）"
	}
	return " (Supplementary query window)"
}
