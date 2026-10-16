package tool

import (
	"encoding/json"
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceCatalogExportTeaching = `For an explicitly requested evidence package, use action=export with current catalog_id and export.target set to a new package name, e.g. {"action":"export","catalog_id":"<id>","export":{"target":"capture-evidence-v1"}}. It publishes manifest.json, report.md and current bound result copies under managed output only. Export does not run missing queries, restore historical access, infer native availability, or diagnose the results.`

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
	if _, err := traceCatalogAuthorizedRoot(ctx, current.Snapshot().Root); err != nil {
		out.Summary = "export source authorization: " + err.Error()
		return out, nil
	}
	// Reauthorize each exact current member as well as the root. A catalog cannot
	// retain a revoked repository/host pin, and a saved path grants nothing.
	for _, a := range current.Snapshot().Artifacts {
		if _, err := traceCatalogAuthorizedRoot(ctx, a.Path); err != nil {
			out.Summary = fmt.Sprintf("export member authorization %s: %v", a.ID, err)
			return out, nil
		}
	}
	receipt, err := current.Export(contextFromBus(ctx), ctxWorkDir(ctx), p.Export.Target)
	if err != nil {
		out.Summary = "Evidence package was not published: " + err.Error()
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
