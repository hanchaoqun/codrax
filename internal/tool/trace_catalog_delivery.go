package tool

import (
	"errors"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func authorizeTraceCatalogExport(ctx *types.BusContext, c *tracecatalog.Catalog) error {
	snapshot := c.Snapshot()
	if _, err := traceCatalogAuthorizedRoot(ctx, snapshot.Root); err != nil {
		return err
	}
	// Reauthorize every immutable member, not a saved path or a model receipt.
	for _, a := range snapshot.Artifacts {
		if _, err := traceCatalogAuthorizedRoot(ctx, a.Path); err != nil {
			return fmt.Errorf("member %s: %w", a.ID, err)
		}
	}
	return nil
}

// AuthorizedTraceCatalogDeliveries is a read-only projection of private live
// publications. It grants no query, evidence, diagnosis or write authority.
// Both the current host source scope and every package file are checked again
// at consumption. Other tool results, disk JSON and model text cannot add rows.
func AuthorizedTraceCatalogDeliveries(ctx *types.BusContext) ([]tracecatalog.ExportDelivery, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	var out []tracecatalog.ExportDelivery
	var failures []error
	if ctx.DeliveryRoot == nil {
		return nil, nil
	}
	outputRoot, err := ctx.DeliveryRoot.Directory(contextFromBus(ctx), false)
	if err != nil || outputRoot == "" {
		return nil, err
	}
	for _, c := range ctx.Mutable.TraceCatalogs() {
		if err := authorizeTraceCatalogExport(ctx, c); err != nil {
			failures = append(failures, err)
			continue
		}
		deliveries, err := c.ExportDeliveries(contextFromBus(ctx), outputRoot)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		out = append(out, deliveries...)
	}
	return out, errors.Join(failures...)
}
