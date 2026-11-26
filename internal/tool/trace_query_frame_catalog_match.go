package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (t *TraceQuery) finishFrameCatalogMatch(ctx *types.BusContext, inputs [2]types.NativeMeasurementPairSide, targetVsync uint64) types.ToolResult {
	out := types.ToolResult{ToolName: t.Name(), Timestamp: time.Now()}
	var catalogs [2]tracequery.FrameCatalog
	for i, input := range inputs {
		if !input.Result.Success {
			out.Summary = fmt.Sprintf("frame selection side %d failed: %s", i+1, input.Result.Summary)
			return out
		}
		catalog, err := readFrameCatalogResult(input.Result)
		if err != nil {
			out.Summary = fmt.Sprintf("frame selection could not read catalog for side %d: %v", i+1, err)
			return out
		}
		catalogs[i] = catalog
	}
	result, err := tracequery.MatchFrameCatalog(catalogs[1], catalogs[0], targetVsync, 5)
	if err != nil {
		out.Summary = err.Error()
		return out
	}
	var investigation json.RawMessage
	var investigationUnavailable string
	if result.SelectedID != "" {
		frames := [2]tracequery.FrameCatalogEntry{result.Candidates[0].Frame, result.Target}
		var scope *types.RuntimeArtifactScopeProfile
		if ctx != nil && ctx.AnalysisIR != nil {
			scope = ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
		}
		for i, frame := range frames {
			if frame.EndNS <= frame.StartNS {
				investigationUnavailable = "a selected frame does not have a positive-duration window"
				break
			}
			if scope != nil && scope.Comparison != nil {
				side := scope.Comparison.Sides()[i]
				if !scope.ComparisonWindowAllows(side.Path, ctx.RepoRoot, float64(frame.StartNS)/1e9, float64(frame.EndNS)/1e9) {
					investigationUnavailable = "a selected frame extends outside its requested comparison window"
					break
				}
			}
		}
		if investigationUnavailable == "" {
			investigation, err = json.Marshal(map[string]traceMeasurementPairParams{
				"comparison": {Baseline: frameInvestigationSide(inputs[0].Request, frames[0]), Current: frameInvestigationSide(inputs[1].Request, frames[1])},
			})
			if err != nil {
				out.Summary = err.Error()
				return out
			}
		}
	}
	payload, err := json.Marshal(struct {
		tracequery.FrameCatalogMatch
		Investigation json.RawMessage `json:"investigation,omitempty"`
	}{FrameCatalogMatch: result, Investigation: investigation})
	if err != nil {
		out.Summary = err.Error()
		return out
	}
	out.RawRef = StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-frame-selection.json", string(payload))
	var summary strings.Builder
	fmt.Fprintf(&summary, "# Trace Query: frame selection\nstatus=%s selected_baseline_id=%q target_vsync=%d target_tid=%d target_start_ns=%d target_end_ns=%d baseline_considered=%d\n",
		result.Status, result.SelectedID, result.Target.Vsync, result.Target.TID, result.Target.StartNS, result.Target.EndNS, result.Considered)
	for i, candidate := range result.Candidates {
		frame := candidate.Frame
		fmt.Fprintf(&summary, "rank=%d id=%s vsync=%d tid=%d start_ns=%d end_ns=%d score=%.4f window_agrees=%t evidence=%v\n",
			i+1, frame.ID, frame.Vsync, frame.TID, frame.StartNS, frame.EndNS, candidate.Score, candidate.WindowAgrees, candidate.Evidence)
	}
	if len(investigation) != 0 {
		fmt.Fprintf(&summary, "For a requested investigation, call trace_query with this prepared pair of frame windows:\n%s\n", investigation)
	} else if investigationUnavailable != "" {
		fmt.Fprintf(&summary, "No prepared frame investigation: %s.\n", investigationUnavailable)
	}
	out.Success = true
	out.Summary = summary.String()
	return out
}

// The selectors have already been validated by executeMeasurementPair. Carry
// only capture and preparation parameters into the exact-thread frame query.
func frameInvestigationSide(request json.RawMessage, frame tracequery.FrameCatalogEntry) json.RawMessage {
	var source map[string]json.RawMessage
	_ = json.Unmarshal(request, &source)
	side := make(map[string]json.RawMessage)
	for _, name := range []string{"source", "path", "archive_member", "trace_flavor", "platform", "core_topology"} {
		if value, ok := source[name]; ok {
			side[name] = value
		}
	}
	side["view"] = json.RawMessage(`"window_stats"`)
	side["pid"] = json.RawMessage(fmt.Sprint(frame.TID))
	side["time_start"] = json.RawMessage(fmt.Sprintf("%d.%09d", frame.StartNS/1_000_000_000, frame.StartNS%1_000_000_000))
	side["time_end"] = json.RawMessage(fmt.Sprintf("%d.%09d", frame.EndNS/1_000_000_000, frame.EndNS%1_000_000_000))
	encoded, _ := json.Marshal(side)
	return encoded
}

func frameCatalogComparisonThread(out types.ToolResult, vsync uint64) (string, error) {
	if !out.Success {
		return "", fmt.Errorf("frame selection current catalog failed: %s", out.Summary)
	}
	catalog, err := readFrameCatalogResult(out)
	if err != nil {
		return "", err
	}
	focus, err := tracequery.FrameCatalogTarget(catalog, vsync)
	if err != nil {
		return "", err
	}
	if focus.Thread == "" {
		return "", fmt.Errorf("target frame %d has no observed application thread label; provide an exact baseline thread selector", vsync)
	}
	return focus.Thread, nil
}

func readFrameCatalogResult(out types.ToolResult) (tracequery.FrameCatalog, error) {
	var catalog tracequery.FrameCatalog
	data, err := os.ReadFile(out.RawRef)
	if err != nil {
		return catalog, err
	}
	err = json.Unmarshal(data, &catalog)
	return catalog, err
}
