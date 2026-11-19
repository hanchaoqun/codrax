package tool

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// EVOLUTION RECORD 2026-11-19 (HMC377 E2, product-owner ruling 2026-11-18
// "同意继续"): the wakeup-credential sentences of the trace_query Description
// are REPLACED (never added to) by one rule sentence quoted from
// skill.TraceWakeupDependencyCreditTeaching: a thread that woke the analysed
// thread is a cause only for its own time inside the wait that wakeup ended.
// Three existing slots change: the closed-matrix contract sentence, the
// semantic span-work row paragraph, and the root-cause comparison replace arm
// (the internal "edge=credential, pre-edge=effective, post-edge=released"
// formula leaves every model-facing surface). No view, note key, parameter or
// evidence permission changes. Independent literals below reverse exactly
// these three slots so every older full-description digest stays pinned; the
// h2/h3 dispatch A/B the golden ritual asks for was not run in this lane (no
// model calls) — open live debt, recorded in the lane document.

const traceDependencyCreditRule = "A thread that woke the analysed thread counts as a cause only for its own time (ready, blocked, or deterministic work) inside the wait of the analysed thread that this wakeup ended; its time before that wait began or after the wakeup is supporting context, and a wakeup that found the analysed thread not waiting counts nothing."

var traceDependencyCreditDescriptionDeltas = [][2]string{
	{
		traceDependencyCreditRule + " A non-target semantic span priced by that rule, and one intersecting a typed chain interval (priced by that exact intersection), are on-chain effective attribution under the same rule as every state family;",
		"A non-target semantic span preceding its host's typed wakeup edge prices its pre-edge share, and one intersecting a typed chain interval prices that exact intersection, as on-chain effective attribution — the single credential rule for every state family (edge=credential, pre-edge=effective, post-edge=released);",
	},
	{
		"A non-target semantic span seated by its host's wakeup of the analysed thread (host_wakeup_edge_pre_span) is priced by the wakeup-dependency rule of the root-cause matrix, and a span intersecting a typed chain interval (semantic_chain_interval_relation) prices that exact intersection, as on-chain effective attribution under the same rule as every state family;",
		"A non-target semantic span seated by its host's typed wakeup edge (host_wakeup_edge_pre_span) prices its PRE-EDGE share as on-chain effective attribution and a span intersecting a typed chain interval (semantic_chain_interval_relation) prices that exact intersection — the single credential rule for every state family (edge=credential, pre-edge=effective, post-edge=released);",
	},
	{
		"rows join that comparison by their priced share (mechanism-unproven is a disclosure, not a pricing verdict)",
		"rows join that comparison by their priced pre-edge/intersection share (mechanism-unproven is a disclosure, not a pricing verdict)",
	},
}

// traceDescriptionBeforeDependencyCreditSHA is the full canonical golden of
// d5ef1c6fe (before this evolution), not a regenerated hash.
const traceDescriptionBeforeDependencyCreditSHA = "06dd871acefa805b82c9edcc652bf11c9e4f43e5f6aef83ce7c755354bddb2a1"

func reverseTraceQueryDependencyCreditEvolution(description string) (string, bool) {
	for _, delta := range traceDependencyCreditDescriptionDeltas {
		if strings.Count(description, delta[0]) != 1 || strings.Contains(description, delta[1]) {
			return "", false
		}
		description = strings.Replace(description, delta[0], delta[1], 1)
	}
	return description, true
}

func traceQueryDescriptionBeforeDependencyCreditEvolution(t *testing.T, description string) string {
	t.Helper()
	prior, ok := reverseTraceQueryDependencyCreditEvolution(description)
	if !ok {
		t.Fatal("dependency-credit teaching is not the unique approved replacement of its three slots")
	}
	return prior
}

func TestHMC377TraceQueryDependencyCreditDescriptionOnlyEvolution(t *testing.T) {
	description := (&TraceQuery{}).Description()
	prior := traceQueryDescriptionBeforeDependencyCreditEvolution(t, description)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != traceDescriptionBeforeDependencyCreditSHA {
		t.Fatalf("Description changed outside the exact approved dependency-credit replacement: %s", got)
	}
	for _, retired := range []string{"edge=credential", "pre-edge=effective", "post-edge=released", "PRE-EDGE share"} {
		if strings.Contains(description, retired) {
			t.Errorf("the retired internal formula %q is still model-facing", retired)
		}
	}
	if strings.Count(description, traceDependencyCreditRule) != 1 {
		t.Error("the rule sentence must appear exactly once (one source, quoted)")
	}
	for name, mutated := range map[string]string{
		"missing":   strings.Replace(description, traceDependencyCreditRule, "", 1),
		"duplicate": description + " " + traceDependencyCreditRule,
		"old_rule":  description + " (edge=credential, pre-edge=effective, post-edge=released);",
	} {
		if other, ok := reverseTraceQueryDependencyCreditEvolution(mutated); ok &&
			fmt.Sprintf("%x", sha256.Sum256([]byte(other))) == traceDescriptionBeforeDependencyCreditSHA {
			t.Errorf("%s unapproved edit was concealed by the reverse projection", name)
		}
	}
}
