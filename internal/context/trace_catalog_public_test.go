package context_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Public discovery/planning and actual native queries share only navigation
// identities. The finalizer must receive the full status roster without
// promoting planned/missing objects or a catalog row into measured evidence.
func TestTraceCatalogPublicNativeObjectsReachFinalizerContextWithoutCrossBinding(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(root, "capture.data"), filepath.Join(root, "nested/capture.data")}
	for _, path := range paths {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	start, end := 10.0, 10.05
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	bus := &types.BusContext{RepoRoot: root, WorkDir: work, Language: "en", Mutable: types.NewMutableState("Inspect resource events for threads 101 and 999 in both captures"),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")}), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	bus.Mutable.SetRequestModel(rm)
	queryPlans := []map[string]any{{"view": "resource_stack", "pid": 101, "time_start": start, "time_end": end}, {"view": "resource_stack", "pid": 999, "time_start": start, "time_end": end}}
	raw, _ := json.Marshal(map[string]any{"queries": queryPlans})
	discovery, err := (&tool.TraceCatalog{}).Execute(bus, raw)
	if err != nil || !discovery.Success {
		t.Fatalf("public discover: %v %+v", err, discovery)
	}
	if len(discovery.Observations) != 0 || discovery.ReadCoverage != nil {
		t.Fatal("discovery acquired evidence authority")
	}
	catalog := bus.Mutable.TraceCatalogs()[0]
	if len(catalog.Snapshot().Queries) != 4 {
		t.Fatal("expected object roster not frozen")
	}
	beforeLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	for _, record := range beforeLedger.Records {
		if record.Producer == "trace_catalog" || record.Predicate == tool.TraceResourceStackPredicate {
			t.Fatal("catalog plan became resource fact")
		}
	}
	var results []types.ToolResult
	// Deliberately reverse source and target order; leave one expected object
	// unexecuted. Both same-named captures contain identical native event IDs.
	for _, call := range []struct {
		path string
		pid  int
	}{{paths[1], 999}, {paths[0], 101}, {paths[1], 101}} {
		raw, _ := json.Marshal(map[string]any{"source": "path", "path": call.path, "view": "resource_stack", "pid": call.pid, "time_start": start, "time_end": end})
		result, err := (&tool.TraceQuery{}).Execute(bus, raw)
		if err != nil || !result.Success {
			t.Fatalf("public native query: %v %+v", err, result)
		}
		found := false
		for _, record := range result.Observations {
			p, ok := tool.DecodeTraceResourceStack(record)
			if !ok {
				continue
			}
			found = true
			if p.TargetPID != call.pid || p.Window.StartTs != start || p.Window.EndTs != end || p.Window.EndInclusive {
				t.Fatalf("source/object/window altered: %+v", p)
			}
			want := 3
			if call.pid == 999 {
				want = 0
			}
			if p.MatchedEvents != want {
				t.Fatalf("wrong population: %d want %d", p.MatchedEvents, want)
			}
			for _, event := range p.Events {
				if event.Source.TID != call.pid {
					t.Fatal("neighbor owner entered resource facts")
				}
			}
		}
		if !found {
			t.Fatal("actual native producer missing")
		}
		results = append(results, result)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	ledgerBytes, _ := json.Marshal(ledger)
	producers := map[string]bool{}
	for _, record := range ledger.Records {
		if record.Producer == "trace_catalog" {
			t.Fatal("navigation became measured observation")
		}
		if _, ok := tool.DecodeTraceResourceStack(record); ok {
			producers[record.SourceRef.Path] = true
		}
	}
	if len(producers) != 2 {
		t.Fatalf("same-named independent sources collapsed: %+v", producers)
	}
	states := map[tracecatalog.Outcome]int{}
	for _, q := range catalog.Snapshot().Queries {
		states[q.Outcome]++
	}
	if states[tracecatalog.OutcomeSuccess] != 2 || states[tracecatalog.OutcomeEmpty] != 1 || states[tracecatalog.OutcomeNotExecuted] != 1 {
		t.Fatalf("status census lost: %+v", states)
	}
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageFinalize} {
		ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
		pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "catalog-context-test"})
		section := artifactDenialSection(t, pc, ctxbuilder.SectionRuntimeArtifactChoice)
		for _, want := range []string{catalog.ID(), `"success":2`, `"empty":1`, `"not_executed":1`, paths[0], paths[1], "navigation", "10050000000"} {
			if !strings.Contains(section, want) {
				t.Errorf("%s context lost %q: %s", stage, want, section)
			}
		}
		for _, forbidden := range []string{"ImageCache::reserve", "UIFrame::render", "priority_inversion"} {
			if strings.Contains(section, forbidden) {
				t.Errorf("catalog navigation leaked/created measured or causal claim %q", forbidden)
			}
		}
	}
	afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	afterBytes, _ := json.Marshal(afterLedger)
	if !bytes.Equal(ledgerBytes, afterBytes) {
		t.Fatal("context construction mutated actual query evidence")
	}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, body) {
			t.Fatal("catalog/native query changed source capture")
		}
	}
}
