package tracequery

import (
	"strings"
	"testing"
)

// HMC377 CV-1: the class-verification literals are word-bounded. ART's
// "VerifyClassLoaderContextMatch" checks that the compiled code's recorded
// class-loader context matches while that code is opened; it is not class
// verification and must not join the class-verification family (it did in
// every one of the 75 baseline captures: 3 to 7 markers each, 251 in all
// (58 captures with 3, 10 with 4, 6 with 5, 1 with 7); the longest is
// 1.075 ms (105 网易云音乐 background-kill cold start #3,
// ftrace-20261019_17.55.02, TID 37022); Douyin reboot5 TID 11105 counted
// 127 / 92.266 ms instead of 126 / 92.026 because of its one 0.240 ms marker).
//
// EVOLUTION RECORD (HMC377 round 9, R9-4, register R8C-2, 2026-11-21): this
// comment said "1-7 markers each, up to 0.24 ms"; the measured census is the
// one above (fix8/review-cv1/oracle/vlcm_dur.tsv; TIDs fix9/r4/vlcm_tid.out).
func TestHMC377CV1ClassVerificationLiteralsAreWordBounded(t *testing.T) {
	for _, name := range []string{
		"VerifyClass com.example.Foo",
		"VerifyClass Lcom/example/Foo;",
		"H:VerifyClass com.example.Foo",
		"art::ClassVerifier::VerifyClass",
		"verify_class(foo)",
		"class_verification",
		"ClassVerifier",
		"class verifier pass",
		"Verify class com.example.Foo",
	} {
		if got := traceSpanSemanticClass(name); got != "class_verification" {
			t.Errorf("%q: class %q, want class_verification", name, got)
		}
	}
	for _, name := range []string{
		"VerifyClassLoaderContextMatch",
		"H:VerifyClassLoaderContextMatch",
		"ReverifyClassTable",
		"classverifiers2",
	} {
		if got := traceSpanSemanticClass(name); got == "class_verification" {
			t.Errorf("%q: must not be class verification", name)
		}
	}
}

func TestHMC377CV1WordBoundedLiteralFindsALaterBoundedOccurrence(t *testing.T) {
	// The first occurrence runs into a letter; a later one is bounded.
	if !traceSpanContainsWordBoundedLiteral("verifyclassloader verifyclass foo", "verifyclass") {
		t.Fatal("a later word-bounded occurrence must match")
	}
	if traceSpanContainsWordBoundedLiteral("verifyclassloader", "verifyclass") {
		t.Fatal("an occurrence followed by a letter must not match")
	}
	if traceSpanContainsWordBoundedLiteral("averifyclass", "verifyclass") {
		t.Fatal("an occurrence preceded by a letter must not match")
	}
	if !traceSpanContainsWordBoundedLiteral("verifyclass", "verifyclass") {
		t.Fatal("the whole name is bounded")
	}
}

