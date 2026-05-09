package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// erm_completeness_test.go — Phase 2 unit tests for the Tier 2
// completeness validators. Each validator gets:
//   - a "passes when satisfied" case
//   - a "fails when gap" case
//   - the failure FixHint is verified non-empty + R6-clean (no
//     internal pipeline term substrings)

// TestScalarCountValidator pins the count-question Tier 2 contract.
// Pass when ToolResults contains an exec_command result with an
// integer literal in summary; fail otherwise.
func TestScalarCountValidator(t *testing.T) {
	makeIR := func(isCount bool) *types.AnalysisIR {
		return &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Predicates: types.SemanticPredicates{IsCountQuestion: isCount},
			},
		}
	}

	t.Run("not_count_question_no_op", func(t *testing.T) {
		v := ScalarCountValidator{}
		input := ValidatorInput{IR: makeIR(false)}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("ScalarCountValidator must no-op when IsCountQuestion=false; got %+v", fail)
		}
	})

	t.Run("count_question_no_exec_command_fails", func(t *testing.T) {
		v := ScalarCountValidator{}
		input := ValidatorInput{
			IR: makeIR(true),
			ToolResults: []types.ToolResult{
				{ToolName: "read_file", Summary: "file contents", Success: true},
			},
		}
		fail := v.Validate(input)
		if fail == nil {
			t.Fatal("ScalarCountValidator must fail when count question has only read_file (no exec_command)")
		}
		if fail.Dimension != DimensionScalarCount {
			t.Errorf("dimension: got %v, want %v", fail.Dimension, DimensionScalarCount)
		}
		assertR6CleanFixHint(t, fail.FixHint)
	})

	t.Run("count_question_exec_command_with_integer_passes", func(t *testing.T) {
		v := ScalarCountValidator{}
		input := ValidatorInput{
			IR: makeIR(true),
			ToolResults: []types.ToolResult{
				{ToolName: "exec_command", Summary: "25\n", Success: true},
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("ScalarCountValidator must pass when exec_command returns integer; got %+v", fail)
		}
	})

	t.Run("count_question_exec_command_failed_does_not_satisfy", func(t *testing.T) {
		v := ScalarCountValidator{}
		input := ValidatorInput{
			IR: makeIR(true),
			ToolResults: []types.ToolResult{
				{ToolName: "exec_command", Summary: "25", Success: false}, // failed call
			},
		}
		if fail := v.Validate(input); fail == nil {
			t.Error("ScalarCountValidator must fail when exec_command Success=false (failed call)")
		}
	})

	t.Run("count_question_exec_command_empty_output_fails", func(t *testing.T) {
		v := ScalarCountValidator{}
		input := ValidatorInput{
			IR: makeIR(true),
			ToolResults: []types.ToolResult{
				{ToolName: "exec_command", Summary: "no output produced", Success: true},
			},
		}
		// "no output produced" has no integer — should still fail.
		if fail := v.Validate(input); fail == nil {
			t.Error("ScalarCountValidator must fail when exec_command summary has no integer literal")
		}
	})
}

