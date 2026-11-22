package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 12 (R11B-1, owner ruling R-377-12 ⑥): a retained native fact
// carries every typed identity its live copy carries and decides exactly as
// the live copy does. Round 11 (c9fd66ea7/18ad91c51) gave trace_query rank
// rows and semantic records an own-work account identity
// (ObservationRecord.TraceOwnWorkAccount, an in-process carrier) but the
// native-fact retention wire did not carry it: after the dispatch results
// were reset the retained copy had no identity, and every consumer fell back
// to the value-keyed pairing the identity replaced (the R10P-1 double). The
// full internal/agent suite failed on the field inventory
// (hmc377/fix11/merge/agentfail/suite_agent_merge_fa2f01465.log) and an
// overlay probe showed the identity lost across the boundary
// (hmc377/fix11/merge/agentfail/retention_probe.log).

type hmc377R12Line struct {
	ts   float64
	text string
}

type hmc377R12Trace struct{ lines []hmc377R12Line }

func (c *hmc377R12Trace) add(ts float64, comm string, tid, tgid, cpu int, format string, args ...any) {
	c.lines = append(c.lines, hmc377R12Line{ts, fmt.Sprintf("%16s-%-5d (%7d) [%03d] .... %.6f: %s", comm, tid, tgid, cpu, ts, fmt.Sprintf(format, args...))})
}

func (c *hmc377R12Trace) run(ts float64, comm string, tid, tgid, cpu int) {
	c.add(ts, "swapper", 0, 0, cpu, "sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=120", comm, tid)
}

func (c *hmc377R12Trace) sleep(ts float64, comm string, tid, tgid, cpu int) {
	c.add(ts, comm, tid, tgid, cpu, "sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120", comm, tid)
}

func (c *hmc377R12Trace) span(start, end float64, comm string, tid, tgid, cpu int, name string) {
	c.add(start, comm, tid, tgid, cpu, "tracing_mark_write: B|%d|%s", tgid, name)
	c.add(end, comm, tid, tgid, cpu, "tracing_mark_write: E|%d", tgid)
}

func (c *hmc377R12Trace) text() string {
	sort.SliceStable(c.lines, func(i, j int) bool { return c.lines[i].ts < c.lines[j].ts })
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	for _, line := range c.lines {
		b.WriteString(line.text + "\n")
	}
	return b.String()
}

// The two round-11 own-work account shapes (the same synthetic captures as
// internal/tool/trace_query_own_work_account_hmc377_test.go):
//   - split: main-100 verifies three classes in a 200 ms window, a 5 ms chain
//     wait inside the second; own work 30 ms + wait part 5 ms = class union 35 ms;
//   - window: the w27w2 shape, one class across the whole 100 ms window with a
//     98.9 ms CPU wait inside it; own work + wait part = the 100 ms window.
func hmc377R12SplitCapture() string {
	var c hmc377R12Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000100, "waker", 150, 100, 2)
	c.span(10.010000, 10.020000, "main", 100, 100, 1, "VerifyClass com.example.A")
	c.span(10.030000, 10.050000, "main", 100, 100, 1, "VerifyClass com.example.B")
	c.sleep(10.035000, "main", 100, 100, 1)
	c.add(10.040000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.040010, "main", 100, 100, 1)
	c.span(10.060000, 10.065000, "main", 100, 100, 1, "VerifyClass com.example.C")
	c.sleep(10.100000, "main", 100, 100, 1)
	c.add(10.150000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.150010, "main", 100, 100, 1)
	c.sleep(10.190000, "main", 100, 100, 1)
	return c.text()
}

