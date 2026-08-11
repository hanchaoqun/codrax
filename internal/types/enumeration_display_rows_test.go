package types

import (
	"strings"
	"testing"
)

func TestEnumerationDisplaySetAuthorizesPrincipalContract_ExactRowsButNeverInventsRelationAuthority(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:    AnswerAggregateMemberSet,
		Label:   "enum types",
		Value:   "2",
		Members: []string{"Intent", "Scenario"},
	}
	set := EnumerationDisplaySet{Rows: []EnumerationDisplayRow{
		{Member: "Intent", HasCitation: true, Source: "analysis_ir.go", LineStart: 10},
		{Member: "Scenario", HasCitation: true, Source: "analysis_ir.go", LineStart: 20},
	}}
	plain := &RequestModel{Intent: IntentEnumerate, Predicates: SemanticPredicates{IsCategoryEnumeration: true}}
	if !EnumerationDisplaySetAuthorizesPrincipalContract(plain, fact, set) {
		t.Fatal("ordinary enumeration with exact support for every row should be answer-grade")
	}

	relation := *plain
	relation.Predicates.IsRelationalLookup = true
	if EnumerationDisplaySetAuthorizesPrincipalContract(&relation, fact, set) {
		t.Fatal("individually cited nodes must not prove a relation/call-chain contract")
	}
}

func TestEnumerationDisplaySetSelectionFamily_RequiresUnanimousTypedRowFamily(t *testing.T) {
	tests := []struct {
		name string
		rows []EnumerationDisplayRow
		want string
	}{
		{
			name: "unanimous family survives richer row terms",
			rows: []EnumerationDisplayRow{
				{SurfaceTerms: []string{"public class", "public class Animal"}},
				{SurfaceTerms: []string{"public class", "public class Service", "public abstract class"}},
			},
			want: "public class",
		},
		{
			name: "mixed typed families fail closed",
			rows: []EnumerationDisplayRow{
				{SurfaceTerms: []string{"public class", "public class Animal"}},
				{SurfaceTerms: []string{"foreign func", "foreign func native_add"}},
			},
		},
		{
			name: "missing typed family fails closed",
			rows: []EnumerationDisplayRow{
				{SurfaceTerms: []string{"public class", "public class Animal"}},
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := enumerationDisplaySetSelectionFamily(tt.rows); got != tt.want {
				t.Fatalf("selection family = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompileEnumerationDisplaySets_MultiCategoryRowsPreserveRichNotes(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{
			{
				Kind:        AnswerAggregateMemberSet,
				Label:       "公开类型",
				Value:       "2",
				Role:        AnswerAggregateRolePrincipalAnswer,
				Unit:        "类型",
				Members:     []string{"Kind", "Env"},
				SupportRefs: []string{"Kind @ internal/analysis/criterion/grammar.go:26", "Env @ internal/analysis/criterion/grammar.go:124"},
			},
			{
				Kind:        AnswerAggregateMemberSet,
				Label:       "公开函数",
				Value:       "1",
				Role:        AnswerAggregateRolePrincipalAnswer,
				Unit:        "函数",
				Members:     []string{"Eval"},
				SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
			},
		},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-kind",
				Kind:            EvidenceDirect,
				Subject:         "Kind",
				AnchorSymbol:    "Kind",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       26,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "Kind 是 Criterion 的公开类型别名，用于统一承载所有判定种类。",
			},
			{
				ID:              "ev-env",
				Kind:            EvidenceDirect,
				Subject:         "Env",
				AnchorSymbol:    "Env",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       124,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "Env 聚合评估 Criterion 时需要的运行环境。",
			},
			{
				ID:              "ev-eval",
				Kind:            EvidenceDirect,
				Subject:         "Eval",
				AnchorSymbol:    "Eval",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/eval.go",
				LineStart:       15,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "Eval 对单个 Criterion 进行求值并返回 Result。",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 2 {
		t.Fatalf("sets=%d, want 2: %+v", len(sets), sets)
	}
	if sets[0].Label != "公开类型" || len(sets[0].Rows) != 2 {
		t.Fatalf("first set = %+v", sets[0])
	}
	kind := sets[0].Rows[0]
	if kind.DisplayLabel != "Kind" ||
		kind.Location != "internal/analysis/criterion/grammar.go:26" ||
		kind.ClaimForm != ClaimDefinitionFact ||
		!strings.Contains(kind.Note, "公开类型别名") {
		t.Fatalf("Kind row lost identity/location/note: %+v", kind)
	}
	eval := sets[1].Rows[0]
	if eval.Category != "函数" || !strings.Contains(eval.Note, "单个 Criterion") {
		t.Fatalf("Eval row should preserve category and rich note: %+v", eval)
	}
}

func TestCompileEnumerationDisplaySets_DiagnosticMechanismSupportOnly(t *testing.T) {
	rm := &RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
			Confidence:          0.9,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码"},
			Confidence:                          0.8,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "诊断机制关键点",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"ErrStreamFirstByteTimeout", "canUseFinalizerOutputAfterTransientProgress"},
			SupportRefs: []string{
				"ErrStreamFirstByteTimeout @ internal/llm/stream_errors.go:78",
				"canUseFinalizerOutputAfterTransientProgress @ internal/orchestrator/orchestrator.go:7506",
			},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "timeout",
				Kind:            EvidenceDirect,
				Subject:         "ErrStreamFirstByteTimeout",
				AnchorSymbol:    "ErrStreamFirstByteTimeout",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/llm/stream_errors.go",
				LineStart:       78,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "first-byte timeout sentinel",
			},
			{
				ID:              "transient",
				Kind:            EvidenceMechanism,
				Subject:         "canUseFinalizerOutputAfterTransientProgress",
				AnchorSymbol:    "canUseFinalizerOutputAfterTransientProgress",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/orchestrator/orchestrator.go",
				LineStart:       7506,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "transient progress preservation guard",
			},
		},
	}

	if ShouldCompileEnumerationDisplaySetsForRequest(*rm) {
		t.Fatal("diagnostic/current-source mechanism shape must not compile principal enumeration rows")
	}
	if sets := CompileEnumerationDisplaySets(rm, plan); len(sets) != 0 {
		t.Fatalf("diagnostic mechanism support member_set must stay support-only, got %+v", sets)
	}
}

