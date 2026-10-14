package agent

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventParticipantPublicInventoryAndNativeSelectorHMC240(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n"+
			"other-12 (10) [001] .... 1.003000: sched_wakeup: comm=other pid=99 prio=120 target_cpu=1\n"+
			"record-11 (10) [000] .... 1.010000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n", 1, 1.01, nil)
	ctx := traceInventoryScopeContext(result, 1, 1.01)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Thread: "target-42", Source: "user_explicit"}}
	original := traceInventoryScopeReceipt(t, ctx)
	before := traceInventoryScopeAuthority(t, ctx)
	if len(original.EventSearchInventory.Rows) != 3 {
		t.Fatal("fixture requires native inclusive lookup superset")
	}
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 1 || len(views[0].Inventory.Rows) != 1 || !reflect.DeepEqual(views[0].SelectorRowIDs, []string{"event-1"}) {
		t.Errorf("finite prompt did not retain only the exact in-window participant: %+v", views)
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	if contract == nil || len(contract.Tables) != 1 || contract.Tables[0].EventData == nil {
		t.Fatal("native event publication missing")
	}
	data := contract.Tables[0].EventData
	if len(data.Rows) != 1 || data.Rows[0].ID != "event-1" {
		t.Errorf("native selector uses a different target member set: %+v", data.Rows)
	}
	for _, id := range []string{"event-2", "event-3"} {
		r := &types.AnswerRuntimeMeasurementReceipt{ObservationID: original.ID, View: types.RuntimeMeasurementMembers, EventSelection: &types.RuntimeEventSelection{RowIDs: []string{id}}}
		if types.BindRuntimeMeasurementReceipt(r, contract) {
			t.Errorf("selector recovered excluded member %s", id)
		}
	}
	ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: original.ID, View: types.RuntimeMeasurementMembers, EventSelection: &types.RuntimeEventSelection{FieldKeys: []string{"event.trace_time_seconds", "emitter.tid", "scheduler.wakee.tid"}}}
	if !types.BindRuntimeMeasurementReceipt(ref, contract) || !reflect.DeepEqual(ref.BoundTable.Rows, [][]string{{"1.001", "11", "42"}}) {
		t.Fatalf("typed row tuple changed: %+v", ref.BoundTable)
	}
	notes := strings.Join(ref.BoundTable.Notes, " ")
	for _, want := range []string{"查询匹配：3", "另有1条查询记录不满足请求时间窗", "另有1条查询记录未验证为请求主体"} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing exact selection note %q: %s", want, notes)
		}
	}
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		answer := render.RenderAnswerDocumentSurfaces(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: ref}}}, "zh").Answer
		for _, want := range []string{"1.001", "11", "42", "唤醒目标线程编号"} {
			if !strings.Contains(answer, want) {
				t.Errorf("%s lost native tuple member %q: %s", kind, want, answer)
			}
		}
		if kind == types.BlockDiagram && strings.Contains(answer, "-->") {
			t.Fatal("participant display minted a causal/relation edge")
		}
	}
	group := &types.AnswerRuntimeMeasurementReceipt{ObservationID: original.ID, View: types.RuntimeMeasurementMembers, EventSelection: &types.RuntimeEventSelection{GroupBy: []string{"scheduler.wakee.tid"}}}
	if !types.BindRuntimeMeasurementReceipt(group, contract) || !reflect.DeepEqual(group.BoundTable.Rows, [][]string{{"42", "1"}}) {
		t.Fatal("group borrowed whole-query population")
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("participant display changed original ledger or causal projection")
	}
}

func TestEventParticipantPublicCrossReceiptAndProcessHMC240(t *testing.T) {
	base := traceInventoryScopePublicQuery(t, "record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n", 1, 1.01, nil)
	for name, change := range map[string]func(*types.ObservationRecord){
		"no_typed_receipt":   func(r *types.ObservationRecord) { r.RichNotes = nil },
		"different_query":    func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += "other" },
		"different_payload":  func(r *types.ObservationRecord) { r.SourceRef.PayloadRef += "other" },
		"different_capture":  func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath = "/another.trace" },
		"process_not_thread": nil,
	} {
		t.Run(name, func(t *testing.T) {
			result := base
			result.Observations = append([]types.ObservationRecord(nil), base.Observations...)
			for n := range result.Observations {
				if result.Observations[n].Predicate == "scheduler_wakeup_event" && change != nil {
					change(&result.Observations[n])
				}
			}
			ctx := traceInventoryScopeContext(result, 1, 1.01)
			kind := types.RuntimeTargetKindThread
			if change == nil {
				kind = types.RuntimeTargetKindProcess
			}
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: kind, PID: 42, Thread: "target-42", Source: "user_explicit"}}
			before := traceInventoryScopeAuthority(t, ctx)
			views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
			if len(views) != 1 || len(views[0].Inventory.Rows) != 0 {
				t.Fatalf("unqualified participant leaked into prompt: %+v", views)
			}
			c := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromAgentContext(ctx, 0))
			if c == nil || len(c.Tables) != 1 || len(c.Tables[0].EventData.Rows) != 0 {
				t.Fatal("native selection borrowed incompatible participant receipt")
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("negative display mutated original evidence")
			}
		})
	}
}

func TestEventParticipantAttachedPreflightAndMissingRawHMC240(t *testing.T) {
	ctx := traceEventInventoryPublicContext(nil)
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.AttachedHitrace = "record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=0\n"
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: filepath.Join(ctx.RepoRoot, "original.trace"), Carrier: "attachment"}}}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"event_search","time_start":1,"time_end":1.01,"pid":42}`))
	if err != nil || !result.Success {
		t.Fatalf("attached query failed: %v %s", err, result.Summary)
	}
	for n := range result.Observations {
		r := &result.Observations[n]
		if !types.IsValidTraceEventSearchInventoryRecord(*r) {
			continue
		}
		r.EventSearchInventory = types.CloneTraceEventSearchInventory(r.EventSearchInventory)
		r.EventSearchInventory.Rows[0].Raw = ""
		p, ok := types.BuildTraceEventRuntimeMeasurementPublication(*r)
		if !ok {
			t.Fatal("raw-less typed inventory cannot publish")
		}
		wire, _ := json.Marshal(p)
		for k, note := range r.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
				r.RichNotes[k] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(wire)
			}
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	request := traceInventoryScopeContext(result, 1, 1.01).AnalysisIR.RequestModel
	request.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Source: "user_explicit"}}
	ctx.AnalysisIR.RequestModel = request
	original := traceInventoryScopeReceipt(t, ctx)
	if original.SourceRef.CaptureIdentityPath == "" || original.SourceRef.ToolCallID == "" {
		t.Fatal("preflight identity not exercised")
	}
	views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
	if len(views) != 1 || len(views[0].Inventory.Rows) != 1 || !reflect.DeepEqual(views[0].SelectorRowIDs, []string{"event-1"}) {
		t.Fatalf("preflight or missing raw lost exact participant: %+v", views)
	}
	c := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	if c == nil || len(c.Tables) != 1 || len(c.Tables[0].EventData.Rows) != 1 {
		t.Fatal("preflight native display mismatch")
	}
}
