package context_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the same public merge used after another sibling wins convergence.
// Catalog operations, native queries and Finalizer context are all real; no
// serialized catalog or fabricated query outcome supplies their private state.
func TestHMC288CompletedRepresentativeSelectionSurvivesSiblingHandoff(t *testing.T) {
	for _, mode := range []string{"ordinary_full_merge", "published_tools_only"} {
		t.Run(mode, func(t *testing.T) {
			root, work := t.TempDir(), t.TempDir()
			fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"one.data", "two.data", "three.data"} {
				if err := os.WriteFile(filepath.Join(root, name), fixture, 0600); err != nil {
					t.Fatal(err)
				}
			}
			parent := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("Compare representatives"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
			first, err := (&tool.TraceCatalog{}).Execute(parent, []byte(`{"queries":[{"view":"resource_stack","pid":101,"target_scope":"thread","time_start":10,"time_end":10.05}]}`))
			if err != nil || !first.Success {
				t.Fatalf("initial discovery: %v %s", err, first.Summary)
			}
			if len(parent.Mutable.TraceCatalogs()[0].Snapshot().Queries) != 3 {
				t.Fatal("initial plans must be three")
			}
			fork := parent.Mutable.ForkForExploreDispatch()
			child := *parent
			child.Mutable = fork
			selected, err := (&tool.TraceCatalog{}).Execute(&child, []byte(`{"queries":[{"view":"resource_stack","pid":202,"target_scope":"thread","time_start":10,"time_end":10.05}],"representatives":{"per_cluster":1}}`))
			if err != nil || !selected.Success {
				t.Fatalf("combined discover+representatives: %v %s", err, selected.Summary)
			}
			fork.AppendDispatchToolResult(selected)
			catalog := fork.TraceCatalogs()[0]
			selection, ok := catalog.Representatives()
			if !ok || selection.SelectionID == "" || len(catalog.Snapshot().Queries) != 6 {
				t.Fatal("fork selection not committed")
			}
			var page struct {
				Calls map[string]json.RawMessage `json:"query_calls"`
			}
			if err := json.Unmarshal([]byte(selected.Summary), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Calls) != 1 {
				t.Fatalf("selected calls = %d, want 1", len(page.Calls))
			}
			var queried types.ToolResult
			var executedID string
			for queryID, call := range page.Calls {
				executedID = queryID
				queried, err = (&tool.TraceQuery{}).Execute(&child, call)
				if err != nil || !queried.Success {
					t.Fatalf("native query: %v %s", err, queried.Summary)
				}
				fork.AppendDispatchToolResult(queried)
			}
			if mode == "ordinary_full_merge" {
				fork.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{selected, queried}})
				parent.Mutable.MergeExploreFork(fork)
			} else {
				retained := parent.Mutable.MergeExploreForkPublishedTools(fork)
				if len(retained) != 1 || !retained[0].Success {
					t.Fatalf("completed native query not retained: %d", len(retained))
				}
			}
			merged := parent.Mutable.TraceCatalogs()[0]
			mergedSelection, hasSelection := merged.Representatives()
			section := catalogFinalizerSectionHMC288(t, parent)
			if !hasSelection || mergedSelection.SelectionID != selection.SelectionID || len(merged.Snapshot().Queries) != 6 || !strings.Contains(section, selection.SelectionID) || strings.Contains(section, "No representative selection has occurred") {
				t.Fatalf("completed current-run selection lost while native data retained:\n%s", section)
			}
			before := merged.CurrentExecution()
			assertCatalogSingleSuccessHMC288(t, before, executedID)
			if mode == "published_tools_only" {
				if again := parent.Mutable.MergeExploreForkPublishedTools(fork); len(again) != 0 {
					t.Fatal("repeated merge duplicated successful producer data")
				}
				after := merged.CurrentExecution()
				assertCatalogSingleSuccessHMC288(t, after, executedID)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("repeated merge changed exact attempts, selection or source-local execution")
				}
			}
		})
	}
}

