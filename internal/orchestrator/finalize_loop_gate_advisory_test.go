package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalize_loop_gate_advisory_test.go — §40.43 F-orch 三轮复核 finding S.
// The cluster-closure exit advisory must take EVERY gate that precedes
// AdvanceRepairExecutionPlan in loop order and state per arm whether its
// pre-emption is total or conditional. The earlier two-arm form (P6 +
// W2.6) said "reachable" at hardCap 3 / perRoot 4 / class 1 while the
// same-error-class governor shipped the run on failure 2, and said
// "unreachable" at stable 2 / hardCap 3 / perRoot 3 while a new root on
// failure 3 kept W2.6 from shipping and the exit fired.

// gateFor returns the first pre-empting gate whose name starts with prefix.
func gateFor(r clusterExitReachability, prefix string) *finalizeLoopGate {
	for i := range r.PreEmpting {
		if strings.HasPrefix(r.PreEmpting[i].Name, prefix) {
			return &r.PreEmpting[i]
		}
	}
	return nil
}

// Predicate pins over the reviewers' configurations plus the gates the
// two-arm form never modelled.
func TestClusterClosureExitReachability_EveryPrecedingGate(t *testing.T) {
	base := finalizeLoopCaps{StableBudget: 2, HardCap: 2, PerRootCap: 3, ClassCap: 1, TemplateRetryBudget: 5}
	with := func(f func(*finalizeLoopCaps)) finalizeLoopCaps { c := base; f(&c); return c }
	cases := []struct {
		name        string
		caps        finalizeLoopCaps
		wantVerdict clusterExitVerdict
		// wantFirst is the earliest pre-empting gate (name prefix + failure
		// + total/conditional); empty when the exit is reachable.
		wantFirst      string
		wantFirstOn    int
		wantFirstTotal bool
		wantMentions   []string
	}{
		{
			name: "defaults: class governor pre-empts on failure 2 (conditional), P6 total on failure 3",
			caps: base, wantVerdict: clusterExitUnreachable,
			wantFirst: "same-error-class governor", wantFirstOn: 2, wantFirstTotal: false,
			wantMentions: []string{"P6 finalize repair hard cap 2 ships on failure 3 (total", "W2.6 per-root attempt cap 3 ships on failure 3 (conditional", "unchanged"},
		},
		{
			name: "hard 3 / perRoot 4 / class 1: class governor still pre-empts (conditional), P6 only with outside retries",
			caps: with(func(c *finalizeLoopCaps) { c.HardCap = 3; c.PerRootCap = 4 }), wantVerdict: clusterExitConditional,
			wantFirst: "same-error-class governor", wantFirstOn: 2,
			wantMentions: []string{"P6 finalize repair hard cap 3 ships on failure 3 (outside-chain retries only: only when at least 1 retry were recorded outside"},
		},
		{
			name: "hard 3 / perRoot 4 / class 0: reachable — only outside-chain retries could let P6 / the template budget pre-empt",
			caps: with(func(c *finalizeLoopCaps) { c.HardCap = 3; c.PerRootCap = 4; c.ClassCap = 0 }), wantVerdict: clusterExitReachable,
			wantFirst: "P6 finalize repair hard cap 3", wantFirstOn: 3,
			wantMentions: []string{"P6 finalize repair hard cap 3 ships on failure 3 (outside-chain retries only: only when at least 1 retry", "template retry budget 5 ships on failure 3 (outside-chain retries only: only when at least 3 retries"},
		},
		{
			name: "hard 3 / perRoot 4 / class 2: class governor on failure 3 (conditional) precedes the outside-retry-only P6",
			caps: with(func(c *finalizeLoopCaps) { c.HardCap = 3; c.PerRootCap = 4; c.ClassCap = 2 }), wantVerdict: clusterExitConditional,
			wantFirst: "same-error-class governor cap 2", wantFirstOn: 3,
			wantMentions: []string{"same-error-class governor cap 2 ships on failure 3 (conditional", "P6 finalize repair hard cap 3 ships on failure 3 (outside-chain retries only"},
		},
		{
			name: "defaults with class 0: P6 total on failure 3",
			caps: with(func(c *finalizeLoopCaps) { c.ClassCap = 0 }), wantVerdict: clusterExitUnreachable,
			wantFirst: "P6 finalize repair hard cap 2", wantFirstOn: 3, wantFirstTotal: true,
		},
		{
			name: "stable 2 / hard 3 / perRoot 3 / class 0: W2.6 is conditional on an unchanged root set — a new root on failure 3 lets the exit fire",
			caps: with(func(c *finalizeLoopCaps) { c.HardCap = 3; c.PerRootCap = 3; c.ClassCap = 0 }), wantVerdict: clusterExitConditional,
			wantFirst: "W2.6 per-root attempt cap 3", wantFirstOn: 3,
			wantMentions: []string{"W2.6 per-root attempt cap 3 ships on failure 3 (conditional: only while the actionable root set is unchanged"},
		},
		{
			name: "template retry budget below the stable budget is a total pre-emption",
			caps: with(func(c *finalizeLoopCaps) {
				c.HardCap = 10
				c.PerRootCap = 10
				c.ClassCap = 0
				c.TemplateRetryBudget = 1
			}), wantVerdict: clusterExitUnreachable,
			wantFirst: "template retry budget 1", wantFirstOn: 2, wantFirstTotal: true,
		},
		{
			name: "configured per-kind cap pre-empts conditionally",
			caps: with(func(c *finalizeLoopCaps) {
				c.HardCap = 10
				c.PerRootCap = 10
				c.ClassCap = 0
				c.TemplateRetryBudget = 10
				c.KindCaps = map[string]int{"other": 1}
			}), wantVerdict: clusterExitConditional,
			wantFirst: "per-kind retry cap other=1", wantFirstOn: 2,
			wantMentions: []string{"billed to the kind ledger"},
		},
		{
			name: "low-yield kill pre-empts conditionally from failure 2",
			caps: with(func(c *finalizeLoopCaps) {
				c.HardCap = 10
				c.PerRootCap = 10
				c.ClassCap = 0
				c.TemplateRetryBudget = 10
				c.MinRetryYield = 1
			}), wantVerdict: clusterExitConditional,
			wantFirst: "low-yield kill", wantFirstOn: 2,
		},
		{
			name: "every gate raised above the cluster budget: reachable (the retryUsed gates are stated as outside-chain-only notes)",
			caps: with(func(c *finalizeLoopCaps) {
				c.HardCap = 10
				c.PerRootCap = 10
				c.ClassCap = 0
				c.TemplateRetryBudget = 10
			}), wantVerdict: clusterExitReachable,
			wantFirst: "P6 finalize repair hard cap 10", wantFirstOn: 3,
			wantMentions: []string{"at least 8 retries were recorded outside"},
		},
		{
			name: "no gate at all: reachable with nothing listed",
			caps: finalizeLoopCaps{StableBudget: 2}, wantVerdict: clusterExitReachable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := clusterClosureExitReachability(tc.caps)
			if r.Verdict != tc.wantVerdict {
				t.Fatalf("verdict %q, want %q (%s)", r.Verdict, tc.wantVerdict, r.Reason)
			}
			if r.ExitFailure != tc.caps.StableBudget+1 {
				t.Fatalf("exit failure %d, want %d", r.ExitFailure, tc.caps.StableBudget+1)
			}
			if tc.wantFirst == "" {
				if len(r.PreEmpting) != 0 {
					t.Fatalf("no gate configured: nothing may be listed, got %s", r.Reason)
				}
				return
			}
			if tc.wantVerdict == clusterExitReachable {
				for _, g := range r.PreEmpting {
					if g.Total || !g.OutsideRetryOnly {
						t.Fatalf("a reachable verdict may list only outside-chain-retry notes, got %+v", g)
					}
				}
			}
			first := r.PreEmpting[0]
			if !strings.HasPrefix(first.Name, tc.wantFirst) || first.FiresOnFailure != tc.wantFirstOn || first.Total != tc.wantFirstTotal {
				t.Fatalf("first pre-empting gate = %+v, want %q on failure %d total=%t (%s)", first, tc.wantFirst, tc.wantFirstOn, tc.wantFirstTotal, r.Reason)
			}
			for _, want := range tc.wantMentions {
				if !strings.Contains(r.Reason, want) {
					t.Fatalf("reason must state %q, got %s", want, r.Reason)
				}
			}
			for _, g := range r.PreEmpting {
				if g.Qualifier == "" {
					t.Fatalf("every listed gate states its qualifier, %+v has none", g)
				}
			}
		})
	}
}

