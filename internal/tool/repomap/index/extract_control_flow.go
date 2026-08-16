package index

import (
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

type controlFlowBranchSpec struct {
	selector  string
	condition string
	guardLine int
	arm       types.ControlFlowBranchArm
	body      []*sitter.Node
}

// extractControlFlowBranches derives lexical branch ownership directly from
// the parsed tree. It never scans source/model prose for control keywords.
// Grammar-specific node names are a closed parser adapter; the emitted carrier
// is shared by every downstream language lowerer.
func extractControlFlowBranches(root *sitter.Node, src []byte) []types.ControlFlowBranch {
	if root == nil {
		return nil
	}
	var out []types.ControlFlowBranch
	var walk func(*sitter.Node, string)
	walk = func(node *sitter.Node, selector string) {
		if node == nil {
			return
		}
		if next := controlFlowSelector(node, src); next != "" {
			selector = next
		}
		for _, spec := range controlFlowBranchSpecs(node, selector, src) {
			branch, ok := materializeControlFlowBranch(spec, src)
			if ok {
				out = append(out, branch)
			}
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walk(node.NamedChild(i), selector)
		}
	}
	walk(root, "")
	if len(out) == 0 {
		return nil
	}
	return out
}

func controlFlowSelector(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	var field string
	switch node.Type() {
	case "expression_switch_statement":
		field = "value"
	case "match_statement":
		field = "subject"
	case "switch_statement":
		field = "value"
		if node.ChildByFieldName(field) == nil {
			field = "condition"
		}
		if node.ChildByFieldName(field) == nil {
			field = "expr"
		}
	case "switch_expression":
		field = "condition"
	case "match_expression":
		field = "value"
	case "case":
		field = "value"
	case "when_expression":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "when_subject" {
				return compactControlFlowText(nodeText(child, src))
			}
		}
	}
	if field == "" {
		return ""
	}
	return compactControlFlowText(nodeText(node.ChildByFieldName(field), src))
}

func controlFlowBranchSpecs(node *sitter.Node, selector string, src []byte) []controlFlowBranchSpec {
	if node == nil {
		return nil
	}
	switch node.Type() {
	case "if_statement", "if_expression", "if", "unless", "unless_statement",
		"ternary_expression", "conditional_expression", "guard_statement":
		return ifLikeControlFlowBranchSpecs(node, src)
	case "expression_case", "type_case", "default_case", "case_clause",
		"switch_case", "switch_default", "switch_block_statement_group",
		"when_entry", "match_arm", "case_statement", "when", "switch_entry":
		if spec, ok := caseControlFlowBranchSpec(node, selector, src); ok {
			return []controlFlowBranchSpec{spec}
		}
	case "case":
		// Ruby's case grammar exposes the default arm as a direct `else`
		// child rather than a field on each `when` arm.
		for _, child := range namedControlFlowChildren(node) {
			if child == nil || child.Type() != "else" {
				continue
			}
			body := namedControlFlowChildren(child)
			if len(body) > 0 {
				return []controlFlowBranchSpec{{
					selector: selector, guardLine: int(child.StartPoint().Row) + 1,
					arm: types.ControlFlowArmDefault, body: body,
				}}
			}
		}
	}
	return nil
}

func ifLikeControlFlowBranchSpecs(node *sitter.Node, src []byte) []controlFlowBranchSpec {
	condition := node.ChildByFieldName("condition")
	consequence := node.ChildByFieldName("consequence")
	alternative := node.ChildByFieldName("alternative")

	// Kotlin, Swift, and Lua grammars do not expose the conventional fields.
	// Decode their named-child grammar shapes without consulting raw words.
	if condition == nil || consequence == nil {
		named := namedControlFlowChildren(node)
		if node.Type() == "if_statement" && hasControlFlowNodeType(named, "if_then") {
			return luaIfControlFlowBranchSpecs(node, named, src)
		} else if len(named) > 0 {
			condition = named[0]
			remaining := named[1:]
			if idx := indexControlFlowNodeType(remaining, "else"); idx >= 0 {
				if idx > 0 {
					consequence = controlFlowSyntheticBody(remaining[:idx])
				}
				if idx+1 < len(remaining) {
					alternative = controlFlowSyntheticBody(remaining[idx+1:])
				}
			} else {
				if len(remaining) > 0 {
					consequence = remaining[0]
				}
				if len(remaining) > 1 {
					alternative = remaining[1]
				}
			}
		}
	}
	if condition == nil || consequence == nil {
		return nil
	}
	conditionText := compactControlFlowText(nodeText(condition, src))
	guardLine := int(node.StartPoint().Row) + 1
	out := []controlFlowBranchSpec{{
		condition: conditionText,
		guardLine: guardLine,
		arm:       types.ControlFlowArmConsequence,
		body:      []*sitter.Node{consequence},
	}}
	if alternative != nil {
		out = append(out, controlFlowBranchSpec{
			condition: conditionText,
			guardLine: guardLine,
			arm:       types.ControlFlowArmAlternative,
			body:      []*sitter.Node{alternative},
		})
	}
	return out
}

