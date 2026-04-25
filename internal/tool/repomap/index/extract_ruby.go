package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractRuby walks a tree-sitter-ruby parse tree and emits top-level
// classes / modules / methods / constants. Ruby's grammar surfaces the
// usual node types:
//
//   - class                — class Foo ... end
//   - module               — module Bar ... end
//   - method               — def foo(...) ... end (top-level or in body)
//   - singleton_method     — def self.bar(...) ... end (class methods)
//   - assignment with constant LHS — FOO = ...
//
// "Package" maps to the file's directory (Ruby has no package
// keyword); resolver_ruby.go re-derives a clean module path from
// `lib/<pkg>/...` layouts when present.
func extractRuby(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	pkg = rubyDerivePackage(file)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "class":
			cls, methods, classRels := rubyExtractClassLike(ch, src, file, "class")
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		case "module":
			cls, methods, classRels := rubyExtractClassLike(ch, src, file, "module")
			syms = append(syms, cls...)
			syms = append(syms, methods...)
			rels = append(rels, classRels...)
		case "method":
			if s, ok := rubyExtractMethod(ch, src, file, ""); ok {
				syms = append(syms, s)
			}
		case "singleton_method":
			if s, ok := rubyExtractMethod(ch, src, file, ""); ok {
				s.Kind = "method"
				syms = append(syms, s)
			}
		case "assignment":
			if s, ok := rubyExtractConstant(ch, src, file); ok {
				syms = append(syms, s)
			}
		case "call":
			// `require "foo"` / `require_relative "bar"` show up as
			// call expressions whose method ident is the keyword.
			if imp, ok := rubyExtractRequire(ch, src, file); ok {
				imps = append(imps, imp)
			}
		}
	}
	return
}

func rubyDerivePackage(file string) string {
	// `lib/foo/bar.rb` → `foo`; bare `foo.rb` → empty.
	parts := strings.Split(file, "/")
	for i, p := range parts {
		if p == "lib" && i+1 < len(parts)-1 {
			return parts[i+1]
		}
	}
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

func rubyExtractClassLike(node *sitter.Node, src []byte, file, kind string) (cls, methods []types.Symbol, rels []types.Relation) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     kind, // "class" or "module"
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
	})
	// superclass → inheritance relation
	if super := node.ChildByFieldName("superclass"); super != nil {
		baseName := strings.TrimPrefix(strings.TrimSpace(nodeText(super, src)), "<")
		baseName = strings.TrimSpace(baseName)
		if baseName != "" {
			rels = append(rels, types.Relation{
				Kind: "inheritance",
				From: file + ":" + name,
				To:   baseName,
				File: file,
				Line: nodeLine(super),
			})
		}
	}
	if body := node.ChildByFieldName("body"); body != nil {
		for j := 0; j < int(body.NamedChildCount()); j++ {
			ch := body.NamedChild(j)
			switch ch.Type() {
			case "method", "singleton_method":
				if s, ok := rubyExtractMethod(ch, src, file, name); ok {
					methods = append(methods, s)
				}
			case "class", "module":
				nestedCls, nestedMethods, nestedRels := rubyExtractClassLike(ch, src, file, ch.Type())
				cls = append(cls, nestedCls...)
				methods = append(methods, nestedMethods...)
				rels = append(rels, nestedRels...)
			}
		}
	}
	return
}

func rubyExtractMethod(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	kind := "method"
	if parent == "" {
		kind = "function"
	}
	var sig string
	if params := node.ChildByFieldName("parameters"); params != nil {
		sig = nodeText(params, src)
		if len(sig) > 120 {
			sig = sig[:117] + "..."
		}
	}
	return types.Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      nodeLine(node),
		EndLine:   nodeEndLine(node),
		Exported:  true,
		Parent:    parent,
		Signature: sig,
	}, true
}

func rubyExtractConstant(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	// Only top-level CONSTANT = ... (LHS starts with uppercase).
	left := node.ChildByFieldName("left")
	if left == nil {
		return types.Symbol{}, false
	}
	name := nodeText(left, src)
	if name == "" || !isRubyConstant(name) {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     name,
		Kind:     "const",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
	}, true
}

func isRubyConstant(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

func rubyExtractRequire(node *sitter.Node, src []byte, file string) (types.Import, bool) {
	method := node.ChildByFieldName("method")
	if method == nil {
		return types.Import{}, false
	}
	name := nodeText(method, src)
	if name != "require" && name != "require_relative" && name != "load" && name != "autoload" {
		return types.Import{}, false
	}
	args := node.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return types.Import{}, false
	}
	first := args.NamedChild(0)
	path := strings.Trim(nodeText(first, src), `"'`)
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
