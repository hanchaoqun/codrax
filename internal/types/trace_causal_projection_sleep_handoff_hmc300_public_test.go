package types_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC300SleepInstanceQueryHandoff(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var results []types.ToolResult
	for _, pid := range []int{100, 200} {
		for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": .999, "time_end": 1.051})
			r, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !r.Success {
				t.Fatalf("query pid=%d view=%s: %v %s", pid, view, err, r.Summary)
			}
			results = append(results, r)
			label := fmt.Sprintf("native pid=%d view=%s", pid, view)
			for _, record := range r.Observations {
				if record.Subject == "document-worker-200" && record.Predicate == "wakeup_causal_impact" {
					encoded, _ := json.Marshal(record)
					t.Logf("%s record=%s", label, encoded)
				}
			}
			hmc300LogSleepProjection(t, label, types.CompileTraceCausalProjection(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})))
		}
	}
	mut := types.NewMutableState("Inspect recorded sleep instances")
	for _, r := range results {
		mut.AppendDispatchToolResult(r)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: mut, ToolResults: results}, types.ObservationExtractLedgerEvidenceLimit))
	closedRecords := 0
	for _, record := range ledger.Records {
		if record.Subject == "document-worker-200" && record.Predicate == "wakeup_causal_impact" {
			t.Logf("ledger id=%s span=%+v value=%s", record.ID, record.Span, record.Value)
			if record.Span.EndTs == 1.040010 {
				closedRecords++
			}
		}
	}
	if closedRecords == 0 {
		t.Fatal("native closed interval missing before projection")
	}
	p := types.CompileTraceCausalProjection(ledger)
	hmc300LogSleepProjection(t, "combined", p)
	closed := 0
	for _, bucket := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SupportingHops} {
		for _, n := range bucket {
			if n.Subject == "document-worker-200" && n.IsSleepState() && n.EndTs == 1.040010 {
				closed++
				if !n.DrilldownWakeupPointKnown || n.DrilldownWakeupTs != 1.040010 || n.DrilldownWakeupLine != 12 {
					t.Errorf("exact closed wait lost closure: %+v", n)
				}
			}
		}
	}
	if closed == 0 {
		t.Fatal("closed ledger interval lost in combined projection")
	}
	want := hmc300SleepFacts(p)
	for _, reverse := range []bool{false, true} {
		for _, displayChanged := range []bool{false, true} {
			raw, _ := json.Marshal(results)
			var reordered []types.ToolResult
			if err := json.Unmarshal(raw, &reordered); err != nil {
				t.Fatal(err)
			}
			if reverse {
				for i, j := 0, len(reordered)-1; i < j; i, j = i+1, j-1 {
					reordered[i], reordered[j] = reordered[j], reordered[i]
				}
			}
			if displayChanged {
				// Opaque observation IDs may change with an additive payload
				// presentation field. Neither their ordering nor prose is a
				// physical measurement identity; native scopes stay unchanged.
				for i := range reordered {
					for j := range reordered[i].Observations {
						r := &reordered[i].Observations[j]
						if r.Predicate == "wakeup_causal_impact" {
							r.ID = fmt.Sprintf("display-variant-%03d-%03d", len(reordered)-i, len(reordered[i].Observations)-j)
							r.Summary += " (additional presentation detail)"
						}
					}
				}
			}
			projected := types.CompileTraceCausalProjection(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: reordered}))
			if got := hmc300SleepFacts(projected); !reflect.DeepEqual(got, want) {
				t.Fatalf("reverse=%t displayChanged=%t changed physical sleep instances\nwant=%v\ngot=%v", reverse, displayChanged, want, got)
			}
		}
	}
}

func hmc300SleepFacts(p types.TraceCausalProjection) []string {
	var facts []string
	for _, bucket := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SupportingHops} {
		for _, n := range bucket {
			if n.Subject == "document-worker-200" && n.IsSleepState() {
				facts = append(facts, fmt.Sprintf("%.9f..%.9f:%.6f:wake=%t:%.9f:%d", n.StartTs, n.EndTs, n.ImpactMS, n.DrilldownWakeupPointKnown, n.DrilldownWakeupTs, n.DrilldownWakeupLine))
			}
		}
	}
	sort.Strings(facts)
	return facts
}

func hmc300LogSleepProjection(t *testing.T, label string, p types.TraceCausalProjection) {
	t.Helper()
	for _, bucket := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SupportingHops} {
		for _, n := range bucket {
			if n.Subject == "document-worker-200" && n.IsSleepState() {
				t.Logf("%s projection id=%s span=%.9f..%.9f impact=%g cumulative=%g lines=%d..%d merged=%v merge_count=%d family=%d wake=%t origins=%d", label, n.EvidenceID, n.StartTs, n.EndTs, n.ImpactMS, n.CumulativeImpactMS, n.LineStart, n.LineEnd, n.MergedEvidenceIDs, n.MergedCount, n.FamilyMemberCount, n.DrilldownWakeupPointKnown, len(n.MeasurementOrigins))
			}
		}
	}
}
