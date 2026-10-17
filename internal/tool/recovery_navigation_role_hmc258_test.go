package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func recoveryNavigationFixture258(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	bus, saved := checkpointRead247(t)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile:          &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{},
	}}
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "settings.json") // purpose, never basename
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return bus, path
}

func TestHMC258RecoveryNavigationPublicReadIsNotCurrentSource(t *testing.T) {
	bus, path := recoveryNavigationFixture258(t)
	out := readCheckpoint247(t, bus, path)
	if out.ReadCoverage != nil || len(out.Observations) != 0 {
		t.Fatalf("complete navigation minted current source coverage/observations: %+v / %+v", out.ReadCoverage, out.Observations)
	}
	if !strings.Contains(out.Summary, "Checkpoint navigation") || !strings.Contains(out.Summary, "│") {
		t.Fatal("navigation lost actual readable bytes")
	}
	bus.Mutable.AppendDispatchToolResult(out)
	// A stale or direct producer item must not bypass the same material role.
	bus.EvidenceItems = []types.EvidenceItem{{ID: "navigation-field", Source: "settings.json", LineStart: 1, LineEnd: 1, Scope: types.ScopeLine, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, Summary: "historical plan field"}}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	if len(ledger.Records) == 0 {
		t.Fatal("navigation field silently erased")
	}
	for _, r := range ledger.Records {
		if r.SourceRef.Kind == types.ObservationSourceCurrentSource || r.SourceRef.Kind == types.ObservationSourceRuntimeArtifact {
			t.Fatalf("historical navigation gained source/runtime authority: %+v", r)
		}
	}
	if _, ok := newPreEmitCheckContext(bus).citedEvidenceItems(types.Citation{File: "settings.json", Line: 1}); ok {
		t.Fatal("direct citation pool bypassed navigation role")
	}
}

