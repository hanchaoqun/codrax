package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc267PresentationQueries(t *testing.T, selectors []map[string]any) *types.AgentContext {
	t.Helper()
	ctx := samplingSelectionAnalysisContext(t)
	var results []types.ToolResult
	for _, selector := range selectors {
		args := map[string]any{"source": "attached_trace", "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2}
		for key, value := range selector {
			args[key] = value
		}
		raw, _ := json.Marshal(args)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("native query %s: %v %+v", raw, err, result)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

func TestHMC267PublicPresentationReusesNativePopulation(t *testing.T) {
	one := hmc267PresentationQueries(t, []map[string]any{{}})
	baseline := dependencyObservationMessages(t, one)
	ctx := hmc267PresentationQueries(t, []map[string]any{{}, {"view": "perf_stats"}, {"thread": "document-worker"}})
	before, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
	prompt := dependencyObservationMessages(t, ctx)
	if count := strings.Count(prompt, "Sampling prefix tree:"); count != 1 {
		t.Errorf("same native population was fully displayed %d times", count)
	}
	for _, marker := range []string{`"recorded_symbol":`, `view="summary" label="采样热点`, `view="distribution" label="采样热点`} {
		want, got := strings.Count(baseline, marker), strings.Count(prompt, marker)
		if want == 0 || got == 0 || got > want {
			t.Errorf("same native fields duplicated across queries: marker=%q single=%d repeated=%d", marker, want, got)
		}
	}
	for _, want := range []string{"total_weight=35", "total_weight=7", "total_weight=100", "unavailable_weight=5", "sys;root", "/app/A.so", "/app/B.so", "CPU 0"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("native field was lost: %q", want)
		}
	}
	for _, marker := range []string{"native_tree_reuse", "native_table_reuse", "sample_weight=15 denominator=35"} {
		if !strings.Contains(prompt, marker) {
			t.Errorf("missing receipt or independent legacy summary: %q", marker)
		}
	}
	// Separate original summaries can consume the bounded flat preview even
	// when their structured fields share a population. Omission is honest,
	// not permission to enlarge the pool or sign an unpublished alias.
	p := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocObservationLedgerWithPublication(ctx, p)
	ledger := answerDocObservationLedger(ctx)
	withheld := 0
	mirrors := p.samplingMirrorReferences(ledger)
	for _, r := range samplingMemberPresentationOrder(ledger, p, nil) {
		if !p.records[traceEventPublicationRecordKey(r)] {
			withheld++
			if mirrors[r.ID] != "" {
				t.Fatal("unpublished member signed as mirror")
			}
		}
	}
	if withheld == 0 || !strings.Contains(text, "Recorded hotspot preview omitted=") || p.completeFor(ctx) {
		t.Fatal("bounded multi-query omission was not disclosed or falsely complete")
	}
	after, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
	if string(before) != string(after) || len(ctx.Mutable.TurnAArtifacts().ToolResults) != 3 {
		t.Fatal("presentation changed query receipts, original ledger or causal projection")
	}
	t.Logf("adapter bytes single=%d repeated=%d", len(baseline), len(prompt))
}

func TestHMC267PublishedMemberAliasRetainsIndependentReceipts(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{}, {"thread": "document-worker"}})
	ledger := answerDocObservationLedger(ctx)
	p := &answerDocNativePresentationReceipt{}
	renderAnswerDocPerfSamplingTrees(ctx, ledger, p)
	selected := ledger
	selected.Records = nil
	for _, r := range ledger.Records {
		if r.Predicate == "perf_sample_top_symbol" && r.Value != "" && len(r.RichNotes) > 0 {
			selected.Records = append(selected.Records, r)
		}
	}
	text := renderAnswerDocPerfMemberFacts(selected, p)
	if len(selected.Records) != 8 || strings.Count(text, "native_member_reuse ") != 4 || strings.Count(text, `"recorded_symbol":`) != 4 {
		t.Fatalf("qualified alias was not actually published: records=%d aliases=%d", len(selected.Records), strings.Count(text, "native_member_reuse "))
	}
	for _, r := range selected.Records {
		summary, _ := json.Marshal(r.Summary)
		if !p.records[traceEventPublicationRecordKey(r)] || !strings.Contains(text, string(summary)) {
			t.Fatal("published alias lost its own whole summary or receipt")
		}
	}
}

