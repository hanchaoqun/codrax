package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC243PublicSamplingMeasurement(t *testing.T) {
	bundle, direct := samplingSourceBundleHMC241(t)
	for _, path := range []string{bundle, direct} {
		result, _ := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
		contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
		var group *types.RuntimeMeasurementTable
		for _, table := range contract.Choices() {
			if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
				copy := table
				group = &copy
			}
		}
		if group == nil {
			t.Fatal("successful native filtered sampling query has no selectable same-source sampling facts")
		}
		if len(group.Rows) != 3 {
			t.Fatalf("independent populations must remain three rows: %+v", group.Rows)
		}
	}
}

func TestHMC243SamplingImplicitEndpointAndScope(t *testing.T) {
	_, path := samplingSourceBundleHMC241(t)
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("selected samples")}
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "pattern": "dso=/app/A.so"})
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	payload, _, _ := samplingRepairPayloadHMC235(t, result)
	if payload.PerfStats == nil || len(payload.PerfStats.Cohorts) == 0 || !payload.PerfStats.Cohorts[0].Scope.Window.EndInclusive {
		t.Fatal("fixture did not exercise inclusive native capture end")
	}
	for _, mode := range []string{"implicit", "narrow_half_open", "broader_half_open"} {
		input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}}
		if mode != "implicit" {
			start, end := 1.0, 3.0
			if mode == "broader_half_open" {
				end = 4
			}
			input.RequestModel = &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "selected reporting window", TimeStart: &start, TimeEnd: &end}}
		}
		found := false
		for _, table := range types.BuildRuntimeMeasurementContract(input).Choices() {
			if !table.HasNativeSamplingFacts() || table.View != types.RuntimeMeasurementSummary {
				continue
			}
			if mode == "narrow_half_open" {
				t.Fatal("inclusive aggregate crossed the requested half-open end")
			}
			if !strings.Contains(strings.Join(table.Notes, " "), "[1.100000, 3.000000]") {
				t.Fatalf("native closed endpoint was relabeled: %v", table.Notes)
			}
			for _, row := range table.Rows {
				if row[0] == "cycles" && row[1] == "原始 perf 数据" {
					found = row[4] == "3" && row[5] == "915" && row[6] == "3" && row[7] == "915"
				}
			}
		}
		if mode != "narrow_half_open" && !found {
			t.Fatalf("%s changed inclusive population instead of reporting it", mode)
		}
	}
}

