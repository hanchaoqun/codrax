package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type traceMeasurementPairParams struct {
	Baseline         json.RawMessage `json:"baseline"`
	Current          json.RawMessage `json:"current"`
	TargetFrameVsync *uint64         `json:"target_frame_vsync,omitempty"`
}

// Registration is by producer capability, not metric names or question text.
var traceMeasurementPairViews = []string{"window_stats", "cpu_state_frequency", "process_measurements", "preferred_frame_rate", "measurements"}

var traceMeasurementPairFields = []string{"source", "path", "archive_member", "view", "time_start", "time_end", "line_start", "line_end", "pid", "thread", "target_scope", "bucket_ms", "limit", "trace_flavor", "platform", "core_topology"}

func traceQueryMeasurementPairSchema(raw json.RawMessage) json.RawMessage {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return raw
	}
	properties, _ := schema["properties"].(map[string]any)
	sideProperties := map[string]any{}
	descriptions := map[string]string{
		"source": "attached_trace or path; this side is selected independently.", "path": "Exact trace path for this side; normal binary/SQLite preparation applies.",
		"archive_member": traceQueryArchiveMemberTeaching,
		"time_start":     "This side's start in trace seconds.", "time_end": "This side's exclusive end in trace seconds.",
		"line_start": "Optional original input line start.", "line_end": "Optional original input line end.",
		"pid": "Exact target PID/TID when the selected native view permits it.", "thread": "Thread selector when the selected native view permits it.",
		"target_scope": "Explicit target identity scope; raw measurements has no thread/process target.", "bucket_ms": "Time-bucket width in milliseconds for views which support it.",
		"limit": "Retained output limit; does not turn a partial population into complete coverage.", "trace_flavor": "Optional producer flavor for this side.",
		"platform": "Optional platform semantics for this side, not proof of device identity.", "core_topology": "Optional explicit CPU class map for this side, not proof of matching hardware.",
	}
	for _, name := range traceMeasurementPairFields {
		if value, ok := properties[name]; ok {
			encoded, _ := json.Marshal(value)
			var property map[string]any
			_ = json.Unmarshal(encoded, &property)
			property["description"] = descriptions[name]
			sideProperties[name] = property
		}
	}
	views := append(append([]string(nil), traceMeasurementPairViews...), "frame_catalog")
	sideProperties["view"] = map[string]any{"type": "string", "enum": views, "description": "Native measurement producer view; each side retains its own population, units and interval policy. For target_frame_vsync, both sides use frame_catalog."}
	side := map[string]any{"type": "object", "properties": sideProperties, "required": []string{"source", "view"}, "additionalProperties": false}
	properties["comparison"] = map[string]any{"type": "object", "properties": map[string]any{"baseline": side, "current": side,
		"target_frame_vsync": map[string]any{"type": "integer", "minimum": 0, "description": "Select this frame number from the current capture only and rank locally enumerated baseline frames. Both sides use frame_catalog with their own operation scope. If the baseline thread is omitted, search its catalog on the exact application thread label discovered on the current capture."}}, "additionalProperties": false,
		"description": "Query two captures independently in one call. Each side uses ordinary source/path/view/window selectors and the same automatic binary preparation. In frame selection mode, use target_frame_vsync and two frame_catalog sides to acquire and rank a reference from the original traces. Otherwise this publishes separate native tables and a two-side status table, not differences, clock alignment or causality. Omit an unknown measurement side rather than invent a path; failed or absent sides retain their own status. Do not combine comparison with outer single-source fields."}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(schema) != nil {
		return raw
	}
	return out.Bytes()
}

