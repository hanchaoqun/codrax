package tracediag

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestIORequestDistributionFieldDisposition(t *testing.T) {
	typ := reflect.TypeOf(tracequery.IORequestLatencyDistribution{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		got = append(got, f.Name+"|"+f.Type.String()+"|"+f.Tag.Get("json"))
	}
	want := []string{
		"SampleCount|int|sample_count", "MinMs|float64|min_ms", "MaxMs|float64|max_ms", "MeanMs|float64|mean_ms",
		"P50Ms|float64|p50_ms", "P90Ms|float64|p90_ms", "P95Ms|float64|p95_ms", "P99Ms|float64|p99_ms",
		"QuantileMethod|string|quantile_method", "SamplePolicy|string|sample_policy", "LatencyCaliber|string|latency_caliber",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("distribution fields need explicit zero/value/caliber render disposition: got=%q want=%q", got, want)
	}
	field, ok := reflect.TypeOf(tracequery.StorageLatencySummary{}).FieldByName("RequestLatencyDistribution")
	if !ok || field.Type != reflect.PointerTo(typ) || field.Tag.Get("json") != "request_latency_distribution,omitempty" {
		t.Fatalf("distribution must remain an optional typed child: %+v", field)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, reflect.TypeOf(tracequery.StorageLatencySummary{}), "RequestLatencyDistribution") {
		t.Fatal("distribution must remain visible under its exact source/family/device/operation group")
	}
	for _, field := range []string{"StorageLatencyOverflowGroups", "StorageLatencyOverflowPairedCount"} {
		if !policySkipsDetailField(&nonEventDetailPolicy, reflect.TypeOf(tracequery.WindowStats{}), field) {
			t.Fatalf("key-first overflow disclosure %s must not be duplicated by generic detail", field)
		}
	}
}

// Removing only the reviewed additive fields must reproduce the prior
// schema fingerprints. Nested distribution fields get their own explicit pin.
func TestIORequestDistributionSchemaEvolutionIsAdditive(t *testing.T) {
	for _, tc := range []struct {
		typ      reflect.Type
		previous string
		added    map[string]bool
	}{
		{reflect.TypeOf(tracequery.WindowStats{}), "99190b281118d0e07633db78707e85c985fc5014c7e793fc53a886583467a63b", map[string]bool{
			"StorageLatencyOverflowGroups|int|storage_latency_overflow_groups,omitempty":            true,
			"StorageLatencyOverflowPairedCount|int|storage_latency_overflow_paired_count,omitempty": true,
		}},
		{reflect.TypeOf(tracequery.StorageLatencySummary{}), "0dd6c71d18f36308bc3771f2dd87270d3c02a194f0b3051ceaffc36a961a7559", map[string]bool{
			"RequestLatencyDistribution|*tracequery.IORequestLatencyDistribution|request_latency_distribution,omitempty": true,
			"RequestResidenceCaliber|string|request_residence_caliber,omitempty":                                         true,
		}},
	} {
		t.Run(tc.typ.Name(), func(t *testing.T) {
			_, schema := detailSchemaFingerprint(tc.typ)
			schema = nonEventSchemaBeforeIOInFlight(t, tc.typ, schema)
			var previous []string
			addedCount := 0
			for _, field := range strings.Split(schema, ";") {
				if tc.added[field] {
					addedCount++
					continue
				}
				previous = append(previous, field)
			}
			if addedCount != len(tc.added) {
				t.Fatalf("reviewed additions missing: got %d want %d; schema=%s", addedCount, len(tc.added), schema)
			}
			sum := sha256.Sum256([]byte(strings.Join(previous, ";")))
			if got := hex.EncodeToString(sum[:]); got != tc.previous {
				t.Fatalf("unreviewed schema change: got=%s want=%s", got, tc.previous)
			}
		})
	}
}

func TestStorageRequestResidenceCaliberRenderDisposition(t *testing.T) {
	field, ok := reflect.TypeOf(tracequery.StorageLatencySummary{}).FieldByName("RequestResidenceCaliber")
	if !ok || field.Type.Kind() != reflect.String || field.Tag.Get("json") != "request_residence_caliber,omitempty" {
		t.Fatalf("endpoint ruler needs explicit optional scalar disposition: %+v", field)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, reflect.TypeOf(tracequery.StorageLatencySummary{}), field.Name) {
		t.Fatal("endpoint ruler belongs to the group detail, not the key-first aggregate")
	}
	for _, tc := range []struct{ caliber, endpoints string }{
		{tracequery.BlockIOWaitCaliberIssueToComplete, "block_rq_issue → block_rq_complete"},
		{tracequery.BlockIOWaitCaliberBIOQueueToComplete, "block_bio_queue → block_bio_complete"},
		{"future_caliber", "未说明；不据事件族推定耗时口径"},
	} {
		for _, measured := range []bool{false, true} {
			group := tracequery.StorageLatencySummary{Layer: "block", Event: "block_rq", RequestResidenceCaliber: tc.caliber, UnpairedDoneCount: 1}
			if measured {
				group.RequestLatencyDistribution = &tracequery.IORequestLatencyDistribution{SampleCount: 1}
			}
			r := tracequery.Result{WindowStats: &tracequery.WindowStats{StorageLatencyByLayer: []tracequery.StorageLatencySummary{group}}}
			before, _ := json.Marshal(r)
			for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
				var lines []string
				renderResultDetailWithPolicy(&r, func(s string) { lines = append(lines, s) }, policy)
				out := strings.Join(lines, "\n")
				if !strings.Contains(out, tc.endpoints) || strings.Count(out, "请求起止事件:") != 1 || strings.Contains(out, tc.caliber) {
					t.Errorf("ruler lost, duplicated or internal enum leaked: %s", out)
				}
				if strings.Contains(out, "P99=") != measured {
					t.Errorf("nil/real zero distribution conflated: %s", out)
				}
			}
			after, _ := json.Marshal(r)
			if !bytes.Equal(before, after) {
				t.Fatal("endpoint rendering mutated measurement")
			}
		}
	}
}