func TestHMC243SamplingIndependentQualityAndAvailability(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	for _, filter := range []string{"", "symbol=work", "dso=/app/A.so"} {
		r, _ := samplingIdentityQueryHMC239(t, bundle, "perf_stats", map[string]any{"pid": 11, "pattern": filter})
		contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
		populations := map[string][]string{}
		for _, table := range contract.Choices() {
			if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
				for _, row := range table.Rows {
					if row[9] == "" || populations[row[9]] != nil {
						t.Fatal("cohort labels are missing or ambiguous")
					}
					populations[row[9]] = row
				}
			}
		}
		found := false
		for _, table := range contract.Choices() {
			if !strings.Contains(table.Label, "/app/A.so") {
				continue
			}
			if table.View == types.RuntimeMeasurementSummary {
				found = true
				for _, row := range table.Rows {
					if row[0] == "cycles" && row[1] == "原始 perf 数据" {
						wantCount, wantWeight, wantFraction := "3", "35", "15/35"
						if filter == "dso=/app/A.so" {
							wantCount, wantWeight, wantFraction = "2", "15", "15/15"
						}
						if row[4] != "2" || row[5] != "15" || row[6] != wantCount || row[7] != wantWeight || row[8] != wantFraction {
							t.Fatalf("filter=%s changed independent population: %v", filter, row)
						}
					}
				}
			}
			if table.View == types.RuntimeMeasurementDistribution && filter != "dso=/app/A.so" {
				unknown, exact, unavailable := false, false, false
				for _, row := range table.Rows {
					summary := populations[row[9]]
					if summary == nil || summary[0] != row[0] || summary[1] != row[1] || summary[2] != row[10] || summary[3] != row[2] || summary[6] != row[7] || summary[7] != row[8] || summary[10] != row[11] {
						t.Fatalf("quality row lost its exact population identity: %v", row)
					}
					if row[0] != "cycles" || row[1] != "原始 perf 数据" {
						continue
					}
					unknown = unknown || row[3] == "样本类型" && row[4] == "未知" && row[5] == "3" && row[6] == "35"
					exact = exact || row[3] == "帧边界" && row[4] == "精确帧边界" && row[5] == "2" && row[6] == "30"
					unavailable = unavailable || row[3] == "帧边界" && row[4] == "帧边界不可用" && row[5] == "1" && row[6] == "5"
				}
				if !unknown || !exact || !unavailable {
					t.Fatalf("query quality confused sample-kind/frames/subset: %+v", table.Rows)
				}
			}
		}
		if !found {
			t.Fatal("native facts missing")
		}
	}
	for _, mode := range []string{"unknown", "overflow", "topn"} {
		var body strings.Builder
		for i := 0; i < 12; i++ {
			known, weight, symbol := "true", "1", fmt.Sprintf("work%d", i)
			if mode == "unknown" {
				known = "false"
			}
			if mode == "overflow" {
				weight = "9223372036854775807"
				symbol = "work"
			}
			fmt.Fprintf(&body, "worker-11 (10) [000] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=%s sample_weight=%s event=cycles symbol=%s dso=/app/A.so source=raw\n", 100+i, known, weight, symbol)
		}
		path := filepath.Join(t.TempDir(), "samples.perftrace")
		if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
			t.Fatal(err)
		}
		r, _ := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "pattern": "dso=/app/A.so"})
		choices := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}}).Choices()
		if mode != "topn" && len(choices) > 0 {
			t.Fatalf("%s acquired unavailable population facts", mode)
		}
		if mode == "topn" {
			count := 0
			for _, table := range choices {
				if table.View == types.RuntimeMeasurementSummary {
					count++
					if table.Rows[0][6] != "12" || table.Rows[0][7] != "12" || !strings.Contains(strings.Join(table.Notes, " "), "展示截断") {
						t.Fatalf("topn silently changed denominator: %+v", table)
					}
				}
			}
			if count == 0 || count >= 12 {
				t.Fatalf("topn not exercised: %d", count)
			}
		}
	}
}

func TestHMC243NativeFactDiagramPersistBoundary(t *testing.T) {
	ctx, event := eventDisplayPublicContext236(t)
	block := eventBlock236("facts", "diagram", event.ID, map[string]any{"field_keys": []string{"jank.appid"}})
	input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Observed event facts."}, block}}
	if out := b1659bExecuteAnswer(t, ctx, input, false); !out.Success {
		t.Fatalf("native event diagram persist: %s", out.Summary)
	}
	bound := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
	if !runtimeMeasurementBoundFactSurface(bound) {
		t.Fatal("diagram did not bind native authority")
	}
	for _, mode := range []string{"stale", "json", "ordinary"} {
		b := bound
		b.RuntimeMeasurement = bound.RuntimeMeasurement.Clone()
		switch mode {
		case "stale":
			b.RuntimeMeasurement.ObservationID += ".other"
		case "json":
			raw, _ := json.Marshal(b)
			b = types.AnswerBlock{}
			_ = json.Unmarshal(raw, &b)
		case "ordinary":
			table := runtimeMeasurementTestView().RuntimeMeasurementContract.Choices()[0]
			b.RuntimeMeasurement = &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View, BoundTable: &table}
		}
		if len(collectMergedV2DocBlockViolations(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{b}})) == 0 {
			t.Fatalf("%s bypassed missing diagram without native fact authority", mode)
		}
	}
	if bound.Diagram != nil || len(bound.EdgeAnchors) != 0 || len(bound.RelationClaims) != 0 {
		t.Fatal("fact diagram minted relation evidence")
	}
}