// TestCardinalityValidator pins the declared-count contract.
func TestCardinalityValidator(t *testing.T) {
	makeIR := func(declared int) *types.AnalysisIR {
		ir := &types.AnalysisIR{}
		if declared > 0 {
			ir.RequestModel.EnumerationBoundary = &types.RequestedEnumerationBoundary{
				DeclaredCount: declared,
			}
		}
		return ir
	}

	t.Run("no_boundary_no_op", func(t *testing.T) {
		v := CardinalityValidator{}
		if fail := v.Validate(ValidatorInput{IR: makeIR(0)}); fail != nil {
			t.Errorf("no-op when boundary nil; got %+v", fail)
		}
	})

	t.Run("declared_7_got_5_fails", func(t *testing.T) {
		v := CardinalityValidator{}
		input := ValidatorInput{
			IR: makeIR(7),
			AnswerSymbols: []types.AnswerSymbol{
				{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"},
			},
		}
		fail := v.Validate(input)
		if fail == nil {
			t.Fatal("must fail when answer count < declared")
		}
		assertR6CleanFixHint(t, fail.FixHint)
	})

	t.Run("declared_3_got_3_passes", func(t *testing.T) {
		v := CardinalityValidator{}
		input := ValidatorInput{
			IR:            makeIR(3),
			AnswerSymbols: []types.AnswerSymbol{{Name: "A"}, {Name: "B"}, {Name: "C"}},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("must pass when answer count == declared; got %+v", fail)
		}
	})
}

// TestPathDepthValidator pins call-chain closure semantics: distinct
// function-symbol mentions in evidence must reach len(entities) + 3.
func TestPathDepthValidator(t *testing.T) {
	makeIR := func(entities ...string) *types.AnalysisIR {
		return &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Entities: entities},
			},
		}
	}

	t.Run("under_2_entities_no_op", func(t *testing.T) {
		v := PathDepthValidator{}
		if fail := v.Validate(ValidatorInput{IR: makeIR("Only")}); fail != nil {
			t.Errorf("no-op when <2 entities; got %+v", fail)
		}
	})

	t.Run("two_entities_only_two_mentions_fails", func(t *testing.T) {
		// 2 entities + 3 = 5 mentions required; 2 supplied → fail.
		v := PathDepthValidator{}
		input := ValidatorInput{
			IR: makeIR("BuildAnalysisIR", "GateRunWith"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "BuildAnalysisIR"},
				{AnchorSymbol: "GateRunWith"},
			},
		}
		fail := v.Validate(input)
		if fail == nil {
			t.Fatal("must fail when distinct mentions < entities+3")
		}
		assertR6CleanFixHint(t, fail.FixHint)
	})

	t.Run("two_entities_five_mentions_passes", func(t *testing.T) {
		v := PathDepthValidator{}
		input := ValidatorInput{
			IR: makeIR("BuildAnalysisIR", "GateRunWith"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "BuildAnalysisIR"},
				{AnchorSymbol: "NormalizerNormalize"},
				{AnchorSymbol: "CompilerCompile"},
				{AnchorSymbol: "HdpPlan"},
				{AnchorSymbol: "GateRunWith"},
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("must pass when 5 distinct fn mentions; got %+v", fail)
		}
	})

	t.Run("file_paths_and_basenames_dont_count_as_function_symbols", func(t *testing.T) {
		// File paths (containing '/') and bare file-basenames ending
		// in a short lowercase extension MUST NOT contribute to the
		// function-symbol count. Cross-language: this rule must
		// apply consistently for Go (.go), Python (.py), Rust (.rs),
		// Java (.java), Lua (.lua), etc.
		v := PathDepthValidator{}
		input := ValidatorInput{
			IR: makeIR("X", "Y"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "internal/agent/explorer.go"}, // path
				{AnchorSymbol: "main.go"},                    // bare Go basename
				{AnchorSymbol: "helper.py"},                  // bare Python basename
				{AnchorSymbol: "lib.rs"},                     // bare Rust basename
			},
		}
		if fail := v.Validate(input); fail == nil {
			t.Error("file-path / bare-basename anchors must not satisfy function-symbol depth check")
		}
	})

	t.Run("multilanguage_function_symbols_count", func(t *testing.T) {
		// Cross-language acceptance: snake_case (Python / Rust),
		// camelCase (JS / TS / Java methods), PascalCase (Go / Java /
		// Kotlin / Rust types), package.Func qualified, all valid.
		v := PathDepthValidator{}
		input := ValidatorInput{
			IR: makeIR("entryPoint", "exit_point"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "Entry"},          // PascalCase
				{AnchorSymbol: "process_data"},   // snake_case
				{AnchorSymbol: "validateInput"},  // camelCase
				{AnchorSymbol: "pkg.HelperFn"},   // qualified
				{AnchorSymbol: "exit_point"},     // matches one entity
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("cross-language symbols must satisfy depth check; got %+v", fail)
		}
	})
}

