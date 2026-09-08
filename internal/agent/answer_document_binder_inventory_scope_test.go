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

func b1607ScopeTraceFile(t *testing.T, dir, name string, peer int) string {
	t.Helper()
	// One complete request/reply/wakeup cohort. Distinct peer identities make
	// cross-capture borrowing observable without changing either wait total.
	trace := fmt.Sprintf(`target-41 (41) [000] .... 10.000900: binder_transaction: transaction=7 dest_proc=%[1]d dest_thread=%[1]d reply=0 flags=0x0 code=0x1
target-41 (41) [000] .... 10.001000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=server next_pid=%[1]d next_prio=120
server-%[1]d (%[1]d) [000] .... 10.001010: binder_transaction_received: transaction=7
server-%[1]d (%[1]d) [000] .... 10.001150: binder_transaction: transaction=8 dest_proc=41 dest_thread=41 reply=1 flags=0x0 code=0x0
server-%[1]d (%[1]d) [000] .... 10.001200: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000
server-%[1]d (%[1]d) [000] .... 10.001250: sched_switch: prev_comm=server prev_pid=%[1]d prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... 10.001300: binder_transaction_received: transaction=8
`, peer)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func b1607ScopeQuery(t *testing.T, dir, path string, start, end float64) []types.ObservationRecord {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "window_stats", "pid": 41,
		"time_start": start, "time_end": end,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("actual trace_query failed: %v; %s", err, result.Summary)
	}
	return result.Observations
}

func b1607ScopeInventoryAndRow(t *testing.T, records []types.ObservationRecord) (types.ObservationRecord, types.ObservationRecord) {
	t.Helper()
	var set, row types.ObservationRecord
	sets, rows := 0, 0
	for _, r := range records {
		switch r.Predicate {
		case "target_binder_wait_inventory":
			set, sets = r, sets+1
		case "target_binder_wait_interval":
			row, rows = r, rows+1
		}
	}
	if sets != 1 || rows != 1 || set.ResultCount == nil || *set.ResultCount != 1 || set.Value != "0.200" {
		t.Fatalf("fixture did not publish one real 0.200ms closed wait: sets=%d rows=%d set=%+v", sets, rows, set)
	}
	return set, row
}

func TestB1607BinderInventoryActualCapturePathsRemainDistinct(t *testing.T) {
	dir := t.TempDir()
	pathA := b1607ScopeTraceFile(t, dir, "capture-a.ftrace", 51)
	pathB := b1607ScopeTraceFile(t, dir, "capture-b.ftrace", 61)
	a := b1607ScopeQuery(t, dir, pathA, 10, 10.003)
	b := b1607ScopeQuery(t, dir, pathB, 10, 10.003)
	setA, rowA := b1607ScopeInventoryAndRow(t, a)
	setB, rowB := b1607ScopeInventoryAndRow(t, b)
	if setA.SourceRef.ArtifactID != "trace_query" || setB.SourceRef.ArtifactID != "trace_query" ||
		types.RuntimeArtifactCaptureIdentityPath(setA.SourceRef) == types.RuntimeArtifactCaptureIdentityPath(setB.SourceRef) {
		t.Fatal("test must exercise actual generic tool labels on different physical captures")
	}
	if setA.Summary != setB.Summary || rowA.Summary == rowB.Summary {
		t.Fatal("fixture must isolate capture identity: equal totals/summaries, different physical peers")
	}
	ledger := types.ObservationLedger{Records: append(append([]types.ObservationRecord{}, a...), b...)}
	ctx := tracePrincipalValueAuthorityTestContext("target-41", 41, ledger.Records)
	before, _ := json.Marshal(ledger)
	got := renderAnswerDocBinderInventory(ledger, &ctx.AnalysisIR.RequestModel, "en")
	if strings.Count(got, "verified_wait_union=0.200ms") != 2 || !strings.Contains(got, pathA) || !strings.Contains(got, pathB) {
		t.Errorf("different physical captures were folded behind the shared tool label:\n%s", got)
	}
	for _, tc := range []struct {
		set, own, other types.ObservationRecord
	}{{setA, rowA, rowB}, {setB, rowB, rowA}} {
		// Keeping both leaf rows while exposing only one inventory isolates the
		// exact row-attachment predicate from the inventory dedup predicate.
		one := types.ObservationLedger{Records: []types.ObservationRecord{tc.set, rowA, rowB}}
		text := renderAnswerDocBinderInventory(one, &ctx.AnalysisIR.RequestModel, "en")
		if strings.Count(text, tc.own.Summary) != 1 || strings.Contains(text, tc.other.Summary) {
			t.Errorf("inventory borrowed the other capture's physical wait:\n%s", text)
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("display scope selection mutated accepted observations")
	}
}

func b1607InventoryPromptSection(text string) string {
	const heading = "### Independently verified target Binder waits\n"
	start := strings.Index(text, heading)
	if start < 0 {
		return ""
	}
	text = text[start:]
	if end := strings.Index(text[len(heading):], "\n##"); end >= 0 {
		return text[:len(heading)+end]
	}
	return text
}

func TestB1607BinderInventoryFinalRecapSharesExplicitWindowPoolBeforeCap(t *testing.T) {
	dir := t.TempDir()
	path := b1607ScopeTraceFile(t, dir, "selected-window.ftrace", 51)
	var records []types.ObservationRecord
	var broadIDs []string
	// Every old query fully contains the real wait but exceeds the requested
	// window. Eight such accounts must not consume the eight-group recap cap.
	for i := 0; i < 8; i++ {
		query := b1607ScopeQuery(t, dir, path, 9-float64(i), 11+float64(i))
		set, _ := b1607ScopeInventoryAndRow(t, query)
		broadIDs = append(broadIDs, set.ID)
		records = append(records, query...)
	}
	exact := b1607ScopeQuery(t, dir, path, 10, 10.003)
	contained := b1607ScopeQuery(t, dir, path, 10.00095, 10.0014)
	exactSet, _ := b1607ScopeInventoryAndRow(t, exact)
	containedSet, _ := b1607ScopeInventoryAndRow(t, contained)
	records = append(records, exact...)
	records = append(records, contained...)
	ctx := tracePrincipalValueAuthorityTestContext("target-41", 41, records)
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	start, end := 10.0, 10.003
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "10.000000..10.003000",
	}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	for name, prompt := range map[string]string{
		"observation": renderAnswerDocObservationLedger(ctx),
		"final recap": renderAnswerDocTracePrincipalValueAuthority(ctx),
		"finalizer":   (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		section := b1607InventoryPromptSection(prompt)
		if !strings.Contains(section, "["+exactSet.ID+"]") || !strings.Contains(section, "["+containedSet.ID+"]") {
			t.Errorf("%s lost exact/contained inventories after older broad queries:\n%s", name, section)
		}
		for _, id := range broadIDs {
			if strings.Contains(section, "["+id+"]") {
				t.Errorf("%s reintroduced a broad query into the principal inventory pool: %s", name, id)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("prompt-only window projection changed the full accepted ledger")
	}
}