func TestHMC258RecoveryNavigationOrdinaryConfigPreservesSource(t *testing.T) {
	bus, path := recoveryNavigationFixture258(t)
	for _, body := range []string{`{"navigation_only":true,"root":"/tmp","threads":4}`, `{"schema_version":1,"navigation_only":true}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		out := readCheckpoint247(t, bus, path)
		if out.ReadCoverage == nil || len(out.Observations) != 1 {
			t.Fatal("ordinary JSON lost source role")
		}
	}
}

func TestHMC258RecoveryNavigationResumeWithoutReadIsNotSource(t *testing.T) {
	bus, path := recoveryNavigationFixture258(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	nav := BuildTraceCatalogReadNavigation(bus, path, data, true)
	raw, _ := json.Marshal(nav.Refinement.PreferredParams)
	out, err := (&TraceCatalog{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	bus.EvidenceItems = []types.EvidenceItem{{ID: "old-config", Source: "settings.json", LineStart: 1, Scope: types.ScopeLine, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded}}
	if _, ok := newPreEmitCheckContext(bus).citedEvidenceItems(types.Citation{File: "settings.json", Line: 1}); ok {
		t.Fatal("successful resume did not preserve material purpose")
	}
}

func TestHMC258RecoveryNavigationLifecycleAndIdentity(t *testing.T) {
	for _, action := range []string{"alias", "same_name_other_file", "same_bytes_new_generation", "ordinary_rewrite", "reset", "json_restore", "dispatch_reset"} {
		t.Run(action, func(t *testing.T) {
			bus, path := recoveryNavigationFixture258(t)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			readCheckpoint247(t, bus, path)
			fork := bus.Mutable.ForkForExploreDispatch()
			if !fork.IsRecoveryNavigationMaterial(bus.RepoRoot, "settings.json") {
				t.Fatal("fork lost current role")
			}
			want := false
			switch action {
			case "alias":
				alias := filepath.Join(bus.RepoRoot, "alias.json")
				if err := os.Symlink(path, alias); err != nil {
					t.Fatal(err)
				}
				path, want = alias, true
			case "same_name_other_file":
				path = filepath.Join(t.TempDir(), "settings.json")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "same_bytes_new_generation":
				replacement := filepath.Join(bus.RepoRoot, "replacement")
				if err := os.WriteFile(replacement, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			case "ordinary_rewrite":
				if err := os.WriteFile(path, []byte(`{"workers":8}`), 0600); err != nil {
					t.Fatal(err)
				}
				if out := readCheckpoint247(t, bus, path); out.ReadCoverage == nil || len(out.Observations) == 0 {
					t.Fatal("new source inherited old exclusion")
				}
			case "reset":
				pending := fork.PrepareRecoveryNavigationRead(path)
				bus.Mutable.ResetTurnAArtifacts()
				if fork.RecordValidatedRecoveryNavigation(context.Background(), pending) {
					t.Fatal("old fork revived revoked epoch")
				}
				if fork.IsRecoveryNavigationMaterial(bus.RepoRoot, path) {
					t.Fatal("old fork retained revoked role")
				}
			case "json_restore":
				wire, err := json.Marshal(bus.Mutable)
				if err != nil {
					t.Fatal(err)
				}
				var restored types.MutableState
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				bus.Mutable = &restored
			case "dispatch_reset":
				bus.Mutable.ResetDispatchToolResults()
				want = true
			}
			if got := bus.Mutable.IsRecoveryNavigationMaterial(bus.RepoRoot, path); got != want {
				t.Fatalf("role=%v want %v", got, want)
			}
		})
	}
}

func TestHMC258RecoveryNavigationPartialCanceledAndForgedHints(t *testing.T) {
	for _, action := range []string{"partial", "canceled_commit", "changed_before_commit", "retarget_before_commit", "forged_hint"} {
		t.Run(action, func(t *testing.T) {
			bus, path := recoveryNavigationFixture258(t)
			switch action {
			case "partial":
				raw, _ := json.Marshal(map[string]any{"path": path, "line_offset": 1, "limit": 2})
				out, err := (&ReadFile{}).Execute(bus, raw)
				if err != nil || !out.Success {
					t.Fatal(err, out.Summary)
				}
				if out.ReadCoverage == nil {
					t.Fatal("partial read made unproven material decision")
				}
			case "canceled_commit":
				pending := bus.Mutable.PrepareRecoveryNavigationRead(path)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if bus.Mutable.RecordValidatedRecoveryNavigation(ctx, pending) {
					t.Fatal("canceled read registered")
				}
			case "changed_before_commit":
				pending := bus.Mutable.PrepareRecoveryNavigationRead(path)
				if err := os.WriteFile(path, []byte(`{"workers":8}`), 0600); err != nil {
					t.Fatal(err)
				}
				if bus.Mutable.RecordValidatedRecoveryNavigation(context.Background(), pending) {
					t.Fatal("changed read registered")
				}
			case "forged_hint":
				bus.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "read_file", Success: true, Refinement: &types.ToolRefinementHint{ReasonCode: "historical_catalog_navigation"}})
			case "retarget_before_commit":
				alias := filepath.Join(bus.RepoRoot, "current.json")
				if err := os.Symlink(path, alias); err != nil {
					t.Fatal(err)
				}
				pending := bus.Mutable.PrepareRecoveryNavigationRead(alias)
				other := filepath.Join(bus.RepoRoot, "other.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(other, data, 0600); err != nil {
					t.Fatal(err)
				}
				replacement := filepath.Join(bus.RepoRoot, "new-link")
				if err := os.Symlink(other, replacement); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, alias); err != nil {
					t.Fatal(err)
				}
				if bus.Mutable.RecordValidatedRecoveryNavigation(context.Background(), pending) {
					t.Fatal("retargeted read qualified unread old object")
				}
			}
			if bus.Mutable.IsRecoveryNavigationMaterial(bus.RepoRoot, path) {
				t.Fatal("unwitnessed role accepted")
			}
		})
	}
}

func TestHMC258RecoveryNavigationMixedSourceAndRuntimeStayIndependent(t *testing.T) {
	bus, path := recoveryNavigationFixture258(t)
	readCheckpoint247(t, bus, path)
	queried := catalogPublicQuery(t, bus, filepath.Join(bus.RepoRoot, "capture.data"), 101)
	if !queried.Success || len(queried.Observations) == 0 {
		t.Fatal("fixture must have current native observations", queried.Summary)
	}
	bus.Mutable.AppendDispatchToolResult(queried)
	source := filepath.Join(bus.RepoRoot, "workers.json")
	if err := os.WriteFile(source, []byte(`{"workers":8}`), 0600); err != nil {
		t.Fatal(err)
	}
	bus.Mutable.AppendDispatchToolResult(readCheckpoint247(t, bus, source))
	bus.EvidenceItems = []types.EvidenceItem{
		{ID: "navigation", Source: "settings.json", LineStart: 1, Scope: types.ScopeLine, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		{ID: "source", Source: "workers.json", LineStart: 1, Scope: types.ScopeLine, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	seenSource, seenNav, seenRuntime := false, false, false
	for _, r := range ledger.Records {
		if r.SourceRef.Path == "settings.json" {
			seenNav = true
			if r.SourceRef.Kind != types.ObservationSourceRecoveryNavigation {
				t.Fatal("navigation misqualified")
			}
		}
		if r.SourceRef.Path == "workers.json" && r.SourceRef.Kind == types.ObservationSourceCurrentSource {
			seenSource = true
		}
		if r.SourceRef.Kind == types.ObservationSourceRuntimeArtifact {
			seenRuntime = true
		}
	}
	if !seenSource || !seenNav || !seenRuntime {
		t.Fatal("mixed evidence lost an independent lane")
	}
	if _, ok := newPreEmitCheckContext(bus).citedEvidenceItems(types.Citation{File: "workers.json", Line: 1}); !ok {
		t.Fatal("real config citation lost")
	}
}

func TestHMC258RecoveryNavigationSourcePurposeAndWriteReadsRemainSource(t *testing.T) {
	for _, mode := range []string{"source", "mixed_source", "soft_fallback", "write", "write_after_navigation"} {
		t.Run(mode, func(t *testing.T) {
			bus, path := recoveryNavigationFixture258(t)
			if mode == "write_after_navigation" {
				readCheckpoint247(t, bus, path)
			}
			switch mode {
			case "source":
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentConfigQuery, AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{"settings.json"}}}}
				bus.TraceInputPreparer = nil
			case "mixed_source":
				bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"settings.json"}
				bus.AnalysisIR.RequestModel.CurrentSourceExplanationProfile.IsCurrentSourceExplanationRequested = true
			case "soft_fallback":
				bus.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = nil
				bus.TurnRouteHint = types.TurnRouteHint{Route: "repo", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			default:
				bus.Mode = types.ModeApply
			}
			out := readCheckpoint247(t, bus, path)
			if out.ReadCoverage == nil || len(out.Observations) == 0 {
				t.Fatal("snapshot format overrode actual source/write purpose")
			}
			if bus.Mode.IsWrite() {
				bus.Mutable.AppendDispatchToolResult(out)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !dispatchReadVersionComplete(t, bus, "settings.json", string(data)) {
					t.Fatal("write-mode complete source version lost")
				}
			}
		})
	}
}
