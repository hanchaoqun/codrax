package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceCatalogResumeTeaching = "For an interrupted saved roster, call trace_catalog with action=resume, snapshot_path and the explicitly authorized same root. Only historical members are re-admitted; every current query starts pending, including historical successes. Use each returned query_calls JSON with trace_query to produce fresh facts. Failed/canceled attempts stay in history and can be retried. History restores neither source permission nor declared-window authority."

func traceCatalogRevalidatePlan(raw json.RawMessage, artifactID string) (tracecatalog.Plan, error) {
	schema := (&TraceQuery{}).Parameters()
	if err := ToolArgumentEnvelopeIntegrityError("trace_query", raw, schema); err != nil {
		return tracecatalog.Plan{}, err
	}
	if err := toolparam.Validate(raw, schema); err != nil {
		return tracecatalog.Plan{}, err
	}
	var p traceQueryParams
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return tracecatalog.Plan{}, err
	}
	if p.Source != "" || p.Path != "" || p.BusinessSpanRef != "" || p.Comparison != nil {
		return tracecatalog.Plan{}, fmt.Errorf("historical plan must omit source/path/comparison and capture-local references")
	}
	return traceCatalogQueryPlan(p, raw, artifactID)
}

func traceCatalogResume(ctx *types.BusContext, p traceCatalogParams) (*tracecatalog.Catalog, error) {
	if strings.TrimSpace(p.Root) == "" || p.SnapshotPath == "" || p.CatalogID != "" || p.Recursive != nil || len(p.Queries) > 0 {
		return nil, fmt.Errorf("resume requires explicit root and snapshot_path, without catalog_id/recursive/queries")
	}
	root, err := traceCatalogAuthorizedRoot(ctx, p.Root)
	if err != nil {
		return nil, err
	}
	path, err := traceCatalogAuthorizedRoot(ctx, p.SnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot authorization: %w", err)
	}
	old, err := tracecatalog.Load(contextFromBus(ctx), path)
	if err != nil {
		return nil, err
	}
	plans := make([]tracecatalog.Plan, len(old.Queries))
	for i, q := range old.Queries {
		plans[i], err = traceCatalogRevalidatePlan(q.Parameters, q.ArtifactID)
		if err != nil {
			return nil, fmt.Errorf("historical query %s is not valid under the current tool schema: %w", q.ID, err)
		}
		if plans[i].View != q.View || plans[i].Object != q.Object || !reflect.DeepEqual(plans[i].Window, q.Window) {
			return nil, fmt.Errorf("historical query %s has inconsistent parallel object/view/window metadata", q.ID)
		}
	}
	c, err := tracecatalog.Resume(contextFromBus(ctx), root, old, plans)
	if err != nil {
		return nil, err
	}
	return ctx.Mutable.InstallTraceCatalog(c)
}

// Cancellation may stop computation, not erase a completed navigation update.
// This bounded cleanup writes only catalog JSON; it does not resolve/read any
// source or run tools after cancellation.
func saveTraceCatalogAfterAttempt(ctx *types.BusContext, c *tracecatalog.Catalog) (string, error) {
	if contextFromBus(ctx).Err() == nil {
		return saveTraceCatalog(ctx, c)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(contextFromBus(ctx)), 2*time.Second)
	defer cancel()
	copy := ctx.ShallowClone()
	copy.Ctx = cleanup
	return saveTraceCatalog(copy, c)
}
