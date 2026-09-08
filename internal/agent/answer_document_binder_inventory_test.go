package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1607BinderPromptRecords() []types.ObservationRecord {
	ref := types.ObservationSourceRef{ArtifactID: "capture-A"}
	return []types.ObservationRecord{
		{ID: "inventory", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: ref, Subject: "client-100", Predicate: "target_binder_wait_inventory", Object: "indexed_target_verified_closed_waits",
			Span: types.ObservationSpan{StartTs: 10, EndTs: 11}, Value: "3.094", Unit: "ms",
			Summary:   "Verified closed Binder waits: 5, union=3.094ms; unresolved=0, unassociated=60. Index scan complete; not all waits or roots.",
			RichNotes: []string{types.TraceNoteKeySelectedWindow + "=10.000000..11.000000"}},
		{ID: "chosen-wait", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: ref, Subject: "client-100", Predicate: "critical_blocking", Object: "binder_wait",
			Span: types.ObservationSpan{StartTs: 10.1, EndTs: 10.101409}, Value: "1.409", Unit: "ms",
			RichNotes: []string{types.TraceNoteKeySelectedWindow + "=10.000000..11.000000", types.TraceNoteKeyType + "=binder_wait", types.TraceNoteKeyCapacityTruncated + "=true"}},
	}
}

func TestB1607BinderInventoryZeroFromActualToolSurvivesBothHandoffs(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) {
			dir := t.TempDir()
			trace := "idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120\n"
			if pending {
				trace += "client-100 (100) [000] .... 10.000100: binder_transaction: transaction=1 dest_node=1 dest_proc=200 dest_thread=200 reply=0 flags=0x0 code=0x1\n"
			}
			trace += "client-100 (100) [000] .... 10.001000: sched_switch: prev_comm=client prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=200 next_prio=120\nworker-200 (200) [000] .... 10.002000: sched_wakeup: comm=client pid=100 prio=120 target_cpu=0\nworker-200 (200) [000] .... 10.003000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120\n"
			path := filepath.Join(dir, "zero.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "time_start": 10.0, "time_end": 10.004})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("query failed %v %s", err, result.Summary)
			}
			ctx := tracePrincipalValueAuthorityTestContext("client-100", 100, result.Observations)
			for name, got := range map[string]string{"observation": renderAnswerDocObservationLedger(ctx), "recap": renderAnswerDocTracePrincipalValueAuthority(ctx)} {
				if !strings.Contains(got, "verified_wait_union=0.000ms") || !strings.Contains(got, "not all waits or roots") {
					t.Errorf("%s lost the actual zero-confirmed inventory", name)
				}
				if pending && !strings.Contains(got, "unresolved=1") {
					t.Errorf("%s lost unresolved candidate", name)
				}
			}
		})
	}
}

