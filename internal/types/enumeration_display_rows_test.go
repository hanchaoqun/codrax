package types

import (
	"strings"
	"testing"
)

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

func TestCompileEnumerationDisplaySets_RuntimeArtifactDoesNotPromoteFramePathToCurrentCitation(t *testing.T) {
	rm := &RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
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
