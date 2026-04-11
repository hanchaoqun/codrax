package agent

import (
	"strings"
	"testing"
)

func TestExpandKeywordsAbbreviations(t *testing.T) {
	tests := []struct {
		input []string
		want  string // at least this keyword should be in output
	}{
		{[]string{"auth"}, "authentication"},
		{[]string{"authentication"}, "auth"},
		{[]string{"config"}, "configuration"},
		{[]string{"configuration"}, "config"},
		{[]string{"ctx"}, "context"},
		{[]string{"exec"}, "execute"},
		{[]string{"eval"}, "evaluate"},
	}
	for _, tt := range tests {
		expanded := expandKeywords(tt.input)
		found := false
		for _, kw := range expanded {
			if strings.ToLower(kw) == tt.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords(%v): expected %q in result %v", tt.input, tt.want, expanded)
		}
	}
}

func TestExpandKeywordsNamingConventions(t *testing.T) {
	expanded := expandKeywords([]string{"sub_agent"})
	// Note: "subagent" (concatenated) is deduplicated with "SubAgent"
	// (CamelCase) because they share the same lowercase form.
	wants := []string{"sub_agent", "SubAgent", "sub-agent"}
	for _, w := range wants {
		found := false
		for _, kw := range expanded {
			if kw == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords([sub_agent]): expected %q in result %v", w, expanded)
		}
	}
}

func TestExpandKeywordsNoAbbrevForUnknown(t *testing.T) {
	expanded := expandKeywords([]string{"foobar"})
	// "foobar" has no abbreviation pair, should only get itself
	if len(expanded) != 1 {
		t.Errorf("expandKeywords([foobar]): expected 1 keyword, got %d: %v", len(expanded), expanded)
	}
}
