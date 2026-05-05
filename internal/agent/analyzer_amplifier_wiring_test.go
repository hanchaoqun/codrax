package agent

import (
	"os"
	"strings"
	"testing"
)

// TestBuildAnalysisIR_AmplifierInsertionOrder is the Phase 1.2 / 2.1
// nail-down test: it asserts that the amplifier wiring in
// internal/agent/analyzer.go::buildAnalysisIR sits at the documented
// insertion points.
//
// Why source-level instead of behavioural: a runtime test would have
// to drive buildAnalysisIR with a fully populated AgentContext +
// repomap graph + hypotheses just to exercise ordering — high
// friction for low signal. The source-level assertion catches the
// regression we actually care about (someone moves the amplifier
// call before normalizer.Normalize or past compiler.Compile).
//
// Phase 2.1 corrected the pre-compile insertion point: the original
// design placed Amplify before the reconcile chain, but rm.TermGraph
// is not populated until normalizer.Normalize runs LATER in the
// function. R1 reads rm.TermGraph.Canonical, so without TermGraph
// data R1 cannot fire. The corrected contract:
//   - Amplify after normalizer.Normalize (so TermGraph is populated)
//     and before compiler.Compile (so the template picks on the
//     amplified predicates).
//   - AmplifyPostCompile after compiler.RecomputeBudget and before
//     binder.BindByRelevance.
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
	idxNormalize := strings.Index(body, "rm.TermGraph = normalizer.Normalize(")
	if idxNormalize < 0 {
		t.Fatal("normalizer.Normalize assignment missing — has the analyzer pipeline been refactored?")
	}
	if idxAmplify <= idxNormalize {
		t.Errorf(
			"amplifier.Amplify must be called AFTER normalizer.Normalize populates rm.TermGraph "+
				"(got Normalize@%d, Amplify@%d). R1 reads TermGraph.Canonical and fires zero "+
				"rules without it — see docs/design/analyzer_amplifier_layer.md §4.2",
			idxNormalize, idxAmplify,
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
