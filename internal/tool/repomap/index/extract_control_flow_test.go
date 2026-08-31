package index

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExtractControlFlowBranches_IfArmsAcrossExecutableTreeSitterLanguages(t *testing.T) {
	cases := []struct{ lang, src string }{
		{types.LangGo, "package p\nfunc f(x bool) { if x { run() } else { stop() } }"},
		{types.LangPython, "def f(x):\n  if x:\n    run()\n  else:\n    stop()\n"},
		{types.LangJavaScript, "function f(x) { if (x) { run(); } else { stop(); } }"},
		{types.LangTypeScript, "function f(x: boolean) { if (x) { run(); } else { stop(); } }"},
		{types.LangJava, "class A { void f(boolean x) { if (x) { run(); } else { stop(); } } }"},
		{types.LangKotlin, "fun f(x: Boolean) { if (x) { run() } else { stop() } }"},
		{types.LangRust, "fn f(x: bool) { if x { run(); } else { stop(); } }"},
		{types.LangC, "void f(int x) { if (x) { run(); } else { stop(); } }"},
		{types.LangCpp, "void f(bool x) { if (x) { run(); } else { stop(); } }"},
		{types.LangRuby, "def f(x)\n  if x\n    run()\n  else\n    stop()\n  end\nend\n"},
		{types.LangSwift, "func f(_ x: Bool) { if x { run() } else { stop() } }"},
		{types.LangLua, "function f(x)\n  if x then run() else stop() end\nend\n"},
		{types.LangArkTS, "function f(x: boolean) { if (x) { run(); } else { stop(); } }"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			root := parseSourceFor(t, tc.lang, tc.src)
			branches := extractControlFlowBranches(root, []byte(tc.src))
			assertControlFlowArmCall(t, branches, types.ControlFlowArmConsequence, "run")
			assertControlFlowArmCall(t, branches, types.ControlFlowArmAlternative, "stop")
		})
	}
}

func TestExtractControlFlowBranches_RustIfExpressionPreservesMatcherPolarity(t *testing.T) {
	src := `fn run(pattern: &str, fixed: bool) {
    let m = if fixed {
        Box::new(LiteralMatcher::new(pattern))
    } else {
        Box::new(RegexLikeMatcher::new(pattern))
    };
}`
	root := parseSourceFor(t, types.LangRust, src)
	branches := extractControlFlowBranches(root, []byte(src))
	assertControlFlowArmCall(t, branches, types.ControlFlowArmConsequence, "LiteralMatcher::new")
	assertControlFlowArmCall(t, branches, types.ControlFlowArmAlternative, "RegexLikeMatcher::new")
}

func TestExtractControlFlowBranches_CaseArmsAcrossBranchingLanguages(t *testing.T) {
	cases := []struct{ lang, src string }{
		{types.LangGo, "package p\nfunc f(x int) { switch x { case 1: one(); default: other() } }"},
		{types.LangPython, "def f(x):\n  match x:\n    case 1:\n      one()\n    case _:\n      other()\n"},
		{types.LangJavaScript, "function f(x) { switch (x) { case 1: one(); break; default: other(); } }"},
		{types.LangJava, "class A { void f(int x) { switch (x) { case 1: one(); break; default: other(); } } }"},
		{types.LangKotlin, "fun f(x: Int) { when (x) { 1 -> one() else -> other() } }"},
		{types.LangRust, "fn f(x: i32) { match x { 1 => one(), _ => other(), }; }"},
		{types.LangC, "void f(int x) { switch (x) { case 1: one(); break; default: other(); } }"},
		{types.LangCpp, "void f(int x) { switch (x) { case 1: one(); break; default: other(); } }"},
		{types.LangRuby, "def f(x)\n  case x\n  when 1\n    one()\n  else\n    other()\n  end\nend\n"},
		{types.LangSwift, "func f(_ x: Int) { switch x { case 1: one() default: other() } }"},
		{types.LangArkTS, "function f(x: number) { switch (x) { case 1: one(); break; default: other(); } }"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			root := parseSourceFor(t, tc.lang, tc.src)
			branches := extractControlFlowBranches(root, []byte(tc.src))
			assertAnyControlFlowCall(t, branches, "one")
			assertAnyControlFlowCall(t, branches, "other")
			for _, branch := range branches {
				if branch.Selector == "" {
					t.Fatalf("branch lost switch/match selector: %+v", branch)
				}
			}
		})
	}
}

func TestExtractControlFlowBranches_SkipsNestedCallableEffects(t *testing.T) {
	src := `function select(ready: boolean) {
  if (ready) {
    const deferred = () => hidden();
    visible();
  }
}`
	root := parseSourceFor(t, types.LangTypeScript, src)
	branches := extractControlFlowBranches(root, []byte(src))
	assertAnyControlFlowCall(t, branches, "visible")
	for _, branch := range branches {
		for _, effect := range branch.Effects {
			if effect.Kind == types.ControlFlowEffectCall && strings.Contains(effect.Expression, "hidden") {
				t.Fatalf("nested callable body was attributed to outer guard: %+v", branch)
			}
		}
	}
}

func TestCangjieStructuralFeaturesPublishGuardBranches(t *testing.T) {
	src := []byte(`package demo
func select(ready: Bool): Unit {
  if (ready) {
    run()
  } else {
    stop()
  }
}`)
	_, _, _, _, features, branches, tier := extractCangjieWithStructuralFeatures(src, "src/select.cj")
	if tier != 1 {
		t.Fatalf("tier=%d, want 1", tier)
	}
	if !lineFeatureSetContains(features[3], types.LineFeatureGuard) ||
		!lineFeatureSetContains(features[5], types.LineFeatureGuard) {
		t.Fatalf("Cangjie guard features missing: %+v", features)
	}
	assertControlFlowArmCall(t, branches, types.ControlFlowArmConsequence, "run")
	assertControlFlowArmCall(t, branches, types.ControlFlowArmAlternative, "stop")
}

func assertControlFlowArmCall(t *testing.T, branches []types.ControlFlowBranch, arm types.ControlFlowBranchArm, call string) {
	t.Helper()
	for _, branch := range branches {
		if branch.Arm != arm {
			continue
		}
		for _, effect := range branch.Effects {
			if effect.Kind == types.ControlFlowEffectCall && strings.Contains(effect.Expression, call) {
				return
			}
		}
	}
	t.Fatalf("missing %s arm call %q in %+v", arm, call, branches)
}

func assertAnyControlFlowCall(t *testing.T, branches []types.ControlFlowBranch, call string) {
	t.Helper()
	for _, branch := range branches {
		for _, effect := range branch.Effects {
			if effect.Kind == types.ControlFlowEffectCall && strings.Contains(effect.Expression, call) {
				return
			}
		}
	}
	t.Fatalf("missing call %q in %+v", call, branches)
}
