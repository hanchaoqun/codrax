package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This ordinary marker is deliberately not a special parsed metadata family.
// Field visibility must follow the fact's role, not a keyword/type exception.
func readerFieldScopePublicResult(t *testing.T) types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "records.ftrace")
	raw := "writer-101 (101) [001] .... 6.000000: tracing_mark_write: B|201|ReadBatch: result=E_BUSY, sample_count=9007199254740993\n" +
		"writer-101 (101) [001] .... 6.000010: tracing_mark_write: E|201\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "ReadBatch", "limit": 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("native query failed: %v %+v", err, result)
	}
	return result
}

func readerFieldScopePublicContext(lang string, scope types.RuntimeQuestionScope, results []types.ToolResult) *types.AgentContext {
	const request = "List the recorded result and sample_count with their source and units; explain the bounded evidence."
	rm := types.RequestModel{RawRequest: request, Language: lang, Intent: types.IntentTrace,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: scope, Confidence: 1,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOccurrenceTime}}}
	mu := types.NewMutableState(request)
	mu.SetRequestModel(rm)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "result=E_BUSY is a recorded business status, not a proved cause. 模型原文。"}}})
	bus := &types.BusContext{Language: lang, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}}}
	return ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
}

func readerFieldScopePublicSnapshot(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	data, err := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), ctx.AnalysisIR.RequestModel, ledger,
		types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.AnswerDocumentV2()})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReaderFieldScopePublicNativeFiniteInstruction(t *testing.T) {
	result := readerFieldScopePublicResult(t)
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeBoundedEffectVerdict} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx := readerFieldScopePublicContext(lang, scope, []types.ToolResult{result})
				before := readerFieldScopePublicSnapshot(t, ctx)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if readerFieldScopePublicSnapshot(t, ctx) != before {
					t.Fatal("display teaching changed raw evidence, request facets, authority, or model-owned prose")
				}
				views := traceEventInventoryPromptViews(t, prompt)
				if len(views) != 1 || views[0].Inventory.Coverage.MatchedTotal != 1 || !views[0].Inventory.RowsComplete || len(views[0].Inventory.Rows) != 1 {
					t.Fatalf("native inventory lost exact membership: %+v", views)
				}
				row := views[0].Inventory.Rows[0]
				if row.Line != 1 || row.EmitterTID != 101 || row.EmitterTGID != 101 || row.MarkerPID != 201 || row.TraceTimeSeconds != 6 || row.SourcePath != views[0].Source.Path ||
					!strings.Contains(row.Raw, "result=E_BUSY, sample_count=9007199254740993") {
					t.Fatalf("native raw fields, precision, identity, or source changed: %+v", row)
				}
				readerFieldScopePublicRejectOverbroadTeaching(t, prompt)
				marker := "## Reader-ready finite-window facts"
				ownership := "The system neither checks nor rewrites model prose and does not choose the conclusion"
				if lang == "zh" {
					marker = "## 有限窗口查询的读者事实卡"
					ownership = "系统不检查或修改模型正文，也不代替模型给结论"
				}
				if strings.Count(prompt, marker) != 1 || !strings.Contains(prompt, ownership) ||
					!strings.Contains(prompt, "Runtime finite-scope presentation hint") ||
					!strings.Contains(prompt, "does not decide yes/no/mixed/unproven for the model") {
					t.Fatal("finite card activation, model ownership, or causal boundary lost")
				}
				card := readerFieldScopePublicSection(t, prompt, marker)
				readerFieldScopePublicAssertTeaching(t, card, lang)
				vocabulary := "dimension names are reading aids, not a closed vocabulary"
				if lang == "zh" {
					vocabulary = "维度名称是阅读提示，不是封闭词表"
				}
				if !strings.Contains(card, vocabulary) {
					t.Fatal("finite card still closes the vocabulary over its own dimension labels")
				}
				readerFieldScopePublicAssertTeaching(t, readerFieldScopePublicSection(t, prompt, "- Runtime user-facing language hint:"), "en")
			})
		}
	}
}

func readerFieldScopePublicSection(t *testing.T, prompt, marker string) string {
	t.Helper()
	at := strings.Index(prompt, marker)
	if at < 0 {
		t.Fatalf("missing public instruction section %q", marker)
	}
	section := prompt[at:]
	endMarker := "\n## "
	if strings.HasPrefix(marker, "- ") {
		endMarker = "\n"
	}
	if end := strings.Index(section, endMarker); end >= 0 {
		section = section[:end]
	}
	return section
}

