package index

import (
	"bytes"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// This deliberately covers only direct, terminated returns within the body
// the Cangjie parser actually consumed. Nested brace groups are opaque (they
// may be closures or control arms); no return inside them gets the outer name.
// The complete expression must also fit the bounded atom/member/call grammar
// below. This is syntax ownership, not type checking or execution proof.
// Expression-body/tail, operators, nested control, and unbounded statement
// continuation remain unavailable until the native parser owns their grammar.
func cangjieCallableReturns(source []byte, tokens []cangjieToken, sym types.Symbol) []types.CallableReturnExpression {
	if len(source) == 0 || len(tokens) < 2 || !sym.HasParserOwnedBody() {
		return nil
	}
	var out []types.CallableReturnExpression
	depth := 0
	for i := 1; i < len(tokens)-1; i++ {
		t := tokens[i]
		if t.Kind == cjTokLBrace {
			depth++
			continue
		}
		if t.Kind == cjTokRBrace {
			depth--
			continue
		}
		if depth != 0 || t.Text != "return" || (t.Kind != cjTokIdent && t.Kind != cjTokKeyword) || i+1 >= len(tokens)-1 {
			continue
		}
		first := tokens[i+1]
		if first.Line != t.Line {
			continue
		}
		var stack []cangjieTokKind
		last := -1
		terminated := false
	expressionLoop:
		for j := i + 1; j < len(tokens); j++ {
			n := tokens[j]
			if len(stack) == 0 && (n.Kind == cjTokSemicolon || (j == len(tokens)-1 && n.Kind == cjTokRBrace)) {
				terminated = last >= i+1
				break
			}
			if n.Kind == cjTokLBrace || n.Kind == cjTokRBrace || n.Kind == cjTokKeyword || n.Kind == cjTokEOF {
				break
			}
			// Without a complete statement grammar, a new top-level source
			// line is not silently joined to the current return expression.
			if len(stack) == 0 && n.Line != first.Line {
				break
			}
			switch n.Kind {
			case cjTokLParen:
				stack = append(stack, cjTokRParen)
			case cjTokLBracket:
				stack = append(stack, cjTokRBracket)
			case cjTokRParen, cjTokRBracket:
				if len(stack) == 0 || stack[len(stack)-1] != n.Kind {
					terminated = false
					last = -1
					break expressionLoop
				}
				stack = stack[:len(stack)-1]
			}
			last = j
		}
		if !terminated || len(stack) != 0 || last < i+1 {
			continue
		}
		if !cangjieReturnExpressionShape(tokens[i+1 : last+1]) {
			continue
		}
		start, end := first.Offset, tokens[last].Offset+len(tokens[last].Text)
		if start < 0 || end <= start || end > len(source) {
			continue
		}
		r := types.CallableReturnExpression{CallableName: sym.Name, CallableReceiver: sym.Receiver, CallableParent: sym.Parent, CallableLine: sym.Line, CallableEndLine: sym.EndLine,
			Kind: types.CallableReturnExplicit, Expression: string(source[start:end]), LineStart: first.Line, LineEnd: bytes.Count(source[:end-1], []byte{'\n'}) + 1, StartByte: uint32(start), EndByte: uint32(end), Provenance: types.ProvenanceCangjieParser, ResolvedBy: "cangjie_callable_return"}
		if r.MatchesCallable(sym) {
			out = append(out, r)
		}
	}
	return out
}

// A terminated token span is not by itself an expression: `1 garbage`,
// `x => "y"`, and `x = y` are not silently certified. This deliberately small
// grammar accepts identifiers, closed string/rune atoms, decimal integers,
// parenthesized expressions, explicit member access and nested call arguments.
// Every token must be consumed; unknown syntax retains no return receipt.
func cangjieReturnExpressionShape(tokens []cangjieToken) bool {
	if len(tokens) == 0 || len(tokens) > 256 {
		return false
	}
	pos := 0
	var expression func(int) bool
	expression = func(depth int) bool {
		if depth > 32 || pos >= len(tokens) {
			return false
		}
		token := tokens[pos]
		switch {
		case token.Kind == cjTokLiteral:
			pos++
		case cangjieReturnIdentifier(token):
			pos++
		case cangjieReturnDecimalDigit(token):
			// The native lexer splits digits. Only adjoining digit bytes form
			// one integer atom; spaces cannot turn `1 2` into `12`.
			pos++
			for pos < len(tokens) && cangjieReturnDecimalDigit(tokens[pos]) && tokens[pos].Offset == tokens[pos-1].Offset+len(tokens[pos-1].Text) {
				pos++
			}
		case token.Kind == cjTokLParen:
			pos++
			if !expression(depth+1) || pos >= len(tokens) || tokens[pos].Kind != cjTokRParen {
				return false
			}
			pos++
		default:
			return false
		}
		for pos < len(tokens) {
			switch tokens[pos].Kind {
			case cjTokDot:
				pos++
				if pos >= len(tokens) || !cangjieReturnIdentifier(tokens[pos]) {
					return false
				}
				pos++
			case cjTokLParen:
				pos++
				if pos < len(tokens) && tokens[pos].Kind == cjTokRParen {
					pos++
					continue
				}
				for {
					if !expression(depth+1) || pos >= len(tokens) {
						return false
					}
					if tokens[pos].Kind == cjTokRParen {
						pos++
						break
					}
					if tokens[pos].Kind != cjTokComma {
						return false
					}
					pos++
				}
			default:
				return true
			}
		}
		return true
	}
	return expression(0) && pos == len(tokens)
}

func cangjieReturnDecimalDigit(token cangjieToken) bool {
	return token.Kind == cjTokOther && len(token.Text) == 1 && token.Text[0] >= '0' && token.Text[0] <= '9'
}

func cangjieReturnIdentifier(token cangjieToken) bool {
	if token.Kind != cjTokIdent || cangjieNonCallHead(token.Text) {
		return false
	}
	// The navigation lexer's deliberately small keyword table leaves these
	// control/operator words as ident tokens. They are not atoms in this
	// expression subset. Match exact lexer tokens, never raw source substrings.
	switch token.Text {
	case "case", "do", "break", "continue", "finally", "in", "is":
		return false
	}
	return true
}
