package tracequery

import "testing"

// HMC377 CV-1: the class-verification literals are word-bounded. ART's
// "VerifyClassLoaderContextMatch" checks that the compiled code's recorded
// class-loader context matches while that code is opened; it is not class
// verification and must not join the class-verification family (it did in
// every one of the 75 baseline captures, 1-7 markers each, up to 0.24 ms;
// Douyin reboot5 TID 11105 counted 127 / 92.266 ms instead of 126 / 92.026).
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