func TestIORequestDistributionRenderingPreservesZeroAndAuthority(t *testing.T) {
	r := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
		StorageLatencyByLayer: []tracequery.StorageLatencySummary{{Layer: "block", Event: "block_rq", Dev: "8,0", Operation: "R",
			RequestLatencyDistribution: &tracequery.IORequestLatencyDistribution{
				SampleCount: 2, MinMs: 0, MaxMs: 2000000000, MeanMs: 1000000000,
				P50Ms: 1000000000, P90Ms: 1800000000, P95Ms: 1900000000, P99Ms: 1980000000,
				QuantileMethod: "linear_interpolation_n_minus_1", SamplePolicy: "complete_pairs_intersecting_query", LatencyCaliber: "full_request_start_to_completion",
			}}},
	}}
	before, _ := json.Marshal(r)
	for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
		var lines []string
		renderResultDetailWithPolicy(&r, func(s string) { lines = append(lines, s) }, policy)
		report := strings.Join(lines, "\n")
		for _, want := range []string{
			"window_stats.storage_latency_by_layer[0].request_latency_distribution:",
			"已配对请求=2", "最短=0.000 ms", "最长=2000000000.000 ms", "平均=1000000000.000 ms",
			"P50=1000000000.000 ms", "P90=1800000000.000 ms", "P95=1900000000.000 ms", "P99=1980000000.000 ms",
			"该组内与查询范围相交且起止完整配对的全部请求", "请求起始至完成的全程耗时，不按窗口边界裁剪",
			"按排序样本的位置线性插值", "不等于线程阻塞或响应耗时，也不单独证明链上根因",
		} {
			if !strings.Contains(report, want) {
				t.Errorf("missing %q:\n%s", want, report)
			}
		}
		for _, absent := range []string{"e+", "linear_interpolation_n_minus_1", "complete_pairs_intersecting_query", "full_request_start_to_completion"} {
			if strings.Contains(report, absent) {
				t.Errorf("internal enum/scientific notation leaked: %s\n%s", absent, report)
			}
		}
		if strings.Count(report, "request_latency_distribution:") != 1 {
			t.Fatalf("distribution must render exactly once:\n%s", report)
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("rendering changed underlying measurement or authority")
	}
	r.WindowStats.StorageLatencyByLayer[0].RequestLatencyDistribution = nil
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 100}, stepOutcome{result: &r}).lines, "\n")
	if strings.Contains(report, "request_latency_distribution") || strings.Contains(report, "统计口径") || strings.Contains(report, "P99=") {
		t.Fatal("absent distribution was rendered as a zero-duration measurement")
	}
}

