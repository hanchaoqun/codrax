package agent

import "strings"

// tokenSource distinguishes the two production rules the question
// tokeniser walks. Callers that treat the two sources differently
// (e.g. backtick = unconditional admit, run-scan = shape-gated)
// branch on this flag; callers that treat them uniformly ignore it.
type tokenSource int

const (
	tokenBacktick tokenSource = iota // `Foo.Bar` — delimited identifier
	tokenRun                         // unquoted [A-Za-z0-9_.] run
)

// scanQuestionTokens yields raw candidate identifier tokens from the
// user's question text. Two sources:
//
//  1. Backtick-quoted segments (e.g. `MyClass.doThing`) — the most
//     explicit identifier form, yielded verbatim minus the backticks.
//  2. Contiguous runs of [A-Za-z0-9_.] characters — ident-like spans
//     that span CamelCase, snake_case, dotted, and lowercase tokens.
//
// The tokeniser is pure: no dedup, no trimming, no case folding, no
// length / shape gating. Callers apply their own policy in the visit
// callback, branching on the tokenSource tag when the policy differs
// by source. Callback-based rather than slice-returning so callers
// can dedup in-place without an intermediate materialisation.
//
// Consolidates the byte-identical scan loops that used to live in
// explorer.extractQuestionEntities and evidence.extractRankingEntitiesWithGraph
// — both tokeniser bodies (backtick loop + [A-Za-z0-9_.] run loop with
// inIdent/identStart state machine) were word-for-word the same;
// their differences lived entirely in the admit-time policy.
func scanQuestionTokens(question string, visit func(raw string, src tokenSource)) {
	// 1. Backtick-quoted identifiers.
	rest := question
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start+1:], "`")
		if end < 0 {
			break
		}
		visit(rest[start+1:start+1+end], tokenBacktick)
		rest = rest[start+1+end+1:]
	}

	// 2. Contiguous [A-Za-z0-9_.] runs.
	inIdent := false
	identStart := 0
	for i := 0; i <= len(question); i++ {
		var c byte
		if i < len(question) {
			c = question[i]
		}
		isIdent := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.'
		if isIdent && !inIdent {
			identStart = i
			inIdent = true
		} else if !isIdent && inIdent {
			visit(question[identStart:i], tokenRun)
			inIdent = false
		}
	}
}
