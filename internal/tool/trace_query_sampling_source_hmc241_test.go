package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingSourceBundleHMC241(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	child := filepath.Join(dir, "physical-samples.perftrace")
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	// Conversion output explicitly attests typed identity; bundle admission
	// must not recover unknown IDs from header spelling alone.
	body = []byte(strings.ReplaceAll(string(body), "tid=11 ", "tid=11 thread_identity_known=true "))
	if err := os.WriteFile(child, body, 0600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "prepared.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundle, []byte(`{"artifacts":[{"type":"perftrace","path":"physical-samples.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}]}`))
	bundle, err = filepath.EvalSymlinks(bundle)
	if err != nil {
		t.Fatal(err)
	}
	child, err = filepath.EvalSymlinks(child)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, child
}

func samplingSourceRecordHMC241(t *testing.T, result types.ToolResult) types.ObservationRecord {
	t.Helper()
	for _, record := range result.Observations {
		if record.Predicate == TracePerfSamplingTreePredicate {
			return record
		}
	}
	t.Fatalf("sampling tree receipt missing: %s", result.Summary)
	return types.ObservationRecord{}
}

func TestSamplingPreparedSourceSelectionPublicHMC241(t *testing.T) {
	bundle, child := samplingSourceBundleHMC241(t)
	for _, view := range samplingSelectionViewsHMC235 {
		t.Run(view, func(t *testing.T) {
			result, payload := samplingIdentityQueryHMC239(t, bundle, view, map[string]any{"pid": 11})
			if payload.SourcePath != bundle || len(payload.TraceArtifacts) != 1 || payload.TraceArtifacts[0].SourcePath != child {
				t.Fatalf("fixture did not traverse verified preparation/member separation: %+v", payload.TraceArtifacts)
			}
			r := samplingSourceRecordHMC241(t, result)
			p, ok := DecodeTracePerfSamplingTree(r)
			if !ok || p.SampleCount != 5 || len(p.Cohorts) != 3 {
				t.Fatalf("native sampling denominator changed: bound=%t tree=%+v", ok, p)
			}
			before, _ := json.Marshal(r)
			if subject, ok := TraceQueryAggregateSelectionSubject(r); !ok || subject != "document-worker-11" {
				t.Errorf("validated manifest/member path difference lost selected subject: subject=%q ok=%t parent=%q member=%q", subject, ok, p.SourcePath, p.Cohorts[0].SourcePath)
			}
			after, _ := json.Marshal(r)
			if !bytes.Equal(before, after) {
				t.Fatal("display source bridge mutated the producer record")
			}
		})
	}
}

func TestSamplingPreparedSourceBindingNegativesHMC241(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	result, _ := samplingIdentityQueryHMC239(t, bundle, "perf_sampling_tree", map[string]any{"pid": 11})
	base := samplingSourceRecordHMC241(t, result)
	for _, tc := range []struct {
		name string
		edit func(*types.ObservationRecord, *tracequery.PerfSamplingTreeResult)
	}{
		{"parent_source", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.Path += ".other"
			p.SourcePath = r.SourceRef.Path
		}},
		{"member_source", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			for i := range p.Cohorts {
				p.Cohorts[i].SourcePath += ".other"
			}
		}},
		{"member_generation", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			for i := range p.Cohorts {
				p.Cohorts[i].SourceGeneration += ".other"
			}
		}},
		{"query_scope", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryScopeID += ".other"
		}},
		{"payload_ref", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.PayloadRef += ".other"
		}},
		{"window", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryWindowEndTs = 3
			r.Span.EndTs = 3
			p.Window.EndTs = 3
		}},
		{"target", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 12
			p.TargetPID = 12
			for i := range p.Cohorts {
				p.Cohorts[i].ThreadIdentity.TID = 12
			}
		}},
		{"unknown_thread", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[0].ThreadIdentity = nil
		}},
		{"missing_source_receipt", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			notes := r.RichNotes[:0]
			for _, n := range r.RichNotes {
				if !strings.HasPrefix(n, "perf_sampling_source_binding=") {
					notes = append(notes, n)
				}
			}
			r.RichNotes = notes
		}},
		{"duplicate_source_receipt", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			for _, n := range r.RichNotes {
				if strings.HasPrefix(n, "perf_sampling_source_binding=") {
					r.RichNotes = append(r.RichNotes, n)
					break
				}
			}
		}},
		{"unknown_receipt_field", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			for i, n := range r.RichNotes {
				if strings.HasPrefix(n, "perf_sampling_source_binding={") {
					r.RichNotes[i] = strings.Replace(n, "={", `={"unrecognized":true,`, 1)
				}
			}
		}},
		{"duplicate_receipt_field", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			for i, n := range r.RichNotes {
				if strings.HasPrefix(n, "perf_sampling_source_binding={") {
					r.RichNotes[i] = strings.Replace(n, "={", `={"version":1,`, 1)
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var r types.ObservationRecord
			_ = json.Unmarshal(raw, &r)
			p, ok := DecodeTracePerfSamplingTree(r)
			if !ok {
				t.Fatal("invalid baseline")
			}
			tc.edit(&r, &p)
			body, _ := json.Marshal(p)
			for i, n := range r.RichNotes {
				if strings.HasPrefix(n, "perf_sampling_tree=") {
					r.RichNotes[i] = "perf_sampling_tree=" + string(body)
				}
			}
			if subject, ok := TraceQueryAggregateSelectionSubject(r); ok {
				t.Fatalf("unbound source/query/member acquired display target: %q", subject)
			}
		})
	}
}

func TestSamplingPreparedSourceAdmissionPublicHMC241(t *testing.T) {
	for _, mode := range []string{"known", "unknown_capability", "not_ready", "unknown_row"} {
		t.Run(mode, func(t *testing.T) {
			bundle, child := samplingSourceBundleHMC241(t)
			manifest, err := os.ReadFile(bundle)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(manifest, &doc); err != nil {
				t.Fatal(err)
			}
			capability := doc["artifacts"].([]any)[0].(map[string]any)["perf_capability"].(map[string]any)
			switch mode {
			case "unknown_capability":
				capability["thread_identity"] = "unknown"
			case "not_ready":
				capability["trace_query_ready"] = false
			case "unknown_row":
				body, err := os.ReadFile(child)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(child, []byte(strings.ReplaceAll(string(body), "thread_identity_known=true", "thread_identity_known=false")), 0600); err != nil {
					t.Fatal(err)
				}
			}
			manifest, _ = json.Marshal(doc)
			writeToolTraceBundleV2Fixture(t, bundle, manifest)
			for _, view := range []string{"perf_sampling_tree", "window_stats"} {
				if mode == "not_ready" && view == "window_stats" {
					dir := t.TempDir()
					ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling source admission")}
					params, _ := json.Marshal(map[string]any{"source": "path", "path": bundle, "view": view, "pid": 11, "time_start": 1, "time_end": 2})
					result, err := (&TraceQuery{}).Execute(ctx, params)
					if err != nil || !result.Success {
						t.Fatalf("not-ready companion boundary query failed: %v %s", err, result.Summary)
					}
					for _, r := range result.Observations {
						if _, selected := TraceQueryAggregateSelectionSubject(r); selected {
							t.Fatal("not-ready companion acquired thread selection")
						}
					}
					continue // Indexed no-event windows legitimately have no sampling product.
				}
				result, payload := samplingIdentityQueryHMC239(t, bundle, view, map[string]any{"pid": 11})
				r := samplingSourceRecordHMC241(t, result)
				_, selected := TraceQueryAggregateSelectionSubject(r)
				if selected != (mode == "known") {
					t.Errorf("%s/%s gained or lost admitted thread selection: selected=%t tree=%+v", mode, view, selected, payload.PerfSamplingTree)
				}
				if mode == "unknown_capability" || mode == "unknown_row" {
					if payload.PerfSamplingTree.Status != "unavailable" || payload.PerfSamplingTree.Reason != "typed_identity_unproven" {
						t.Errorf("%s/%s unknown was silently treated as proven/empty: %+v", mode, view, payload.PerfSamplingTree)
					}
				}
				if mode == "not_ready" && payload.PerfSamplingTree.SampleCount != 0 {
					t.Errorf("not-ready child entered %s population", view)
				}
			}
		})
	}
}
