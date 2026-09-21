package tool

import (
	"context"
	"strings"
	"time"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// javaProbeSourceUnit is transport preparation, not syntax authority. A parser
// gap leaves authored source intact for javac; it never rejects an emitted plan.
type javaProbeSourceUnit struct {
	Source    string
	FileName  string
	MainClass string
	Issue     string
}

func prepareJavaProbeSourceUnit(parent context.Context, code string) javaProbeSourceUnit {
	unit := javaProbeSourceUnit{Source: code, FileName: "CodraxVerificationProbe.java", Issue: "Java probe entry point could not be inspected within the source-selection boundary"}
	if parent == nil {
		parent = context.Background()
	}
	if parent.Err() != nil {
		return unit
	}
	ctx, cancel := context.WithTimeout(parent, 250*time.Millisecond)
	defer cancel()
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(repotypes.GetSitterLanguage(repotypes.LangJava))
	tree, err := parser.ParseCtx(ctx, nil, []byte(code))
	if err != nil || tree == nil || ctx.Err() != nil {
		if tree != nil {
			tree.Close()
		}
		return unit
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return unit
	}
	unit.Issue = "Java probe has no unambiguous supported top-level main entry point"
	var packageName string
	var publicTypes, mains []string
	var imports [][2]uint32
	fullUnit := false
	unitOnly, statements, seenBody := false, false, false
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(i)
		if node.Type() != "line_comment" && node.Type() != "block_comment" && node.Type() != "import_declaration" {
			seenBody = true
		}
		switch node.Type() {
		case "package_declaration":
			fullUnit, unitOnly = true, true
			for j := 0; j < int(node.NamedChildCount()); j++ {
				part := node.NamedChild(j)
				if part.Type() == "identifier" || part.Type() == "scoped_identifier" {
					packageName = javaProbeQualifiedName(part, code)
				}
			}
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
			fullUnit = true
			name := node.ChildByFieldName("name")
			if name == nil || name.IsMissing() {
				continue
			}
			if javaProbeHasModifier(node, "public") {
				publicTypes = append(publicTypes, name.Content([]byte(code)))
			}
			if body := node.ChildByFieldName("body"); body != nil {
				for j := 0; j < int(body.NamedChildCount()); j++ {
					member := body.NamedChild(j)
					if javaProbeIsMainMethod(member, code, node.Type() == "interface_declaration") {
						mains = append(mains, name.Content([]byte(code)))
					}
					if member.Type() == "enum_body_declarations" {
						for k := 0; k < int(member.NamedChildCount()); k++ {
							if javaProbeIsMainMethod(member.NamedChild(k), code, false) {
								mains = append(mains, name.Content([]byte(code)))
							}
						}
					}
				}
			}
		case "module_declaration", "method_declaration":
			fullUnit, unitOnly = true, true
		case "ERROR":
			// Recovery nodes may contain an incomplete class header rather than
			// a class_declaration. Do not turn that full source into a snippet.
			if javaProbeErrorHasDeclaration(node) {
				fullUnit, unitOnly = true, true
			}
		case "import_declaration":
			// A late import must stay late (and fail javac), not be repaired
			// by moving it ahead of the authored executable statements.
			if !seenBody {
				imports = append(imports, [2]uint32{node.StartByte(), node.EndByte()})
			}
		default:
			statements = statements || strings.HasSuffix(node.Type(), "_statement") || node.Type() == "local_variable_declaration"
		}
	}
	// Local class declarations can be part of a genuine statement fragment.
	// Only a mixed declaration/statement AST without compilation-unit-only
	// constructs may try the wrapper; a pure (even broken) unit stays intact.
	if statements && !unitOnly && len(publicTypes) == 0 {
		fullUnit = false
	}
	if fullUnit {
		if len(publicTypes) == 1 {
			unit.FileName = publicTypes[0] + ".java"
		} else if len(publicTypes) == 0 && len(mains) == 1 {
			unit.FileName = mains[0] + ".java"
		}
		// An unrelated method-body grammar gap is not syntax authority.
		// Select only precise entry declarations; javac judges the whole unit.
		if len(publicTypes) > 1 || len(mains) != 1 {
			return unit
		}
		unit.MainClass = mains[0]
		if packageName != "" {
			unit.MainClass = packageName + "." + unit.MainClass
		}
		unit.Issue = ""
		return unit
	}
	// Retain the legacy statement-fragment lane without a second parser
	// validity gate: javac owns syntax, including grammar-subset gaps. Imports
	// are parser-owned leading spans; comments/strings never select a class.
	unit.Source, unit.MainClass, unit.Issue = javaProbeWrapStatements(code, imports), "CodraxVerificationProbe", ""
	return unit
}

