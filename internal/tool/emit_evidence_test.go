package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newEmitCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func TestEmitEvidence_AcceptsValidBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "Foo", "object": "bar", "source": "internal/agent/foo.go", "line_start": 12, "summary": "Foo registers bar", "anchor_kind": "call", "anchor_symbol": "Register"},
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	for _, it := range got {
		if it.Producer != EmitEvidenceProducer {
			t.Errorf("producer = %q, want %q", it.Producer, EmitEvidenceProducer)
		}
		if it.ID == "" {
			t.Errorf("missing stable ID")
		}
		if it.LineEnd == 0 {
			t.Errorf("LineEnd should default to LineStart")
		}
	}
	if got[0].Kind != types.EvidenceRegistration {
		t.Errorf("kind = %q", got[0].Kind)
	}
	if got[0].Predicate != "registration" {
		t.Errorf("predicate default failed: %q", got[0].Predicate)
	}
}

func TestEmitEvidence_RejectsUnknownKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"speculation","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "unknown evidence_kind") {
		t.Errorf("error msg should mention unknown evidence_kind, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_RejectsEvidenceKindValueInsideAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"evidence_kind":"direct","source":"x.go","line_start":12,"anchor_kind":"direct","anchor_symbol":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "belongs to evidence_kind, not anchor_kind") {
		t.Fatalf("expected targeted anchor_kind guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_ParametersExposeEvidenceKindNotLegacyKind(t *testing.T) {
	tool := &EmitEvidence{}
	schema := string(tool.Parameters())
	if !strings.Contains(schema, `"evidence_kind"`) {
		t.Fatalf("schema should expose evidence_kind: %s", schema)
	}
	if strings.Contains(schema, `"required":["kind"`) || strings.Contains(schema, `"required": ["kind"`) {
		t.Fatalf("schema should not require legacy kind field: %s", schema)
	}
}

func TestEmitEvidence_DemotesDocumentationStyleExactConfigMention(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "defines",
				"source": "docs/analysis_contract.md",
				"line_start": 367,
				"summary": "The token appears in a comment example only.",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "illustrative_only"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with demotion feedback, got: %s", res.Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("summary should steer the model away from repairing doc mentions, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should mark the mention non-repairable, got: %s", got[0].GroundingNote)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("drop-only illustrative mention must not queue structured repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_QueuesStructuredRepairTargetsForRecoveredEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "conditional",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "fullScan",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 148,
				"summary": "full scan branch",
				"anchor_kind": "call",
				"anchor_symbol": "fullScan"
			},
			{
				"kind": "conditional",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "loadFromCache",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 156,
				"summary": "cache branch",
				"anchor_kind": "call",
				"anchor_symbol": "loadFromCache"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with structured repair feedback, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "evidence_line_text_repair" {
		t.Fatalf("expected structured line-text repair metadata, got %+v", res.Repair)
	}
	if len(res.Repair.Targets) != 1 {
		t.Fatalf("expected one grouped repair target, got %+v", res.Repair.Targets)
	}
	target := res.Repair.Targets[0]
	if target.File != "internal/tool/repomap/tool.go" {
		t.Fatalf("repair target file = %q, want internal/tool/repomap/tool.go", target.File)
	}
	if !reflect.DeepEqual(target.Lines, []int{148, 156}) {
		t.Fatalf("repair target lines = %v, want [148 156]", target.Lines)
	}
	if target.Action != string(types.RepairReadFile) {
		t.Fatalf("repair target action = %q, want %q", target.Action, types.RepairReadFile)
	}
}

func TestEmitEvidence_PreservesValidatedDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "rootFlags",
				"predicate": "config",
				"source": "cmd/root.go",
				"line_start": 1381,
				"summary": "CLI override applies when non-nil.",
				"anchor_kind": "assignment",
				"anchor_symbol": "opts",
				"diagram_role_hint": "override"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("diagram role = %q, want override", got[0].DiagramRole)
	}
}

