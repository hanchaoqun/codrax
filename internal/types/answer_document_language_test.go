package types

import "testing"

func TestResolveAnswerDocumentLanguage(t *testing.T) {
	cases := []struct {
		name, configured, contract, request, want string
		source                                    AnswerDocumentLanguageSource
	}{
		{"project_wins", "zh", "en", "en", "zh", AnswerDocumentLanguageConfigured},
		{"reverse", "en", "zh", "zh", "en", AnswerDocumentLanguageConfigured},
		{"off", " OFF ", "zh", "zh", "en", AnswerDocumentLanguageDisabled},
		{"none", "none", "zh", "zh", "en", AnswerDocumentLanguageDisabled},
		{"auto", "auto", "zh", "en", "zh", AnswerDocumentLanguageAnalysisContract},
		{"follow", "follow", "en", "zh", "en", AnswerDocumentLanguageAnalysisContract},
		{"blank", "", "", "zh", "zh", AnswerDocumentLanguageAnalysisRequest},
		{"contract_off", "auto", "off", "zh", "zh", AnswerDocumentLanguageAnalysisRequest},
		{"contract_none", "", "none", "zh", "zh", AnswerDocumentLanguageAnalysisRequest},
		{"unknown", "fr", "de", "zh", "zh", AnswerDocumentLanguageAnalysisRequest},
		{"unknown_region", "zh-TW", "en-GB", "zh", "zh", AnswerDocumentLanguageAnalysisRequest},
		{"empty", "", "", "", "en", AnswerDocumentLanguageDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang, source := ResolveAnswerDocumentLanguage(tc.configured, tc.contract, tc.request)
			if lang != tc.want || source != tc.source {
				t.Fatalf("got %q/%q, want %q/%q", lang, source, tc.want, tc.source)
			}
		})
	}
	for _, group := range []struct {
		want    string
		aliases []string
	}{
		{"zh", []string{"zh", " ZH-CN ", "cn", "CHINESE", "简体中文"}},
		{"en", []string{"en", " EN-US ", "English"}},
	} {
		for _, alias := range group.aliases {
			for level := 0; level < 3; level++ {
				in := [3]string{}
				in[level] = alias
				got, _ := ResolveAnswerDocumentLanguage(in[0], in[1], in[2])
				if got != group.want {
					t.Errorf("alias %q at level %d: %q", alias, level, got)
				}
			}
		}
	}
}
