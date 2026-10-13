package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func eventPreflightNativeContext236(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	for n, app := range []int{27599, 27599, 41002, 41002} {
		ctx.AttachedHitrace += fmt.Sprintf("main-%d (%d) [002] .... 1.%03d000: tracing_mark_write: B|%d|jank_event_sync: start_ts=%d, end_ts=%d, jank_frames=%d, appid=%d\n", app, app, 100+n*100, app, 1000000000+n*100000000, 1020000000+n*100000000, n+3, app)
		ctx.AttachedHitrace += fmt.Sprintf("main-%d (%d) [002] .... 1.%03d000: tracing_mark_write: E|%d\n", app, app, 101+n*100, app)
	}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: filepath.Join(ctx.RepoRoot, "capture.tracebundle.json"), Carrier: "attachment"}}}
	args := json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"jank_event_sync","time_start":1,"time_end":1.5,"limit":40}`)
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("native attached query failed: %v / %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx
}

func TestNativeEventPublicationPreflightHandoffHMC236(t *testing.T) {
	ctx := eventPreflightNativeContext236(t)
	ledger := answerDocObservationLedger(ctx)
	record := traceInventoryScopeReceipt(t, ctx)
	var original types.ObservationRecord
	for _, row := range ctx.Mutable.TurnAArtifacts().ToolResults[0].Observations {
		if row.ID == record.ID {
			original = row
		}
	}
	if original.SourceRef.ArtifactID != "attached_trace" || !strings.HasPrefix(record.SourceRef.ArtifactID, "runtime_artifact:") || record.SourceRef.CaptureIdentityPath == "" || record.SourceRef.ToolCallID == "" {
		t.Fatalf("fixture missed actual preflight enrichment: before=%+v after=%+v", original.SourceRef, record.SourceRef)
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(original); !ok {
		t.Fatal("native input publication was not valid before ledger enrichment")
	}
	before, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts().ToolResults, ledger, types.CompileTraceCausalProjectionSet(ledger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"=") {
		t.Fatal("preflight-enriched receipt leaked complete publication into prompt")
	}
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || !reflect.DeepEqual(views[0].SelectorRowIDs, []string{"event-1", "event-2", "event-3", "event-4"}) || !reflect.DeepEqual(views[0].Source, record.SourceRef) {
		t.Fatalf("preflight broke native member selectors or rewrote enriched source: %+v", views)
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts().ToolResults, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("display bridge changed raw receipts, ledger or causal authority")
	}
}

func TestNativeEventPublicationPreflightBridgeExactRecordHMC236(t *testing.T) {
	ctx := eventPreflightNativeContext236(t)
	record := traceInventoryScopeReceipt(t, ctx)
	bridge := buildTraceEventPublicationBridge(ctx)
	if _, ok := traceEventInventoryVerifiedPublication(record, bridge); !ok {
		t.Fatal("exact compiled record did not acquire its original publication")
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"capture":    func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath += ".other" },
		"artifact":   func(r *types.ObservationRecord) { r.SourceRef.ArtifactID += "other" },
		"path":       func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"clock":      func(r *types.ObservationRecord) { r.SourceRef.TimeDomain = "other" },
		"window":     func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs++ },
		"query":      func(r *types.ObservationRecord) { r.EventSearchInventory.Query.Limit++ },
		"member":     func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].JankEvent.Values.AppID++ },
		"notes":      func(r *types.ObservationRecord) { r.RichNotes = append(r.RichNotes, "new caller note") },
		"tool order": func(r *types.ObservationRecord) { r.SourceRef.ToolCallID = "trace_query[9]" },
	} {
		t.Run(name, func(t *testing.T) {
			r := record
			r.EventSearchInventory = types.CloneTraceEventSearchInventory(record.EventSearchInventory)
			r.RichNotes = append([]string(nil), record.RichNotes...)
			mutate(&r)
			if _, ok := traceEventInventoryVerifiedPublication(r, bridge); ok {
				t.Fatal("changed ledger record borrowed source publication")
			}
			if !reflect.DeepEqual(traceEventInventoryPromptNotes(r, bridge), r.RichNotes) || traceEventInventorySelectorMembers(r, bridge) != nil {
				t.Fatal("failed exact bridge silently stripped notes or guessed selectors")
			}
		})
	}
	ctx.RuntimeArtifactPreflight.Artifacts[0].Source += ".other"
	if _, ok := traceEventInventoryVerifiedPublication(record, buildTraceEventPublicationBridge(ctx)); ok {
		t.Fatal("different preflight witness matched the prior capture")
	}
}

func TestNativeEventPublicationReplayIsInventoryOnlyAndOrderPreservingHMC236(t *testing.T) {
	ctx := eventPreflightNativeContext236(t)
	result := ctx.Mutable.TurnAArtifacts().ToolResults[0]
	result.Summary = "unrelated large summary"
	input := []types.ToolResult{{ToolName: "other", Success: true, Summary: "source context"}, result}
	before, _ := json.Marshal(input)
	lean := traceEventPublicationReplayResults(input)
	if len(lean) != 2 || lean[0].Success || lean[1].Summary != "" || lean[1].ToolName != "trace_query" || lean[1].RawRef != result.RawRef || !lean[1].Timestamp.Equal(result.Timestamp) || len(lean[1].Observations) != 1 {
		t.Fatalf("replay changed indexing or retained unrelated carriers: %+v", lean)
	}
	for _, row := range lean[1].Observations {
		if row.Predicate != types.TraceEventSearchInventoryPredicate {
			t.Fatal("replay included unrelated observations")
		}
	}
	inputLedger := types.CompileObservationLedger(types.ObservationLedgerInput{RepoRoot: ctx.RepoRoot, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight, ToolResults: input})
	leanLedger := types.CompileObservationLedger(types.ObservationLedgerInput{RepoRoot: ctx.RepoRoot, RuntimeArtifactPreflight: ctx.RuntimeArtifactPreflight, ToolResults: lean})
	var want, got types.ObservationRecord
	for _, row := range inputLedger.Records {
		if row.Predicate == types.TraceEventSearchInventoryPredicate {
			want = row
		}
	}
	for _, row := range leanLedger.Records {
		if row.Predicate == types.TraceEventSearchInventoryPredicate {
			got = row
		}
	}
	if !reflect.DeepEqual(want, got) || got.SourceRef.ToolCallID != "trace_query[1]" {
		t.Fatal("narrow replay changed the compiler's exact decoration order")
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("replay changed original tool results")
	}
}

func TestNativeEventPublicationPreflightCannotRehabilitateInvalidOriginalHMC236(t *testing.T) {
	ctx := eventPreflightNativeContext236(t)
	compiled := traceInventoryScopeReceipt(t, ctx)
	results := append([]types.ToolResult(nil), ctx.Mutable.TurnAArtifacts().ToolResults...)
	results[0].Observations = append([]types.ObservationRecord(nil), results[0].Observations...)
	for n, record := range results[0].Observations {
		if record.ID != compiled.ID {
			continue
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(record)
		if !ok {
			t.Fatal("fixture was not originally verified")
		}
		// Forge the publication's source to the future preflight identity,
		// without changing the actual original producer's source receipt.
		publication.Source = compiled.SourceRef
		publication.Source.ToolCallID = ""
		raw, _ := json.Marshal(publication)
		record.RichNotes = append([]string(nil), record.RichNotes...)
		for k, note := range record.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
				record.RichNotes[k] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
			}
		}
		if _, valid := types.DecodeRuntimeMeasurementPublication(record); valid {
			t.Fatal("forged original source unexpectedly validated")
		}
		results[0].Observations[n] = record
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	compiled = traceInventoryScopeReceipt(t, ctx)
	if _, ok := traceEventInventoryVerifiedPublication(compiled); !ok {
		t.Fatal("negative control did not reproduce the tempting post-enrichment match")
	}
	bridge := buildTraceEventPublicationBridge(ctx)
	if bridge == nil || len(bridge) != 0 {
		t.Fatal("invalid originals must publish an explicitly empty bridge")
	}
	if _, ok := traceEventInventoryVerifiedPublication(compiled, bridge); ok {
		t.Fatal("preflight rehabilitated an invalid original publication")
	}
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].SelectorRowIDs) != 0 || !strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"=") {
		t.Fatal("invalid original acquired selectors or had its unverified note silently stripped")
	}
}
