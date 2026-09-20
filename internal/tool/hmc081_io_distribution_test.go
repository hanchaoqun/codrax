package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are public-tool and context-contract tests, not live model evals.
// Decode the published JSON independently so zero-valued machine fields and
// the model-facing summary cannot silently disappear behind a Go-only test.
func hmc081PublishedDistribution(t *testing.T, row tracequery.StorageLatencySummary) map[string]any {
	t.Helper()
	body, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	distribution, ok := decoded["request_latency_distribution"].(map[string]any)
	if !ok {
		t.Fatalf("public storage row lost its measured request distribution: %s", body)
	}
	for _, field := range []string{"sample_count", "min_ms", "max_ms", "mean_ms", "p50_ms", "p90_ms", "p95_ms", "p99_ms"} {
		if _, ok := distribution[field].(float64); !ok {
			t.Fatalf("distribution omitted numeric field %q, including valid zero: %s", field, body)
		}
	}
	for _, field := range []string{"quantile_method", "sample_policy", "latency_caliber"} {
		if value, ok := distribution[field].(string); !ok || value == "" {
			t.Fatalf("distribution lost explicit measurement semantics %q: %s", field, body)
		}
	}
	return distribution
}

func hmc081AssertNumber(t *testing.T, distribution map[string]any, field string, want float64) {
	t.Helper()
	got, ok := distribution[field].(float64)
	if !ok || math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s=%v, want %.9f; distribution=%+v", field, distribution[field], want, distribution)
	}
}

func hmc081WriteTrace(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "requests.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC081PublicDistributionUsesEveryPairBeyondEventTopEight(t *testing.T) {
	var previousMean float64
	for _, lastMs := range []float64{1, .5} {
		t.Run(fmt.Sprintf("eleventh_request_%.1f_ms", lastMs), func(t *testing.T) {
			var body strings.Builder
			body.WriteString("# tracer: nop\n")
			for i := 0; i < 11; i++ {
				start := 1 + float64(i)*.1
				duration := float64(i + 2)
				if i == 10 {
					duration = lastMs
				}
				fmt.Fprintf(&body, "reader-40 (40) [001] .... %.6f: block_rq_issue: 8,0 R 4096 () %d + 8 [reader]\n", start, 1000+i*8)
				fmt.Fprintf(&body, "irq-2 (2) [001] .... %.6f: block_rq_complete: 8,0 R () %d + 8 [0]\n", start+duration/1000, 1000+i*8)
			}
			path := hmc081WriteTrace(t, body.String())
			bus, _, _ := hmc17NamedPathContext(t)
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1.0, "time_end": 2.1})
			payload := hmc17NamedPayload(t, result)
			if payload.TimeStart != 1 || payload.TimeEnd != 2.1 || payload.WindowStats == nil {
				t.Fatalf("public query changed its explicit window or lost stats: %+v", payload)
			}
			stats := payload.WindowStats
			if len(stats.IOLatencies) != 8 || len(stats.StorageLatencyByLayer) != 1 {
				t.Fatalf("fixture no longer has 11 pairs with an 8-event display: events=%d groups=%d", len(stats.IOLatencies), len(stats.StorageLatencyByLayer))
			}
			for _, event := range stats.IOLatencies {
				if event.Sector == 1080 {
					t.Fatal("changed eleventh request unexpectedly entered displayed Top 8")
				}
			}
			distribution := hmc081PublishedDistribution(t, stats.StorageLatencyByLayer[0])
			for field, want := range map[string]float64{"sample_count": 11, "min_ms": lastMs, "max_ms": 11, "mean_ms": (65 + lastMs) / 11, "p50_ms": 6, "p90_ms": 10, "p95_ms": 10.5, "p99_ms": 10.9} {
				hmc081AssertNumber(t, distribution, field, want)
			}
			mean := distribution["mean_ms"].(float64)
			if previousMean != 0 && mean == previousMean {
				t.Fatal("changing a non-displayed request did not change the full-population mean")
			}
			previousMean = mean
			for _, want := range []string{"request latency in this group (ms): samples=11", "p50=6.000", "p99=10.900", "not target blocking time"} {
				if !strings.Contains(result.Summary, want) {
					t.Fatalf("model-facing tool summary lost %q: %s", want, result.Summary)
				}
			}
			foundObservation := false
			for _, row := range result.Observations {
				if row.Predicate != "storage_latency_by_layer" {
					continue
				}
				foundObservation = true
				context := row.Summary + "\n" + strings.Join(row.RichNotes, "\n")
				if row.Role != types.AnswerAggregateRoleSupportingCoverage || !strings.Contains(context, "samples=11") || !strings.Contains(context, "p99=10.900") || !strings.Contains(context, "not target blocking time") {
					t.Fatalf("typed observation lost distribution or changed supporting role: %+v", row)
				}
				for _, opts := range []types.ObservationPromptProjectionOptions{types.DefaultObservationPromptProjectionOptions(1), types.SemanticReviewObservationPromptProjectionOptions(1)} {
					projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, opts)
					if len(projected) != 1 || !strings.Contains(projected[0].Summary, "samples=11") || !strings.Contains(projected[0].Summary, "p99=10.900") || !strings.Contains(projected[0].Summary, "not target blocking time") {
						t.Fatalf("compact model context lost measured tail or boundary: %+v", projected)
					}
					notes := strings.Join(projected[0].Notes, "\n")
					for _, want := range []string{"layer=block", "event=block_rq", "dev=8,0", "op=R", "issuers=all", "storage_source_path=", "selected_window=1.000000..2.100000", "本组完整配对请求", "不是目标阻塞时长或因果证明"} {
						if !strings.Contains(notes, want) {
							t.Fatalf("compact model context lost %q: %+v", want, projected)
						}
					}
				}
			}
			if !foundObservation {
				t.Fatal("public tool did not publish the storage evidence observation")
			}
		})
	}
}

