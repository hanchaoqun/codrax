package tool

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC342FullScopeSiblingCoordinatesAndReceiptIsolation(t *testing.T) {
	dir := t.TempDir()
	path := b1631FrequencyTrace(t, dir, "capture")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(body), "prev_state=S", "prev_state=D")), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 42})
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("public Execute: %v %+v", err, result)
	}
	var account, coverage types.ObservationRecord
	for _, record := range result.Observations {
		switch record.Predicate {
		case "target_window_wait_occurrences":
			account = record
		case types.RuntimeArtifactScopeCoveragePredicate:
			coverage = record
		}
	}
	if account.ID == "" || coverage.ID == "" {
		t.Fatal("actual query did not publish both account and coverage")
	}
	prefix, ok := strings.CutSuffix(account.ID, "#target_window_wait_occurrences")
	if !ok || coverage.ID != prefix+"#runtime_artifact_scope_coverage" {
		t.Fatal("same query's scope siblings were minted from different parent receipts")
	}
	rm := &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Thread: "target-42"}}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "whole capture"}}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"current exact":           nil,
		"another query copied ID": func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += ":other" },
		"another raw copied ID":   func(r *types.ObservationRecord) { r.SourceRef.RawRef += ".other" },
		"another source copied ID": func(r *types.ObservationRecord) {
			r.SourceRef.Path += ".other"
		},
		"untrusted coverage": func(r *types.ObservationRecord) { r.GroundingPolicy = types.ClaimGroundingSoft },
	} {
		t.Run(name, func(t *testing.T) {
			other := coverage
			if mutate != nil {
				mutate(&other)
			}
			rows := append([]types.ObservationRecord(nil), result.Observations...)
			for i := range rows {
				if rows[i].ID == coverage.ID {
					rows[i] = other
				}
			}
			got := types.BuildTraceTargetWaitSummaryAuthorities(types.ObservationLedger{Records: rows}, rm)
			if len(got) != 1 || got[0].Count != 1 || got[0].DStateOccurrences != 1 || got[0].IsRequestedScopePrincipal() != (mutate == nil) {
				t.Fatalf("scope authority must follow the complete same-result coverage, not a copied ID: %+v", got)
			}
		})
	}
}