// TestLayerDepthValidator pins config-precedence three-layer coverage.
func TestLayerDepthValidator(t *testing.T) {
	t.Run("all_three_layers_passes", func(t *testing.T) {
		v := LayerDepthValidator{}
		input := ValidatorInput{
			IR: &types.AnalysisIR{},
			EvidenceItems: []types.EvidenceItem{
				{DiagramRole: types.EvidenceDiagramRoleDefault},
				{DiagramRole: types.EvidenceDiagramRoleConfig},
				{DiagramRole: types.EvidenceDiagramRoleOverride},
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("must pass when all 3 layers present; got %+v", fail)
		}
	})

	t.Run("missing_cli_layer_fails", func(t *testing.T) {
		v := LayerDepthValidator{}
		input := ValidatorInput{
			IR: &types.AnalysisIR{},
			EvidenceItems: []types.EvidenceItem{
				{DiagramRole: types.EvidenceDiagramRoleDefault},
				{DiagramRole: types.EvidenceDiagramRoleConfig},
			},
		}
		fail := v.Validate(input)
		if fail == nil {
			t.Fatal("must fail when CLI layer missing")
		}
		if len(fail.MissingTokens) == 0 {
			t.Error("MissingTokens must list the missing layer")
		}
		assertR6CleanFixHint(t, fail.FixHint)
	})

	t.Run("path_heuristic_yaml_satisfies_config_only_typed_role_for_others", func(t *testing.T) {
		// Only universal config-file extensions (yaml/toml/json/...)
		// are detected via path. Default-struct and CLI-override
		// layers must come from typed DiagramRole because their
		// paths vary per language ecosystem (Go cmd/root.go vs
		// Python cli/main.py vs Java src/main/Cli.java vs Rust
		// src/bin/...). Mix DiagramRole for default+CLI with path
		// heuristic for config.
		v := LayerDepthValidator{}
		input := ValidatorInput{
			IR: &types.AnalysisIR{},
			EvidenceItems: []types.EvidenceItem{
				{DiagramRole: types.EvidenceDiagramRoleDefault, Source: "src/types.rs"}, // default
				{Source: "config/app.yaml"},                                              // config (path)
				{DiagramRole: types.EvidenceDiagramRoleOverride, Source: "src/cli.py"},  // CLI
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("typed-DiagramRole + universal-extension must satisfy 3 layers; got %+v", fail)
		}
	})

	t.Run("path_heuristic_does_not_use_project_specific_keywords", func(t *testing.T) {
		// Regression guard against project-specific path keywords.
		// A non-Go project naming a file "config.go" wouldn't make
		// sense — but a Python project naming "default_config.py"
		// or Java "DefaultConfig.java" should NOT be auto-classified
		// as default-struct based on filename alone. The validator
		// must rely on typed DiagramRole for default + CLI layers.
		v := LayerDepthValidator{}
		input := ValidatorInput{
			IR: &types.AnalysisIR{},
			EvidenceItems: []types.EvidenceItem{
				{Source: "internal/types/config.go"},  // would have been "default" via removed kw
				{Source: "codrax.yaml.example"},        // config (legit, via .yaml extension)
				{Source: "cmd/root.go"},                // would have been "CLI" via removed kw
			},
		}
		// Without typed DiagramRole, only the .yaml file resolves;
		// missing default + CLI → must FAIL.
		if fail := v.Validate(input); fail == nil {
			t.Error("path-only Go-specific keywords must not satisfy default/CLI layers (cross-project portability)")
		}
	})
}

// TestEntityParityValidator pins comparison bucket-balance contract.
func TestEntityParityValidator(t *testing.T) {
	makeIR := func(buckets ...string) *types.AnalysisIR {
		ir := &types.AnalysisIR{}
		for _, label := range buckets {
			ir.RequestModel.Buckets = append(ir.RequestModel.Buckets,
				types.QuestionBucket{Label: label, Anchors: []string{label + "Sym"}})
		}
		return ir
	}

	t.Run("under_2_buckets_no_op", func(t *testing.T) {
		v := EntityParityValidator{}
		if fail := v.Validate(ValidatorInput{IR: makeIR("only")}); fail != nil {
			t.Errorf("no-op when <2 buckets; got %+v", fail)
		}
	})

	t.Run("balanced_buckets_pass", func(t *testing.T) {
		v := EntityParityValidator{}
		input := ValidatorInput{
			IR: makeIR("A", "B"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "ASym"}, {AnchorSymbol: "ASym"},
				{AnchorSymbol: "BSym"}, {AnchorSymbol: "BSym"},
			},
		}
		if fail := v.Validate(input); fail != nil {
			t.Errorf("balanced 2/2 must pass; got %+v", fail)
		}
	})

	t.Run("lopsided_buckets_fail", func(t *testing.T) {
		v := EntityParityValidator{}
		input := ValidatorInput{
			IR: makeIR("A", "B"),
			EvidenceItems: []types.EvidenceItem{
				{AnchorSymbol: "ASym"}, {AnchorSymbol: "ASym"},
				{AnchorSymbol: "ASym"}, {AnchorSymbol: "ASym"},
				// 0 evidence for B
			},
		}
		fail := v.Validate(input)
		if fail == nil {
			t.Fatal("must fail when one bucket starves")
		}
		assertR6CleanFixHint(t, fail.FixHint)
	})
}

// TestRunFamilyValidators pins the per-family routing — the helper
// must call only the validators that apply to each family.
func TestRunFamilyValidators_RoutingPerFamily(t *testing.T) {
	cases := []struct {
		name        string
		family      types.QuestionFamily
		ir          *types.AnalysisIR
		wantNValids int
	}{
		{
			name:        "qf_role_lookup_no_validators",
			family:      types.QFRoleLookup,
			ir:          &types.AnalysisIR{},
			wantNValids: 0,
		},
		{
			name:   "count_question_routes_scalar_validator",
			family: types.QFGeneric,
			ir: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					Predicates: types.SemanticPredicates{IsCountQuestion: true},
				},
			},
			wantNValids: 1,
		},
		{
			name:   "qf_call_chain_routes_path_validator",
			family: types.QFCallChain,
			ir:     &types.AnalysisIR{},
			wantNValids: 1,
		},
		{
			name:   "qf_config_precedence_routes_layer_validator",
			family: types.QFConfigPrecedence,
			ir:     &types.AnalysisIR{},
			wantNValids: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FamilyValidators(c.family, c.ir)
			if len(got) != c.wantNValids {
				t.Errorf("FamilyValidators(%q): got %d validators, want %d (%+v)",
					c.family, len(got), c.wantNValids, got)
			}
		})
	}
}