func javaProbeHasModifier(node *sitter.Node, modifier string) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		mods := node.NamedChild(i)
		if mods.Type() != "modifiers" {
			continue
		}
		for j := 0; j < int(mods.ChildCount()); j++ {
			if mods.Child(j).Type() == modifier {
				return true
			}
		}
	}
	return false
}

func javaProbeIsMainMethod(node *sitter.Node, code string, inInterface bool) bool {
	public := javaProbeHasModifier(node, "public") || (inInterface && !javaProbeHasModifier(node, "private"))
	if node.Type() != "method_declaration" || !public || !javaProbeHasModifier(node, "static") {
		return false
	}
	name, result, params := node.ChildByFieldName("name"), node.ChildByFieldName("type"), node.ChildByFieldName("parameters")
	if name == nil || name.IsMissing() || name.Content([]byte(code)) != "main" || result == nil || result.IsMissing() || result.Type() != "void_type" || params == nil || params.HasError() || params.IsMissing() {
		return false
	}
	var parameter *sitter.Node
	for i := 0; i < int(params.NamedChildCount()); i++ {
		child := params.NamedChild(i)
		if child.Type() == "line_comment" || child.Type() == "block_comment" {
			continue
		}
		if parameter != nil || (child.Type() != "formal_parameter" && child.Type() != "spread_parameter") {
			return false
		}
		parameter = child
	}
	if parameter == nil {
		return false
	}
	typ := parameter.ChildByFieldName("type")
	if typ == nil && parameter.Type() == "spread_parameter" {
		for i := 0; i < int(parameter.NamedChildCount()); i++ {
			child := parameter.NamedChild(i)
			if child.Type() == "type_identifier" || child.Type() == "scoped_type_identifier" {
				typ = child
			}
		}
	}
	if typ == nil {
		return false
	}
	dimensions := 0
	if parameter.Type() == "spread_parameter" {
		dimensions = 1
	}
	if typ.Type() == "array_type" {
		dimensions += javaProbeDimensions(typ.ChildByFieldName("dimensions"))
		typ = typ.ChildByFieldName("element")
	}
	dimensions += javaProbeDimensions(parameter.ChildByFieldName("dimensions"))
	return dimensions == 1 && (javaProbeQualifiedName(typ, code) == "String" || javaProbeQualifiedName(typ, code) == "java.lang.String")
}

func javaProbeDimensions(node *sitter.Node) int {
	if node == nil {
		return 0
	}
	n := 0
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == "[" {
			n++
		}
	}
	return n
}

func javaProbeQualifiedName(node *sitter.Node, code string) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "identifier", "type_identifier":
		return node.Content([]byte(code))
	case "scoped_identifier", "scoped_type_identifier":
		var parts []string
		for i := 0; i < int(node.NamedChildCount()); i++ {
			if part := javaProbeQualifiedName(node.NamedChild(i), code); part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, ".")
	}
	return ""
}

func javaProbeErrorHasDeclaration(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "class", "interface", "enum", "record", "package", "module":
			return true
		case "ERROR":
			if javaProbeErrorHasDeclaration(child) {
				return true
			}
		}
	}
	return false
}

func javaProbeWrapStatements(code string, imports [][2]uint32) string {
	var prefix, body strings.Builder
	var offset uint32
	for _, span := range imports {
		prefix.WriteString(code[span[0]:span[1]])
		prefix.WriteByte('\n')
		body.WriteString(code[offset:span[0]])
		offset = span[1]
	}
	body.WriteString(code[offset:])
	return prefix.String() + "public final class CodraxVerificationProbe {\n  public static void main(String[] args) throws Exception {\n" + body.String() + "\n  }\n}\n"
}
