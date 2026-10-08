package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceCatalogQueryPlan(p traceQueryParams, raw json.RawMessage, artifactID string) (tracecatalog.Plan, error) {
	if err := tracequery.ValidateViewName(p.View); err != nil {
		return tracecatalog.Plan{}, err
	}
	var args map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&args); err != nil {
		return tracecatalog.Plan{}, err
	}
	delete(args, "source")
	delete(args, "path")
	args["view"] = tracequery.CanonicalViewName(p.View)
	object := tracecatalog.Object{Kind: "capture", ID: "all"}
	if p.PID.Int() > 0 {
		object = tracecatalog.Object{Kind: "thread", ID: strconv.Itoa(p.PID.Int())}
		if p.TargetScope == tracequery.TargetScopeProcess {
			object.Kind = "process"
		}
		args["pid"] = p.PID.Int()
	} else if p.Thread != "" {
		object = tracecatalog.Object{Kind: "thread_selector", ID: p.Thread}
	}
	if p.TimeStart.Set() {
		args["time_start"] = p.TimeStart.Seconds()
	}
	if p.TimeEnd.Set() {
		args["time_end"] = p.TimeEnd.Seconds()
	}
	parameters, err := json.Marshal(args)
	if err != nil {
		return tracecatalog.Plan{}, err
	}
	if len(parameters) > 16<<10 {
		return tracecatalog.Plan{}, fmt.Errorf("planned query exceeds 16 KiB")
	}
	plan := tracecatalog.Plan{ArtifactID: artifactID, Object: object, View: tracequery.CanonicalViewName(p.View), Parameters: parameters}
	if p.TimeStart.Set() && p.TimeEnd.Set() {
		start, ok1 := traceCatalogExactNS(p.TimeStart.Seconds())
		end, ok2 := traceCatalogExactNS(p.TimeEnd.Seconds())
		if ok1 && ok2 && end > start {
			plan.Window = &tracecatalog.Window{StartNS: start, EndNS: end}
		}
	}
	return plan, nil
}

func traceCatalogExactNS(seconds float64) (int64, bool) {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(seconds, 'g', -1, 64))
	if !ok {
		return 0, false
	}
	r.Mul(r, big.NewRat(1_000_000_000, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, false
	}
	return r.Num().Int64(), true
}

type traceCatalogQueryTicket struct {
	catalog     *tracecatalog.Catalog
	artifact    tracecatalog.Artifact
	queryID     string
	raw         json.RawMessage
	predeclared bool
	revision    string
}

// Called on an already decoded explicit path, before preparation. Failed
// preparation is therefore retained instead of vanishing from the roster.
func traceCatalogBeginQuery(ctx *types.BusContext, p traceQueryParams, raw json.RawMessage) []traceCatalogQueryTicket {
	if ctx == nil || ctx.Mutable == nil || p.Path == "" || p.Source == "attached_trace" {
		return nil
	}
	path := resolveToolPath(ctx, p.Path)
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		path = canonical
	}
	var tickets []traceCatalogQueryTicket
	for _, c := range ctx.Mutable.TraceCatalogs() {
		a, ok := c.ArtifactForPath(path)
		if !ok {
			continue
		}
		plan, err := traceCatalogQueryPlan(p, raw, a.ID)
		if err != nil {
			continue
		}
		q, err := c.RegisterPlannedQuery(contextFromBus(ctx), plan)
		if err != nil {
			continue
		}
		tickets = append(tickets, traceCatalogQueryTicket{catalog: c, artifact: a, queryID: q.ID, raw: append(json.RawMessage(nil), raw...), predeclared: c.IsDeclaredQuery(q.ID)})
	}
	return tickets
}

func traceCatalogBindQuery(ctx *types.BusContext, tickets []traceCatalogQueryTicket, p traceQueryParams, path string, material *attachment.TraceMaterial) error {
	if len(tickets) == 0 {
		return nil
	}
	v, err := tracequery.CaptureTraceSourceVersionContext(contextFromBus(ctx), path)
	if err != nil {
		return err
	}
	validate := func(c context.Context) error {
		if material != nil {
			if err := material.Validate(c, material.Preview()); err != nil {
				return err
			}
		}
		return v.ValidateContext(c, path)
	}
	for i := range tickets {
		t := &tickets[i]
		if err := t.catalog.BindSource(contextFromBus(ctx), t.artifact.ID, v.Fingerprint(), validate); err != nil {
			return err
		}
		plan, err := traceCatalogQueryPlan(p, t.raw, t.artifact.ID)
		if err != nil {
			return err
		}
		q, err := t.catalog.RegisterPlannedQuery(contextFromBus(ctx), plan)
		if err != nil {
			return err
		}
		t.queryID = q.ID
		t.revision = v.Fingerprint()
	}
	return nil
}

func traceCatalogFinishQuery(ctx *types.BusContext, tickets []traceCatalogQueryTicket, out *types.ToolResult, executeErr error) {
	if out == nil {
		return
	}
	for _, t := range tickets {
		completion := tracecatalog.Completion{Outcome: tracecatalog.OutcomeSuccess, SourceRevision: t.revision, RawRef: out.RawRef}
		if out.EnumerationAuthority != nil {
			completion.Truncated = out.EnumerationAuthority.Status == "incomplete"
		}
		for _, record := range out.Observations {
			if completion.PayloadRef == "" {
				completion.PayloadRef = record.SourceRef.PayloadRef
			}
			if stack, ok := DecodeTraceResourceStack(record); ok && stack.MatchedEvents == 0 && stack.Status == "available" {
				completion.Outcome = tracecatalog.OutcomeEmpty
			}
		}
		if out.TraceViewCancellation != nil || errors.Is(executeErr, context.Canceled) || contextFromBus(ctx).Err() != nil {
			completion.Outcome = tracecatalog.OutcomeCanceled
			completion.Error = &tracecatalog.Issue{Code: "query_canceled", Path: t.artifact.Path, Message: "Query was canceled; available partial results do not prove completion."}
		}
		if !out.Success || executeErr != nil {
			if completion.Outcome != tracecatalog.OutcomeCanceled {
				completion.Outcome = tracecatalog.OutcomeFailure
			}
			code := "query_failed"
			if out.Repair != nil && out.Repair.Code != "" {
				code = out.Repair.Code
			}
			completion.Error = &tracecatalog.Issue{Code: code, Path: t.artifact.Path, Message: out.Summary}
		}
		_, currentErr := t.catalog.Resolve(contextFromBus(ctx), t.artifact.ID)
		if currentErr != nil && contextFromBus(ctx).Err() == nil {
			completion.Outcome = tracecatalog.OutcomeStale
			completion.Error = &tracecatalog.Issue{Code: "source_changed", Path: t.artifact.Path, Message: currentErr.Error()}
		}
		if _, err := t.catalog.Complete(contextFromBus(ctx), t.queryID, completion); err != nil {
			out.Summary += "\nCapture index update failed: " + err.Error()
		} else if currentErr == nil && t.predeclared && !out.Success && out.Repair != nil && out.Repair.Metadata["stage"] == "trace_input_admission" && out.Repair.Metadata["status"] == types.ToolRepairStatusActionRequired &&
			len(t.catalog.Snapshot().Artifacts) > 1 && !strings.HasSuffix(strings.ToLower(t.artifact.Path), ".tracebundle.json") {
			out.TraceCatalogIndependentFailure = true
		}
		if _, err := saveTraceCatalog(ctx, t.catalog); err != nil {
			out.Summary += "\nCapture index persistence failed: " + err.Error()
		}
	}
}
