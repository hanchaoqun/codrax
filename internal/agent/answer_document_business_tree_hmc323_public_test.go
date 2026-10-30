package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The account is deliberately computed from all 18 direct children before
// either the 16-segment producer preview or the finalizer's row budgets.
// Integer microseconds establish 111487 - 107549 = 3938 independently of
// the renderer; no displayed child preview is used as an arithmetic oracle.
func hmc323BusinessBudgetContext(t *testing.T) (*types.AgentContext, string) {
	t.Helper()
	ctx, path := hmc320BusinessFinalizer(t)
	var raw strings.Builder
	raw.WriteString("# tracer: nop\n")
	line := func(us int64, marker string) {
		fmt.Fprintf(&raw, "business-700 (600) [003] .... %d.%06d: tracing_mark_write: %s\n", us/1000000, us%1000000, marker)
	}
	for _, start := range []int64{5000000, 5200000} {
		line(start, "B|600|RepeatedBusiness")
		cursor := start + 1000
		for child := 0; child < 18; child++ {
			line(cursor, fmt.Sprintf("B|600|Child%02d", child))
			duration := int64(5900)
			if child == 17 {
				duration = 7249
			}
			cursor += duration
			line(cursor, "E|600")
			cursor += 100
		}
		line(start+111487, "E|600")
	}
	if err := os.WriteFile(path, []byte(raw.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.4
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.0..5.4"}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return ctx, path
}

func hmc323FindBusinessAccount(t *testing.T, ledger types.ObservationLedger, queryStart, physicalStart float64) (types.ObservationRecord, tool.TraceBusinessTreeFact) {
	t.Helper()
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Window.StartTs == queryStart && f.Node.ActualStartTs == physicalStart {
			return r, f
		}
	}
	t.Fatalf("no exact source account query_start=%g physical_start=%g", queryStart, physicalStart)
	return types.ObservationRecord{}, tool.TraceBusinessTreeFact{}
}

func TestHMC323BusinessAccountPublicFairBudgetKeepsCompleteAndClipped(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_arrival_%t", reverse), func(t *testing.T) {
			ctx, path := hmc323BusinessBudgetContext(t)
			windows := [][2]float64{{5.201, 5.2069}, {5, 5.112}, {5.2, 5.312}}
			if reverse {
				windows[0], windows[2] = windows[2], windows[0]
			}
			for _, w := range windows {
				hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "time_start": w[0], "time_end": w[1]})
			}
			ledger := answerDocObservationLedger(ctx)
			full, ff := hmc323FindBusinessAccount(t, ledger, 5.2, 5.2)
			clipped, cf := hmc323FindBusinessAccount(t, ledger, 5.201, 5.2)
			if ff.Node.ID != cf.Node.ID || full.SourceRef.QueryScopeID == clipped.SourceRef.QueryScopeID ||
				ff.Node.Self == nil || cf.Node.Self == nil || math.Abs(ff.Node.Self.DurationMs-3.938) > 1e-8 || cf.Node.Self.DurationMs != 0 ||
				ff.Node.Inclusive == nil || math.Abs(ff.Node.Inclusive.DurationMs-111.487) > 1e-8 || ff.Node.Self.OmittedSegments == 0 {
				t.Fatalf("independent public source prerequisites: full=%+v clipped=%+v", ff, cf)
			}
			publication := &answerDocNativePresentationReceipt{}
			choices := renderAnswerDocRuntimeMeasurementChoices(ctx, publication)
			if !strings.Contains(choices, "preview rows=128/128") {
				t.Fatalf("public fixture did not exercise the existing shared row budget:\n%s", choices)
			}
			t.Logf("native annotation count=%d", len(publication.businessAnnotations))
			partial := false
			for _, annotation := range publication.businessAnnotations {
				if annotation.Name == ff.Node.Name {
					p, ok := types.DecodeRuntimeMeasurementPublication(annotation.Record)
					t.Logf("summary candidate query=%q expected=%q matched=%t source=%q expected_source=%q interval=%g..%g lines=%d..%d valid=%t tables=%d", annotation.Record.SourceRef.QueryScopeID, full.SourceRef.QueryScopeID, businessAccountMatchesPublishedInterval(ff, annotation), annotation.Record.SourceRef.Path, ff.Node.SourcePath, annotation.StartTs, annotation.EndTs, annotation.StartLine, annotation.EndLine, ok, len(p.Tables))
					if ok && len(p.Tables) == 1 {
						t.Logf("summary candidate rows=%d presentation=%+v", len(p.Tables[0].Rows), publication.measurementViewPresentation(p.Tables[0]))
					}
				}
				if annotation.Record.SourceRef.QueryScopeID != full.SourceRef.QueryScopeID || !businessAccountMatchesPublishedInterval(ff, annotation) {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(annotation.Record)
				if ok && len(p.Tables) == 1 {
					partial = !publication.measurementViewPresentation(p.Tables[0]).rowsComplete
				}
			}
			// The late full query must reproduce the partial-summary case.
			// Reversing arrival may legitimately give that summary all rows;
			// the complete and clipped account obligations remain identical.
			if !reverse && !partial {
				t.Fatal("full-instance summary must remain a bounded preview, not a borrowed full-table receipt")
			}
			before, _ := json.Marshal(ledger)
			causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			requestBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			t.Logf("complete public finalizer handoff:\n%s", prompt)
			for _, r := range []types.ObservationRecord{full, clipped} {
				f, _ := tool.DecodeTraceBusinessTreeFact(r)
				want := tool.TraceBusinessTreeNodeText(f.Node)
				t.Logf("account query=%q physical=%q window=%+v full=%s", r.SourceRef.QueryScopeID, f.Node.ID, f.Window, want)
				if !strings.Contains(prompt, want) || !strings.Contains(prompt, "observation_id="+quotedTreeValue(r.ID)) {
					t.Errorf("complete atomic account was starved by prefix/shared table budget: query=%s self=%g", r.SourceRef.QueryScopeID, f.Node.Self.DurationMs)
				}
			}
			// Field supplementation still may not turn the partial summary
			// into a complete-table permission, even after principal display.
			principal := hmc320PrincipalLedger(ctx, ledger)
			if extra := renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication); strings.Contains(extra, "对应同源账户 observation_id="+quotedTreeValue(full.ID)) {
				t.Fatal("partial table acquired complete native field-supplement authority")
			}
			afterLedger := answerDocObservationLedger(ctx)
			after, _ := json.Marshal(afterLedger)
			causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(afterLedger))
			requestAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			if string(before) != string(after) || string(causalBefore) != string(causalAfter) || string(requestBefore) != string(requestAfter) {
				t.Fatal("presentation changed original observations, root-cause identity, or requested scope")
			}
		})
	}
}