func TestCompileEnumerationDisplaySets_CallChainRelationNeedsExplicitSetBoundary(t *testing.T) {
	memberFact := AnswerAggregateFact{
		Kind:    AnswerAggregateMemberSet,
		Label:   "observed call-chain nodes",
		Value:   "3",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Controller.create", "Service.run", "Repository.insert"},
	}
	narrative := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		Predicates: SemanticPredicates{
			IsRelationalLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqCallChain)},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{memberFact}}

	if ShouldCompileEnumerationDisplaySetsForRequest(narrative) {
		t.Fatal("narrative call-chain aggregates are model exploration context, not deterministic principal enumeration authority")
	}
	if got := CompileEnumerationDisplaySets(&narrative, plan); len(got) != 0 {
		t.Fatalf("narrative call-chain aggregate must not compile system-authored visible rows: %+v", got)
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(plan.StableAggregateFacts, &narrative); len(got) != 0 {
		t.Fatalf("narrative call-chain aggregate must not become a hard principal roster: %+v", got)
	}

	explicit := narrative
	explicit.Intent = IntentEnumerate
	if !ShouldCompileEnumerationDisplaySetsForRequest(explicit) {
		t.Fatal("explicit relation enumeration must retain deterministic principal-row support")
	}
}

func TestCompileEnumerationDisplaySets_SourceInventorySuppressesUnrequestedValues(t *testing.T) {
	rm := &RequestModel{
		Language: "zh",
		Intent:   IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType, AnswerCandidateRoleConstant},
			TypeUnderlying:    SourceInventoryTypeUnderlyingString,
			RequiresConstSet:  true,
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
			},
			Confidence: 0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "公开字符串枚举类型",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"AnswerCandidateRole", "AnswerSymbolVisibility"},
			MemberNotes: []string{
				"AnswerCandidateRole 枚举 — 候选符号的角色分类（function/method/type/constant/variable/field/package/file/test/generated/private/documentation）",
				"AnswerSymbolVisibility 枚举 — 决定 private/internal 符号是否可作为 principal member，包含 public_exported / all / private_only 等 4 种范围常量。",
			},
			SupportRefs: []string{"AnswerCandidateRole @ internal/types/answer_candidate_role.go:9", "AnswerSymbolVisibility @ internal/types/answer_visibility_profile.go:7"},
		}},
	}
	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	joined := sets[0].Rows[0].Note + "\n" + sets[0].Rows[1].Note
	for _, banned := range []string{"function/method", "private"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("unrequested enum values / English visibility terms should not leak into source-inventory notes: %q", joined)
		}
	}
	if !strings.Contains(joined, "候选符号的角色分类") || !strings.Contains(joined, "非公开/内部") {
		t.Fatalf("sanitizer should preserve useful summary while localizing visibility wording: %q", joined)
	}
	if strings.Contains(joined, "public_exported") || strings.Contains(joined, "private_only") || strings.Contains(joined, "包含") {
		t.Fatalf("unrequested value-list clause should be removed: %q", joined)
	}
}

func TestCompileEnumerationDisplaySets_ConfigMappingDoesNotAuthorizeSystemRows(t *testing.T) {
	rm := &RequestModel{
		Intent:        IntentConfigQuery,
		Scenario:      ScenarioConfigTrace,
		PredicateAxis: AxisConfigure,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqConfigMapping)},
		Predicates:    SemanticPredicates{IsScalarAnswer: false},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "configuration layers",
		Value:   "3",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"code default", "config file", "runtime environment"},
	}}}
	if ShouldCompileEnumerationDisplaySetsForRequest(*rm) {
		t.Fatal("non-enumeration config mapping must not authorize system-authored principal rows")
	}
	if sets := CompileEnumerationDisplaySets(rm, plan); len(sets) != 0 {
		t.Fatalf("config mapping aggregate should stay model guidance, got %+v", sets)
	}
}

