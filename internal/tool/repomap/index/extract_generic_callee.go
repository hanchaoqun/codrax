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
// no arm ever publishes the instantiated spelling — a qualified callee is
// unwrapped through EVERY qualifier level to its terminal name
// (`std::chrono::duration_cast<std::chrono::milliseconds>(d)` publishes
// `duration_cast`, §40.59 收编复核三轮 #2). The one grammar ambiguity — a
// comparison chain `a < b && c > (d)` that tree-sitter also reads as a
// template call — is rejected by the typed discriminator
// cppTemplateReadingIsComparisonChain, not by a byte scan.

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

// cppNameKind is what a C++ identifier resolves to at a call site: the
// typed answer the comparison-chain discriminator reads. It is deliberately
// coarser than a symbol table — only the distinction that decides whether a
// `<` after the name can open a template-argument list is kept.
type cppNameKind int

const (
	// cppNameUnresolved: no declaration of the name is visible in this
	// translation unit (a header-declared entity, a macro, a using-imported
	// name) — the grammar's own reading stands.
	cppNameUnresolved cppNameKind = iota
	// cppNameValue: a runtime value — a function or lambda parameter, a
	// non-const local, a non-static or non-const data member, a non-const
	// namespace / file-scope variable, a range-for or structured binding, a
	// catch parameter, a condition-clause declaration. A runtime value can
	// never be a template argument and can never take template arguments.
	cppNameValue
	// cppNameConstant: a constant-capable value — a const / constexpr local
	// or global, a static const / constexpr data member, a non-type
	// template parameter, an enumerator. It cannot take template arguments
	// either, but it IS a legal non-type template argument, so it is no
	// witness against a template reading when it appears INSIDE one.
	cppNameConstant
	// cppNameCallable: a function, method or function template declared or
	// defined in this translation unit — the template reading of `f<…>(…)`
	// is the only reading.
	cppNameCallable
)

// cppTemplateReadingIsComparisonChain is the C++ precision discriminator
// for the one shape the grammar cannot decide: `a<b && c>(d)` and
// `a < b && c > (d)` both parse as call_expression{function:
// template_function{name: a, arguments: template_argument_list(
// binary_expression)}} although a real compiler reads them as a comparison
// chain whenever `a` names a value. The decisive typed signal is the CALLEE
// (§40.59 收编复核三轮 #0/#1): a name that resolves to a declared value —
// parameter or local of any enclosing function (through lambdas), a field
// of the enclosing class or of the class an out-of-line qualifier names, a
// namespace / file-scope variable, a range binding, a non-type template
// parameter — can never take template arguments, so the reading is a
// comparison (or comma) expression whatever the interior spells, and no arm
// mints a row. A name that resolves to a declared callable keeps the
// template reading unconditionally.
//
// When the callee is unresolvable in this translation unit the template
// reading stands, except on two typed witnesses that make it impossible:
//   - an interior operand identifier spells the callee's own name
//     (`x < lo || x > (hi)`, the range-check idiom): one name cannot be
//     both the template and a value operand of its own argument list;
//   - the template_argument_list holds exactly one binary_expression whose
//     top-level operator is `&&` / `||` and whose BOTH operands are integer
//     literals or declared RUNTIME values (cppNameValue): a runtime value is
//     never a constant expression, so it cannot be a template argument.
//     Constant-capable operands (constexpr / static const / non-type
//     template parameters) are legal bool non-type arguments and keep the
//     template reading: `dispatch<kFast && kSafe>(x)` mints `dispatch`.
//
// fn is the raw callee of a call_expression before genericCalleeBase: a
// template_function, or a qualified_identifier chain of any depth whose
// terminal is one (`ns::inner::a<b && c>(d)`); a template_method behind
// `.`/`->`/`template` can never be a comparison chain and is never
// consulted. resolve reports what an identifier node denotes at its own
// position (see cppScopeResolver.resolve).
func cppTemplateReadingIsComparisonChain(fn *sitter.Node, src []byte, resolve func(ident *sitter.Node) cppNameKind) bool {
	terminal, _ := cppQualifiedTerminal(fn)
	if terminal == nil || terminal.Type() != "template_function" {
		return false
	}
	callee := terminal.ChildByFieldName("name")
	if callee == nil || callee.Type() != "identifier" || resolve == nil {
		return false
	}
	switch resolve(callee) {
	case cppNameValue, cppNameConstant:
		return true
	case cppNameCallable:
		return false
	}
	args := terminal.ChildByFieldName("arguments")
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
	calleeName := strings.TrimSpace(nodeText(callee, src))
	left, right := chain.ChildByFieldName("left"), chain.ChildByFieldName("right")
	for _, operand := range []*sitter.Node{left, right} {
		if operand != nil && operand.Type() == "identifier" && strings.TrimSpace(nodeText(operand, src)) == calleeName {
			return true
		}
	}
	runtimeValueShaped := func(operand *sitter.Node) bool {
		if operand == nil {
			return false
		}
		switch operand.Type() {
		case "number_literal":
			return true
		case "identifier":
			return resolve(operand) == cppNameValue
		}
		return false
	}
	return runtimeValueShaped(left) && runtimeValueShaped(right)
}