func TestHMC243SamplingMixedSchema(t *testing.T) {
	_, view, sampling := samplingMeasurementContextHMC243(t)
	eventCtx, event := eventDisplayPublicContext236(t)
	events := types.BuildAnswerSemanticViewForBusContext(eventCtx).RuntimeMeasurementContract.Choices()
	legacy := runtimeMeasurementTestView().RuntimeMeasurementContract.Choices()
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, events...)
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, legacy...)
	view.RequiredBlocks = []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}}
	view.OptionalBlocks = []types.BlockRequirement{{Kind: types.BlockTable}, {Kind: types.BlockDiagram}}
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	for _, patch := range []bool{false, true} {
		schema, key := BuildAnswerDocumentParametersFor(view), "blocks"
		if patch {
			schema, key = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
		}
		encoded, _ := json.Marshal(schema)
		for _, teaching := range []string{"event_selection chooses row_ids", "Published native sampling facts"} {
			if !strings.Contains(string(encoded), teaching) {
				t.Fatalf("mixed teaching lost %q", teaching)
			}
		}
		for _, family := range []string{"event", "sampling", "legacy"} {
			for _, kind := range []string{"summary", "table", "diagram"} {
				pair := map[string]any{"observation_id": sampling.ObservationID, "view": sampling.View}
				if family == "event" {
					pair = map[string]any{"observation_id": event.ID, "view": "members", "event_selection": map[string]any{"field_keys": []string{"jank.appid"}}}
				}
				if family == "legacy" {
					pair = map[string]any{"observation_id": legacy[0].ObservationID, "view": legacy[0].View}
				}
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": pair}
				raw, _ := json.Marshal(map[string]any{key: []any{block}})
				want := family != "legacy" || kind == "table"
				if err := toolparam.Validate(raw, schema); (err == nil) != want {
					t.Fatalf("family=%s kind=%s patch=%t schema=%v", family, kind, patch, err)
				}
				b, _ := json.Marshal(block)
				var wire emitAnswerBlockV2
				_ = json.Unmarshal(b, &wire)
				normal, err := NormalizeEmitAnswerBlock(wire, "blocks[0]", view.RuntimeMeasurementContract)
				if (err == nil && runtimeMeasurementBindingValid(normal, view)) != want {
					t.Fatalf("family=%s kind=%s binding differs: %v", family, kind, err)
				}
				if !want {
					continue
				}
				block["text"] = "copied measurements"
				raw, _ = json.Marshal(map[string]any{key: []any{block}})
				if toolparam.Validate(raw, schema) == nil {
					t.Fatal("competing copied text accepted")
				}
			}
		}
	}
}

func samplingMeasurementContextHMC243(t *testing.T) (*types.BusContext, *types.AnswerSemanticView, types.RuntimeMeasurementTable) {
	t.Helper()
	bundle, _ := samplingSourceBundleHMC241(t)
	c := samplingFactsCaseInHMC243(t, bundle, 1, 2)
	return c.ctx, c.view, c.table
}

const (
	samplingFixtureHMC243 = "../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace"
	// samplingPathNoiseMarkerHMC243 names the directory of the path-noise
	// control; it needs no escaping on any surface, so its presence is exact.
	samplingPathNoiseMarkerHMC243 = "run-900"
)

// samplingFactsCaseHMC243 is one public sampling query with everything the
// structural oracle needs: the tool result owns the producer record behind the
// bound table.
type samplingFactsCaseHMC243 struct {
	ctx    *types.BusContext
	view   *types.AnswerSemanticView
	table  types.RuntimeMeasurementTable
	result types.ToolResult
}

func samplingFactsCaseInHMC243(t *testing.T, bundle string, start, end float64) samplingFactsCaseHMC243 {
	t.Helper()
	dir := t.TempDir()
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling facts"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": bundle, "view": "perf_stats", "pid": 11, "time_start": start, "time_end": end, "pattern": "symbol=work"})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
			return samplingFactsCaseHMC243{ctx: ctx, view: view, table: table, result: r}
		}
	}
	t.Fatal("missing native group")
	return samplingFactsCaseHMC243{}
}

// samplingRawPopulationHMC243 is one (event, capture source) population
// recomputed from the raw fixture lines by a parser that shares no code with
// the product. The expectation for "which samples lie inside [start,end)" is
// the capture itself, never a literal searched on a page.
type samplingRawPopulationHMC243 struct {
	hitN, hitW, allN, allW int64
	hitLo, hitHi           float64
	allLo, allHi           float64
}

