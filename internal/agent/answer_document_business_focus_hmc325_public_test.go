package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The queried owner is the ftrace header TID, not marker payload PID 2227.
// Eighteen earlier same-name roots force the selected non-root beyond slot 16.
// The independently known account is 4 ms inclusive - 2 ms child = 2 ms self.
func hmc325BusinessFocusContext(t *testing.T, shape string) (*types.AgentContext, string) {
	t.Helper()
	ctx, path := hmc320BusinessFinalizer(t)
	type event struct {
		us     int
		tid    int
		marker string
	}
	var events []event
	add := func(us, tid int, marker string) { events = append(events, event{us, tid, marker}) }
	for i := 0; i < 18; i++ {
		add(5000000+i*10, 900+i, "B|2227|SharedPhase")
		add(5012000, 900+i, "E|2227")
	}
	if shape != "root" {
		add(5000300, 700, "B|2227|SharedPhase")
	}
	add(5004000, 700, "B|2227|SharedPhase")
	if shape == "ambiguous" {
		add(5004000, 700, "B|2227|SharedPhase")
	}
	add(5005000, 700, "B|2227|SharedPhase")
	add(5007000, 700, "E|2227")
	if shape != "unclosed" {
		add(5008000, 700, "E|2227")
		if shape == "ambiguous" {
			add(5008000, 700, "E|2227")
		}
		if shape != "root" {
			add(5011700, 700, "E|2227")
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].us < events[j].us })
	var raw strings.Builder
	raw.WriteString("# tracer: nop\n")
	for _, e := range events {
		fmt.Fprintf(&raw, "business-%d (600) [003] .... %d.%06d: tracing_mark_write: %s\n", e.tid, e.us/1000000, e.us%1000000, e.marker)
	}
	if err := os.WriteFile(path, []byte(raw.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.02
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.0..5.02"}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{
		types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOtherObservedValue}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return ctx, path
}

func hmc325Query(t *testing.T, ctx *types.AgentContext, path string, extra map[string]any) {
	t.Helper()
	params := map[string]any{"time_start": 5.004, "time_end": 5.008, "target_scope": "thread"}
	for k, v := range extra {
		params[k] = v
	}
	hmc320BusinessQuery(t, ctx, path, params)
}

func hmc325FullAccounts(ledger types.ObservationLedger) []answerDocBusinessAccount {
	var out []answerDocBusinessAccount
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok {
			out = append(out, answerDocBusinessAccount{record: r, fact: f})
		}
	}
	return out
}

func hmc325IsTarget(a answerDocBusinessAccount) bool {
	return a.fact.Node.Thread.PID == 700 && a.fact.Node.ActualStartTs == 5.004
}

func TestHMC325BusinessQueryExactInstanceSurvivesPublicFinalizer(t *testing.T) {
	for _, shape := range []string{"parent", "root"} {
		t.Run(shape, func(t *testing.T) {
			ctx, path := hmc325BusinessFocusContext(t, shape)
			hmc325Query(t, ctx, path, map[string]any{"pid": 700, "thread": "business"})
			ledger := answerDocObservationLedger(ctx)
			var target answerDocBusinessAccount
			for _, a := range hmc325FullAccounts(ledger) {
				if hmc325IsTarget(a) {
					target = a
				}
			}
			n := target.fact.Node
			wantParent := "observed_parent"
			if shape == "root" {
				wantParent = "observed_root"
			}
			if target.record.ID == "" || n.ParentStatus != wantParent || n.Closure != "closed" ||
				n.ActualEndTs == nil || *n.ActualEndTs != 5.008 || n.Inclusive == nil || n.Self == nil ||
				math.Abs(n.Inclusive.DurationMs-4) > 1e-8 || math.Abs(n.Self.DurationMs-2) > 1e-8 ||
				target.record.SourceRef.QueryTargetPID != 700 || target.record.SourceRef.QueryTargetScope != "thread" {
				t.Fatalf("public native prerequisite: %+v", target)
			}
			before, _ := json.Marshal(ledger)
			causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			requestBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			statusBefore, focusBefore := ctx.Mutable.AcceptedTraceBusinessFocus()
			plan := planAnswerDocBusinessTreeFacts(ctx, ledger)
			if len(plan.accounts) != 16 || !hmc325IsTarget(plan.accounts[0]) {
				t.Errorf("unique query-owner complete instance did not lead the unchanged 16-slot plan: shown=%d first=%s", len(plan.accounts), plan.accounts[0].fact.Node.ID)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, tool.TraceBusinessTreeNodeText(n)) || !strings.Contains(prompt, "observation_id="+quotedTreeValue(target.record.ID)) {
				t.Error("public finalizer lost the full non-prefix account")
			}
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
			requestAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			statusAfter, focusAfter := ctx.Mutable.AcceptedTraceBusinessFocus()
			if string(before) != string(after) || string(causalBefore) != string(causalAfter) || string(requestBefore) != string(requestAfter) ||
				statusBefore != statusAfter || focusBefore.Token() != focusAfter.Token() {
				t.Fatal("display ranking changed observations, causal projection, request scope or accepted focus")
			}
		})
	}
}

