package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	return "Discover candidate capture files across a selected directory and keep a per-capture, per-object query index. Recursive discovery preserves same-named files in different directories. Optional queries are ordinary trace_query arguments without source/path: they register expected work for each candidate but do not execute it. For directory work with one explicit request window, put that window in the expected calls once; after admission, queries of those same members can inherit it when all their declared plans agree. Discovery alone does not bind a request window. Continue with trace_query using the returned paths; status shows results, empty results, failures and unexecuted work separately. Candidates and saved indexes are navigation only, not measurements, input admission or causal evidence. " + traceCatalogResumeTeaching + " " + traceCatalogRepresentativesTeaching
}
func (*TraceCatalog) Parameters() json.RawMessage {
	return traceCatalogPlanSchema(json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
	"action":{"type":"string","enum":["discover","status","resume","representatives","representative_status"],"description":"Default discover; status reads a current catalog; resume re-admits a historical roster. representatives freezes soft-ranked representatives of exact query-specification groups; representative_status pages that fixed selection with current execution states."},
	"representatives":{"type":"object","additionalProperties":false,"properties":{"per_cluster":{"type":"integer","minimum":1,"maximum":5,"description":"Default 1; resource limit, not statistical coverage."},"max_clusters":{"type":"integer","minimum":1,"maximum":50,"description":"Default 10; resource limit. Other groups remain listed as budget-skipped."},"priority":{"type":"string","enum":["unexecuted_first","retry_first","largest_input_first"],"description":"Default unexecuted_first. Navigation metadata only, not severity; unknown sizes are not zero. Exact query IDs break ties."}},"description":"Only action=representatives. Does not execute or prohibit any member; a new explicit selection replaces the previous run-local selection."},
	"snapshot_path":{"type":"string","description":"Resume only: exact currently authorized historical navigation JSON. This file grants no access to its members or parent directory."},
	"root":{"type":"string","description":"Selected capture directory (default .), inside the active repository or a host-pinned path."},
	"recursive":{"type":"boolean","description":"Default true. Symbolic links are not followed; skipped and incomplete traversal are disclosed."},
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
	if p.Representatives != nil && p.Action != "representatives" {
		out.Summary = "representatives configuration requires action=representatives"
		return out, nil
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
					if page, pageErr := traceCatalogPage(retained.Snapshot(), "", p.Offset); pageErr == nil {
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
		for _, a := range c.Snapshot().Artifacts {
			for i, q := range p.Queries {
				plan, _ := traceCatalogQueryPlan(parsed[i], q, a.ID)
				if _, err := c.RegisterDeclaredQuery(contextFromBus(ctx), plan); err != nil {
					out.Summary = err.Error()
					return out, nil
				}
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
	ref, err := saveTraceCatalog(ctx, c)
	if err != nil {
		out.Summary = "catalog retained in this run, but persistence failed: " + err.Error()
		return out, nil
	}
	if p.Action == "representatives" || p.Action == "representative_status" {
		out.Summary, err = traceCatalogRepresentativePage(ctx, c, ref, p.Offset)
	} else {
		out.Summary, err = traceCatalogPage(c.Snapshot(), ref, p.Offset)
	}
	if err != nil {
		out.Summary = err.Error()
		return out, nil
	}
	out.Success = true
	return out, nil
}

func saveTraceCatalog(ctx *types.BusContext, c *tracecatalog.Catalog) (string, error) {
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
	return path, c.Save(contextFromBus(ctx), path)
}

func traceCatalogPage(s tracecatalog.Snapshot, ref string, offset int) (string, error) {
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
			records = append(records, traceCatalogRecord("current_query_plan", q))
			queryCalls[q.ID] = map[string]any{"catalog_query_ref": map[string]string{"catalog_id": s.ID, "query_id": q.ID}}
		}
	}
	page := map[string]any{"catalog_id": s.ID, "root": s.Root, "navigation_only": true, "discovery": s.Discovery,
		"candidate_count": len(s.Artifacts), "planned_query_count": len(s.Queries), "offset": offset, "total_records": total, "records": records, "saved_index": ref,
		"guidance": "Candidates are not yet admitted captures. Query each selected path with trace_query. Window numbers are argument bounds, not an inferred endpoint-inclusion contract; use actual query evidence for boundaries. Query records preserve source/object/window separately; unexecuted, failed and empty are different. Do not combine capture clocks or infer causes from this index."}
	page["current_counts"] = traceCatalogCounts(s.Queries)
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