// cppScopeResolver resolves what an identifier denotes at its own position
// in one C++ translation unit, following the lexical scopes a compiler
// consults, innermost first: block locals and for / condition-clause
// declarations that precede the use, lambda parameters, the enclosing
// function's parameters, the class an in-class member or an out-of-line
// qualifier (`bool Reader::Check() {`) names — its own members and those of
// its in-file base classes —, non-type template parameters of enclosing
// template declarations, the enclosing namespaces' and the file's
// declarations that precede the use (preprocessor branches and `extern
// "C"` blocks included). A qualified name (`ns::a`, `Q::a`) is looked up in
// the class or namespace its immediate qualifier names anywhere in the
// file. Anything the file does not declare is cppNameUnresolved — a
// header-declared class, a macro, a using-declaration; the resolver never
// guesses across files.
type cppScopeResolver struct {
	src        []byte
	root       *sitter.Node
	types      map[string][]*sitter.Node // class_specifier / struct_specifier / union_specifier by name
	namespaces map[string][]*sitter.Node // namespace_definition by name
}

func newCppScopeResolver(root *sitter.Node, src []byte) *cppScopeResolver {
	return &cppScopeResolver{src: src, root: root}
}

func (r *cppScopeResolver) indexNamedScopes() {
	if r.types != nil {
		return
	}
	r.types = map[string][]*sitter.Node{}
	r.namespaces = map[string][]*sitter.Node{}
	if r.root == nil {
		return
	}
	walkNamedChildren(r.root, true, func(node *sitter.Node) {
		switch node.Type() {
		case "class_specifier", "struct_specifier", "union_specifier":
			if name := node.ChildByFieldName("name"); name != nil && node.ChildByFieldName("body") != nil {
				key := cppScopeSegmentName(name, r.src)
				r.types[key] = append(r.types[key], node)
			}
		case "namespace_definition":
			if name := node.ChildByFieldName("name"); name != nil {
				key := strings.TrimSpace(nodeText(name, r.src))
				r.namespaces[key] = append(r.namespaces[key], node)
			}
		}
	})
}

// cppScopeSegmentName is the bare name a scope segment spells:
// `Parser<T>` → `Parser`, `ns` → `ns`, `Reader` → `Reader`.
func cppScopeSegmentName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() == "template_type" {
		if name := node.ChildByFieldName("name"); name != nil {
			node = name
		}
	}
	if node.Type() == "qualified_identifier" {
		terminal, _ := cppQualifiedTerminal(node)
		return cppScopeSegmentName(terminal, src)
	}
	return strings.TrimSpace(nodeText(node, src))
}

