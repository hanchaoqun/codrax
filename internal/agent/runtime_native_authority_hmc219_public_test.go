package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Test the actual finalizer adapter, with native queries and the real accepted
// closure lifecycle. Identifiable prose is fixture data, never a production
// keyword rule. A runtime query does not turn unrelated advisory notes into
// evidence, and a current-source obligation does not hide artifact facts.
func TestHMC219NativeLogAuthorityActualFinalizer(t *testing.T) {
	for _, mode := range []types.TurnRouteCurrentSourceEvidenceMode{types.TurnRouteCurrentSourceEvidenceRequired, types.TurnRouteCurrentSourceEvidenceOptional} {
		t.Run(string(mode), func(t *testing.T) {
			var inputs []loginput.Input
			for _, relative := range []string{"app/session.log", "kernel/session.log.gz"} {
				path, err := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/" + relative)
				if err != nil {
					t.Fatal(err)
				}
				inputs = append(inputs, loginput.Input{Path: path})
			}
			catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
			if err != nil {
				t.Fatal(err)
			}
			bundle := &types.LogBundle{Errors: []types.LogError{{Type: "ResourceError", Message: "status=missing",
				Frames: []types.LogFrame{{Func: "loadCatalog", Raw: "at loadCatalog (AssetLoader.ets:42)"}}}}}
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, LogTriage: bundle,
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis}}
			bus := hmc219NativeBus(t, rm, mode)
			bus.AttachedLogCatalog = catalog
			bus.Mutable.SetLogTriage(bundle)
			var results []types.ToolResult
			for _, source := range catalog.Sources() {
				args, _ := json.Marshal(map[string]any{"source_ids": []string{source.ID}})
				result, err := (&tool.LogQuery{}).Execute(bus, args)
				if err != nil || !result.Success {
					t.Fatalf("actual log query: %v / %s", err, result.Summary)
				}
				results = append(results, types.AttachToolHandoffCarrier(result))
			}
			hmc219AcceptedLifecycle(bus.Mutable, results, true)
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			messages := hmc219AssertNativeAdvisoryHandoff(t, ctx, mode == types.TurnRouteCurrentSourceEvidenceRequired, false)
			records := map[string]map[string]any{}
			for _, object := range hmc218MessageJSONObjects(messages) {
				if id, ok := object["record_id"].(string); ok {
					records[id] = object
				}
			}
			if len(records) != 9 {
				t.Fatalf("native log roster=%d, want 9", len(records))
			}
			for _, result := range results {
				for _, row := range result.Observations {
					if row.Predicate != "log_record" {
						continue
					}
					var source map[string]any
					decoder := json.NewDecoder(strings.NewReader(row.Value))
					decoder.UseNumber()
					if err := decoder.Decode(&source); err != nil {
						t.Fatal(err)
					}
					got := records[source["record_id"].(string)]
					for _, field := range []string{"source_id", "first_line", "last_line", "pid", "tid", "level", "comm", "clock_domain", "status"} {
						if !reflect.DeepEqual(got[field], source[field]) {
							t.Errorf("native %s %s changed: got=%v want=%v", row.ID, field, got[field], source[field])
						}
					}
				}
			}
			var ids []string
			for _, result := range results {
				for _, row := range result.Observations {
					if row.Predicate == "log_record" {
						ids = append(ids, row.ID)
					}
				}
			}
			hmc219AssertNativeSupportReferences(t, messages, "log_record", ids)
		})
	}
}

func TestHMC219NativeMeasurementAuthorityActualFinalizer(t *testing.T) {
	for _, mode := range []types.TurnRouteCurrentSourceEvidenceMode{types.TurnRouteCurrentSourceEvidenceRequired, types.TurnRouteCurrentSourceEvidenceOptional} {
		t.Run(string(mode), func(t *testing.T) {
			start, end := 1.0, 2.0
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
					FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart: &start, TimeEnd: &end, Confidence: 1}}
			bus := hmc219NativeBus(t, rm, mode)
			bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(bus.WorkDir, ".codrax")})
			path, err := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": start, "time_end": end})
			result, err := (&tool.TraceQuery{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("actual measurement query: %v / %s", err, result.Summary)
			}
			hmc219AcceptedLifecycle(bus.Mutable, []types.ToolResult{types.AttachToolHandoffCarrier(result)}, false)
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			if len(choices) != 3 {
				t.Fatalf("native table oracle=%d, want summary/members/timeline", len(choices))
			}
			// Trace-only optional context already withholds unbound model
			// synthesis. The new mixed-source handoff must not undo that
			// safety boundary just to preserve prose claiming to be useful.
			suppress := mode == types.TurnRouteCurrentSourceEvidenceOptional
			if suppress && !runtimeObservationOnlyForAnswerDoc(ctx) {
				t.Fatal("optional negative control missed the existing observation-only boundary")
			}
			messages := hmc219AssertNativeAdvisoryHandoff(t, ctx, !suppress, suppress)
			previews := hmc218MeasurementMessagePreviews(t, messages)
			rows := 0
			for _, table := range choices {
				preview := previews[table.ObservationID+"/"+string(table.View)]
				if !reflect.DeepEqual(preview.Rows, table.Rows) || !reflect.DeepEqual(preview.Columns, table.Columns) || preview.OmittedRows != 0 {
					t.Errorf("native table changed or disappeared: %s/%s", table.ObservationID, table.View)
				}
				rows += len(preview.Rows)
			}
			if rows != 33 {
				t.Errorf("native complete preview=%d, want 33", rows)
			}
			var ids []string
			for _, table := range choices {
				ids = append(ids, table.ObservationID)
			}
			hmc219AssertNativeSupportReferences(t, messages, "trace_record", ids)
		})
	}
}

