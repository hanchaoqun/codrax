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
		got := languageDirective(c.in, "")
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

// TestLanguageDirective_AssertionFromQuestion pins the hard-assertion
// path: when the user's question has dominant CJK or Latin prose,
// the directive is prefixed with an imperative "The user's question
// is in X. You MUST write your answer in X." The 2026-04-19
// regression was that the conditional base directive, while clear,
// was ignored when surrounding context (tool outputs, skill prompts)
// was English-heavy; the assertion removes that ambiguity.
func TestLanguageDirective_AssertionFromQuestion(t *testing.T) {
	// Chinese question with English code identifiers — classic mixed
	// shape. Must still fire the CJK assertion.
	zhQ := "explorer是怎么调用subagent的？"
	got := languageDirective("zh", zhQ)
	if !strings.Contains(got, "question is written in Chinese") {
		t.Errorf("expected CJK assertion prefix, got: %q", got)
	}
	if !strings.Contains(got, "MUST write your answer in Simplified Chinese") {
		t.Errorf("expected imperative MUST clause, got: %q", got)
	}
	// English question — Latin assertion fires when letters >= 20
	// and CJK == 0.
	enQ := "how does the explorer agent invoke the subagent mechanism"
	gotEn := languageDirective("zh", enQ)
	if !strings.Contains(gotEn, "question is written in English") {
		t.Errorf("expected Latin assertion prefix for English question, got: %q", gotEn)
	}
	// Pure code / too-short question → no assertion, falls through
	// to the base conditional directive.
	shortQ := "Foo()"
	gotShort := languageDirective("zh", shortQ)
	if strings.Contains(gotShort, "question is written") {
		t.Errorf("assertion should not fire on too-short question, got: %q", gotShort)
	}
	if !strings.Contains(gotShort, "same natural language") {
		t.Errorf("base directive missing on short input, got: %q", gotShort)
	}
	// Empty question — no assertion, base directive only.
	gotEmpty := languageDirective("zh", "")
	if strings.Contains(gotEmpty, "question is written") {
		t.Errorf("assertion should not fire on empty question")
	}
}

func TestDetectedLanguageAssertion(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantSub  string // substring that must appear; empty means "no assertion"
		wantNone bool
	}{
		{"Chinese with code identifiers", "explorer是怎么调用subagent的", "Chinese", false},
		{"Pure Chinese", "这个函数是怎么工作的", "Chinese", false},
		{"Japanese hiragana", "これは何ですかね教えてください", "Chinese", false}, // our assertion says "Chinese" but the CJK gate covers JP/KR too
		{"English sentence", "how does the router dispatch requests to handlers", "English", false},
		{"Short single word", "lang", "", true},
		{"Pure code", "Foo()", "", true},
		{"Mixed below CJK threshold", "是 Foo", "", true}, // 1 CJK char
		{"Empty", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectedLanguageAssertion(c.in)
			if c.wantNone {
				if got != "" {
					t.Errorf("expected no assertion, got: %q", got)
				}
				return
			}
			if got == "" {
				t.Errorf("expected assertion containing %q, got empty", c.wantSub)
			}
			if !strings.Contains(got, c.wantSub) {
				t.Errorf("expected assertion to mention %q, got: %q", c.wantSub, got)
			}
		})
	}
}