// resolve reports what the identifier node denotes. A callee identifier
// inside a qualified chain (`ns::a<…>`) is resolved in the scope its
// immediate qualifier names; any other identifier is resolved from its own
// lexical position outward.
func (r *cppScopeResolver) resolve(ident *sitter.Node) cppNameKind {
	if r == nil || ident == nil {
		return cppNameUnresolved
	}
	name := strings.TrimSpace(nodeText(ident, r.src))
	if name == "" {
		return cppNameUnresolved
	}
	if scope := cppCalleeQualifier(ident); scope != nil {
		return r.resolveQualified(cppScopeSegmentName(scope, r.src), name)
	}
	return r.resolveUnqualified(ident, name)
}

// cppCalleeQualifier returns the scope node that immediately qualifies a
// callee identifier (`ns::a<b>` → `ns`; `a<b>` → nil). The identifier is
// the `name` of a template_function (or the callee itself) whose parent is
// a qualified_identifier naming it.
func cppCalleeQualifier(ident *sitter.Node) *sitter.Node {
	node := ident
	if parent := node.Parent(); parent != nil && (parent.Type() == "template_function") {
		node = parent
	}
	parent := node.Parent()
	if parent == nil || (parent.Type() != "qualified_identifier" && parent.Type() != "scoped_identifier") {
		return nil
	}
	if name := parent.ChildByFieldName("name"); name == nil || !name.Equal(node) {
		return nil
	}
	return parent.ChildByFieldName("scope")
}

func (r *cppScopeResolver) resolveQualified(scopeName, name string) cppNameKind {
	if scopeName == "" {
		return cppNameUnresolved
	}
	r.indexNamedScopes()
	visited := map[string]bool{}
	for _, class := range r.types[scopeName] {
		if kind := r.lookupClassMember(class, name, visited); kind != cppNameUnresolved {
			return kind
		}
	}
	for _, namespace := range r.namespaces[scopeName] {
		if kind := r.lookupDeclarationList(namespace.ChildByFieldName("body"), name, 0); kind != cppNameUnresolved {
			return kind
		}
	}
	return cppNameUnresolved
}

func (r *cppScopeResolver) resolveUnqualified(ident *sitter.Node, name string) cppNameKind {
	use := ident.StartByte()
	for current := ident.Parent(); current != nil; current = current.Parent() {
		var kind cppNameKind
		switch current.Type() {
		case "compound_statement":
			kind = r.lookupDeclarationList(current, name, use)
		case "for_statement":
			kind = r.lookupDeclaration(current.ChildByFieldName("initializer"), name)
		case "for_range_loop":
			kind = cppDeclaratorKind(current.ChildByFieldName("declarator"), name, r.src, cppNameValue)
		case "if_statement", "while_statement", "switch_statement":
			kind = r.lookupConditionClause(current.ChildByFieldName("condition"), name)
		case "catch_clause":
			kind = r.lookupParameterList(current.ChildByFieldName("parameters"), name)
		case "lambda_expression":
			if declarator := current.ChildByFieldName("declarator"); declarator != nil {
				kind = r.lookupParameterList(declarator.ChildByFieldName("parameters"), name)
			}
		case "function_definition":
			kind = r.lookupFunctionScope(current, name)
		case "template_declaration":
			kind = r.lookupTemplateParameters(current.ChildByFieldName("parameters"), name)
		case "class_specifier", "struct_specifier", "union_specifier":
			kind = r.lookupClassMember(current, name, map[string]bool{})
		case "namespace_definition":
			kind = r.lookupDeclarationList(current.ChildByFieldName("body"), name, use)
		case "translation_unit":
			kind = r.lookupDeclarationList(current, name, use)
		}
		if kind != cppNameUnresolved {
			return kind
		}
	}
	return cppNameUnresolved
}

