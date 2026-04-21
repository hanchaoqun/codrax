package logtriage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestStripBuildPathPrefix_BuiltinPrefixes pins the built-in prefix
// cascade against the 8 canonical CI / distro / home roots. Each
// entry strips to the suffix a user-repo could match.
func TestStripBuildPathPrefix_BuiltinPrefixes(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/build/src/internal/foo.go", "internal/foo.go"},
		{"/build/source/pkg/foo.go", "pkg/foo.go"},
		{"/builddir/build/BUILD/pkg/foo.c", "pkg/foo.c"},
		{"/rpmbuild/BUILD/pkg-1.0/src/foo.c", "pkg-1.0/src/foo.c"},
		{"/workspace/source/foo.java", "foo.java"},
		{"/workspace/foo.py", "foo.py"},
		{"/tmp/build/foo.go", "foo.go"},
	}
	for _, c := range cases {
		got := StripBuildPathPrefix(c.in, "", nil)
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStripBuildPathPrefix_HomeScan covers the /home/*/src/ family.
func TestStripBuildPathPrefix_HomeScan(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/home/user/work/src/foo.go", "foo.go"},
		{"/home/user/repo/source/pkg/bar.py", "pkg/bar.py"},
		{"/home/user/workspace/baz.java", "baz.java"},
		{"/home/user/project/qux.c", "qux.c"},
	}
	for _, c := range cases {
		got := StripBuildPathPrefix(c.in, "", nil)
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStripBuildPathPrefix_ExtraPrefixWins confirms that extraPrefixes
// takes priority over built-ins.
func TestStripBuildPathPrefix_ExtraPrefixWins(t *testing.T) {
	in := "/ci/our-custom/src/foo.go"
	got := StripBuildPathPrefix(in, "", []string{"/ci/our-custom/src"})
	if got != "foo.go" {
		t.Errorf("extraPrefix lost: %q → %q, want foo.go", in, got)
	}
}

// TestStripBuildPathPrefix_RepoBasename verifies the repoBaseName pass
// fires when the basename appears as a path component.
func TestStripBuildPathPrefix_RepoBasename(t *testing.T) {
	in := "/unknown/root/myrepo/internal/foo.go"
	got := StripBuildPathPrefix(in, "myrepo", nil)
	if got != "internal/foo.go" {
		t.Errorf("repoBase fail: %q → %q", in, got)
	}
}

// TestStripBuildPathPrefix_NoMatchReturnsInput covers the passthrough
// case — unknown prefix, function hands back the input unchanged.
func TestStripBuildPathPrefix_NoMatchReturnsInput(t *testing.T) {
	in := "/elsewhere/foo.go"
	got := StripBuildPathPrefix(in, "", nil)
	if got != in {
		t.Errorf("unexpected rewrite: %q → %q", in, got)
	}
}

// TestStripBuildPathPrefix_SourcePrefixGlobalOverride verifies the
// package-level override is consulted when extraPrefixes is empty.
func TestStripBuildPathPrefix_SourcePrefixGlobalOverride(t *testing.T) {
	SetSourcePrefix("/my/build/")
	defer SetSourcePrefix("")
	got := StripBuildPathPrefix("/my/build/foo.go", "", nil)
	if got != "foo.go" {
		t.Errorf("global override not consulted: got %q", got)
	}
}

// TestResolveJavaFile_PackagePathTierOne pins the tier-1 match: exact
// package-path suffix wins over shorter candidates.
func TestResolveJavaFile_PackagePathTierOne(t *testing.T) {
	cands := []string{
		"src/main/java/com/bar/Util.java",
		"src/main/java/com/foo/Util.java", // matches com.foo.*
	}
	got := ResolveJavaFile("com.foo", "Util.java", cands)
	if len(got) == 0 || got[0] != "src/main/java/com/foo/Util.java" {
		t.Errorf("tier1 fail: %v", got)
	}
}

// TestResolveJavaFile_SourceRootTierTwo pins tier-2 fallback: when the
// dir does not suffix-match pkgPath but sits under src/main/java/
// plus pkgPath tail.
func TestResolveJavaFile_SourceRootTierTwo(t *testing.T) {
	cands := []string{"src/main/java/com/foo/Util.java"}
	got := ResolveJavaFile("com.foo", "Util.java", cands)
	if len(got) == 0 {
		t.Fatal("no hit")
	}
	if got[0] != "src/main/java/com/foo/Util.java" {
		t.Errorf("wrong hit: %q", got[0])
	}
}

// TestResolveJavaFile_BaselineOnly covers the fallthrough when no
// package match is possible — still returns the basename hits in
// path-length order.
func TestResolveJavaFile_BaselineOnly(t *testing.T) {
	cands := []string{
		"vendor/com/other/Deeply/Nested/Util.java",
		"src/Util.java",
	}
	got := ResolveJavaFile("", "Util.java", cands)
	if len(got) != 2 {
		t.Fatalf("want 2 hits, got %d", len(got))
	}
	// Shorter path first.
	if got[0] != "src/Util.java" {
		t.Errorf("ordering: first = %q", got[0])
	}
}

// TestIsRuntimeInternalFile exercises the stdlib / runtime filter.
func TestIsRuntimeInternalFile(t *testing.T) {
	in := map[string]bool{
		"":                                    true,
		"runtime/proc.go":                     true,
		"/usr/local/go/src/runtime/proc.go":   true,
		"asm_amd64.s":                         true,
		"node:internal/modules/cjs/loader.js": true,
		"java.base/java/lang/Thread.java":     true,
		"internal/agent/foo.go":               false,
		"com/example/Service.java":            false,
		"main.py":                             false,
	}
	for path, want := range in {
		if got := IsRuntimeInternalFile(path); got != want {
			t.Errorf("IsRuntimeInternalFile(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestResolveFrameFile_AbsolutePathUnderRepo covers absolute-path
// happy path: path lives under repoRoot, Rel returns clean suffix.
func TestResolveFrameFile_AbsolutePathUnderRepo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pkg", "foo.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveFrameFile(dir, target)
	if got != "pkg/foo.go" {
		t.Errorf("got %q, want pkg/foo.go", got)
	}
}

// TestResolveFrameFile_EscapeRejected covers the "../" escape attempt.
func TestResolveFrameFile_EscapeRejected(t *testing.T) {
	dir := t.TempDir()
	outside := "/etc/passwd"
	got := ResolveFrameFile(dir, outside)
	if got != "" {
		t.Errorf("escape not rejected: got %q", got)
	}
}

// TestResolveFrameFile_RelativeResolves covers the repo-relative
// input path that lives inside the repo.
func TestResolveFrameFile_RelativeResolves(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bar.go"),
		[]byte("package bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveFrameFile(dir, "bar.go")
	if got != "bar.go" {
		t.Errorf("got %q, want bar.go", got)
	}
}

// TestResolveFrameFile_NonExistentDropped confirms os.Stat gate:
// a path that doesn't exist returns "".
func TestResolveFrameFile_NonExistentDropped(t *testing.T) {
	dir := t.TempDir()
	got := ResolveFrameFile(dir, "nonexistent.go")
	if got != "" {
		t.Errorf("got %q, want empty (stat fail)", got)
	}
}

// TestGlobByBasename_SkipsVendorAndHidden verifies the walk skips
// .git / node_modules / vendor and hidden directories.
func TestGlobByBasename_SkipsVendorAndHidden(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "src", "Util.java"),
		filepath.Join(dir, "vendor", "Util.java"),       // skipped
		filepath.Join(dir, "node_modules", "Util.java"), // skipped
		filepath.Join(dir, ".hidden", "Util.java"),      // skipped
	}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("//"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := GlobByBasename(dir, "Util.java")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/Util.java"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (vendor/node_modules/.hidden must be skipped)",
			got, want)
	}
}