func TestHMC081PublicZeroRequestIsMeasuredNotMissing(t *testing.T) {
	path := hmc081WriteTrace(t, "reader-40 (40) [001] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [reader]\nirq-2 (2) [001] .... 1.000000: block_rq_complete: 8,0 R () 1000 + 8 [0]\n")
	bus, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": .9, "time_end": 1.1})
	payload := hmc17NamedPayload(t, result)
	if payload.WindowStats == nil || len(payload.WindowStats.StorageLatencyByLayer) != 1 {
		t.Fatalf("valid zero-duration pair lost its group: %+v", payload)
	}
	distribution := hmc081PublishedDistribution(t, payload.WindowStats.StorageLatencyByLayer[0])
	hmc081AssertNumber(t, distribution, "sample_count", 1)
	for _, field := range []string{"min_ms", "max_ms", "mean_ms", "p50_ms", "p90_ms", "p95_ms", "p99_ms"} {
		hmc081AssertNumber(t, distribution, field, 0)
	}
	if !strings.Contains(result.Summary, "samples=1") || !strings.Contains(result.Summary, "p99=0.000") {
		t.Fatalf("model context presented a measured zero as absent: %s", result.Summary)
	}
}

func TestHMC081PublicDistributionPreservesBusinessChainAndExplicitWindow(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := hmc081WriteTrace(t, string(body))
	bus, _, _ := hmc17NamedPathContext(t)
	for _, window := range [][2]float64{{1, 1.05}, {1.01, 1.03}} {
		result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": window[0], "time_end": window[1]})
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != window[0] || payload.TimeEnd != window[1] || payload.WindowStats == nil {
			t.Fatalf("statistics changed explicit bounds: %+v", payload)
		}
		found := map[string]bool{}
		for _, row := range payload.WindowStats.StorageLatencyByLayer {
			if row.Event != "block_rq" {
				continue
			}
			want, relevant := map[string]float64{"R": 35, "W": 47}[row.Operation]
			if !relevant {
				continue
			}
			found[row.Operation] = true
			distribution := hmc081PublishedDistribution(t, row)
			hmc081AssertNumber(t, distribution, "sample_count", 1)
			hmc081AssertNumber(t, distribution, "p99_ms", want)
		}
		if !found["R"] || !found["W"] {
			t.Fatalf("foreground/background request populations disappeared or were clipped: window=%v rows=%+v", window, payload.WindowStats.StorageLatencyByLayer)
		}
	}
	start, end := 1.0, 1.05
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.050"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	seed := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 100, "time_start": start, "time_end": end})
	if !seed.Success {
		t.Fatalf("public seed query failed: %+v", seed)
	}
	bus.ToolResults = append(bus.ToolResults, seed)
	supplement := RunTraceQuerySystemSupplement(bus)
	if !supplement.Attempted || len(supplement.Executed) == 0 || supplement.SkipReason != "" {
		t.Fatalf("automatic causal supplementation regressed: %+v", supplement)
	}
	foundIO, foundBusiness := false, false
	for _, result := range bus.Mutable.SystemTraceSupplementResults() {
		if !result.Success {
			continue
		}
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != start || payload.TimeEnd != end {
			t.Fatalf("causal supplementation replaced explicit bounds: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		if payload.RootCauseRank != nil {
			for _, row := range payload.RootCauseRank.Items {
				if row.Thread.PID == 900 && (row.ChainRelevance == "on_chain" || row.Tier == "primary") {
					t.Fatalf("longer background request gained causal authority: %+v", row)
				}
				foundIO = foundIO || row.Type == "io_latency" && row.Thread.PID == 200 && row.ChainRelevance == "on_chain" && row.ResourceCompletionClosure && math.Abs(row.EffectiveImpactMs-31) < 1e-6
			}
		}
		for _, row := range result.Observations {
			foundBusiness = foundBusiness || strings.Contains(row.Subject, "LoadDocumentIndex") || strings.Contains(row.Object, "LoadDocumentIndex") || strings.Contains(strings.Join(row.RichNotes, "\n"), "LoadDocumentIndex")
		}
	}
	if !foundIO || !foundBusiness {
		t.Fatalf("request distribution displaced verified S-state IO wait or business clues: io=%t business=%t", foundIO, foundBusiness)
	}
}

func TestHMC081IORequestDistributionTeachingReachesExplorerContext(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	explorer, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	parts := append([]string(nil), explorer.Workflow...)
	for _, item := range explorer.WorkflowTierB {
		parts = append(parts, item.Body)
	}
	context := strings.Join(parts, "\n")
	matrix := skill.RenderTraceQueryViewMatrix()
	if !strings.Contains(context, matrix) || !strings.Contains(matrix, skill.TraceIORequestLatencyDistributionTeaching) || !strings.Contains(context, skill.TraceIORequestLatencyDistributionTeaching) {
		t.Fatal("registered explorer workflow lost the shared request-latency teaching")
	}
	for _, boundary := range []string{"sample_count", "p50_ms", "p90_ms", "p95_ms", "p99_ms", "all admitted pairs", "not Top-N requests", "without clipping", "Never average group percentiles", "not target blocking time or causal proof", "IO请求耗时"} {
		if !strings.Contains(skill.TraceIORequestLatencyDistributionTeaching, boundary) {
			t.Fatalf("shared teaching lost measurement or customer-facing boundary %q", boundary)
		}
	}
}

func TestHMC081DistributionDiscreteNotesAreRegisteredDisplayOnlyAndKeepZeroIdentity(t *testing.T) {
	result := tracequery.Result{View: "window_stats", SourcePath: "/capture/source.systrace", WindowStats: &tracequery.WindowStats{
		Window:                       tracequery.TimeWindow{StartTs: 1, EndTs: 2},
		StorageLatencyOverflowGroups: 3, StorageLatencyOverflowPairedCount: 9,
		StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
			SourcePath: "/capture/source.systrace", Layer: "f2fs", Event: "f2fs_direct_io", Dev: "12,80", Inode: "12345", Operation: "R",
			Thread: tracequery.ThreadRef{Comm: "reader", PID: 40}, Count: 2, PairedCount: 1, LineStart: 5, LineEnd: 6, StartTs: 1.5, EndTs: 1.5,
			Example: strings.Repeat("long physical example ", 40),
			RequestLatencyDistribution: &tracequery.IORequestLatencyDistribution{
				SampleCount: 1, QuantileMethod: "linear_interpolation_n_minus_1", SamplePolicy: "complete_pairs_intersecting_query", LatencyCaliber: "full_request_start_to_completion",
			},
		}},
	}}
	records := traceQueryTypedObservations(result, result.SourcePath, "payload.json", "raw.txt", "hmc081", time.Unix(0, 0))
	for _, row := range records {
		if row.Predicate != "storage_latency_by_layer" {
			continue
		}
		notes := map[string]string{}
		for _, note := range row.RichNotes {
			key, value, ok := strings.Cut(note, "=")
			if ok {
				notes[key] = value
			}
		}
		for key, want := range map[string]string{
			"io_request_samples": "1", "io_request_min_ms": "0.000", "io_request_mean_ms": "0.000", "io_request_max_ms": "0.000",
			"io_request_p50_ms": "0.000", "io_request_p90_ms": "0.000", "io_request_p95_ms": "0.000", "io_request_p99_ms": "0.000",
			"io_request_quantile_method": "linear_interpolation_n_minus_1", "io_request_sample_policy": "complete_pairs_intersecting_query", "io_request_latency_caliber": "full_request_start_to_completion",
			"storage_source_path": "/capture/source.systrace", "storage_group_inode": "12345", "storage_group_pid": "40",
			"storage_latency_overflow_groups": "3", "storage_latency_overflow_paired_count": "9",
		} {
			if notes[key] != want {
				t.Fatalf("discrete note %q=%q, want %q; long example must not hide it", key, notes[key], want)
			}
			registered, ok := types.TraceNoteKeyLookup(key)
			if !ok || registered.Carrier != types.TraceNoteCarrierDisplayOnly {
				t.Fatalf("measurement note %q gained a parser/gate or is unregistered: %+v", key, registered)
			}
		}
		if notes["selected_window"] != "1.000000..2.000000" || notes["layer"] != "f2fs" || notes["event"] != "f2fs_direct_io" || notes["dev"] != "12,80" || notes["op"] != "R" {
			t.Fatalf("request distribution lost exact group/query identity: %+v", notes)
		}
		if row.Role != types.AnswerAggregateRoleSupportingCoverage || row.Confidence != .72 || row.Unit != "ms" {
			t.Fatalf("distribution changed supporting measurement authority: %+v", row)
		}
		for _, opts := range []types.ObservationPromptProjectionOptions{types.DefaultObservationPromptProjectionOptions(1), types.SemanticReviewObservationPromptProjectionOptions(1)} {
			projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, opts)
			if len(projected) != 1 {
				t.Fatalf("compact context dropped measured zero group: %+v", projected)
			}
			context := projected[0].Summary + "\n" + strings.Join(projected[0].Notes, "\n")
			for _, want := range []string{"samples=1", "p99=0.000", "layer=f2fs", "event=f2fs_direct_io", "dev=12,80", "op=R", "inode=12345", "pid=40", "/capture/source.systrace", "selected_window=1.000000..2.000000", "omitted_groups=3 omitted_complete_pairs=9"} {
				if !strings.Contains(context, want) {
					t.Fatalf("compact context lost group identity, zero or coverage %q: %+v", want, projected)
				}
			}
		}
		return
	}
	t.Fatal("producer witness did not emit storage latency observation")
}

