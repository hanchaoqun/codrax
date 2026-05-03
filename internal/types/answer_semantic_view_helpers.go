package types

// Semantic-obligation helpers on AnswerSemanticView. These replace
// the legacy AnswerShape/RequiredAnswerShape switches scattered
// across extractor / explorer / finalizer / context builder. Every
// helper reads ONLY view's typed fields (no question-prose
// matching, no keyword tables, no shape enum) so the
// precise-signals-for-hard-gates red line is preserved.
//
// Naming convention: "Needs" = principal block obligation the
// answer MUST emit; "Allows" = optional surface the answer MAY
// emit when other conditions are met.

// NeedsPrincipalScalar reports whether the view requires a
// principal scalar payload — i.e. one BlockScalar (or equivalent)
// at SurfaceRoleHint=SurfacePrincipal carrying the resolved
// literal. Replaces the legacy ShapeValue / ShapeBoolean /
// ShapeConfigValue runtime branch.
func (v *AnswerSemanticView) NeedsPrincipalScalar() bool {
	if v == nil {
		return false
	}
	for _, br := range v.RequiredBlocks {
		if !br.Required {
			continue
		}
		if br.Kind != BlockScalar {
			continue
		}
		if br.SurfaceRoleHint == SurfacePrincipal {
			return true
		}
	}
	return false
}

// NeedsOrderedPrincipalList reports whether the view requires a
// principal ordered list — i.e. a BlockOrderedList at
// SurfaceRoleHint=SurfacePrincipal. Replaces the legacy
// ShapeStepList runtime branch.
func (v *AnswerSemanticView) NeedsOrderedPrincipalList() bool {
	if v == nil {
		return false
	}
	for _, br := range v.RequiredBlocks {
		if !br.Required {
			continue
		}
		if br.Kind != BlockOrderedList {
			continue
		}
		if br.SurfaceRoleHint == SurfacePrincipal {
			return true
		}
	}
	return false
}

// NeedsEnumerationSlate reports whether the view's family + block
// contract treat the principal payload as an enumeration of named
// anchors. Replaces the legacy ShapeListOfSymbols runtime branch.
//
// QFEnumeration is the canonical family for "list every X". The
// helper uses Family directly because the family enum is itself a
// precise-signal — the enumeration check from
// types/facet_plan.go::ResolveQuestionFamily reads only typed
// emit_analysis output (Intent / SemanticPredicates), no question
// prose.
func (v *AnswerSemanticView) NeedsEnumerationSlate() bool {
	if v == nil {
		return false
	}
	return v.Family == QFEnumeration
}

// NeedsBoundedMechanismList reports whether the view's family
// produces a bounded ordered hop list — i.e. call-chain / root-
// cause-trace style answers where the user typically declares a
// count boundary (前 N 步). Replaces ad-hoc shape gating in
// extractor / explorer "is this a step-list with declared count"
// checks.
func (v *AnswerSemanticView) NeedsBoundedMechanismList() bool {
	if v == nil {
		return false
	}
	switch v.Family {
	case QFRootCauseTrace, QFCallChain:
		return true
	}
	return false
}

// AllowsAnchorSkeleton reports whether an explanation-style answer
// may legitimately carry the optional symbols[] anchor skeleton.
// Reserved for multi-topic answers where each sub-topic gets one
// load-bearing anchor beneath the main prose. Single-topic
// explanations should keep symbols[] empty so auxiliary symbol
// validation cannot derail an otherwise valid narrative answer.
//
// The multi-topic signal comes from the analyzer's typed
// RequestModel.SubTopics (precise enum-derived signal); a
// principal scalar / ordered list excludes the anchor-skeleton
// path because those families already have their own principal
// payload type.
func (v *AnswerSemanticView) AllowsAnchorSkeleton(rm RequestModel) bool {
	if v == nil {
		return false
	}
	if v.NeedsPrincipalScalar() || v.NeedsOrderedPrincipalList() {
		return false
	}
	if v.Family == QFEnumeration {
		return false
	}
	return len(rm.SubTopics) > 1
}

// NeedsCitationFreeScalarIngest reports whether the prompt builder
// should surface the Raw Tool Outputs section so the LLM has
// visibility into command-level scalars (e.g. wc -l, grep -c)
// that the structured Evidence channel cannot carry. Replaces
// builder.go::isCitationFreeValueAnswer's legacy ShapeValue gate.
//
// Truth source: any view whose principal payload is a scalar
// literal benefits from raw-output access — config_value /
// role_lookup / explicit value answers all qualify.
func (v *AnswerSemanticView) NeedsCitationFreeScalarIngest() bool {
	if v == nil {
		return false
	}
	return v.NeedsPrincipalScalar()
}

// IRAllowsAnchorSkeleton is a free-standing convenience that builds
// a fresh view from the IR and asks AllowsAnchorSkeleton. Replaces
// the legacy types.ExplanationAllowsAnchorSkeleton helper. Callers
// that already have a compiled view should prefer
// view.AllowsAnchorSkeleton(rm) directly.
func IRAllowsAnchorSkeleton(ir *AnalysisIR) bool {
	if ir == nil {
		return false
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		return false
	}
	return view.AllowsAnchorSkeleton(ir.RequestModel)
}
