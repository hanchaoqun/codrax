package tool

import (
	"encoding/json"
	"errors"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceCatalogExportTeaching = `For an explicitly requested evidence package, use action=export with current catalog_id and the requested export.target, e.g. {"action":"export","catalog_id":"<id>","export":{"target":"capture-evidence-v1"}}. Complete the intended current queries before export. It publishes manifest.json, an execution report.md and bound result copies under persistent per-run managed delivery, independent of temporary query storage; live branches share the same target and identical verified receipt. Repeating an unchanged export reuses that receipt, not query execution. A changed execution returns the frozen delivery and its mismatch; do not invent another target to hide this conflict. Export does not run missing queries, restore historical access, infer native availability, or diagnose the results.`

type traceCatalogExportParams struct {
	Target string `json:"target"`
}

func traceCatalogExport(ctx *types.BusContext, p traceCatalogParams, out types.ToolResult) (types.ToolResult, error) {
	if p.Action != "export" || p.Export == nil || p.CatalogID == "" || p.Root != "" || p.Recursive != nil || len(p.Queries) > 0 || p.SnapshotPath != "" || p.Representatives != nil || p.Offset != 0 {
		out.Summary = "export requires only action=export, current catalog_id and export.target; it cannot be combined with discovery, resume, selection or pagination"
		return out, nil
	}
	var current *tracecatalog.Catalog
	for _, c := range ctx.Mutable.TraceCatalogs() {
		if c.ID() == p.CatalogID {
			current = c
			break
		}
	}
	if current == nil {
		out.Summary = "export needs a current live catalog_id; historical JSON does not restore export or source access"
		return out, nil
	}
	if err := authorizeTraceCatalogExport(ctx, current); err != nil {
		out.Summary = "export source authorization: " + err.Error()
		return out, nil
	}
	outputRoot, err := ctx.DeliveryRoot.Directory(contextFromBus(ctx), true)
	if err != nil {
		out.Summary = "Evidence-package host destination unavailable: " + err.Error()
		return out, nil
	}
	receipt, err := current.Export(contextFromBus(ctx), outputRoot, p.Export.Target)
	if err != nil {
		var conflict *tracecatalog.ExportConflictError
		if errors.As(err, &conflict) {
			data, _ := json.Marshal(conflict.Delivery)
			out.Summary = "Existing frozen delivery is not current completion; no new package was published: " + string(data)
			return out, nil
		}
		out.Summary = "Evidence-package delivery could not be verified: " + err.Error()
		return out, nil
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		out.Summary = "Evidence package receipt encoding failed: " + err.Error()
		return out, nil
	}
	out.Success = true
	out.Summary = string(data)
	return out, nil
}