func TestCompileEnumerationDisplaySets_SourceInventoryRowAttributesPreservePackageDimension(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
			},
			Confidence: 0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Cangjie entrypoints",
			Value:       "1",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Unit:        "function",
			Members:     []string{"extend Cart"},
			SupportRefs: []string{"extend Cart @ src/cart/cart.cj:30"},
		}},
		SourceInventoryObservation: SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Scopes:   []string{"src"},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Count:    1,
				Members: []SourceInventoryObservationMember{{
					Name:          "extend Cart",
					File:          "src/cart/cart.cj",
					Line:          30,
					Language:      "cangjie",
					SurfaceTerms:  []string{"extend", "extend Cart"},
					CoverageState: SourceInventoryCoverageObserved,
					Attributes: []SourceInventoryObservationAttribute{{
						Name:          "demo.cart",
						Role:          AnswerCandidateRolePackage,
						File:          "src/cart/cart.cj",
						Language:      "cangjie",
						CoverageState: SourceInventoryCoverageObserved,
					}},
				}},
			}},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	attrs := sets[0].Rows[0].Attributes
	if len(attrs) != 1 || attrs[0].Role != AnswerCandidateRolePackage || attrs[0].Name != "demo.cart" {
		t.Fatalf("row package attribute not preserved: %+v", sets[0].Rows[0])
	}
	if got := strings.Join(sets[0].Rows[0].SurfaceTerms, "|"); !strings.Contains(got, "extend") || !strings.Contains(got, "extend Cart") {
		t.Fatalf("row-local source-inventory surface terms not preserved: %+v", sets[0].Rows[0].SurfaceTerms)
	}
}

func TestCompileEnumerationDisplaySets_PreservesPackageAttributeFromAlignedMemberNotes(t *testing.T) {
	rm := &RequestModel{
		Intent:   IntentEnumerate,
		Language: "zh",
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
				SourceInventoryFieldPackage,
			},
			Confidence: 0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "foreign func 声明",
			Value:       "2",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"foreign func native_add", "foreign func native_add"},
			SupportRefs: []string{"foreign func native_add: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6", "foreign func native_add: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6"},
			MemberNotes: []string{
				"foreign func native_add 声明，FFI 外部函数声明，package demo.bridge，两个文件均声明 native_add",
				"foreign func native_add 声明，FFI 外部函数声明，package demo.ffi",
			},
		}},
	}
	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	seen := map[string]string{}
	for _, row := range sets[0].Rows {
		if len(row.Attributes) != 1 {
			t.Fatalf("row should carry one typed package attribute: %+v", row)
		}
		seen[normalizeAnswerSupportLocation(row.Location)] = row.Attributes[0].Name
	}
	for loc, want := range map[string]string{
		"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6":                  "demo.bridge",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6": "demo.ffi",
	} {
		if got := seen[normalizeAnswerSupportLocation(loc)]; got != want {
			t.Fatalf("package for %s = %q, want %q; rows=%+v", loc, got, want, sets[0].Rows)
		}
	}
}

func TestCompileEnumerationDisplaySets_SourceInventoryAttributesStayLocationScopedForDuplicateLabels(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
			},
			Confidence: 0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "native declarations",
			Value: "2",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"native_add @ fixtures/bridge.cj:6",
				"native_add @ thirdparty/ffi.cj:6",
			},
			SupportRefs: []string{
				"native_add @ fixtures/bridge.cj:6",
				"native_add @ thirdparty/ffi.cj:6",
			},
		}},
		SourceInventoryObservation: SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Scopes:   []string{"."},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Count:    2,
				Members: []SourceInventoryObservationMember{
					{
						Name:          "native_add",
						File:          "fixtures/bridge.cj",
						Line:          6,
						Language:      "cangjie",
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Name:          "demo.bridge",
							Role:          AnswerCandidateRolePackage,
							File:          "fixtures/bridge.cj",
							Line:          4,
							Language:      "cangjie",
							CoverageState: SourceInventoryCoverageObserved,
						}},
					},
					{
						Name:          "native_add",
						File:          "thirdparty/ffi.cj",
						Line:          6,
						Language:      "cangjie",
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Name:          "demo.ffi",
							Role:          AnswerCandidateRolePackage,
							File:          "thirdparty/ffi.cj",
							Line:          4,
							Language:      "cangjie",
							CoverageState: SourceInventoryCoverageObserved,
						}},
					},
				},
			}},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	byLocation := map[string][]EnumerationDisplayRowAttribute{}
	for _, row := range sets[0].Rows {
		byLocation[row.Location] = row.Attributes
	}
	assertAttr := func(location, want string) {
		t.Helper()
		attrs := byLocation[location]
		if len(attrs) != 1 || attrs[0].Name != want {
			t.Fatalf("location %s attrs = %+v, want exactly %s", location, attrs, want)
		}
	}
	assertAttr("fixtures/bridge.cj:6", "demo.bridge")
	assertAttr("thirdparty/ffi.cj:6", "demo.ffi")
}