func assertCatalogSingleSuccessHMC288(t *testing.T, execution tracecatalog.ExecutionView, executedID string) {
	t.Helper()
	c := execution.Coverage
	if c.DeclaredPlans != 6 || c.ObservedQueries != 0 || c.ResumedPlans != 0 || c.Attempts != 1 || c.QueriesWithAttempts != 1 || c.QueriesWithoutAttempts != 5 || c.AttemptOutcomes[tracecatalog.OutcomeSuccess] != 1 || c.PlannedOutcomes[tracecatalog.OutcomeSuccess] != 1 || c.PlannedOutcomes[tracecatalog.OutcomeNotExecuted] != 5 {
		t.Fatalf("exact execution counts lost or unexecuted work promoted: %+v", c)
	}
	found := false
	for _, q := range execution.Snapshot.Queries {
		if q.ID == executedID {
			found = true
			if q.Outcome != tracecatalog.OutcomeSuccess || len(q.Attempts) != 1 || q.Attempts[0].Outcome != tracecatalog.OutcomeSuccess {
				t.Fatalf("actual selected query's success/attempt lost: %+v", q)
			}
		} else if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 {
			t.Fatalf("unexecuted query promoted or given another member's attempt: %+v", q)
		}
	}
	if !found {
		t.Fatal("executed exact query identity missing")
	}
}

