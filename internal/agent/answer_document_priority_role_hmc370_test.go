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
	"github.com/hanchaoqun/codrax/internal/render"
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
	measured := finalMeasurementWindowLine(got, "  - subject=", "measured_state_occupancy=45.378ms")
	if !strings.Contains(measured, "-64176") || !strings.Contains(measured, "locator_range=`11343.250704..11343.297396`") ||
		!strings.Contains(measured, "dependency analysis window (not a continuous state interval)") || strings.Contains(measured, "window_projection=") {
		t.Fatalf("full-capture dependency occupancy was confused with a query window: %s", measured)
	}
	// EVOLUTION RECORD (HMC377 D3, merge of fix round 2): old → new =
	// "ready thread main-64156" → "ready thread ele.crowdsource-64156" (here
	// and in the sidecar evidence below). One physical thread now carries one
	// display name on every face; 64156's `main` is the zygote-inherited name
	// it dropped at L20057 (11343.248385), and the overlap's own rows name it
	// ele.crowdsource (L25227 sched_wakeup comm=ele.crowdsource pid=64156,
	// L25229 next_comm=ele.crowdsource). Value, lines, CPU and interval are
	// unchanged.
	hmc370PriorityRoleLine(t, got, "priority_inversion_runnable_wait",
		[]string{"0.013ms", "same-CPU ready/run overlap", "ready thread ele.crowdsource-64156", "running competitor", "-64176", "CPU4",
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
			!strings.Contains(evidence, "L25227–L25229") || !strings.Contains(evidence, "就绪线程 ele.crowdsource-64156（L25227–L25229）") ||
			strings.Contains(evidence, "main-64156") ||
			!strings.Contains(evidence, "-64176") || strings.Contains(evidence, "低优先级依赖方供给候选") {
			t.Fatalf("sidecar applied the wrong scope or magnitude: %+v", item)
		}
		found = true
	}
	if !found {
		t.Fatal("expected the native on-chain runnable-overlap seat to remain selectable")
	}

	// The same original produced the misleading system-authored 3.928ms
	// single-method statement. Keep this witness beside the original causal
	// claims so neither measurement repair nor graph rendering can hide it.
	work := types.BuildRuntimeWorkRelationContract(types.ObservationLedgerInputFromAgentContext(ctx, 64), true)
	found = false
	if work != nil {
		for _, row := range work.Rows {
			m := row.SemanticSpanMeasurement()
			if m.LongestLabel != "VerifyClass android.net.TrafficStats" {
				continue
			}
			if row.WorkLabel != "class_verification" || m.MemberCount != 5 ||
				row.MeasuredDurationMS != 3.868 || m.FullDurationMS != 3.928 || m.LongestMS != 2.954 {
				t.Fatalf("native class-verification rulers crossed: row=%+v measurement=%+v", row, m)
			}
			receipt := &types.AnswerRuntimeWorkRelationReceipt{ObservationID: row.ObservationID, Conclusion: types.RuntimeWorkRelationConclusionTargetSelfWorkObserved}
			if !types.BindRuntimeWorkRelationReceipt(receipt, work) {
				t.Fatal("native self-work conclusion lost its authority")
			}
			if types.BindRuntimeWorkRelationReceipt(&types.AnswerRuntimeWorkRelationReceipt{ObservationID: row.ObservationID, Conclusion: types.RuntimeWorkRelationConclusionCausalContributionSupported}, work) {
				t.Fatal("a timing group gained authority to prove causal contribution")
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "work", Kind: types.BlockSummary, Text: "类校验排查。", RuntimeWorkRelation: receipt}}}
			visible := render.RenderAnswerDocument(doc, "zh")
			for _, want := range []string{"`类校验`", "5 条记录", "查询窗内合计 3.868ms", "[11343.248000, 11343.310000)", "`VerifyClass android.net.TrafficStats`（窗内 2.954ms）", "完整配对范围合计 3.928ms"} {
				if !strings.Contains(visible, want) {
					t.Errorf("original receipt missing %q:\n%s", want, visible)
				}
			}
			found = true
		}
	}
	if !found {
		t.Fatal("the complete Honeybird capture no longer offers its retained class-verification family")
	}
}
