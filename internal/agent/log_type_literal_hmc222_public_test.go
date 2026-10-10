package agent

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func emitHMC222Log(t *testing.T, raw string, errors []map[string]any) *types.BusContext {
	t.Helper()
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "captured.log", Data: []byte(raw)}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(16384), Mutable: types.NewMutableState("inspect attached failures")}
	params, _ := json.Marshal(map[string]any{"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": errors})
	r := tool.NewRegistry()
	r.Register(&tool.EmitLogTriage{})
	result, err := r.Execute(bus, "emit_log_triage", params)
	if err != nil || !result.Success {
		t.Fatalf("public emit failed: %+v %v", result, err)
	}
	return bus
}

func TestHMC222LogTypeLiteralPublicRealSpellingAndCauseSurvive(t *testing.T) {
	for _, tc := range []struct{ typ, message, raw string }{
		{"runtime error: invalid memory address or nil pointer dereference", "invalid memory address or nil pointer dereference", "panic: runtime error: invalid memory address or nil pointer dereference\n[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x401000]\n"},
		{"java.lang.IllegalStateException", "reader unavailable", "Exception in thread \"main\" java.lang.IllegalStateException: reader unavailable\n\tat example.Reader.open(Reader.java:12)\n"},
		{"ZeroDivisionError", "division by zero", "Traceback (most recent call last):\n  File \"worker.py\", line 4, in parse\n    return 1 / 0\nZeroDivisionError: division by zero\n"},
		{"CatalogUnavailable", "no catalog installed", "CatalogUnavailable: no catalog installed\r\n"},
		{"FatalHeaderOnly", "", "FatalHeaderOnly\n"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			bus := emitHMC222Log(t, tc.raw, []map[string]any{{"type": tc.typ, "message": tc.message, "frames": []any{}}})
			bundle := bus.Mutable.LogTriage()
			seeds := types.CollectArtifactExternalObservationSeeds(bundle, nil, nil)
			literalSeeds := 0
			for _, seed := range seeds {
				if seed.Kind == "error_type" && seed.Raw == tc.typ {
					literalSeeds++
				}
			}
			if literalSeeds != 1 {
				t.Fatalf("native literal must retain exactly one compatible seed: %+v", seeds)
			}
			support := types.BuildAnswerSupportPlan(types.RequestModel{Intent: types.IntentRootCause, LogTriage: bundle}, &types.AnswerSurfacePlan{ExternalObservationSeeds: seeds})
			if support == nil {
				t.Fatal("observed artifact support plan lost")
			}
			literalRendered := false
			for _, lane := range support.Lanes {
				if lane.Kind != types.SupportLaneObservedArtifact {
					continue
				}
				for _, entry := range lane.Entries {
					if strings.Contains(entry.Text, "source-line literal \""+tc.typ+"\"") && strings.Contains(entry.Text, "diagnostic category not established") {
						literalRendered = true
					}
				}
			}
			if !literalRendered {
				t.Fatalf("native literal seed not rendered with its authority boundary: %+v", support.Lanes)
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "preserve their verified source-line spelling: `"+tc.typ+"`") {
				t.Fatalf("real source literal disappeared: %s", prompt)
			}
			pc := ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "answer-document"})
			var raw string
			for _, section := range pc.UserSections {
				raw += section.Content
			}
			if !strings.Contains(raw, "**"+tc.typ+"** [source literal]") || !strings.Contains(raw, "not its classification or role") {
				t.Fatalf("context lost literal/interpretation separation: %s", raw)
			}
		})
	}
	marker := "Caused by: example.InnerFault: inner failed"
	bus := emitHMC222Log(t, "example.OuterFault: outer failed\n    "+marker+"\n", []map[string]any{{
		"type": "example.OuterFault", "message": "outer failed", "frames": []any{},
		"cause":          map[string]any{"type": "example.InnerFault", "message": "inner failed", "frames": []any{}},
		"cause_relation": map[string]any{"authority": "explicit_artifact_marker", "marker": marker},
	}})
	e := bus.Mutable.LogTriage().Errors[0]
	if e.ObservedTypeLiteral() != "example.OuterFault" || e.Cause == nil || e.Cause.ObservedTypeLiteral() != "example.InnerFault" || e.CauseRelation == nil || e.CauseRelation.Marker != marker {
		t.Fatalf("real exception spelling or explicit causal marker changed: %+v", e)
	}
}

