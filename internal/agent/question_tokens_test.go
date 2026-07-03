package agent

import (
	"reflect"
	"testing"
)

// TestScanQuestionTokens_Backticks pins backtick extraction: two
// yields per delimited segment (one per matching ` pair), verbatim
// content minus delimiters, unterminated trailing backticks ignored.
func TestScanQuestionTokens_Backticks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single", "call `Foo.Bar` now", []string{"Foo.Bar"}},
		{"two", "`A` and `B.C`", []string{"A", "B.C"}},
		{"empty between", "``", []string{""}},
		{"unterminated ignored", "what about `Foo", nil},
		{"no backticks", "plain prose", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			scanQuestionTokens(tc.in, func(raw string, src tokenSource) {
				if src == tokenBacktick {
					got = append(got, raw)
				}
			})
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestScanQuestionTokens_Runs pins run-scan extraction: every
// contiguous [A-Za-z0-9_.] span yields one token, boundary on the
// first non-ident byte, trailing run emitted at end-of-string.
func TestScanQuestionTokens_Runs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"camel + lower", "SubAgent and handler", []string{"SubAgent", "and", "handler"}},
		{"dotted", "Foo.Bar thing", []string{"Foo.Bar", "thing"}},
		{"snake", "get_user from api", []string{"get_user", "from", "api"}},
		{"digits", "version_2 of api_v3", []string{"version_2", "of", "api_v3"}},
		{"chinese separators", "查询 Agent 调用", []string{"Agent"}},
		{"empty string", "", nil},
		{"leading+trailing punct", "? Foo !", []string{"Foo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			scanQuestionTokens(tc.in, func(raw string, src tokenSource) {
				if src == tokenRun {
					got = append(got, raw)
				}
			})
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestScanQuestionTokens_Order pins two ordering contracts:
//  1. ALL backtick tokens are yielded BEFORE any run tokens.
//  2. The run scan does NOT treat backticks as structural — the
//     ident characters INSIDE backticks are re-yielded as run
//     tokens on the second pass. Callers rely on their own dedup
//     (case-folded seen map) to absorb the overlap. This test
//     makes that non-obvious contract load-bearing.
func TestScanQuestionTokens_Order(t *testing.T) {
	in := "what about `Foo` and SubAgent and `Bar`?"
	var srcSeq []tokenSource
	var tokens []string
	scanQuestionTokens(in, func(raw string, src tokenSource) {
		srcSeq = append(srcSeq, src)
		tokens = append(tokens, raw)
	})

	// Phase 1: two backtick tokens (Foo, Bar) in input order.
	// Phase 2: seven run tokens — note Foo and Bar appear AGAIN
	// because the run scan treats backticks as non-ident boundaries.
	wantSrc := []tokenSource{
		tokenBacktick, tokenBacktick,
		tokenRun, tokenRun, tokenRun, tokenRun, tokenRun, tokenRun, tokenRun,
	}
	wantTok := []string{
		"Foo", "Bar",
		"what", "about", "Foo", "and", "SubAgent", "and", "Bar",
	}
	if !reflect.DeepEqual(srcSeq, wantSrc) {
		t.Errorf("source order: got %v, want %v", srcSeq, wantSrc)
	}
	if !reflect.DeepEqual(tokens, wantTok) {
		t.Errorf("token order: got %v, want %v", tokens, wantTok)
	}
}
