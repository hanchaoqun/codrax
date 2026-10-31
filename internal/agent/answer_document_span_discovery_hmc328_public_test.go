package agent

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A successful paired-span lookup must reach the same native publication
// channel as window statistics. No investigation prose supplies these facts.
func TestHMC328SpanDiscoveryPublicNativeHandoff(t *testing.T) {
	for _, narrow := range []bool{false, true} {
		t.Run(map[bool]string{false: "whole_pair", true: "narrow_lookup_preserves_pair"}[narrow], func(t *testing.T) {
			ctx, path := hmc320BusinessFinalizer(t)
			start, end, duration := 5.0, 5.012, 5.0
			coverageStart, coverageEnd := start, end
			if narrow {
				// span_window locates complete pairs intersecting this range;
				// it is not a clipped window_stats measurement. Keep both.
				start, end = 5.003, 5.006
				coverageStart, coverageEnd = 5.002, 5.007
			}
			result := hmc320BusinessQuery(t, ctx, path, map[string]any{
				"view": "span_window", "span_name": "LoadIndex", "pid": 700,
				"time_start": start, "time_end": end,
			})
			var rows []types.ObservationRecord
			for _, r := range result.Observations {
				if r.Predicate == types.TraceBusinessSpanPredicate {
					rows = append(rows, r)
				}
			}
			if len(rows) != 1 {
				t.Fatalf("paired lookup result lost its native measured fact: got %d span records", len(rows))
			}
			r := rows[0]
			if r.Object != "LoadIndex" || r.Subject != "business-700" || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan ||
				r.Role != types.AnswerAggregateRoleSupportingCoverage || r.Unit != "ms" || r.SourceRef.QueryTargetPID != 700 ||
				!r.SourceRef.QueryWindowKnown || r.SourceRef.QueryWindowStartTs != coverageStart || r.SourceRef.QueryWindowEndTs != coverageEnd ||
				r.Span.StartTs != 5.002 || r.Span.EndTs != 5.007 ||
				math.Abs((r.Span.EndTs-r.Span.StartTs)*1000-duration) > 1e-8 {
				t.Fatalf("lookup changed identity, query ruler or measured span: %+v", r)
			}
			p, ok := types.DecodeRuntimeMeasurementPublication(r)
			if !ok || len(p.Tables) != 1 || p.Tables[0].View != types.RuntimeMeasurementSummary {
				t.Fatalf("paired lookup has no reusable native summary: %+v", r)
			}
			data, _ := json.Marshal(p.Tables[0])
			if narrow && (!strings.Contains(string(data), "原始查找条件：time_start=5.003000、time_end=5.006000") || !strings.Contains(string(data), "定位结果覆盖 [5.002000, 5.007000)")) {
				t.Fatalf("lookup filter and complete pair coverage were conflated: %s", data)
			}
			for _, absent := range []string{"运行（ms）", "睡眠（ms）", "自身耗时"} {
				if strings.Contains(string(data), absent) {
					t.Fatalf("pair-only lookup invented an unqueried account %q: %s", absent, data)
				}
			}
			ledgerBefore, _ := json.Marshal(answerDocObservationLedger(ctx))
			focusBefore, refBefore := ctx.Mutable.AcceptedTraceBusinessFocus()
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "LoadIndex") || !strings.Contains(prompt, r.ID) || !strings.Contains(prompt, r.SourceRef.QueryScopeID) {
				t.Fatal("native lookup fact did not reach the public finalizer with its query identity")
			}
			ledgerAfter, _ := json.Marshal(answerDocObservationLedger(ctx))
			focusAfter, refAfter := ctx.Mutable.AcceptedTraceBusinessFocus()
			if string(ledgerBefore) != string(ledgerAfter) || focusBefore != focusAfter || refBefore != refAfter {
				t.Fatal("display mutated native evidence or accepted a causal/business focus")
			}
			if len(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()) == 0 {
				t.Fatal("native summary cannot be reused by answer surfaces")
			}
		})
	}
}