func TestHMC222LogTypeLiteralPublicUnobservedClassificationIsNotMandatory(t *testing.T) {
	bus := emitHMC222Log(t, "09:00 E AssetLoad: malformed timestamp\n09:01 lookup status=not_found\n", []map[string]any{
		{"type": "TimestampError", "message": "malformed timestamp", "frames": []any{}},
		{"type": "StorageLookup", "message": "lookup status=not_found", "frames": []any{}},
	})
	bundle := bus.Mutable.LogTriage()
	if len(bundle.Errors) != 2 || bundle.Errors[0].Type != "TimestampError" || bundle.Errors[1].Type != "StorageLookup" {
		t.Fatalf("soft diagnostic labels lost: %+v", bundle)
	}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "must mention literally in `summary`") || strings.Contains(prompt, "name each structured log error type or exception identifier from Log Triage") {
		t.Fatalf("model labels became mandatory source literals: %s", prompt)
	}
	pc := ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "answer-document"})
	var contextText string
	for _, section := range pc.UserSections {
		contextText += section.Content
	}
	if !strings.Contains(contextText, "**TimestampError** [diagnostic label (unverified)]") {
		t.Fatalf("soft diagnostic interpretation was lost or promoted: %s", contextText)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
	for _, row := range ledger.Records {
		if row.Subject == "TimestampError" || row.Subject == "StorageLookup" {
			t.Fatalf("diagnostic label became source fact: %+v", row)
		}
	}
	bindings := types.CompileRuntimeArtifactClaimBindings(&types.RequestModel{Intent: types.IntentRootCause, LogTriage: bundle}, nil)
	for _, binding := range bindings {
		if binding.TargetRef == "TimestampError" || binding.TargetRef == "StorageLookup" {
			t.Fatalf("diagnostic label became a claim target: %+v", binding)
		}
	}
}

func TestHMC222LogTypeLiteralPublicNativeQueryEmitRender(t *testing.T) {
	const raw = "10-09 09:00:00.003 10 11 E AssetLoad: catalog unavailable\n10-09 09:00:00.008 10 10 W Startup: request aborted\n"
	for _, label := range []string{"AssetLoad", "InventedLookupError"} {
		t.Run(label, func(t *testing.T) {
			bus := emitHMC222Log(t, raw, []map[string]any{{"type": label, "message": "catalog unavailable", "frames": []any{}}})
			bus.WorkDir = t.TempDir()
			bus.RepoRoot = bus.WorkDir
			query, err := (&tool.LogQuery{}).Execute(bus, json.RawMessage(`{}`))
			if err != nil || !query.Success {
				t.Fatalf("native query: %+v / %v", query, err)
			}
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, LogTriage: bus.Mutable.LogTriage(),
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1,
					Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1, Label: "original records"}}}}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
			bus.Mutable.SetRequestModel(rm)
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
			const explanation = "The original records are shown below. The cause remains unconfirmed."
			params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "interpretation", "kind": "summary", "text": explanation}}})
			out, err := (&tool.EmitAnswerDocument{}).Execute(bus, params)
			if err != nil || !out.Success {
				t.Fatalf("public final emission: %+v / %v", out, err)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			rows, tables := 0, 0
			for _, block := range doc.Blocks {
				if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() {
					tables++
					rows += len(block.RuntimeMeasurement.BoundTable.Rows)
				}
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "en"))
			if tables != 1 || rows != 2 || !strings.Contains(visible, explanation) || !strings.Contains(visible, "catalog unavailable") || !strings.Contains(visible, "request aborted") {
				t.Fatalf("native facts/explanation changed: %d tables/%d rows: %s", tables, rows, visible)
			}
			if strings.Contains(visible, "InventedLookupError") {
				t.Fatal("system inserted an unobserved diagnostic label into the final answer")
			}
		})
	}
}