const hmc219CurrentClosure = "HMC219 current model synthesis: the observed values might describe a resource bottleneck; the unit and relationship still need proof."
const hmc219PriorClosure = "HMC219 superseded model synthesis: the earlier wider query was treated as the requested interval."

var hmc219IndependentNotes = []string{
	"HMC219 source note: current implementation has a separate input-validation branch; it does not prove this capture executed that branch.",
	"HMC219 negative search: no unique cross-source relationship has been established in the inspected material.",
	"HMC219 business hypothesis: inspect the resource-loading operation as a possible follow-up, not a proven cause.",
}

func hmc219NativeBus(t *testing.T, rm types.RequestModel, mode types.TurnRouteCurrentSourceEvidenceMode) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	mu := types.NewMutableState("Explain the supplied runtime observations and their supported relationships.")
	mu.SetRequestModel(rm)
	return &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu,
		AnalysisIR:    &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}},
		TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: mode}}
}

func hmc219AcceptedLifecycle(mu *types.MutableState, results []types.ToolResult, forkMerge bool) {
	prior := types.TurnAArtifacts{AcceptedClosureReason: hmc219PriorClosure, AcceptedResultKind: "resolved",
		InvestigationNotes: append([]string(nil), hmc219IndependentNotes...), ToolResults: results,
		HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)}
	mu.SetTurnAArtifacts(prior)
	if forkMerge {
		fork := mu.ForkForExploreDispatch()
		current := fork.TurnAArtifacts()
		current.AcceptedClosureReason = hmc219CurrentClosure
		fork.SetTurnAArtifacts(*current)
		mu.MergeExploreFork(fork)
	} else {
		current := prior
		current.AcceptedClosureReason = hmc219CurrentClosure
		mu.SetTurnAArtifacts(mergeTurnAArtifactsWithPrior(&prior, current))
	}
	mu.SetInvestigationComplete(hmc219CurrentClosure)
}

func hmc219AssertNativeAdvisoryHandoff(t *testing.T, ctx *types.AgentContext, preserveAdvisory, suppressUnbound bool) string {
	t.Helper()
	auditBefore, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if !bytes.Contains(auditBefore, []byte(hmc219PriorClosure)) {
		t.Fatal("actual lifecycle did not preserve the superseded closure for audit")
	}
	projectionBefore := hmc218CausalProjection(t, ctx)
	messages := dependencyObservationMessages(t, ctx)
	if count := strings.Count(messages, hmc219CurrentClosure); count > 1 || preserveAdvisory && count != 1 || suppressUnbound && count != 0 {
		t.Errorf("current model synthesis count=%d, preserve_advisory=%t suppress_unbound=%t", count, preserveAdvisory, suppressUnbound)
	}
	if strings.Contains(hmc219Section(messages, "## Accepted Closure Status"), hmc219CurrentClosure) {
		t.Error("accepted-status section replayed the model synthesis as a second fact authority")
	}
	if strings.Contains(messages, hmc219PriorClosure) {
		t.Error("superseded closure re-entered the actual answer context")
	}
	for _, note := range hmc219IndependentNotes {
		if count := strings.Count(messages, note); count > 1 || preserveAdvisory && count != 1 || suppressUnbound && count != 0 {
			t.Errorf("advisory note count=%d, preserve=%t suppress=%t: %s", count, preserveAdvisory, suppressUnbound, note)
		}
	}
	auditAfter, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if !bytes.Equal(auditBefore, auditAfter) || !bytes.Equal(projectionBefore, hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed durable audit data or Trace causal projection")
	}
	return messages
}

func hmc219AssertNativeSupportReferences(t *testing.T, messages, kind string, nativeIDs []string) {
	t.Helper()
	lanes := hmc219Section(messages, "## Typed Answer Support Lanes")
	refs := map[string]bool{}
	for _, object := range hmc218MessageJSONObjects(lanes) {
		if object["producer_kind"] != kind {
			continue
		}
		ids, ok := object["observation_ids"].([]any)
		if !ok {
			t.Errorf("native support references lack an observation_ids array: %+v", object)
			continue
		}
		for _, id := range ids {
			if id, ok := id.(string); ok {
				refs[id] = true
			}
		}
	}
	for _, id := range nativeIDs {
		if !refs[id] {
			t.Errorf("principal support boundary omits source-bound %s identity %s", kind, id)
		}
	}
	// The lane is a reference/permission boundary, not a second native table.
	if len(hmc218MeasurementMessagePreviews(t, lanes)) != 0 || strings.Contains(lanes, `"message_preview":`) {
		t.Error("support guidance copied native data instead of referring to its source-bound carrier")
	}
}

func hmc219Section(messages, heading string) string {
	_, section, ok := strings.Cut(messages, heading+"\n")
	if !ok {
		return ""
	}
	section, _, _ = strings.Cut(section, "\n## ")
	return fmt.Sprintf("%s\n%s", heading, section)
}