func TestHMC328SpanDiscoveryTouchingLookupEndpoints(t *testing.T) {
	for _, bounds := range [][2]float64{{5, 5.002}, {5.007, 5.008}} {
		ctx, path := hmc320BusinessFinalizer(t)
		result := hmc320BusinessQuery(t, ctx, path, map[string]any{
			"view": "span_window", "span_name": "LoadIndex", "pid": 700,
			"time_start": bounds[0], "time_end": bounds[1],
		})
		found := 0
		for _, r := range result.Observations {
			if r.Predicate != types.TraceBusinessSpanPredicate {
				continue
			}
			found++
			p, ok := types.DecodeRuntimeMeasurementPublication(r)
			if !ok || len(p.Tables) != 1 || r.Span.StartTs != 5.002 || r.Span.EndTs != 5.007 || r.Value != "5.000" {
				t.Fatalf("native endpoint inclusion or complete pair changed: %+v", r)
			}
			data, _ := json.Marshal(p.Tables[0])
			if !strings.Contains(string(data), "原始查找条件：time_start=") || strings.Contains(string(data), "原始查找范围 [") {
				t.Fatalf("lookup endpoint inclusion was replaced by a half-open measurement ruler: %s", data)
			}
		}
		if found != 1 {
			t.Fatalf("touching endpoint lookup lost its native pair: %v, %d", bounds, found)
		}
	}
}

func TestHMC328SpanDiscoveryIndependentQueries(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	for _, name := range []string{"LoadIndex", "DecodeRecord"} {
		hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "span_window", "span_name": name, "pid": 700})
	}
	ledger := answerDocObservationLedger(ctx)
	queries := map[string]bool{}
	for _, r := range ledger.Records {
		if r.Predicate == types.TraceBusinessSpanPredicate {
			queries[r.SourceRef.QueryScopeID] = true
			if r.SourceRef.PayloadRef == "" || len(r.SupportRefs) == 0 {
				t.Fatal("lookup fact has no independent original")
			}
		}
	}
	if len(queries) != 2 {
		t.Fatalf("independent lookups collapsed or disappeared: %d", len(queries))
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, name := range []string{"LoadIndex", "DecodeRecord"} {
		if !strings.Contains(prompt, name) {
			t.Fatalf("independently queried business missing from finalization: %s", name)
		}
	}
	// A lookup token remains navigation only; no accepted completion is minted.
	if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone {
		t.Fatalf("lookup acquired accepted focus: %s", status)
	}
}

func TestHMC328SpanDiscoveryAsyncKeepsMeasuredMeaning(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	result := hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "span_window", "span_name": "AsyncPrefetch", "pid": 700})
	var count int
	for _, r := range result.Observations {
		if r.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		count++
		p, valid := types.DecodeRuntimeMeasurementPublication(r)
		if !valid || len(p.Tables) != 1 || r.Value != "7.500" || r.Object != "AsyncPrefetch" {
			t.Fatalf("asynchronous pairing lost original measurement: %+v", r)
		}
		data, _ := json.Marshal(p.Tables[0])
		if !strings.Contains(string(data), "异步配对区间") || strings.Contains(string(data), "运行（ms）") {
			t.Fatalf("asynchronous elapsed interval became CPU execution: %s", data)
		}
	}
	if count != 1 {
		t.Fatalf("asynchronous original missing or duplicated: %d", count)
	}
}

func TestHMC328BusinessAccountPublicBoundOwnerKeepsLateChild(t *testing.T) {
	ctx, path := hmc325BusinessFocusContext(t, "parent")
	hmc325Query(t, ctx, path, map[string]any{"pid": 700})
	ledger := answerDocObservationLedger(ctx)
	var childID string
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Node.Thread.PID == 700 && f.Node.ActualStartTs == 5.005 {
			childID = r.ID
		}
	}
	if childID == "" {
		t.Fatal("producer did not supply the late measured child")
	}
	before, _ := json.Marshal(ledger)
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	plan := planAnswerDocBusinessTreeFacts(ctx, ledger)
	if len(plan.accounts) != 16 {
		t.Fatalf("changed shared account budget: %d", len(plan.accounts))
	}
	if !strings.Contains((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil), "observation_id="+quotedTreeValue(childID)) {
		t.Fatal("same-owner measured child was displaced by earlier background roots at finalization")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
	if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
		t.Fatal("measured display ranking mutated raw or causal facts")
	}
}