func TestCompileEnumerationDisplaySets_SurfaceTermsStayLineScopedForSameFileDuplicateLabels(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
				SourceInventoryFieldPackage,
			},
			SourceQuotes: []string{"extend", "public class"},
			Confidence:   0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "Cangjie declarations",
			Value: "2",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"Cart @ src/cart/Cart.cj:30 (package demo.cart)",
				"Cart @ src/cart/Cart.cj:14 (package demo.cart)",
			},
			SupportRefs: []string{
				"Cart @ src/cart/Cart.cj:30",
				"Cart @ src/cart/Cart.cj:14",
			},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "extend-cart",
				Kind:            EvidenceDirect,
				Subject:         "Cart",
				Object:          "extend Cart",
				AnchorSymbol:    "Cart",
				AnchorKind:      AnchorDefinition,
				Source:          "src/cart/Cart.cj",
				LineStart:       30,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				SurfaceTerms:    []string{"extend", "extend Cart"},
			},
			{
				ID:              "class-cart",
				Kind:            EvidenceDirect,
				Subject:         "Cart",
				Object:          "public class Cart",
				AnchorSymbol:    "Cart",
				AnchorKind:      AnchorDefinition,
				Source:          "src/cart/Cart.cj",
				LineStart:       14,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				SurfaceTerms:    []string{"public class", "public class Cart"},
			},
		},
		SourceInventoryObservation: SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Scopes:   []string{"src/cart"},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Count:    2,
				Members: []SourceInventoryObservationMember{
					{
						Name:          "Cart",
						File:          "src/cart/Cart.cj",
						Line:          30,
						Language:      "cangjie",
						SurfaceTerms:  []string{"extend", "extend Cart"},
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Name:          "demo.cart",
							Role:          AnswerCandidateRolePackage,
							File:          "src/cart/Cart.cj",
							Line:          1,
							Language:      "cangjie",
							CoverageState: SourceInventoryCoverageObserved,
						}},
					},
					{
						Name:          "Cart",
						File:          "src/cart/Cart.cj",
						Line:          14,
						Language:      "cangjie",
						SurfaceTerms:  []string{"public class", "public class Cart"},
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Name:          "demo.cart",
							Role:          AnswerCandidateRolePackage,
							File:          "src/cart/Cart.cj",
							Line:          1,
							Language:      "cangjie",
							CoverageState: SourceInventoryCoverageObserved,
						}},
					},
				},
			}},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	byLocation := map[string]EnumerationDisplayRow{}
	for _, row := range sets[0].Rows {
		byLocation[row.Location] = row
	}
	extendRow := byLocation["src/cart/Cart.cj:30"]
	if strings.Contains(strings.Join(extendRow.SurfaceTerms, "\n"), "public class") ||
		strings.Contains(extendRow.Note, "public class") {
		t.Fatalf("extend row inherited public-class terms from same-file class row: %+v", extendRow)
	}
	classRow := byLocation["src/cart/Cart.cj:14"]
	if strings.Contains(strings.Join(classRow.SurfaceTerms, "\n"), "extend") ||
		strings.Contains(classRow.Note, "extend") {
		t.Fatalf("class row inherited extend terms from same-file extend row: %+v", classRow)
	}
}

func TestCompileEnumerationDisplaySets_UsesCanonicalMembersAndSupportRefs(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "Kind 常量",
		Value: "2",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Unit:  "常量",
		Members: []string{
			"KindSymbolPresent: internal/analysis/criterion/grammar.go:29",
			"KindSymbolPresent",
			"KindNoCallSites: internal/analysis/criterion/grammar.go:30",
			"KindNoCallSites",
		},
		SupportRefs: []string{
			"",
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
		},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: facts,
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-symbol-present",
				Kind:            EvidenceDirect,
				Subject:         "KindSymbolPresent",
				AnchorSymbol:    "KindSymbolPresent",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       29,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "KindSymbolPresent 对应 CritSymbolPresent，用于判定目标符号是否出现。",
			},
			{
				ID:              "ev-no-call-sites",
				Kind:            EvidenceDirect,
				Subject:         "KindNoCallSites",
				AnchorSymbol:    "KindNoCallSites",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       30,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "KindNoCallSites 对应 CritNoCallSites，用于表达没有调用点的判定。",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	if sets[0].Rows[0].Member != "KindSymbolPresent" ||
		sets[0].Rows[0].Location != "internal/analysis/criterion/grammar.go:29" ||
		strings.Contains(sets[0].Rows[0].Location, "kindsymbolpresent:") ||
		!strings.Contains(sets[0].Rows[0].Note, "CritSymbolPresent") {
		t.Fatalf("first row did not keep canonical identity/support/note split: %+v", sets[0].Rows[0])
	}
	if sets[0].Rows[1].Member != "KindNoCallSites" ||
		sets[0].Rows[1].Location != "internal/analysis/criterion/grammar.go:30" ||
		!strings.Contains(sets[0].Rows[1].Note, "没有调用点") {
		t.Fatalf("second row did not keep canonical identity/support/note split: %+v", sets[0].Rows[1])
	}
}