// HMC377 round 9 (R9-3, register R8C-3): the CV-1 word boundary and the
// platform vocabulary's operation boundary stay two rules. The vocabulary
// matches an operation head at the start of the name only, and lets the
// bytes after it continue the name when they are a letter, or "_" / "::"
// followed by a letter; a digit there is a glued parameter stamp
// ("PartialGC::RunPhases0;GCReason2"). A class-verification literal can occur
// anywhere in a name, so it needs a rule on both sides, and it follows its
// built-in sibling arms (texture upload, the GC prefix boundary): an ASCII
// letter or digit on either side continues the word, anything else bounds it.
// On the 75 baseline captures (1,304,813 census names, fix9/r3):
//   - the vocabulary's rule on both sides (mirrored before the literal)
//     would lose the pinned positive "art::ClassVerifier::VerifyClass" (a
//     "::" qualifier before the literal is a namespace, not a continuation).
//     That is a synthetic-pin reason, not a capture measurement: no raw line
//     of the 75 captures contains that spelling and 0 of the 1,304,813
//     census names flip under the mirrored rule (round 10 correction,
//     fix9/review-cv1/oracle/raw_flip_spellings.txt, census/head.log);
//   - the CV-1 rule in the vocabulary fails its real ArkTS heads with a
//     glued numeric stamp ("H:PartialGC::RunPhases0;GCReason2;…");
//   - the vocabulary's rule after the literal and this rule before it keeps
//     the census set (2,871 names) and every verdict here, but splices two
//     rules into one predicate, moves the class-verification arm off the
//     boundary of its sibling arms, and flips four spellings no capture
//     contains (below).
//
// So the current rule stays, and the reviewer's boundary probe
// (fix8/review-cv1/oracle/boundary_probe.out) is pinned as its explicit
// expectations.
func TestHMC377R9ClassVerificationBoundaryProbeExpectations(t *testing.T) {
	for _, c := range []struct {
		name string
		cv   bool
	}{
		{"VerifyClassLoaderContextMatch", false},
		{"verify_class_loader_context_match", true}, // the "verify_class" literal arm (the token arm agrees)
		{"Verify_Class_Loader_Context_Match", true}, // the "verify_class" literal arm (the token arm agrees)
		{"verify class loader context match", true}, // the verify + class token arm
		// '_', '::' and '/' after the literal bound it (the vocabulary's
		// rule would continue the name after "_" / "::" + letter).
		{"VerifyClass_LoaderContextMatch", true},
		{"VerifyClass::LoaderContextMatch", true},
		{"VerifyClass/LoaderContextMatch", true},
		{"VerifyClass;", true},
		{"Lcom/example/VerifyClass;", true},
		{"VerifyClass(Foo)", true},
		// A non-ASCII byte bounds the literal on either side.
		{"VerifyClass中文", true},
		{"中VerifyClass", true},
		{"VerifyClassé", true},
		{"éVerifyClass", true},
		{"VERIFYCLASS FOO", true},
		// Go lowercases U+0130 to 'i', so the "verifyclass" literal arm decides
		// (fix9/review-cv1/mutant/probe_arms.log: token_arm=false).
		{"VerİfyClass foo", true},
		// A digit on either side continues the word (the vocabulary's rule
		// would end the name before a digit stamp).
		{"VerifyClass2", false},
		{"2VerifyClass", false},
		{"classverification2", false},
		{"VerifyClasses", false},
		{"ReverifyClass", false},
		{"art::verifier::ClassVerifier::VerifyClass", true},
		{"H:VerifyClass foo", true},
		{"H:[a,0,0]#VerifyClass foo", true},
		{"verifyclass verifyclassx", true},
		{"xverifyclass verifyclass", true},
		{"verifyclas", false},
		{"", false},
		{" ", false},
		{"VerifyClass\tfoo", true},
		{"ClassVerifierCache", false},
		{"class_verifier_cache", true}, // the verify + class token arm
		{"VerifyClassUsingOatFile", false},
		{"verifyClassUsingOatFile", false},
	} {
		lower := strings.ToLower(strings.TrimSpace(c.name))
		predicate := lower != "" && traceSpanLooksLikeClassVerification(lower, traceSpanNameTokenSet(lower))
		class := traceSpanSemanticClass(c.name)
		if predicate != c.cv || (class == "class_verification") != c.cv {
			t.Errorf("%q: predicate %t class %q, want class verification %t", c.name, predicate, class, c.cv)
		}
	}
	for _, c := range []struct {
		lower, literal string
		bounded        bool
	}{
		{"", "x", false}, {"x", "", false}, {"ab", "abc", false}, {"abc", "abc", true}, {"aabc", "abc", false},
		{"abca", "abc", false}, {"abcabc", "abc", false}, {"abc-abc", "abc", true}, {"aaaa", "aa", false}, {"a-aa", "aa", true},
	} {
		if got := traceSpanContainsWordBoundedLiteral(c.lower, c.literal); got != c.bounded {
			t.Errorf("bounded(%q, %q) = %t, want %t", c.lower, c.literal, got, c.bounded)
		}
	}
}