func TestIORequestDistributionUnknownEnumsStayNonAuthoritative(t *testing.T) {
	var lines []string
	renderIORequestLatencyDistributionDetail(tracequery.IORequestLatencyDistribution{
		QuantileMethod: "future_quantile", SamplePolicy: "future_population", LatencyCaliber: "future_caliber",
	}, "distribution", func(s string) { lines = append(lines, s) })
	report := strings.Join(lines, "\n")
	for _, want := range []string{"已配对请求=0", "最短=0.000 ms", "P99=0.000 ms", "分位数算法未说明", "样本范围未说明", "耗时口径未说明"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing required zero/unknown disclosure %s: %s", want, report)
		}
	}
	if strings.Contains(report, "future_") || strings.Contains(report, "线性插值") {
		t.Fatal("unknown typed metadata was leaked or assigned a known meaning")
	}
}

func TestIORequestDistributionFollowsGroupIdentity(t *testing.T) {
	group := tracequery.StorageLatencySummary{
		SourcePath: "source-a.ftrace", Layer: "f2fs", Event: "f2fs_direct_io", Dev: "8,0", Inode: "inode-11",
		EntryName: "image.cache", Operation: "R", Thread: tracequery.ThreadRef{PID: 40, Comm: "image-loader"},
		RequestLatencyDistribution: &tracequery.IORequestLatencyDistribution{SampleCount: 1},
	}
	raw, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	valueAt := strings.Index(string(raw), `"request_latency_distribution":`)
	for _, field := range []string{"source_path", "layer", "event", "dev", "inode", "entry_name", "operation", "thread"} {
		identityAt := strings.Index(string(raw), `"`+field+`":`)
		if identityAt < 0 || valueAt < identityAt {
			t.Fatalf("truncated JSON could expose a distribution before group field %s: %s", field, raw)
		}
	}
	r := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{StorageLatencyByLayer: []tracequery.StorageLatencySummary{group}}}
	full := renderStepBody(&Step{View: "window_stats", effMaxLines: 100}, stepOutcome{result: &r}).lines
	// Every prefix that exposes statistics must already contain its original
	// group identity; no global P99 can be fabricated by detail-line truncation.
	seenDistribution := false
	for cap := 1; cap <= len(full); cap++ {
		report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: cap}, stepOutcome{result: &r}).lines, "\n")
		valueAt := strings.Index(report, ".request_latency_distribution:")
		if valueAt < 0 {
			continue
		}
		seenDistribution = true
		for _, identity := range []string{"来源=source-a.ftrace", "层=f2fs", "事件族=f2fs_direct_io", "设备=8,0", "inode=inode-11", "名称=image.cache", "操作=R", "image-loader-40"} {
			identityAt := strings.Index(report, identity)
			if identityAt < 0 || valueAt < identityAt {
				t.Fatalf("line cap %d exposed statistics without prior identity %s:\n%s", cap, identity, report)
			}
		}
	}
	if !seenDistribution {
		t.Fatal("distribution never rendered in the full report")
	}
}

func TestIORequestDistributionOverflowDisclosureSurvivesSmallCap(t *testing.T) {
	r := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
		StorageLatencyByLayer:        []tracequery.StorageLatencySummary{{Layer: "block", PairedCount: 3}},
		StorageLatencyOverflowGroups: 4, StorageLatencyOverflowPairedCount: 19,
	}}
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 3}, stepOutcome{result: &r}).lines, "\n")
	for _, want := range []string{"已展示分组=1 未展示分组=4 未展示已配对请求=19", "请求耗时不等于线程阻塞或响应耗时"} {
		if !strings.Contains(report, want) {
			t.Fatalf("small-cap report hid the full population boundary %s:\n%s", want, report)
		}
	}
	report = strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 100}, stepOutcome{result: &r}).lines, "\n")
	if strings.Count(report, "IO请求统计展示范围") != 1 || strings.Contains(report, "storage_latency_overflow_") {
		t.Fatalf("overflow has two contradictory rendering owners:\n%s", report)
	}
}

func TestIORequestDistributionActualEngineZeroDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero-request.ftrace")
	const body = `io-40 (40) [003] .... 6793224.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-80 (2) [003] .... 6793224.000000: block_rq_complete: 8,0 R () 123 + 8 [0]
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: 6793224, TimeEnd: 6793224.01})
	if r.WindowStats == nil || len(r.WindowStats.StorageLatencyByLayer) != 1 || r.WindowStats.StorageLatencyByLayer[0].RequestLatencyDistribution == nil {
		t.Fatalf("actual paired zero-duration request lost distribution: %+v", r.WindowStats)
	}
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &r}).lines, "\n")
	for _, want := range []string{"已配对请求=1", "最短=0.000 ms", "最长=0.000 ms", "平均=0.000 ms", "P50=0.000 ms", "P90=0.000 ms", "P95=0.000 ms", "P99=0.000 ms"} {
		if !strings.Contains(report, want) {
			t.Fatalf("actual measured zero vanished, missing %q:\n%s", want, report)
		}
	}
	// The existing raw-evidence appendix retains engine originals. The typed
	// detail lane must not duplicate Summary and the new numeric child itself.
	detail, _, _ := strings.Cut(report, "观测记录(")
	if strings.Count(detail, "P99=") != 1 || strings.Contains(detail, "request latency in this group") {
		t.Fatalf("typed distribution was duplicated through the engine Summary:\n%s", report)
	}
	for cap := 1; cap <= 20; cap++ {
		prefix := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: cap}, stepOutcome{result: &r}).lines, "\n")
		valueAt := strings.Index(prefix, "P99=")
		if valueAt < 0 {
			continue
		}
		for _, identity := range []string{"来源=zero-request.ftrace", "事件族=block_rq", "设备=8,0", "操作=R", "代表提交者=io-40(tgid=40)（不限定本组线程）"} {
			identityAt := strings.Index(prefix, identity)
			if identityAt < 0 || identityAt > valueAt {
				t.Fatalf("actual Summary-bearing group lost prior identity %s under cap %d:\n%s", identity, cap, prefix)
			}
		}
	}
	if strings.Contains(report, filepath.Dir(path)) {
		t.Fatal("distribution renderer exposed the collection machine's directory")
	}
}

func TestIORequestDistributionBlockRepresentativeDoesNotNarrowPopulation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two-issuers.ftrace")
	const body = `reader-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [reader]
irq-80 (2) [003] .... 1.001000: block_rq_complete: 8,0 R () 123 + 8 [0]
loader-41 (41) [003] .... 1.010000: block_rq_issue: 8,0 R 4096 () 456 + 8 [loader]
irq-80 (2) [003] .... 1.013000: block_rq_complete: 8,0 R () 456 + 8 [0]
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.02})
	if r.WindowStats == nil || len(r.WindowStats.StorageLatencyByLayer) != 1 || r.WindowStats.StorageLatencyByLayer[0].PairedCount != 2 {
		t.Fatalf("two issuers must share one source/family/device/operation group: %+v", r.WindowStats)
	}
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &r}).lines, "\n")
	for _, want := range []string{"代表提交者=reader-40(tgid=40)（不限定本组线程）", "已配对请求=2", "最短=1.000 ms", "最长=3.000 ms", "该组内与查询范围相交"} {
		if !strings.Contains(report, want) {
			t.Fatalf("representative narrowed the measurement population; missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "组内线程=") {
		t.Fatalf("block group was falsely scoped to a unique submitting PID:\n%s", report)
	}
}

func TestIORequestDistributionNilKeepsLegacySummaryBytes(t *testing.T) {
	r := tracequery.Result{WindowStats: &tracequery.WindowStats{StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
		SourcePath: "source.ftrace", Layer: "block", Event: "block_rq", Thread: tracequery.ThreadRef{PID: 40, Comm: "reader"}, Summary: "legacy storage summary",
	}}}}
	for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
		var lines []string
		renderResultDetailWithPolicy(&r, func(s string) { lines = append(lines, s) }, policy)
		const want = "明细 window_stats:\n- window_stats.storage_latency_by_layer[0]: legacy storage summary; thread=reader-40"
		if got := strings.Join(lines, "\n"); got != want {
			t.Fatalf("nil distribution changed legacy summary bytes:\ngot=%q\nwant=%q", got, want)
		}
	}
}
