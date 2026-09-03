package tracefence_test

// impact_caliber_definition_faces_test.go — V1-1 §40.25 「词面来自 tracefence
// 单源」, §40.48 fold-in: the legend/definition faces that DEFINE a ruler word
// (the crown-face column legend 「- 窗口投影 = …」, the tree legend entries whose
// definiendum is a ruler word, the row-class definitions 「…行:有效归因 = …」)
// are derived from Table ③e by concatenation, never hand-spelled. The class
// is PRECISE: a legend definition entry (string expression starting with
// "- " and containing " = ") whose definiendum names a Table ③e word — head
// equal to / starting with the word, the word as an exact `code span`, or
// the segment after a scope colon starting with the word. Ordinary prose
// mentions of a ruler word (no definition shape, or a definiendum that is an
// annotation token) stay out of scope — they are prose, not the ruler's
// definition (recorded in the ledger).
//
// The sweep works on whole string EXPRESSIONS (constant-folded `+` chains,
// with tracefence.ImpactCaliber* selectors resolved to their words), so
// splitting the head off and hand-spelling the body is caught too —
// every literal piece of a definition entry must be free of every ③e word
// (either face). Self-red subtests below pin each evasion shape.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

func TestImpactCaliberWords_DefinitionFacesReadTheTable(t *testing.T) {
	hits, err := impactCaliberDefinitionFaceHandCopies([]string{"../tool", "../analysis/tracefinding", "../context", "../types", "../agent", "../orchestrator", "../preview"})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range hits {
		t.Errorf("%s — a legend definition of a ruler word must concatenate tracefence.ImpactCaliber* (Table ③e), not hand-spell it", hit)
	}
}

func TestImpactCaliberDefinitionFaceResolverCoversEveryFace(t *testing.T) {
	resolved := map[string]bool{}
	for _, word := range impactCaliberTableConstants {
		resolved[word] = true
	}
	for _, zh := range []bool{true, false} {
		for _, word := range tracefence.ImpactCaliberWordFaces(zh) {
			if !resolved[word] {
				t.Errorf("Table ③e face %q has no entry in impactCaliberTableConstants — a definition head built from its constant would evade the sweep", word)
			}
		}
	}
}