var samplingRawLineHMC243 = regexp.MustCompile(`\s(\d+\.\d+): perf_sample: .*\btid=(\d+) .*\bsample_weight=(\d+) event=(\S+) symbol=(\S+) dso=(\S+) .*\bsource=(\S+)`)

// samplingRawOracleHMC243 returns the in-window populations and, separately,
// the weights of the same thread's records that lie outside the window.
func samplingRawOracleHMC243(t *testing.T, tid int, symbol, dso string, start, end float64) (map[[2]string]samplingRawPopulationHMC243, map[string]bool) {
	t.Helper()
	f, err := os.Open(samplingFixtureHMC243)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[[2]string]samplingRawPopulationHMC243{}
	outside := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		m := samplingRawLineHMC243.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		ts, _ := strconv.ParseFloat(m[1], 64)
		gotTID, _ := strconv.Atoi(m[2])
		weight, _ := strconv.ParseInt(m[3], 10, 64)
		if gotTID != tid || m[5] != symbol {
			continue
		}
		if ts < start || ts >= end {
			outside[m[3]] = true
			continue
		}
		key := [2]string{m[4], m[7]}
		p, seen := out[key]
		if !seen || ts < p.allLo {
			p.allLo = ts
		}
		if !seen || ts > p.allHi {
			p.allHi = ts
		}
		p.allN, p.allW = p.allN+1, p.allW+weight
		if m[6] == dso {
			if p.hitN == 0 || ts < p.hitLo {
				p.hitLo = ts
			}
			if p.hitN == 0 || ts > p.hitHi {
				p.hitHi = ts
			}
			p.hitN, p.hitW = p.hitN+1, p.hitW+weight
		}
		out[key] = p
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("raw oracle found no in-window sample; fixture or parser drifted")
	}
	return out, outside
}

var samplingSourceLabelHMC243 = map[string]string{"raw_perfdata_fallback": "原始 perf 数据", "simpleperf_report_sample": "simpleperf 样本报告"}

var (
	samplingEnvelopeHMC243    = regexp.MustCompile(`^\[(\d+\.\d{6}), (\d+\.\d{6})\]$`)
	samplingDiagramNodeHMC243 = regexp.MustCompile(`^N\d+\["`)
	samplingDigitsHMC243      = regexp.MustCompile(`[0-9]+`)
)

// samplingFactCarriersHMC243 parses the rendered page into the lines that
// carry facts of the requested kind: summary bullets, table body rows or
// diagram nodes, plus the table header and every Mermaid body. Assertions read
// these parsed parts; the page as a whole also holds the material path, so it
// is never searched for a bare value.
func samplingFactCarriersHMC243(visible string, kind types.AnswerBlockKind) (header string, carriers, bodies []string, closed bool) {
	closed = true
	for _, part := range strings.Split(visible, "```mermaid\n")[1:] {
		body, _, ok := strings.Cut(part, "```")
		closed = closed && ok
		bodies = append(bodies, body)
	}
	for _, line := range strings.Split(visible, "\n") {
		trimmed := strings.TrimSpace(line)
		switch kind {
		case types.BlockSummary:
			if strings.HasPrefix(trimmed, "- 采样事件: ") {
				carriers = append(carriers, trimmed)
			}
		case types.BlockTable:
			switch {
			case strings.HasPrefix(trimmed, "| 采样事件 "):
				header = trimmed
			case strings.HasPrefix(trimmed, "| ") && !strings.HasPrefix(trimmed, "| --- "):
				carriers = append(carriers, trimmed)
			}
		case types.BlockDiagram:
			if samplingDiagramNodeHMC243.MatchString(trimmed) {
				carriers = append(carriers, trimmed)
			}
		}
	}
	return header, carriers, bodies, closed
}