func TestHMC081PublicBlockGroupIncludesMultipleIssuersWithoutSinglePIDClaim(t *testing.T) {
	path := hmc081WriteTrace(t, "first-40 (40) [001] .... 1.000000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [first]\nirq-2 (2) [001] .... 1.002000: block_rq_complete: 8,0 R () 1000 + 8 [0]\nsecond-41 (41) [001] .... 1.010000: block_rq_issue: 8,0 R 4096 () 2000 + 8 [second]\nirq-2 (2) [001] .... 1.020000: block_rq_complete: 8,0 R () 2000 + 8 [0]\n")
	bus, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": .9, "time_end": 1.1})
	payload := hmc17NamedPayload(t, result)
	if payload.WindowStats == nil || len(payload.WindowStats.StorageLatencyByLayer) != 1 || payload.WindowStats.StorageLatencyByLayer[0].RequestLatencyDistribution.SampleCount != 2 {
		t.Fatalf("block group did not retain both issuers: %+v", payload.WindowStats)
	}
	for _, row := range result.Observations {
		if row.Predicate != "storage_latency_by_layer" {
			continue
		}
		for _, note := range row.RichNotes {
			if strings.HasPrefix(note, "storage_group_pid=") || strings.HasPrefix(note, "storage_group_inode=") {
				t.Fatalf("block representative thread became a group constraint: %+v", row)
			}
		}
		for _, opts := range []types.ObservationPromptProjectionOptions{types.DefaultObservationPromptProjectionOptions(1), types.SemanticReviewObservationPromptProjectionOptions(1)} {
			projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{row}, nil, nil, opts)
			context := projected[0].Summary + "\n" + strings.Join(projected[0].Notes, "\n")
			for _, want := range []string{"samples=2", "p99=9.920", "issuers=all", "dev=8,0", "event=block_rq", "op=R"} {
				if !strings.Contains(context, want) {
					t.Fatalf("compact context lost cross-issuer group %q: %+v", want, projected)
				}
			}
		}
	}
}

