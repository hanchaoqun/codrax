package types

import "strings"

// MermaidSyntaxFamily is the structural syntax family detected from a
// Mermaid body. It is separate from DiagramKind: DiagramKind is the
// answer's semantic contract (flow / sequence / architecture /
// call_dag), while MermaidSyntaxFamily is the concrete carrier syntax
// the renderer will parse.
type MermaidSyntaxFamily string

const (
	MermaidSyntaxUnknown     MermaidSyntaxFamily = ""
	MermaidSyntaxFlow        MermaidSyntaxFamily = "flow"
	MermaidSyntaxSequence    MermaidSyntaxFamily = "sequence"
	MermaidSyntaxUnsupported MermaidSyntaxFamily = "unsupported"
)

// DiagramSyntaxProfile is the single compatibility table between
// semantic diagram kinds and Mermaid body syntax families. Validators,
// prompt helpers, and tests should read this registry instead of
// rediscovering kind-specific exceptions locally.
type DiagramSyntaxProfile struct {
	Kind            DiagramKind
	MermaidFamilies []MermaidSyntaxFamily
	CodeEndpoint    bool
}

// GroundedSourceDiagramEdgeOwnershipContract is the shared wording for the
// source-diagram body/metadata boundary. Keep it free of JSON-significant
// quoting so the canonical tool schema can splice it into a description
// without maintaining a second escaped copy.
const GroundedSourceDiagramEdgeOwnershipContract = "Every visible body edge in a semantic call_dag, or in any diagram whose typed source relation axis is call, register, return, configure, condition, implement, or flow (flow, architecture, sequence, or call_dag), requires one same-direction edge_anchors entry with its honest relation_kind; only a dashed sequence reply structurally paired with its forward invocation is metadata-free."

// GroundedSourceDiagramRelationEvidenceContract is the single-source authoring
// rule for explicit logical non-call edges in every non-runtime diagram family.
// Keep this separate from the body-ownership sentence: relation_kind records
// what the model claims an edge means, while a citable typed EvidenceItem is
// the authority that may prove it. A presentation-only diagram may omit typed
// relations only when the semantic view does not classify its arrows as the
// requested source relation. In particular, typed source relation-axis arrows
// cannot shed their owners to enter that lane. Runtime/root-
// cause trace diagrams use their own causal relation authority and are
// intentionally outside this contract.
const GroundedSourceDiagramRelationEvidenceContract = "In every non-runtime diagram family, an explicit guard/import/precedence/observe relation_kind requires one same-direction citable typed EvidenceItem of the matching claim form; relation_kind alone is not evidence. A guard edge is enclosing callable -> condition identity (or an explicit typed conditional object). Containment has no edge-level claim form, so use Mermaid subgraph/grouping instead of a contain arrow. A diagram may omit typed relations only when the semantic view does not classify its arrows as the requested source relation. In a typed source relation-axis request, every visible arrow remains a principal relation claim and cannot drop edge_anchors to become presentation-only."

// PredicateAxisRequiresDiagramEdgeOwnership reports whether parsed arrows in
// a source diagram are themselves the schema-selected relation requested by
// the user. These are typed analyzer enums, never request or diagram-label
// guesses. AxisDefine stays outside: a definition/architecture answer may use
// arrows as presentation unless it explicitly opts into typed edge metadata.
// Runtime/root-cause trace diagrams are excluded by their caller and retain
// their independent causal authority.
func PredicateAxisRequiresDiagramEdgeOwnership(axis PredicateAxis) bool {
	switch axis {
	case AxisCall,
		AxisRegister,
		AxisReturn,
		AxisConfigure,
		AxisCondition,
		AxisImplement,
		AxisFlow:
		return true
	default:
		return false
	}
}

func DiagramSyntaxProfileFor(kind DiagramKind) DiagramSyntaxProfile {
	switch kind {
	case DiagramFlow:
		return DiagramSyntaxProfile{
			Kind:            kind,
			MermaidFamilies: []MermaidSyntaxFamily{MermaidSyntaxFlow},
		}
	case DiagramSequence:
		return DiagramSyntaxProfile{
			Kind:            kind,
			MermaidFamilies: []MermaidSyntaxFamily{MermaidSyntaxSequence},
		}
	case DiagramCallDAG:
		return DiagramSyntaxProfile{
			Kind:            kind,
			MermaidFamilies: []MermaidSyntaxFamily{MermaidSyntaxFlow},
			CodeEndpoint:    true,
		}
	case DiagramArchitecture:
		return DiagramSyntaxProfile{
			Kind:            kind,
			MermaidFamilies: []MermaidSyntaxFamily{MermaidSyntaxFlow},
		}
	}
	return DiagramSyntaxProfile{Kind: kind}
}

