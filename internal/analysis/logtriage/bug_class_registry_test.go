package logtriage

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDetectBugClasses_PerClass walks every BugClass and verifies
// at least one cross-language fixture fires it. Failure here means
// the registry has a class with NO patterns — equivalent to dead
// code.
func TestDetectBugClasses_PerClass(t *testing.T) {
	fixtures := map[types.BugClass]string{
		types.BugClassRace:                "fatal error: concurrent map writes",
		types.BugClassDeadlock:            "fatal error: all goroutines are asleep - deadlock!",
		types.BugClassNilDeref:            "panic: runtime error: invalid memory address or nil pointer dereference",
		types.BugClassBounds:              "panic: runtime error: index out of range [3] with length 3",
		types.BugClassTypeAssertion:       "panic: interface conversion: int is not string",
		types.BugClassStackOverflow:       "java.lang.StackOverflowError\n at Foo.recurse(Foo.java:5)",
		types.BugClassDivByZero:           "panic: runtime error: integer divide by zero",
		types.BugClassResourceExhaustion:  "open /tmp/foo: too many open files",
		types.BugClassUnhandledAsync:      "(node:1234) UnhandledPromiseRejectionWarning: Error: bang",
		types.BugClassSerialization:       "json: cannot unmarshal: unexpected end of JSON input",
		types.BugClassIntegrity:           "fatal: bad object 1234abcd",
		types.BugClassAuth:                `{"error":"invalid_grant","error_description":"Bad credentials"}`,
		types.BugClassTLSCert:             "x509: certificate signed by unknown authority",
		types.BugClassUseAfterFree:        "==12345==ERROR: AddressSanitizer: heap-use-after-free at 0x6020",
		types.BugClassBufferOverflow:      "==12345==ERROR: AddressSanitizer: heap-buffer-overflow at 0x6020",
		types.BugClassUncaughtException:   "Traceback (most recent call last):\n  File foo.py line 3\nValueError: bad",
		types.BugClassFileSystem:          "open /etc/secret: permission denied",
		types.BugClassEncoding:            "UnicodeDecodeError: 'utf-8' codec can't decode byte 0x80",
		types.BugClassConfig:              "yaml: line 5: cannot unmarshal !!str into int",
	}
	for class, fixture := range fixtures {
		got := DetectBugClasses(fixture)
		found := false
		for _, d := range got {
			if d.Class == class {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("class %q: fixture %q produced %v, expected to detect %q", class, fixture, classNames(got), class)
		}
	}
}

// TestDetectBugClasses_MultiLanguageCoverage: each cross-language
// pattern subset for a class fires the same Class + a meaningful
// HumanLabel.
func TestDetectBugClasses_MultiLanguageCoverage(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		class   types.BugClass
	}{
		// Race — 6 langs/runtimes
		{"race-go-runtime", "fatal error: concurrent map writes", types.BugClassRace},
		{"race-go-detector", "WARNING: DATA RACE: write at 0x00\n  goroutine 1", types.BugClassRace},
		{"race-tsan-cpp", "WARNING: ThreadSanitizer: data race", types.BugClassRace},
		{"race-helgrind", "==1234==  Possible data race during write of size 4", types.BugClassRace},
		{"race-jvm", "java.util.ConcurrentModificationException at HashMap", types.BugClassRace},
		{"race-generic", "log: race condition observed in main loop", types.BugClassRace},

		// NilDeref — 7 langs
		{"nil-go", "panic: runtime error: invalid memory address or nil pointer dereference", types.BugClassNilDeref},
		{"nil-java", "java.lang.NullPointerException: Cannot invoke \"X\"", types.BugClassNilDeref},
		{"nil-kotlin", "kotlin.KotlinNullPointerException", types.BugClassNilDeref},
		{"nil-swift", "Fatal error: Unexpectedly found nil while implicitly unwrapping", types.BugClassNilDeref},
		{"nil-rust", "thread 'main' panicked at: called `Option::unwrap()` on a `None` value", types.BugClassNilDeref},
		{"nil-python", "AttributeError: 'NoneType' object has no attribute 'foo'", types.BugClassNilDeref},
		{"nil-ruby", "NoMethodError: undefined method `foo' for nil:NilClass", types.BugClassNilDeref},
		{"nil-js", "TypeError: Cannot read properties of undefined (reading 'foo')", types.BugClassNilDeref},

		// Stack overflow — 5 runtimes
		{"sof-go", "runtime: goroutine stack exceeds 1000000000-byte limit", types.BugClassStackOverflow},
		{"sof-jvm", "Exception in thread \"main\" java.lang.StackOverflowError", types.BugClassStackOverflow},
		{"sof-rust", "thread 'main' has overflowed its stack", types.BugClassStackOverflow},
		{"sof-python", "RecursionError: maximum recursion depth exceeded", types.BugClassStackOverflow},
		{"sof-node", "RangeError: Maximum call stack size exceeded", types.BugClassStackOverflow},

		// Memory unsafety — ASan / MSan / Valgrind
		{"uaf-asan", "==12345==ERROR: AddressSanitizer: heap-use-after-free", types.BugClassUseAfterFree},
		{"oob-asan", "==12345==ERROR: AddressSanitizer: heap-buffer-overflow", types.BugClassBufferOverflow},
		{"uaf-glibc", "free(): invalid pointer", types.BugClassUseAfterFree},
		{"oob-glibc", "*** stack smashing detected ***", types.BugClassBufferOverflow},

		// HarmonyOS-specific (ArkTS / Cangjie)
		{"arkts-runtime", "Error: Cannot find variable: foo at Index.ets:42", types.BugClassUncaughtException},
		{"cangjie-runtime", "Exception in thread \"main\" at app/Main.cj:18", types.BugClassUncaughtException},

		// Lua + Ruby + Swift uncaught
		{"lua-error", "lua: foo.lua:10: attempt to index nil value", types.BugClassUncaughtException},

		// Encoding
		{"enc-py", "UnicodeDecodeError: 'utf-8' codec can't decode byte 0x80", types.BugClassEncoding},
		{"enc-go", "invalid UTF-8 sequence", types.BugClassEncoding},
		{"enc-rust", "thread 'main' panicked at: Utf8Error", types.BugClassEncoding},

		// Filesystem (cross-platform errno)
		{"fs-errno", "system call failed: ENOENT", types.BugClassFileSystem},
		{"fs-go", "open /etc/foo: no such file or directory", types.BugClassFileSystem},
		{"fs-py", "FileNotFoundError: [Errno 2] No such file", types.BugClassFileSystem},

		// TLS
		{"tls-go", "x509: certificate signed by unknown authority", types.BugClassTLSCert},
		{"tls-py", "SSL: CERTIFICATE_VERIFY_FAILED", types.BugClassTLSCert},
		{"tls-jvm", "javax.net.ssl.SSLHandshakeException: PKIX path building failed", types.BugClassTLSCert},

		// Config
		{"config-env", "ENV variable DATABASE_URL is required", types.BugClassConfig},
		{"config-yaml", "yaml: line 3: cannot unmarshal !!str into int", types.BugClassConfig},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetectBugClasses(c.fixture)
			found := false
			for _, d := range got {
				if d.Class == c.class {
					if d.HumanZh == "" && d.HumanEn == "" {
						t.Errorf("missing HumanLabel for %q", c.class)
					}
					if d.MatchedSignature == "" {
						t.Errorf("MatchedSignature empty")
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected class %q on fixture, got %v", c.class, classNames(got))
			}
		})
	}
}