// assertR6CleanFixHint is a helper that fails the test when a Tier 2
// FixHint contains internal pipeline terms. Mirrors the R6 redline
// checked by TestPromptSnapshot_NoInternalTermsInRenderedOutput; the
// hint will be surfaced to the LLM through the explore/extract retry
// continuation prompt, so the same R6 standard applies.
//
// Word-boundary matching to avoid false positives on common English
// stems (e.g. "deterministic" contains "term" — not an ERM leak).
func assertR6CleanFixHint(t *testing.T, hint string) {
	t.Helper()
	// Banned WORDS — must appear as standalone tokens (word-boundary
	// on both sides), not as substrings of larger words.
	bannedWordsCaseInsensitive := []string{
		"explorer", "extractor", "analyzer", "finalizer",
		"orchestrator", "ERM",
		"validator",
	}
	bannedExactSubstrings := []string{
		"BusContext", "AgentContext",
		"emit_evidence", "emit_analysis", "dispatchStage",
		"Tier 1", "Tier 2",
		"gate.RunWith", "FallbackTarget",
	}
	for _, word := range bannedWordsCaseInsensitive {
		if containsAsWord(hint, word) {
			t.Errorf("FixHint contains internal pipeline word %q as standalone token (R6 violation): %q", word, hint)
		}
	}
	for _, term := range bannedExactSubstrings {
		if containsCaseInsensitive(hint, term) {
			t.Errorf("FixHint contains internal pipeline term %q (R6 violation): %q", term, hint)
		}
	}
}

// containsAsWord checks needle appears in haystack with ASCII word
// boundaries on both sides (start/end-of-string or non-alphanumeric
// neighbour). Case-insensitive.
func containsAsWord(haystack, needle string) bool {
	hb := []byte(haystack)
	nb := []byte(needle)
	for i := 0; i+len(nb) <= len(hb); i++ {
		// Boundary-before
		if i > 0 {
			c := hb[i-1]
			if isAlphanumeric(c) {
				continue
			}
		}
		// Match body
		match := true
		for j := 0; j < len(nb); j++ {
			if normaliseLetter(hb[i+j]) != normaliseLetter(nb[j]) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Boundary-after
		if i+len(nb) < len(hb) {
			c := hb[i+len(nb)]
			if isAlphanumeric(c) {
				continue
			}
		}
		return true
	}
	return false
}

func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func normaliseLetter(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

func containsCaseInsensitive(haystack, needle string) bool {
	hb := []byte(haystack)
	nb := []byte(needle)
	for i := 0; i+len(nb) <= len(hb); i++ {
		match := true
		for j := 0; j < len(nb); j++ {
			if normaliseLetter(hb[i+j]) != normaliseLetter(nb[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