func TestHMC267SharedNativeBudgetNeverCertifiesOmittedFields(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{}, {"thread": "document-worker"}})
	for _, limit := range []int{1, 8 << 10} {
		t.Run(string(rune('A'+limit/1024)), func(t *testing.T) {
			p := &answerDocNativePresentationReceipt{presentationLimit: limit}
			text := renderAnswerDocObservationLedgerWithPublication(ctx, p)
			if p.presentationBytes > limit {
				t.Fatal("detail budget exceeded")
			}
			if !strings.Contains(text, "preview_not_shown=shared_native_detail_budget") {
				t.Fatal("missing whole-table omission receipt")
			}
			if p.completeFor(ctx) || p.answerFactsCompleteFor(ctx) {
				t.Fatal("omitted detail signed complete")
			}
			if limit == 1 {
				for _, complete := range p.records {
					if complete {
						t.Fatal("unpublished row received a receipt")
					}
				}
				if strings.Contains(text, "native_tree_reuse") || strings.Contains(text, "native_table_reuse") {
					t.Fatal("failed representative reused")
				}
			}
		})
	}
}

func TestHMC267NavigationBudgetKeepsWholeRecords(t *testing.T) {
	p := &answerDocNativePresentationReceipt{}
	var b strings.Builder
	oversized := strings.Repeat("exact-path-", 2000)
	if p.writePresentationNavigation(&b, oversized) || b.Len() != 0 || p.navigationOmitted != 1 {
		t.Fatal("oversized navigation was truncated or uncounted")
	}
	if !p.writePresentationNavigation(&b, "whole-small-reference\n") || b.String() != "whole-small-reference\n" {
		t.Fatal("large record starved later whole reference")
	}
}

func TestHMC267UnpublishedReuseNavigationCannotCertifyAlias(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{}, {"thread": "document-worker"}})
	p := &answerDocNativePresentationReceipt{navigationBytes: 16 << 10}
	text := renderAnswerDocObservationLedgerWithPublication(ctx, p)
	if strings.Contains(text, "native_tree_reuse") || strings.Contains(text, "native_table_reuse") || p.navigationOmitted == 0 {
		t.Fatal("full navigation budget did not withhold aliases")
	}
	shown, withheld := 0, 0
	for _, r := range answerDocObservationLedger(ctx).Records {
		if _, valid := tool.DecodeTracePerfSamplingTree(r); !valid {
			continue
		}
		if p.records[traceEventPublicationRecordKey(r)] {
			shown++
		} else {
			withheld++
		}
	}
	if shown != 1 || withheld != 1 {
		t.Fatalf("tree receipt counts shown=%d withheld=%d", shown, withheld)
	}
	if !strings.Contains(text, "Native presentation reference records omitted=") {
		t.Fatal("omission count lost")
	}
}

func TestHMC267StructuredMemberDoesNotEraseIndependentSummary(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{}})
	artifacts := ctx.Mutable.TurnAArtifacts()
	const marker = "INDEPENDENT_NATIVE_MEMBER_SUMMARY_MUST_REMAIN"
	for i := range artifacts.ToolResults[0].Observations {
		r := &artifacts.ToolResults[0].Observations[i]
		if r.Predicate == "perf_sample_top_symbol" && r.Value != "" && len(r.RichNotes) > 0 {
			r.Summary = marker
			break
		}
	}
	ctx.Mutable.SetTurnAArtifacts(*artifacts)
	p := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocObservationLedgerWithPublication(ctx, p)
	if !strings.Contains(text, marker) {
		t.Fatal("structured projection erased unshown independent summary")
	}
}

func TestHMC267UnpublishedNotesCannotReserveDirectoryIDs(t *testing.T) {
	directory := samplingNoteDirectory{}
	_, catalog, pending := directory.propose([]string{strings.Repeat("whole-unpublished-note", 2000)})
	p := &answerDocNativePresentationReceipt{presentationLimit: 1}
	if p.takePresentationBytes(len(catalog)) {
		t.Fatal("oversized directory fit")
	}
	if len(directory) != 0 || len(pending) != 1 {
		t.Fatal("proposal mutated directory")
	}
	refs, catalog, pending := directory.propose([]string{"native complete note", "native complete note"})
	if len(refs) != 2 || refs[0] != "sampling-note-1" || refs[1] != refs[0] || strings.Count(catalog, "native_note_ref=") != 1 {
		t.Fatal("unpublished proposal reserved IDs or duplicate note copied")
	}
	directory.commit(pending)
	refs, catalog, pending = directory.propose([]string{"native complete note"})
	if refs[0] != "sampling-note-1" || catalog != "" || len(pending) != 0 {
		t.Fatal("published note did not resolve")
	}
}