func TestB1607BinderInventoryPromptSourceScopeAndCaps(t *testing.T) {
	ctx := tracePrincipalValueAuthorityTestContext("client-100", 100, nil)
	rm := &ctx.AnalysisIR.RequestModel
	set := b1607BinderPromptRecords()[0]
	runScoped := set
	runScoped.Producer = "trace_query:run2"
	if got := renderAnswerDocBinderInventory(types.ObservationLedger{Records: []types.ObservationRecord{runScoped}}, rm, "en"); !strings.Contains(got, "verified_wait_union=3.094ms") {
		t.Fatal("run-scoped deterministic producer lost verified inventory")
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{set, set}}
	for i := 0; i < 10; i++ {
		row := set
		row.ID, row.Predicate, row.Object = fmt.Sprintf("wait-%d", i), "target_binder_wait_interval", "verified_reply_wakeup"
		row.Span.StartTs, row.Span.EndTs = 10+float64(i)/100, 10+float64(i+1)/100
		row.Summary = fmt.Sprintf("unique verified interval %d", i)
		ledger.Records = append(ledger.Records, row)
	}
	got := renderAnswerDocBinderInventory(ledger, rm, "en")
	if strings.Count(got, "verified_wait_union=3.094ms") != 1 || !strings.Contains(got, "8/10 available evidence rows") || strings.Contains(got, "unique verified interval 8") {
		t.Fatalf("recap caps or exact duplicate identity lost: %s", got)
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"model":      func(r *types.ObservationRecord) { r.Producer = "model" },
		"origin":     func(r *types.ObservationRecord) { r.Origin = types.AnswerEvidenceOriginCurrentSource },
		"role":       func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer },
		"target":     func(r *types.ObservationRecord) { r.Subject = "other-200" },
		"no capture": func(r *types.ObservationRecord) { r.SourceRef = types.ObservationSourceRef{} },
		"value":      func(r *types.ObservationRecord) { r.Value = "NaN" },
		"scope":      func(r *types.ObservationRecord) { r.Object = "chain_candidates" },
	} {
		t.Run(name, func(t *testing.T) {
			r := set
			mutate(&r)
			if got := renderAnswerDocBinderInventory(types.ObservationLedger{Records: []types.ObservationRecord{r}}, rm, "en"); got != "" {
				t.Fatalf("unqualified inventory entered focused recap: %s", got)
			}
		})
	}
	block := types.TraceBlockingWallClockAuthority{Type: "binder_wait", Subject: set.Subject, ArtifactLabel: "capture-A", SelectedWindow: "10.000000..11.000000", Occurrences: []types.TraceBlockingWallClockOccurrence{{RecordIDs: []string{"inventory"}}}}
	if !traceBinderInventoryMatchesBlocking([]types.ObservationRecord{set}, block, ledger) {
		t.Fatal("same target/capture/window not matched")
	}
	for _, field := range []string{"capture", "target", "window", "type"} {
		other := block
		switch field {
		case "capture":
			other.Occurrences = []types.TraceBlockingWallClockOccurrence{{RecordIDs: []string{"missing-capture-record"}}}
		case "target":
			other.Subject = "client-200"
		case "window":
			other.SelectedWindow = "9.000000..11.000000"
		case "type":
			other.Type = "io_wait"
		}
		if traceBinderInventoryMatchesBlocking([]types.ObservationRecord{set}, other, ledger) {
			t.Fatalf("%s crossed scope", field)
		}
	}
	changed := set
	changed.ID, changed.Value, changed.Summary = "conflicting measurement", "0", "Zero confirmed, not proof of absence"
	if got := renderAnswerDocBinderInventory(types.ObservationLedger{Records: []types.ObservationRecord{set, changed}}, rm, "en"); !strings.Contains(got, "3.094ms") || !strings.Contains(got, "verified_wait_union=0ms") {
		t.Fatal("conflicting scope measurements silently elected or zero suppressed")
	}
}

func TestB1607BinderInventoryBothPromptEntrypointsKeepSeparateScopes(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		ctx := tracePrincipalValueAuthorityTestContext("client-100", 100, b1607BinderPromptRecords())
		ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
		model := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned conclusion stays unchanged."}}}
		ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, model)
		ledger := answerDocObservationLedger(ctx)
		before, _ := json.Marshal([]any{ledger, model, types.CompileTraceCausalProjectionSet(ledger), types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)})
		for name, rendered := range map[string]string{
			"handoff":   renderAnswerDocObservationLedger(ctx),
			"recap":     renderAnswerDocTracePrincipalValueAuthority(ctx),
			"finalizer": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
		} {
			if !strings.Contains(rendered, "3.094ms") || !strings.Contains(rendered, "1.409ms") {
				t.Errorf("%s/%s lost either measured account", name, lang)
			}
			want := "different selection and proof scopes"
			if lang == "zh" {
				want = "不同的筛选和证明范围"
			}
			if !strings.Contains(rendered, want) {
				t.Errorf("%s/%s omitted exact inventory vs chosen-chain boundary", name, lang)
			}
		}
		afterLedger := answerDocObservationLedger(ctx)
		after, _ := json.Marshal([]any{afterLedger, ctx.Mutable.AnswerDocumentV2(), types.CompileTraceCausalProjectionSet(afterLedger), types.BuildTraceBlockingWallClockAuthorities(afterLedger, &ctx.AnalysisIR.RequestModel)})
		if string(before) != string(after) {
			t.Fatal("prompt changed model answer or numeric/projection authority")
		}
	}
}