// E2E table (the reviewers' five configurations, real Orchestrator.Run: a
// two-node graph, a finalizer that fails a hard must_include term on every
// round, must_include routed finalizer_only — its default): which gate
// ships the run and on which failure, against what the advisory predicts
// for the same live caps. Gate witnesses are typed: P6 emits
// NoticeFinalizeRepairCap; the cluster exit persists the plan with
// HasFailLoud; the class governor does neither.
func TestFinalizeLoopGates_E2E_TableAgreesWithAdvisory(t *testing.T) {
	restoreStable := ClusterStableBudget()
	restorePerRoot := maxRepairAttemptsPerRootValue
	restoreClass := sameErrorClassRetryCap()
	t.Cleanup(func() {
		SetClusterStableBudget(restoreStable)
		SetMaxRepairAttemptsPerRoot(restorePerRoot)
		SetSameErrorClassRetryCap(restoreClass)
		SetSoftViolationKinds(nil, nil)
	})
	SetSoftViolationKinds(nil, []string{string(types.ViolMustInclude)})
	SetClusterStableBudget(2)

	cases := []struct {
		name                       string
		hardCap, perRoot, classCap int
		wantFinalize               int
		wantGate                   string // "class" | "P6" | "cluster_exit"
	}{
		{name: "defaults → class governor on failure 2", hardCap: 2, perRoot: 3, classCap: 1, wantFinalize: 2, wantGate: "class"},
		{name: "hard 3 / perRoot 4 / class 1 → still class governor on failure 2", hardCap: 3, perRoot: 4, classCap: 1, wantFinalize: 2, wantGate: "class"},
		{name: "hard 3 / perRoot 4 / class 0 → cluster exit fires on failure 3", hardCap: 3, perRoot: 4, classCap: 0, wantFinalize: 3, wantGate: "cluster_exit"},
		{name: "hard 3 / perRoot 4 / class 2 → class governor on failure 3", hardCap: 3, perRoot: 4, classCap: 2, wantFinalize: 3, wantGate: "class"},
		{name: "defaults with class 0 → P6 on failure 3", hardCap: 2, perRoot: 3, classCap: 0, wantFinalize: 3, wantGate: "P6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetMaxRepairAttemptsPerRoot(tc.perRoot)
			SetSameErrorClassRetryCap(tc.classCap)
			ir := &types.AnalysisIR{
				Version:      types.AnalysisIRVersion,
				RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain},
				TaskGraph: types.TaskGraph{
					Nodes: []types.TaskNode{
						{ID: "n0", Type: types.NodeEvidence, Objective: "collect"},
						{ID: "n1", Type: types.NodeFinalize, Objective: "render"},
					},
					Edges:           []types.TaskEdge{{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency}},
					ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 5, CriticalPath: []string{"n0", "n1"}},
				},
				AnswerContract: types.AnswerContract{Language: "en", MustInclude: []string{"FORCE_RETRY"}},
			}
			finalizeCalls := 0
			agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					return &agent.StageOutput{
						MissingPiece:  types.MissingFacts,
						EvidenceItems: []types.EvidenceItem{{Subject: "sym", Source: "test.go", LineStart: 1, AnchorKind: types.AnchorDefinition}},
					}, nil
				},
				types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					finalizeCalls++
					return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "answer body without the sentinel"}, nil
				},
			}
			ar, sr, sar := buildRegistries(agentFns)
			o := New(types.PipelineSettings{}, ar, sr, sar)
			o.SetMaxSteps(20)
			o.SetFinalizeRepairHardCap(tc.hardCap)
			var mu sync.Mutex
			p6Notice := false
			o.SetEmitter(func(ev render.Event) {
				if ev.Kind == render.EventOrchestratorNotice && ev.NoticeKind == render.NoticeFinalizeRepairCap {
					mu.Lock()
					p6Notice = true
					mu.Unlock()
				}
			})

			done := make(chan struct{})
			var bus *types.BusContext
			go func() {
				bus, _ = o.Run("finalize loop gates", "/tmp/repo", "main")
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("Run did not terminate within 10s")
			}
			if bus == nil || bus.Mutable == nil {
				t.Fatal("Run returned no bus")
			}
			clusterExit := false
			if plan, ok := bus.Mutable.RepairExecutionPlan().(RepairExecutionPlan); ok {
				clusterExit = plan.HasFailLoud
			}
			gate := "class"
			switch {
			case clusterExit:
				gate = "cluster_exit"
			case p6Notice:
				gate = "P6"
			}
			if finalizeCalls != tc.wantFinalize || gate != tc.wantGate {
				t.Fatalf("observed gate=%s on finalize call %d, want %s on %d (p6Notice=%t clusterExit=%t)", gate, finalizeCalls, tc.wantGate, tc.wantFinalize, p6Notice, clusterExit)
			}
			if result := bus.Mutable.Result(); !strings.Contains(result, "answer body without the sentinel") || strings.TrimSpace(result) == "answer body without the sentinel" {
				t.Fatalf("every gate ships the draft WITH caveats, got:\n%s", result)
			}

			// The advisory over the same live caps must agree with what
			// happened: no TOTAL gate fires before the observed failure, and
			// the observed gate is listed on the observed failure (the
			// cluster exit is never listed — it is the subject).
			r := clusterClosureExitReachability(o.finalizeLoopCaps(ir))
			for _, g := range r.PreEmpting {
				if g.Total && g.FiresOnFailure < tc.wantFinalize {
					t.Fatalf("advisory lists a total gate before the observed failure: %+v (observed %s on %d)", g, tc.wantGate, tc.wantFinalize)
				}
			}
			switch tc.wantGate {
			case "class":
				g := gateFor(r, "same-error-class governor")
				if g == nil || g.FiresOnFailure != tc.wantFinalize || g.Total {
					t.Fatalf("advisory must list the class governor as conditional on failure %d, got %+v (%s)", tc.wantFinalize, g, r.Reason)
				}
			case "P6":
				g := gateFor(r, "P6 finalize repair hard cap")
				if g == nil || g.FiresOnFailure != tc.wantFinalize || !g.Total || r.Verdict != clusterExitUnreachable {
					t.Fatalf("advisory must list P6 as total on failure %d with verdict unreachable, got %+v (%s)", tc.wantFinalize, g, r.Reason)
				}
			case "cluster_exit":
				if r.Verdict == clusterExitUnreachable || r.ExitFailure != tc.wantFinalize {
					t.Fatalf("the exit fired on failure %d; the advisory must not call it unreachable (%s)", tc.wantFinalize, r.Reason)
				}
			}
		})
	}
}