func (t *TraceQuery) executeMeasurementPair(ctx *types.BusContext, raw json.RawMessage, pair *traceMeasurementPairParams) (types.ToolResult, error) {
	now := time.Now()
	var outer map[string]json.RawMessage
	if json.Unmarshal(raw, &outer) != nil || len(outer) != 1 || pair == nil || len(pair.Baseline)+len(pair.Current) == 0 {
		return types.ToolResult{ToolName: t.Name(), Timestamp: now, Summary: "comparison must contain at least one side and cannot mix outer single-source selectors", Repair: &types.ToolRepair{Code: "trace_measurement_pair_shape", Fields: []string{"comparison"}}}, nil
	}
	if pair.TargetFrameVsync != nil && (len(pair.Baseline) == 0 || len(pair.Current) == 0) {
		return types.ToolResult{ToolName: t.Name(), Timestamp: now, Summary: "frame selection requires both original capture sides"}, nil
	}
	var inputs [2]types.NativeMeasurementPairSide
	sides := [2]json.RawMessage{pair.Baseline, pair.Current}
	order := [2]int{0, 1}
	if pair.TargetFrameVsync != nil {
		order = [2]int{1, 0}
	}
	for _, i := range order {
		request := sides[i]
		inputs[i].Request = request
		if len(request) == 0 {
			continue
		}
		var child traceQueryParams
		decoder := json.NewDecoder(strings.NewReader(string(request)))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&child)
		allowed := false
		for _, view := range traceMeasurementPairViews {
			if child.View == view {
				allowed = true
			}
		}
		if pair.TargetFrameVsync != nil {
			allowed = child.View == "frame_catalog"
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(request, &fields) != nil || fields == nil {
			allowed = false
		}
		for field := range fields {
			found := false
			for _, known := range traceMeasurementPairFields {
				if field == known {
					found = true
					break
				}
			}
			if !found {
				allowed = false
			}
		}
		if err != nil || child.Comparison != nil || !allowed || child.Source != "path" && child.Source != "attached_trace" {
			inputs[i].Result = types.ToolResult{ToolName: t.Name(), Summary: fmt.Sprintf("invalid native measurement side: view=%q; decode=%v", child.View, err)}
			continue
		}
		if pair.TargetFrameVsync != nil && i == 0 && child.PID.Int() <= 0 && strings.TrimSpace(child.Thread) == "" {
			thread, err := frameCatalogComparisonThread(inputs[1].Result, *pair.TargetFrameVsync)
			if err != nil {
				return types.ToolResult{ToolName: t.Name(), Timestamp: now, Summary: err.Error()}, nil
			}
			child.Thread = thread
			fields["thread"], _ = json.Marshal(thread)
			request, _ = json.Marshal(fields)
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			scope := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
			if scope != nil && scope.Comparison != nil {
				sides := scope.Comparison.Sides()
				if !scope.Comparison.Valid() || child.Source != "path" || !types.RuntimeArtifactComparisonPathEqual(sides[i].Path, child.Path, ctx.RepoRoot) {
					inputs[i].Result = types.ToolResult{ToolName: t.Name(), Summary: "This comparison role does not match its original requested source; use the accepted per-source comparison binding."}
					continue
				}
				// Keep the request-owned spelling in the report, while routing
				// ordinary preparation through its equivalent local spelling.
				child.Path = types.RuntimeArtifactComparisonLocalPath(child.Path, ctx.RepoRoot)
				fields["path"], _ = json.Marshal(child.Path)
				request, _ = json.Marshal(fields)
			}
		}
		if err := contextFromBus(ctx).Err(); err != nil {
			inputs[i].Result = traceQueryCancellationResult(child.View, child.Path, err)
			continue
		}
		sideCtx := ctx
		if logging.IsDebug() && ctx != nil {
			side := *ctx
			side.Ctx = logging.TraceTimingRole(contextFromBus(ctx), [...]string{"baseline", "current"}[i])
			sideCtx = &side
		}
		childTool := *t
		if child.View == "window_stats" && !t.skipNativeMeasurementMemo {
			selector, _ := json.Marshal(fields)
			childTool.pairWindowStatsSelector = [...]string{"baseline", "current"}[i] + "\x00" + string(selector)
		}
		result, err := childTool.Execute(sideCtx, request)
		if err == nil && result.Success && result.ReusedFromRunMemo && result.TraceViewCancellation == nil && contextFromBus(ctx).Err() == nil {
			// Pure results are memoized before Execute's source/replay stamps.
			// A comparison needs independently current read credentials, not
			// a new permission minted from cached values. Re-read once locally;
			// leave ordinary query memo behavior and all failure paths intact.
			fresh := *t
			fresh.skipNativeMeasurementMemo = true
			result, err = fresh.Execute(sideCtx, request)
		}
		if err != nil && result.Summary == "" {
			result.Summary = err.Error()
		}
		inputs[i].Result = result
		inputs[i].Material = traceMeasurementPairMaterial(ctx, result)
	}
	if pair.TargetFrameVsync != nil {
		return t.finishFrameCatalogMatch(ctx, inputs, *pair.TargetFrameVsync), nil
	}
	var owner *types.MutableState
	if ctx != nil {
		owner = ctx.Mutable
	}
	receipt := types.NewNativeMeasurementPair(owner, inputs)
	report, _ := receipt.Report()
	publishStarted := time.Now()
	encoded, _ := json.MarshalIndent(report, "", "  ")
	ref := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-measurement-pair.json", string(encoded))
	logging.TraceTiming(contextFromBus(ctx), "phase=pair_publish baseline=%q current=%q bytes=%d elapsed=%s",
		report.Sides[0].Status, report.Sides[1].Status, len(encoded), time.Since(publishStarted))
	var summary strings.Builder
	fmt.Fprintf(&summary, "Independent native measurements: baseline=%s; current=%s. %s\n", report.Sides[0].Status, report.Sides[1].Status, report.Boundary)
	for i, side := range report.Sides {
		if side.Status != "available" {
			continue
		}
		for j, publication := range side.Publications {
			if side.ProducerStates[j].Predicate != TraceThreadTimelinePredicate {
				continue
			}
			for _, table := range publication.Tables {
				if table.View == types.RuntimeMeasurementTimeline {
					fmt.Fprintf(&summary, "%s: retained thread timeline rows=%d observation_id=%s view=%s\n", side.Role, len(table.Rows), publication.ObservationID, table.View)
				}
			}
		}
		for _, observation := range inputs[i].Result.Observations {
			if observation.Predicate == "target_window_states" {
				fmt.Fprintf(&summary, "%s: %s evidence_lines=%d-%d\n", side.Role, observation.Summary, observation.Span.LineStart, observation.Span.LineEnd)
				break
			}
		}
	}
	if ref != "" {
		fmt.Fprintf(&summary, "Full report: %s\nRead the existing report by observation_id and line range for details from these same selectors.", ref)
	}
	// Composition succeeded even if a side failed. The explicit side statuses,
	// not this wrapper transport flag, report acquisition success.
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: summary.String(), RawRef: ref, Timestamp: now, RuntimeMeasurementPair: receipt}, nil
}

func traceMeasurementPairMaterial(ctx *types.BusContext, result types.ToolResult) *attachment.TraceMaterial {
	if ctx == nil {
		return nil
	}
	var materials []*attachment.TraceMaterial
	if ctx.AttachedTraceMaterial != nil {
		materials = append(materials, ctx.AttachedTraceMaterial)
	}
	if ctx.TraceInputPreparer != nil {
		materials = append(materials, ctx.TraceInputPreparer.PreparedMaterials()...)
	}
	for _, observation := range result.Observations {
		publication, valid := types.DecodeRuntimeMeasurementPublication(observation)
		if !valid {
			continue
		}
		for _, material := range materials {
			// A native table can retain an admitted bundle child's exact source.
			// This receipt check is display ownership, never a query-path alias.
			if material != nil && material.OwnsQueryArtifact(publication.Source.Path) && material.Validate(contextFromBus(ctx), material.Preview()) == nil {
				return material
			}
		}
	}
	return nil
}