// DiagramKindAllowsMermaidSyntax reports whether a declared semantic
// kind is compatible with the Mermaid directive detected in the body.
// DiagramNone is intentionally permissive: it means the model omitted
// the semantic family, not that the body is invalid.
func DiagramKindAllowsMermaidSyntax(kind DiagramKind, family MermaidSyntaxFamily) bool {
	if family == MermaidSyntaxUnknown {
		return true
	}
	if kind == DiagramNone {
		return family != MermaidSyntaxUnsupported
	}
	profile := DiagramSyntaxProfileFor(kind)
	for _, allowed := range profile.MermaidFamilies {
		if family == allowed {
			return true
		}
	}
	return false
}

// DiagramKindUsesCodeEndpoints centralizes the "diagram nodes are code
// symbols" decision. This is intentionally diagram-kind based: a
// component-level relation_kind=call in an architecture / flow /
// sequence diagram does not by itself make the visible node ids code
// symbols.
func DiagramKindUsesCodeEndpoints(kind DiagramKind) bool {
	return DiagramSyntaxProfileFor(kind).CodeEndpoint
}

// MermaidBodySyntaxFamily returns the Mermaid syntax family declared by
// the first non-empty, non-comment line. Known Mermaid families that the
// current validator cannot parse are marked unsupported rather than
// silently accepted.
func MermaidBodySyntaxFamily(body string) MermaidSyntaxFamily {
	for _, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" || strings.HasPrefix(lower, "%%") {
			continue
		}
		switch {
		case strings.HasPrefix(lower, "sequencediagram"):
			return MermaidSyntaxSequence
		case lower == "flowchart" ||
			strings.HasPrefix(lower, "flowchart ") ||
			strings.HasPrefix(lower, "flowchart\t") ||
			lower == "graph" ||
			strings.HasPrefix(lower, "graph "):
			return MermaidSyntaxFlow
		case hasKnownUnsupportedMermaidDirective(lower):
			return MermaidSyntaxUnsupported
		default:
			return MermaidSyntaxUnknown
		}
	}
	return MermaidSyntaxUnknown
}

func hasKnownUnsupportedMermaidDirective(line string) bool {
	directives := []string{
		"classdiagram",
		"statediagram",
		"erdiagram",
		"journey",
		"gantt",
		"pie",
		"gitgraph",
		"mindmap",
		"timeline",
		"quadrantchart",
		"requirementdiagram",
		"c4context",
		"c4container",
		"c4component",
		"c4dynamic",
		"xychart",
		"sankey",
		"block-beta",
		"packet-beta",
	}
	for _, directive := range directives {
		if line == directive ||
			strings.HasPrefix(line, directive+" ") ||
			strings.HasPrefix(line, directive+"\t") ||
			strings.HasPrefix(line, directive+"-") {
			return true
		}
	}
	return false
}

func MermaidSyntaxRepairAdvice(kind DiagramKind, family MermaidSyntaxFamily) string {
	switch family {
	case MermaidSyntaxSequence:
		return "set diagram.kind=sequence, or rewrite diagram.body as flowchart/graph syntax if the answer needs a flow, architecture, or call_dag diagram"
	case MermaidSyntaxFlow:
		return "set diagram.kind to flow, architecture, or call_dag, or rewrite diagram.body as sequenceDiagram syntax if the answer needs a sequence diagram"
	case MermaidSyntaxUnsupported:
		if kind == DiagramSequence {
			return "rewrite diagram.body as sequenceDiagram syntax, or omit the diagram when the requested diagram type is not supported by the current answer contract"
		}
		return "rewrite diagram.body as flowchart/graph syntax, or omit the diagram when the requested diagram type is not supported by the current answer contract"
	default:
		return "rewrite diagram.body so its Mermaid directive matches the declared semantic diagram.kind"
	}
}
