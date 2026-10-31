package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc327RequestText(capture *hmc326BudgetCapture) string {
	var b strings.Builder
	for _, m := range capture.messages {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHMC327FinalizerPublicBytePolicyCompatibility(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	before := readerFieldScopePublicSnapshot(t, ctx)
	for _, tc := range []struct {
		name     string
		settings types.AgentSettings
		enforce  bool
	}{
		{name: "unresolved legacy"},
		{name: "explicit advisory", settings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteLegacyAdvisory}},
		{name: "explicit enforcement", settings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}, enforce: true},
		{name: "invalid explicit policy", settings: types.AgentSettings{FinalizerRequestBytePolicy: "invalid"}, enforce: true},
		{name: "resolved production default", settings: types.ResolvedAgentSettings(types.AgentSettings{}), enforce: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured request")}, window: 1000}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			base := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1, AgentSettings: tc.settings}).(*BaseAgent)
			_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			eval := base.eval.(*answerDocumentEvaluator)
			if eval.promptBudget == nil || eval.promptBudget.RequestAllocationBytes != 3500 || eval.promptBudget.OutputKnown || eval.promptBudget.OutputReserveBytes != nil {
				t.Fatal("policy changed byte allocation or invented an output reservation")
			}
			var allocation *AnswerContextByteBudgetError
			if tc.enforce {
				if capture.calls != 0 || !errors.As(err, &allocation) || allocation.AllocatedBytes != 3500 || allocation.ActualBytes <= allocation.AllocatedBytes || eval.promptBudget.Policy != types.FinalizerRequestByteEnforce {
					t.Fatalf("explicit enforcement failed: calls=%d err=%v", capture.calls, err)
				}
			} else if capture.calls != 1 || !errors.Is(err, capture.stop) || errors.As(err, &allocation) || eval.promptBudget.Policy != types.FinalizerRequestByteLegacyAdvisory ||
				eval.promptBudget.DynamicPublishedBytes != len(hmc326DynamicRequestMessage(t, capture)) {
				t.Fatalf("legacy entry lost request or truthful planning receipt: calls=%d err=%v", capture.calls, err)
			}
			if before != readerFieldScopePublicSnapshot(t, ctx) || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("policy selection changed original facts or created an accepted answer")
			}
		})
	}
}

func TestHMC327FinalizerPublicPlanSharesOnlyPublishedInventorySources(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	rows := answerDocObservationPromptRecords(ctx, ledger.Records, answerDocObservationLedgerPromptLimit)
	var priorLines []string
	for _, tc := range []struct {
		name   string
		policy types.FinalizerRequestBytePolicy
	}{
		{name: "advisory", policy: types.FinalizerRequestByteLegacyAdvisory},
		{name: "enforcement", policy: types.FinalizerRequestByteEnforce},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured source-sharing request")}, window: 2000000}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			base := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
				AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: tc.policy}}).(*BaseAgent)
			_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			if capture.calls != 1 || !errors.Is(err, capture.stop) {
				t.Fatalf("sufficient allocation failed: %v", err)
			}
			prompt := hmc327RequestText(capture)
			views := traceEventInventoryPromptViews(t, prompt)
			if len(views) != 2 || !strings.Contains(prompt, "inventory_source_1: complete source of inventory observation_id=") {
				budget := base.eval.(*answerDocumentEvaluator).promptBudget
				t.Fatalf("complete inventory was not reused by the actual request: budget=%+v closed_plan=%+v", budget, budget.NativeLedgerPlan)
			}
			for _, view := range views {
				matched := false
				for _, r := range ledger.Records {
					if r.ID == view.ObservationID {
						matched = reflect.DeepEqual(view.Source, r.SourceRef) && reflect.DeepEqual(view.Inventory, r.EventSearchInventory) && view.PromptRowsOmitted == 0
					}
				}
				if !matched {
					t.Fatal("query/source/census borrowed from another inventory")
				}
			}
			var lines []string
			for _, row := range rows {
				prefix := fmt.Sprintf("- `%s`: origin=`%s`; producer=`%s`; source=", row.ID, row.Origin, row.Producer)
				start := strings.Index(prompt, prefix)
				if row.Claim == types.TraceEventSearchInventoryPredicate {
					if start >= 0 {
						t.Fatal("complete inventory metadata duplicated as a generic record")
					}
					continue
				}
				if start < 0 {
					t.Fatalf("independent generic record lost: %s", row.ID)
				}
				line, _, _ := strings.Cut(prompt[start:], "\n")
				for _, field := range []string{fmt.Sprintf("role=`%s`; policy=`%s`", row.Role, row.GroundingPolicy), fmt.Sprintf("claim=%q", row.Claim), fmt.Sprintf("summary=%q", row.Summary)} {
					if !strings.Contains(line, field) {
						t.Fatalf("independent field lost: %s", field)
					}
				}
				if row.Span != "" && !strings.Contains(line, "span="+row.Span) || len(row.Notes) > 0 && !strings.Contains(line, "notes="+renderAnswerDocObservationNotes(row.Notes, len(row.Notes))) {
					t.Fatal("source reuse changed independent span/notes")
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 || priorLines != nil && !reflect.DeepEqual(priorLines, lines) {
				t.Fatal("byte policy changed factual planning/source aliases")
			}
			priorLines = lines
		})
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("planning changed original evidence or causal projection")
	}
}