func TestImpactCaliberDefinitionFaceSweepCatchesEveryEvasionShape(t *testing.T) {
	dir := t.TempDir()
	src := "package scratch\n\n" +
		"import \"github.com/hanchaoqun/codrax/internal/tracefence\"\n\n" +
		"const w = \"X\"\n" +
		"var wholeLiteral = \"- 窗口投影 = 该列的定义\"\n" +
		"var headOnly = \"- \" + tracefence.ImpactCaliberWindowProjectionZH + \" = 定义里又提到 有效归因 的词\"\n" +
		"var scopedHeadOnly = \"- periodic rows: \" + tracefence.ImpactCaliberEffectiveEN + \" = runnable in full (the window projection keeps the raw value)\"\n" +
		"var otherTableHead = \"- \" + tracefence.SeatChannelChainZH + \" = 该列只承载计入的链上影响,与窗口投影不同\"\n" +
		"var backtickHead = \"- `chain total` = the column definition\"\n" +
		"var colonScoped = \"- periodic rows: attribution = a formula\"\n" +
		"var listHead = \"- 行内 `链上累计`/`实际状态` 口径词 = 定义\"\n" +
		"var okConcatenated = \"- \" + tracefence.ImpactCaliberChainCumulativeEN + \" = the definition without any ruler word\"\n" +
		"var proseMention = \"- 优先级反转项的「窗口投影」列为构成值,不是单一状态时长。\"\n" +
		"var annotationHead = \"- `inherited attribution` = the row's value is inherited\"\n" +
		"var suffixOnly = \"- attributions = plural is another word\"\n" +
		"var constHeadWithTail = \"- \" + w + \"依据 = 各项折算后的空间(即 有效归因)\"\n" +
		"var okConcatenatedTail = \"- `\" + tracefence.ImpactCaliberEffectiveZH + \" V = …` 分解行 = 各分量按口径计入\"\n" +
		"var concatenatedTailBody = \"- `\" + tracefence.ImpactCaliberEffectiveZH + \" V = …` 分解行 = 有效归因的构成\"\n" +
		"var bodyBoundary = \"- \" + tracefence.ImpactCaliberWindowProjectionEN + \" = measured as a cross-thread cumulative, not the attributions of others\"\n" +
		"var bodyWordAtBoundary = \"- \" + tracefence.ImpactCaliberWindowProjectionEN + \" = differs from the attribution (see the chain total)\"\n"
	if err := os.WriteFile(filepath.Join(dir, "scratch.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, err := impactCaliberDefinitionFaceHandCopies([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(hits, "\n")
	for _, want := range []string{"wholeLiteral", "headOnly", "scopedHeadOnly", "backtickHead", "colonScoped", "listHead", "concatenatedTailBody", "bodyWordAtBoundary"} {
		if !strings.Contains(joined, "var="+want+" ") {
			t.Errorf("evasion shape %s not caught:\n%s", want, joined)
		}
	}
	for _, exempt := range []string{"okConcatenated", "proseMention", "annotationHead", "suffixOnly", "constHeadWithTail", "otherTableHead", "okConcatenatedTail", "bodyBoundary"} {
		if strings.Contains(joined, "var="+exempt+" ") {
			t.Errorf("%s is not a hand-copied definition face and must not be flagged:\n%s", exempt, joined)
		}
	}
}

// impactCaliberDefinitionFaceHandCopies returns one line per hand-copied
// definition face found under dirs ("<pos> var=<name> word=<w>").
func impactCaliberDefinitionFaceHandCopies(dirs []string) ([]string, error) {
	var words []string
	for _, zh := range []bool{true, false} {
		words = append(words, tracefence.ImpactCaliberWordFaces(zh)...)
	}
	fset := token.NewFileSet()
	var hits []string
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			seen := map[ast.Node]bool{}
			varName := ""
			ast.Inspect(file, func(n ast.Node) bool {
				if spec, ok := n.(*ast.ValueSpec); ok && len(spec.Names) > 0 {
					varName = spec.Names[0].Name
				}
				if seen[n] {
					return true
				}
				expr, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				switch expr.(type) {
				case *ast.BinaryExpr, *ast.BasicLit, *ast.ParenExpr:
				default:
					return true
				}
				text, pieces, ok := impactCaliberStringExpression(expr)
				if !ok {
					return true
				}
				ast.Inspect(n, func(c ast.Node) bool { seen[c] = true; return true })
				if !impactCaliberDefinitionEntryDefinesWord(text, words) {
					return true
				}
				for _, piece := range pieces {
					for _, word := range words {
						if impactCaliberContainsWord(piece, word) {
							hits = append(hits, fmt(fset.Position(n.Pos()).String(), varName, word))
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return hits, nil
}

func fmt(pos, varName, word string) string {
	return pos + " var=" + varName + " word=" + strconv.Quote(word)
}

// impactCaliberTableConstants resolves a `tracefence.ImpactCaliber*` selector
// to its word, so a properly concatenated definiendum ("- " + tracefence.X +
// " = …") is still recognized as the ruler word's definition and a
// hand-spelled word left in its BODY is caught. Compile-tied to the real
// constants; any other operand (another table's word, a call) folds to a
// NUL placeholder and defines something else.
var impactCaliberTableConstants = map[string]string{
	"ImpactCaliberEffectiveZH":        tracefence.ImpactCaliberEffectiveZH,
	"ImpactCaliberEffectiveEN":        tracefence.ImpactCaliberEffectiveEN,
	"ImpactCaliberWindowProjectionZH": tracefence.ImpactCaliberWindowProjectionZH,
	"ImpactCaliberWindowProjectionEN": tracefence.ImpactCaliberWindowProjectionEN,
	"ImpactCaliberChainCumulativeZH":  tracefence.ImpactCaliberChainCumulativeZH,
	"ImpactCaliberChainCumulativeEN":  tracefence.ImpactCaliberChainCumulativeEN,
	"ImpactCaliberActualStateZH":      tracefence.ImpactCaliberActualStateZH,
	"ImpactCaliberActualStateEN":      tracefence.ImpactCaliberActualStateEN,
	"ImpactCaliberCrossThreadCumZH":   tracefence.ImpactCaliberCrossThreadCumZH,
	"ImpactCaliberCrossThreadCumEN":   tracefence.ImpactCaliberCrossThreadCumEN,
}

// impactCaliberStringExpression folds a `+` chain of string literals into
// its text (a Table ③e selector resolves to its word; any other non-literal
// operand becomes a NUL placeholder) and the raw literal pieces.
func impactCaliberStringExpression(expr ast.Expr) (text string, pieces []string, ok bool) {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		if pkg, isIdent := x.X.(*ast.Ident); isIdent && pkg.Name == "tracefence" {
			if word, known := impactCaliberTableConstants[x.Sel.Name]; known {
				return word, nil, true
			}
		}
		return "", nil, false
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", nil, false
		}
		s, err := strconv.Unquote(x.Value)
		if err != nil {
			return "", nil, false
		}
		return s, []string{s}, true
	case *ast.ParenExpr:
		return impactCaliberStringExpression(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", nil, false
		}
		lt, lp, lok := impactCaliberStringExpression(x.X)
		rt, rp, rok := impactCaliberStringExpression(x.Y)
		if !lok {
			lt = "\x00"
		}
		if !rok {
			rt = "\x00"
		}
		return lt + rt, append(lp, rp...), lok || rok
	}
	return "", nil, false
}

// impactCaliberDefinitionEntryDefinesWord — the precise class: a legend
// definition entry whose definiendum names a ruler word.
func impactCaliberDefinitionEntryDefinesWord(text string, words []string) bool {
	if !strings.HasPrefix(text, "- ") {
		return false
	}
	eq := strings.Index(text, " = ")
	if eq < 0 {
		return false
	}
	head := text[2:eq]
	bare := strings.TrimSpace(strings.ReplaceAll(head, "`", ""))
	scoped := bare
	if i := strings.LastIndexAny(bare, ":："); i >= 0 {
		scoped = strings.TrimSpace(bare[i+len(string(bare[i])):])
	}
	for _, word := range words {
		if strings.Contains(head, "`"+word+"`") ||
			impactCaliberStartsWithWord(bare, word) || impactCaliberStartsWithWord(scoped, word) {
			return true
		}
	}
	return false
}

// impactCaliberStartsWithWord — prefix match with a word boundary after the
// word (so "attribution" never matches "attributions").
func impactCaliberStartsWithWord(s, word string) bool {
	if !strings.HasPrefix(s, word) {
		return false
	}
	return impactCaliberWordBoundaryAfter(s[len(word):])
}

// impactCaliberContainsWord — substring match on a word boundary at both
// ends (so the EN face "cross-thread cum" is not found inside the plain
// prose "cross-thread cumulative", and "attribution" not inside
// "attributions"); Han faces have no letter boundary and match verbatim.
func impactCaliberContainsWord(s, word string) bool {
	for offset := 0; ; {
		i := strings.Index(s[offset:], word)
		if i < 0 {
			return false
		}
		start := offset + i
		before := []rune(s[:start])
		if (len(before) == 0 || impactCaliberWordBoundaryRune(before[len(before)-1])) &&
			impactCaliberWordBoundaryAfter(s[start+len(word):]) {
			return true
		}
		offset = start + len(word)
	}
}

func impactCaliberWordBoundaryAfter(rest string) bool {
	runes := []rune(rest)
	return len(runes) == 0 || impactCaliberWordBoundaryRune(runes[0])
}

func impactCaliberWordBoundaryRune(r rune) bool {
	return !unicode.IsLetter(r) || unicode.Is(unicode.Han, r)
}