func readerFieldScopePublicAssertTeaching(t *testing.T, section, lang string) {
	t.Helper()
	wants := []string{
		"Do not expose internal protocol, validation, routing, or ranking-control field names, enum values, status codes, or key/value pairs",
		"relevant, evidence-supported raw-data field names, units, identifiers, and business statuses",
		"their presence in structured data does not make them internal metadata",
		"does not change a fact's source, scope, precision, or evidence strength",
		"grants no additional causal or current-source authority",
	}
	if lang == "zh" {
		wants = []string{
			"内部协议、校验、路由或排序控制所用的字段名、枚举值、状态码和机器键值",
			"与问题相关且证据支持的原始数据字段名、单位、标识和业务状态应保留并解释",
			"不因其出现在结构化数据中就视为内部元数据",
			"不改变事实的来源、范围、精度或证据强度",
			"不授予额外的因果或源码证明",
		}
	}
	for _, want := range wants {
		if !strings.Contains(section, want) {
			t.Errorf("reader section lost scoped vocabulary or authority boundary %q", want)
		}
	}
}

func readerFieldScopePublicRejectOverbroadTeaching(t *testing.T, prompt string) {
	t.Helper()
	for _, old := range []string{
		"字段名、枚举值、状态码和机器键值只用于校验",
		"Field names, enum values, status codes, and machine key/value pairs in earlier structured rows are validation metadata",
		"正文只使用本卡中的读者维度名称",
		"use only the reader dimension names and natural-language boundaries from this card",
		"structured keys and enum values are evidence metadata, not customer-facing vocabulary",
		"不要展示 JSON 字段名、内部枚举值或状态码",
		"do not expose JSON field names, internal enum values, status codes",
	} {
		if strings.Contains(prompt, old) {
			t.Errorf("system still treats raw data vocabulary as internal metadata: %q", old)
		}
	}
}

// The causal fixture is an existing typed producer receipt, not a new native
// causal proof. This companion only checks the sibling card's same teaching.
func TestReaderFieldScopePublicCausalInstruction(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := finalValueComponentContext(t, lang, []types.ObservationRecord{
				finalValueComponentRecord("reader-field-cause", "io_wait", "io_wait", "io_dependency", 2, 11, 11),
			})
			before := readerFieldScopePublicSnapshot(t, ctx)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			readerFieldScopePublicRejectOverbroadTeaching(t, prompt)
			marker := "## Reader-ready Trace facts"
			if lang == "zh" {
				marker = "## 面向读者的 Trace 成文事实卡"
			}
			if strings.Count(prompt, marker) != 1 || !strings.Contains(prompt, "principal_root_cause_population=`typed_on_chain_only`") {
				t.Fatal("causal card or existing on-chain authority boundary lost")
			}
			readerFieldScopePublicAssertTeaching(t, readerFieldScopePublicSection(t, prompt, marker), lang)
			readerFieldScopePublicAssertTeaching(t, readerFieldScopePublicSection(t, prompt, "- Runtime user-facing language hint:"), "en")
			if readerFieldScopePublicSnapshot(t, ctx) != before {
				t.Fatal("causal presentation changed rank values, evidence, or model prose")
			}
		})
	}
}

func TestReaderFieldScopePublicDoesNotActivateFiniteCardWithoutTypedScopeOrEvidence(t *testing.T) {
	result := readerFieldScopePublicResult(t)
	for _, tc := range []struct {
		name    string
		scope   types.RuntimeQuestionScope
		results []types.ToolResult
	}{
		{"non_finite", types.RuntimeQuestionScopeCausalDiagnosis, []types.ToolResult{result}},
		{"not_applicable", types.RuntimeQuestionScopeNotApplicable, []types.ToolResult{result}},
		{"no_evidence", types.RuntimeQuestionScopeBoundedFactSet, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := readerFieldScopePublicContext("en", tc.scope, tc.results)
			before := readerFieldScopePublicSnapshot(t, ctx)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "## Reader-ready finite-window facts") {
				t.Fatal("field visibility teaching activated a new finite fact scope")
			}
			if readerFieldScopePublicSnapshot(t, ctx) != before {
				t.Fatal("negative control changed underlying facts or model prose")
			}
		})
	}
}
