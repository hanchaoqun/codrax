package logparse

import (
	"strings"
	"testing"
)

// ── Go panic ──────────────────────────────────────────────────────

func TestParseGo_CanonicalPanic(t *testing.T) {
	raw := `panic: runtime error: index out of range [5] with length 3

goroutine 1 [running]:
main.crashyThing(0x0, 0x0)
	/src/repo/cmd/app/main.go:42 +0x1e
main.main()
	/src/repo/cmd/app/main.go:10 +0x2a
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:1571 +0x1
`
	b := Detect(raw)
	if b.Lang != "go" {
		t.Fatalf("Lang=%q want go; bundle=%+v", b.Lang, b)
	}
	if len(b.Frames) != 3 {
		t.Fatalf("frames=%d want 3: %+v", len(b.Frames), b.Frames)
	}
	if b.Frames[0].File != "/src/repo/cmd/app/main.go" || b.Frames[0].Line != 42 {
		t.Errorf("frame[0] file:line = %q:%d, want /src/repo/cmd/app/main.go:42",
			b.Frames[0].File, b.Frames[0].Line)
	}
	if b.Frames[0].Func != "main.crashyThing" {
		t.Errorf("frame[0].Func = %q, want main.crashyThing", b.Frames[0].Func)
	}
	if !b.HasStack {
		t.Error("HasStack=false; want true for a canonical panic")
	}
	// panic message should appear in literals.
	found := false
	for _, lit := range b.ErrorLiterals {
		if strings.Contains(lit, "index out of range") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ErrorLiterals missing panic message: %v", b.ErrorLiterals)
	}
}

func TestParseGo_NoGoroutineHeader_Empty(t *testing.T) {
	raw := "just some prose without a goroutine header"
	if got := parseGo(raw); got.Lang != "" || len(got.Frames) != 0 {
		t.Errorf("parseGo returned non-empty for plain prose: %+v", got)
	}
}

// ── Java stacktrace ───────────────────────────────────────────────

func TestParseJava_WithCausedBy(t *testing.T) {
	raw := `Exception in thread "main" java.lang.NullPointerException: Cannot invoke "Object.toString()" because "x" is null
	at com.example.Service.run(Service.java:42)
	at com.example.App.main(App.java:10)
Caused by: java.lang.IllegalStateException: bad
	at com.example.Inner.boom(Inner.java:5)
`
	b := Detect(raw)
	if b.Lang != "java" {
		t.Fatalf("Lang=%q want java", b.Lang)
	}
	if len(b.Frames) != 3 {
		t.Fatalf("frames=%d want 3: %+v", len(b.Frames), b.Frames)
	}
	// First frame
	if b.Frames[0].File != "Service.java" || b.Frames[0].Line != 42 {
		t.Errorf("frame[0] = %+v, want Service.java:42", b.Frames[0])
	}
	if b.Frames[0].Func != "com.example.Service.run" {
		t.Errorf("frame[0].Func = %q, want com.example.Service.run", b.Frames[0].Func)
	}
	// Pkg is the Java package (everything before the last dot in the
	// fully-qualified class name); "Service" is the class, not part
	// of the package.
	if b.Frames[0].Pkg != "com.example" {
		t.Errorf("frame[0].Pkg = %q, want com.example", b.Frames[0].Pkg)
	}
	// Caused-by frame
	if b.Frames[2].File != "Inner.java" || b.Frames[2].Line != 5 {
		t.Errorf("frame[2] (caused-by) = %+v, want Inner.java:5", b.Frames[2])
	}
	// Literals should contain both exception types.
	foundNPE := false
	foundISE := false
	for _, lit := range b.ErrorLiterals {
		if strings.Contains(lit, "NullPointerException") {
			foundNPE = true
		}
		if strings.Contains(lit, "IllegalStateException") {
			foundISE = true
		}
	}
	if !foundNPE || !foundISE {
		t.Errorf("ErrorLiterals missing exception types: %v", b.ErrorLiterals)
	}
}

func TestParseJava_NativeMethod(t *testing.T) {
	raw := `java.lang.RuntimeException: x
	at java.base/java.lang.Thread.start0(Native Method)
	at com.example.App.main(App.java:10)
`
	b := Detect(raw)
	if b.Lang != "java" {
		t.Fatalf("Lang=%q want java", b.Lang)
	}
	if len(b.Frames) != 2 {
		t.Fatalf("frames=%d want 2", len(b.Frames))
	}
	// Native method: Line==0 and File empty
	if b.Frames[0].Line != 0 || b.Frames[0].File != "" {
		t.Errorf("Native Method frame should have empty file and Line=0, got %+v", b.Frames[0])
	}
}

// ── C/C++ ASAN ────────────────────────────────────────────────────

