package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func authorizePendingTraceCatalog(ctx *types.BusContext, c *tracecatalog.Catalog) error {
	for _, current := range ctx.Mutable.TraceCatalogs() {
		if c.SameLiveLineage(current) {
			return authorizeTraceCatalogExport(ctx, c)
		}
	}
	return fmt.Errorf("accepted export catalog no longer belongs to the current live run")
}

// AuthorizedPendingTraceCatalogExports is status only; disk/model JSON does
// not install an intent and a pending request has no physical success/path.
func AuthorizedPendingTraceCatalogExports(ctx *types.BusContext) ([]tracecatalog.ExportRequest, error) {
	if ctx == nil || ctx.Mutable == nil || ctx.PipelineExports == nil {
		return nil, nil
	}
	return ctx.PipelineExports.Requests(contextFromBus(ctx), ctx.DeliveryRoot, func(c *tracecatalog.Catalog) error {
		return authorizePendingTraceCatalog(ctx, c)
	})
}

// MaterializePendingTraceCatalogExports is called only by the terminal agent
// adapter. It is intentionally not registered as a model-callable tool.
func MaterializePendingTraceCatalogExports(ctx *types.BusContext) ([]tracecatalog.ExportRequest, error) {
	if ctx == nil || ctx.Mutable == nil || ctx.PipelineExports == nil {
		return nil, nil
	}
	return ctx.PipelineExports.Materialize(contextFromBus(ctx), ctx.DeliveryRoot, func(c *tracecatalog.Catalog) error {
		return authorizePendingTraceCatalog(ctx, c)
	})
}