func hmc327LongSourceOverlapContext(t *testing.T) *types.AgentContext {
	t.Helper()
	// Real filesystem coordinates (each component below NAME_MAX), not
	// fabricated observation fields or padding added to model messages. Nine
	// independent windows reuse the same 14 observed events; all are queried.
	dir := t.TempDir()
	for _, component := range []string{"capture", "application", "scenario", "session"} {
		dir = filepath.Join(dir, component+"-"+strings.Repeat("coordinate", 16))
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "overlap.systrace")
	var source strings.Builder
	for n := 0; n < 7; n++ {
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: B|42|AppStartup\n", 10+n*10)
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: E|42\n", 14+n*10)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = dir, dir
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	var results []types.ToolResult
	for n := 0; n < 9; n++ {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1.0, "time_end": 1.1 + float64(n)/100, "limit": 40})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("actual long-coordinate query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

// Both public fixtures were measured to favor omission. Earlier attempts to
// call either a lower-cost-full positive had invalid premises; their original
// failed runs remain evidence. The optional lower-cost-full branch has NOT
// received public trigger coverage. Roomy advisory/enforce controls above
// separately verify full publication without making that coverage claim.
func TestHMC327FinalizerPublicSoftTargetKeepsSmallerOmissionPlans(t *testing.T) {
	for _, tc := range []struct {
		name       string
		longSource bool
	}{
		{name: "two_queries_omission_smaller"},
		{name: "nine_long_source_queries_omission_smaller", longSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ctx *types.AgentContext
			wantQueries := 2
			if tc.longSource {
				ctx = hmc327LongSourceOverlapContext(t)
				wantQueries = 9
			} else {
				// Preserve the original v1/v2 fixture. Its exact public plans
				// proved omission smaller; it must NOT be used as a full-wins
				// prerequisite or forced to publish a larger preview.
				ctx = traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
				ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
			}
			ledger := answerDocObservationLedger(ctx)
			originals := make(map[string]types.ObservationRecord)
			for _, r := range ledger.Records {
				if !types.IsValidTraceEventSearchInventoryRecord(r) {
					continue
				}
				v := r.EventSearchInventory
				wantRows := 14
				if !tc.longSource {
					wantRows = 7
					if len(v.Query.EventFieldFilters) > 0 {
						wantRows = 3
					}
				}
				if len(v.Rows) != wantRows || v.Coverage.MatchedTotal != wantRows || v.Coverage.Emitted != wantRows || !v.RowsComplete || v.HandoffRowsOmitted != 0 {
					t.Fatalf("fixture changed the complete original population: rows=%d coverage=%+v", len(v.Rows), v.Coverage)
				}
				if _, exists := originals[r.SourceRef.QueryScopeID]; exists || r.SourceRef.QueryScopeID == "" {
					t.Fatal("independent queries collapsed into one identity")
				}
				originals[r.SourceRef.QueryScopeID] = r
			}
			if len(originals) != wantQueries {
				t.Fatalf("original query count=%d want %d", len(originals), wantQueries)
			}
			before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
			capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured closed-plan choice")}, window: 2000000}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			base := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
				AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce, ContextPressureSoftRatio: .0001}}).(*BaseAgent)
			_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			budget := base.eval.(*answerDocumentEvaluator).promptBudget
			if budget == nil || budget.NativeLedgerPlan == nil {
				t.Fatal("fixture produced no closed-plan accounting")
			}
			plan := budget.NativeLedgerPlan
			// Prove the premise BEFORE asserting which group was published.
			if plan.WithoutInventoryBytes >= plan.WithInventoryBytes {
				t.Fatalf("smaller-omission prerequisite invalid: plans=%+v", plan)
			}
			fixed := budget.StaticBytes + budget.SchemaBytes + budget.TailBytes + len("user") + budget.protectedDynamicBytes
			if plan.WithInventoryBytes <= 0 || plan.WithoutInventoryBytes <= 0 ||
				fixed+plan.WithInventoryBytes > budget.RequestAllocationBytes || fixed+plan.WithoutInventoryBytes > budget.RequestAllocationBytes ||
				fixed+min(plan.WithInventoryBytes, plan.WithoutInventoryBytes) <= budget.OptionalTargetBytes {
				t.Fatalf("fixture must fit hard allocation but exceed soft target: budget=%+v plans=%+v", budget, plan)
			}
			if capture.calls != 1 || !errors.Is(err, capture.stop) || plan.InventoryPublished || plan.PublishedLedgerBytes != plan.WithoutInventoryBytes {
				t.Fatalf("wrong closed-plan publication: calls=%d err=%v plans=%+v", capture.calls, err, plan)
			}
			if budget.DynamicPublishedBytes != len(hmc326DynamicRequestMessage(t, capture)) || llm.EstimateRequestBytes(capture.messages, capture.tools) > budget.RequestAllocationBytes {
				t.Fatal("actual dynamic/schema/tail bytes escaped the same allocation")
			}
			prompt := hmc327RequestText(capture)
			if !strings.Contains(prompt, "Native event preview dependency group not published") || strings.Contains(prompt, "### Trace Event Search Inventories\n") ||
				strings.Contains(prompt, "inventory_source_1") || strings.Contains(prompt, "selector_row_ids=") || strings.Contains(prompt, "row_refs=[") || strings.Count(prompt, "- original_query_navigation=") != wantQueries {
				t.Fatal("smaller omission plan lost navigation or gained unpublished aliases")
			}
			seen := make(map[string]bool)
			for _, line := range strings.Split(prompt, "\n") {
				if raw, ok := strings.CutPrefix(line, "- original_query_navigation="); ok {
					var navigation struct {
						Source       types.ObservationSourceRef              `json:"source"`
						Query        types.TraceEventSearchInventoryQuery    `json:"query"`
						Coverage     types.TraceEventSearchInventoryCoverage `json:"coverage"`
						RetainedRows int                                     `json:"retained_rows"`
						PromptRows   int                                     `json:"prompt_rows"`
					}
					if json.Unmarshal([]byte(raw), &navigation) != nil {
						t.Fatal("malformed query navigation")
					}
					r, exists := originals[navigation.Source.QueryScopeID]
					if !exists || seen[navigation.Source.QueryScopeID] || !reflect.DeepEqual(navigation.Source, r.SourceRef) ||
						!reflect.DeepEqual(navigation.Query, r.EventSearchInventory.Query) || !reflect.DeepEqual(navigation.Coverage, r.EventSearchInventory.Coverage) ||
						navigation.RetainedRows != len(r.EventSearchInventory.Rows) || navigation.PromptRows != 0 {
						t.Fatal("query navigation borrowed a source/count or promoted omitted rows to a complete preview")
					}
					seen[navigation.Source.QueryScopeID] = true
				}
			}
			if len(seen) != wantQueries {
				t.Fatal("navigation lost an independent original")
			}
			afterLedger := answerDocObservationLedger(ctx)
			after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
			if string(before) != string(after) {
				t.Fatal("soft preference changed original evidence or causal projection")
			}
		})
	}
}