func TestCompileEnumerationDisplaySets_PreservesSameLabelDistinctSourceLocations(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "foreign declarations",
		Value: "2",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Unit:  "symbol",
		Members: []string{
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6 (package demo.ffi)",
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6 (package demo.bridge)",
		},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: facts,
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-ffi",
				Kind:            EvidenceDirect,
				Subject:         "native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func native_add is declared in package demo.ffi.",
			},
			{
				ID:              "ev-bridge",
				Kind:            EvidenceDirect,
				Subject:         "native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func native_add is declared in package demo.bridge.",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("same-label members at distinct locations must produce two rows, got %+v", sets)
	}
	if sets[0].Value != "2" || sets[0].Rows[0].DisplayLabel != "native_add" || sets[0].Rows[1].DisplayLabel != "native_add" {
		t.Fatalf("row display labels/count drifted: %+v", sets[0])
	}
	gotLocations := []string{sets[0].Rows[0].Location, sets[0].Rows[1].Location}
	if !stringSliceContains(gotLocations, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6") ||
		!stringSliceContainsFold(gotLocations, "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6") {
		t.Fatalf("rows should keep both source locations, got %+v", gotLocations)
	}
}

func TestCompileEnumerationDisplaySets_PreservesDecoratedPackageAttributesWithoutSourceInventory(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
				SourceInventoryFieldPackage,
			},
			Confidence: 0.95,
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "foreign func 声明",
		Value: "2",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Unit:  "function",
		Members: []string{
			"native_add (internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6, package demo.ffi)",
			"native_add (eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6, package demo.bridge)",
		},
		SupportRefs: []string{
			"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
			"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	sets := CompileEnumerationDisplaySets(rm, &AnswerSurfacePlan{StableAggregateFacts: facts})
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("decorated package rows should compile two rows, got %+v", sets)
	}
	byLocation := map[string][]EnumerationDisplayRowAttribute{}
	for _, row := range sets[0].Rows {
		byLocation[row.Location] = row.Attributes
	}
	for location, wantPackage := range map[string]string{
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6": "demo.ffi",
		"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6":                  "demo.bridge",
	} {
		attrs := byLocation[location]
		if len(attrs) != 1 || attrs[0].Role != AnswerCandidateRolePackage || attrs[0].Name != wantPackage {
			t.Fatalf("location %s package attribute = %+v, want %s", location, attrs, wantPackage)
		}
	}
}

func TestCompileEnumerationDisplaySets_JoinsRequestedPackageFromGroundedSameFileDeclaration(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields: []SourceInventoryRequestedField{
				SourceInventoryFieldName,
				SourceInventoryFieldLocation,
				SourceInventoryFieldPackage,
			},
			Confidence: 0.95,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "public classes",
			Value:       "1",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"public class Bridge"},
			SupportRefs: []string{"public class Bridge: fixtures/bridge/Bridge.cj:15"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "class-bridge",
				Subject:         "public class Bridge",
				Object:          "demo.bridge",
				Source:          "fixtures/bridge/Bridge.cj",
				LineStart:       15,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "Bridge",
				GroundingStatus: GroundingGrounded,
				Scope:           ScopeLine,
			},
			{
				ID:              "package-bridge",
				Subject:         "package",
				Source:          "fixtures/bridge/Bridge.cj",
				LineStart:       4,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "demo.bridge",
				GroundingStatus: GroundingGrounded,
				Scope:           ScopeLine,
			},
			{
				ID:              "package-wrong-file",
				Subject:         "package",
				Source:          "fixtures/other/Other.cj",
				LineStart:       2,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "demo.bridge",
				GroundingStatus: GroundingGrounded,
				Scope:           ScopeLine,
			},
			{
				ID:              "package-ungrounded",
				Subject:         "package",
				Source:          "fixtures/bridge/Bridge.cj",
				LineStart:       1,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "demo.unverified",
				GroundingStatus: GroundingUngrounded,
				Scope:           ScopeLine,
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	attrs := sets[0].Rows[0].Attributes
	if len(attrs) != 1 {
		t.Fatalf("row attributes = %+v, want one grounded package attribute", attrs)
	}
	if got := attrs[0]; got.Role != AnswerCandidateRolePackage || got.Name != "demo.bridge" ||
		got.Location != "fixtures/bridge/Bridge.cj:4" {
		t.Fatalf("joined package attribute = %+v", got)
	}
}