// controlFlowSyntheticBody returns a node spanning a contiguous sequence only
// when the grammar already wraps it. The current Swift shape always supplies a
// `statements` wrapper, so multiple loose nodes are intentionally not fused by
// source range (doing so would reintroduce adjacency inference).
func controlFlowSyntheticBody(nodes []*sitter.Node) *sitter.Node {
	if len(nodes) == 1 {
		return nodes[0]
	}
	return nil
}

func luaIfControlFlowBranchSpecs(node *sitter.Node, named []*sitter.Node, src []byte) []controlFlowBranchSpec {
	thenIdx := indexControlFlowNodeType(named, "if_then")
	if thenIdx <= 0 {
		return nil
	}
	var condition *sitter.Node
	for i := thenIdx - 1; i >= 0; i-- {
		if !controlFlowMarkerNode(named[i].Type()) {
			condition = named[i]
			break
		}
	}
	elseIdx := indexControlFlowNodeType(named, "if_else")
	endIdx := indexControlFlowNodeType(named, "if_end")
	if endIdx < 0 {
		endIdx = len(named)
	}
	consEnd := endIdx
	if elseIdx >= 0 {
		consEnd = elseIdx
	}
	if condition == nil {
		return nil
	}
	filter := func(nodes []*sitter.Node) []*sitter.Node {
		out := make([]*sitter.Node, 0, len(nodes))
		for _, child := range nodes {
			if child != nil && !controlFlowMarkerNode(child.Type()) {
				out = append(out, child)
			}
		}
		return out
	}
	conditionText := compactControlFlowText(nodeText(condition, src))
	guardLine := int(node.StartPoint().Row) + 1
	var out []controlFlowBranchSpec
	if body := filter(named[thenIdx+1 : consEnd]); len(body) > 0 {
		out = append(out, controlFlowBranchSpec{condition: conditionText, guardLine: guardLine, arm: types.ControlFlowArmConsequence, body: body})
	}
	if elseIdx >= 0 && elseIdx+1 < endIdx {
		if body := filter(named[elseIdx+1 : endIdx]); len(body) > 0 {
			out = append(out, controlFlowBranchSpec{condition: conditionText, guardLine: guardLine, arm: types.ControlFlowArmAlternative, body: body})
		}
	}
	return out
}

func caseControlFlowBranchSpec(node *sitter.Node, selector string, src []byte) (controlFlowBranchSpec, bool) {
	named := namedControlFlowChildren(node)
	var condition *sitter.Node
	var body []*sitter.Node
	arm := types.ControlFlowArmCase
	switch node.Type() {
	case "default_case", "switch_default":
		arm = types.ControlFlowArmDefault
		body = named
	case "expression_case", "type_case", "switch_case", "case_statement":
		condition = node.ChildByFieldName("value")
		if condition == nil && len(named) > 0 && node.Type() != "case_statement" {
			condition = named[0]
		}
		body = controlFlowChildrenExcept(named, condition, nil)
		if condition == nil {
			arm = types.ControlFlowArmDefault
		}
	case "case_clause":
		condition = firstChildOfControlFlowType(named, "case_pattern")
		bodyNode := node.ChildByFieldName("consequence")
		if bodyNode != nil {
			body = []*sitter.Node{bodyNode}
		}
	case "switch_block_statement_group":
		label := firstChildOfControlFlowType(named, "switch_label")
		if label != nil && int(label.NamedChildCount()) > 0 {
			condition = label.NamedChild(0)
		} else {
			arm = types.ControlFlowArmDefault
		}
		body = controlFlowChildrenExcept(named, label, nil)
	case "when_entry":
		condition = firstChildOfControlFlowType(named, "when_condition")
		if condition == nil {
			arm = types.ControlFlowArmDefault
		}
		body = controlFlowChildrenExcept(named, condition, nil)
	case "match_arm":
		condition = node.ChildByFieldName("pattern")
		value := node.ChildByFieldName("value")
		if value != nil {
			body = []*sitter.Node{value}
		}
	case "when":
		condition = node.ChildByFieldName("pattern")
		bodyNode := node.ChildByFieldName("body")
		if bodyNode != nil {
			body = []*sitter.Node{bodyNode}
		}
	case "switch_entry":
		condition = firstChildOfControlFlowType(named, "switch_pattern")
		if condition == nil {
			arm = types.ControlFlowArmDefault
		}
		body = controlFlowChildrenExcept(named, condition, map[string]bool{"default_keyword": true})
	}
	if len(body) == 0 {
		return controlFlowBranchSpec{}, false
	}
	return controlFlowBranchSpec{
		selector:  selector,
		condition: compactControlFlowText(nodeText(condition, src)),
		guardLine: int(node.StartPoint().Row) + 1,
		arm:       arm,
		body:      body,
	}, true
}

