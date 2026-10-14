package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func filteredPerfRecordsHMC242(t *testing.T, result types.ToolResult) []types.ObservationRecord {
	t.Helper()
	var out []types.ObservationRecord
	for _, r := range result.Observations {
		if _, ok, _ := TraceQueryFilteredPerfMemberSelection(r); ok {
			out = append(out, r)
		}
	}
	return out
}

func TestFilteredPerfPublicQueryHMC242(t *testing.T) {
	bundle, direct := samplingSourceBundleHMC241(t)
	for _, path := range []string{bundle, direct} {
		for _, view := range []string{"window_stats", "perf_stats", "trace_perf_bundle"} {
			for _, filter := range []map[string]any{{"pattern": "dso=/app/A.so"}, {"event_types": []string{"perf_sample"}}, {"line_start": 2, "line_end": 5}} {
				filter["pid"] = 11
				r, p := samplingIdentityQueryHMC239(t, path, view, filter)
				if p.PerfSamplingTree == nil || p.PerfSamplingTree.Status != "unavailable" || p.PerfSamplingTree.Reason != "requires_time_and_thread_selectors_only" {
					t.Fatalf("tree filter diagnosis changed: %+v", p.PerfSamplingTree)
				}
				rows := filteredPerfRecordsHMC242(t, r)
				if len(rows) == 0 {
					t.Fatalf("no independent member receipt: %s %v", view, filter)
				}
				for _, row := range rows {
					if subject, ok := TraceQueryAggregateSelectionSubject(row); !ok || subject != "document-worker-11" {
						t.Fatalf("missing own target: %q %t", subject, ok)
					}
				}
				if filter["pattern"] != "dso=/app/A.so" {
					continue
				}
				if len(rows) != 3 {
					t.Fatalf("filtered cohort rows=%d", len(rows))
				}
				weights := map[int64]int{}
				for _, row := range rows {
					pop, _ := runtimeTracePerfQualityScope(row)
					weights[pop.Weight] = pop.Samples
					if row.Object != "/app/A.so" {
						t.Fatal("B leaked through literal filter")
					}
				}
				if weights[15] != 2 || weights[100] != 1 || weights[7] != 1 || len(weights) != 3 {
					t.Fatalf("denominators=%v", weights)
				}
			}
		}
	}
}

func TestFilteredPerfReceiptBoundariesHMC242(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	r, _ := samplingIdentityQueryHMC239(t, bundle, "window_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
	base := filteredPerfRecordsHMC242(t, r)
	if len(base) != 4 {
		t.Fatalf("baseline=%d", len(base))
	}
	for _, name := range []string{"source", "scope", "payload", "window", "target", "process", "line", "clock", "offset", "slope", "canonical_clock", "role", "negative", "value", "generation", "filter", "completeness", "duplicate", "unknown_field", "truncated_json", "borrow_sibling"} {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(base[0])
			var row types.ObservationRecord
			_ = json.Unmarshal(b, &row)
			switch name {
			case "source":
				row.SourceRef.Path += ".other"
			case "scope":
				row.SourceRef.QueryScopeID += ".other"
			case "payload":
				row.SourceRef.PayloadRef += ".other"
			case "window":
				row.SourceRef.QueryWindowEndTs++
			case "target":
				row.SourceRef.QueryTargetPID++
			case "process":
				row.SourceRef.QueryTargetScope = "process"
			case "line":
				row.SourceRef.QueryLineEnd++
			case "clock":
				row.SourceRef.TimeDomain = "other"
			case "canonical_clock":
				row.SourceRef.CanonicalTimeDomain = "other"
			case "offset":
				x := 3.0
				row.SourceRef.ClockOffsetSec = &x
			case "slope":
				x := 2.0
				row.SourceRef.ClockSlope = &x
			case "role":
				row.Role = types.AnswerAggregateRolePrincipalAnswer
			case "negative":
				row.Negative = true
			case "value":
				row.Value = "900"
			default:
				for i, n := range row.RichNotes {
					if !strings.HasPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=") {
						continue
					}
					var receipt traceQueryFilteredPerfMemberReceipt
					_ = json.Unmarshal([]byte(strings.TrimPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=")), &receipt)
					switch name {
					case "generation":
						receipt.CohortID += "other"
					case "filter":
						receipt.Filters.Pattern = "other"
					case "completeness":
						receipt.MembersComplete = !receipt.MembersComplete
					case "duplicate":
						row.RichNotes = append(row.RichNotes, n)
						continue
					case "unknown_field":
						row.RichNotes[i] = strings.Replace(n, "={", `={"unknown":true,`, 1)
						continue
					case "truncated_json":
						row.RichNotes[i] = n[:len(n)-2]
						continue
					case "borrow_sibling":
						for _, other := range base[1].RichNotes {
							if strings.HasPrefix(other, types.TraceNoteKeyPerfFilteredMember+"=") {
								row.RichNotes[i] = other
							}
						}
						continue
					}
					data, _ := json.Marshal(receipt)
					row.RichNotes[i] = types.TraceNoteKeyPerfFilteredMember + "=" + string(data)
				}
			}
			if _, ok, _ := TraceQueryFilteredPerfMemberSelection(row); ok {
				t.Fatal("altered member acquired selection")
			}
		})
	}
}

func TestFilteredPerfUnknownOverflowAndTopNHMC242(t *testing.T) {
	for _, mode := range []string{"unknown", "overflow", "topn"} {
		t.Run(mode, func(t *testing.T) {
			var b strings.Builder
			for i := 0; i < 12; i++ {
				known, weight, symbol := "true", "1", fmt.Sprintf("work%d", i)
				if mode == "unknown" {
					known = "false"
				}
				if mode == "overflow" {
					weight = "9223372036854775807"
					symbol = "work"
				}
				fmt.Fprintf(&b, "worker-11 (10) [000] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=%s sample_weight=%s event=cycles symbol=%s dso=/app/A.so source=raw\n", 100+i, known, weight, symbol)
			}
			path := filepath.Join(t.TempDir(), "samples.perftrace")
			if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
				t.Fatal(err)
			}
			r, _ := samplingIdentityQueryHMC239(t, path, "window_stats", map[string]any{"pid": 11, "pattern": "dso=/app/A.so"})
			rows := filteredPerfRecordsHMC242(t, r)
			if mode != "topn" {
				if len(rows) != 0 {
					t.Fatal("unknown/overflow minted a member receipt")
				}
				return
			}
			if len(rows) == 0 || len(rows) >= 12 {
				t.Fatalf("topn fixture not capped: %d", len(rows))
			}
			for _, row := range rows {
				_, ok, complete := TraceQueryFilteredPerfMemberSelection(row)
				pop, _ := runtimeTracePerfQualityScope(row)
				if !ok || complete || pop.Weight != 12 || pop.Samples != 12 {
					t.Fatalf("topn changed denominator/completeness: %t %t %+v", ok, complete, pop)
				}
			}
		})
	}
}