func TestCompileEnumerationDisplaySets_ImportPathSuffixDisambiguatesSameTail(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "internal imports",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Unit:    "package",
			Members: []string{"internal/tool/repomap/types", "internal/types"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-repomap-types",
				Kind:            EvidenceDirect,
				AnchorSymbol:    "github.com/hanchaoqun/codrax/internal/tool/repomap/types",
				AnchorKind:      AnchorImport,
				Source:          "internal/agent/explorer.go",
				LineStart:       28,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "仓库映射类型定义包，别名 repotypes",
			},
			{
				ID:              "ev-types",
				Kind:            EvidenceDirect,
				AnchorSymbol:    "github.com/hanchaoqun/codrax/internal/types",
				AnchorKind:      AnchorImport,
				Source:          "internal/agent/explorer.go",
				LineStart:       29,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "通用类型定义包",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	if got := sets[0].Rows[0]; got.Member != "internal/tool/repomap/types" ||
		got.Location != "internal/agent/explorer.go:28" ||
		!strings.Contains(got.Note, "仓库映射类型") {
		t.Fatalf("repomap/types row matched the wrong import evidence: %+v", got)
	}
	if got := sets[0].Rows[1]; got.Member != "internal/types" ||
		got.Location != "internal/agent/explorer.go:29" ||
		!strings.Contains(got.Note, "通用类型") {
		t.Fatalf("internal/types row must not fall back to ambiguous tail `types`: %+v", got)
	}
}

func TestCompileEnumerationDisplaySets_DedupAppendsSameAnchorSummaries(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind 常量",
			Value:       "1",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Unit:        "常量",
			Members:     []string{"KindSymbolPresent"},
			SupportRefs: []string{"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-symbol-present-a",
				Kind:            EvidenceDirect,
				Subject:         "KindSymbolPresent",
				AnchorSymbol:    "KindSymbolPresent",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       29,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "KindSymbolPresent = Kind(types.CritSymbolPresent)",
			},
			{
				ID:              "ev-symbol-present-b",
				Kind:            EvidenceDirect,
				Subject:         "KindSymbolPresent",
				AnchorSymbol:    "KindSymbolPresent",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       29,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "read-mode Kind: 检查符号是否存在于证据槽或答案列表",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	note := sets[0].Rows[0].Note
	for _, want := range []string{"Kind(types.CritSymbolPresent)", "检查符号是否存在"} {
		if !strings.Contains(note, want) {
			t.Fatalf("compiled row note lost same-anchor summary %q: %q", want, note)
		}
	}
}

func TestCompileEnumerationDisplaySets_MergesSameAnchorSummariesAcrossTypedFields(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "公开函数",
			Value:       "1",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Unit:        "函数",
			Members:     []string{"Eval"},
			SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-eval-definition",
				Kind:            EvidenceDirect,
				Subject:         "Eval",
				AnchorSymbol:    "Eval",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/eval.go",
				LineStart:       15,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "Eval 对单个 Criterion 求值。",
			},
			{
				ID:              "ev-eval-result",
				Kind:            EvidenceDirect,
				Subject:         "criterion evaluator",
				Object:          "Eval",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/eval.go",
				LineStart:       15,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "未知 Kind 通过 Result.UnknownKind 报告。",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	note := sets[0].Rows[0].Note
	for _, want := range []string{"单个 Criterion", "Result.UnknownKind"} {
		if !strings.Contains(note, want) {
			t.Fatalf("compiled row note lost same-anchor typed-field summary %q: %q", want, note)
		}
	}
}

func TestCompileEnumerationDisplaySets_DedupesEquivalentPrincipalFactRefs(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateGroupedCount,
				Label:   "Kind const 成员数量",
				Value:   "2",
				Role:    AnswerAggregateRolePrincipalAnswer,
				Unit:    "个",
				Members: []string{"KindSymbolPresent", "KindNoCallSites"},
			},
			{
				Kind:        AnswerAggregateMemberSet,
				Label:       "Kind 常量成员",
				Value:       "2",
				Role:        AnswerAggregateRolePrincipalAnswer,
				Unit:        "个",
				Members:     []string{"KindSymbolPresent", "KindNoCallSites"},
				SupportRefs: []string{"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29", "KindNoCallSites @ internal/analysis/criterion/grammar.go:30"},
			},
		},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-symbol-present",
				Kind:            EvidenceDirect,
				Subject:         "KindSymbolPresent",
				AnchorSymbol:    "KindSymbolPresent",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       29,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "KindSymbolPresent 对应 CritSymbolPresent。",
			},
			{
				ID:              "ev-no-call-sites",
				Kind:            EvidenceDirect,
				Subject:         "KindNoCallSites",
				AnchorSymbol:    "KindNoCallSites",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/analysis/criterion/grammar.go",
				LineStart:       30,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "KindNoCallSites 对应 CritNoCallSites。",
			},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 {
		t.Fatalf("equivalent principal member carriers should compile once, got %+v", sets)
	}
	if sets[0].Label != "Kind 常量成员" || len(sets[0].Rows) != 2 {
		t.Fatalf("member_set carrier with support refs should win, got %+v", sets[0])
	}
}