// samplingFactsViolationsHMC243 is the whole acceptance oracle. It returns the
// violations instead of failing so the same function is exercised by the
// positive cases and by the negative controls.
func samplingFactsViolationsHMC243(t *testing.T, c samplingFactsCaseHMC243, doc *types.AnswerDocumentV2, kind types.AnswerBlockKind, start, end float64) []string {
	t.Helper()
	var bad []string
	fail := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }
	facts := blockByID(t, doc, "facts")
	if facts.Kind != kind || !facts.RuntimeMeasurement.IsBound() {
		return []string{"facts block is not a bound native measurement of the requested kind"}
	}
	bound := facts.RuntimeMeasurement.BoundTable

	// (1) Relation evidence: typed fields, never a page scan.
	if facts.Diagram != nil || len(facts.EdgeAnchors) != 0 || len(facts.RelationClaims) != 0 {
		fail("fact block minted relation evidence: diagram=%t edge_anchors=%d relation_claims=%d", facts.Diagram != nil, len(facts.EdgeAnchors), len(facts.RelationClaims))
	}

	// (2) Source window: the producer record behind the bound table.
	var source *types.ObservationRecord
	for i := range c.result.Observations {
		if c.result.Observations[i].ID == bound.ObservationID {
			source = &c.result.Observations[i]
		}
	}
	if source == nil || !source.SourceRef.QueryWindowKnown || source.SourceRef.QueryWindowStartTs != start || source.SourceRef.QueryWindowEndTs != end {
		fail("bound table is not backed by a producer record for query window [%v,%v)", start, end)
	}
	windowNote := fmt.Sprintf("查询窗口 [%.6f, %.6f) 秒", start, end)
	windowNotes := 0
	for _, note := range bound.Notes {
		if strings.HasPrefix(note, "查询窗口 ") {
			windowNotes++
			if !strings.HasPrefix(note, windowNote+"；") {
				fail("bound window note %q does not state %q", note, windowNote)
			}
		}
	}
	if windowNotes != 1 {
		fail("bound table carries %d query-window notes, want exactly 1", windowNotes)
	}
	oracle, outside := samplingRawOracleHMC243(t, 11, "work", "/app/A.so", start, end)
	// The former page search also covered the producer notes. They are read
	// as typed strings; the one note that names the material paths is
	// identified by its own prefix and left out.
	materialNotes := 0
	for _, note := range bound.Notes {
		if strings.HasPrefix(note, "查询材料：") {
			materialNotes++
			continue
		}
		for _, number := range samplingDigitsHMC243.FindAllString(note, -1) {
			if outside[number] {
				fail("a note states the weight %s of a record outside the query window: %q", number, note)
			}
		}
	}
	if materialNotes != 1 {
		fail("bound table carries %d material notes, want exactly 1", materialNotes)
	}

	// (3) Bound cells by column name against the raw capture.
	col := map[string]int{}
	for i, name := range bound.Columns {
		col[name] = i
	}
	for _, name := range []string{"采样事件", "采集方式", "样本类型", "命中记录数", "命中权重", "查询总体记录数", "查询总体权重", "权重占比", "记录占比", "热点记录时刻包络（秒）", "查询总体记录时刻包络（秒）"} {
		if _, ok := col[name]; !ok {
			return append(bad, "bound table lost column "+name)
		}
	}
	if len(bound.Rows) != len(oracle) {
		fail("bound rows=%d, raw in-window populations=%d", len(bound.Rows), len(oracle))
	}
	seen := map[[2]string]bool{}
	for _, row := range bound.Rows {
		cell := func(name string) string { return row[col[name]] }
		var key [2]string
		for raw, label := range samplingSourceLabelHMC243 {
			if label == cell("采集方式") {
				key = [2]string{cell("采样事件"), raw}
			}
		}
		want, ok := oracle[key]
		if !ok || seen[key] {
			fail("row %q/%q is not exactly one raw in-window population", cell("采样事件"), cell("采集方式"))
			continue
		}
		seen[key] = true
		for _, expect := range [][2]string{
			{"命中记录数", strconv.FormatInt(want.hitN, 10)},
			{"命中权重", strconv.FormatInt(want.hitW, 10)},
			{"查询总体记录数", strconv.FormatInt(want.allN, 10)},
			{"查询总体权重", strconv.FormatInt(want.allW, 10)},
			{"权重占比", fmt.Sprintf("%d/%d", want.hitW, want.allW)},
			{"记录占比", fmt.Sprintf("%d/%d", want.hitN, want.allN)},
			{"热点记录时刻包络（秒）", fmt.Sprintf("[%.6f, %.6f]", want.hitLo, want.hitHi)},
			{"查询总体记录时刻包络（秒）", fmt.Sprintf("[%.6f, %.6f]", want.allLo, want.allHi)},
			{"样本类型", "未知"},
		} {
			if cell(expect[0]) != expect[1] {
				fail("%s/%s %s=%q, raw capture in [%v,%v) gives %q", key[0], key[1], expect[0], cell(expect[0]), start, end, expect[1])
			}
		}
		// (4) Independent of the oracle arithmetic: every published record
		// instant lies inside the half-open query window.
		for _, name := range []string{"热点记录时刻包络（秒）", "查询总体记录时刻包络（秒）"} {
			m := samplingEnvelopeHMC243.FindStringSubmatch(cell(name))
			if m == nil {
				fail("%s is not a closed record envelope: %q", name, cell(name))
				continue
			}
			lo, _ := strconv.ParseFloat(m[1], 64)
			hi, _ := strconv.ParseFloat(m[2], 64)
			if lo < start || hi >= end || hi < lo {
				fail("%s %q leaves the query window [%v,%v)", name, cell(name), start, end)
			}
		}
	}

	// (5) Rendered form: every bound row is carried by exactly one fact
	// carrier of this kind, in order, and no Mermaid body declares an edge.
	header, carriers, bodies, closed := samplingFactCarriersHMC243(html.UnescapeString(render.RenderAnswerDocument(doc, "zh")), kind)
	if !closed {
		fail("unterminated Mermaid fence")
	}
	wantBodies := 0
	if kind == types.BlockDiagram {
		wantBodies = 1
	}
	if len(bodies) != wantBodies {
		fail("rendered %d Mermaid bodies for kind=%s, want %d", len(bodies), kind, wantBodies)
	}
	for _, body := range bodies {
		if edges := mermaidcompat.ParseEdges(body); len(edges) != 0 {
			fail("fact diagram declares %d edge(s): %+v", len(edges), edges)
		}
	}
	if kind == types.BlockTable {
		delimited := strings.ReplaceAll(header, " | ", " | | ")
		for _, name := range bound.Columns {
			if !strings.Contains(delimited, "| "+name+" |") {
				fail("rendered table header lost column %q", name)
			}
		}
	}
	if len(carriers) != len(bound.Rows) {
		fail("rendered %d fact carriers for %d bound rows", len(carriers), len(bound.Rows))
	}
	for i, row := range bound.Rows {
		if i >= len(carriers) {
			break
		}
		probe := carriers[i]
		if kind == types.BlockTable {
			probe = strings.ReplaceAll(probe, " | ", " | | ") // every cell individually delimited
		}
		for j, value := range row {
			want := bound.Columns[j] + ": " + value
			if kind == types.BlockTable {
				want = "| " + value + " |"
			}
			if !strings.Contains(probe, want) {
				fail("rendered carrier %d lost bound cell %s=%q", i+1, bound.Columns[j], value)
			}
		}
	}
	return bad
}