func TestHMC081EvidenceFactRequiresUniqueTypedGroupAndPhysicalSource(t *testing.T) {
	base := tracequery.StorageLatencySummary{SourcePath: "/capture/a.systrace", Layer: "block", Event: "block_rq", Dev: "8,0", Operation: "R", LineStart: 2, LineEnd: 4, StartTs: 1, EndTs: 1.01, RequestLatencyDistribution: &tracequery.IORequestLatencyDistribution{SampleCount: 1, P99Ms: 10}}
	fact := tracequery.EvidenceFact{Subject: "block", Predicate: "storage_latency_by_layer", Object: "block_rq", LineStart: 2, LineEnd: 4, StartTs: 1, EndTs: 1.01, Summary: "original fact", Confidence: .72}
	for _, tc := range []struct {
		name   string
		mutate func(*tracequery.Result)
		want   bool
	}{
		{"unique", func(*tracequery.Result) {}, true},
		{"no_counterpart", func(r *tracequery.Result) { r.WindowStats.StorageLatencyByLayer = nil }, false},
		{"wrong_time", func(r *tracequery.Result) { r.EvidencePack[0].EndTs = 1.02 }, false},
		{"wrong_lines", func(r *tracequery.Result) { r.EvidencePack[0].LineEnd = 5 }, false},
		{"different_source", func(r *tracequery.Result) { r.WindowStats.StorageLatencyByLayer[0].SourcePath = "/capture/b.systrace" }, false},
		{"ambiguous_operation", func(r *tracequery.Result) {
			other := base
			other.Operation = "W"
			r.WindowStats.StorageLatencyByLayer = append(r.WindowStats.StorageLatencyByLayer, other)
		}, false},
		{"ambiguous_source", func(r *tracequery.Result) {
			other := base
			other.SourcePath = "/capture/b.systrace"
			r.WindowStats.StorageLatencyByLayer = append(r.WindowStats.StorageLatencyByLayer, other)
		}, false},
		{"source_span_mismatch", func(r *tracequery.Result) {
			r.EvidencePack[0].SourceSpans = []tracequery.TraceArtifactSpan{{SourcePath: "/capture/b.systrace", LocalLineStart: 2, LocalLineEnd: 4}}
		}, false},
		{"source_span_match", func(r *tracequery.Result) {
			r.SourcePath = "/capture/bundle.json"
			r.EvidencePack[0].SourceSpans = []tracequery.TraceArtifactSpan{{SourcePath: base.SourcePath, LocalLineStart: 2, LocalLineEnd: 4}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tracequery.Result{View: "window_stats", SourcePath: base.SourcePath, WindowStats: &tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: .9, EndTs: 1.1}, StorageLatencyByLayer: []tracequery.StorageLatencySummary{base}}, EvidencePack: []tracequery.EvidenceFact{fact}}
			tc.mutate(&result)
			rows := traceQueryTypedObservations(result, result.SourcePath, "payload.json", "raw.txt", "hmc081-facts", time.Unix(0, 0))
			for _, row := range rows {
				if !strings.Contains(row.ID, "#evidence_fact:") {
					continue
				}
				if row.Role != types.AnswerAggregateRoleSupportingCoverage || row.Confidence != .72 || row.Value != "" || row.Unit != "" {
					t.Fatalf("display enrichment changed authority/caliber: %+v", row)
				}
				if tc.want {
					if !strings.Contains(row.Summary, "samples=1") || !strings.Contains(row.Summary, "p99=10.000") || len(row.RichNotes) == 0 {
						t.Fatalf("exact typed counterpart lost stats: %+v", row)
					}
				} else if row.Summary != fact.Summary || len(row.RichNotes) != 0 {
					t.Fatalf("unrelated group statistics borrowed into fact: %+v", row)
				}
				return
			}
			t.Fatal("fact disappeared")
		})
	}
}

