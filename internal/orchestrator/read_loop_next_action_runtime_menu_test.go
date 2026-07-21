package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeMenuTestMutable(t *testing.T) *types.MutableState {
	t.Helper()
	mut := types.NewMutableState("trace run")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedResultKind:    "resolved",
		AcceptedClosureReason: "trace evidence sufficient",
		EvidenceItems: []types.EvidenceItem{{
			ID:      "ev-1",
			Source:  "test_trace_02.systrace",
			Summary: "wakeup chain frontier",
		}},
	})
	return mut
}

func runtimeMenuTestBus(mut *types.MutableState, withTrace bool) *types.BusContext {
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"不分析代码"},
			},
		}},
	}
	if withTrace {
		bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:   "trace",
				Source: "test_trace_02.systrace",
			}},
		}
	}
	return bus
}

// TestReadLoopCheckpointMenu_RuntimeArtifactOnlyFiltersRepoTools pins
// §29.174 RUN2AUDIT-1 F8 against the runnable_2.txt witness (:75): a
// pure-trace run (typed gate = Run-entry trace preflight ∧ analyzer
// explicit current-source exclusion) whose resume checkpoint reaches
// the add_proof lane must RECOMMEND trace-side proof tools on the
// route_tools face, not the repo-code family
// (run_tests/repo_map/read_file/grep), and must name the structural
// gap (closure_incomplete) instead of proof_weak when the typed
// accepted-closure state is already present.
//
// Only the advisory faces (route_tools / preferred) are filtered. The
// policy_allowed_tools face words the ENFORCEMENT allowlist, which
// deliberately keeps read_file/grep executable on trace runs (blob
// payload drill §29.19/§29.121; trace_query's documented fallback for
// unsupported formats) — merge review 2026-07-20 rolled back the
// original enforcement-side filter as an out-of-discipline hard gate.
func TestReadLoopCheckpointMenu_RuntimeArtifactOnlyFiltersRepoTools(t *testing.T) {
	mut := runtimeMenuTestMutable(t)
	summary := readLoopAddProofActionSummaryForBus(runtimeMenuTestBus(mut, true), mut)
	if summary == "" {
		t.Fatalf("fixture must produce an active add_proof decision summary")
	}
	routeTools := ""
	for _, field := range strings.Fields(summary) {
		if strings.HasPrefix(field, "route_tools=") {
			routeTools = field
		}
	}
	if routeTools == "" {
		t.Fatalf("summary must carry a route_tools face:\n%s", summary)
	}
	for _, forbidden := range []string{"run_tests", "repo_map", "read_file", "grep"} {
		if strings.Contains(routeTools, forbidden) {
			t.Errorf("runtime-artifact-only route_tools face must not recommend repo-code tool %q:\n%s", forbidden, summary)
		}
	}
	if !strings.Contains(routeTools, "trace_query") {
		t.Errorf("runtime-artifact-only route_tools face must admit trace_query into the menu:\n%s", summary)
	}
	// Enforcement disclosure stays honest: read_file remains allowed
	// (blob drill lane) and trace_query is admitted, matching what the
	// install-time §29.118 arm will actually enforce.
	allowedTools := ""
	for _, field := range strings.Fields(summary) {
		if strings.HasPrefix(field, "policy_allowed_tools=") {
			allowedTools = field
		}
	}
	if allowedTools == "" {
		t.Fatalf("summary must carry a policy_allowed_tools face:\n%s", summary)
	}
	if !strings.Contains(allowedTools, "read_file") {
		t.Errorf("enforcement allowlist must keep read_file executable (trace payload blob drill):\n%s", summary)
	}
	if !strings.Contains(allowedTools, "trace_query") {
		t.Errorf("enforcement allowlist disclosure must include the admitted trace_query:\n%s", summary)
	}
	if !strings.Contains(summary, "reason=closure_incomplete") {
		t.Errorf("structural-closure gap must be named closure_incomplete, not proof_weak:\n%s", summary)
	}
	// The ROUTE face ("source=proof_authority reason=...") is the menu
	// the model reads — it must carry the relabeled word. The trailing
	// "loop shadow ..." segment is the kernel's diagnostic mirror and
	// deliberately keeps the raw authority reason (rewording a shadow
	// comparison face would falsify the kernel state it exists to show).
	if strings.Contains(summary, "source=proof_authority reason=proof_weak") {
		t.Errorf("proof_weak must not survive on the relabeled route face:\n%s", summary)
	}
	// The action lane itself is untouched — soft menu/word faces only.
	if !strings.Contains(summary, "loop next-action=add_proof") {
		t.Errorf("add_proof action must stay untouched:\n%s", summary)
	}
}

