package repomap

import "fmt"

// GenerateView produces a markdown view of the graph. Every
// supported view type is routed through the Phase 3 dual-channel
// path (GenerateViewData → RenderMarkdown), so programmatic
// consumers can walk the structured form directly via
// GenerateViewData and human-facing callers get the rendered
// markdown here.
//
// Unknown view types fall back to "overview".
func GenerateView(g *Graph, viewType string, params ViewParams) string {
	if data := GenerateViewData(g, viewType, params); data != nil {
		return RenderMarkdown(data)
	}
	return RenderMarkdown(GenerateViewData(g, "overview", params))
}

// abbreviate is shared by the view builders in render.go to cap
// long file/symbol lists at maxN entries, appending an "(+N more)"
// marker when truncated.
func abbreviate(items []string, maxN int) []string {
	if len(items) <= maxN {
		return items
	}
	out := make([]string, maxN)
	copy(out, items[:maxN])
	out = append(out, fmt.Sprintf("(+%d more)", len(items)-maxN))
	return out
}
