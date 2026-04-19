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
		// Locked directive must be imperative: "MUST write" + "hard
		// requirement set by the project configuration".
		if !strings.Contains(got, "MUST write") {
			t.Errorf("languageDirective(%q) missing imperative MUST: %q", c.in, got)
		}
		if !strings.Contains(got, "hard requirement set by the project configuration") {
			t.Errorf("languageDirective(%q) missing config-locked clause: %q", c.in, got)
		}
		if !strings.Contains(strings.ToLower(got), "code identifiers") {
			t.Errorf("languageDirective(%q) missing term-preservation clause: %q", c.in, got)
		}
	}
}

// TestLanguageDirective_ConfigWinsOverQuestionLanguage pins the
// config-priority model. A persistent `lang: zh` config forces
// Chinese output even when the user asks in English, and vice
// versa. Question-driven language matching is only active on the
// `auto` / `follow` sentinel values.
func TestLanguageDirective_ConfigWinsOverQuestionLanguage(t *testing.T) {
	// Config lang=zh + English question → Chinese lock wins.
	enQ := "how does the explorer agent invoke the subagent mechanism"
	got := languageDirective("zh", enQ)
	if !strings.Contains(got, "Simplified Chinese") {
		t.Errorf("expected Chinese lock, got: %q", got)
	}
	if strings.Contains(got, "question is written in English") {
		t.Errorf("config-lock path must NOT emit a question-language assertion, got: %q", got)
	}
	// Config lang=en + Chinese question → English lock wins.
	zhQ := "explorer是怎么调用subagent的？"
	gotEn := languageDirective("en", zhQ)
	if !strings.Contains(gotEn, "in English") {
		t.Errorf("expected English lock, got: %q", gotEn)
	}
	if strings.Contains(gotEn, "question is written in Chinese") {
		t.Errorf("config-lock path must NOT emit a question-language assertion, got: %q", gotEn)
	}
}

// TestLanguageDirective_AutoFollowsQuestionLanguage pins the
// opt-in follow-question path. `lang: auto` is the only sentinel
// that consults question language.
func TestLanguageDirective_AutoFollowsQuestionLanguage(t *testing.T) {
	zhQ := "explorer是怎么调用subagent的？"
	got := languageDirective("auto", zhQ)
	if !strings.Contains(got, "question is written in Chinese") {
		t.Errorf("expected CJK assertion prefix, got: %q", got)
	}
	if !strings.Contains(got, "MUST write your answer in Simplified Chinese") {
		t.Errorf("expected imperative MUST clause, got: %q", got)
	}
	enQ := "how does the explorer agent invoke the subagent mechanism"
	gotEn := languageDirective("auto", enQ)
	if !strings.Contains(gotEn, "question is written in English") {
		t.Errorf("expected Latin assertion for English question, got: %q", gotEn)
	}
	// Too-short question on auto → no assertion, falls through to
	// the base "same-language" rule with a default.
	shortQ := "Foo()"
	gotShort := languageDirective("auto", shortQ)
	if strings.Contains(gotShort, "question is written") {
		t.Errorf("assertion should not fire on too-short question, got: %q", gotShort)
	}
	if !strings.Contains(gotShort, "same natural language") {
		t.Errorf("base conditional directive missing on short input, got: %q", gotShort)
	}
	// Empty question on auto → no assertion, base directive only.
	gotEmpty := languageDirective("auto", "")
	if strings.Contains(gotEmpty, "question is written") {
		t.Errorf("assertion should not fire on empty question")
	}
	// `follow` is an alias for `auto`.
	gotFollow := languageDirective("follow", zhQ)
	if !strings.Contains(gotFollow, "question is written in Chinese") {
		t.Errorf("`follow` alias did not trigger assertion, got: %q", gotFollow)
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