func hmc377R12WindowCapture() string {
	var c hmc377R12Trace
	c.run(10.099000, "main", 100, 100, 1)
	c.span(10.099500, 10.200500, "main", 100, 100, 1, "VerifyClass com.example.Window")
	c.add(10.100100, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.199000, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.add(10.199600, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.199900, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.sleep(10.201000, "main", 100, 100, 1)
	return c.text()
}

// hmc377R12RetainedOwnWorkContext issues the two views the model issues
// (wakeup_chain, root_cause_rank) through the public tool entry and the
// dispatch admission, so both publications enter the generation-local
// retention lane exactly as a live run's do.
func hmc377R12RetainedOwnWorkContext(t *testing.T, capture string, start, end float64) *types.AgentContext {
	t.Helper()
	ctx, path := hmc320BusinessFinalizer(t)
	if err := os.WriteFile(path, []byte(capture), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "explicit window"}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		raw, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 100,
			"target_scope": "thread", "time_start": start, "time_end": end})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("public %s query: %v %s", view, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	if got := ctx.Mutable.NativeFactRetentionStatus().RetainedQueries; got < 2 {
		t.Fatalf("both native publications must enter the retention lane, retained %d", got)
	}
	return ctx
}

type hmc377R12Decision struct {
	accounts   map[string]string // record ID -> own-work account fact
	records    string            // compiled ledger records with their account facts
	projection string            // the compiled projection set
	nodes      []types.TraceCausalProjectionNode
}

func hmc377R12Decide(t *testing.T, input types.ObservationLedgerInput) hmc377R12Decision {
	t.Helper()
	out := hmc377R12Decision{accounts: map[string]string{}}
	for _, result := range input.ToolResults {
		for _, row := range result.Observations {
			if fact := row.TraceOwnWorkAccount.Fact(); fact != "" {
				out.accounts[row.ID] = fact
			}
		}
	}
	ledger := types.CompileObservationLedger(input)
	type keyed struct {
		Record  types.ObservationRecord
		Account string
	}
	var records []keyed
	for _, record := range ledger.Records {
		records = append(records, keyed{record, record.TraceOwnWorkAccount.Fact()})
	}
	out.records = hmc325BoundaryJSON(t, records)
	set := types.CompileTraceCausalProjectionSet(ledger)
	out.projection = hmc325BoundaryJSON(t, set)
	for _, projection := range set.Projections {
		for _, bucket := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.RankedSeats,
			projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses, projection.SemanticSpans} {
			out.nodes = append(out.nodes, bucket...)
		}
	}
	return out
}

func TestHMC377R12RetainedOwnWorkAccountDecidesAsLive(t *testing.T) {
	for _, shape := range []struct {
		name       string
		capture    string
		start, end float64
		union      float64 // the class union of main-100's class verification (ms)
	}{
		{"split_family", hmc377R12SplitCapture(), 10.0, 10.2, 35},
		{"window_span_wait", hmc377R12WindowCapture(), 10.1, 10.2, 100},
	} {
		t.Run(shape.name, func(t *testing.T) {
			ctx := hmc377R12RetainedOwnWorkContext(t, shape.capture, shape.start, shape.end)
			live := hmc377R12Decide(t, hmc325BoundaryInput(ctx))
			if len(live.accounts) == 0 {
				t.Fatal("prerequisite: the live publications carry no own-work account identity")
			}
			ctx.Mutable.ResetDispatchToolResults()
			input := hmc325BoundaryInput(ctx)
			if len(input.ToolResults) != 2 {
				t.Fatalf("both retained native publications must return, got %d", len(input.ToolResults))
			}
			retained := hmc377R12Decide(t, input)
			t.Run("identity", func(t *testing.T) {
				for id, fact := range live.accounts {
					if retained.accounts[id] != fact {
						t.Errorf("retained record %s carries account %q, live %q", id, retained.accounts[id], fact)
					}
				}
				if len(retained.accounts) != len(live.accounts) {
					t.Errorf("retained %d account rows, live %d", len(retained.accounts), len(live.accounts))
				}
			})
			t.Run("ledger", func(t *testing.T) {
				if retained.records != live.records {
					t.Error("the retained publications compile a different ledger from the live ones")
				}
			})
			t.Run("projection", func(t *testing.T) {
				if retained.projection != live.projection {
					t.Error("the retained publications compile a different projection from the live ones")
				}
			})
			t.Run("bound", func(t *testing.T) {
				// The class's two parts carry disjoint instants: no node of
				// it publishes more than the class union, whichever copy.
				for _, node := range retained.nodes {
					if strings.TrimSpace(node.SemanticClass) != "class_verification" {
						continue
					}
					if node.EffectiveImpactMS > shape.union+0.0015 || node.ImpactMS > shape.union+0.0015 {
						t.Errorf("retained node %s publishes eff=%.3f impact=%.3f above the class union %.3f (merged=%d)",
							node.EvidenceID, node.EffectiveImpactMS, node.ImpactMS, shape.union, node.MergedCount)
					}
				}
			})
		})
	}
}
