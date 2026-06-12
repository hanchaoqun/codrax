package index

import (
	"fmt"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func extractGo(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "package_clause":
			if id := childByType(ch, "package_identifier"); id != nil {
				pkg = nodeText(id, src)
			}

		case "import_declaration":
			imps = append(imps, goExtractImports(ch, src, file)...)

		case "function_declaration":
			if s, ok := goExtractFunc(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "method_declaration":
			if s, ok := goExtractMethod(ch, src, file); ok {
				syms = append(syms, s)
			}

		case "type_declaration":
			syms = append(syms, goExtractTypes(ch, src, file)...)
			rels = append(rels, goExtractEmbeddings(ch, src, file)...)

		case "const_declaration", "var_declaration":
			syms = append(syms, goExtractVarConst(ch, src, file)...)
		}
	}

	// extract call relations
	rels = append(rels, goExtractCalls(root, src, file)...)
	// extract type references
	rels = append(rels, goExtractTypeRefs(root, src, file)...)

	return
}

func goExtractImports(node *sitter.Node, src []byte, file string) []types.Import {
	var imps []types.Import
	// tree-sitter-go places grouped imports (import ( ... )) under an
	// import_spec_list child, not directly under import_declaration.
	// Verified against codrax/internal/orchestrator/orchestrator.go via
	// eval/repomap_v3/probe_ast (2026-04-13). Always check the
	// spec_list first; fall back to direct import_spec children for
	// parser variants; fall back to the single-import literal last.
	specs := childrenByType(node, "import_spec")
	if len(specs) == 0 {
		if list := childByType(node, "import_spec_list"); list != nil {
			specs = childrenByType(list, "import_spec")
		}
	}
	if len(specs) == 0 {
		// single import: import "path"
		if lit := childByType(node, "interpreted_string_literal"); lit != nil {
			path := strings.Trim(nodeText(lit, src), `"`)
			imps = append(imps, types.Import{
				Raw:  nodeText(node, src),
				Path: path,
				File: file,
				Line: nodeLine(node),
			})
		}
		return imps
	}
	for _, spec := range specs {
		var alias string
		if name := spec.ChildByFieldName("name"); name != nil {
			alias = nodeText(name, src)
		}
		if pathNode := spec.ChildByFieldName("path"); pathNode != nil {
			path := strings.Trim(nodeText(pathNode, src), `"`)
			imps = append(imps, types.Import{
				Raw:   nodeText(spec, src),
				Path:  path,
				Alias: alias,
				File:  file,
				Line:  nodeLine(spec),
			})
		}
	}
	return imps
}

func goExtractFunc(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	sig := goFuncSignature(node, src)
	return types.Symbol{
		Name:            name,
		Kind:            "function",
		File:            file,
		Line:            nodeLine(node),
		EndLine:         nodeEndLine(node),
		Exported:        unicode.IsUpper(rune(name[0])),
		Signature:       sig,
		Arity:           goFuncArity(node),
		Doc:             prevSiblingComment(node, src),
		ReturnTypeNames: extractReturnTypeNames(node, src),
	}, true
}

func goExtractMethod(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)

	var receiver string
	if recvNode := node.ChildByFieldName("receiver"); recvNode != nil {
		// parameter_list → parameter_declaration → type
		for j := 0; j < int(recvNode.NamedChildCount()); j++ {
			param := recvNode.NamedChild(j)
			if param.Type() == "parameter_declaration" {
				if t := param.ChildByFieldName("type"); t != nil {
					receiver = nodeText(t, src)
					receiver = strings.TrimPrefix(receiver, "*")
				}
			}
		}
	}

	sig := goFuncSignature(node, src)
	return types.Symbol{
		Name:            name,
		Kind:            "method",
		File:            file,
		Line:            nodeLine(node),
		EndLine:         nodeEndLine(node),
		Exported:        unicode.IsUpper(rune(name[0])),
		Receiver:        receiver,
		Signature:       sig,
		Arity:           goFuncArity(node),
		Doc:             prevSiblingComment(node, src),
		ReturnTypeNames: extractReturnTypeNames(node, src),
	}, true
}

