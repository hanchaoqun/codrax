package tool

import "github.com/hanchaoqun/codrax/internal/tracecatalog"

func traceCatalogLivePage(c *tracecatalog.Catalog, ref string, offset int) (string, error) {
	execution := c.CurrentExecution()
	return traceCatalogPage(execution.Snapshot, ref, offset, execution)
}

func traceCatalogExecutionRecord(q tracecatalog.QueryRecord, origin tracecatalog.QueryOrigin) any {
	kind := "current_query_plan"
	if origin == tracecatalog.QueryOriginObserved {
		kind = "observed_query"
	}
	return struct {
		Kind        string                   `json:"record_kind"`
		QueryOrigin tracecatalog.QueryOrigin `json:"query_origin"`
		tracecatalog.QueryRecord
	}{kind, origin, q}
}
