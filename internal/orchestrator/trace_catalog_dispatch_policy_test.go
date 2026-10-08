package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceCatalogDispatchBus(t *testing.T) *types.BusContext {
	t.Helper()
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect captures")}
	if err := os.WriteFile(filepath.Join(bus.RepoRoot, "capture"), []byte("format not asserted by discovery"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceCatalog{}).Execute(bus, json.RawMessage(`{"action":"discover"}`))
	if err != nil || !result.Success || len(bus.Mutable.TraceCatalogs()) != 1 {
		t.Fatalf("current authorized discovery: %+v %v", result, err)
	}
	if bus.RuntimeArtifactPreflight.HasTraceArtifact() || bus.Mutable.TraceQueryRuntimeObservationCount() != 0 {
		t.Fatal("navigation must not mint a trace preflight or measurement")
	}
	return bus
}

func traceCatalogProofPolicy() types.ReadDispatchPolicy {
	return readDispatchPolicyForNextAction(readLoopNextActionDecision{Active: true, Action: loopkernel.LoopActionAddProof}, nil)
}

func TestTraceCatalogReadDispatchContinuationAndBudget(t *testing.T) {
	bus := traceCatalogDispatchBus(t)
	policy := traceCatalogProofPolicy()
	before := types.NormalizeReadDispatchPolicy(policy)
	priorBudget := &types.ExploreBudget{OverallCap: 20, PerToolCap: map[string]int{"trace_query": 2}}
	bus.Mutable.SetExploreBudget(priorBudget)
	o := &Orchestrator{busCtx: bus}
	restore := o.installReadDispatchPolicyForExplore(policy, true)
	installed := bus.ReadDispatchPolicy
	for _, name := range append(append([]string(nil), policy.AllowedTools...), "trace_query", "trace_catalog") {
		if !installed.AllowsTool(name) {
			t.Fatalf("proof continuation lost %q: %+v", name, installed)
		}
	}
	if !reflect.DeepEqual(policy, before) || !reflect.DeepEqual(installed.ScopePaths, before.ScopePaths) ||
		!reflect.DeepEqual(installed.DeniedTools, before.DeniedTools) || installed.MaxToolCalls != before.MaxToolCalls {
		t.Fatal("catalog tool visibility changed source scope, denials, budget or caller policy")
	}
	if twice := admitLiveTraceCatalogToolsIntoReadDispatchPolicy(installed, bus); !reflect.DeepEqual(twice, installed) {
		t.Fatal("catalog admission is not idempotent")
	}
	budget := bus.Mutable.ExploreBudget()
	if budget == nil || budget.OverallCap != before.MaxToolCalls || budget.PerToolCap["trace_catalog"] != before.MaxToolCalls || budget.PerToolCap["trace_query"] != 2 {
		t.Fatalf("bounded continuation lost its caps: %+v", budget)
	}
	restore()
	if bus.ReadDispatchPolicy.Active || !reflect.DeepEqual(bus.Mutable.ExploreBudget(), priorBudget) {
		t.Fatal("one-dispatch policy/budget were not restored")
	}
}

func TestTraceCatalogReadDispatchHonorsEachExplicitDenial(t *testing.T) {
	bus := traceCatalogDispatchBus(t)
	for _, denied := range [][]string{{"trace_query"}, {"trace_catalog"}, {"trace_query", "trace_catalog"}} {
		policy := traceCatalogProofPolicy()
		policy.DeniedTools = append(policy.DeniedTools, denied...)
		restore := (&Orchestrator{busCtx: bus}).installReadDispatchPolicyForExplore(policy, true)
		for _, name := range []string{"trace_query", "trace_catalog"} {
			want := true
			for _, disallowed := range denied {
				want = want && name != disallowed
			}
			if bus.ReadDispatchPolicy.AllowsTool(name) != want {
				t.Fatalf("denied=%v tool=%s policy=%+v", denied, name, bus.ReadDispatchPolicy)
			}
		}
		restore()
	}
}

func TestTraceCatalogReadDispatchNoFormOrHistoricalAuthority(t *testing.T) {
	bus := traceCatalogDispatchBus(t)
	landing, _, active := localLandingRepairDispatchPolicy([]types.RepairDirective{{Kind: types.RepairStructuredHandoff, Origin: types.RepairOriginCompletionFormPrefix + "reason"}}, nil, nil, nil, "", "", nil)
	if !active {
		t.Fatal("missing real form-only lane")
	}
	proof := traceCatalogProofPolicy()
	for name, edit := range map[string]func(*types.ReadDispatchPolicy){
		"inactive":      func(p *types.ReadDispatchPolicy) { p.Active = false },
		"unrestricted":  func(p *types.ReadDispatchPolicy) { p.AllowedTools = nil },
		"other_action":  func(p *types.ReadDispatchPolicy) { p.Action = "repair" },
		"other_surface": func(p *types.ReadDispatchPolicy) { p.RouteSurface = types.ReadDispatchPolicySurfaceHandoff },
		"form_only":     func(p *types.ReadDispatchPolicy) { *p = landing },
	} {
		t.Run(name, func(t *testing.T) {
			p := proof
			edit(&p)
			if got := admitLiveTraceCatalogToolsIntoReadDispatchPolicy(p, bus); !reflect.DeepEqual(got, p) {
				t.Fatalf("non-proof policy gained tools: before=%+v after=%+v", p, got)
			}
		})
	}
	for _, without := range []*types.BusContext{nil, {}, {Mutable: types.NewMutableState("trace_catalog source text is not a handle")}} {
		if got := admitLiveTraceCatalogToolsIntoReadDispatchPolicy(proof, without); !reflect.DeepEqual(got, proof) {
			t.Fatal("missing live authority changed policy")
		}
	}
	// Persisted navigation and an old fork survive as values, but neither can
	// reintroduce current handles after the turn reset.
	fork := bus.Mutable.ForkForExploreDispatch()
	snapshot, err := json.Marshal(bus.Mutable.TraceCatalogs()[0].Snapshot())
	if err != nil || !json.Valid(snapshot) {
		t.Fatal(err)
	}
	bus.Mutable.ResetTurnAArtifacts()
	bus.Mutable.MergeExploreFork(fork)
	if len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatal("old catalog lineage revived after reset")
	}
	if got := admitLiveTraceCatalogToolsIntoReadDispatchPolicy(proof, bus); !reflect.DeepEqual(got, proof) {
		t.Fatal("historical navigation reopened continuation permissions")
	}
}