// lookupFunctionScope resolves name against a function definition: its
// parameters first, then — for an out-of-line member (`bool Reader::Check()
// {`, `void ns::Reader::Check() {`) — the members of the class the
// qualifier names, resolved in this file.
func (r *cppScopeResolver) lookupFunctionScope(function *sitter.Node, name string) cppNameKind {
	declarator := function.ChildByFieldName("declarator")
	for declarator != nil && declarator.Type() != "function_declarator" {
		next := declarator.ChildByFieldName("declarator")
		if next == nil {
			next = childByType(declarator, "function_declarator")
		}
		declarator = next
	}
	if declarator == nil {
		return cppNameUnresolved
	}
	if kind := r.lookupParameterList(declarator.ChildByFieldName("parameters"), name); kind != cppNameUnresolved {
		return kind
	}
	if _, scope := cppQualifiedTerminal(declarator.ChildByFieldName("declarator")); scope != nil {
		r.indexNamedScopes()
		visited := map[string]bool{}
		for _, class := range r.types[cppScopeSegmentName(scope, r.src)] {
			if kind := r.lookupClassMember(class, name, visited); kind != cppNameUnresolved {
				return kind
			}
		}
	}
	return cppNameUnresolved
}

func (r *cppScopeResolver) lookupParameterList(parameters *sitter.Node, name string) cppNameKind {
	if parameters == nil {
		return cppNameUnresolved
	}
	for i := 0; i < int(parameters.NamedChildCount()); i++ {
		parameter := parameters.NamedChild(i)
		switch parameter.Type() {
		case "parameter_declaration", "optional_parameter_declaration", "variadic_parameter_declaration":
			if kind := cppDeclaratorKind(parameter.ChildByFieldName("declarator"), name, r.src, cppNameValue); kind != cppNameUnresolved {
				return kind
			}
		}
	}
	return cppNameUnresolved
}

// lookupTemplateParameters resolves a non-type template parameter
// (`template<bool F>`: a parameter_declaration) as a constant; type
// parameters (`typename T`) denote types, not values, and stay unresolved.
func (r *cppScopeResolver) lookupTemplateParameters(parameters *sitter.Node, name string) cppNameKind {
	if parameters == nil {
		return cppNameUnresolved
	}
	for i := 0; i < int(parameters.NamedChildCount()); i++ {
		parameter := parameters.NamedChild(i)
		switch parameter.Type() {
		case "parameter_declaration", "optional_parameter_declaration", "variadic_parameter_declaration":
			if kind := cppDeclaratorKind(parameter.ChildByFieldName("declarator"), name, r.src, cppNameConstant); kind != cppNameUnresolved {
				return kind
			}
		}
	}
	return cppNameUnresolved
}

func (r *cppScopeResolver) lookupConditionClause(condition *sitter.Node, name string) cppNameKind {
	if condition == nil {
		return cppNameUnresolved
	}
	if kind := r.lookupDeclaration(condition.ChildByFieldName("value"), name); kind != cppNameUnresolved {
		return kind
	}
	if initializer := condition.ChildByFieldName("initializer"); initializer != nil {
		for i := 0; i < int(initializer.NamedChildCount()); i++ {
			if kind := r.lookupDeclaration(initializer.NamedChild(i), name); kind != cppNameUnresolved {
				return kind
			}
		}
	}
	return cppNameUnresolved
}

// lookupDeclarationList resolves name among the declarations a block,
// namespace body or translation unit holds directly (descending only into
// preprocessor branches and linkage specifications, which do not open a
// scope). A block or file-level declaration is visible only when it starts
// before the use (use == 0 disables the order check for named-scope
// lookups, where every member is visible).
func (r *cppScopeResolver) lookupDeclarationList(list *sitter.Node, name string, use uint32) cppNameKind {
	if list == nil {
		return cppNameUnresolved
	}
	for i := 0; i < int(list.NamedChildCount()); i++ {
		child := list.NamedChild(i)
		if use > 0 && child.StartByte() >= use {
			continue
		}
		var kind cppNameKind
		switch child.Type() {
		case "declaration", "function_definition", "template_declaration", "enum_specifier":
			kind = r.lookupDeclaration(child, name)
		case "preproc_if", "preproc_ifdef", "preproc_ifndef", "preproc_elif", "preproc_else", "preproc_elifdef", "declaration_list":
			kind = r.lookupDeclarationList(child, name, use)
		case "linkage_specification":
			// `extern "C" { … }` carries a declaration_list; `extern "C" int
			// f();` carries the one declaration directly.
			if body := child.ChildByFieldName("body"); body != nil && body.Type() == "declaration_list" {
				kind = r.lookupDeclarationList(body, name, use)
			} else {
				kind = r.lookupDeclaration(body, name)
			}
		}
		if kind != cppNameUnresolved {
			return kind
		}
	}
	return cppNameUnresolved
}

