package tool

import "strings"

// probeCouplingToken is a deliberately small lexical carrier for module-edge
// extraction.  It does not infer behavior from model prose: it separates code
// identifiers/punctuation from literal payloads so only an actual language
// import/require form can mint the typed changed-module coupling edge.
type probeCouplingToken struct {
	kind probeCouplingTokenKind
	text string
}

type probeCouplingTokenKind uint8

const (
	probeCouplingIdentifier probeCouplingTokenKind = iota + 1
	probeCouplingString
	probeCouplingPunct
)

func javascriptProbeModuleRefs(code string) []string {
	tokens := lexProbeCouplingTokens(code, true, false)
	var out []string
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.kind != probeCouplingIdentifier {
			continue
		}
		switch tok.text {
		case "require":
			if i > 0 && tokens[i-1].kind == probeCouplingPunct && tokens[i-1].text == "." {
				continue
			}
			if ref, ok := probeCallStringArgument(tokens, i+1); ok {
				out = append(out, ref)
			}
		case "import":
			if ref, ok := probeCallStringArgument(tokens, i+1); ok {
				out = append(out, ref)
				continue
			}
			if i+1 < len(tokens) && tokens[i+1].kind == probeCouplingString {
				out = append(out, tokens[i+1].text)
				continue
			}
			for j := i + 1; j < len(tokens); j++ {
				if tokens[j].kind == probeCouplingPunct && (tokens[j].text == ";" || tokens[j].text == "\n") {
					break
				}
				if tokens[j].kind == probeCouplingIdentifier && tokens[j].text == "from" && j+1 < len(tokens) && tokens[j+1].kind == probeCouplingString {
					out = append(out, tokens[j+1].text)
					break
				}
			}
		}
	}
	return uniqueNonEmptyStrings(out)
}

func rubyProbeModuleRefs(code string) []string {
	tokens := lexProbeCouplingTokens(code, false, true)
	var out []string
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind != probeCouplingIdentifier {
			continue
		}
		switch tokens[i].text {
		case "require", "require_relative", "load":
			j := i + 1
			if j < len(tokens) && tokens[j].kind == probeCouplingPunct && tokens[j].text == "(" {
				j++
			}
			if j < len(tokens) && tokens[j].kind == probeCouplingString {
				out = append(out, tokens[j].text)
			}
		}
	}
	return uniqueNonEmptyStrings(out)
}

func probeCallStringArgument(tokens []probeCouplingToken, start int) (string, bool) {
	if start+1 >= len(tokens) || tokens[start].kind != probeCouplingPunct || tokens[start].text != "(" || tokens[start+1].kind != probeCouplingString {
		return "", false
	}
	return tokens[start+1].text, strings.TrimSpace(tokens[start+1].text) != ""
}

// lexProbeCouplingTokens preserves only the distinctions required by the
// JavaScript and Ruby module carriers.  Quoted payloads become string tokens;
// comments and all text inside them disappear.  Newlines stay explicit so a
// static import cannot consume a later unrelated `from` clause.
func lexProbeCouplingTokens(src string, slashComments, hashComments bool) []probeCouplingToken {
	var out []probeCouplingToken
	for i := 0; i < len(src); {
		ch := src[i]
		if ch == '\n' {
			out = append(out, probeCouplingToken{kind: probeCouplingPunct, text: "\n"})
			i++
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\f' {
			i++
			continue
		}
		if hashComments && ch == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if slashComments && ch == '/' && i+1 < len(src) {
			switch src[i+1] {
			case '/':
				i += 2
				for i < len(src) && src[i] != '\n' {
					i++
				}
				continue
			case '*':
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					if src[i] == '\n' {
						out = append(out, probeCouplingToken{kind: probeCouplingPunct, text: "\n"})
					}
					i++
				}
				if i+1 < len(src) {
					i += 2
				}
				continue
			}
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote := ch
			i++
			var b strings.Builder
			escaped := false
			for i < len(src) {
				current := src[i]
				i++
				if escaped {
					b.WriteByte(current)
					escaped = false
					continue
				}
				if current == '\\' {
					escaped = true
					continue
				}
				if current == quote {
					break
				}
				b.WriteByte(current)
			}
			out = append(out, probeCouplingToken{kind: probeCouplingString, text: b.String()})
			continue
		}
		if probeCouplingIdentifierStart(ch) {
			start := i
			i++
			for i < len(src) && probeCouplingIdentifierPart(src[i]) {
				i++
			}
			out = append(out, probeCouplingToken{kind: probeCouplingIdentifier, text: src[start:i]})
			continue
		}
		out = append(out, probeCouplingToken{kind: probeCouplingPunct, text: string(ch)})
		i++
	}
	return out
}

func probeCouplingIdentifierStart(ch byte) bool {
	return ch == '_' || ch == '$' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func probeCouplingIdentifierPart(ch byte) bool {
	return probeCouplingIdentifierStart(ch) || ch >= '0' && ch <= '9'
}
