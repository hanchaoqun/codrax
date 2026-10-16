package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceCatalogRepresentativesTeaching = `For representative comparison, first register the requested per-capture query specifications with discover.queries (view is required; object/window/filter fields come from the task). Re-discover the same root to add plans without resetting attempts. Then use {"action":"representatives","catalog_id":"<current id>"} with an optional representatives budget, and pass a returned query_calls object unchanged to trace_query. action="representative_status" refreshes states without changing the selected members. Groups mean exact query specifications, not shared physical identities or common causes. Unselected, source-skipped and unmatched candidates are not proven normal; success/empty/failure/pending remain separate.`

func traceCatalogRepresentativePage(ctx *types.BusContext, c *tracecatalog.Catalog, catalogRef string, offset int) (string, error) {
	s, ok := c.Representatives()
	if !ok {
		return "", fmt.Errorf("no current representative selection; use action=representatives first (historical JSON does not restore a selection)")
	}
	if err := contextFromBus(ctx).Err(); err != nil {
		return "", err
	}
	// Independent immutable navigation snapshot, not a query result. Keeping it
	// separate avoids adding a restoration path or increasing old schema trust.
	full, err := json.MarshalIndent(struct {
		Kind string `json:"kind"`
		tracecatalog.RepresentativeSnapshot
	}{"query_specification_representatives/v1", s}, "", "  ")
	if err != nil {
		return "", err
	}
	if len(full) > tracecatalog.MaxSnapshotBytes {
		return "", fmt.Errorf("representative navigation snapshot exceeds 8 MiB; no page was published; the live selection remains recorded but requires a smaller catalog for this bounded display")
	}
	digest := sha256.Sum256(full)
	ref := StoreBlobArtifact(ctxWorkDir(ctx), "trace_catalog", "representatives-"+hex.EncodeToString(digest[:])+".json", string(full))
	if ref == "" {
		return "", fmt.Errorf("representative navigation snapshot could not be persisted")
	}
	var records []any
	for _, g := range s.Clusters {
		records = append(records, map[string]any{"record_kind": "query_specification_cluster", "cluster_id": g.ID, "specification": g.Specification, "coverage": g.Coverage, "budget_skipped": g.BudgetSkipped})
		for _, m := range g.Members {
			records = append(records, map[string]any{"record_kind": "cluster_member", "cluster_id": g.ID, "member": m})
		}
	}
	for _, a := range s.Unmatched {
		records = append(records, map[string]any{"record_kind": "unmatched_candidate", "artifact": a})
	}
	for _, id := range s.NewQueryIDs {
		records = append(records, map[string]any{"record_kind": "query_not_in_selection", "query_id": id})
	}
	if offset < 0 || offset > len(records) {
		return "", fmt.Errorf("representative offset exceeds %d navigation records", len(records))
	}
	page := map[string]any{"catalog_id": s.CatalogID, "selection_id": s.SelectionID, "navigation_only": true, "options": s.Options, "coverage": s.Coverage, "discovery": s.Discovery, "saved_index": catalogRef, "selection_snapshot": ref, "selection_snapshot_sha256": hex.EncodeToString(digest[:]), "offset": offset, "total_records": len(records), "guidance": "Selection membership is frozen; outcomes and original result references are current navigation status. Same specification across captures does not prove the same thread, device, clock or cause. These counts are query plans, not runtime events or completed artifacts. Do not extrapolate selected results to unselected members; even an empty result is only that actual query's observation. New plans are outside this selection. Use actual payload evidence for values."}
	shown := []any{}
	calls := map[string]any{}
	encode := func() ([]byte, error) {
		page["records"], page["query_calls"] = shown, calls
		page["displayed_records"], page["omitted_records"] = len(shown), len(records)-offset-len(shown)
		delete(page, "next_call")
		if offset+len(shown) < len(records) {
			page["next_call"] = map[string]any{"action": "representative_status", "catalog_id": s.CatalogID, "offset": offset + len(shown)}
		}
		return json.Marshal(page)
	}
	for i := offset; i < len(records) && len(shown) < 16; i++ {
		record := records[i].(map[string]any)
		shown = append(shown, record)
		id := ""
		if m, ok := record["member"].(tracecatalog.RepresentativeMember); ok && m.Selected && m.SourceStatus != "stale" && m.SourceStatus != "unavailable" {
			id = m.QueryID
			calls[id] = map[string]any{"catalog_query_ref": map[string]string{"catalog_id": s.CatalogID, "query_id": id}}
		}
		b, err := encode()
		if err != nil {
			return "", err
		}
		if len(b) > 64<<10 {
			shown = shown[:len(shown)-1]
			delete(calls, id)
			if len(shown) == 0 {
				return "", fmt.Errorf("one representative navigation record exceeds 64 KiB; full selection is at %s", ref)
			}
			break
		}
	}
	if err := contextFromBus(ctx).Err(); err != nil {
		return "", err
	}
	b, err := encode()
	return string(b), err
}