func TestHMC081StorageGroupIdentitySurvivesLongPathAndLegacyRowsStayUnchanged(t *testing.T) {
	row := tracequery.StorageLatencySummary{SourcePath: "/capture/" + strings.Repeat("directory/", 30) + "same.systrace", Layer: "block", Event: "block_rq", Dev: "8,0", Operation: "R", RequestLatencyDistribution: &tracequery.IORequestLatencyDistribution{SampleCount: 2, P99Ms: 10}}
	other := row
	other.SourcePath = "/different" + row.SourcePath
	if traceQueryStorageGroupClaim(row) == traceQueryStorageGroupClaim(other) {
		t.Fatal("same basename collapsed different physical sources")
	}
	stats := tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2}, StorageLatencyByLayer: []tracequery.StorageLatencySummary{row}}
	records := traceQueryTypedObservations(tracequery.Result{View: "window_stats", SourcePath: row.SourcePath, WindowStats: &stats}, row.SourcePath, "payload.json", "raw.txt", "hmc081-long", time.Unix(0, 0))
	for _, record := range records {
		if record.Predicate != "storage_latency_by_layer" {
			continue
		}
		for _, opts := range []types.ObservationPromptProjectionOptions{types.DefaultObservationPromptProjectionOptions(1), types.SemanticReviewObservationPromptProjectionOptions(1)} {
			projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{record}, nil, nil, opts)
			if projected[0].Claim != traceQueryStorageGroupClaim(row) || !strings.Contains(strings.Join(projected[0].Notes, "\n"), "…") {
				t.Fatalf("long path did not visibly truncate while retaining exact group identity: %+v", projected)
			}
		}
	}
	row.RequestLatencyDistribution = nil
	if notes := traceQueryStorageDistributionNotes(row, tracequery.WindowStats{}); len(notes) != 0 {
		t.Fatalf("legacy nil distribution gained priority notes: %+v", notes)
	}
	if claim := traceQueryStorageGroupClaim(row); claim != "storage_latency:block" {
		t.Fatalf("legacy nil distribution changed claim: %s", claim)
	}
}

