package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventMemberScopeHMC237ExactMirrorOnly(t *testing.T) {
	ctx, facts := eventMemberScopeHMC237Context(t)
	roster := answerDocEventMemberWindowRoster(ctx)
	outside := facts[5]
	if !roster.excludes(outside) {
		t.Fatal("native right-boundary mirror was not witnessed")
	}
	for name, change := range map[string]func(*types.ObservationRecord){
		"query":     func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += "other" },
		"payload":   func(r *types.ObservationRecord) { r.SourceRef.PayloadRef += "other" },
		"path":      func(r *types.ObservationRecord) { r.SourceRef.Path += "other" },
		"clock":     func(r *types.ObservationRecord) { r.SourceRef.TimeDomain = "other" },
		"capture":   func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath = "other" },
		"artifact":  func(r *types.ObservationRecord) { r.SourceRef.ArtifactID += "other" },
		"tool call": func(r *types.ObservationRecord) { r.SourceRef.ToolCallID += "other" },
		"line":      func(r *types.ObservationRecord) { r.Span.LineStart++; r.Span.LineEnd++ },
		"time":      func(r *types.ObservationRecord) { r.Span.StartTs++; r.Span.EndTs++ },
		"interval":  func(r *types.ObservationRecord) { r.Span.EndTs++ },
		"event type": func(r *types.ObservationRecord) {
			r.Predicate = "sched_wakeup"
			r.ClaimKey = "evidence_fact:" + r.Predicate
		},
		"causal lane": func(r *types.ObservationRecord) { r.ProvenanceLane = types.ObservationProvenanceObservedDirectCause },
		"other claim": func(r *types.ObservationRecord) { r.ClaimKey = "causal_predecessor" },
		"model":       func(r *types.ObservationRecord) { r.Producer = "extractor" },
	} {
		t.Run(name, func(t *testing.T) {
			unrelated := outside
			change(&unrelated)
			if roster.excludes(unrelated) {
				t.Fatal("unassociated observation borrowed event membership")
			}
		})
	}
	// Values/text have no role in a precise association. Even a conflicting
	// old model-facing summary cannot resurrect an excluded native member.
	outside.Summary = "some other prose"
	if !roster.excludes(outside) {
		t.Fatal("membership depended on prose")
	}
}

func TestEventMemberScopeHMC237BusinessAndCausalLanesUnchanged(t *testing.T) {
	for _, name := range []string{"business selector", "causal predecessor", "no finite profile", "explorer"} {
		t.Run(name, func(t *testing.T) {
			ctx, facts := eventMemberScopeHMC237Context(t)
			switch name {
			case "business selector":
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeBoundedSelector}
			case "causal predecessor":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "no finite profile":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
			case "explorer":
				ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
			}
			got, excluded := answerDocSelectedWindowObservationRecords(ctx, facts)
			if excluded != 0 || !reflect.DeepEqual(got, facts) {
				t.Fatal("presentation-only member roster changed an independent business/causal/exploration lane")
			}
		})
	}
}

func TestEventMemberScopeHMC237ObservedZeroIsNotUnknownOrderingZero(t *testing.T) {
	for _, kind := range []string{"trace_mark", "measure_interval", "process_measure_interval"} {
		row := types.TraceEventSearchInventoryRow{Line: 1, TraceTimeSeconds: 0, SourceTimeKnown: true, SourceTimeSeconds: 0, EventType: kind}
		if _, known := answerDocEventMemberIdentity("exact-source", row); !known {
			t.Errorf("%s: observed zero became unknown", kind)
		}
		if kind == "trace_mark" {
			continue
		}
		row.SourceTimeKnown = false
		if _, known := answerDocEventMemberIdentity("exact-source", row); known {
			t.Errorf("%s: unknown ordering zero acquired member authority", kind)
		}
		row.SourceTimeKnown, row.SourceTimeSeconds = true, 7
		if _, known := answerDocEventMemberIdentity("exact-source", row); known {
			t.Errorf("%s: synthetic zero borrowed unrelated reported time", kind)
		}
	}
	result := traceInventoryScopePublicQuery(t, "main-11 (11) [000] .... 0.000000: tracing_mark_write: I|11|observed-zero\nmain-11 (11) [000] .... 1.000000: tracing_mark_write: I|11|right-boundary\n", 0, 1, nil)
	ctx := traceInventoryScopeContext(result, 0, 1)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	roster := answerDocEventMemberWindowRoster(ctx)
	var retained, excluded int
	for _, record := range answerDocObservationLedger(ctx).Records {
		if record.ClaimKey != "evidence_fact:"+record.Predicate {
			continue
		}
		if roster.excludes(record) {
			excluded++
		} else {
			retained++
		}
	}
	if retained != 1 || excluded != 1 {
		t.Fatalf("public known-zero boundary mismatch: retained=%d excluded=%d", retained, excluded)
	}
}

