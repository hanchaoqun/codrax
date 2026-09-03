package tracequery

import "regexp"

// CarrierWireTokenGrammar is the ONE grammar of a codrax carrier wire token:
// `codrax_<family>/v<N>` with a lower-case alphanumeric/underscore family and
// a decimal version. Every codrax-authored carrier line — an ftrace-body
// carrier published under SourceRawVisibilityEventName, or a `# `-prefixed
// comment carrier — starts with such a token, and the namespace is reserved:
// a device-authored ftrace body that begins with a wire token is refused by
// the converter's owned-output postvalidation. The runtime body gate, the
// producer registry check and the structural literal census
// (internal/hitraceconv source_raw_visibility_reserved_name_test.go) all
// derive from this constant so census and runtime can never disagree about
// what is a wire token (colleague_merge_audit §40.38 fold-in F9).
const CarrierWireTokenGrammar = `codrax_[a-z0-9_]+/v[0-9]+`

// CarrierCommentLinePrefix is the prefix every comment carrier line wears
// before its wire token (`# codrax_<family>/v<N> …`).
const CarrierCommentLinePrefix = "# "

var carrierWireTokenAtStart = regexp.MustCompile(`^(` + CarrierWireTokenGrammar + `)(?:\s|$)`)

// CarrierWireToken returns the codrax carrier wire token that starts text as
// a whole token — terminated by whitespace or the end of text — and false
// when text does not start with one. A token followed by any other byte
// (`codrax_x/v1x`) is not a wire token.
func CarrierWireToken(text string) (string, bool) {
	match := carrierWireTokenAtStart.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	return match[1], true
}
