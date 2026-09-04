package index

import (
	"strings"

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
// no arm ever publishes the instantiated spelling (§40.59 收编复核再收编).
// The one grammar ambiguity — a comparison chain `a < b && c > (d)` that
// tree-sitter also reads as a template call — is rejected by the typed
// discriminator cppTemplateReadingIsComparisonChain, not by a byte scan.

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

// cppTemplateReadingIsComparisonChain is the C++ precision discriminator
// for the one shape the grammar cannot decide: `a<b && c>(d)` and
// `a < b && c > (d)` both parse as call_expression{function:
// template_function{name: a, arguments: template_argument_list(
// binary_expression)}} although a real compiler reads them as a comparison
// chain whenever `a` names a value. The discriminator fires only on the
// typed shape of that chain — the callee's template_argument_list holds
// exactly one binary_expression whose top-level operator is `&&` or `||`
// and whose BOTH operands are value-shaped: an integer literal
// (number_literal) or an identifier the enclosing function declares as a
// parameter or local (or the enclosing class declares as a field). Any
// other interior keeps tree-sitter's template reading, so a genuine
// `f<N - 1>(x)`, `f<-1>(x)`, `f<T&&>(x)`, `f<true && flag>(x)` or a bool
// non-type argument over undeclared names still mints its bare-name row.
//
// fn is the raw callee of a call_expression before genericCalleeBase: a
// template_function, or a qualified_identifier whose name child is one
// (`ns::a<b && c>(d)`); a template_method behind `.`/`->`/`template` can
// never be a comparison chain and is never consulted. declared reports
// whether an identifier is a declared value name at the call.
func cppTemplateReadingIsComparisonChain(fn *sitter.Node, src []byte, declared func(name string) bool) bool {
	if fn == nil {
		return false
	}
	if fn.Type() == "qualified_identifier" || fn.Type() == "scoped_identifier" {
		fn = fn.ChildByFieldName("name")
		if fn == nil {
			return false
		}
	}
	if fn.Type() != "template_function" {
		return false
	}
	args := fn.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() != 1 {
		return false
	}
	chain := args.NamedChild(0)
	if chain.Type() != "binary_expression" {
		return false
	}
	operator := chain.ChildByFieldName("operator")
	if operator == nil {
		return false
	}
	switch nodeText(operator, src) {
	case "&&", "||":
	default:
		return false
	}
	valueShaped := func(operand *sitter.Node) bool {
		if operand == nil {
			return false
		}
		switch operand.Type() {
		case "number_literal":
			return true
		case "identifier":
			return declared != nil && declared(strings.TrimSpace(nodeText(operand, src)))
		}
		return false
	}
	return valueShaped(chain.ChildByFieldName("left")) && valueShaped(chain.ChildByFieldName("right"))
}

// cppEnclosingValueNames collects the value names lexically visible at
// node for the comparison-chain discriminator: every parameter and local
// declarator of the nearest enclosing function body or lambda (for-loop
// initialisers and nested blocks included) and every field of the nearest
// enclosing class/struct. File-scope variables are deliberately absent —
// this is the same "declaration alone does not prove which object a name
// denotes" boundary cReceiverDeclarations draws.
func cppEnclosingValueNames(node *sitter.Node, src []byte) map[string]bool {
	names := map[string]bool{}
	var function, class *sitter.Node
	for current := node; current != nil; current = current.Parent() {
		switch current.Type() {
		case "function_definition", "lambda_expression":
			if function == nil {
				function = current
			}
		case "class_specifier", "struct_specifier":
			if class == nil {
				class = current
			}
		}
	}
	collect := func(root *sitter.Node, declarationTypes ...string) {
		if root == nil {
			return
		}
		walkNamedChildren(root, true, func(child *sitter.Node) {
			matched := false
			for _, declarationType := range declarationTypes {
				if child.Type() == declarationType {
					matched = true
					break
				}
			}
			if !matched {
				return
			}
			for i := 0; i < int(child.NamedChildCount()); i++ {
				if name := cDeclaratorName(child.NamedChild(i), src); name != "" {
					names[name] = true
				}
			}
		})
	}
	collect(function, "parameter_declaration", "optional_parameter_declaration", "declaration")
	collect(class, "field_declaration")
	return names
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
