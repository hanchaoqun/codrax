package index

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractProto walks tree-sitter-protobuf parse trees. Captures the
// four cross-file-relevant symbol kinds:
//
//   - message     — class-shaped record type
//   - enum        — enum
//   - service     — gRPC service
//   - rpc         — service method (parent = service name)
//
// "package" comes from the proto `package foo.bar;` directive when
// present, falling back to the file's containing dir. Imports surface
// from `import "other.proto";`.
func extractProto(root *sitter.Node, src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	pkg = protoDerivePackage(file)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		ch := root.NamedChild(i)
		switch ch.Type() {
		case "package":
			if name := protoPackageName(ch, src); name != "" {
				pkg = name
			}
		case "import":
			if imp, ok := protoExtractImport(ch, src, file); ok {
				imps = append(imps, imp)
			}
		case "message":
			t, fields, nestedRels := protoExtractMessage(ch, src, file, "")
			syms = append(syms, t...)
			syms = append(syms, fields...)
			rels = append(rels, nestedRels...)
		case "enum":
			if s, ok := protoExtractEnum(ch, src, file); ok {
				syms = append(syms, s)
			}
		case "service":
			t, methods := protoExtractService(ch, src, file)
			syms = append(syms, t...)
			syms = append(syms, methods...)
		}
	}
	return
}

func protoDerivePackage(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

func protoPackageName(node *sitter.Node, src []byte) string {
	// Strip leading "package " keyword; the grammar exposes the name
	// as one or more identifier nodes joined by dots.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "full_ident" || ch.Type() == "identifier" {
			return strings.TrimSpace(nodeText(ch, src))
		}
	}
	// Last resort: text after "package " keyword, before ";".
	t := strings.TrimSpace(nodeText(node, src))
	t = strings.TrimPrefix(t, "package")
	t = strings.TrimSuffix(strings.TrimSpace(t), ";")
	return t
}

func protoExtractImport(node *sitter.Node, src []byte, file string) (types.Import, bool) {
	// Find the string literal child (the imported file path).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == "string" || ch.Type() == "string_literal" {
			path := strings.Trim(nodeText(ch, src), `"'`)
			if path == "" {
				continue
			}
			return types.Import{
				Raw:  nodeText(node, src),
				Path: path,
				File: file,
				Line: nodeLine(node),
			}, true
		}
	}
	return types.Import{}, false
}

func protoExtractMessage(node *sitter.Node, src []byte, file, parent string) (cls, fields []types.Symbol, rels []types.Relation) {
	nameNode := childByType(node, "message_name")
	if nameNode == nil {
		nameNode = childByType(node, "identifier")
	}
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	cls = append(cls, types.Symbol{
		Name:     name,
		Kind:     "message",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
		Parent:   parent,
	})
	body := childByType(node, "message_body")
	if body == nil {
		return
	}
	for j := 0; j < int(body.NamedChildCount()); j++ {
		ch := body.NamedChild(j)
		switch ch.Type() {
		case "field":
			if s, ok := protoExtractField(ch, src, file, name); ok {
				fields = append(fields, s)
			}
		case "message":
			nestedCls, nestedFields, nestedRels := protoExtractMessage(ch, src, file, name)
			cls = append(cls, nestedCls...)
			fields = append(fields, nestedFields...)
			rels = append(rels, nestedRels...)
		case "enum":
			if s, ok := protoExtractEnum(ch, src, file); ok {
				s.Parent = name
				cls = append(cls, s)
			}
		}
	}
	return
}

func protoExtractField(node *sitter.Node, src []byte, file, parent string) (types.Symbol, bool) {
	nameNode := childByType(node, "identifier")
	if nameNode == nil {
		return types.Symbol{}, false
	}
	name := nodeText(nameNode, src)
	if name == "" {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     name,
		Kind:     "field",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
		Parent:   parent,
	}, true
}

func protoExtractEnum(node *sitter.Node, src []byte, file string) (types.Symbol, bool) {
	nameNode := childByType(node, "enum_name")
	if nameNode == nil {
		nameNode = childByType(node, "identifier")
	}
	if nameNode == nil {
		return types.Symbol{}, false
	}
	return types.Symbol{
		Name:     nodeText(nameNode, src),
		Kind:     "enum",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
	}, true
}

func protoExtractService(node *sitter.Node, src []byte, file string) (svc, methods []types.Symbol) {
	nameNode := childByType(node, "service_name")
	if nameNode == nil {
		nameNode = childByType(node, "identifier")
	}
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	svc = append(svc, types.Symbol{
		Name:     name,
		Kind:     "service",
		File:     file,
		Line:     nodeLine(node),
		EndLine:  nodeEndLine(node),
		Exported: true,
	})
	// In tree-sitter-protobuf, `rpc` declarations live as direct
	// named children of the service node (no service_body wrapper).
	for j := 0; j < int(node.NamedChildCount()); j++ {
		ch := node.NamedChild(j)
		if ch.Type() != "rpc" {
			continue
		}
		rpcName := childByType(ch, "rpc_name")
		if rpcName == nil {
			rpcName = childByType(ch, "identifier")
		}
		if rpcName == nil {
			continue
		}
		methods = append(methods, types.Symbol{
			Name:     nodeText(rpcName, src),
			Kind:     "rpc",
			File:     file,
			Line:     nodeLine(ch),
			EndLine:  nodeEndLine(ch),
			Exported: true,
			Parent:   name,
		})
	}
	return
}