func TestParseCppASAN(t *testing.T) {
	raw := `==12345==ERROR: AddressSanitizer: heap-use-after-free on address 0x602000000010
READ of size 4 at 0x602000000010 thread T0
    #0 0x401234 in Foo::bar() /src/repo/foo.cpp:42:5
    #1 0x401567 in main /src/repo/main.cpp:10:3
SUMMARY: AddressSanitizer: heap-use-after-free /src/repo/foo.cpp:42:5 in Foo::bar()
`
	b := Detect(raw)
	if b.Lang != "cpp" {
		t.Fatalf("Lang=%q want cpp", b.Lang)
	}
	if len(b.Frames) != 2 {
		t.Fatalf("frames=%d want 2: %+v", len(b.Frames), b.Frames)
	}
	if b.Frames[0].File != "/src/repo/foo.cpp" || b.Frames[0].Line != 42 {
		t.Errorf("frame[0] = %+v, want /src/repo/foo.cpp:42", b.Frames[0])
	}
	if b.Frames[0].Func != "Foo::bar()" {
		t.Errorf("frame[0].Func = %q, want Foo::bar()", b.Frames[0].Func)
	}
	foundErr := false
	for _, lit := range b.ErrorLiterals {
		if strings.Contains(lit, "heap-use-after-free") {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("ErrorLiterals missing heap-use-after-free: %v", b.ErrorLiterals)
	}
}

// ── C/C++ gdb ─────────────────────────────────────────────────────

func TestParseCppGDB(t *testing.T) {
	raw := `Program received signal SIGSEGV, Segmentation fault.
#0  0x00007ffff7b2a1a0 in Foo::bar (this=0x12345678, x=5) at /src/repo/foo.cpp:42
#1  0x00007ffff7b2a2b0 in main () at /src/repo/main.cpp:10
`
	b := Detect(raw)
	if b.Lang != "cpp" {
		t.Fatalf("Lang=%q want cpp; bundle=%+v", b.Lang, b)
	}
	if len(b.Frames) != 2 {
		t.Fatalf("frames=%d want 2: %+v", len(b.Frames), b.Frames)
	}
	if b.Frames[0].File != "/src/repo/foo.cpp" || b.Frames[0].Line != 42 {
		t.Errorf("frame[0] = %+v, want /src/repo/foo.cpp:42", b.Frames[0])
	}
}

// ── Python ────────────────────────────────────────────────────────

func TestParsePython(t *testing.T) {
	raw := `Traceback (most recent call last):
  File "/src/repo/app.py", line 42, in handle
    result = compute(x)
  File "/src/repo/util.py", line 10, in compute
    raise ValueError("bad input")
ValueError: bad input
`
	b := Detect(raw)
	if b.Lang != "python" {
		t.Fatalf("Lang=%q want python", b.Lang)
	}
	if len(b.Frames) != 2 {
		t.Fatalf("frames=%d want 2: %+v", len(b.Frames), b.Frames)
	}
	if b.Frames[0].File != "/src/repo/app.py" || b.Frames[0].Line != 42 || b.Frames[0].Func != "handle" {
		t.Errorf("frame[0] = %+v, want /src/repo/app.py:42 in handle", b.Frames[0])
	}
	foundVE := false
	for _, lit := range b.ErrorLiterals {
		if lit == "ValueError" {
			foundVE = true
			break
		}
	}
	if !foundVE {
		t.Errorf("ErrorLiterals missing ValueError: %v", b.ErrorLiterals)
	}
}

func TestParsePython_NoHeader_Rejected(t *testing.T) {
	raw := `ValueError: bad input`
	b := Detect(raw)
	if b.Lang == "python" {
		t.Errorf("bare exception should not parse as python: %+v", b)
	}
}

// ── Node.js ───────────────────────────────────────────────────────

func TestParseNode(t *testing.T) {
	raw := `TypeError: Cannot read property 'x' of undefined
    at Object.handler (/src/repo/app.js:42:13)
    at async main (/src/repo/main.js:10:5)
    at Module._compile (node:internal/modules/cjs/loader:1105:14)
`
	b := Detect(raw)
	if b.Lang != "node" {
		t.Fatalf("Lang=%q want node; bundle=%+v", b.Lang, b)
	}
	if len(b.Frames) != 3 {
		t.Fatalf("frames=%d want 3: %+v", len(b.Frames), b.Frames)
	}
	if b.Frames[0].File != "/src/repo/app.js" || b.Frames[0].Line != 42 {
		t.Errorf("frame[0] = %+v, want /src/repo/app.js:42", b.Frames[0])
	}
	if b.Frames[0].Func != "Object.handler" {
		t.Errorf("frame[0].Func = %q, want Object.handler", b.Frames[0].Func)
	}
}

func TestParseNode_AnonymousFrame(t *testing.T) {
	raw := `Error: boom
    at /src/repo/cli.js:3:1
`
	b := Detect(raw)
	if b.Lang != "node" {
		t.Fatalf("Lang=%q want node", b.Lang)
	}
	if len(b.Frames) != 1 {
		t.Fatalf("frames=%d want 1", len(b.Frames))
	}
	if b.Frames[0].File != "/src/repo/cli.js" || b.Frames[0].Func != "" {
		t.Errorf("anon frame = %+v, want /src/repo/cli.js:3 with empty Func", b.Frames[0])
	}
}

// ── Detect priority / empty cases ─────────────────────────────────

func TestDetect_EmptyInput(t *testing.T) {
	if got := Detect(""); got.Lang != "" {
		t.Errorf("Detect(\"\") returned non-empty: %+v", got)
	}
	if got := Detect("   \n\n  "); got.Lang != "" {
		t.Errorf("Detect(whitespace) returned non-empty: %+v", got)
	}
}

func TestDetect_PlainProse_NoMatch(t *testing.T) {
	raw := "Please investigate why the login endpoint is returning 500 errors after deployment."
	b := Detect(raw)
	if b.Lang != "" {
		t.Errorf("Detect(prose) returned %+v, want empty", b)
	}
	if b.HasStack {
		t.Error("HasStack=true for plain prose")
	}
}

// ── StripBuildPathPrefix ──────────────────────────────────────────

func TestStripBuildPathPrefix(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		repo     string
		extra    []string
		expected string
	}{
		{"build-src strip", "/build/src/foo.cpp", "", nil, "foo.cpp"},
		{"rpmbuild strip", "/rpmbuild/BUILD/pkg-1.0/src/foo.cpp", "pkg-1.0", nil, "src/foo.cpp"},
		{"home-user-src strip", "/home/builder/src/proj/main.c", "", nil, "proj/main.c"},
		{"repo-basename embedded", "/home/alice/work/codrax/internal/foo.go", "codrax", nil, "internal/foo.go"},
		{"extra-prefix wins", "/custom/root/app/src/bar.c", "", []string{"/custom/root/app/src/"}, "bar.c"},
		{"no match passes through", "relative/path/foo.c", "", nil, "relative/path/foo.c"},
		{"empty returns empty", "", "", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StripBuildPathPrefix(c.path, c.repo, c.extra)
			if got != c.expected {
				t.Errorf("StripBuildPathPrefix(%q, %q, %v) = %q, want %q",
					c.path, c.repo, c.extra, got, c.expected)
			}
		})
	}
}

