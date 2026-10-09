package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func primaryWindowPriorityContext(t *testing.T) *types.BusContext {
	t.Helper()
	ctx := primaryWindowContext(t)
	// Arm the existing C-lite analyzer-hint surface, exactly as its public
	// real-capture tests do; the new replay priority does not inspect prose.
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords = []string{"VSync"}
	// Reuse an unchanged, independently exercised real generator record. Its
	// original timestamp is outside the requested rendering window: only the
	// existing windowless C-lite pass should publish this generator census.
	real, err := os.ReadFile(vsyncSAF2RealTraceRel)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(real), "\n") {
		if strings.Contains(line, "VSyncGenerator-1682") && strings.Contains(line, "H:GenerateVsyncCount:1, period:16552213") {
			ctx.AttachedHitrace += line + "\n"
			return ctx
		}
	}
	t.Fatal("existing real generator witness missing")
	return nil
}

func primaryWindowPriorityQuery(t *testing.T, ctx *types.BusContext, view string) {
	t.Helper()
	args := map[string]any{"source": "attached_trace", "view": view, "time_start": 1, "time_end": 1.051}
	if view == "event_search" {
		args["pattern"] = "RNViewBase"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("original %s failed: %v %s", view, err, result.Summary)
	}
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !current {
		t.Fatalf("original %s did not mint a current native replay ticket", view)
	}
	ctx.ToolResults = append(ctx.ToolResults, result)
}

func primaryWindowPriorityPreconditions(t *testing.T, ctx *types.BusContext, count int) []byte {
	t.Helper()
	path, label, _, reject := resolveReadyTraceQuerySource(ctx, traceQueryParams{})
	if reject != nil {
		t.Fatalf("source preparation failed: %+v", reject)
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	if calls := primaryWindowCalls(ctx, path, label, input); len(calls) != count {
		t.Fatalf("expected %d current same-source native queries, got %d", count, len(calls))
	}
	if _, _, targetOK := traceSupplementDeriveTarget(ctx); targetOK {
		t.Fatal("test accidentally enabled the existing targeted causal supplement")
	}
	if !traceSupplementVsyncFamilyHit(ctx) || traceSupplementObservationsCarryVsyncCensus(types.CompileObservationLedger(input).Records) {
		t.Fatal("test did not require the existing C-lite reserved slot")
	}
	before, err := json.Marshal([]any{ctx.ToolResults, ctx.Mutable.TraceQueryCallWindows()})
	if err != nil {
		t.Fatal(err)
	}
	return before
}

func assertPrimaryWindowPriorityCensusAndOriginals(t *testing.T, ctx *types.BusContext, before []byte) {
	t.Helper()
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || !meta.CensusLite || meta.CensusLitePattern != "vsync" || len(meta.MemberWindows) != 1 {
		t.Fatalf("existing C-lite pass or member accounting lost: %+v", meta)
	}
	if len(ctx.Mutable.SystemTraceSupplementResults()) != 2 {
		t.Fatalf("expected exactly one windowed query plus C-lite, got %d results", len(ctx.Mutable.SystemTraceSupplementResults()))
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	census := false
	for _, record := range ledger.Records {
		if record.Predicate == "vsync_generator_census" {
			census = true
			if !record.SystemSupplement || record.Subject != "VSyncGenerator-1682" || !strings.Contains(record.Value, "period:16552213ns") {
				t.Fatalf("C-lite did not publish the real generator on its dedicated lane: %+v", record)
			}
		}
	}
	if !census {
		t.Fatal("no native generator census reached the system ledger")
	}
	after, err := json.Marshal([]any{ctx.ToolResults, ctx.Mutable.TraceQueryCallWindows()})
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("supplement changed original model results or registered its own query windows")
	}
}

func TestPrimaryWindowPrioritySemanticBeforeRawPublic(t *testing.T) {
	for _, order := range []struct {
		name  string
		views []string
	}{
		{"raw_first", []string{"event_search", "rendering_candidates"}},
		{"semantic_first", []string{"rendering_candidates", "event_search"}},
	} {
		t.Run(order.name, func(t *testing.T) {
			ctx := primaryWindowPriorityContext(t)
			for _, view := range order.views {
				primaryWindowPriorityQuery(t, ctx, view)
			}
			before := primaryWindowPriorityPreconditions(t, ctx, 2)
			out := RunTraceQuerySystemSupplement(ctx)
			assertPrimaryWindowPriorityCensusAndOriginals(t, ctx, before)
			meta := ctx.Mutable.SystemTraceSupplementMeta()
			member := meta.MemberWindows[0]
			if !reflect.DeepEqual(out.Executed, []string{"rendering_candidates", "event_search"}) ||
				!reflect.DeepEqual(member.Views, []string{"rendering_candidates"}) ||
				!reflect.DeepEqual(member.SkippedViews, []string{"event_search"}) ||
				member.SkipReason != types.TraceSupplementReasonQueryBudgetExceeded {
				t.Errorf("raw discovery displaced an already successful semantic projection: executed=%v member=%+v", out.Executed, member)
			}
			primary := false
			for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
				for _, record := range result.Observations {
					p, ok := DecodeTraceRenderingCandidates(record)
					if !ok {
						continue
					}
					primary = true
					if p.Window.StartTs != 1 || p.Window.EndTs != 1.05 || p.Window.EndInclusive || len(p.Candidates) == 0 {
						t.Fatalf("incorrect requested-window projection: %+v", p)
					}
					for _, candidate := range p.Candidates {
						if candidate.OwnerID == 900 {
							t.Fatal("right-endpoint PID 900 entered the primary projection")
						}
					}
				}
			}
			if !primary {
				t.Error("requested-window semantic projection was never published")
			}
		})
	}
}

func TestPrimaryWindowPriorityRawOnlyStillReplaysPublic(t *testing.T) {
	ctx := primaryWindowPriorityContext(t)
	primaryWindowPriorityQuery(t, ctx, "event_search")
	before := primaryWindowPriorityPreconditions(t, ctx, 1)
	out := RunTraceQuerySystemSupplement(ctx)
	assertPrimaryWindowPriorityCensusAndOriginals(t, ctx, before)
	member := ctx.Mutable.SystemTraceSupplementMeta().MemberWindows[0]
	if !reflect.DeepEqual(out.Executed, []string{"event_search", "event_search"}) ||
		!reflect.DeepEqual(member.Views, []string{"event_search"}) || len(member.SkippedViews) != 0 || member.SkipReason != "" {
		t.Fatalf("soft priority became a raw-discovery ban: executed=%v member=%+v", out.Executed, member)
	}
	result := ctx.Mutable.SystemTraceSupplementResults()[0]
	_, raw, current := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay)
	var p traceQueryParams
	if !current || json.Unmarshal(raw, &p) != nil || p.View != "event_search" || p.Pattern != "RNViewBase" ||
		p.TimeStart.Seconds() != 1 || p.TimeEnd.Seconds() != 1.05 {
		t.Fatalf("raw-only replay changed its original filter or requested bounds: current=%v params=%s", current, raw)
	}
}
