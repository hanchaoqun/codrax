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
	return "Discover candidate capture files across a selected directory and keep a per-capture, per-object query index. Recursive discovery preserves same-named files in different directories. Optional queries are ordinary trace_query arguments without source/path: they register expected work for each candidate but do not execute it. Continue with trace_query using the returned paths; status shows results, empty results, failures and unexecuted work separately. Candidates and saved indexes are navigation only, not measurements, input admission or causal evidence."
}
func (*TraceCatalog) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
	"action":{"type":"string","enum":["discover","status"],"description":"Default discover; status reads a current catalog without rediscovering."},
	"root":{"type":"string","description":"Selected capture directory (default .), inside the active repository or a host-pinned path."},
	"recursive":{"type":"boolean","description":"Default true. Symbolic links are not followed; skipped and incomplete traversal are disclosed."},
	"queries":{"type":"array","maxItems":16,"items":{"type":"object"},"description":"Optional expected trace_query calls for every candidate. Reuse that tool's argument contract, omit source/path. No execution or evidence is produced by planning."},
	"catalog_id":{"type":"string","description":"Exact ID returned by discover; required for status."},
	"offset":{"type":"integer","minimum":0,"description":"Navigation page offset, default 0."}
	}}`)
}

type traceCatalogParams struct {
	Action    string            `json:"action"`
	Root      string            `json:"root"`
	Recursive *bool             `json:"recursive"`
	Queries   []json.RawMessage `json:"queries"`
	CatalogID string            `json:"catalog_id"`
	Offset    int               `json:"offset"`
}

func (t *TraceCatalog) Execute(ctx *types.BusContext, raw json.RawMessage) (types.ToolResult, error) {
	out := types.ToolResult{ToolName: t.Name(), Timestamp: time.Now()}
	var p traceCatalogParams
	if _, failure, err := decodeStrictToolParams(t.Name(), raw, t.Parameters(), &p, nil); err != nil {
		return *failure, err
	}
	if ctx == nil || ctx.Mutable == nil {
		out.Summary = "trace_catalog needs current run state"
		return out, nil
	}
	if p.Offset < 0 || len(p.Queries) > 16 {
		out.Summary = "invalid catalog page or query count"
		return out, nil
	}
	var c *tracecatalog.Catalog
	switch p.Action {
	case "", "discover":
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
			if parsed[i].Source != "" || parsed[i].Path != "" || parsed[i].BusinessSpanRef != "" {
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
	case "status":
		if p.Root != "" || p.Recursive != nil || len(p.Queries) > 0 {
			out.Summary = "status accepts only catalog_id and offset"
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
		for _, a := range c.Snapshot().Artifacts {
			_, _ = c.Resolve(contextFromBus(ctx), a.ID)
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
	out.Summary, err = traceCatalogPage(c.Snapshot(), ref, p.Offset)
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
	for i := offset; i < total && len(records) < pageSize; i++ {
		if i < len(s.Artifacts) {
			records = append(records, s.Artifacts[i])
		} else {
			records = append(records, s.Queries[i-len(s.Artifacts)])
		}
	}
	page := map[string]any{"catalog_id": s.ID, "root": s.Root, "navigation_only": true, "discovery": s.Discovery,
		"candidate_count": len(s.Artifacts), "planned_query_count": len(s.Queries), "offset": offset, "total_records": total, "records": records, "saved_index": ref,
		"guidance": "Candidates are not yet admitted captures. Query each selected path with trace_query. Query records preserve source/object/window separately; unexecuted, failed and empty are different. Do not combine capture clocks or infer causes from this index."}
	if offset+len(records) < total {
		page["next_call"] = map[string]any{"action": "status", "catalog_id": s.ID, "offset": offset + len(records)}
	}
	b, err := json.Marshal(page)
	if err == nil && len(b) > 64<<10 {
		return "", fmt.Errorf("catalog page exceeds 64 KiB; use a narrower discovery/plan (full index was saved)")
	}
	return string(b), err
}