func TestCompileEnumerationDisplaySets_PreservesSameMemberAtDistinctSourceLocations(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
	}
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateMemberSet,
		Label: "foreign func 声明（Cangjie）",
		Value: "2",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6 (package demo.bridge)",
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6 (package demo.ffi)",
		},
		SupportRefs: []string{
			"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
		},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts failed: %v", err)
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: facts,
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "ev-fixture-native-add",
				Kind:            EvidenceDirect,
				Subject:         "Bridge.cj",
				Object:          "foreign func native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func 声明，属于 package demo.bridge",
			},
			{
				ID:              "ev-corpus-native-add",
				Kind:            EvidenceDirect,
				Subject:         "07_foreign_ffi.cj",
				Object:          "foreign func native_add",
				AnchorSymbol:    "native_add",
				AnchorKind:      AnchorDefinition,
				Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
				LineStart:       6,
				Scope:           ScopeLine,
				GroundingStatus: GroundingGrounded,
				Summary:         "foreign func 声明，属于 package demo.ffi",
			},
		},
		SourceInventoryObservation: SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Count:    2,
				Members: []SourceInventoryObservationMember{
					{
						Name:          "native_add",
						Role:          AnswerCandidateRoleFunction,
						File:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
						Line:          6,
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Role: AnswerCandidateRolePackage,
							Name: "demo.bridge",
							File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
							Line: 1,
						}},
					},
					{
						Name:          "native_add",
						Role:          AnswerCandidateRoleFunction,
						File:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
						Line:          6,
						CoverageState: SourceInventoryCoverageObserved,
						Attributes: []SourceInventoryObservationAttribute{{
							Role: AnswerCandidateRolePackage,
							Name: "demo.ffi",
							File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
							Line: 1,
						}},
					},
				},
			}},
		},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("same-name declarations should compile as distinct rows: %+v", sets)
	}
	seen := map[string]string{}
	rowIDs := map[string]bool{}
	for _, row := range sets[0].Rows {
		if rowIDs[row.RowID] {
			t.Fatalf("same-name declaration rows must not share row_id: %+v", sets[0].Rows)
		}
		rowIDs[row.RowID] = true
		seen[normalizeAnswerSupportLocation(row.Location)] = row.Attributes[0].Name
		if row.Member != "native_add" {
			t.Fatalf("member label should remain stable while location disambiguates, got %+v", row)
		}
	}
	for loc, wantPackage := range map[string]string{
		"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6":                  "demo.bridge",
		"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6": "demo.ffi",
	} {
		if gotPackage := seen[normalizeAnswerSupportLocation(loc)]; gotPackage != wantPackage {
			t.Fatalf("location %s package = %q, want %q; rows=%+v", loc, gotPackage, wantPackage, sets[0].Rows)
		}
	}
}

func TestCompileEnumerationDisplaySets_AppendsExtractorRationaleAsEnrichment(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "Kind 常量",
			Value:       "1",
			Role:        AnswerAggregateRolePrincipalAnswer,
			Unit:        "常量",
			Members:     []string{"KindSymbolPresent"},
			SupportRefs: []string{"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29"},
		}},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "ev-symbol-present",
			Kind:            EvidenceDirect,
			Subject:         "KindSymbolPresent",
			AnchorSymbol:    "KindSymbolPresent",
			AnchorKind:      AnchorDefinition,
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       29,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "Kind常量，值=Kind(types.CritSymbolPresent)，第1个Kind常量",
		}},
		StepBackbone: []StepSurfaceAnchor{{
			Name:      "KindSymbolPresent",
			File:      "internal/analysis/criterion/grammar.go",
			Line:      29,
			Rationale: "Kind常量，符号存在于答案中",
		}},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	note := sets[0].Rows[0].Note
	for _, want := range []string{"Kind(types.CritSymbolPresent)", "符号存在于答案中"} {
		if !strings.Contains(note, want) {
			t.Fatalf("compiled row note lost extractor rationale %q: %q", want, note)
		}
	}
}

func TestCompileEnumerationDisplaySets_PreservesNonFileRows(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "运行时模式",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Unit:    "模式",
			Members: []string{"read", "write"},
		}},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	for _, row := range sets[0].Rows {
		if row.HasCitation || row.Location != "" || row.CitationKey != "" {
			t.Fatalf("non-file row should not fake a citation: %+v", row)
		}
	}
}

func TestCompileEnumerationDisplaySets_PositionalBareSupportRefsCarryLocations(t *testing.T) {
	for _, raw := range []string{"cmd/root.go:88", "codrax.yaml.example:485", "cmd/root.go:649"} {
		if got, ok := ParseAnswerSourceLocationSurface(raw); !ok {
			t.Fatalf("ParseAnswerSourceLocationSurface(%q) failed: %+v", raw, got)
		}
	}
	rm := &RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioConfigTrace,
		Predicates: SemanticPredicates{
			HasPerMemberTable: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqConfigMapping),
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "pipeline_max_steps 配置优先级（后者覆盖前者）",
			Value:   "3",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"代码默认值", "codrax.yaml", "CLI --pipeline-max-steps"},
			SupportRefs: []string{
				"cmd/root.go:88",
				"codrax.yaml.example:485",
				"cmd/root.go:649",
			},
		}},
	}
	normalized := NormalizeAnswerAggregateMemberSetSurfaces(plan.StableAggregateFacts)
	if got := normalized[0].SupportRefs; len(got) != 3 {
		t.Fatalf("normalized support refs = %#v", got)
	}
	if source, line, location := aggregateMemberStructuredLocation(normalized[0], 0, normalized[0].Members[0]); source != "cmd/root.go" || line != 88 || location != "cmd/root.go:88" {
		t.Fatalf("aggregateMemberStructuredLocation = %q:%d %q; fact=%+v", source, line, location, normalized[0])
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 3 {
		t.Fatalf("sets = %+v", sets)
	}
	for i, want := range []string{"cmd/root.go:88", "codrax.yaml.example:485", "cmd/root.go:649"} {
		if got := sets[0].Rows[i].Location; got != want {
			t.Fatalf("row %d location = %q, want %q; row=%+v", i, got, want, sets[0].Rows[i])
		}
	}
}