// The advisory fires once per Orchestrator, reads the live caps and the
// IR's template budget, and stays silent when the exit is reachable.
func TestLogClusterClosureExitReachabilityOnce_EmitsOncePerOrchestrator(t *testing.T) {
	restoreStable := ClusterStableBudget()
	restorePerRoot := maxRepairAttemptsPerRootValue
	restoreClass := sameErrorClassRetryCap()
	t.Cleanup(func() {
		SetClusterStableBudget(restoreStable)
		SetMaxRepairAttemptsPerRoot(restorePerRoot)
		SetSameErrorClassRetryCap(restoreClass)
	})
	SetClusterStableBudget(2)
	SetMaxRepairAttemptsPerRoot(3)
	SetSameErrorClassRetryCap(1)
	ir := &types.AnalysisIR{TaskGraph: types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 5}}}

	o := &Orchestrator{}
	o.SetFinalizeRepairHardCap(2)
	msg, emitted := o.logClusterClosureExitReachabilityOnce(ir)
	if !emitted || !strings.Contains(msg, "unreachable") || !strings.Contains(msg, "same-error-class governor cap 1 ships on failure 2") {
		t.Fatalf("default caps must emit the advisory naming the class governor first, got emitted=%t msg=%q", emitted, msg)
	}
	if _, again := o.logClusterClosureExitReachabilityOnce(ir); again {
		t.Fatal("the advisory is emitted once per Orchestrator")
	}

	raised := &Orchestrator{}
	raised.SetFinalizeRepairHardCap(10)
	SetMaxRepairAttemptsPerRoot(10)
	SetSameErrorClassRetryCap(0)
	if msg, emitted := raised.logClusterClosureExitReachabilityOnce(&types.AnalysisIR{TaskGraph: types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 10}}}); emitted {
		t.Fatalf("with every gate raised the exit is reachable; no advisory, got %q", msg)
	}
	if _, emitted := (*Orchestrator)(nil).logClusterClosureExitReachabilityOnce(ir); emitted {
		t.Fatal("nil orchestrator emits nothing")
	}
}

