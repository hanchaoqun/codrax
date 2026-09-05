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
// the symbol table agree on one spelling. The tree-sitter reading is the
// precise witness on this tier: every template_function / template_method
// the grammar produces is unwrapped to its bare name, whatever the
// template-argument interior spells (a type, a negative or arithmetic
// non-type argument, an rvalue reference, a string literal, a lambda), and
// no arm ever publishes the instantiated spelling — a qualified callee is
// unwrapped through EVERY qualifier level to its terminal name
// (`std::chrono::duration_cast<std::chrono::milliseconds>(d)` publishes
// `duration_cast`, §40.59 收编复核三轮 #2). The one grammar ambiguity — a
// comparison chain `a < b && c > (d)` that tree-sitter also reads as a
// template call — is rejected by the typed discriminator
// cppTemplateReadingIsComparisonChain, keyed on the grammar shape alone
// (§40.59 收编复核四轮), not by a byte scan and not by name resolution.

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
		if inner := fn.ChildByFieldName("name"); inner != nil {
			return inner
		}
	case "dependent_name":
		if fn.NamedChildCount() > 0 {
			return genericCalleeBase(fn.NamedChild(0))
		}
	}
	return fn
}

// cppQualifiedTerminal unwraps a C/C++ qualified callee
// (`qualified_identifier` / `scoped_identifier` chains of any depth,
// `std::chrono::duration_cast<…>`) to the terminal node the chain names and
// the scope node that immediately qualifies it (`chrono`); a callee that is
// not qualified is returned unchanged with a nil scope. The immediate scope
// is the receiver the symbol table keys on (MethodIndex keys C++ methods by
// their Parent class), so `ns::Reader::Check(x)` resolves through `Reader`
// exactly like `Reader::Check(x)` does.
func cppQualifiedTerminal(fn *sitter.Node) (terminal, scope *sitter.Node) {
	terminal = fn
	for terminal != nil && (terminal.Type() == "qualified_identifier" || terminal.Type() == "scoped_identifier") {
		name := terminal.ChildByFieldName("name")
		if name == nil {
			return nil, nil
		}
		scope = terminal.ChildByFieldName("scope")
		terminal = name
	}
	return terminal, scope
}

// cppTemplateReadingIsComparisonChain is the C++ precision discriminator
// for the one shape the grammar cannot decide: `a<b && c>(d)` and
// `a < b && c > (d)` both parse as call_expression{function:
// template_function{name: a, arguments: template_argument_list(
// binary_expression)}} although a real compiler reads them as a comparison
// chain whenever `a` names a value. The precise signal is the grammar shape
// itself (§40.59 收编复核四轮): the callee is a template_function — bare, or
// the terminal of a qualified chain of any depth (`ns::inner::a<b && c>(d)`)
// — whose template_argument_list holds exactly one top-level
// binary_expression whose operator is `&&` or `||`. That exact shape mints
// no call row in any arm, whatever the operands spell (`x < lo || x > (hi)`,
// `field_number_ < 1 || field_number_ > (1 << 29) - 1`, `a<b && c || d>(e)`,
// `x < n + 1 && y > (m)`).
//
// Deciding instead whether the callee NAMES a value would re-implement C++
// name lookup inside the extractor — most-vexing-parse locals (`Timer
// t(clock);`), anonymous and inline namespaces, same-named classes in
// sibling namespaces, lambda init-captures, declarations under a case
// label, header-only classes, using-declarations, macros — an unbounded,
// noisy signal, which the "precise signals for hard gates" red line forbids
// under a row-minting gate. The one disclosed residual of the shape rule is
// the legitimate constexpr-bool non-type argument spelled as a single `&&`
// / `||` expression (`dispatch<kFast && kSafe>(x)`, `f<true && flag>(x)`,
// `f<A && B>(x)` over non-type template parameters): it is the same grammar
// shape and mints no row either; the parenthesised spelling `f<(a && b)>(x)`
// is a parenthesized_expression, not the ruled shape, and keeps its row.
// Every other template reading — a type, a comma list (`a<b, c>(d)` is a
// comma expression only when `a` is a value, which the extractor does not
// decide), `f<-1>`, `f<N + 1>`, `f<T&&>`, `f<a == b>`, a ternary, sizeof, a
// string literal, a lambda, a nested template — keeps its bare-name row with
// the immediate qualifier as receiver.
//
// fn is the raw callee of a call_expression before genericCalleeBase. A
// template_method behind `.` / `->` / `template` (`obj.f<a && b>(x)`) is
// never consulted: the grammar's reading stands there on every tree,
// disclosed.
func cppTemplateReadingIsComparisonChain(fn *sitter.Node, src []byte) bool {
	terminal, _ := cppQualifiedTerminal(fn)
	if terminal == nil || terminal.Type() != "template_function" {
		return false
	}
	args := terminal.ChildByFieldName("arguments")
	if args == nil {
		return false
	}
	var chain *sitter.Node
	for i := 0; i < int(args.NamedChildCount()); i++ {
		child := args.NamedChild(i)
		if child.Type() == "comment" {
			continue
		}
		if chain != nil {
			return false
		}
		chain = child
	}
	if chain == nil || chain.Type() != "binary_expression" {
		return false
	}
	operator := chain.ChildByFieldName("operator")
	if operator == nil {
		return false
	}
	switch nodeText(operator, src) {
	case "&&", "||":
		return true
	}
	return false
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
// yields no call row rather than a guessed one. The lexer has no AST
// witness, so unlike the C++ arm above it keeps the byte-shape guard; the
// grounder's regex arm (typeArgumentListClosesIntoCall) is on the same
// footing.
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