// ── ResolveJavaFile ───────────────────────────────────────────────

func TestResolveJavaFile_PackageSuffix(t *testing.T) {
	candidates := []string{
		"src/main/java/com/example/Service.java",
		"src/test/java/com/example/Service.java",
		"other/module/com/example/Service.java",
		"unrelated/Service.java",
	}
	got := ResolveJavaFile("com.example", "Service.java", candidates)
	if len(got) == 0 {
		t.Fatal("ResolveJavaFile returned empty list")
	}
	// Tier 1 (package suffix) should outrank tier 3 (basename only).
	// All three matching-package paths should come before the
	// unrelated/Service.java, which is tier 3.
	last := got[len(got)-1]
	if last != "unrelated/Service.java" {
		t.Errorf("last candidate = %q, want unrelated/Service.java (weakest match); got list = %v", last, got)
	}
}

func TestResolveJavaFile_MultiModuleAmbiguous(t *testing.T) {
	// Same package in two Maven modules — return both, caller handles.
	candidates := []string{
		"moduleA/src/main/java/com/foo/Util.java",
		"moduleB/src/main/java/com/foo/Util.java",
	}
	got := ResolveJavaFile("com.foo", "Util.java", candidates)
	if len(got) != 2 {
		t.Errorf("expected 2 candidates, got %d: %v", len(got), got)
	}
}

func TestResolveJavaFile_EmptyInputs(t *testing.T) {
	if got := ResolveJavaFile("", "", nil); got != nil {
		t.Errorf("empty inputs should return nil, got %v", got)
	}
	if got := ResolveJavaFile("com.foo", "", []string{"a.java"}); got != nil {
		t.Errorf("empty baseName should return nil, got %v", got)
	}
}

// ── IsRuntimeInternalFile ─────────────────────────────────────────

func TestIsRuntimeInternalFile(t *testing.T) {
	cases := map[string]bool{
		"":                                         true,
		"/usr/local/go/src/runtime/asm_amd64.s":    true,
		"runtime/asm_amd64.s":                      true,
		"node:internal/modules/cjs/loader":         true,
		"java.base/java/lang/Thread.java":          true,
		"/src/repo/app.go":                         false,
		"internal/agent/analyzer.go":               false,
	}
	for path, want := range cases {
		if got := IsRuntimeInternalFile(path); got != want {
			t.Errorf("IsRuntimeInternalFile(%q) = %v, want %v", path, got, want)
		}
	}
}