func TestHMC325BusinessQueryNoUniqueInstanceKeepsLegacyOrder(t *testing.T) {
	for _, tc := range []struct {
		name, shape string
		params      map[string]any
	}{
		{"no_owner", "parent", nil},
		{"name_only", "parent", map[string]any{"thread": "business"}},
		{"wrong_tid", "parent", map[string]any{"pid": 900}},
		{"marker_pid_not_header_tid", "parent", map[string]any{"pid": 2227}},
		{"clipped_parent_is_not_instance", "parent", map[string]any{"pid": 700, "time_start": 5.0045, "time_end": 5.0075}},
		{"wider_query_is_not_instance", "parent", map[string]any{"pid": 700, "time_start": 5.003, "time_end": 5.009}},
		{"multiple_same_owner_same_bounds", "ambiguous", map[string]any{"pid": 700}},
		{"unclosed", "unclosed", map[string]any{"pid": 700}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := hmc325BusinessFocusContext(t, tc.shape)
			hmc325Query(t, ctx, path, tc.params)
			accounts := hmc325FullAccounts(answerDocObservationLedger(ctx))
			if len(accounts) < 19 {
				t.Fatalf("public fixture did not retain late native account: %d", len(accounts))
			}
			// This is the unchanged fallback contract, independent of the new
			// matching helper: native root, physical start line, full record key.
			sort.Slice(accounts, func(i, j int) bool {
				a, b := accounts[i], accounts[j]
				ar, br := a.fact.Node.ParentStatus == "observed_root", b.fact.Node.ParentStatus == "observed_root"
				if ar != br {
					return ar
				}
				if a.fact.Node.StartLine != b.fact.Node.StartLine {
					return a.fact.Node.StartLine < b.fact.Node.StartLine
				}
				return traceEventPublicationRecordKey(a.record) < traceEventPublicationRecordKey(b.record)
			})
			got := selectAnswerDocBusinessAccounts(accounts)
			if len(got.accounts) != 16 {
				t.Fatalf("changed 16-slot bound: %d", len(got.accounts))
			}
			for i, a := range got.accounts {
				if traceEventPublicationRecordKey(a.record) != traceEventPublicationRecordKey(accounts[i].record) {
					t.Fatalf("unbound/ambiguous instance changed fallback rank at %d", i)
				}
			}
		})
	}
}

func TestHMC325BusinessQueryProcessScopeCannotCreateThreadAccount(t *testing.T) {
	ctx, path := hmc325BusinessFocusContext(t, "parent")
	params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 600,
		"target_scope": "process", "time_start": 5.004, "time_end": 5.008})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || result.Success || len(result.Observations) != 0 || !strings.Contains(result.Summary, "rejected target_scope=process") {
		t.Fatalf("process scope acquired thread-account permission: err=%v success=%v observations=%d summary=%s", err, result.Success, len(result.Observations), result.Summary)
	}
}

func TestHMC325BusinessQueryPriorityKeepsSourceAndWindowGroups(t *testing.T) {
	ctx, path := hmc325BusinessFocusContext(t, "parent")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "same-name-other-capture.systrace")
	if err := os.WriteFile(other, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, other} {
		hmc325Query(t, ctx, p, map[string]any{"pid": 700})
	}
	hmc325Query(t, ctx, path, map[string]any{"pid": 700, "time_start": 5.0045, "time_end": 5.0075})
	ledger := answerDocObservationLedger(ctx)
	accounts := hmc325FullAccounts(ledger)
	forward := selectAnswerDocBusinessAccounts(accounts)
	for i, j := 0, len(accounts)-1; i < j; i, j = i+1, j-1 {
		accounts[i], accounts[j] = accounts[j], accounts[i]
	}
	reverse := selectAnswerDocBusinessAccounts(accounts)
	if len(forward.accounts) != 16 || forward.groups != 3 || forward.shownGroups != 3 || renderAnswerDocBusinessAccountPlan(forward) != renderAnswerDocBusinessAccountPlan(reverse) {
		t.Fatal("query-local priority changed bounded fairness or deterministic arrival order")
	}
	exactSources := map[string]bool{}
	for _, a := range forward.accounts {
		if hmc325IsTarget(a) && a.record.SourceRef.QueryWindowStartTs == 5.004 && a.record.SourceRef.QueryWindowEndTs == 5.008 {
			exactSources[a.fact.Node.SourcePath] = true
		}
	}
	if len(exactSources) != 2 {
		t.Fatalf("equal-name/source-independent full accounts lost: %v", exactSources)
	}
}
