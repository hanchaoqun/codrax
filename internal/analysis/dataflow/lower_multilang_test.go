package dataflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	basetypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestNewLowererRegistry_CoversAllSupportedReadLanguages(t *testing.T) {
	registry := newLowererRegistry()
	for _, lang := range repomap.SupportedReadLanguages() {
		if _, ok := registry[lang]; !ok {
			t.Fatalf("language %q missing from lowerer registry", lang)
		}
	}
}

func TestGenericLowerer_ControlFlowBranchEffect_AllExecutableLanguages(t *testing.T) {
	languages := []string{
		repomap.LangGo, repomap.LangPython, repomap.LangJavaScript,
		repomap.LangTypeScript, repomap.LangArkTS, repomap.LangCangjie,
		repomap.LangKotlin, repomap.LangRuby, repomap.LangSwift,
		repomap.LangLua, repomap.LangJava, repomap.LangRust,
		repomap.LangC, repomap.LangCpp,
	}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			file := &repomap.FileInfo{
				RelPath:  "src/sample." + lang,
				Language: lang,
				Hash:     "fixture",
				Symbols: []repomap.Symbol{{
					Name: "run", Kind: "function", File: "src/sample." + lang,
					Line: 1, EndLine: 5,
				}},
				ControlFlowBranches: []rmtypes.ControlFlowBranch{{
					Condition: "ready", GuardLine: 2,
					Arm:           rmtypes.ControlFlowArmConsequence,
					BodyLineStart: 2, BodyLineEnd: 4,
					Provenance: repomap.ProvenanceTreeSitter,
					ResolvedBy: "fixture_parser",
					Effects: []rmtypes.ControlFlowEffect{{
						Kind: rmtypes.ControlFlowEffectCall, Expression: "dispatch(job)",
						LineStart: 3, LineEnd: 3,
					}},
				}},
			}
			if lang == repomap.LangCangjie {
				file.ControlFlowBranches[0].Provenance = repomap.ProvenanceCangjieParser
			}
			lowered := (genericLowerer{lang: lang}).LowerFile("", file,
				[]string{"fn run() {", "if ready {", "dispatch(job)", "}", "}"},
				Options{MaxNodesPerFunc: 100})

			var got *basetypes.EvidenceItem
			for i := range lowered.Evidence {
				if lowered.Evidence[i].Kind == basetypes.EvidenceControlFlow {
					got = &lowered.Evidence[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("language %s: no deterministic control-flow evidence: %+v", lang, lowered.Evidence)
			}
			if !basetypes.IsDeterministicControlFlowEvidence(*got) {
				t.Fatalf("language %s: invalid deterministic carrier: %+v", lang, *got)
			}
			if form := basetypes.ClaimFormOf(*got); form != basetypes.ClaimBranchEffect {
				t.Fatalf("language %s: claim form=%q, want %q", lang, form, basetypes.ClaimBranchEffect)
			}
			if got.Subject != "if ready" || got.Object != "dispatch(job)" ||
				got.Predicate != basetypes.ControlFlowPredicateConsequence || got.LineStart != 2 || got.LineEnd != 3 {
				t.Fatalf("language %s: wrong exact endpoints/range: %+v", lang, *got)
			}
		})
	}
}

func TestControlFlowEvidenceItem_SameLineUsesValidLineScope(t *testing.T) {
	branch := rmtypes.ControlFlowBranch{
		Condition: "ready", GuardLine: 4, Arm: rmtypes.ControlFlowArmConsequence,
		Provenance: rmtypes.ProvenanceTreeSitter, ResolvedBy: "tree_sitter_control_branch",
	}
	effect := rmtypes.ControlFlowEffect{Kind: rmtypes.ControlFlowEffectReturn, Expression: "return cached", LineStart: 4, LineEnd: 4}
	item, ok := controlFlowEvidenceItem(
		&repomap.FileInfo{RelPath: "src/compact.go", Language: repomap.LangGo},
		"compact",
		branch,
		effect,
	)
	if !ok {
		t.Fatal("same-line parser branch effect was dropped")
	}
	if item.Scope != basetypes.ScopeLine || item.LineStart != 4 || item.LineEnd != 4 {
		t.Fatalf("same-line effect must use line scope: %+v", item)
	}
	if err := item.ValidateScope(); err != nil {
		t.Fatalf("same-line deterministic evidence must satisfy shared scope contract: %v", err)
	}
	branch.Provenance = rmtypes.ProvenanceRegexFallback
	if _, ok := controlFlowEvidenceItem(
		&repomap.FileInfo{RelPath: "src/compact.go", Language: repomap.LangGo},
		"compact", branch, effect,
	); ok {
		t.Fatal("regex/proximity branch carrier must not mint branch-effect authority")
	}
}

func TestIsCommentLine_ExtendedLanguages(t *testing.T) {
	cases := []struct {
		lang string
		line string
	}{
		{repomap.LangRuby, "# comment"},
		{repomap.LangLua, "-- comment"},
		{repomap.LangKotlin, "// comment"},
		{repomap.LangSwift, "// comment"},
		{repomap.LangArkTS, "// comment"},
		{repomap.LangCangjie, "// comment"},
	}
	for _, c := range cases {
		if !isCommentLine(c.lang, c.line) {
			t.Fatalf("isCommentLine(%q, %q) = false, want true", c.lang, c.line)
		}
	}
}

// TestDetectGuard_ExtendedLanguages was retired 2026-05-03
// (Phase 6 stage 28). The byte-prefix detectGuard signature
// was replaced with a typed detectGuard(*FileInfo, line, text)
// that reads LineFeatureGuard populated by repomap's AST
// walker. The retired keyword table ({"if ", "if(", "else if",
// "elseif", "elif", "unless", "guard", "switch", "case",
// "when", "match", "until"}) is now AST-detected via the
// closed enum if_statement / case_clause / when_entry /
// guard_statement / unless_statement / etc.
func TestDetectGuard_TypedFeature(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"swift guard", "guard user != nil else {", "guard user != nil else"},
		{"kotlin when", "when (state) {", "when (state)"},
		{"lua elseif", "elseif ready then", "elseif ready then"},
		{"ruby unless", "unless enabled?", "unless enabled?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fi := &repomap.FileInfo{
				LineFeatures: map[int][]repomap.LineFeature{1: {repomap.LineFeatureGuard}},
			}
			if got := detectGuard(fi, 1, c.line); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestDetectUnknownEffect_ExtendedLanguages was retired
// 2026-05-03 (Phase 6 stage 27). The byte-token detectUnknownEffect
// signature was replaced with a typed
// detectUnknownEffect(*FileInfo, line, text) that reads
// LineFeatures populated by repomap's AST walker. The retired
// per-language byte-token tables (Class.forName / public_send /
// Mirror / loadfile / Reflect.get) are now AST-detected via
// dynamicDispatchCallees + typed LineFeatureUnknownEffect.
//
// Replacement: TestDetectUnknownEffect_TypedFeature exercises the
// typed contract — pass a synthetic FileInfo whose LineFeatures
// includes LineFeatureUnknownEffect at the given line and verify
// detectUnknownEffect returns the language's descriptor.
func TestDetectUnknownEffect_TypedFeature(t *testing.T) {
	for _, lang := range repomap.SupportedReadLanguages() {
		fi := &repomap.FileInfo{
			Language:     lang,
			LineFeatures: map[int][]repomap.LineFeature{1: {repomap.LineFeatureUnknownEffect}},
		}
		if got := detectUnknownEffect(fi, 1, "irrelevant"); got == "" {
			t.Errorf("language %s: typed feature should yield descriptor; got empty", lang)
		}
	}
}
