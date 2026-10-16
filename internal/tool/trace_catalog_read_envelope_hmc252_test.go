package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Public reader and grounding/dispatch-delivery APIs share the exact produced
// bytes. No call here represents a model response or authorizes test execution.
func TestHMC252CheckpointReadEnvelopePublicConsumers(t *testing.T) {
	for _, kind := range []string{"full", "partial", "cropped_message", "json_replayed_result", "read_mode"} {
		t.Run(kind, func(t *testing.T) {
			bus, oldPath := checkpointRead247(t)
			body, err := os.ReadFile(oldPath)
			if err != nil {
				t.Fatal(err)
			}
			const path = "checkpoint.json"
			if err := os.WriteFile(filepath.Join(bus.RepoRoot, path), body, 0600); err != nil {
				t.Fatal(err)
			}
			bus.Mode = types.ModePlan
			if kind == "read_mode" {
				bus.Mode = types.ModeRead
			}
			bus.Mutable = types.NewMutableState("read the saved navigation")
			params := map[string]any{"path": path}
			if kind == "partial" {
				params["line_offset"], params["limit"] = 1, 1
			}
			raw, _ := json.Marshal(params)
			result, err := (&ReadFile{}).Execute(bus, raw)
			if err != nil || !result.Success || result.ReadCoverage == nil {
				t.Fatalf("real checkpoint read: %v %+v", err, result)
			}
			wantNavigation := kind != "partial"
			gotNavigation := result.Refinement != nil && result.Refinement.ReasonCode == "historical_catalog_navigation" && result.Refinement.PreferredNextTool == "trace_catalog"
			if gotNavigation != wantNavigation {
				t.Fatalf("navigation=%v want=%v", gotNavigation, wantNavigation)
			}
			if kind == "cropped_message" {
				result.Summary = strings.SplitN(result.Summary, "\n", 2)[0]
			}
			if kind == "json_replayed_result" {
				wire, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				result = types.ToolResult{}
				if err := json.Unmarshal(wire, &result); err != nil {
					t.Fatal(err)
				}
				bus.Mutable = types.NewMutableState("replayed JSON cannot recreate a read receipt")
			}
			bus.Mutable.AppendDispatchToolResult(result)
			root, err := filepath.EvalSymlinks(bus.RepoRoot)
			if err != nil {
				t.Fatal(err)
			}
			digest := dispatchReadVersionSHA(string(body))
			generation := bus.Mutable.BeginDispatchRepositoryFileRead()
			ticket := bus.Mutable.BindDispatchRepositoryReadMessage(generation, "read-checkpoint", result)
			bound := ticket.MatchesMessage("read-checkpoint", result.Summary)
			wantBound := kind == "full" || kind == "partial"
			if bound != wantBound {
				t.Errorf("exact-message receipt bound=%v want=%v", bound, wantBound)
			}
			if bus.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(root, path, digest) {
				t.Error("binding alone claimed successful message delivery")
			}
			if bound && !bus.Mutable.RecordDispatchRepositoryReadDelivery(generation, []types.DispatchRepositoryReadMessageReceipt{ticket}) {
				t.Error("exact produced message receipt could not be committed")
			}
			if got := bus.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(root, path, digest); got != (kind == "full") {
				t.Errorf("whole-file delivered qualification=%v want=%v", got, kind == "full")
			}
			if kind != "json_replayed_result" {
				item := types.EvidenceItem{Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: path,
					LineStart: 2, AnchorKind: types.AnchorStringLiteral, AnchorSymbol: "schema_version", Subject: "schema_version"}
				ground.GroundItem(&item, ground.BuildContext(bus))
				wantGround := kind != "cropped_message"
				if got := item.GroundingStatus == types.GroundingGrounded && item.GroundingTier == types.TierLineText; got != wantGround {
					t.Errorf("literal line grounding=%v want=%v status=%s tier=%s note=%s", got, wantGround, item.GroundingStatus, item.GroundingTier, item.GroundingNote)
				}
			}
			if len(bus.Mutable.TraceCatalogs()) != 0 || bus.Mutable.TraceQueryRuntimeObservationCount() != 0 || result.TraceQuerySourceRead.Path() != "" || bus.Mutable.ChangePlan() != nil || bus.Mutable.ChangeReport() != nil {
				t.Error("read/navigation gained live query, plan, or execution authority")
			}
			actual, err := os.ReadFile(filepath.Join(bus.RepoRoot, path))
			if err != nil || string(actual) != string(body) {
				t.Error("read modified checkpoint bytes")
			}
		})
	}
}
