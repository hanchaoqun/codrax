package types

// trace_state_kinds_pin_test.go — TSH batch bidirectional StateKind word pin
// (memory reaudit P1-1, types-side half; the tool-side twin covers the
// renderer switches and the tracequery dominant-lane bridge).
//
//   - Universe golden: the 9-word closed set, exact.
//   - Production direction: the Object-fallback recognizer
//     (traceCausalProjectionCanonicalStateWord) accepts EXACTLY the universe
//     (it reads the registry, so this is a behavior pin, not a copy check),
//     and rejects non-state cause words / ThreadState members without a word
//     lane (stopped/dead/unknown).
//   - Consumption direction: every switch on a StateKind-derived string in
//     this package's projection files covers the universe, carries a default,
//     or declares fall-through members in the ledger; every string literal
//     compared against a StateKind-derived expression must be a registered
//     word (or ""). matched==0 fatals throughout. The scanner + rule checker
//     are the SHARED implementation in trace_state_kinds_scan.go (one scanner
//     for this pin and the tool-side pin — TSH review F6 removed the twin
//     copies whose own drift had gone unrecorded).

import (
	"reflect"
	"strings"
	"testing"
)

func TestTraceStateKindUniverseGolden(t *testing.T) {
	golden := []string{
		"running",
		"runnable",
		"sleep",
		"s_sleep",
		"sleep_wait",
		"d_sleep",
		"d_state",
		"io_wait",
		"uninterruptible_sleep",
	}
	if !reflect.DeepEqual(TraceStateKindUniverse, golden) {
		t.Fatalf("TraceStateKindUniverse drifted:\n got %v\nwant %v", TraceStateKindUniverse, golden)
	}
	for _, word := range golden {
		if !TraceStateKindRegistered(word) {
			t.Errorf("universe word %q not registered", word)
		}
	}
}

func TestTraceStateKindProductionRecognizer(t *testing.T) {
	// Universe ⊆ producible: the Object fallback recognizes every member and
	// returns it verbatim (TrimSpace only — display keeps the producer's
	// casing exactly as the historical switch did).
	for _, word := range TraceStateKindUniverse {
		if got := traceCausalProjectionCanonicalStateWord(word); got != word {
			t.Errorf("canonicalStateWord(%q) = %q, want the word itself", word, got)
		}
		spaced := "  " + strings.ToUpper(word[:1]) + word[1:] + " "
		if got := traceCausalProjectionCanonicalStateWord(spaced); got != strings.TrimSpace(spaced) {
			t.Errorf("canonicalStateWord(%q) = %q, want trimmed original casing %q", spaced, got, strings.TrimSpace(spaced))
		}
	}
	// Non-members never enter StateKind through the Object lane: cause
	// categories, consumer-only aliases, and the §7.11 B-1 non-lane
	// ThreadState members all stay out.
	for _, word := range []string{"compute_supply", "class_verification", "runnable_wait", "stopped", "dead", "unknown", "parked", ""} {
		if got := traceCausalProjectionCanonicalStateWord(word); got != "" {
			t.Errorf("canonicalStateWord(%q) = %q, want \"\" (non-members must not leak into the state column)", word, got)
		}
	}
}

// traceStateKindPinFiles are the StateKind consumer files in this package.
var traceStateKindPinFiles = []string{
	"trace_causal_projection.go",
	"trace_causal_projection_aggregate.go",
}

// traceStateKindClassifierFuncs are functions whose switch tag is a
// StateKind-carrying PARAMETER (callers pass node.StateKind / a state-word
// Object), so tag-text detection cannot see ".StateKind".
var traceStateKindClassifierFuncs = map[string]bool{
	"TraceCausalProjectionStateClass": true,
}

var traceStateKindSwitchFallthroughLedger = map[string]TraceStateKindFallthroughDecl{
	"trace_causal_projection.go:IsSleepState#1": {
		Missing: "running,runnable,d_sleep,d_state,io_wait,uninterruptible_sleep",
		Why:     "deliberately narrow S-sleep family: io_wait/d_state have their OWN inode/resource drilldown path, running/runnable are not sleeps",
	},
	"trace_causal_projection.go:TraceCausalProjectionStateClass#1": {
		Missing: "running,d_sleep,d_state,io_wait,uninterruptible_sleep",
		Why:     "RN-12 coverage classes are runnable + the S-sleep family ONLY (same narrowness rationale as IsSleepState); other states have no full-window cross-reference lane",
	},
}

var traceStateKindSwitchSiteGolden = map[string]string{
	"trace_causal_projection.go:IsSleepState#1":                    "sleep,s_sleep,sleep_wait",
	"trace_causal_projection.go:TraceCausalProjectionStateClass#1": "runnable,sleep,s_sleep,sleep_wait",
}

func TestTraceStateKindSwitchConsumerCoverage(t *testing.T) {
	// This package has no consumer-only aliases: the alias ledger is nil, so
	// any non-universe case/comparison literal is a straight violation (the
	// tool-side pin is the one carrying the RN-6 "runnable_wait" divergence).
	scan, err := ScanTraceStateKindConsumers(traceStateKindPinFiles, traceStateKindClassifierFuncs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range scan.Issues {
		t.Error(issue)
	}
	if len(scan.Sites) == 0 {
		t.Fatal("no StateKind consumer switches matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	if scan.Comparisons == 0 {
		t.Fatal("no StateKind comparison literals matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	for _, issue := range CheckTraceStateKindConsumerRules(scan, traceStateKindSwitchSiteGolden, traceStateKindSwitchFallthroughLedger, nil) {
		t.Error(issue)
	}
}
