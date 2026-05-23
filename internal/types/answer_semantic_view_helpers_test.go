package types

import "testing"

func TestNeedsPrincipalScalar(t *testing.T) {
	tests := []struct {
		name string
		view *AnswerSemanticView
		want bool
	}{
		{name: "nil", view: nil, want: false},
		{
			name: "empty",
			view: &AnswerSemanticView{},
			want: false,
		},
		{
			name: "role_lookup compile produces principal scalar",
			view: compileRoleLookup(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "config_precedence compile produces principal scalar",
			view: compileConfigPrecedence(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "call_chain compile does not produce principal scalar",
			view: compileCallChain(&AnalysisIR{}, nil),
			want: false,
		},
		{
			name: "enumeration compile does not produce principal scalar",
			view: compileEnumeration(&AnalysisIR{}, nil),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.view.NeedsPrincipalScalar(); got != tt.want {
				t.Errorf("NeedsPrincipalScalar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsOrderedPrincipalList(t *testing.T) {
	tests := []struct {
		name string
		view *AnswerSemanticView
		want bool
	}{
		{name: "nil", view: nil, want: false},
		{
			name: "call_chain → ordered principal list",
			view: compileCallChain(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "root_cause_trace → ordered principal list",
			view: compileRootCauseTrace(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "enumeration → ordered principal list (slate)",
			view: compileEnumeration(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "role_lookup → no principal list",
			view: compileRoleLookup(&AnalysisIR{}, nil),
			want: false,
		},
		{
			name: "generic → no principal list",
			view: compileGeneric(&AnalysisIR{}, nil),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.view.NeedsOrderedPrincipalList(); got != tt.want {
				t.Errorf("NeedsOrderedPrincipalList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsEnumerationSlate(t *testing.T) {
	cases := map[QuestionFamily]bool{
		QFEnumeration:      true,
		QFRoleLookup:       false,
		QFConfigPrecedence: false,
		QFCallChain:        false,
		QFRootCauseTrace:   false,
		QFArchitecture:     false,
		QFGeneric:          false,
	}
	for fam, want := range cases {
		view := &AnswerSemanticView{Family: fam}
		if got := view.NeedsEnumerationSlate(); got != want {
			t.Errorf("Family=%s NeedsEnumerationSlate()=%v want %v", fam, got, want)
		}
	}
	if (*AnswerSemanticView)(nil).NeedsEnumerationSlate() {
		t.Errorf("nil view must not need enumeration slate")
	}
}

func TestNeedsBoundedMechanismList(t *testing.T) {
	cases := map[QuestionFamily]bool{
		QFRootCauseTrace:   true,
		QFCallChain:        true,
		QFEnumeration:      false,
		QFRoleLookup:       false,
		QFConfigPrecedence: false,
		QFArchitecture:     false,
		QFGeneric:          false,
	}
	for fam, want := range cases {
		view := &AnswerSemanticView{Family: fam}
		if got := view.NeedsBoundedMechanismList(); got != want {
			t.Errorf("Family=%s NeedsBoundedMechanismList()=%v want %v", fam, got, want)
		}
	}
}

func TestAllowsAnchorSkeleton(t *testing.T) {
	multiTopic := RequestModel{SubTopics: []SubTopic{{Summary: "a"}, {Summary: "b"}}}
	singleTopic := RequestModel{SubTopics: []SubTopic{{Summary: "a"}}}

	tests := []struct {
		name string
		view *AnswerSemanticView
		rm   RequestModel
		want bool
	}{
		{name: "nil view", view: nil, rm: multiTopic, want: false},
		{
			name: "generic + multi-topic",
			view: compileGeneric(&AnalysisIR{}, nil),
			rm:   multiTopic,
			want: true,
		},
		{
			name: "generic + single topic",
			view: compileGeneric(&AnalysisIR{}, nil),
			rm:   singleTopic,
			want: false,
		},
		{
			name: "architecture + multi-topic",
			view: compileArchitecture(&AnalysisIR{}, nil),
			rm:   multiTopic,
			want: true,
		},
		{
			name: "architecture + multi-topic scalar literals excluded",
			view: compileArchitecture(&AnalysisIR{}, nil),
			rm: RequestModel{
				Predicates: SemanticPredicates{IsScalarAnswer: true},
				SubTopics:  multiTopic.SubTopics,
			},
			want: false,
		},
		{
			name: "enumeration + multi-topic (excluded)",
			view: compileEnumeration(&AnalysisIR{}, nil),
			rm:   multiTopic,
			want: false,
		},
		{
			name: "role_lookup + multi-topic (excluded — principal scalar)",
			view: compileRoleLookup(&AnalysisIR{}, nil),
			rm:   multiTopic,
			want: false,
		},
		{
			name: "call_chain + multi-topic (excluded — ordered list)",
			view: compileCallChain(&AnalysisIR{}, nil),
			rm:   multiTopic,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.view.AllowsAnchorSkeleton(tt.rm); got != tt.want {
				t.Errorf("AllowsAnchorSkeleton() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiresAnchorSkeletonNarrowsOptionalArchitectureAnchors(t *testing.T) {
	multiTopic := RequestModel{SubTopics: []SubTopic{{Summary: "a"}, {Summary: "b"}}}
	arch := compileArchitecture(&AnalysisIR{}, nil)
	if !arch.AllowsAnchorSkeleton(multiTopic) {
		t.Fatal("architecture narratives may still render optional key anchors")
	}
	if arch.RequiresAnchorSkeleton(multiTopic) {
		t.Fatal("architecture narratives must not hard-block completion on analyzer sub-topic anchors")
	}
	generic := compileGeneric(&AnalysisIR{}, nil)
	if !generic.RequiresAnchorSkeleton(multiTopic) {
		t.Fatal("generic multi-topic explanations still require the anchor skeleton")
	}
	mechanism := RequestModel{
		Intent:        IntentExplain,
		PredicateAxis: AxisCondition,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		SubTopics:     []SubTopic{{Summary: "retry budget"}, {Summary: "stage output"}},
	}
	if generic.AllowsAnchorSkeleton(mechanism) || generic.RequiresAnchorSkeleton(mechanism) {
		t.Fatal("mechanism sub-topic decomposition must not require or invite an anchor skeleton")
	}
	history := RequestModel{
		Intent:        IntentExplain,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqHistory)},
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		SubTopics: []SubTopic{
			{Summary: "latest merge"},
			{Summary: "commit impact"},
		},
	}
	if !generic.AllowsAnchorSkeleton(history) {
		t.Fatal("history sub-topics should still be available as optional structure guidance")
	}
	if generic.RequiresAnchorSkeleton(history) {
		t.Fatal("history sub-topics must not force a current-source anchor skeleton")
	}
}

func TestNeedsCitationFreeScalarIngest(t *testing.T) {
	tests := []struct {
		name string
		view *AnswerSemanticView
		want bool
	}{
		{name: "nil", view: nil, want: false},
		{
			name: "config_precedence (principal scalar) → true",
			view: compileConfigPrecedence(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "role_lookup (principal scalar) → true",
			view: compileRoleLookup(&AnalysisIR{}, nil),
			want: true,
		},
		{
			name: "call_chain → false",
			view: compileCallChain(&AnalysisIR{}, nil),
			want: false,
		},
		{
			name: "enumeration → false",
			view: compileEnumeration(&AnalysisIR{}, nil),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.view.NeedsCitationFreeScalarIngest(); got != tt.want {
				t.Errorf("NeedsCitationFreeScalarIngest() = %v, want %v", got, tt.want)
			}
		})
	}
}
