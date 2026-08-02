package repl

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	python "github.com/smacker/go-tree-sitter/python"
)

// dataTaskScriptLiteralConsumption returns the paths proven to be passed as
// literal first arguments to the data runner's file-reading helpers. The bool
// is true only when the Python syntax tree is complete and every use of a
// reader helper is a direct call with a simple literal path. Dynamic paths,
// aliases, malformed source, and unsupported literal forms deliberately
// fail-open at the staging caller, which falls back to the declared input
// contract instead of hard-rejecting a structurally valid script.
func dataTaskScriptLiteralConsumption(script string) (map[string]bool, bool) {
	source := []byte(strings.TrimSpace(script))
	if len(source) == 0 {
		return nil, false
	}
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil || tree == nil {
		return nil, false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, false
	}
	paths := map[string]bool{}
	authoritative := true
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil || !authoritative {
			return
		}
		if node.Type() == "call" {
			fn := node.ChildByFieldName("function")
			if fn != nil && fn.Type() == "identifier" && dataTaskScriptReaderHelper(nodeSourceText(fn, source)) {
				p, ok := dataTaskScriptLiteralFirstArgument(node, source)
				if !ok {
					authoritative = false
					return
				}
				paths[normalizeDataTaskCoveragePath(p)] = true
				args := node.ChildByFieldName("arguments")
				if args != nil {
					for i := 0; i < int(args.NamedChildCount()); i++ {
						walk(args.NamedChild(i))
					}
				}
				return
			}
		}
		if node.Type() == "identifier" && dataTaskScriptReaderHelper(nodeSourceText(node, source)) {
			// A helper used outside the function seat of a direct call may be
			// aliased or passed through another callable. We cannot prove its
			// eventual path statically, so keep admission fail-open.
			authoritative = false
			return
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walk(node.NamedChild(i))
		}
	}
	walk(root)
	if !authoritative {
		return nil, false
	}
	delete(paths, "")
	return paths, true
}

func dataTaskScriptReaderHelper(name string) bool {
	switch strings.TrimSpace(name) {
	case "read_text", "csv_rows", "tsv_rows", "json_load", "json_records", "jsonl_rows", "open":
		return true
	default:
		return false
	}
}

func dataTaskScriptLiteralFirstArgument(call *sitter.Node, source []byte) (string, bool) {
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return "", false
	}
	for i := 0; i < int(args.NamedChildCount()); i++ {
		arg := args.NamedChild(i)
		if arg == nil || arg.Type() == "comment" {
			continue
		}
		if arg.Type() == "keyword_argument" {
			name := arg.ChildByFieldName("name")
			value := arg.ChildByFieldName("value")
			if name == nil || value == nil || nodeSourceText(name, source) != "path" {
				return "", false
			}
			return dataTaskPythonSimpleString(value, source)
		}
		return dataTaskPythonSimpleString(arg, source)
	}
	return "", false
}

func dataTaskPythonSimpleString(node *sitter.Node, source []byte) (string, bool) {
	if node == nil || node.Type() != "string" {
		return "", false
	}
	raw := nodeSourceText(node, source)
	for len(raw) > 0 && strings.ContainsRune("rRuU", rune(raw[0])) {
		raw = raw[1:]
	}
	if len(raw) < 2 || (raw[0] != '\'' && raw[0] != '"') || raw[len(raw)-1] != raw[0] {
		return "", false
	}
	if len(raw) >= 6 && strings.HasPrefix(raw, strings.Repeat(string(raw[0]), 3)) {
		return "", false
	}
	value := raw[1 : len(raw)-1]
	if strings.Contains(value, `\`) {
		// Escaped and computed spellings are valid Python but not an exact
		// literal-path proof. The caller will fail-open to declared inputs.
		return "", false
	}
	return value, true
}

func nodeSourceText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	return string(source[start:end])
}