func TestHMC327FinalizerPublicOmittedInventoryCannotPublishSourceAlias(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	// The advisory caller deliberately captures the same complete required
	// records even when the local allocation cannot fit an optional inventory.
	// This is not the strict product no-fit path tested separately above.
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured omitted inventory")}, window: 1000}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteLegacyAdvisory}}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if capture.calls != 1 || !errors.Is(err, capture.stop) {
		t.Fatalf("advisory capture failed: %v", err)
	}
	prompt := hmc327RequestText(capture)
	if !strings.Contains(prompt, "Native event preview dependency group not published") || strings.Contains(prompt, "### Trace Event Search Inventories\n") ||
		strings.Contains(prompt, "inventory_source_1") || strings.Contains(prompt, "selector_row_ids=") || strings.Contains(prompt, "row_refs=[") {
		t.Fatal("omitted source/member dictionary was partially published")
	}
	checked := 0
	for _, row := range answerDocObservationPromptRecords(ctx, ledger.Records, answerDocObservationLedgerPromptLimit) {
		if row.Claim == types.TraceEventSearchInventoryPredicate {
			continue
		}
		prefix := fmt.Sprintf("- `%s`: origin=`%s`; producer=`%s`; source=`%s`", row.ID, row.Origin, row.Producer, row.Source)
		if !strings.Contains(prompt, prefix) {
			t.Fatalf("omitted dictionary left an alias or lost the independent source: %s", row.ID)
		}
		checked++
	}
	if checked == 0 || strings.Count(prompt, "- original_query_navigation=") != 2 {
		t.Fatal("fixture lost independent query navigation or generic records")
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("omission changed original evidence or causal projection")
	}
}

