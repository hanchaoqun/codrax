package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc370PriorityContext(t *testing.T, path, subject, view string, pid int, start, end float64) *types.AgentContext {
	t.Helper()
	dir := t.TempDir()
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid,
		"time_start": start, "time_end": end, "target_scope": "thread", "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("public native query: %v; %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext(subject, pid, result.Observations)
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "selected seconds",
	}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return ctx
}

func hmc370PriorityRoleLine(t *testing.T, handoff, kind string, wants, forbidden []string) {
	t.Helper()
	for _, line := range strings.Split(handoff, "\n") {
		if !strings.Contains(line, "kind=`"+kind+"`") || !strings.Contains(line, "effective_attribution=") {
			continue
		}
		for _, value := range wants {
			if !strings.Contains(line, value) {
				t.Errorf("%s handoff lost %q: %s", kind, value, line)
			}
		}
		for _, value := range forbidden {
			if strings.Contains(line, value) {
				t.Errorf("%s acquired %q: %s", kind, value, line)
			}
		}
		return
	}
	t.Fatalf("no %s rank seat in public handoff:\n%s", kind, handoff)
}

func TestHMC370PublicReadyOverlapWithoutWakeDoesNotAcquireWakePhase(t *testing.T) {
	const trace = `idle-0 (0) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=R+ ==> next_comm=rival next_pid=300 next_prio=20
rival-300 (300) [001] .... 5.005000: sched_switch: prev_comm=rival prev_pid=300 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=159
app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	path := filepath.Join(t.TempDir(), "priority-ready.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := hmc370PriorityContext(t, path, "app-100", "root_cause_rank", 100, 4.998, 5.012)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	hmc370PriorityRoleLine(t, got, "priority_inversion_runnable_wait",
		[]string{"effective_attribution=5.000ms", "same-CPU ready/run overlap", "ready thread app-100", "running competitor rival-300", "[5.000000, 5.005000)", "CPU1"},
		[]string{"before_downstream_wakeup", "after recorded wakeup", "lower_priority_dependency_only"})
}

func TestHMC370HoneybirdWholePriorityRolesKeepNativePartiesAndIntervals(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the pinned complete Honeybird ftrace")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if n, err := io.Copy(h, f); err != nil || n != 106896120 ||
		hex.EncodeToString(h.Sum(nil)) != "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e" {
		t.Fatalf("wrong whole capture: %v", err)
	}
	ctx := hmc370PriorityContext(t, path, "ele.crowdsource-64156", "frame_root_cause_bundle", 64156, 11343.248, 11343.310)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	hmc370PriorityRoleLine(t, got, "priority_inversion_candidate",
		[]string{"64176", "36.311ms", "dependency_scheduler_supply_before_downstream_wakeup"},
		[]string{"running competitor", "after recorded wakeup"})
	hmc370PriorityRoleLine(t, got, "priority_inversion_runnable_wait",
		[]string{"0.013ms", "same-CPU ready/run overlap", "ready thread main-64156", "running competitor", "-64176", "CPU4",
			"[11343.297396, 11343.297409)", "after recorded wakeup (line 25227)", "25229"},
		[]string{"before_downstream_wakeup", "lower_priority_dependency_only"})
	ledger := answerDocObservationLedger(ctx)
	contract, err := tracefinding.CompileCandidateContract(ledger, types.CompileTraceCausalProjectionSet(ledger), tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	found := false
	for _, c := range tracefinding.SelectableRootCauseCandidates(contract) {
		if c.Decision.Token.Token != "priority_inversion_runnable_wait" {
			continue
		}
		got, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
			SchemaVersion: types.TraceRootCauseReportSchemaVersion,
			RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: c.Decision.CandidateID}},
		}, contract)
		if err != nil || len(got.RootCauses) != 1 {
			t.Fatalf("bind the selectable native witness: %v", err)
		}
		item := got.RootCauses[0]
		evidence := strings.Join(item.Evidence, " ")
		if item.MechanismQualifier != types.TraceMechanismSameCPURunnableOverlapCandidate ||
			item.ImpactSeconds == nil || *item.ImpactSeconds != .000013 || item.ImpactBreakdown == nil ||
			!strings.Contains(evidence, "L25227–L25229") || !strings.Contains(evidence, "main-64156") ||
			!strings.Contains(evidence, "-64176") || strings.Contains(evidence, "低优先级依赖方供给候选") {
			t.Fatalf("sidecar applied the wrong scope or magnitude: %+v", item)
		}
		found = true
	}
	if !found {
		t.Fatal("expected the native on-chain runnable-overlap seat to remain selectable")
	}
}