// TestDetectBugClasses_Dedup: same class fires only once even when
// multiple language patterns match.
func TestDetectBugClasses_Dedup(t *testing.T) {
	// fixture matches both go-runtime AND go-detector race patterns
	got := DetectBugClasses("fatal error: concurrent map writes\n... WARNING: DATA RACE")
	raceCount := 0
	for _, d := range got {
		if d.Class == types.BugClassRace {
			raceCount++
		}
	}
	if raceCount != 1 {
		t.Errorf("dedup failed: race fired %d times, want 1", raceCount)
	}
}

// TestDetectBugClasses_NoMatch: clean log returns empty list (no
// false positives on benign content).
func TestDetectBugClasses_NoMatch(t *testing.T) {
	got := DetectBugClasses("INFO: server started on port 8080\nDEBUG: handler registered")
	if len(got) != 0 {
		t.Errorf("clean log triggered detections: %v", classNames(got))
	}
}

// TestDetectBugClasses_Empty: empty input returns nil.
func TestDetectBugClasses_Empty(t *testing.T) {
	if got := DetectBugClasses(""); got != nil {
		t.Errorf("empty input returned %v, want nil", got)
	}
}

func classNames(detected []types.DetectedBugClass) []types.BugClass {
	out := make([]types.BugClass, 0, len(detected))
	for _, d := range detected {
		out = append(out, d.Class)
	}
	return out
}