// lookupDeclaration resolves name against one declaration node: a
// variable declaration (const / constexpr → constant, otherwise a runtime
// value), a function prototype or definition (callable), a template
// declaration (whatever it wraps), an enum (its enumerators are constants).
func (r *cppScopeResolver) lookupDeclaration(node *sitter.Node, name string) cppNameKind {
	if node == nil {
		return cppNameUnresolved
	}
	switch node.Type() {
	case "template_declaration":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "template_parameter_list" {
				continue
			}
			if kind := r.lookupDeclaration(child, name); kind != cppNameUnresolved {
				return kind
			}
		}
		return cppNameUnresolved
	case "function_definition":
		declarator := node.ChildByFieldName("declarator")
		if cppDeclaratorDeclaresCallable(declarator) && cppDeclaratorName(declarator, r.src) == name {
			return cppNameCallable
		}
		return cppNameUnresolved
	case "enum_specifier":
		body := node.ChildByFieldName("body")
		if body == nil {
			return cppNameUnresolved
		}
		for i := 0; i < int(body.NamedChildCount()); i++ {
			enumerator := body.NamedChild(i)
			if enumerator.Type() == "enumerator" && strings.TrimSpace(nodeText(enumerator.ChildByFieldName("name"), r.src)) == name {
				return cppNameConstant
			}
		}
		return cppNameUnresolved
	case "declaration", "field_declaration":
		valueKind := cppNameValue
		if cppDeclarationIsConstantCapable(node, r.src) {
			valueKind = cppNameConstant
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if cppDeclaratorDeclaresCallable(child) {
				if cppDeclaratorName(child, r.src) == name {
					return cppNameCallable
				}
				continue
			}
			if kind := cppDeclaratorKind(child, name, r.src, valueKind); kind != cppNameUnresolved {
				return kind
			}
		}
	}
	return cppNameUnresolved
}

// lookupClassMember resolves name among a class body's members and, in
// this file, its base classes (visited guards cycles).
func (r *cppScopeResolver) lookupClassMember(class *sitter.Node, name string, visited map[string]bool) cppNameKind {
	if class == nil {
		return cppNameUnresolved
	}
	className := cppScopeSegmentName(class.ChildByFieldName("name"), r.src)
	if className != "" {
		if visited[className] {
			return cppNameUnresolved
		}
		visited[className] = true
	}
	body := class.ChildByFieldName("body")
	if body == nil {
		return cppNameUnresolved
	}
	for i := 0; i < int(body.NamedChildCount()); i++ {
		member := body.NamedChild(i)
		var kind cppNameKind
		switch member.Type() {
		case "field_declaration", "declaration", "function_definition", "template_declaration", "enum_specifier":
			kind = r.lookupDeclaration(member, name)
		case "preproc_if", "preproc_ifdef", "preproc_ifndef", "preproc_elif", "preproc_else", "preproc_elifdef":
			kind = r.lookupClassBodyBranch(member, name)
		}
		if kind != cppNameUnresolved {
			return kind
		}
	}
	if bases := childByType(class, "base_class_clause"); bases != nil {
		r.indexNamedScopes()
		for i := 0; i < int(bases.NamedChildCount()); i++ {
			base := bases.NamedChild(i)
			switch base.Type() {
			case "type_identifier", "qualified_identifier", "template_type":
				for _, baseClass := range r.types[cppScopeSegmentName(base, r.src)] {
					if kind := r.lookupClassMember(baseClass, name, visited); kind != cppNameUnresolved {
						return kind
					}
				}
			}
		}
	}
	return cppNameUnresolved
}

