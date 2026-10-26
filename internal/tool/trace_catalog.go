package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceCatalog records navigation and execution coverage, not measurements.
// All actual analysis continues through TraceQuery and its normal admission.
type TraceCatalog struct {
	ReadOnly
	NonEvidenceTool
}

func (*TraceCatalog) Name() string { return "trace_catalog" }
func (*TraceCatalog) Description() string {
	return "Discover candidate capture files across a selected directory and keep a per-capture, per-object query index. Recursive discovery preserves same-named files in different directories. Optional queries are ordinary trace_query arguments without source/path: they register expected work for each candidate but do not execute it. For an accepted artifact work request, explicit request_plan.view reuses its typed targets, common window and per-specification budget; missing inputs remain pending. For directory work with one explicit request window, put that window in the expected calls once; after admission, queries of those same members can inherit it when all their declared plans agree. Discovery alone does not bind a request window. Continue with trace_query using the returned paths; status shows results, empty results, failures and unexecuted work separately. Candidates and saved indexes are navigation only, not measurements, input admission or causal evidence. " + traceCatalogResumeTeaching + " " + traceCatalogRepresentativesTeaching + " " + traceCatalogExportTeaching
}
func (*TraceCatalog) Parameters() json.RawMessage {
	return traceCatalogPlanSchema(json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
	"action":{"type":"string","enum":["discover","status","resume","representatives","representative_status","export"],"description":"Default discover; status reads a current catalog; resume re-admits a historical roster. representatives freezes soft-ranked representatives of exact query-specification groups; representative_status pages that fixed selection with current execution states. export archives current native references to a new explicit managed package target without running queries."},
	"export":{"type":"object","additionalProperties":false,"required":["target"],"properties":{"target":{"type":"string","minLength":1,"maxLength":120,"description":"Explicit new package name, not a path. Published below the host-managed evidence-exports directory; no overwrite or customer source/report mutation."}},"description":"Only action=export with current catalog_id. Package manifest and report separate execution coverage, material copying, and unknown native availability; export is not analysis completion."},
	"representatives":{"type":"object","additionalProperties":false,"properties":{"per_cluster":{"type":"integer","minimum":1,"maximum":5,"description":"Default 1; resource limit, not statistical coverage."},"max_clusters":{"type":"integer","minimum":1,"maximum":50,"description":"Default 10; resource limit. Other groups remain listed as budget-skipped."},"priority":{"type":"string","enum":["unexecuted_first","retry_first","largest_input_first"],"description":"Default unexecuted_first. Navigation metadata only, not severity; unknown sizes are not zero. Exact query IDs break ties."}},"description":"Explicit selection request: with discover and nonempty queries, select only the exact plans registered by that call, excluding prior exploratory plans. With action=representatives, select from the entire current catalog. Neither executes nor prohibits any member; a new explicit selection replaces the previous run-local selection."},
	"snapshot_path":{"type":"string","description":"Resume only: exact currently authorized historical navigation JSON. This file grants no access to its members or parent directory."},
	"root":{"type":"string","description":"Selected capture directory (default .), inside the active repository or a host-pinned path."},
	"recursive":{"type":"boolean","description":"Default true. Symbolic links are not followed; skipped and incomplete traversal are disclosed."},
	"request_plan":{"type":"object","additionalProperties":false,"properties":{"view":{"type":"string","description":"Explicit ordinary trace_query view chosen for the task. Never inferred from the work request, target, answer format or prior result. Omitted view leaves the request pending."}},"description":"Discover-only opt-in: compile the accepted artifact work request's user targets and single common time window into exact plans. Representative mode uses its per-specification budget; exhaustive mode registers all plans without selection. Mutually exclusive with queries and representatives. Missing/unsupported typed inputs leave state unchanged; this does not execute queries or grant source access."},
	"queries":{"type":"array","maxItems":16,"items":{"type":"object"},"description":"Optional expected trace_query calls for every candidate. Reuse that tool's argument contract, omit source/path. No execution or evidence is produced by planning."},
	"catalog_id":{"type":"string","description":"Exact ID returned by discover or resume; required for status."},
	"offset":{"type":"integer","minimum":0,"description":"Navigation page offset, default 0."}
	}}`))
}

type traceCatalogParams struct {
	Action          string                              `json:"action"`
	Root            string                              `json:"root"`
	Recursive       *bool                               `json:"recursive"`
	Queries         []json.RawMessage                   `json:"queries"`
	CatalogID       string                              `json:"catalog_id"`
	Offset          int                                 `json:"offset"`
	SnapshotPath    string                              `json:"snapshot_path"`
	Representatives *tracecatalog.RepresentativeOptions `json:"representatives"`
	Export          *traceCatalogExportParams           `json:"export"`
	RequestPlan     *traceCatalogRequestPlanParams      `json:"request_plan"`
}

func (t *TraceCatalog) Execute(ctx *types.BusContext, raw json.RawMessage) (types.ToolResult, error) {
	out := types.ToolResult{ToolName: t.Name(), Timestamp: time.Now()}
	var p traceCatalogParams
	if _, failure, err := decodeStrictToolParams(t.Name(), raw, t.Parameters(), &p, nil); failure != nil {
		return *failure, err
	}
	if ctx == nil || ctx.Mutable == nil {
		out.Summary = "trace_catalog needs current run state"
		return out, nil
	}
	if p.RequestPlan != nil {
		if (p.Action != "" && p.Action != "discover") || len(p.Queries) != 0 || p.Representatives != nil || p.Export != nil || p.CatalogID != "" || p.SnapshotPath != "" || p.Offset != 0 {
			out.Summary = "request_plan requires discover with no queries, representatives, catalog_id, snapshot_path, export or nonzero offset"
			return out, nil
		}
		// Missing task fields cannot turn an unauthorized traversal into a
		// successful navigation response. The ordinary discovery check repeats
		// immediately before filesystem access after complete plan validation.
		if _, err := traceCatalogAuthorizedRoot(ctx, p.Root); err != nil {
			out.Summary, out.Repair = err.Error(), traceCatalogScopeRepair(err)
			return out, nil
		}
		if err := contextFromBus(ctx).Err(); err != nil {
			out.Summary = err.Error()
			return out, nil
		}
		proposal, err := traceCatalogRequestPlan(ctx.Mutable.RequestModel(), p.RequestPlan.View)
		if err != nil {
			out.Summary = err.Error()
			return out, nil
		}
		if proposal.Status != "ready" {
			encoded, _ := json.Marshal(proposal)
			out.Success, out.Summary = true, string(encoded)
			return out, nil
		}
		for _, query := range proposal.Queries {
			encoded, err := json.Marshal(query)
			if err != nil {
				out.Summary = err.Error()
				return out, nil
			}
			p.Queries = append(p.Queries, encoded)
		}
		p.Representatives = proposal.Representatives
		// Re-enter the existing public plan validation with the complete typed
		// expansion, rather than creating a second catalog/selection executor.
		p.RequestPlan = nil
		raw, _ = json.Marshal(struct {
			Action          string                              `json:"action,omitempty"`
			Root            string                              `json:"root,omitempty"`
			Recursive       *bool                               `json:"recursive,omitempty"`
			Queries         []json.RawMessage                   `json:"queries"`
			Representatives *tracecatalog.RepresentativeOptions `json:"representatives,omitempty"`
		}{p.Action, p.Root, p.Recursive, p.Queries, p.Representatives})
	}
	if p.Export != nil || p.Action == "export" {
		return traceCatalogExport(ctx, p, out)
	}
	discoverSelection := p.Representatives != nil && (p.Action == "" || p.Action == "discover")
	if p.Representatives != nil && p.Action != "representatives" && !discoverSelection {
		out.Summary = "representatives configuration requires action=representatives or discover with explicit queries"
		return out, nil
	}
	if discoverSelection && (len(p.Queries) == 0 || p.Offset != 0) {
		out.Summary = "discover with representatives requires nonempty explicit queries and offset 0; use representative_status to page the resulting selection"
		return out, nil
	}
	if discoverSelection {
		// This explicit combined action has no legacy default-query contract.
		// Validate its published plan/option schema before changing the catalog;
		// never silently replace an omitted view or invalid selection budget.
		if err := toolparam.Validate(raw, t.Parameters()); err != nil {
			out.Summary = "discover representative selection requires schema-valid explicit query specifications and budget: " + err.Error()
			return out, nil
		}
	}
	if p.Offset < 0 || len(p.Queries) > 16 {
		out.Summary = "invalid catalog page or query count"
		return out, nil
	}
	var c *tracecatalog.Catalog
	switch p.Action {
	case "", "discover":
		if p.SnapshotPath != "" {
			out.Summary = "snapshot_path requires action=resume"
			return out, nil
		}
		if p.CatalogID != "" {
			out.Summary = "discover does not accept catalog_id; use status to read an existing catalog"
			return out, nil
		}
		root, err := traceCatalogAuthorizedRoot(ctx, p.Root)
		if err != nil {
			out.Summary = err.Error()
			out.Repair = traceCatalogScopeRepair(err)
			return out, nil
		}
		parsed := make([]traceQueryParams, len(p.Queries))
		for i, q := range p.Queries {
			if err := ToolArgumentEnvelopeIntegrityError("trace_query", q, (&TraceQuery{}).Parameters()); err != nil {
				out.Summary = fmt.Sprintf("queries[%d]: %v", i, err)
				return out, nil
			}
			dec := json.NewDecoder(bytes.NewReader(q))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&parsed[i]); err != nil {
				out.Summary = fmt.Sprintf("queries[%d]: %v", i, err)
				return out, nil
			}
			if parsed[i].Source != "" || parsed[i].Path != "" || parsed[i].BusinessSpanRef != "" || parsed[i].Comparison != nil {
				out.Summary = "planned queries must omit source, path and capture-local business_span_ref"
				return out, nil
			}
			if _, err := traceCatalogQueryPlan(parsed[i], q, ""); err != nil {
				out.Summary = err.Error()
				return out, nil
			}
		}
		recursive := true
		if p.Recursive != nil {
			recursive = *p.Recursive
		}
		c, err = tracecatalog.Discover(contextFromBus(ctx), root, tracecatalog.DiscoverOptions{NonRecursive: !recursive})
		if err != nil {
			out.Summary = err.Error()
			if c != nil {
				if retained, installErr := ctx.Mutable.InstallTraceCatalog(c); installErr == nil {
					if page, pageErr := traceCatalogLivePage(retained, "", p.Offset); pageErr == nil {
						out.Summary += "\nPartial discovery retained in this run (not persisted): " + page
					}
				}
			}
			return out, nil
		}
		c, err = ctx.Mutable.InstallTraceCatalog(c)
		if err != nil {
			out.Summary = err.Error()
			return out, nil
		}
		var scopeQueryIDs []string
		scopeSeen := map[string]bool{}
		for _, a := range c.Snapshot().Artifacts {
			for i, q := range p.Queries {
				plan, _ := traceCatalogQueryPlan(parsed[i], q, a.ID)
				registered, err := c.RegisterDeclaredQuery(contextFromBus(ctx), plan)
				if err != nil {
					out.Summary = err.Error()
					return out, nil
				}
				if !scopeSeen[registered.ID] {
					scopeSeen[registered.ID] = true
					scopeQueryIDs = append(scopeQueryIDs, registered.ID)
				}
			}
		}
		if discoverSelection {
			for _, a := range c.Snapshot().Artifacts {
				_, _ = c.Resolve(contextFromBus(ctx), a.ID)
			}
			if _, err := c.SelectRepresentativesForQueries(contextFromBus(ctx), *p.Representatives, scopeQueryIDs); err != nil {
				out.Summary = "plans retained, but explicit representative selection failed: " + err.Error()
				return out, nil
			}
		}
	case "resume":
		var err error
		c, err = traceCatalogResume(ctx, p)
		if err != nil {
			out.Summary = err.Error()
			return out, nil
		}
	case "status", "representatives", "representative_status":
		if p.Root != "" || p.Recursive != nil || len(p.Queries) > 0 || p.SnapshotPath != "" {
			out.Summary = "current catalog actions accept catalog_id and offset; only representatives accepts its selection configuration"
			return out, nil
		}
		for _, current := range ctx.Mutable.TraceCatalogs() {
			if current.ID() == p.CatalogID {
				c = current
				break
			}
		}
		if c == nil {
			out.Summary = "catalog_id is not from this live run; saved JSON is historical navigation only"
			return out, nil
		}
		if p.Action != "status" {
			if _, err := traceCatalogAuthorizedRoot(ctx, c.Snapshot().Root); err != nil {
				out.Summary = err.Error()
				out.Repair = traceCatalogScopeRepair(err)
				return out, nil
			}
		}
		for _, a := range c.Snapshot().Artifacts {
			_, _ = c.Resolve(contextFromBus(ctx), a.ID)
		}
		if p.Action == "representatives" {
			if p.Offset != 0 {
				out.Summary = "new representative selections start at offset 0; use representative_status for subsequent pages"
				return out, nil
			}
			options := tracecatalog.RepresentativeOptions{}
			if p.Representatives != nil {
				options = *p.Representatives
			}
			if _, err := c.SelectRepresentatives(contextFromBus(ctx), options); err != nil {
				out.Summary = err.Error()
				return out, nil
			}
		}
	default:
		out.Summary = "unknown trace_catalog action"
		return out, nil
	}
	publication, err := publishTraceCatalog(ctx, c)
	if err != nil {
		out.Summary = "catalog retained in this run, but persistence failed: " + err.Error()
		return out, nil
	}
	queryCall := traceCatalogPublicationQueryCall(ctx, c, publication.Execution.Snapshot)
	if discoverSelection || p.Action == "representatives" || p.Action == "representative_status" {
		out.Summary, err = traceCatalogRepresentativePage(ctx, publication, p.Offset, queryCall)
	} else {
		out.Summary, err = traceCatalogPublicationPage(publication, p.Offset, queryCall)
	}
	if err != nil {
		out.Summary = err.Error()
		return out, nil
	}
	out.Success = true
	return out, nil
}

func saveTraceCatalog(ctx *types.BusContext, c *tracecatalog.Catalog) (string, error) {
	path, err := traceCatalogSavePath(ctx, c)
	if err != nil {
		return "", err
	}
	return path, c.Save(contextFromBus(ctx), path)
}

func publishTraceCatalog(ctx *types.BusContext, c *tracecatalog.Catalog) (tracecatalog.Publication, error) {
	path, err := traceCatalogSavePath(ctx, c)
	if err != nil {
		return tracecatalog.Publication{}, err
	}
	return c.SavePublication(contextFromBus(ctx), path)
}

func traceCatalogSavePath(ctx *types.BusContext, c *tracecatalog.Catalog) (string, error) {
	if strings.TrimSpace(ctxWorkDir(ctx)) == "" {
		return "", fmt.Errorf("catalog persistence needs a run output directory")
	}
	path := filepath.Join(ctxWorkDir(ctx), "trace-catalog", strings.TrimPrefix(c.ID(), "catalog:")+".json")
	if info, err := os.Lstat(filepath.Dir(path)); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return "", fmt.Errorf("catalog output directory is not a plain directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	return path, nil
}

func traceCatalogPage(s tracecatalog.Snapshot, ref string, offset int, live ...tracecatalog.ExecutionView) (string, error) {
	return traceCatalogPageBound(s, ref, "", "", offset, nil, live...)
}

func traceCatalogPublicationPage(p tracecatalog.Publication, offset int, queryCall func(string) (any, error)) (string, error) {
	return traceCatalogPageBound(p.Execution.Snapshot, p.IndexPath, p.IndexSHA256, p.LegacyPath, offset, queryCall, p.Execution)
}

func traceCatalogPageBound(s tracecatalog.Snapshot, ref, sha, legacy string, offset int, queryCall func(string) (any, error), live ...tracecatalog.ExecutionView) (string, error) {
	const pageSize = 16
	total := len(s.Artifacts) + len(s.Queries)
	if offset > total {
		return "", fmt.Errorf("catalog offset exceeds %d navigation records", total)
	}
	var records []any
	queryCalls := map[string]any{}
	for i := offset; i < total && len(records) < pageSize; i++ {
		if i < len(s.Artifacts) {
			records = append(records, traceCatalogRecord("candidate_artifact", s.Artifacts[i]))
		} else {
			q := s.Queries[i-len(s.Artifacts)]
			if len(live) == 1 {
				records = append(records, traceCatalogExecutionRecord(q, live[0].QueryOrigins[q.ID]))
			} else {
				records = append(records, traceCatalogRecord("current_query_plan", q))
			}
			queryCalls[q.ID] = map[string]any{"catalog_query_ref": map[string]string{"catalog_id": s.ID, "query_id": q.ID}}
			if queryCall != nil {
				call, err := queryCall(q.ID)
				if err != nil {
					return "", err
				}
				queryCalls[q.ID] = call
			}
		}
	}
	page := map[string]any{"catalog_id": s.ID, "root": s.Root, "navigation_only": true, "discovery": s.Discovery,
		"candidate_count": len(s.Artifacts), "registered_query_count": len(s.Queries), "offset": offset, "total_records": total, "records": records, "saved_index": ref,
		"guidance": "Candidates are not yet admitted captures. Query each selected path with trace_query. Window numbers are argument bounds, not an inferred endpoint-inclusion contract; use actual query evidence for boundaries. Query records preserve source/object/window separately; unexecuted, failed and empty are different. Do not combine capture clocks or infer causes from this index."}
	if sha != "" {
		page["saved_index_sha256"], page["latest_index_alias"] = sha, legacy
	}
	if len(live) == 1 {
		page["current_execution"] = live[0].Coverage
		page["coverage_boundary"] = tracecatalog.ExecutionCoverageBoundary
		page["representative_selection_state"] = live[0].SelectionState()
		if selection := live[0].Selection; selection != nil {
			page["selection_id"] = selection.SelectionID
			page["selection_scope"] = selection.SelectionScope
			page["representative_status_call"] = map[string]any{"action": "representative_status", "catalog_id": s.ID}
		}
	}
	if len(queryCalls) > 0 {
		page["execution"] = "Pass one returned query_calls object unchanged to trace_query for normal preparation and a real query. Plans, attempts, and artifact-object pairs are different counts; no count here is a runtime measurement."
		page["query_calls"] = queryCalls
	} else if len(s.Queries) == 0 {
		page["execution"] = "Only candidate files were discovered; no executable query plans exist. Candidate artifact IDs are not query IDs. For planned comparison, discover again with the same root and queries before selecting representatives; each query requires view and task-supported object/window/filter fields (see queries.items in the current schema). This adds plans without resetting existing attempts. An ordinary trace_query with a selected candidate path remains available for direct investigation."
	} else if offset == total {
		page["execution"] = "End of the current catalog: no more records or query_calls on this page."
	} else {
		page["execution"] = "This page contains candidates only; use next_call to reach planned query_calls. Candidate artifact IDs are not query IDs."
	}
	if len(s.Queries) > 0 {
		page["representatives_call"] = map[string]any{"action": "representatives", "catalog_id": s.ID}
		page["representatives_guidance"] = "Optional comparison navigation: use representatives_call if representative selection fits the task; supply an explicit representative budget when needed. This page does not select or execute any representative."
	}
	if len(s.History) > 0 {
		page["historical_runs"] = len(s.History)
		page["history_notice"] = "All earlier attempts are retained in saved_index.history as navigation only. Current queries start not_executed, including historical successes; execute again for current facts."
	}
	if offset+len(records) < total {
		page["next_call"] = map[string]any{"action": "status", "catalog_id": s.ID, "offset": offset + len(records)}
	}
	b, err := json.Marshal(page)
	if err == nil && len(b) > 64<<10 {
		return "", fmt.Errorf("catalog page exceeds 64 KiB; use a narrower discovery/plan (full index was saved)")
	}
	return string(b), err
}