// goFuncArity counts the number of parameters in a function or
// method declaration. tree-sitter-go exposes the parameters as a
// `parameter_list` under the `parameters` field, containing
// `parameter_declaration` nodes. A single parameter_declaration can
// bind multiple names (`func f(a, b int)`) — we count each name,
// not each declaration, so the arity matches the positional call
// arity the Go runtime uses.
func goFuncArity(node *sitter.Node) int {
	params := node.ChildByFieldName("parameters")
	if params == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(params.NamedChildCount()); i++ {
		decl := params.NamedChild(i)
		if decl.Type() != "parameter_declaration" && decl.Type() != "variadic_parameter_declaration" {
			continue
		}
		// Count names under this declaration; fall back to 1 for
		// anonymous params (`func f(int)`).
		names := 0
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			ch := decl.NamedChild(j)
			if ch.Type() == "identifier" {
				names++
			}
		}
		if names == 0 {
			names = 1
		}
		count += names
	}
	return count
}

func goExtractTypes(node *sitter.Node, src []byte, file string) []types.Symbol {
	var syms []types.Symbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		spec := node.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		typeNode := spec.ChildByFieldName("type")
		kind := "type"
		if typeNode != nil {
			switch typeNode.Type() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
			}
		}
		sym := types.Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     nodeLine(spec),
			EndLine:  nodeEndLine(spec),
			Exported: unicode.IsUpper(rune(name[0])),
			Doc:      prevSiblingComment(node, src),
		}
		// Phase 6 P0 batch (2026-05-03) — populate RequiredMethods
		// for interface declarations so the populateImplementers
		// post-pass can match concrete types against this contract.
		if kind == "interface" && typeNode != nil {
			sym.RequiredMethods = goExtractInterfaceMethods(typeNode, src)
		}
		syms = append(syms, sym)
		if kind == "struct" && typeNode != nil {
			syms = append(syms, goExtractStructFields(name, typeNode, src, file)...)
		}
	}
	return syms
}

func goExtractStructFields(typeName string, structNode *sitter.Node, src []byte, file string) []types.Symbol {
	if structNode == nil || structNode.Type() != "struct_type" {
		return nil
	}
	body := childByType(structNode, "field_declaration_list")
	if body == nil {
		return nil
	}
	var syms []types.Symbol
	for i := 0; i < int(body.NamedChildCount()); i++ {
		field := body.NamedChild(i)
		if field == nil || field.Type() != "field_declaration" {
			continue
		}
		nameNodes := childrenByType(field, "field_identifier")
		if len(nameNodes) == 0 {
			if nameNode := field.ChildByFieldName("name"); nameNode != nil && nameNode.Type() == "field_identifier" {
				nameNodes = append(nameNodes, nameNode)
			}
		}
		if len(nameNodes) == 0 {
			continue
		}
		signature := strings.TrimSpace(nodeText(field, src))
		typeText := ""
		if typeNode := field.ChildByFieldName("type"); typeNode != nil {
			typeText = strings.TrimSpace(nodeText(typeNode, src))
		}
		fieldSignature := typeText
		if fieldSignature == "" {
			fieldSignature = signature
		}
		for _, nameNode := range nameNodes {
			name := strings.TrimSpace(nodeText(nameNode, src))
			if name == "" || name == "_" {
				continue
			}
			syms = append(syms, types.Symbol{
				Name:      name,
				Kind:      "field",
				File:      file,
				Line:      nodeLine(field),
				EndLine:   nodeEndLine(field),
				Exported:  unicode.IsUpper(rune(name[0])),
				Parent:    typeName,
				Signature: fieldSignature,
				Doc:       prevSiblingComment(field, src),
			})
		}
	}
	return syms
}

