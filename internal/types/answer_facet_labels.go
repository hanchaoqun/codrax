package types

import "strings"

var answerFacetPublicLabels = map[AnswerFacetKind]string{
	FacetObservedArtifactFact:    "Observed artifact fact (anchor every claim about the attached log/perf trace to a specific frame or marker)",
	FacetCurrentCodePath:         "Current code path (cite the live source file:line that proves what the code does today)",
	FacetNearestMechanism:        "Nearest mechanism (when the exact target is absent, name the closest grounded approximation and explain the gap)",
	FacetUncertaintyBoundary:     "Uncertainty boundary (name what was searched and what remained unverified rather than hedging silently)",
	FacetConfigPrecedenceRole:    "Config precedence role (cover each grounded layer once with the citation that proves the layer)",
	FacetResolvedLiteralOrSymbol: "Resolved literal or symbol (name the specific identifier the question is about with its file:line)",
	FacetEnumerationItem:         "Enumeration item (one item per element the user asked to enumerate)",
	FacetBucketLabel:             "Bucket label (each user-named partition appears verbatim somewhere in summary or per-item rationale)",
	FacetPrincipalPathEdge:       "Principal path edge (cite each call edge with the line that names both caller and callee)",
	FacetBranchGuard:             "Branch guard (cite the condition that gates a path; do not blend guard and action into one citation)",
	FacetComponentRelation:       "Component relation (cite the import / dependency edge with the line that names both endpoints)",
	FacetDiagramSpine:            "Diagram facet (every node grounded in a citation; relationships supported by typed claim annotations)",
}

// AnswerFacetPublicLabel returns the shared LLM-facing label for a
// facet kind. The fallback is the raw kind string so unknown future
// facets remain renderable even before a tailored label is added.
func AnswerFacetPublicLabel(kind AnswerFacetKind) string {
	if label, ok := answerFacetPublicLabels[kind]; ok {
		return label
	}
	return string(kind)
}

// AnswerFacetPublicLabelString is the string-keyed companion used by
// reviewer / validator surfaces that currently project facet ids as
// strings rather than the enum type.
func AnswerFacetPublicLabelString(kind string) string {
	return AnswerFacetPublicLabel(AnswerFacetKind(strings.TrimSpace(kind)))
}
