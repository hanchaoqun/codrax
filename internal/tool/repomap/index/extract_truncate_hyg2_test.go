package index

// HYG-2 G18 pin (§27.5): the eight extractors' shared sig[:117] byte cut.
// Identifiers (Go/Java/JS… and especially ArkTS/Cangjie-adjacent code) can
// be CJK — the 117-byte cut must land on a rune boundary. Repomap red line:
// this pin exercises ONLY the truncation; language detection and package
// parsing stay untouched.

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestHYG2ExtractGoSignatureTruncationRuneSafe(t *testing.T) {
	// Parameter name of 60 CJK runes → signature "(长…长 s string)" well over
	// the 120-byte guard; byte 117 lands mid-rune ("(" at byte 0, runes at
	// 1+3k — 117 is a continuation byte).
	src := "package p\n\nfunc 处理(" + strings.Repeat("长", 60) + " string) {}\n"
	root := parseSourceFor(t, types.LangGo, src)
	_, syms, _, _ := extractGo(root, []byte(src), "p.go")
	if len(syms) == 0 {
		t.Fatalf("no symbols extracted")
	}
	found := false
	for _, s := range syms {
		if s.Name != "处理" {
			continue
		}
		found = true
		if s.Signature == "" {
			t.Fatalf("signature missing for 处理")
		}
		if !utf8.ValidString(s.Signature) || strings.Contains(s.Signature, "�") {
			t.Fatalf("signature carries invalid UTF-8: %q", s.Signature)
		}
		if !strings.HasSuffix(s.Signature, "...") {
			t.Fatalf("signature lost its truncation marker: %q", s.Signature)
		}
		if len(s.Signature) > 120 {
			t.Fatalf("signature exceeded its byte budget: %d", len(s.Signature))
		}
	}
	if !found {
		t.Fatalf("function 处理 not extracted; got %v", symbolNames(syms))
	}
}

// TestHYG2ExtractorVersionsBumpedForRuneSafeSigCut pins the cache-protocol
// obligation stated at extractorVersions: the HYG-2 rune-safe signature cut
// changed the 8 extractors' output bytes, so every language routed through a
// changed file must carry a generation >= this batch's bump — otherwise an
// existing cache keeps serving the old broken-byte signatures and the fix
// silently never lands. Kotlin/Swift/Proto/Cangjie extractors were untouched
// and deliberately have no floor here.
func TestHYG2ExtractorVersionsBumpedForRuneSafeSigCut(t *testing.T) {
	floors := map[string]int{
		types.LangGo:         6,
		types.LangJava:       4,
		types.LangPython:     5,
		types.LangJavaScript: 4,
		types.LangTypeScript: 4,
		types.LangArkTS:      4, // routes through extractJS
		types.LangRuby:       3,
		types.LangLua:        3,
		types.LangRust:       3,
		types.LangC:          3, // extractCCpp shared
		types.LangCpp:        3, // extractCCpp shared
	}
	for lang, floor := range floors {
		got, ok := extractorVersions[lang]
		if !ok {
			t.Errorf("extractorVersions lost the %q entry", lang)
			continue
		}
		if got < floor {
			t.Errorf("extractorVersions[%q] = %d regressed below the HYG-2 floor %d — stale caches would keep pre-rune-safe signatures", lang, got, floor)
		}
	}
}
