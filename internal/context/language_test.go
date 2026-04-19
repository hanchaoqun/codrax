package context

import (
	"strings"
	"testing"
)

func TestLanguageDirective(t *testing.T) {
	cases := []struct {
		in        string
		wantEmpty bool
		mustHave  string
	}{
		{in: "", wantEmpty: true},
		{in: "off", wantEmpty: true},
		{in: "none", wantEmpty: true},
		{in: "zh", mustHave: "Simplified Chinese"},
		{in: "zh-CN", mustHave: "Simplified Chinese"},
		{in: "chinese", mustHave: "Simplified Chinese"},
		{in: "en", mustHave: "English"},
		{in: "english", mustHave: "English"},
		{in: "fr", mustHave: "fr"},
	}
	for _, c := range cases {
		got := languageDirective(c.in)
		if c.wantEmpty {
			if got != "" {
				t.Errorf("languageDirective(%q) = %q, want empty", c.in, got)
			}
			continue
		}
		if got == "" {
			t.Errorf("languageDirective(%q) returned empty; expected non-empty", c.in)
			continue
		}
		if !strings.Contains(got, c.mustHave) {
			t.Errorf("languageDirective(%q) = %q, want substring %q", c.in, got, c.mustHave)
		}
		if !strings.Contains(strings.ToLower(got), "same natural language") {
			t.Errorf("languageDirective(%q) missing language-match clause: %q", c.in, got)
		}
		if !strings.Contains(strings.ToLower(got), "technical terms") {
			t.Errorf("languageDirective(%q) missing term-preservation clause: %q", c.in, got)
		}
		if !strings.Contains(strings.ToLower(got), "default to") {
			t.Errorf("languageDirective(%q) missing ambiguous-default clause: %q", c.in, got)
		}
	}
}
