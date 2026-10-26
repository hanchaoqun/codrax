package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceCatalogRepresentativesTeaching = `For accepted artifact work with typed targets, one common window and a work budget, use discover with request_plan and your explicit view; its representative mode already includes the accepted selection budget. Do not combine request_plan with queries or representatives. For additional filters or manual specifications, use discover with the explicit requested queries and representatives budget together, retaining task targets/window: this freezes selection only within that call's plans, without executing them. Earlier exploration remains registered but outside that selection. Alternatively, after planning use {"action":"representatives","catalog_id":"<current id>"} to explicitly select from the entire current catalog. Pass returned query_calls unchanged to trace_query. For an existing selection, status and representative_status both deliver the current same-generation saved_index and selection_snapshot with hashes; representative_status additionally pages the fixed selected/unselected roster. Neither status action selects or executes. Plain discovery without either selection request never selects. Groups are exact query specifications, not result similarity, shared physical identities or common causes. Unselected, source-skipped, outside-scope and unmatched work are not proven normal; success/empty/failure/pending remain separate.`

func traceCatalogRepresentativePage(ctx *types.BusContext, publication tracecatalog.Publication, offset int, queryCall func(string) (any, error)) (string, error) {
	if publication.Execution.Selection == nil {
		return "", fmt.Errorf("no current representative selection; use action=representatives first (historical JSON does not restore a selection)")
	}
	s := *publication.Execution.Selection
	if err := contextFromBus(ctx).Err(); err != nil {
		return "", err
	}
	// The index and selection were published together from the same detached
	// live union. Do not reread the executing fork or overwrite identical files.
	fields, err := traceCatalogPublicationFields(publication)
	if err != nil {
		return "", err
	}
	ref := publication.SelectionPath
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
	for _, id := range s.ExcludedQueryIDs {
		records = append(records, map[string]any{"record_kind": "query_outside_selection_scope", "query_id": id})
	}
	for _, id := range s.NewQueryIDs {
		records = append(records, map[string]any{"record_kind": "query_not_in_selection", "query_id": id})
	}
	if offset < 0 || offset > len(records) {
		return "", fmt.Errorf("representative offset exceeds %d navigation records", len(records))
	}
	page := map[string]any{"catalog_id": s.CatalogID, "navigation_only": true, "options": s.Options, "coverage": s.Coverage, "discovery": s.Discovery, "offset": offset, "total_records": len(records), "guidance": "Selection membership is frozen; outcomes and original result references describe the same delivered generation as the content-bound saved_index. latest_index_alias may change after later calls and is not this generation's identity. Same specification across captures does not prove the same thread, device, clock or cause. These counts are query plans, not runtime events or completed artifacts. Do not extrapolate selected results to unselected members; even an empty result is only that actual query's observation. Prior queries excluded by explicit scope and plans registered after selection are listed separately; both remain executable through ordinary catalog status/query calls. Use actual payload evidence for values."}
	for key, value := range fields {
		page[key] = value
	}
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
			call, err := queryCall(id)
			if err != nil {
				return "", err
			}
			calls[id] = call
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