// samplingFactsEmitHMC243 publishes the selected native table through the
// public answer tool and returns the request block with the saved document.
func samplingFactsEmitHMC243(t *testing.T, c samplingFactsCaseHMC243, kind types.AnswerBlockKind) (map[string]any, *types.AnswerDocumentV2) {
	t.Helper()
	selector := map[string]any{"observation_id": c.table.ObservationID, "view": "summary"}
	block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": selector}
	blocks := []any{block}
	if kind != types.BlockSummary {
		blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Independent sampling populations are shown below."}}, blocks...)
	}
	out := b1659bExecuteAnswer(t, c.ctx, map[string]any{"blocks": blocks}, false)
	if !out.Success {
		t.Fatalf("public emit: %s", out.Summary)
	}
	return block, c.ctx.Mutable.AnswerDocumentV2()
}

// HMC376 re-base: the former guard searched the whole reader page for the
// arrow token and for the outside-window weight. The page legitimately prints
// the material path, so that guard failed whenever the temporary directory
// spelled either token, and it could not see an aggregated outside-window
// population. The same two claims (no relation evidence, no outside-window
// value) are now read from typed fields, bound cells and parsed fact carriers;
// the path-noise case and the outside-window control below prove both
// directions.
func TestHMC243SamplingFactsPublicEmitPatchRecovery(t *testing.T) {
	const start, end = 1.0, 2.0
	for _, tc := range []struct{ name, sub string }{
		{"default_path", ""},
		// Positive control for the retired page scan: the material path itself
		// spells the outside-window weight and an arrow.
		{"path_noise", samplingPathNoiseMarkerHMC243 + " --> 900"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.sub != "" {
				dir = filepath.Join(dir, tc.sub)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			bundle, _ := samplingSourceBundleInHMC241(t, dir)
			c := samplingFactsCaseInHMC243(t, bundle, start, end)
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				t.Run(string(kind), func(t *testing.T) {
					block, doc := samplingFactsEmitHMC243(t, c, kind)
					for _, v := range samplingFactsViolationsHMC243(t, c, doc, kind, start, end) {
						t.Error(v)
					}
					visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
					// The reader surface keeps the same values as before, now
					// read from the fact carriers instead of the whole page.
					header, carriers, _, _ := samplingFactCarriersHMC243(visible, kind)
					surface := header + "\n" + strings.Join(carriers, "\n")
					for _, value := range []string{"15/35", "7/7", "100/100", "查询总体记录数", "未知"} {
						if !strings.Contains(surface, value) {
							t.Errorf("surface lost %q: %s", value, surface)
						}
					}
					if tc.sub != "" {
						if !strings.Contains(visible, "900") || !strings.Contains(visible, "-->") {
							t.Fatal("path noise no longer reaches the reader page; this control is vacuous")
						}
						if strings.Contains(surface, "900") || strings.Contains(surface, "-->") {
							t.Fatal("path noise reached a fact carrier")
						}
					}
					// Typed relation tamper: the oracle must see what the
					// retired arrow scan claimed to see.
					tampered := *doc
					tampered.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
					for i := range tampered.Blocks {
						if tampered.Blocks[i].ID == "facts" {
							tampered.Blocks[i].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "N1", ToNode: "N2"}}
						}
					}
					if len(samplingFactsViolationsHMC243(t, c, &tampered, kind, start, end)) == 0 {
						t.Fatal("oracle accepted a fact block carrying an edge anchor")
					}
					wire, _ := json.Marshal(doc)
					var restored types.AnswerDocumentV2
					if json.Unmarshal(wire, &restored) != nil || !types.RebindRuntimeAnswerReceipts(&restored, c.view) {
						t.Fatal("current-answer rebind failed")
					}
					if strings.Contains(string(wire), "15/35") {
						t.Fatal("producer values serialized as model answer input")
					}
					// The saved answer wire carries the selector only, so the
					// scan above reads no environment text: the material
					// directory never reaches it.
					if tc.sub != "" && strings.Contains(string(wire), samplingPathNoiseMarkerHMC243) {
						t.Fatal("saved answer wire now embeds the material path; read the typed receipt instead of scanning it")
					}
					recovered := textRecoveryWithBadSibling(t, block)
					if b := blockByID(t, recovered.Document, "facts"); b.Kind != kind || b.RuntimeMeasurement.IsBound() {
						t.Fatal("recovery changed shape or minted authority")
					}
					if !types.RebindRuntimeAnswerReceipts(recovered.Document, c.view) {
						t.Fatal("recovery failed current bind")
					}
					patched := b1659bExecuteAnswer(t, c.ctx, map[string]any{"replace_blocks": []any{block}}, true)
					if !patched.Success {
						t.Fatalf("public patch: %s", patched.Summary)
					}
				})
			}
		})
	}
}