// goExtractInterfaceMethods walks an `interface_type` node body
// and returns the deduplicated set of "name(arity)" method specs.
// Embedded interface references (e.g. `io.Reader` inside another
// interface) are not expanded here — the post-pass treats embeds
// as a separate transitive case if needed.
//
// tree-sitter-go grammar (verified): interface_type contains
// `method_elem` children directly (no method_spec_list wrapper).
// Each method_elem carries a `field_identifier` for the method
// name + a `parameter_list`; arity is the comma count + 1.
// Older grammar versions used `method_spec` — both names accepted
// for cross-version safety.
func goExtractInterfaceMethods(ifaceNode *sitter.Node, src []byte) []string {
	if ifaceNode == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	collect := func(spec *sitter.Node) {
		if spec == nil {
			return
		}
		// Try field-name first (newer grammars), fall back to
		// child-by-type for `field_identifier` (the actual method
		// name node in the current tree-sitter-go grammar).
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = childByType(spec, "field_identifier")
		}
		if nameNode == nil {
			return
		}
		mname := nodeText(nameNode, src)
		// Arity from parameter_list: count parameter_declaration
		// children. goFuncArity is tuned for method/function
		// declarations whose parameters live under a `parameters`
		// field; method_elem instead has a direct `parameter_list`
		// child, so use a local helper.
		arity := 0
		if pl := childByType(spec, "parameter_list"); pl != nil {
			for j := 0; j < int(pl.NamedChildCount()); j++ {
				if pl.NamedChild(j).Type() == "parameter_declaration" {
					arity++
				}
			}
		}
		key := fmt.Sprintf("%s(%d)", mname, arity)
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	for i := 0; i < int(ifaceNode.NamedChildCount()); i++ {
		spec := ifaceNode.NamedChild(i)
		if spec == nil {
			continue
		}
		switch spec.Type() {
		case "method_elem", "method_spec":
			collect(spec)
		case "method_spec_list":
			// Older grammar: walk the wrapper.
			for j := 0; j < int(spec.NamedChildCount()); j++ {
				inner := spec.NamedChild(j)
				if inner == nil {
					continue
				}
				switch inner.Type() {
				case "method_elem", "method_spec":
					collect(inner)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func goExtractVarConst(node *sitter.Node, src []byte, file string) []types.Symbol {
	kind := "var"
	if node.Type() == "const_declaration" {
		kind = "const"
	}
	var syms []types.Symbol
	// specs are const_spec or var_spec
	for i := 0; i < int(node.NamedChildCount()); i++ {
		spec := node.NamedChild(i)
		if spec.Type() != "const_spec" && spec.Type() != "var_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     nodeLine(spec),
			EndLine:  nodeEndLine(spec),
			Exported: unicode.IsUpper(rune(name[0])),
		})
	}
	return syms
}

func goExtractEmbeddings(typeDecl *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	for i := 0; i < int(typeDecl.NamedChildCount()); i++ {
		spec := typeDecl.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		hostName := nodeText(nameNode, src)

		typeNode := spec.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}

		switch typeNode.Type() {
		case "struct_type":
			if body := childByType(typeNode, "field_declaration_list"); body != nil {
				for j := 0; j < int(body.NamedChildCount()); j++ {
					field := body.NamedChild(j)
					if field.Type() != "field_declaration" {
						continue
					}
					// embedded field: no name, just type
					if field.ChildByFieldName("name") == nil {
						if t := field.ChildByFieldName("type"); t != nil {
							embedded := nodeText(t, src)
							embedded = strings.TrimPrefix(embedded, "*")
							rels = append(rels, types.Relation{
								Kind:       "embedding",
								FromEP:     types.RelationEndpoint{Name: hostName, File: file, Line: nodeLine(field)},
								ToEP:       types.RelationEndpoint{Name: embedded, File: file, Line: nodeLine(field)},
								File:       file,
								Line:       nodeLine(field),
								Confidence: types.ConfidenceAST,
								Provenance: types.ProvenanceTreeSitter,
								ResolvedBy: "go_struct_embedding",
							})
						}
					}
				}
			}
		case "interface_type":
			// interface embedding
			if body := childByType(typeNode, "method_spec_list"); body != nil {
				for j := 0; j < int(body.NamedChildCount()); j++ {
					ch := body.NamedChild(j)
					if ch.Type() == "type_identifier" || ch.Type() == "qualified_type" {
						embedded := nodeText(ch, src)
						rels = append(rels, types.Relation{
							Kind:       "inheritance",
							FromEP:     types.RelationEndpoint{Name: hostName, File: file, Line: nodeLine(ch)},
							ToEP:       types.RelationEndpoint{Name: embedded, File: file, Line: nodeLine(ch)},
							File:       file,
							Line:       nodeLine(ch),
							Confidence: types.ConfidenceAST,
							Provenance: types.ProvenanceTreeSitter,
							ResolvedBy: "go_interface_embedding",
						})
					}
				}
			}
		}
	}
	return rels
}

// goExtractCalls walks the file tree looking for call_expression
// nodes. It builds a function-scoped local scope — receiver name
// plus parameter names, each bound to their declared type — so
// that when a selector_expression call's receiver text matches a
// local binding, the Phase 1 receiver-aware resolver in
// `computeCallAmbiguity` (and P1.2b's rewritten CallersOf) can
// attribute the call to a specific type instead of guessing from
// the bare method name.
//
// Walking strategy: iterate top-level function_declaration and
// method_declaration nodes, derive their local scope, then walk
// each function's body for call sites using that scope. Calls at
// package scope (init blocks, top-level var initializers) use an
// empty scope and contribute only unqualified receivers.
func goExtractCalls(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	for i := 0; i < int(root.NamedChildCount()); i++ {
		top := root.NamedChild(i)
		switch top.Type() {
		case "function_declaration", "method_declaration":
			scope := goLocalScope(top, src)
			body := top.ChildByFieldName("body")
			if body == nil {
				continue
			}
			goWalkCalls(body, src, file, scope, &rels)
		default:
			// Top-level var/const initializers can contain calls too.
			goWalkCalls(top, src, file, nil, &rels)
		}
	}
	return rels
}

// goLocalScope binds every parameter and (for methods) the receiver
// to its declared type for a single function. Returns a map keyed
// by identifier name; values are the cleaned type names with
// pointer/slice/array prefixes stripped. A nil map is returned for
// functions that take no params and have no receiver.
func goLocalScope(fnNode *sitter.Node, src []byte) map[string]string {
	scope := make(map[string]string)
	// Receiver for methods: field "receiver" → parameter_list →
	// parameter_declaration whose names bind to the receiver type.
	if recv := fnNode.ChildByFieldName("receiver"); recv != nil {
		for j := 0; j < int(recv.NamedChildCount()); j++ {
			pd := recv.NamedChild(j)
			if pd.Type() != "parameter_declaration" {
				continue
			}
			typeName := goCleanTypeName(pd.ChildByFieldName("type"), src)
			for k := 0; k < int(pd.NamedChildCount()); k++ {
				ch := pd.NamedChild(k)
				if ch.Type() == "identifier" {
					scope[nodeText(ch, src)] = typeName
				}
			}
		}
	}
	// Parameters: field "parameters" → parameter_list → parameter_declaration.
	if params := fnNode.ChildByFieldName("parameters"); params != nil {
		for j := 0; j < int(params.NamedChildCount()); j++ {
			pd := params.NamedChild(j)
			if pd.Type() != "parameter_declaration" && pd.Type() != "variadic_parameter_declaration" {
				continue
			}
			typeName := goCleanTypeName(pd.ChildByFieldName("type"), src)
			for k := 0; k < int(pd.NamedChildCount()); k++ {
				ch := pd.NamedChild(k)
				if ch.Type() == "identifier" {
					scope[nodeText(ch, src)] = typeName
				}
			}
		}
	}
	if len(scope) == 0 {
		return nil
	}
	return scope
}

// goCleanTypeName strips pointer / slice / array / ellipsis prefixes
// AND the package qualifier from a type expression node's text, so
// receiver matching compares against the bare type name. Empty in
// → empty out. Examples:
//
//	*testing.T          → T
//	[]*repomap.Symbol   → types.Symbol
//	sitter.Node         → Node
//	types.Graph               → types.Graph
//
// The package qualifier is dropped because every downstream
// consumer (SymbolDefs keyed by bare name, typeSet keyed by bare
// name, resolveCallTarget's MethodIndex keyed by (fi.Package,
// receiver, name) tuple) uses the bare-name shape. Losing the
// qualifier does NOT create cross-package collisions in the
// receiver-aware resolver because resolveCallTarget still keys by
// (pkg, receiver, name) — the pkg disambiguates a `T` in `testing`
// from a `T` in some other package.
func goCleanTypeName(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	// Descend through pointer_type → type_identifier.
	cur := n
	for {
		switch cur.Type() {
		case "pointer_type", "slice_type", "array_type":
			if e := cur.NamedChild(0); e != nil {
				cur = e
				continue
			}
		case "parenthesized_type":
			if e := cur.NamedChild(0); e != nil {
				cur = e
				continue
			}
		}
		break
	}
	t := nodeText(cur, src)
	t = strings.TrimPrefix(t, "*")
	t = strings.TrimPrefix(t, "[]")
	t = strings.TrimPrefix(t, "...")
	// Drop the package qualifier: everything after the last "." is
	// the bare type name.
	if i := strings.LastIndex(t, "."); i >= 0 {
		t = t[i+1:]
	}
	return t
}

// goWalkCalls recursively visits `node`, emitting a types.Relation for
// every call_expression. Uses the provided local scope to resolve
// receiver text to a declared type when possible.
func goWalkCalls(node *sitter.Node, src []byte, file string, scope map[string]string, out *[]types.Relation) {
	if node.Type() == "call_expression" {
		goEmitCall(node, src, file, scope, out)
		// Do NOT return: arguments may contain nested calls.
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		goWalkCalls(node.NamedChild(i), src, file, scope, out)
	}
}

func goEmitCall(node *sitter.Node, src []byte, file string, scope map[string]string, out *[]types.Relation) {
	fn := node.ChildByFieldName("function")
	if fn == nil {
		return
	}
	switch fn.Type() {
	case "identifier":
		name := nodeText(fn, src)
		line := nodeLine(fn)
		*out = append(*out, types.Relation{
			Kind:       "call",
			FromEP:     types.RelationEndpoint{File: file, Line: line},
			ToEP:       types.RelationEndpoint{Name: name, File: file, Line: line},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceTreeSitter,
			ResolvedBy: "go_ast_identifier_call",
		})
	case "selector_expression":
		field := fn.ChildByFieldName("field")
		if field == nil {
			return
		}
		name := nodeText(field, src)
		line := nodeLine(fn)
		var receiver string
		if op := fn.ChildByFieldName("operand"); op != nil {
			receiver = nodeText(op, src)
			receiver = strings.TrimPrefix(receiver, "*")
			receiver = strings.TrimPrefix(receiver, "&")
			receiver = strings.Trim(receiver, "()")
		}
		// Receiver-to-type resolution. When the receiver is a plain
		// identifier (no further dots) and appears in the local
		// scope, rewrite it to the declared type. This turns
		// `g.CallersOf()` inside `func (g *types.Graph) M()` into a
		// receiver of "types.Graph" rather than "g".
		if scope != nil && !strings.Contains(receiver, ".") {
			if t, ok := scope[receiver]; ok && t != "" {
				receiver = t
			}
		}
		*out = append(*out, types.Relation{
			Kind:   "call",
			FromEP: types.RelationEndpoint{File: file, Line: line},
			ToEP: types.RelationEndpoint{
				Name:     name,
				Receiver: receiver,
				File:     file,
				Line:     line,
			},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceTreeSitter,
			ResolvedBy: "go_ast_selector_call",
		})
	}
}

func goExtractTypeRefs(root *sitter.Node, src []byte, file string) []types.Relation {
	var rels []types.Relation
	seen := make(map[string]bool)
	walkNamedChildren(root, true, func(node *sitter.Node) {
		if node.Type() != "type_identifier" {
			return
		}
		name := nodeText(node, src)
		key := file + "→" + name
		if seen[key] {
			return
		}
		seen[key] = true
		rels = append(rels, types.Relation{
			Kind:       "type_usage",
			FromEP:     types.RelationEndpoint{File: file, Line: nodeLine(node)},
			ToEP:       types.RelationEndpoint{Name: name, File: file, Line: nodeLine(node)},
			File:       file,
			Line:       nodeLine(node),
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceTreeSitter,
			ResolvedBy: "go_ast_type_usage",
		})
	})
	return rels
}

func goFuncSignature(node *sitter.Node, src []byte) string {
	params := node.ChildByFieldName("parameters")
	result := node.ChildByFieldName("result")
	sig := ""
	if params != nil {
		sig = nodeText(params, src)
	}
	if result != nil {
		sig += " " + nodeText(result, src)
	}
	if len(sig) > 120 {
		sig = sig[:117] + "..."
	}
	return sig
}
