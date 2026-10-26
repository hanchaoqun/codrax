package tool

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// All public navigation shapes deliver identities from the same already-published
// view. A normal status read must not require a second action to obtain the
// selection paired with its index. This only exposes navigation, never installs
// a selection, executes work, or reconstructs permissions from saved JSON.
func traceCatalogPublicationFields(p tracecatalog.Publication) (map[string]any, error) {
	fields := map[string]any{"saved_index": p.IndexPath, "saved_index_sha256": p.IndexSHA256, "latest_index_alias": p.LegacyPath}
	if selection := p.Execution.Selection; selection != nil {
		if p.SelectionPath == "" || p.SelectionSHA256 == "" {
			return nil, fmt.Errorf("representative navigation snapshot could not be persisted")
		}
		fields["selection_id"] = selection.SelectionID
		fields["selection_scope"] = selection.SelectionScope
		fields["selection_snapshot"] = p.SelectionPath
		fields["selection_snapshot_sha256"] = p.SelectionSHA256
	}
	return fields, nil
}

// A published union is navigation, not an installed executing fork. Keep live
// local references unchanged; peer-only plans use the ordinary path/parameters
// entry, which rechecks ordinary source admission and native query
// state. Never install the union or reconstruct authority from serialized JSON.
func traceCatalogPublicationQueryCall(ctx *types.BusContext, local *tracecatalog.Catalog, snapshot tracecatalog.Snapshot) func(string) (any, error) {
	queries := map[string]tracecatalog.QueryRecord{}
	artifacts := map[string]tracecatalog.Artifact{}
	for _, q := range snapshot.Queries {
		queries[q.ID] = q
	}
	for _, a := range snapshot.Artifacts {
		artifacts[a.ID] = a
	}
	return func(id string) (any, error) {
		q, ok := queries[id]
		if !ok {
			return nil, fmt.Errorf("published query is absent from its index")
		}
		if _, ok := local.Query(id); ok {
			return map[string]any{"catalog_query_ref": map[string]string{"catalog_id": snapshot.ID, "query_id": id}}, nil
		}
		// A peer's directory navigation does not bypass current catalog scope.
		// Once explicitly issued, the resulting ordinary single-file call keeps
		// its existing semantics; directory scope is not a file-read capability.
		if _, err := traceCatalogAuthorizedRoot(ctx, snapshot.Root); err != nil {
			return nil, err
		}
		// Reuse the current public query schema and exact metadata projection;
		// adding a physical source must not change object/window/filter identity.
		plan, err := traceCatalogRevalidatePlan(q.Parameters, q.ArtifactID)
		if err != nil || plan.View != q.View || plan.Object != q.Object || !reflect.DeepEqual(plan.Window, q.Window) {
			return nil, fmt.Errorf("published peer query cannot be expressed by the current normal query schema")
		}
		a, ok := artifacts[q.ArtifactID]
		if !ok || a.Path == "" || a.RevisionID != q.ArtifactRevision {
			return nil, fmt.Errorf("published peer query has no exact source member")
		}
		var args map[string]json.RawMessage
		if err := json.Unmarshal(q.Parameters, &args); err != nil {
			return nil, err
		}
		args["source"] = json.RawMessage(`"path"`)
		args["path"], err = json.Marshal(a.Path)
		if err != nil {
			return nil, err
		}
		return args, nil
	}
}