func TestEventMemberScopeHMC237InvalidOriginalCannotExcludeMirrors(t *testing.T) {
	ctx, facts := eventMemberScopeHMC237Context(t)
	results := ctx.Mutable.TurnAArtifacts().ToolResults
	for n := range results[0].Observations {
		if results[0].Observations[n].Predicate == types.TraceEventSearchInventoryPredicate {
			// Keep the stale encoded publication intact: an unverified changed
			// source is not a permission to rebind legacy event mirrors.
			results[0].Observations[n].SourceRef.Path += ".changed"
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	if answerDocEventMemberWindowRoster(ctx).excludes(facts[5]) {
		t.Fatal("invalid original inventory excluded an independent mirror")
	}
}

func TestEventMemberScopeHMC237ComparisonUsesEachOriginalWindow(t *testing.T) {
	ctx, _, wanted := hmc225DisplayContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOccurrenceTime},
	}
	before := traceInventoryScopeAuthority(t, ctx)
	roster := answerDocEventMemberWindowRoster(ctx)
	var retained, excluded int
	for _, record := range answerDocObservationLedger(ctx).Records {
		if record.ClaimKey != "evidence_fact:"+record.Predicate {
			continue
		}
		if roster.excludes(record) {
			excluded++
			if wanted[record.ID] {
				t.Errorf("source-owned member borrowed peer window: %s", record.ID)
			}
		} else if wanted[record.ID] {
			retained++
		}
	}
	if retained == 0 || excluded == 0 {
		t.Fatalf("comparison did not exercise both native member sides: retained=%d excluded=%d", retained, excluded)
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("comparison display changed ledger/causal source ownership")
	}
}

func TestEventMemberScopeHMC237PartialInventoryCannotDecideUnseenMirror(t *testing.T) {
	ctx, facts := eventMemberScopeHMC237Context(t)
	results := ctx.Mutable.TurnAArtifacts().ToolResults
	for n, record := range results[0].Observations {
		if record.Predicate != types.TraceEventSearchInventoryPredicate {
			continue
		}
		record.EventSearchInventory = types.CloneTraceEventSearchInventory(record.EventSearchInventory)
		record.EventSearchInventory.Rows = record.EventSearchInventory.Rows[:5]
		record.EventSearchInventory.HandoffRowsOmitted = 1
		record.EventSearchInventory.RowsComplete = false
		publication, ok := types.BuildTraceEventRuntimeMeasurementPublication(record)
		if !ok {
			t.Fatal("partial native fixture was not internally consistent")
		}
		encoded, _ := json.Marshal(publication)
		notes := make([]string, 0, len(record.RichNotes))
		for _, note := range record.RichNotes {
			if !strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
				notes = append(notes, note)
			}
		}
		record.RichNotes = append(notes, types.TraceNoteKeyRuntimeMeasurement+"="+string(encoded))
		results[0].Observations[n] = record
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	roster := answerDocEventMemberWindowRoster(ctx)
	if len(roster) != 5 || roster.excludes(facts[5]) {
		t.Fatalf("partial inventory inferred an unseen member from a count: roster=%d", len(roster))
	}
}