func TestCompileEnumerationDisplaySets_FileMembersBindSupportRefsByPathNotPosition(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "changed source files",
			Value: "3",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"internal/tool/trace_query.go",
				"internal/agent/answer_document_trace_decision_handoff.go",
				"internal/tool/trace_query_target_wait_preview_test.go",
			},
			// This is a general mechanism-anchor roster, not a member-aligned
			// array: two anchors belong to trace_query.go, one to the handoff,
			// and the test file has no line anchor.
			SupportRefs: []string{
				"internal/tool/trace_query.go:4380",
				"internal/tool/trace_query.go:7907",
				"internal/agent/answer_document_trace_decision_handoff.go:92",
			},
		}},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "ev-query-call",
			Kind:            EvidenceDirect,
			Source:          "internal/tool/trace_query.go",
			LineStart:       4380,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "query summary calls the preview helper",
		}, {
			ID:              "ev-query-helper",
			Kind:            EvidenceDirect,
			Source:          "internal/tool/trace_query.go",
			LineStart:       7907,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "preview helper definition",
		}, {
			ID:              "ev-handoff",
			Kind:            EvidenceDirect,
			Source:          "internal/agent/answer_document_trace_decision_handoff.go",
			LineStart:       92,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "wakeup path authority guidance",
		}},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 3 {
		t.Fatalf("sets = %+v", sets)
	}
	if got := sets[0].Rows[0].Location; got != "internal/tool/trace_query.go:4380" {
		t.Fatalf("trace_query row location = %q, want same-file positional anchor", got)
	}
	if got := sets[0].Rows[1].Location; got != "internal/agent/answer_document_trace_decision_handoff.go:92" {
		t.Fatalf("handoff row location = %q, want same-file non-positional anchor", got)
	}
	if row := sets[0].Rows[2]; row.Location != "" || row.HasCitation || row.Source != "" {
		t.Fatalf("unanchored test-file row must stay uncited instead of borrowing another file: %+v", row)
	}
	if strings.Contains(sets[0].Rows[1].Note, "preview helper") ||
		!strings.Contains(sets[0].Rows[1].Note, "wakeup path authority") {
		t.Fatalf("handoff row inherited a cross-file note: %+v", sets[0].Rows[1])
	}
}

func TestCompileEnumerationDisplaySets_RuntimeArtifactDoesNotPromoteFramePathToCurrentCitation(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "同时出错的 goroutine",
			Value:   "1",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"goroutine 15 @ internal/agent/analyzer.go:100"},
			Dimensions: []AnswerAggregateDimension{
				{Name: "evidence_origin", Value: "runtime_artifact"},
			},
		}},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "ev_code_same_path",
			Kind:            EvidenceDirect,
			Subject:         "writeSession",
			AnchorSymbol:    "writeSession",
			AnchorKind:      AnchorDefinition,
			Source:          "internal/agent/analyzer.go",
			LineStart:       100,
			Summary:         "当前源码里同名函数的定义，不等于运行时日志帧本身。",
			GroundingStatus: GroundingGrounded,
		}},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("sets = %+v", sets)
	}
	row := sets[0].Rows[0]
	if row.DisplayLabel != "goroutine 15" {
		t.Fatalf("display label should keep the artifact member label, got %+v", row)
	}
	if row.HasCitation || row.Source != "" || row.LineStart != 0 || row.Location != "" {
		t.Fatalf("runtime artifact member must not become a current-source citation: %+v", row)
	}
	if len(row.EvidenceOrigins) != 1 || row.EvidenceOrigins[0] != AnswerEvidenceOriginRuntimeArtifact {
		t.Fatalf("runtime artifact origin not preserved on display row: %+v", row.EvidenceOrigins)
	}
}

func TestCompileEnumerationDisplaySets_CrossLanguageSupportRefs(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "跨语言入口",
			Value:   "5",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Unit:    "入口",
			Members: []string{"IndexRender", "BridgeSum", "NativeSum", "JavaHook", "CangjieRun"},
			SupportRefs: []string{
				"IndexRender @ entry/src/main/ets/pages/Index.ets:42",
				"BridgeSum @ src/bridge/Bridge.ts:18",
				"NativeSum @ src/native/sum.cpp:9",
				"JavaHook @ java/com/example/Hook.java:31",
				"CangjieRun @ src/main/cj/demo/run.cj:27",
			},
		}},
	}

	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 5 {
		t.Fatalf("sets = %+v", sets)
	}
	for _, row := range sets[0].Rows {
		if !row.HasCitation || row.Location == "" {
			t.Fatalf("cross-language support_ref did not compile into a citable row: %+v", row)
		}
	}
}