// Strong negative control: a document whose native population really crosses
// the requested end (the fixture's weight-900 record at t=3.000000) must be
// rejected by the same oracle, and accepted once the oracle is told the wider
// window. An oracle that rejects everything proves nothing.
func TestHMC243SamplingFactsOracleRejectsOutsideWindowPopulation(t *testing.T) {
	const start, requestedEnd, widenedEnd = 1.0, 2.0, 4.0
	bundle, _ := samplingSourceBundleHMC241(t)
	c := samplingFactsCaseInHMC243(t, bundle, start, widenedEnd)
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			_, doc := samplingFactsEmitHMC243(t, c, kind)
			// The outside-window record is aggregated (915/935): no bound cell
			// spells the weight the retired page scan looked for, which is why
			// that scan could never see this population. Cells, not the page.
			for _, row := range blockByID(t, doc, "facts").RuntimeMeasurement.BoundTable.Rows {
				for _, cell := range row {
					if strings.Contains(cell, "900") {
						t.Fatalf("fixture changed: a bound cell now spells the outside-window weight (%q); re-derive this control", cell)
					}
				}
			}
			got := strings.Join(samplingFactsViolationsHMC243(t, c, doc, kind, start, requestedEnd), "\n")
			for _, want := range []string{`命中权重="915"`, `查询总体权重="935"`, "leaves the query window", "producer record for query window", "does not state"} {
				if !strings.Contains(got, want) {
					t.Errorf("oracle missed %s on a real outside-window population:\n%s", want, got)
				}
			}
			if bad := samplingFactsViolationsHMC243(t, c, doc, kind, start, widenedEnd); len(bad) != 0 {
				t.Fatalf("oracle rejects a correct wider population, so it proves nothing: %v", bad)
			}
		})
	}
	if edges := mermaidcompat.ParseEdges("flowchart TB\n    N1[\"a\"]\n    N1 --> N2\n"); len(edges) != 1 {
		t.Fatalf("edge parser used by the oracle is blind: %+v", edges)
	}
}