func TestEmitEvidence_PrefersMutableExactContextFilesForDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.RepoRoot = `C:\Users\ssccv\codrax`
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		EvidencePlan: types.EvidencePlan{
			RequiredFiles: []string{"internal/context/builder.go"},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "DefaultExploreHeuristics",
				"predicate": "defines",
				"source": "C:\\Users\\ssccv\\codrax\\internal\\types\\config.go",
				"line_start": 707,
				"summary": "code-default lineage anchor",
				"anchor_kind": "definition",
				"anchor_symbol": "DefaultExploreHeuristics",
				"context_role_hint": "related_context",
				"diagram_role_hint": "default"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Source != "internal/types/config.go" {
		t.Fatalf("source = %q, want canonical repo-relative path", got[0].Source)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleDefault {
		t.Fatalf("diagram role = %q, want default", got[0].DiagramRole)
	}
	if strings.Contains(got[0].GroundingNote, "outside the current same-scope required files") {
		t.Fatalf("grounding note should not treat the mutable exact-context file as out of scope, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_AcceptsLegacyYAMLDiagramRoleAliasForConfigFiles(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 怎么生效？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "ExploreHeuristics",
				"predicate": "documents",
				"source": "codrax.json.example",
				"line_start": 20,
				"summary": "config precedence comment",
				"anchor_kind": "definition",
				"anchor_symbol": "ExploreHeuristics",
				"context_role_hint": "related_context",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want canonical config role", got[0].DiagramRole)
	}
}

func TestEmitEvidence_DropsConfigTraceDiagramRoleForOutOfScopeHelper(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 是怎么生效的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "collectConfigTraceDiagramAnchors",
				"predicate": "assembles",
				"source": "internal/context/builder.go",
				"line_start": 1763,
				"summary": "helper for answer-document prompt assembly",
				"anchor_kind": "definition",
				"anchor_symbol": "collectConfigTraceDiagramAnchors",
				"context_role_hint": "related_context",
				"diagram_role_hint": "runtime"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for out-of-scope helper evidence", got[0].DiagramRole)
	}
}

func TestEmitEvidence_DoesNotInferDiagramRoleWithoutHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario:      types.ScenarioConfigTrace,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore",
				"predicate": "documents",
				"source": "codrax.yaml.example",
				"line_start": 20,
				"summary": "yaml precedence comment",
				"anchor_kind": "definition",
				"anchor_symbol": "explore"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown without explicit hint", got[0].DiagramRole)
	}
	if !strings.Contains(res.Summary, "Config-precedence task detected") {
		t.Fatalf("summary should nudge explicit diagram_role_hint usage, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DropsInconsistentDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "LoadRuntimeConfig",
				"predicate": "binds",
				"source": "internal/config/runtime.go",
				"line_start": 194,
				"summary": "runtime binding layer",
				"anchor_kind": "assignment",
				"anchor_symbol": "resolved",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for inconsistent hint", got[0].DiagramRole)
	}
	if !strings.Contains(got[0].GroundingNote, "diagram_role_hint=config was ignored") {
		t.Fatalf("grounding note should explain why the role hint was ignored, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(got[0].GroundingNote, "`default` / `runtime` / `override`") {
		t.Fatalf("grounding note should point at valid code-layer roles, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(res.Summary, "diagram_role_hint=config was ignored") {
		t.Fatalf("tool summary should surface the ignored-role guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigCommentLineBecomesIllustrativeOnly(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[codrax.yaml.example: showing lines 22-22 of 801]\n" +
					"  22│ #   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags\n",
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "documents precedence comment",
				"anchor_kind": "definition",
				"anchor_symbol": "Precedence",
				"context_role_hint": "related_context",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only for comment-only config line", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for comment-only config line", got[0].DiagramRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("illustrative config comment should be marked non-repairable, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should tell the model to drop illustrative config comments, got: %s", res.Summary)
	}
}

func TestEmitEvidence_KeepsFreeformExactMentionAsRelatedContextWithoutAnchoredTargetWindow(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 42,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "nearby context only") {
		t.Fatalf("freeform exact mention should stay nearby-context only, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "nearby context only") {
		t.Fatalf("tool summary should still surface nearby-context guidance, got: %q", res.Summary)
	}
}

func TestEmitEvidence_PromotesAbsenceSupportWhenAnchoredWindowNamesExactTarget(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[internal/agent/router.go: showing lines 40-42 of 60]\n" +
					"  40│func buildFallbackHandlers() {\n" +
					"  41│    // FooHandler is absent from the runtime router on this branch.\n" +
					"  42│    _ = 0\n",
			},
		},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 40,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "absence support") {
		t.Fatalf("anchored exact mention should be promoted to absence_support, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "absence support") {
		t.Fatalf("evidence summary should stay semantic; absence-support guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "absence support") {
		t.Fatalf("tool summary should still surface absence-support guidance, got: %q", res.Summary)
	}
}

func TestEmitEvidence_DowngradesIllustrativeExactTargetMentionWithoutPendingFinding(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "appears_in",
				"source": "internal/skill/analysis_contract.go",
				"line_start": 367,
				"summary": "analysis_contract.go uses explore_mid_loop_hint_budget only as a docstring example",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "absence_support"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if got[0].Kind != types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("illustrative exact-target mention should be downgraded to unresolved/ungrounded, got kind=%q grounding=%q", got[0].Kind, got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "illustrative mention of the exact") {
		t.Fatalf("grounding note should explain the downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesNegativeExactProbeDefiningHintToAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "RuntimeSettings",
				"predicate": "does not bind",
				"object": "explore_mid_loop_hint_budget",
				"source": "internal/config/runtime.go",
				"line_start": 231,
				"summary": "RuntimeSettings does not bind the missing exact key",
				"anchor_kind": "definition",
				"anchor_symbol": "RuntimeSettings",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative exact-probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesSameFamilyNegativeProbeDefiningHintToRelatedContext(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "ExploreHeuristics",
				"predicate": "does not define",
				"object": "mid_loop_hint_budget",
				"source": "internal/types/config.go",
				"line_start": 627,
				"summary": "ExploreHeuristics does not define a mid_loop_hint_budget field",
				"anchor_kind": "definition",
				"anchor_symbol": "ExploreHeuristics",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative same-family probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesCrossFamilyDefiningHintDuringExactAbsence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "different nearby config family") {
		t.Fatalf("evidence summary should stay semantic; cross-family guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
}

func TestEmitEvidence_KeepsExactTargetPendingWithoutAnalyzerUnverifiedWhenNoDefiningProofExists(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 鐨勬渶缁堟湁鏁堝€兼槸鎬庝箞璁＄畻鍑烘潵鐨勶紵",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want cleared while the exact target remains unresolved", got[0].DiagramRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_RejectsUnknownTopLevelField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"evidence":[{"kind":"direct","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown top-level field")
	}
	if !strings.Contains(res.Summary, "invalid params") {
		t.Errorf("got: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsUnknownItemField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","note":"hi"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown item field")
	}
	if !strings.Contains(res.Summary, "unknown field") {
		t.Errorf("error should mention unknown field: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsMissingSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","subject":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing source")
	}
}

func TestEmitEvidence_RejectsNonPathSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"FooBar"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: 'FooBar' has no slash or dot, not path-shaped")
	}
}

func TestEmitEvidence_RejectsStringLineStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":"twelve"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on string line_start")
	}
}

func TestEmitEvidence_RejectsRelationshipWithoutObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"A","source":"x.go","line_start":1}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: relationship needs object")
	}
}