func (r *cppScopeResolver) lookupClassBodyBranch(branch *sitter.Node, name string) cppNameKind {
	for i := 0; i < int(branch.NamedChildCount()); i++ {
		member := branch.NamedChild(i)
		var kind cppNameKind
		switch member.Type() {
		case "field_declaration", "declaration", "function_definition", "template_declaration", "enum_specifier":
			kind = r.lookupDeclaration(member, name)
		case "preproc_if", "preproc_ifdef", "preproc_ifndef", "preproc_elif", "preproc_else", "preproc_elifdef":
			kind = r.lookupClassBodyBranch(member, name)
		}
		if kind != cppNameUnresolved {
			return kind
		}
	}
	return cppNameUnresolved
}

// cppDeclarationIsConstantCapable reports whether a declaration can name a
// constant expression: a const / constexpr / constinit local or global, a
// STATIC const / constexpr data member. A non-static data member is never
// a constant expression whatever its qualifiers.
func cppDeclarationIsConstantCapable(node *sitter.Node, src []byte) bool {
	static, constant := false, false
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "storage_class_specifier":
			if strings.TrimSpace(nodeText(child, src)) == "static" {
				static = true
			}
		case "type_qualifier":
			switch strings.TrimSpace(nodeText(child, src)) {
			case "const", "constexpr", "constinit", "consteval":
				constant = true
			}
		}
	}
	if node.Type() == "field_declaration" {
		return static && constant
	}
	return constant
}

// cppDeclaratorDeclaresCallable reports whether a declarator declares a
// function (`f(int)`, `Reader::Check()`, `*f(int)` returning a pointer)
// rather than a function pointer variable (`(*fp)(int)`), which is a value.
func cppDeclaratorDeclaresCallable(declarator *sitter.Node) bool {
	for declarator != nil {
		switch declarator.Type() {
		case "function_declarator":
			inner := declarator.ChildByFieldName("declarator")
			return inner != nil && inner.Type() != "parenthesized_declarator"
		case "pointer_declarator", "reference_declarator", "init_declarator":
			next := declarator.ChildByFieldName("declarator")
			if next == nil && declarator.NamedChildCount() > 0 {
				next = declarator.NamedChild(0)
			}
			declarator = next
		default:
			return false
		}
	}
	return false
}

// cppDeclaratorName is the terminal name a declarator declares, with any
// qualifier chain stripped (`Reader::Check` → `Check`).
func cppDeclaratorName(declarator *sitter.Node, src []byte) string {
	for declarator != nil {
		switch declarator.Type() {
		case "function_declarator", "pointer_declarator", "reference_declarator", "init_declarator", "array_declarator", "parenthesized_declarator":
			next := declarator.ChildByFieldName("declarator")
			if next == nil && declarator.NamedChildCount() > 0 {
				next = declarator.NamedChild(0)
			}
			declarator = next
		case "qualified_identifier", "scoped_identifier":
			terminal, _ := cppQualifiedTerminal(declarator)
			declarator = terminal
		case "template_function", "template_method":
			declarator = declarator.ChildByFieldName("name")
		case "identifier", "field_identifier", "destructor_name", "operator_name":
			return strings.TrimSpace(nodeText(declarator, src))
		default:
			return ""
		}
	}
	return ""
}

// cppDeclaratorKind reports kind when the declarator declares name — one
// plain declarator, or every identifier of a structured binding
// (`auto& [k, v]`).
func cppDeclaratorKind(declarator *sitter.Node, name string, src []byte, kind cppNameKind) cppNameKind {
	if declarator == nil {
		return cppNameUnresolved
	}
	if declarator.Type() == "structured_binding_declarator" {
		for i := 0; i < int(declarator.NamedChildCount()); i++ {
			if strings.TrimSpace(nodeText(declarator.NamedChild(i), src)) == name {
				return kind
			}
		}
		return cppNameUnresolved
	}
	if declarator.Type() == "reference_declarator" || declarator.Type() == "pointer_declarator" {
		for i := 0; i < int(declarator.NamedChildCount()); i++ {
			if inner := declarator.NamedChild(i); inner.Type() == "structured_binding_declarator" {
				return cppDeclaratorKind(inner, name, src, kind)
			}
		}
	}
	if cppDeclaratorName(declarator, src) == name {
		return kind
	}
	return cppNameUnresolved
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
