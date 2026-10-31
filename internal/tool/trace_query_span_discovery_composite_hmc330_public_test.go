package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Identical bytes, names, owners, times and local line numbers do not make
// two physical sources interchangeable. Each bundle deliberately admits only
// its declared primary systrace; the other member remains isolated.
func hmc330SpanSources(t *testing.T) (string, []string, []string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	for i := 0; i < 3; i++ {
		start := 10 + float64(i)*0.02
		fmt.Fprintf(&body, "worker-200 (100) [001] .... %.6f: tracing_mark_write: B|100|SharedWork\n", start)
		fmt.Fprintf(&body, "worker-200 (100) [001] .... %.6f: tracing_mark_write: E|100\n", start+float64(i+5)/1000)
	}
	physical := []string{filepath.Join(dir, "a.systrace"), filepath.Join(dir, "b.systrace")}
	bundles := []string{filepath.Join(dir, "capture-a.tracebundle.json"), filepath.Join(dir, "capture-b.tracebundle.json")}
	for _, path := range physical {
		if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i, path := range bundles {
		manifest := fmt.Sprintf(`{"version":"test","systrace":%q,"artifacts":[{"type":"systrace","path":%q},{"type":"systrace","path":%q}]}`,
			filepath.Base(physical[i]), filepath.Base(physical[i]), filepath.Base(physical[1-i]))
		writeToolTraceBundleV2Fixture(t, path, []byte(manifest))
	}
	return dir, physical, bundles
}

func hmc330SpanQuery(t *testing.T, ctx *types.BusContext, path, view string) (types.ToolResult, tracequery.Result) {
	t.Helper()
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "span_name": "SharedWork", "pid": 200,
		"time_start": 9.999, "time_end": 10.06, "limit": 16})
	if !r.Success {
		t.Fatalf("public query failed before publication: %s", r.Summary)
	}
	return r, businessSpanSchedulerPublicPayload(t, r)
}

func hmc330BusinessRows(r types.ToolResult) []types.ObservationRecord {
	var rows []types.ObservationRecord
	for _, record := range r.Observations {
		if record.Predicate == types.TraceBusinessSpanPredicate {
			rows = append(rows, record)
		}
	}
	return rows
}

func TestHMC330SpanDiscoveryCompositeNativePublication(t *testing.T) {
	dir, physical, bundles := hmc330SpanSources(t)
	for _, tc := range []struct {
		name, query, physical string
		composite             bool
	}{
		{"single_source", physical[0], physical[0], false},
		{"bundle_a", bundles[0], physical[0], true},
		{"bundle_b", bundles[1], physical[1], true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState(tc.name)}
			for _, view := range []string{"span_window", "window_stats"} {
				t.Run(view, func(t *testing.T) {
					r, payload := hmc330SpanQuery(t, ctx, tc.query, view)
					spans := payload.SpanWindows
					if view == "window_stats" {
						if payload.WindowStats == nil {
							t.Fatal("public window statistics absent")
						}
						spans = payload.WindowStats.TraceSpans
					}
					rows := hmc330BusinessRows(r)
					if len(spans) != 3 || len(rows) != 3 {
						t.Fatalf("native complete pairs missing before presentation: spans=%d observations=%d", len(spans), len(rows))
					}
					for _, span := range spans {
						if span.SourcePath != tc.physical || span.Name != "SharedWork" || span.Thread.PID != 200 {
							t.Fatalf("isolated member or marker payload PID replaced native owner/source: %+v", span)
						}
						if tc.composite && span.SchedulerStates != nil {
							t.Fatal("composite pairing acquired an unproved scheduler account")
						}
					}
					seen := map[string]bool{}
					for _, record := range rows {
						if record.SourceRef.Path != tc.query || record.SourceRef.QueryTargetPID != 200 || record.SourceRef.QueryScopeID == "" ||
							record.SourceRef.PayloadRef == "" || record.Subject != "worker-200" || record.Role != types.AnswerAggregateRoleSupportingCoverage {
							t.Fatalf("capture/query/owner authority changed: %+v", record)
						}
						if seen[record.ID] || len(record.SupportRefs) != 1 || !strings.HasPrefix(record.SupportRefs[0], tc.physical+":") {
							t.Fatalf("pair identity or physical source support collapsed: %+v", record)
						}
						seen[record.ID] = true
						publication, ok := types.DecodeRuntimeMeasurementPublication(record)
						if tc.composite && view == "window_stats" {
							if ok {
								t.Error("composite state-summary privilege was invented from pairing alone")
							}
							continue
						}
						if !ok || len(publication.Tables) != 1 {
							t.Errorf("native paired observation lost reusable summary: capture=%s physical=%s id=%s support=%v", record.SourceRef.Path, tc.physical, record.ID, record.SupportRefs)
							continue
						}
						if view == "span_window" {
							wire, _ := json.Marshal(publication.Tables[0])
							for _, forbidden := range []string{"运行（ms）", "睡眠（ms）", "自身耗时"} {
								if strings.Contains(string(wire), forbidden) {
									t.Errorf("paired duration acquired an unqueried measure %q", forbidden)
								}
							}
							if !strings.Contains(string(wire), tc.physical) {
								t.Errorf("summary hid physical source behind capture identity: %s", wire)
							}
						}
					}
				})
			}
		})
	}
}

func TestHMC330SpanDiscoveryIndependentCapturesKeepAllPairs(t *testing.T) {
	dir, _, bundles := hmc330SpanSources(t)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState("independent native pairs")}
	ids, scopes := map[string]bool{}, map[string]bool{}
	for _, path := range bundles {
		r, payload := hmc330SpanQuery(t, ctx, path, "span_window")
		if len(payload.SpanWindows) != 3 {
			t.Fatal("native query lost one of three paired instances")
		}
		for _, row := range hmc330BusinessRows(r) {
			if ids[row.ID] {
				t.Fatalf("same coordinates in independent captures share observation identity: %s", row.ID)
			}
			ids[row.ID], scopes[row.SourceRef.QueryScopeID] = true, true
		}
	}
	if len(ids) != 6 || len(scopes) != 2 {
		t.Fatalf("native pair/query roster collapsed: pairs=%d queries=%d", len(ids), len(scopes))
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	if got := len(contract.Choices()); got != 6 {
		t.Fatalf("complete native six-pair/two-query roster is not reusable: summaries=%d want=6", got)
	}
}

func TestHMC330SpanDiscoveryForeignSourceCannotBorrowPublication(t *testing.T) {
	dir, physical, bundles := hmc330SpanSources(t)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState("foreign source rejection")}
	r, _ := hmc330SpanQuery(t, ctx, physical[0], "span_window")
	rows := hmc330BusinessRows(r)
	if len(rows) != 3 {
		t.Fatal("single-source native control missing")
	}
	for _, row := range rows {
		if _, ok := types.DecodeRuntimeMeasurementPublication(row); !ok {
			t.Fatal("single-source native control has no publication")
		}
		for _, foreign := range []string{physical[1], bundles[0]} {
			copy := row
			copy.SourceRef.Path = foreign
			if _, ok := types.DecodeRuntimeMeasurementPublication(copy); ok {
				t.Fatalf("changed source borrowed a publication: original=%s foreign=%s", row.SourceRef.Path, foreign)
			}
		}
	}
}