func TestHMC081CompactContextKeepsSameIssuerDeviceAndOperationGroupsSeparate(t *testing.T) {
	var body strings.Builder
	for i, group := range []struct{ dev, op string }{{"8,0", "R"}, {"8,1", "R"}, {"8,0", "W"}} {
		start := 1 + float64(i)*.1
		fmt.Fprintf(&body, "reader-40 (40) [001] .... %.6f: block_rq_issue: %s %s 4096 () %d + 8 [reader]\n", start, group.dev, group.op, 1000+i*8)
		fmt.Fprintf(&body, "irq-2 (2) [001] .... %.6f: block_rq_complete: %s %s () %d + 8 [0]\n", start+float64(i+1)*.01, group.dev, group.op, 1000+i*8)
	}
	path := hmc081WriteTrace(t, body.String())
	bus, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": .9, "time_end": 1.3})
	var groups []types.ObservationRecord
	claims := map[string]bool{}
	for _, row := range result.Observations {
		if strings.Contains(row.ID, "#storage_latency:") {
			if claims[row.ClaimKey] {
				t.Fatalf("different device/operation groups reused one claim: %s", row.ClaimKey)
			}
			claims[row.ClaimKey] = true
			groups = append(groups, row)
		}
	}
	if len(groups) != 3 {
		t.Fatalf("public tool lost a device/operation group: %+v", groups)
	}
	for _, opts := range []types.ObservationPromptProjectionOptions{types.DefaultObservationPromptProjectionOptions(3), types.SemanticReviewObservationPromptProjectionOptions(3)} {
		projected := types.ProjectObservationPromptRecords(groups, nil, nil, opts)
		if len(projected) != 3 {
			t.Fatalf("compact context collapsed independent groups: %+v", projected)
		}
		for _, expected := range []struct{ group, p99 string }{{"dev=8,0 op=R", "p99=10.000"}, {"dev=8,1 op=R", "p99=20.000"}, {"dev=8,0 op=W", "p99=30.000"}} {
			found := false
			for _, row := range projected {
				notes := strings.Join(row.Notes, "\n")
				if !strings.Contains(notes, expected.group) {
					continue
				}
				found = true
				if !strings.Contains(row.Summary, expected.p99) || !strings.Contains(notes, "issuers=all") || !strings.Contains(notes, "不是目标阻塞时长或因果证明") {
					t.Fatalf("statistics or scope moved to another group: %+v", row)
				}
			}
			if !found {
				t.Fatalf("compact context lost group %s: %+v", expected.group, projected)
			}
		}
	}
}
