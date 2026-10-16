package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Counts describe navigation records, never runtime measurements or completion
// of the user's task. Distinct views/parameters and retries remain separate.
type traceCatalogNavigationCounts struct {
	Plans           int                          `json:"plans"`
	Attempts        int                          `json:"attempts"`
	ArtifactObjects int                          `json:"artifact_objects"`
	Outcomes        map[tracecatalog.Outcome]int `json:"plan_outcomes"`
}

func traceCatalogCounts(queries []tracecatalog.QueryRecord) traceCatalogNavigationCounts {
	c := traceCatalogNavigationCounts{Plans: len(queries), Outcomes: map[tracecatalog.Outcome]int{}}
	members := map[string]bool{}
	for _, q := range queries {
		c.Attempts += len(q.Attempts)
		c.Outcomes[q.Outcome]++
		b, _ := json.Marshal(struct {
			Artifact string
			Object   tracecatalog.Object
		}{q.ArtifactID, q.Object})
		members[string(b)] = true
	}
	c.ArtifactObjects = len(members)
	return c
}

// TraceCatalogReadNavigation is advisory, never a tool receipt, source-read
// credential, live catalog, or measurement.
type TraceCatalogReadNavigation struct {
	Summary    string
	Refinement *types.ToolRefinementHint
}

// BuildTraceCatalogReadNavigation projects only bytes already delivered in
// full. It reads no new bytes and grants no access; resume rechecks admission.
// A caller's complete flag cannot restore old results or install a catalog.
func BuildTraceCatalogReadNavigation(ctx *types.BusContext, path string, data []byte, complete bool) *TraceCatalogReadNavigation {
	if !complete || contextFromBus(ctx).Err() != nil {
		return nil
	}
	s, err := tracecatalog.DecodeSnapshot(data)
	if err != nil {
		return nil
	}
	nav := struct {
		Kind             string                       `json:"kind"`
		NavigationOnly   bool                         `json:"navigation_only"`
		SnapshotPath     string                       `json:"snapshot_path"`
		SnapshotSHA256   string                       `json:"snapshot_sha256"`
		Historical       traceCatalogNavigationCounts `json:"historical_snapshot"`
		EarlierRuns      int                          `json:"earlier_historical_runs"`
		CurrentExecution string                       `json:"current_execution"`
		NextCall         map[string]string            `json:"next_call,omitempty"`
		Boundary         string                       `json:"boundary"`
	}{Kind: "trace_catalog_checkpoint", NavigationOnly: true, SnapshotPath: path, Historical: traceCatalogCounts(s.Queries), EarlierRuns: len(s.History), CurrentExecution: "not_restored", Boundary: "Historical outcomes are navigation, not current results. Resume rechecks current access, source generation and query parameters; every plan, including earlier successes, then needs a fresh trace_query call."}
	digest := sha256.Sum256(data)
	nav.SnapshotSHA256 = hex.EncodeToString(digest[:])
	// This negative qualification survives the existing typed stage handoff;
	// file line anchors still describe the file, not a current execution.
	hint := &types.ToolRefinementHint{ReasonCode: "historical_catalog_navigation", UniverseExcludedReason: "historical_results_not_current_execution_evidence"}
	root, rootErr := traceCatalogAuthorizedRoot(ctx, s.Root)
	snapshotPath, pathErr := traceCatalogAuthorizedRoot(ctx, path)
	if rootErr == nil && pathErr == nil && root == s.Root {
		// Parameters are the existing public resume API, not a new capability.
		params := map[string]string{"action": "resume", "root": root, "snapshot_path": snapshotPath}
		nav.NextCall = params
		// Refinement's generic display caps each value at 240 bytes. Never
		// publish a clipped path as a suggested executable parameter there.
		if len(root) <= 240 && len(snapshotPath) <= 240 {
			hint.PreferredNextTool = "trace_catalog"
			hint.PreferredParams = params
		}
	} else {
		nav.Boundary = "Historical navigation only. Its recorded root or checkpoint path lacks current host authorization; select/pin the intended root before requesting resume. Reading this file grants no member or parent-directory access."
	}
	b, err := json.Marshal(nav)
	if err == nil && contextFromBus(ctx).Err() == nil {
		return &TraceCatalogReadNavigation{Summary: "Checkpoint navigation (not query evidence): " + string(b), Refinement: hint}
	}
	return nil
}

// Keep read_file's existing public result and source-line anchors unchanged.
func finishTraceCatalogReadNavigation(ctx *types.BusContext, path string, data []byte, complete bool, out *types.ToolResult) {
	if out == nil || !out.Success {
		return
	}
	if nav := BuildTraceCatalogReadNavigation(ctx, path, data, complete); nav != nil {
		out.Refinement = nav.Refinement
		out.Summary = nav.Summary + "\n" + out.Summary
	}
}

func traceCatalogRecord(kind string, record any) any {
	switch value := record.(type) {
	case tracecatalog.Artifact:
		return struct {
			Kind string `json:"record_kind"`
			tracecatalog.Artifact
		}{kind, value}
	case tracecatalog.QueryRecord:
		return struct {
			Kind string `json:"record_kind"`
			tracecatalog.QueryRecord
		}{kind, value}
	default:
		return record
	}
}

func traceCatalogWrongReference(c *tracecatalog.Catalog, id string) error {
	for _, a := range c.Snapshot().Artifacts {
		if a.ID == id {
			return fmt.Errorf("catalog_query_ref.query_id names a candidate artifact, not a planned query; use a query_calls entry from trace_catalog status, or issue an ordinary trace_query with this artifact's path and explicit query arguments when no plan exists")
		}
	}
	return fmt.Errorf("catalog_query_ref is not a planned query in this current run; use trace_catalog status for current query_calls or resume with an explicitly authorized root for history")
}