func TestHMC288PureCompletedSelectionSurvivesSiblingHandoff(t *testing.T) {
	for _, mode := range []string{"ordinary_full_merge", "published_tools_only"} {
		t.Run(mode, func(t *testing.T) {
			root, work := t.TempDir(), t.TempDir()
			// Discovery and selection need readable candidates, not native
			// preparation. No query or producer measurement is executed here.
			if err := os.WriteFile(filepath.Join(root, "capture.data"), []byte("candidate material"), 0600); err != nil {
				t.Fatal(err)
			}
			parent := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("Select representatives")}
			first, err := (&tool.TraceCatalog{}).Execute(parent, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`))
			if err != nil || !first.Success {
				t.Fatalf("initial discover: %v %s", err, first.Summary)
			}
			fork := parent.Mutable.ForkForExploreDispatch()
			child := *parent
			child.Mutable = fork
			selected, err := (&tool.TraceCatalog{}).Execute(&child, []byte(`{"queries":[{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}],"representatives":{"per_cluster":1}}`))
			if err != nil || !selected.Success {
				t.Fatalf("combined selection: %v %s", err, selected.Summary)
			}
			fork.AppendDispatchToolResult(selected)
			selection, ok := fork.TraceCatalogs()[0].Representatives()
			if !ok || selection.SelectionID == "" {
				t.Fatal("selection not committed")
			}
			if mode == "ordinary_full_merge" {
				fork.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{selected}})
				parent.Mutable.MergeExploreFork(fork)
			} else if got := parent.Mutable.MergeExploreForkPublishedTools(fork); len(got) != 0 {
				t.Fatalf("navigation-only operation fabricated producer observations: %d", len(got))
			}
			c := parent.Mutable.TraceCatalogs()[0]
			merged, ok := c.Representatives()
			if c.CurrentExecution().Coverage.Attempts != 0 {
				t.Fatal("selection fabricated execution")
			}
			section := catalogFinalizerSectionHMC288(t, parent)
			if !ok || merged.SelectionID != selection.SelectionID || len(c.Snapshot().Queries) != 2 || !strings.Contains(section, selection.SelectionID) {
				t.Fatalf("successful pure navigation discarded:\n%s", section)
			}
		})
	}
}

func catalogCancelBoundaryFixtureHMC288(t *testing.T, pid int) (*types.BusContext, *types.BusContext, json.RawMessage) {
	t.Helper()
	root, work := t.TempDir(), t.TempDir()
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capture.data"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	parent := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("navigation boundaries"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	result, err := (&tool.TraceCatalog{}).Execute(parent, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !result.Success {
		t.Fatalf("discover: %v %s", err, result.Summary)
	}
	child := *parent
	child.Mutable = parent.Mutable.ForkForExploreDispatch()
	raw, _ := json.Marshal(map[string]any{"queries": []any{map[string]any{"view": "resource_stack", "pid": pid, "time_start": 10, "time_end": 10.05}}, "representatives": map[string]any{"per_cluster": 1}})
	selected, err := (&tool.TraceCatalog{}).Execute(&child, raw)
	if err != nil || !selected.Success {
		t.Fatalf("select: %v %s", err, selected.Summary)
	}
	child.Mutable.AppendDispatchToolResult(selected)
	var page struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(selected.Summary), &page); err != nil {
		t.Fatal(err)
	}
	for _, call := range page.Calls {
		return parent, &child, call
	}
	t.Fatal("missing selected query")
	return nil, nil, nil
}

func catalogFinalizerSectionHMC288(t *testing.T, bus *types.BusContext) string {
	t.Helper()
	pc := ctxbuilder.BuildPromptContext(ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), &skill.Config{Name: "cancelled-selection-public-repro"})
	return artifactDenialSection(t, pc, ctxbuilder.SectionRuntimeArtifactChoice)
}

func TestHMC288CancelledCatalogNavigationSafety(t *testing.T) {
	t.Run("failed_attempt_is_navigation_not_success", func(t *testing.T) {
		parent, child, call := catalogCancelBoundaryFixtureHMC288(t, 202)
		if err := os.Remove(filepath.Join(parent.RepoRoot, "capture.data")); err != nil {
			t.Fatal(err)
		}
		result, _ := (&tool.TraceQuery{}).Execute(child, call)
		if result.Success {
			t.Fatal("missing source query unexpectedly passed")
		}
		child.Mutable.AppendDispatchToolResult(result)
		if added := parent.Mutable.MergeExploreForkPublishedTools(child.Mutable); len(added) != 0 {
			t.Fatal("failed query became successful producer data")
		}
		current := parent.Mutable.TraceCatalogs()[0].CurrentExecution()
		if current.Coverage.Attempts != 1 || current.Coverage.AttemptOutcomes[tracecatalog.OutcomeSuccess] != 0 {
			t.Fatalf("failed attempt lost/inflated: %+v", current.Coverage)
		}
		if _, ok := parent.Mutable.TraceCatalogs()[0].Representatives(); !ok {
			t.Fatal("failed query erased prior selection")
		}
	})
	t.Run("empty_attempt_stays_empty_and_idempotent", func(t *testing.T) {
		parent, child, call := catalogCancelBoundaryFixtureHMC288(t, 999)
		result, err := (&tool.TraceQuery{}).Execute(child, call)
		if err != nil || !result.Success {
			t.Fatalf("empty query failed: %v %s", err, result.Summary)
		}
		child.Mutable.AppendDispatchToolResult(result)
		parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
		if again := parent.Mutable.MergeExploreForkPublishedTools(child.Mutable); len(again) != 0 {
			t.Fatal("duplicate evidence")
		}
		current := parent.Mutable.TraceCatalogs()[0].CurrentExecution()
		if current.Coverage.Attempts != 1 || current.Coverage.AttemptOutcomes[tracecatalog.OutcomeEmpty] != 1 {
			t.Fatalf("empty attempt lost/inflated: %+v", current.Coverage)
		}
	})
	t.Run("expired_generation_does_not_restore_catalog", func(t *testing.T) {
		parent, child, _ := catalogCancelBoundaryFixtureHMC288(t, 202)
		parent.Mutable.ResetTurnAArtifacts()
		parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
		if len(parent.Mutable.TraceCatalogs()) != 0 {
			t.Fatal("expired catalog resurrected")
		}
	})
	t.Run("foreign_mutable_run_does_not_restore_catalog", func(t *testing.T) {
		_, child, _ := catalogCancelBoundaryFixtureHMC288(t, 202)
		other := types.NewMutableState("unrelated current run")
		other.MergeExploreForkPublishedTools(child.Mutable)
		if len(other.TraceCatalogs()) != 0 {
			t.Fatal("foreign catalog imported")
		}
	})
	t.Run("navigation_never_accepts_model_completion", func(t *testing.T) {
		parent, child, _ := catalogCancelBoundaryFixtureHMC288(t, 202)
		child.Mutable.SetInvestigationComplete("untrusted sibling closure")
		child.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Label: "model conclusion", Value: "999"}})
		parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
		if parent.Mutable.IsInvestigationComplete() || parent.Mutable.StableInvestigationCompleteReason() != "" || len(parent.Mutable.StableInvestigationAggregateFacts()) != 0 {
			t.Fatal("model authority leaked")
		}
		if _, ok := parent.Mutable.TraceCatalogs()[0].Representatives(); !ok {
			t.Fatal("valid navigation lost")
		}
	})
}

func TestHMC288CatalogNavigationDoesNotInferSelectionFromJSON(t *testing.T) {
	parent, _, _ := catalogCancelBoundaryFixtureHMC288(t, 202)
	fork := parent.Mutable.ForkForExploreDispatch()
	// A plain summary is not a live selector receipt or current query.
	fork.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_catalog", Success: true, Summary: `{"selection_id":"forged","navigation_only":true,"coverage":{"selected":99}}`})
	if added := parent.Mutable.MergeExploreForkPublishedTools(fork); len(added) != 0 {
		t.Fatal("summary became producer fact")
	}
	c := parent.Mutable.TraceCatalogs()[0]
	if _, ok := c.Representatives(); ok {
		t.Fatal("unselected typed state gained selection from JSON")
	}
	if len(c.Snapshot().Queries) != 1 || c.CurrentExecution().Coverage.Attempts != 0 {
		t.Fatal("unselected state invented plans/attempts")
	}
}
