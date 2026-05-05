package agent

import (
	"os"
	"strings"
	"testing"
)

// TestBuildAnalysisIR_AmplifierInsertionOrder is the Phase 1.2 nail-
// down test: it asserts that the amplifier wiring in
// internal/agent/analyzer.go::buildAnalysisIR sits at the documented
// insertion points.
//
// Why source-level instead of behavioural: the amplifier's empty
// registry (Phase 1.1) means a runtime test cannot observe ordering
// without first registering a rule. A behavioural test would also
// have to drive buildAnalysisIR with a fully populated
// AgentContext + repomap graph + hypotheses just to exercise
// ordering — high friction for low signal. The source-level
// assertion catches the regression we actually care about (someone
// moves the amplifier call past reconcileSemanticPredicates or
// past compiler.Compile) and stays meaningful as Phase 2/3 rules
// land, because the call sites do not move.
//
// docs/design/analyzer_amplifier_layer.md §4.2 specifies the
// insertion contract:
//   - Amplify before reconcileSemanticPredicates and before
//     compiler.Compile
//   - AmplifyPostCompile after compiler.RecomputeBudget and
//     before binder.BindByRelevance
func TestBuildAnalysisIR_AmplifierInsertionOrder(t *testing.T) {
	src, err := os.ReadFile("analyzer.go")
	if err != nil {
		t.Fatalf("read analyzer.go: %v", err)
	}
	body := string(src)

	idxAmplify := strings.Index(body, "amplifier.Amplify(rm)")
	if idxAmplify < 0 {
		t.Fatal("amplifier.Amplify(rm) call missing from buildAnalysisIR — Phase 1.2 wiring lost")
	}
	idxReconcilePreds := strings.Index(body, "reconcileSemanticPredicates(rm)")
	if idxReconcilePreds < 0 {
		t.Fatal("reconcileSemanticPredicates call missing — has the reconcile chain been renamed?")
	}
	if idxAmplify >= idxReconcilePreds {
		t.Errorf(
			"amplifier.Amplify must be called BEFORE reconcileSemanticPredicates "+
				"(got Amplify@%d, reconcile@%d) — see docs/design/analyzer_amplifier_layer.md §4.2",
			idxAmplify, idxReconcilePreds,
		)
	}

	idxCompile := strings.Index(body, "out := compiler.Compile(rm, sig)")
	if idxCompile < 0 {
		t.Fatal("compiler.Compile call missing — has the analyzer pipeline been refactored?")
	}
	if idxAmplify >= idxCompile {
		t.Errorf(
			"amplifier.Amplify must be called BEFORE compiler.Compile "+
				"(got Amplify@%d, Compile@%d) — pre-compile pass invariant",
			idxAmplify, idxCompile,
		)
	}

	idxAmplifyPost := strings.Index(body, "amplifier.AmplifyPostCompile(rm, &out.AnswerContract)")
	if idxAmplifyPost < 0 {
		t.Fatal("amplifier.AmplifyPostCompile call missing from buildAnalysisIR — Phase 1.2 wiring lost")
	}
	idxRecompute := strings.Index(body, "compiler.RecomputeBudget(&out, rm, sig)")
	if idxRecompute < 0 {
		t.Fatal("compiler.RecomputeBudget call missing — has the analyzer pipeline been refactored?")
	}
	if idxAmplifyPost <= idxRecompute {
		t.Errorf(
			"amplifier.AmplifyPostCompile must be called AFTER compiler.RecomputeBudget "+
				"(got Recompute@%d, AmplifyPost@%d)",
			idxRecompute, idxAmplifyPost,
		)
	}
	idxBind := strings.Index(body, "binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{})")
	if idxBind < 0 {
		t.Fatal("binder.BindByRelevance call missing — has the binder been renamed?")
	}
	if idxAmplifyPost >= idxBind {
		t.Errorf(
			"amplifier.AmplifyPostCompile must be called BEFORE binder.BindByRelevance "+
				"(got AmplifyPost@%d, Bind@%d) — Phase 4 R3 must surface MustInclude before binder runs",
			idxAmplifyPost, idxBind,
		)
	}
}
