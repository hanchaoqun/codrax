package index

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// extract_generic_callee.go — colleague B1554 (colleague_merge_audit
// §40.59): a callee that carries explicit type arguments is the SAME callee
// as its bare name. Before this file, the tree-sitter extractors either
// dropped the call row (Rust `parse::<u32>(s)` arrives wrapped in a
// `generic_function` node no switch arm accepted; Swift `Box<Int>(x)` is a
// `constructor_expression`, not a `call_expression`) or published the
// instantiated spelling as the endpoint name (C++ `std::make_unique<T>()`:
// the qualified_identifier's `name` child is a `template_function`, so
// ToEP.Name read "make_unique<ConsoleSink>"). The Cangjie token extractor
// required the identifier to be followed by `(` directly, so `f<T>(x)`
// produced no row. Grounding matches rel.ToEP.Name exactly, so every one of
// those shapes made a real call site ungroundable.
//
// One normaliser per grammar family, keyed on the closed list of node types
// the grammars use for "callee + type arguments" (verbatim node names, not a
// substring scan — adding a grammar means adding a row here):
//
//   - Rust      generic_function{function, type_arguments}
//   - C++       template_function{name, arguments}, template_method{name,
//     arguments}, dependent_name(template_method) for `x.template f<T>()`
//   - Swift     constructor_expression{constructed_type: user_type(
//     type_identifier, type_arguments?)} (handled by the navigation extractor;
//     the base is the user_type's own type_identifier)
//   - Kotlin / TypeScript / Java already keep the name and the type
//     arguments in sibling fields; they pass through unchanged.
//
// Type arguments are never part of the published name: the endpoint stays
// the bare callee identifier so the AST row, the regex grounding tier and
// the symbol table agree on one spelling.

// genericCalleeBase unwraps a callee node that carries explicit type
// arguments to the node that names the callee. Any other node is returned
// as is, so every extractor switch can feed its `function` child through
// this normaliser before dispatching on the node type.
func genericCalleeBase(fn *sitter.Node) *sitter.Node {
	if fn == nil {
		return nil
	}
	switch fn.Type() {
	case "generic_function":
		if inner := fn.ChildByFieldName("function"); inner != nil {
			return inner
		}
	case "template_function", "template_method":
		if inner := fn.ChildByFieldName("name"); inner != nil && cppTemplateCalleeTouchesArguments(fn) {
			return inner
		}
	case "dependent_name":
		if fn.NamedChildCount() > 0 {
			return genericCalleeBase(fn.NamedChild(0))
		}
	}
	return fn
}

// cppTemplateCalleeTouchesArguments is the C++ precision guard. The grammar
// cannot tell `a < b && c > (d)` (a comparison chain) from `a<b && c>(d)` (a
// template call) — real compilers decide by name lookup — and parses both
// as template_function. The one structural signal the AST still carries is
// byte adjacency: a written template call spells `name<` with the `<`
// touching the name and `>(` with the `(` touching the closing `>`. The
// same two adjacencies gate the Cangjie lexer arm and the grounder's regex
// arm, so all three tiers accept one shape. A template_function that fails
// the guard is returned unchanged and, as before this file, mints no row.
func cppTemplateCalleeTouchesArguments(fn *sitter.Node) bool {
	name := fn.ChildByFieldName("name")
	args := fn.ChildByFieldName("arguments")
	if name == nil || args == nil || name.EndByte() != args.StartByte() {
		return false
	}
	call := fn.Parent()
	for call != nil && call.Type() != "call_expression" {
		call = call.Parent()
	}
	if call == nil {
		return true
	}
	list := call.ChildByFieldName("arguments")
	return list != nil && list.StartByte() == fn.EndByte()
}

// cangjieCallParenIndex returns the index of the `(` token that opens the
// argument list of a call headed by tokens[nameIndex], or -1 when the
// identifier does not head a call. Two shapes are accepted: the plain
// `ident(` adjacency the extractor always had, and `ident<…>(` where the
// `<` touches the identifier, the angle brackets balance over type-argument
// tokens only, and the closing `>` touches the `(`. The two byte-adjacency
// requirements are the precise signal separating a type-argument list from
// a comparison chain (`a < b && c > (d)` never spells `>(` after a balanced
// `ident<`), and the interior whitelist (identifiers, keywords, `,` `.`
// `[` `]` `(` `)` `->` `?` and nested angles) rejects any operator that a
// type argument cannot contain, so an unbalanced or expression-bearing `<`
// yields no call row rather than a guessed one.
func cangjieCallParenIndex(tokens []cangjieToken, nameIndex int) int {
	if nameIndex < 0 || nameIndex+1 >= len(tokens) || tokens[nameIndex].Kind != cjTokIdent {
		return -1
	}
	next := tokens[nameIndex+1]
	if next.Kind == cjTokLParen {
		return nameIndex + 1
	}
	if next.Kind != cjTokLAngle || !cangjieTokensTouch(tokens[nameIndex], next) {
		return -1
	}
	depth := 0
	for j := nameIndex + 1; j < len(tokens); j++ {
		switch tokens[j].Kind {
		case cjTokLAngle:
			depth++
		case cjTokRAngle:
			depth--
			if depth == 0 {
				if j+1 < len(tokens) && tokens[j+1].Kind == cjTokLParen && cangjieTokensTouch(tokens[j], tokens[j+1]) {
					return j + 1
				}
				return -1
			}
		case cjTokIdent, cjTokKeyword, cjTokComma, cjTokDot, cjTokLBracket, cjTokRBracket, cjTokLParen, cjTokRParen, cjTokArrow:
		case cjTokOther:
			if tokens[j].Text != "?" {
				return -1
			}
		default:
			return -1
		}
	}
	return -1
}

func cangjieTokensTouch(left, right cangjieToken) bool {
	return left.Offset+len(left.Text) == right.Offset
}