func materializeControlFlowBranch(spec controlFlowBranchSpec, src []byte) (types.ControlFlowBranch, bool) {
	if spec.guardLine <= 0 || !spec.arm.IsValid() || len(spec.body) == 0 {
		return types.ControlFlowBranch{}, false
	}
	start, end := 0, 0
	var effects []types.ControlFlowEffect
	seen := make(map[string]bool)
	for _, body := range spec.body {
		if body == nil {
			continue
		}
		bodyStart := int(body.StartPoint().Row) + 1
		bodyEnd := int(body.EndPoint().Row) + 1
		if start == 0 || bodyStart < start {
			start = bodyStart
		}
		if bodyEnd > end {
			end = bodyEnd
		}
		collectControlFlowEffects(body, src, seen, &effects)
	}
	if start <= 0 || end < start {
		return types.ControlFlowBranch{}, false
	}
	return types.ControlFlowBranch{
		Selector:      spec.selector,
		Condition:     spec.condition,
		GuardLine:     spec.guardLine,
		Arm:           spec.arm,
		BodyLineStart: start,
		BodyLineEnd:   end,
		Effects:       effects,
		Provenance:    types.ProvenanceTreeSitter,
		ResolvedBy:    "tree_sitter_control_branch",
	}, true
}

func collectControlFlowEffects(node *sitter.Node, src []byte, seen map[string]bool, out *[]types.ControlFlowEffect) {
	if node == nil {
		return
	}
	if isNestedControlFlowCallable(node.Type()) {
		return
	}
	kind := controlFlowEffectKindForNode(node.Type())
	if kind.IsValid() {
		expr := compactControlFlowText(nodeText(node, src))
		start := int(node.StartPoint().Row) + 1
		end := int(node.EndPoint().Row) + 1
		key := string(kind) + "\x00" + expr + "\x00" + strconv.Itoa(start) + "\x00" + strconv.Itoa(end)
		if expr != "" && start > 0 && !seen[key] {
			seen[key] = true
			*out = append(*out, types.ControlFlowEffect{Kind: kind, Expression: expr, LineStart: start, LineEnd: end})
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		collectControlFlowEffects(node.NamedChild(i), src, seen, out)
	}
}

func controlFlowEffectKindForNode(nodeType string) types.ControlFlowEffectKind {
	switch nodeType {
	case "call_expression", "method_invocation", "call", "function_call":
		return types.ControlFlowEffectCall
	case "return_statement", "return_expression":
		return types.ControlFlowEffectReturn
	case "assignment_statement", "assignment_expression", "augmented_assignment",
		"short_var_declaration", "variable_declarator", "init_declarator",
		"let_declaration", "lexical_declaration", "property_declaration":
		return types.ControlFlowEffectAssignment
	case "break_statement", "continue_statement", "raise_statement", "throw_statement", "throw_expression":
		return types.ControlFlowEffectExit
	default:
		return ""
	}
}

func isNestedControlFlowCallable(nodeType string) bool {
	switch nodeType {
	case "function_declaration", "method_declaration", "function_definition",
		"function_expression", "arrow_function", "method_definition",
		"constructor_declaration", "function_item", "function_signature_item",
		"lambda_expression", "lambda", "closure_expression", "closure",
		"method", "singleton_method", "function_statement", "local_function":
		return true
	default:
		return false
	}
}

func namedControlFlowChildren(node *sitter.Node) []*sitter.Node {
	if node == nil {
		return nil
	}
	out := make([]*sitter.Node, 0, node.NamedChildCount())
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if child := node.NamedChild(i); child != nil {
			out = append(out, child)
		}
	}
	return out
}

func controlFlowChildrenExcept(nodes []*sitter.Node, excluded *sitter.Node, excludedTypes map[string]bool) []*sitter.Node {
	out := make([]*sitter.Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || sameControlFlowNode(node, excluded) || excludedTypes[node.Type()] {
			continue
		}
		out = append(out, node)
	}
	return out
}

func sameControlFlowNode(a, b *sitter.Node) bool {
	return a != nil && b != nil && a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte() && a.Type() == b.Type()
}

func firstChildOfControlFlowType(nodes []*sitter.Node, nodeType string) *sitter.Node {
	for _, node := range nodes {
		if node != nil && node.Type() == nodeType {
			return node
		}
	}
	return nil
}

func hasControlFlowNodeType(nodes []*sitter.Node, nodeType string) bool {
	return indexControlFlowNodeType(nodes, nodeType) >= 0
}

func indexControlFlowNodeType(nodes []*sitter.Node, nodeType string) int {
	for i, node := range nodes {
		if node != nil && node.Type() == nodeType {
			return i
		}
	}
	return -1
}

func controlFlowMarkerNode(nodeType string) bool {
	switch nodeType {
	case "if_start", "if_then", "if_else", "if_elseif", "if_end", "else":
		return true
	default:
		return false
	}
}

func compactControlFlowText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}
