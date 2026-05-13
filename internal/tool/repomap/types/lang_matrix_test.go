package types

import "testing"

func TestSupportedReadLanguages_CoversExtensionLanguageMatrix(t *testing.T) {
	supported := map[string]bool{}
	for _, lang := range SupportedReadLanguages() {
		if lang == "" {
			t.Fatal("SupportedReadLanguages returned an empty language")
		}
		if supported[lang] {
			t.Fatalf("SupportedReadLanguages returned duplicate language %q", lang)
		}
		supported[lang] = true
	}
	for ext, lang := range extToLang {
		if !supported[lang] {
			t.Fatalf("extension %q maps to language %q, but SupportedReadLanguages does not include it", ext, lang)
		}
	}
	if supported[""] {
		t.Fatal("empty language must not be considered supported")
	}
}

func TestSupportedReadLanguages_ReturnsDefensiveCopy(t *testing.T) {
	first := SupportedReadLanguages()
	if len(first) == 0 {
		t.Fatal("SupportedReadLanguages returned no languages")
	}
	original := first[0]
	first[0] = "mutated"
	second := SupportedReadLanguages()
	if second[0] != original {
		t.Fatalf("SupportedReadLanguages leaked mutable backing slice: got %q want %q", second[0], original)
	}
}

func TestSupportedReadLanguages_EdgeRemapsAndDeniedArtifacts(t *testing.T) {
	cases := []struct {
		path      string
		want      string
		supported bool
	}{
		{"kernel.cu", LangCpp, true},
		{"kernel.cuh", LangCpp, true},
		{"legacy.m", LangC, true},
		{"legacy.mm", LangCpp, true},
		{"target/output.cjo", "", false},
	}
	for _, tc := range cases {
		got := DetectLanguage(tc.path)
		if got != tc.want {
			t.Fatalf("DetectLanguage(%q) = %q, want %q", tc.path, got, tc.want)
		}
		if ok := IsSupportedReadLanguage(got); ok != tc.supported {
			t.Fatalf("IsSupportedReadLanguage(%q) = %v, want %v", got, ok, tc.supported)
		}
	}
}

func TestSupportedReadLanguages_HaveParserOrNativeExtractor(t *testing.T) {
	for _, lang := range SupportedReadLanguages() {
		if GetSitterLanguage(lang) == nil && lang != LangCangjie {
			t.Fatalf("language %q has neither tree-sitter grammar nor documented native extractor", lang)
		}
	}
}