// The advisory is wired at scheduler entry: runReadSchedulerLoop calls
// logClusterClosureExitReachabilityOnce with the IR before its dispatch loop.
func TestRunReadSchedulerLoop_EmitsClusterClosureExitAdvisoryAtEntry(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "orchestrator.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var loop *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "runReadSchedulerLoop" {
			loop = fd
		}
	}
	if loop == nil || loop.Body == nil {
		t.Fatal("runReadSchedulerLoop not found")
	}
	var advisoryPos, firstForPos token.Pos
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ForStmt:
			if firstForPos == 0 {
				firstForPos = x.Pos()
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "logClusterClosureExitReachabilityOnce" && advisoryPos == 0 {
				advisoryPos = x.Pos()
				if len(x.Args) != 1 {
					t.Fatalf("the advisory must receive the IR (template retry budget), got %d args", len(x.Args))
				}
			}
		}
		return true
	})
	if advisoryPos == 0 {
		t.Fatal("runReadSchedulerLoop must call logClusterClosureExitReachabilityOnce at scheduler entry")
	}
	if firstForPos != 0 && advisoryPos > firstForPos {
		t.Fatalf("the advisory must be emitted before the dispatch loop (advisory at %v, loop at %v)", fset.Position(advisoryPos), fset.Position(firstForPos))
	}
}
