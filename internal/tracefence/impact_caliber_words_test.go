package tracefence_test

// impact_caliber_words_test.go — V1-1 (colleague_merge_audit §40.25 「词面来自
// tracefence 单源」, 2026-09-03): Table ③e pins.
//
//  1. closed-set tie: every public sidecar caliber token owned by
//     internal/types (AllTraceImpactCalibers) resolves BOTH ruler-word faces
//     and the sidecar evidence phrase in Table ③e (types 加第 3 口径而词表缺
//     → 红); unknown tokens resolve nothing (fail-closed, absence never
//     guesses). Test-only import: tracefence is a leaf (types does not import
//     it in production), so the import graph stays acyclic — the
//     fix_direction_word_test.go precedent.
//  2. golden: the ruler words and the sidecar phrases are a deliberate closed
//     set — a change is a wordface decision, not a drive-by.
//  3. hand-copy census: no production string literal in the emitting
//     packages spells a Table ③e word (zh or en) as its own literal, nor
//     carries a sidecar sentence form (<phrase>为) — the §29.38 hand-copy
//     disease has no place left to grow.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestImpactCaliberWordCoversClosedSet(t *testing.T) {
	tokens := types.AllTraceImpactCalibers()
	if len(tokens) < 2 {
		t.Fatalf("closed set looks broken: %v", tokens)
	}
	for _, caliber := range tokens {
		if !types.ValidTraceImpactCaliber(caliber) {
			t.Fatalf("AllTraceImpactCalibers and ValidTraceImpactCaliber disagree on %q", caliber)
		}
		for _, zh := range []bool{true, false} {
			word, ok := tracefence.ImpactCaliberWord(caliber, zh)
			if !ok || word == "" {
				t.Fatalf("caliber %q (zh=%v) has no Table ③e ruler word", caliber, zh)
			}
			if !reflect.DeepEqual(true, containsWord(tracefence.ImpactCaliberWordFaces(zh), word)) {
				t.Fatalf("ruler word %q for %q is not in the census list ImpactCaliberWordFaces(%v)", word, caliber, zh)
			}
		}
		phrase, _, ok := tracefence.SidecarImpactCaliberPhrase(caliber)
		if !ok || phrase == "" {
			t.Fatalf("caliber %q has no Table ③e sidecar phrase", caliber)
		}
	}
	for _, bogus := range []string{"raw", "", "bogus_caliber"} {
		if _, ok := tracefence.ImpactCaliberWord(bogus, true); ok {
			t.Fatalf("token %q must not resolve a ruler word", bogus)
		}
		if _, _, ok := tracefence.SidecarImpactCaliberPhrase(bogus); ok {
			t.Fatalf("token %q must not resolve a sidecar phrase", bogus)
		}
	}
}

func TestImpactCaliberWordFaces_ClosedSetGolden(t *testing.T) {
	if got, want := tracefence.ImpactCaliberWordFaces(true), []string{"有效归因", "窗口投影", "链上累计", "实际状态", "累计(跨线程)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zh ruler-word closed set changed (a wordface decision): got %v want %v", got, want)
	}
	if got, want := tracefence.ImpactCaliberWordFaces(false), []string{"attribution", "window projection", "chain total", "actual state", "cross-thread cum"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("en ruler-word closed set changed (a wordface decision): got %v want %v", got, want)
	}
	if phrase, suffix, ok := tracefence.SidecarImpactCaliberPhrase(types.TraceImpactCaliberEffectiveAttribution); !ok || phrase != "链上有效归因" || suffix != "" {
		t.Fatalf("effective sidecar phrase drifted: %q %q %v", phrase, suffix, ok)
	}
	if phrase, suffix, ok := tracefence.SidecarImpactCaliberPhrase(types.TraceImpactCaliberWindowProjection); !ok || phrase != "窗内投影占用" || suffix != "（未发布有效归因）" {
		t.Fatalf("window-projection sidecar phrase drifted: %q %q %v", phrase, suffix, ok)
	}
	// CROWNCAL: the window-projection phrase itself never says 有效.
	phrase, _, _ := tracefence.SidecarImpactCaliberPhrase(types.TraceImpactCaliberWindowProjection)
	if strings.Contains(phrase, tracefence.ImpactCaliberEffectiveZH) {
		t.Fatalf("a raw window projection must never be phrased as %s: %q", tracefence.ImpactCaliberEffectiveZH, phrase)
	}
}

// TestImpactCaliberWords_NoHandCopies — AST literal scan of the production
// sources of every package that prints an impact ruler word: a literal
// EXACTLY equal to a Table ③e word (either face), or CONTAINING a sidecar
// sentence form (<phrase>为), is a hand copy that must read the table instead.
// Test files are exempt (they pin the bytes as fixtures); the table itself is
// the single source and is not scanned (it lives in this package, which is
// not in the dir list).
func TestImpactCaliberWords_NoHandCopies(t *testing.T) {
	exact := map[string]bool{}
	for _, zh := range []bool{true, false} {
		for _, word := range tracefence.ImpactCaliberWordFaces(zh) {
			exact[word] = true
			// The EN column headers are the title-cased faces (§40.48): a
			// hand-typed "Window projection" is a hand copy too.
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			exact[string(runes)] = true
		}
	}
	var sentenceForms []string
	for _, caliber := range types.AllTraceImpactCalibers() {
		phrase, _, ok := tracefence.SidecarImpactCaliberPhrase(caliber)
		if ok {
			sentenceForms = append(sentenceForms, phrase+"为")
		}
	}
	fset := token.NewFileSet()
	for _, dir := range []string{"../tool", "../analysis/tracefinding", "../context", "../types", "../agent", "../orchestrator", "../preview"} {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if exact[raw] {
					t.Errorf("%s: hand-copied impact ruler word %q — read tracefence Table ③e (ImpactCaliber*) instead", fset.Position(lit.Pos()), raw)
				}
				for _, form := range sentenceForms {
					if strings.Contains(raw, form) {
						t.Errorf("%s: hand-copied sidecar sentence form %q — read tracefence.SidecarImpactCaliberPhrase instead", fset.Position(lit.Pos()), form)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("sweep %s: %v", dir, err)
		}
	}
}

func containsWord(words []string, want string) bool {
	for _, word := range words {
		if word == want {
			return true
		}
	}
	return false
}
