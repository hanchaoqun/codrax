package logtriage

import (
	"reflect"
	"testing"
)

// TestResolveArkTSFile_StageModelTierOne pins tier-1 match: Stage
// Model layout `entry/src/main/ets/...` is the canonical ArkTS
// app layout — a file there outranks identically-named files
// elsewhere in the repo.
func TestResolveArkTSFile_StageModelTierOne(t *testing.T) {
	cands := []string{
		"entry/src/main/ets/pages/Index.ets",
		"commons/src/main/ets/pages/Index.ets",
		"scratch/Index.ets",
	}
	got := ResolveArkTSFile("Index.ets", cands)
	want := []string{
		"entry/src/main/ets/pages/Index.ets",
		"commons/src/main/ets/pages/Index.ets",
		"scratch/Index.ets",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranked list drift\nwant %v\ngot  %v", want, got)
	}
}

// TestResolveArkTSFile_BasenameBaseline verifies that a file with
// no Stage Model / commons signal still resolves via basename.
func TestResolveArkTSFile_BasenameBaseline(t *testing.T) {
	cands := []string{"tools/Index.ets", "work/Index.ets"}
	got := ResolveArkTSFile("Index.ets", cands)
	if len(got) != 2 {
		t.Errorf("want 2 resolved (tier 3 baseline); got %v", got)
	}
}

// TestResolveCangjieFile_PackagePathTierOne pins tier-1 match for
// a frame with a package hint — dir must end with the package
// path translated to slashes.
func TestResolveCangjieFile_PackagePathTierOne(t *testing.T) {
	cands := []string{
		"src/cart/Cart.cj",
		"thirdparty/tmp/Cart.cj",
	}
	got := ResolveCangjieFile("demo.cart", "Cart.cj", cands)
	want := []string{
		"src/cart/Cart.cj",
		"thirdparty/tmp/Cart.cj",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranked list drift\nwant %v\ngot  %v", want, got)
	}
}

// TestResolveCangjieFile_SourceRootTierTwo pins tier-2: file under
// a canonical source root but the dir does not match the full pkg.
func TestResolveCangjieFile_SourceRootTierTwo(t *testing.T) {
	cands := []string{
		"src/cangjie/demo/cart/Cart.cj",
		"scratch/Cart.cj",
	}
	got := ResolveCangjieFile("demo.cart", "Cart.cj", cands)
	if got[0] != "src/cangjie/demo/cart/Cart.cj" {
		t.Errorf("tier-2 expectation failed; got first %q full %v", got[0], got)
	}
}

// TestResolveKotlinFile_AndroidGradleLayout pins tier-1 / tier-2
// tiers for an Android Gradle layout.
func TestResolveKotlinFile_AndroidGradleLayout(t *testing.T) {
	cands := []string{
		"app/src/main/kotlin/com/example/ui/MainActivity.kt",
		"app/src/main/java/com/example/ui/MainActivity.kt",
		"scratch/MainActivity.kt",
	}
	got := ResolveKotlinFile("com.example.ui", "MainActivity.kt", cands)
	// Both src/main/kotlin AND src/main/java ending with the
	// package path yield tier 2 → both precede tier-3 scratch.
	if got[len(got)-1] != "scratch/MainActivity.kt" {
		t.Errorf("scratch should rank last; got order %v", got)
	}
}

// TestIsKotlinBasename covers .kt and .kts.
func TestIsKotlinBasename(t *testing.T) {
	cases := map[string]bool{
		"MainActivity.kt":  true,
		"build.gradle.kts": true,
		"MainActivity.cj":  false,
		"path/to/file.kt":  false, // basename check rejects dir sep
		"":                 false,
	}
	for path, want := range cases {
		if got := isKotlinBasename(path); got != want {
			t.Errorf("isKotlinBasename(%q): want %v, got %v", path, want, got)
		}
	}
}

// TestIsArkTSBasename covers .ets only (not .ts, which is language-
// ambiguous at the log_triage layer).
func TestIsArkTSBasename(t *testing.T) {
	cases := map[string]bool{
		"Index.ets":      true,
		"EntryAbility.ts": false, // .ts stays JavaScript/TypeScript at log_triage
		"Cart.cj":        false,
		"pages/Index.ets": false, // dir sep rejects
	}
	for path, want := range cases {
		if got := isArkTSBasename(path); got != want {
			t.Errorf("isArkTSBasename(%q): want %v, got %v", path, want, got)
		}
	}
}

// TestIsCangjieBasename covers .cj only (not .cjo).
func TestIsCangjieBasename(t *testing.T) {
	cases := map[string]bool{
		"Cart.cj":  true,
		"Cart.cjo": false,
		"":         false,
	}
	for path, want := range cases {
		if got := isCangjieBasename(path); got != want {
			t.Errorf("isCangjieBasename(%q): want %v, got %v", path, want, got)
		}
	}
}
