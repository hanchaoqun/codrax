package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (t *TraceQuery) streamFrameCatalog(ctx *types.BusContext, p traceQueryParams, path string, window traceQueryNormalizedWindow) types.ToolResult {
	q := tracequery.Query{
		PID: p.PID.Int(), Thread: p.Thread, TargetScope: p.TargetScope,
		TimeStart: window.RequestedStart, TimeEnd: window.RequestedEnd, TimeStartSet: p.TimeStart.Set(), TimeEndSet: p.TimeEnd.Set(),
		LineStart: p.LineStart.Int(), LineEnd: p.LineEnd.Int(),
	}
	catalog, err := tracequery.StreamFrameCatalog(contextFromBus(ctx), path, q)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}
	}
	ref := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "frame-catalog.json", string(payload))
	var summary strings.Builder
	shown := min(len(catalog.Frames), tracequery.ViewCapacityFor(tracequery.ViewFrameCatalog).ClampLimit(p.Limit.Int()))
	fmt.Fprintf(&summary, "# Trace Query: frame_catalog\nscanned_lines=%d closed_frames=%d unclosed_frames=%d shown=%d omitted=%d\n", catalog.ScannedLines, len(catalog.Frames), catalog.Unclosed, shown, len(catalog.Frames)-shown)
	if ref != "" {
		fmt.Fprintf(&summary, "full_catalog=%s\n", ref)
	}
	for _, frame := range catalog.Frames[:shown] {
		fmt.Fprintf(&summary, "- %s vsync=%d tid=%d thread=%q start_ns=%d end_ns=%d lines=%d-%d points=%q\n", frame.ID, frame.Vsync, frame.TID, frame.Thread, frame.StartNS, frame.EndNS, frame.StartLine, frame.EndLine, frame.Points)
	}
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: summary.String(), RawRef: ref, Timestamp: time.Now()}
}
