// Package context — bug_classes_intent_aligned_test.go (2026-05-10).
//
// User-intent-aligned reframe of renderBugClassesSection. Three
// structural changes, all rooted in the red line "user-question
// intent drives the answer focus, the system must NOT correct user
// intent":
//
//   1. No "USE THESE EXACT TERMS" mandate — terminology is
//      conditional on whether the user asked about failure KIND.
//   2. "VOCABULARY AID" → "FACTUAL CONTEXT" reframing — labels are
//      data, not lexical hints, but still don't override the user's
//      question intent.
//   3. Evidence priority no longer leads with "function names".
//      Argument dumps (e.g. (0x0)) explicitly tagged as
//      observation-only and NOT diagnostic of failure KIND.
//
// Forensic anchor: 2026-05-10 sweep b3jmwty80 logtri_goroutine_dump
// where the model read (0x0) arg as "nil map" and overrode the
// canonical "concurrent map writes" classification.
package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func raceBugClass() types.DetectedBugClass {
	return types.DetectedBugClass{
		Class:            types.BugClassRace,
		HumanZh:          "数据竞争(并发 map 写)",
		HumanEn:          "data race (concurrent map writes)",
		MatchedSignature: "fatal error: concurrent map writes",
		SourceLang:       "go",
	}
}

// === Removed-mandate assertions ===

func TestRenderBugClasses_NoForceTermsMandate(t *testing.T) {
	// User-intent red line: the renderer must NOT order the model
	// to "USE THESE EXACT TERMS" (the old phrasing). That mandate
	// forced terminology even on questions where the failure kind
	// wasn't what the user asked about.
	out := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	for _, banned := range []string{
		"USE THESE EXACT TERMS",
		"VOCABULARY AID",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("removed phrase %q reappeared in output:\n%s", banned, out)
		}
	}
}

func TestRenderBugClasses_FactualContextFraming(t *testing.T) {
	// New framing: classification is FACTUAL CONTEXT, not a term
	// mandate. Confirms the deliberate reframe wording lands.
	out := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	if !strings.Contains(out, "FACTUAL CONTEXT") {
		t.Errorf("expected FACTUAL CONTEXT framing; got:\n%s", out)
	}
	if !strings.Contains(out, "User-question intent drives the answer focus") {
		t.Errorf("expected user-intent-driven framing; got:\n%s", out)
	}
}

// === Intent split assertions ===

func TestRenderBugClasses_KindIntent_VerbatimEncouraged(t *testing.T) {
	// When the user asks about FAILURE KIND, the renderer should
	// signal that the canonical term is the right answer.
	out := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	if !strings.Contains(out, "FAILURE KIND") {
		t.Errorf("expected FAILURE KIND branch; got:\n%s", out)
	}
	if !strings.Contains(out, "use the term verbatim") {
		t.Errorf("expected verbatim guidance for kind questions; got:\n%s", out)
	}
}

func TestRenderBugClasses_OtherIntent_NotForced(t *testing.T) {
	// When the user asks about anything else (timing / scope /
	// audit / etc.), the renderer should explicitly defer to user
	// intent.
	out := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	for _, intent := range []string{"timing", "scope", "frequency", "audit"} {
		if !strings.Contains(out, intent) {
			t.Errorf("expected non-kind intent %q in else-branch enumeration; got:\n%s", intent, out)
		}
	}
	if !strings.Contains(out, "Do NOT force the canonical term") {
		t.Errorf("expected explicit anti-force directive; got:\n%s", out)
	}
}

// === Argument-dump observability assertion ===

func TestRenderBugClasses_ArgDumpObservationOnly(t *testing.T) {
	// arg-dump must be flagged as observation-only with concrete
	// examples, NOT abstract "positional values" jargon, AND
	// explicitly NOT diagnostic of failure kind.
	out := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	for _, concrete := range []string{"(0x0)", "null", "register snapshots", "observation-only"} {
		if !strings.Contains(out, concrete) {
			t.Errorf("expected concrete arg-dump example %q in renderer; got:\n%s", concrete, out)
		}
	}
	if !strings.Contains(out, "NOT diagnostic of WHAT KIND of failure") {
		t.Errorf("expected explicit 'NOT diagnostic of failure kind' for arg dumps; got:\n%s", out)
	}
}

// === Modality symmetry: log + trace share the renderer ===

func TestRenderBugClasses_LogAndTraceShareSemantics(t *testing.T) {
	// Both modalities must carry the same user-intent framing
	// — same renderer, same red lines.
	logOut := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	traceOut := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "trace")
	for _, expected := range []string{
		"FACTUAL CONTEXT",
		"FAILURE KIND",
		"User-question intent drives",
		"Do NOT force the canonical term",
		"observation-only",
	} {
		if !strings.Contains(logOut, expected) {
			t.Errorf("log modality missing %q", expected)
		}
		if !strings.Contains(traceOut, expected) {
			t.Errorf("trace modality missing %q", expected)
		}
	}
	// Modality-specific noun substitution still works.
	if !strings.Contains(logOut, "log") || !strings.Contains(traceOut, "trace") {
		t.Errorf("modality noun substitution broken")
	}
	if strings.Contains(logOut, "tag names") {
		t.Errorf("log modality leaked trace-only descriptor 'tag names'")
	}
	if strings.Contains(traceOut, "function names, the message text") {
		t.Errorf("trace modality leaked log-only contentDescriptor wording")
	}
}

// === Empty-pattern branch sanity (unchanged path still works) ===

