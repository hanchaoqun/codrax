package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractLua surfaces top-level decls from tree-sitter-lua. The grammar
// in this vendored module uses these node-type names (verified via
// tree dump):
//
//   function_statement     — both `function foo()` and `local function foo()`.
//                            Distinguished by a `local` child node.
//                            function_name child holds the dotted/colon
//                            form (`M.foo` / `Class:method`); a bare
//                            identifier child means "global function".
//   variable_declaration   — `local x = require "mod"` / `M = {}`
//   function_call          — `require "foo"` (when used as expression)
//   module_return_statement / return_statement
//
// We surface:
//   - function_statement → Symbol(kind=function|method)
//   - require() calls inside function_call or as RHS of variable_declaration
func extractLua(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	pkg = luaDerivePackage(file)
	walkLua(root, src, file, &syms, &imps)
	return
}

func luaDerivePackage(file string) string {
	parts := strings.Split(file, "/")
	for i, p := range parts {
		if (p == "lua" || p == "lib") && i+1 < len(parts)-1 {
			return parts[i+1]
		}
	}
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

func walkLua(node *sitter.Node, src []byte, file string, syms *[]types.Symbol, imps *[]types.Import) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "function_statement":
			if s, ok := luaExtractFuncStmt(ch, src, file); ok {
				*syms = append(*syms, s)
			}
		case "variable_declaration":
			if imp, ok := luaExtractVarDeclRequire(ch, src, file); ok {
				*imps = append(*imps, imp)
			}
		case "function_call":
			if imp, ok := luaExtractCallRequire(ch, src, file); ok {
				*imps = append(*imps, imp)
			}
		case "do_statement", "if_statement", "while_statement", "for_statement", "repeat_statement":
			walkLua(ch, src, file, syms, imps)
		}
	}
}

// luaExtractFuncStmt handles both `function ...` and `local function ...`.
// The grammar puts a `local` keyword child for the local form.
func luaExtractFuncStmt(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	isLocal := false
	var nameRaw string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "local":
			isLocal = true
		case "identifier":
			// Bare global function: `function foo(...)`.
			if nameRaw == "" {
				nameRaw = strings.TrimSpace(nodeText(ch, src))
			}
		case "function_name":
			// Dotted/colon: `function M.foo` / `Class:method`.
			nameRaw = strings.TrimSpace(nodeText(ch, src))
		}
	}
	if nameRaw == "" {
		return types.Symbol{}, false
	}
	parent, name := splitLuaName(nameRaw)
	kind := "function"
	if parent != "" {
		kind = "method"
	}
	// Signature: walk parameter_list child.
	var sig string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "parameter_list" {
			sig = nodeText(ch, src)
			if len(sig) > 120 {
				sig = sig[:117] + "..."
			}
			break
		}
	}
	return types.Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  !isLocal,
		Parent:    parent,
		Signature: sig,
	}, true
}

func splitLuaName(full string) (parent, name string) {
	full = strings.TrimSpace(full)
	if idx := strings.LastIndexAny(full, ".:"); idx >= 0 {
		return full[:idx], full[idx+1:]
	}
	return "", full
}

// luaExtractVarDeclRequire handles `local socket = require "socket"`.
// The variable_declaration has a function_call child whose first
// identifier is "require" and whose argument is the module path.
func luaExtractVarDeclRequire(node *sitter.Node, src []byte, file string) (types.Import, bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "function_call" {
			if imp, ok := luaExtractCallRequire(ch, src, file); ok {
				return imp, true
			}
		}
	}
	return types.Import{}, false
}

// luaExtractCallRequire handles a top-level `require "foo"` call.
// The first identifier child is the function name; the
// string_argument child carries the path.
func luaExtractCallRequire(node *sitter.Node, src []byte, file string) (types.Import, bool) {
	if node.NamedChildCount() == 0 {
		return types.Import{}, false
	}
	first := node.NamedChild(0)
	if first.Type() != "identifier" {
		return types.Import{}, false
	}
	name := strings.TrimSpace(nodeText(first, src))
	if name != "require" && name != "load" && name != "dofile" {
		return types.Import{}, false
	}
	// Find string_argument or arguments child.
	var path string
	for i := 1; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		switch ch.Type() {
		case "string_argument", "string", "string_argument_list":
			path = strings.Trim(nodeText(ch, src), `"'`)
		case "arguments":
			if ch.NamedChildCount() > 0 {
				path = strings.Trim(nodeText(ch.NamedChild(0), src), `"'`)
			}
		}
		if path != "" {
			break
		}
	}
	if path == "" {
		return types.Import{}, false
	}
	return types.Import{
		Raw:  nodeText(node, src),
		Path: path,
		File: file,
		Line: nodeLine(node),
	}, true
}