func TestHMC327BudgetEntryUsesSinglePublicDispatchReset(t *testing.T) {
	ctx, _ := hmc320BusinessFinalizer(t)
	e := &answerDocumentEvaluator{postEmitAdvisoryDelivered: true, preferPatchNext: true}
	resolved := 0
	budget := &finalizerPromptBudget{Policy: types.FinalizerRequestByteEnforce, RequestAllocationBytes: 8000000, OptionalTargetBytes: 8000000}
	first := e.BuildInitialInstructionWithBudget(ctx, nil, func() *finalizerPromptBudget {
		resolved++
		if e.postEmitAdvisoryDelivered || e.preferPatchNext || e.pendingPromptBudget != nil {
			t.Fatal("schema budget resolved before the one public dispatch reset")
		}
		return budget
	})
	if resolved != 1 || e.promptBudget != budget || budget.DynamicPublishedBytes != len(first) || strings.Count(first, "> Note — structural emit-time constraints") != 1 || strings.Contains(first, "## Finalizer content-byte allocation receipt") {
		t.Fatal("budget entry rebuilt or lost its one current allocation")
	}
	e.postEmitAdvisoryDelivered, e.preferPatchNext = true, true
	second := e.BuildInitialInstruction(ctx, nil)
	if resolved != 1 || e.postEmitAdvisoryDelivered || e.preferPatchNext || e.promptBudget != nil || e.pendingPromptBudget != nil || strings.Contains(second, "## Finalizer content-byte allocation receipt") {
		t.Fatal("ordinary public entry inherited the previous budget/latch")
	}
}
