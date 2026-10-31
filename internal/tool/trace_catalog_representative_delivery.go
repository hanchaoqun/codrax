package tool

import (
	"context"
	"errors"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// MaterializeTraceCatalogRepresentativeDeliveries is a deterministic terminal
// adapter, deliberately not a registered/model-callable tool. Plain catalogs
// and historical/resumed navigation without a new live selection are no-ops.
func MaterializeTraceCatalogRepresentativeDeliveries(ctx *types.BusContext) []tracecatalog.RepresentativeDelivery {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	var rows []tracecatalog.RepresentativeDelivery
	for _, catalog := range ctx.Mutable.TraceCatalogs() {
		selection, selected := catalog.Representatives()
		if !selected {
			continue
		}
		row := tracecatalog.RepresentativeDelivery{Kind: "trace_representative_delivery/v1", Status: "failed", CatalogID: catalog.ID(), SelectionID: selection.SelectionID}
		publish := func() (tracecatalog.Publication, error) {
			if err := contextFromBus(ctx).Err(); err != nil {
				return tracecatalog.Publication{}, err
			}
			if err := authorizeTraceCatalogExport(ctx, catalog); err != nil {
				return tracecatalog.Publication{}, err
			}
			path, err := traceCatalogSavePath(ctx, catalog)
			if err != nil {
				return tracecatalog.Publication{}, err
			}
			return catalog.SaveRepresentativePublication(contextFromBus(ctx), path, func(view *tracecatalog.Catalog) error { return authorizeTraceCatalogExport(ctx, view) })
		}
		p, err := publish()
		if err != nil {
			row.Reason = err.Error()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				row.Status = "cancelled"
			}
		} else {
			row.Status = "published"
			row.SelectionID = p.Execution.Selection.SelectionID
			row.IndexPath, row.IndexSHA256 = p.IndexPath, p.IndexSHA256
			row.SelectionPath, row.SelectionSHA256 = p.SelectionPath, p.SelectionSHA256
			row.ExecutionCoverage = &p.Execution.Coverage
			row.RepresentativeCoverage = &p.Execution.Selection.Coverage
		}
		rows = append(rows, row)
	}
	return rows
}