// TestReadLoopCheckpointMenu_NonTraceRunUnchanged is the negative arm:
// without the trace-artifact preflight leg the typed gate must not
// fire — the historical repo-tool vocabulary and the proof_weak reason
// stay byte-present, proving the filter cannot leak onto ordinary
// repository runs.
func TestReadLoopCheckpointMenu_NonTraceRunUnchanged(t *testing.T) {
	mut := runtimeMenuTestMutable(t)
	summary := readLoopAddProofActionSummaryForBus(runtimeMenuTestBus(mut, false), mut)
	if summary == "" {
		t.Fatalf("fixture must produce an active add_proof decision summary")
	}
	if !strings.Contains(summary, "run_tests") {
		t.Errorf("non-trace run must keep the historical verification vocabulary:\n%s", summary)
	}
	if !strings.Contains(summary, "reason=proof_weak") {
		t.Errorf("non-trace run must keep the proof_weak reason:\n%s", summary)
	}
	if strings.Contains(summary, "closure_incomplete") {
		t.Errorf("closure_incomplete must not leak onto non-trace runs:\n%s", summary)
	}
	// nil-bus caller (legacy summary surface) also stays unchanged.
	legacy := readLoopAddProofActionSummaryFromMutable(mut)
	if !strings.Contains(legacy, "run_tests") || !strings.Contains(legacy, "reason=proof_weak") {
		t.Errorf("nil-bus summary must stay on the historical face:\n%s", legacy)
	}
}

// TestReadLoopCheckpointMenu_GateNeedsBothTypedLegs pins the ∧ shape
// of the §29.174 F8 gate: EACH leg alone must not fire the rewrite.
// A trace-attached run WITHOUT the analyzer's explicit current-source
// exclusion (mixed code+trace question) and a log-only attachment run
// (artifact present, wrong kind) both keep the historical face —
// filtering repo-tool advice on a run whose question may need the
// repository would invert the fix into new misguidance.
func TestReadLoopCheckpointMenu_GateNeedsBothTypedLegs(t *testing.T) {
	// Leg A alone: trace preflight present, no exclusion policy.
	mut := runtimeMenuTestMutable(t)
	bus := runtimeMenuTestBus(mut, true)
	bus.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
	summary := readLoopAddProofActionSummaryForBus(bus, mut)
	if !strings.Contains(summary, "run_tests") || !strings.Contains(summary, "reason=proof_weak") {
		t.Errorf("trace-without-exclusion run must keep the historical face:\n%s", summary)
	}
	if strings.Contains(summary, "closure_incomplete") {
		t.Errorf("closure_incomplete must not fire without the exclusion leg:\n%s", summary)
	}

	// Leg B alone: explicit exclusion present, but the only preflight
	// artifact is a LOG (HasTraceArtifact must stay false).
	mut2 := runtimeMenuTestMutable(t)
	bus2 := runtimeMenuTestBus(mut2, false)
	bus2.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:   "log",
			Source: "panic.txt",
		}},
	}
	summary2 := readLoopAddProofActionSummaryForBus(bus2, mut2)
	if !strings.Contains(summary2, "run_tests") || !strings.Contains(summary2, "reason=proof_weak") {
		t.Errorf("log-only attachment run must keep the historical face:\n%s", summary2)
	}
}