func TestHMC243SamplingPublicationTampering(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	result, _ := samplingIdentityQueryHMC239(t, bundle, "perf_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == types.RuntimeSamplingMeasurementPredicate {
			original = r
			break
		}
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(original); !ok {
		t.Fatal("native baseline missing")
	}
	for _, mode := range []string{"json", "source", "window", "query", "clock", "target", "subject", "role", "value", "filter", "duplicate", "unknown", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			r := original
			r.RichNotes = append([]string(nil), r.RichNotes...)
			switch mode {
			case "json":
				b, _ := json.Marshal(r)
				_ = json.Unmarshal(b, &r) // Explicitly use a fresh zero record below.
				var clean types.ObservationRecord
				_ = json.Unmarshal(b, &clean)
				r = clean
			case "source":
				r.SourceRef.Path += ".other"
			case "window":
				r.SourceRef.QueryWindowEndTs++
			case "query":
				r.SourceRef.QueryScopeID += ".other"
			case "clock":
				r.SourceRef.TimeDomain = "other"
			case "target":
				r.SourceRef.QueryTargetScope = "process"
			case "subject":
				r.Subject = "other-12"
			case "role":
				r.Role = types.AnswerAggregateRolePrincipalAnswer
			case "value":
				r.Value = "900"
			default:
				for i, n := range r.RichNotes {
					if !strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
						continue
					}
					switch mode {
					case "duplicate":
						r.RichNotes = append(r.RichNotes, n)
					case "unknown":
						r.RichNotes[i] = strings.Replace(n, "={", "={\"forged\":true,", 1)
					case "truncated":
						r.RichNotes[i] = n[:len(n)-1]
					case "filter":
						r.RichNotes[i] = strings.ReplaceAll(n, "symbol=work", "symbol=other")
					}
				}
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("mutated/JSON-only receipt acquired native fact capability")
			}
		})
	}
	pub, _ := types.DecodeRuntimeMeasurementPublication(original)
	for _, table := range pub.Tables {
		if table.HasCompleteNativeDisplayPopulation() || table.MemberSet != nil || !table.HasNativeSamplingFacts() {
			t.Fatal("wrong display authority")
		}
	}
	contract := &types.RuntimeMeasurementContract{Tables: pub.Tables}
	ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: original.ID, View: types.RuntimeMeasurementSummary}
	if !types.BindRuntimeMeasurementReceipt(ref, contract) || !ref.IsBound() {
		t.Fatal("native bind")
	}
	clone := ref.Clone()
	clone.BoundTable.Rows[0][0] = "changed"
	if clone.IsBound() || !ref.IsBound() {
		t.Fatal("mutation accepted or clone aliases")
	}
}