func TestEmitEvidence_NormalizesCallDirectionToCallerCallee(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 149,
						To:   "fullScan",
						ToEP: repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"conditional","subject":"fullScan","predicate":"calls","object":"buildOrLoadGraph","source":"internal/tool/repomap/tool.go","line_start":149,"condition":"index.NeedsFullRescan(cacheDir)","summary":"If cache directory needs a full rescan, it calls fullScan.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Subject != "buildOrLoadGraph" {
		t.Fatalf("subject = %q, want buildOrLoadGraph", got[0].Subject)
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].Predicate != "calls" {
		t.Fatalf("predicate = %q, want calls", got[0].Predicate)
	}
}

func TestEmitEvidence_NormalizeCallDirection_DoesNotFallbackToWrongSameLineCall(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 147,
						To:   "NeedsFullRescan",
						ToEP: repomap.RelationEndpoint{Name: "NeedsFullRescan", Receiver: "index", File: "internal/tool/repomap/tool.go", Line: 147},
					},
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 149,
						To:   "fullScan",
						ToEP: repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"conditional","subject":"buildOrLoadGraph","predicate":"calls","object":"fullScan","source":"internal/tool/repomap/tool.go","line_start":147,"condition":"index.NeedsFullRescan(cacheDir)","summary":"Calls fullScan if no cache is found or a full scan is needed.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"if index.NeedsFullRescan(cacheDir) {\n    logging.Info(\"repo_map: full scan (%d files, no cache)\", len(entries))\n    return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].LineStart != 149 {
		t.Fatalf("line_start = %d, want recovery to 149", got[0].LineStart)
	}
}

// TestEmitEvidence_AutoSwapsLineEndBeforeStart pins the 2026-04-17
// change: line_end < line_start is treated as an obvious
// transposition typo and auto-swapped. The summary includes a
// "double-check the range" warning so the LLM can notice. Earlier
// behaviour rejected the whole batch over this single keystroke
// mistake; trace 1776439797257469553 iter=2 lost four otherwise-
// valid evidence items to a single typo on item[3].
func TestEmitEvidence_AutoSwapsLineEndBeforeStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{
		"kind":"direct","source":"x.go","line_start":50,"line_end":10,
		"anchor_kind":"definition","anchor_symbol":"Foo",
		"subject":"Foo","summary":"test"
	}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("auto-swap path must succeed, got rejection: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "AUTO-SWAPPED") {
		t.Errorf("summary must warn about auto-swap: %q", res.Summary)
	}
	// Item should be in the accepted buffer with line_start=10, line_end=50.
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 accepted item, got %d", len(emitted))
	}
	if emitted[0].LineStart != 10 || emitted[0].LineEnd != 50 {
		t.Errorf("swapped range wrong: got [%d, %d], want [10, 50]",
			emitted[0].LineStart, emitted[0].LineEnd)
	}
}

// TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch locks in
// the graceful-degrade behaviour: a single invalid item in a mostly-
// healthy batch skips that item and keeps the rest. Fires ONLY when
// the reject ratio is below 50%; above that the batch is rejected
// wholesale.
func TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 4 valid + 1 invalid (missing anchor_kind). 1/5 = 20% → sparse
	// reject path. Batch succeeds with 4 accepted; 1 skipped in the
	// Summary. (The healthy items all need required fields filled.)
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"B","subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"},
		{"kind":"direct","source":"d.go","line_start":4,"anchor_kind":"definition","anchor_symbol":"D","subject":"D","summary":"x"},
		{"kind":"direct","source":"e.go","line_start":5,"anchor_kind":"definition","anchor_symbol":"E","subject":"E","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sparse-reject batch must succeed, got rejection: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 4 {
		t.Errorf("expected 4 accepted, got %d", len(ctx.Mutable.EmittedEvidence()))
	}
	if !strings.Contains(res.Summary, "SKIPPED") {
		t.Errorf("summary must mention skipped item: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "items[2]") {
		t.Errorf("summary must identify the bad item index: %q", res.Summary)
	}
}

// TestEmitEvidence_MajorityRejectFailsEntireBatch — when ≥ 50% of
// items fail, the whole call fails with every reason listed in one
// error envelope. Keeps poisoned batches from slipping through.
func TestEmitEvidence_MajorityRejectFailsEntireBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 1 valid + 2 invalid (missing anchor_kind on two items).
	// 2/3 = 66% → majority reject path.
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("majority-reject batch must fail wholesale, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "2 of 3 items") {
		t.Errorf("summary must report the ratio: %q", res.Summary)
	}
}

func TestEmitEvidence_ConfigAbsentPrimaryMarksSubstituteEvidenceContextOnly(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: missingKey + " 在哪里定义？",
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{missingKey},
			Entities:        []string{missingKey},
			ExactTargets:    []string{missingKey},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	}}
	ctx.StageReports = []types.StageReport{{
		Stage:    types.StageAnalyze,
		Agent:    types.AgentAnalyzer,
		Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
	}}
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "AgentLoopMaxMidLoopInjects", "source": "internal/config/runtime.go", "line_start": 184, "summary": "related YAML key controls midloop injection budget", "anchor_kind": "definition", "anchor_symbol": "AgentLoopMaxMidLoopInjects"}
        ]
    }`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("related substitute evidence context role=%q, want related_context", got[0].ContextRole)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should suppress substitute repair loops: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should still steer away from repairing substitute exact hits, got: %q", res.Summary)
	}
}

func TestEmitEvidence_RejectsEmptyItems(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on empty items")
	}
}

func TestEmitEvidence_RejectsMissingMutable(t *testing.T) {
	tool := &EmitEvidence{}
	res, _ := tool.Execute(&types.BusContext{}, json.RawMessage(`{"items":[]}`))
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

func TestEmitEvidence_StableIDDedups(t *testing.T) {
	// Same content emitted twice produces identical IDs so the
	// downstream merger can dedup. Verify the IDs are stable.
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	body := `{"items":[{"kind":"direct","subject":"Foo","source":"a/b.go","line_start":7,"summary":"x","anchor_kind":"definition","anchor_symbol":"Foo"}]}`
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer (dedup happens later), got %d", len(got))
	}
	if got[0].ID == "" || got[0].ID != got[1].ID {
		t.Errorf("IDs should match for identical content: %q vs %q", got[0].ID, got[1].ID)
	}
}

func TestEmitEvidence_ResetEmittedEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	_, _ = tool.Execute(ctx, json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"X"}]}`))
	if len(ctx.Mutable.EmittedEvidence()) != 1 {
		t.Fatalf("expected 1 item before reset")
	}
	ctx.Mutable.ResetEmittedEvidence()
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("expected empty buffer after reset")
	}
}

func TestEmitEvidence_ToolSurface(t *testing.T) {
	tool := &EmitEvidence{}
	if tool.Name() != "emit_evidence" {
		t.Errorf("name = %q", tool.Name())
	}
	if tool.IsWrite() {
		t.Errorf("emit_evidence must not be write-classified")
	}
	if tool.Confidence() != 0 {
		t.Errorf("confidence = %f, want 0 (NonEvidenceTool — the tool itself isn't a fact source)", tool.Confidence())
	}
	if !strings.Contains(tool.Description(), "emit_evidence") &&
		!strings.Contains(tool.Description(), "structured evidence") {
		// loose check; the description must guide the LLM
		t.Errorf("description should explain the tool's purpose")
	}
	if !json.Valid(tool.Parameters()) {
		t.Errorf("parameters JSON schema is invalid")
	}
}