func TestRenderBugClasses_EmptyPatterns_AddressUserQuestion(t *testing.T) {
	out := renderBugClassesSection(nil, "log")
	if !strings.Contains(out, "Pattern Classification") {
		t.Errorf("empty-pattern branch should render '### Pattern Classification' heading; got:\n%s", out)
	}
	if !strings.Contains(out, "address the user's question") {
		t.Errorf("empty-pattern branch should preserve user-intent framing; got:\n%s", out)
	}
}

// === Issue A 2026-05-10: non-enumerative reframe ===

// TestRenderBugClasses_HowToReadPreambleFiresAlways pins that the
// "Read + judge" preamble appears for both empty and non-empty
// bug_class detection — the registry is finite enumeration; the
// preamble is the unconditional "trust your reading" anchor.
func TestRenderBugClasses_HowToReadPreambleFiresAlways(t *testing.T) {
	emptyOut := renderBugClassesSection(nil, "log")
	matchOut := renderBugClassesSection([]types.DetectedBugClass{raceBugClass()}, "log")
	for _, out := range []string{emptyOut, matchOut} {
		if !strings.Contains(out, "How to read this log") {
			t.Errorf("preamble missing — must fire on every render: %q", out)
		}
		if !strings.Contains(out, "form your own judgment") {
			t.Errorf("preamble must invite the model to form its own judgment: %q", out)
		}
		if !strings.Contains(out, "FACTUAL CONTEXT") {
			t.Errorf("preamble must label registry output as FACTUAL CONTEXT, not verdict: %q", out)
		}
		if !strings.Contains(out, "do NOT force-fit") {
			t.Errorf("preamble must explicitly authorise non-fit when registry labels do not match what the model sees: %q", out)
		}
	}
}

// TestRenderBugClasses_BroadShapeCoverage pins that the empty
// branch covers a wide cross-stack mental space — failure /
// retry / resource / concurrency / config / connectivity / build /
// security / database / runtime hot-path / AI / behavioural /
// informational. This is the "do not assume crash, do not assume
// the registry is exhaustive" guidance the customer-reported
// REPL forced-read iteration storm showed was missing, generalised
// 2026-05-10 to cover many language / app / orchestration scenarios.
func TestRenderBugClasses_BroadShapeCoverage(t *testing.T) {
	out := renderBugClassesSection(nil, "log")
	for _, shape := range []string{
		// non-crash shapes the customer scenario surfaced
		"Iteration / retry / feedback-loop",
		"forced-read or forced-action cycles",
		"tool-call retry storms",
		// resource / capacity (cross-language)
		"Resource / capacity / throttling / budget",
		"OOMKilled",
		"connection-pool",
		// concurrency / timing
		"Concurrency / ordering / timing / latency",
		"replication lag",
		// configuration drift
		"Configuration / drift / version-skew",
		"schema-version mismatch",
		// connectivity / dependency
		"Connectivity / dependency / discovery",
		"DNS resolution failure",
		// build / deployment / orchestration
		"Build / test / deployment / orchestration",
		"CrashLoopBackoff",
		// security / integrity
		"Security / policy / integrity",
		"signature / hash failure",
		// database / persistence
		"Database / persistence / cache",
		"deadlock graph",
		// runtime hot-path / GC / JIT
		"runtime hot-path / GC / JIT",
		// AI / agent / tool-call (the customer scenario class)
		"AI / agent / tool-call",
		"LLM iteration loop",
		// behavioural
		"Behavioural / business / audit",
		// clean baseline
		"Informational baseline",
		// the meta-rule: list is non-exhaustive
		"deliberately incomplete",
		"Do NOT force-fit",
	} {
		if !strings.Contains(out, shape) {
			t.Errorf("empty branch missing %q from broad-coverage list: got %q", shape, out)
		}
	}
}

// TestRenderBugClasses_TraceModalityHasMatchingPreamble — modality
// substitution preserves the preamble shape for "trace" inputs.
func TestRenderBugClasses_TraceModalityHasMatchingPreamble(t *testing.T) {
	out := renderBugClassesSection(nil, "trace")
	if !strings.Contains(out, "How to read this trace") {
		t.Errorf("trace modality must substitute noun in preamble heading: %q", out)
	}
	if strings.Contains(out, "How to read this log") {
		t.Errorf("log-noun leaked into trace modality: %q", out)
	}
}

// TestRenderBugClasses_PreambleR6 — internal pipeline jargon must
// stay out of the LLM-facing preamble (R6 red line).
func TestRenderBugClasses_PreambleR6(t *testing.T) {
	out := renderBugClassesSection(nil, "log")
	for _, banned := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"BusContext", "MutableState",
		"L1 gate", "L2 gate", "L3", "L4",
		"BugClass", "DetectedBugClass",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("internal term %q leaked into preamble: %q", banned, out)
		}
	}
}

// === Cross-language test: every bug class flows through the same gate ===

func TestRenderBugClasses_NilDerefAlsoIntentAligned(t *testing.T) {
	// Whatever the BugClass, the renderer must apply the same
	// user-intent framing — never over-fit to race / nil_deref
	// individually.
	bc := types.DetectedBugClass{
		Class:            types.BugClassNilDeref,
		HumanZh:          "空指针(nil)解引用",
		HumanEn:          "nil pointer dereference",
		MatchedSignature: "runtime error: invalid memory address or nil pointer dereference",
		SourceLang:       "go",
	}
	out := renderBugClassesSection([]types.DetectedBugClass{bc}, "log")
	if !strings.Contains(out, "FACTUAL CONTEXT") {
		t.Errorf("nil_deref class missing FACTUAL CONTEXT framing")
	}
	if strings.Contains(out, "USE THESE EXACT TERMS") {
		t.Errorf("nil_deref class regressed to old VOCABULARY AID phrasing")
	}
}