func TestHMC323BusinessSupplementUsesActualPublishedRecord(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "time_start": 5.0, "time_end": 5.4})
	ledger := answerDocObservationLedger(ctx)
	principal := hmc320PrincipalLedger(ctx, ledger)
	text := renderAnswerDocBusinessTreeFacts(ctx, principal)
	publication := &answerDocNativePresentationReceipt{}
	renderAnswerDocRuntimeMeasurementChoices(ctx, publication)
	var omitted types.ObservationRecord
	var fact tool.TraceBusinessTreeFact
	var annotation types.NativeBusinessDiagramAnnotation
	var table types.RuntimeMeasurementTable
	for _, r := range principal.Records {
		f, ok := tool.DecodeTraceBusinessTreeFact(r)
		if !ok || strings.Contains(text, "observation_id="+quotedTreeValue(r.ID)) {
			continue
		}
		for _, a := range publication.businessAnnotations {
			p, valid := types.DecodeRuntimeMeasurementPublication(a.Record)
			if valid && len(p.Tables) == 1 && a.Record.SourceRef.QueryScopeID == r.SourceRef.QueryScopeID && businessAccountMatchesPublishedInterval(f, a) {
				omitted, fact, annotation, table = r, f, a, p.Tables[0]
				break
			}
		}
		if omitted.ID != "" {
			break
		}
	}
	if omitted.ID == "" {
		t.Fatal("no native candidate omitted by the 16-account principal budget")
	}
	// This receipt represents actual full-table publication, using the exact
	// freshly compiled producer table, not an ID-only or fabricated summary.
	publication = &answerDocNativePresentationReceipt{businessAnnotations: []types.NativeBusinessDiagramAnnotation{annotation}}
	publication.observeMeasurementView(table, true)
	extra := renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication)
	if !strings.Contains(extra, tool.TraceBusinessTreeNodeText(fact.Node)) || !strings.Contains(extra, "对应同源账户 observation_id="+quotedTreeValue(omitted.ID)) {
		t.Fatalf("candidate presence suppressed an account that was never actually displayed: %s\n%s", omitted.ID, extra)
	}
	publication.observeMeasurementViewDetails(table, false, true)
	if extra := renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication); extra != "" {
		t.Fatalf("partial publication gained full account permission: %s", extra)
	}
}

func TestHMC323BusinessBudgetIsDeterministicAndRetainsOverflow(t *testing.T) {
	ctx := businessTreeFinalizerContext(t)
	ledger := answerDocObservationLedger(ctx)
	var original types.ObservationRecord
	for _, r := range ledger.Records {
		if _, ok := tool.DecodeTraceBusinessTreeFact(r); ok {
			original = r
			break
		}
	}
	if original.ID == "" {
		t.Fatal("missing native account")
	}
	ledger.Records = nil
	for i := 0; i < 20; i++ {
		r := original
		r.ID += fmt.Sprintf("-query-%02d", i)
		r.SourceRef.QueryScopeID += fmt.Sprintf("-query-%02d", i)
		ledger.Records = append(ledger.Records, r)
	}
	forward := renderAnswerDocBusinessTreeFacts(ctx, ledger)
	for i, j := 0, len(ledger.Records)-1; i < j; i, j = i+1, j-1 {
		ledger.Records[i], ledger.Records[j] = ledger.Records[j], ledger.Records[i]
	}
	reverse := renderAnswerDocBusinessTreeFacts(ctx, ledger)
	if forward != reverse {
		t.Error("fair account selection depends on arrival/map iteration order")
	}
	if strings.Count(forward, "observation_id=") != 16 || !strings.Contains(forward, "另省略 4 条已发布记录") {
		t.Fatal("existing 16+4 bounded publication contract changed")
	}
}

func TestHMC323BusinessSameRecordIDDoesNotMergeSources(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "same-bytes.systrace")
	if err := os.WriteFile(other, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, other} {
		hmc320BusinessQuery(t, ctx, p, map[string]any{"pid": 700, "time_start": 5.2, "time_end": 5.312})
	}
	ledger := answerDocObservationLedger(ctx)
	var rows []types.ObservationRecord
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Node.ActualStartTs == 5.2 {
			r.ID = "same-display-observation-id"
			rows = append(rows, r)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("public source prerequisites: %d", len(rows))
	}
	ledger.Records = rows
	text := renderAnswerDocBusinessTreeFacts(ctx, ledger)
	if strings.Count(text, "observation_id=\"same-display-observation-id\"") != 2 ||
		!strings.Contains(text, "physical_source="+quotedTreeValue(rows[0].SourceRef.Path)) || !strings.Contains(text, "physical_source="+quotedTreeValue(rows[1].SourceRef.Path)) {
		t.Fatalf("bare display ID merged independent source accounts: %s", text)
	}
}
