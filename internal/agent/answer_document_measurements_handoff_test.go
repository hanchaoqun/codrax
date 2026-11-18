package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// measurementsFinalizerContextHMC376 prepares the fixture and runs the public
// raw-measurement query for one requested window, anchored in a directory
// chosen by the caller so path text is a test input instead of an accident.
func measurementsFinalizerContextHMC376(t *testing.T, dir, path string, start, end float64) (*types.AgentContext, types.RequestModel) {
	t.Helper()
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒的量测记录", Confidence: 1}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("1到2秒的量测记录"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("default prepare/query: %v / %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx, rm
}

// measurementsPreviewHMC376 is one selectable table as the finalizer
// instruction previews it.
type measurementsPreviewHMC376 struct {
	ObservationID, View string
	Columns             []string   `json:"columns"`
	Rows                [][]string `json:"rows"`
}

var measurementsPreviewLineHMC376 = regexp.MustCompile(`^- observation_id="([^"]+)" view="([^"]+)" `)

// measurementsPromptSectionHMC376 parses the verified-measurement section of
// the finalizer instruction into its teaching lines and its table previews.
// The instruction as a whole also carries source paths, payload references,
// content ids and a source version built from file times, so claims about
// measured records are read from these parsed parts and never by searching
// the instruction for a bare value.
func measurementsPromptSectionHMC376(t *testing.T, prompt string) (teaching string, previews []measurementsPreviewHMC376) {
	t.Helper()
	_, section, found := strings.Cut(prompt, "\n### 已核对的测量表 / Verified measurement tables\n")
	if !found {
		t.Fatal("finalizer instruction lost the verified measurement section")
	}
	var lines []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			break
		}
		m := measurementsPreviewLineHMC376.FindStringSubmatch(line)
		switch {
		case m != nil:
			_, encoded, ok := strings.Cut(line, "; preview=")
			if !ok {
				t.Fatalf("selector line carries no preview: %.120s", line)
			}
			preview := measurementsPreviewHMC376{ObservationID: m[1], View: m[2]}
			if err := json.NewDecoder(strings.NewReader(encoded)).Decode(&preview); err != nil {
				t.Fatalf("preview is not one JSON object: %v", err)
			}
			previews = append(previews, preview)
		case strings.HasPrefix(line, "- native_title_source "), strings.HasPrefix(line, "- native_measurement_note "), strings.HasPrefix(line, "- `"), strings.HasPrefix(line, "  "):
			// Source titles, producer notes, ledger rows and reference lists
			// are data lines; they may name paths.
		default:
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), previews
}

// measurementsLeadingIntegerHMC376 reads the integer of a typed raw cell such
// as `900000000 (integer)` or of a plain nanosecond cell.
func measurementsLeadingIntegerHMC376(cell string) (int64, bool) {
	text, _, _ := strings.Cut(cell, " (")
	v, err := strconv.ParseInt(text, 10, 64)
	return v, err == nil
}

// measurementsRowBoundaryViolationsHMC376 is the right-boundary oracle for one
// table of measured records, typed or previewed: every row is observed inside
// the half-open query window and no source record starts at or after its
// right boundary. checked reports whether the table carries measured records
// at all.
func measurementsRowBoundaryViolationsHMC376(where string, columns []string, rows [][]string, startNs, endNs int64) (bad []string, checked bool) {
	fail := func(format string, args ...any) { bad = append(bad, where+": "+fmt.Sprintf(format, args...)) }
	for i, name := range columns {
		switch name {
		case "窗口内起点(ns)":
			checked = true
			for n, row := range rows {
				if v, ok := measurementsLeadingIntegerHMC376(row[i]); !ok {
					fail("row %d in-window start is not an integer instant: %q", n+1, row[i])
				} else if v < startNs || v >= endNs {
					fail("row %d is observed at %d ns, outside the half-open query window [%d,%d)", n+1, v, startNs, endNs)
				}
			}
		case "原起点(ns)":
			checked = true
			for n, row := range rows {
				if v, ok := measurementsLeadingIntegerHMC376(row[i]); !ok {
					fail("row %d source start is not an integer instant: %q", n+1, row[i])
				} else if v >= endNs {
					fail("row %d starts at %d ns, at or after the right boundary %d", n+1, v, endNs)
				}
			}
		case "原起点(ns)→原值（单位未知）":
			checked = true
			for n, row := range rows {
				for _, entry := range strings.Split(row[i], "; ") {
					instant, _, ok := strings.Cut(entry, "→")
					if v, isInt := measurementsLeadingIntegerHMC376(instant); !ok || !isInt {
						fail("row %d entry %q is not a start→value pair", n+1, entry)
					} else if v >= endNs {
						fail("row %d lists a record starting at %d ns, at or after the right boundary %d", n+1, v, endNs)
					}
				}
			}
		}
	}
	return bad, checked
}

// measurementsFinalizerBoundaryViolationsHMC376 applies the right-boundary
// oracle to the complete native tables and to every preview the finalizer
// instruction shows, and requires each previewed row to be a row of its native
// table. It returns the violations so the same function serves the positive
// cases and the widened-window control.
func measurementsFinalizerBoundaryViolationsHMC376(t *testing.T, tables []types.RuntimeMeasurementTable, prompt string, startNs, endNs int64) []string {
	t.Helper()
	var bad []string
	native := 0
	for _, table := range tables {
		v, checked := measurementsRowBoundaryViolationsHMC376("native "+string(table.View), table.Columns, table.Rows, startNs, endNs)
		bad = append(bad, v...)
		if checked {
			native++
		}
	}
	_, previews := measurementsPromptSectionHMC376(t, prompt)
	previewed := 0
	for _, preview := range previews {
		v, checked := measurementsRowBoundaryViolationsHMC376("previewed "+preview.View, preview.Columns, preview.Rows, startNs, endNs)
		bad = append(bad, v...)
		if !checked {
			continue
		}
		previewed++
		var source *types.RuntimeMeasurementTable
		for i := range tables {
			if tables[i].ObservationID == preview.ObservationID && string(tables[i].View) == preview.View {
				source = &tables[i]
			}
		}
		if source == nil || !reflect.DeepEqual(source.Columns, preview.Columns) {
			bad = append(bad, "previewed "+preview.View+": no native table with the same selector and columns")
			continue
		}
		for n, row := range preview.Rows {
			known := false
			for _, nativeRow := range source.Rows {
				known = known || reflect.DeepEqual(nativeRow, row)
			}
			if !known {
				bad = append(bad, fmt.Sprintf("previewed %s: row %d is not a row of its native table: %q", preview.View, n+1, row))
			}
		}
	}
	if native != 3 || previewed != 3 {
		bad = append(bad, fmt.Sprintf("oracle read %d native and %d previewed record tables, want 3 and 3", native, previewed))
	}
	return bad
}

// measurementsColumnHMC376 returns one named column of the first table that
// has it, as typed cells.
func measurementsColumnHMC376(t *testing.T, column string, tables ...measurementsPreviewHMC376) []string {
	t.Helper()
	for _, table := range tables {
		for i, name := range table.Columns {
			if name != column {
				continue
			}
			var cells []string
			for _, row := range table.Rows {
				cells = append(cells, row[i])
			}
			return cells
		}
	}
	t.Fatalf("no table carries column %q", column)
	return nil
}

func measurementsNativePreviewsHMC376(tables []types.RuntimeMeasurementTable, view types.RuntimeMeasurementView) []measurementsPreviewHMC376 {
	var out []measurementsPreviewHMC376
	for _, table := range tables {
		if table.View == view {
			out = append(out, measurementsPreviewHMC376{ObservationID: table.ObservationID, View: string(table.View), Columns: table.Columns, Rows: table.Rows})
		}
	}
	return out
}

// measurementsFinalizerHandoffChecksHMC376 holds the reader-facing claims of
// the handoff: the teaching and the three filter names reach the finalizer,
// the complete native tables keep the exact values, and no record on the
// right boundary reaches either surface.
func measurementsFinalizerHandoffChecksHMC376(t *testing.T, choices []types.RuntimeMeasurementTable, prompt string, startNs, endNs int64) {
	t.Helper()
	teaching, previews := measurementsPromptSectionHMC376(t, prompt)
	for _, want := range []string{"runtime_measurement", "system fills"} {
		if !strings.Contains(teaching, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	names := measurementsColumnHMC376(t, "过滤器名称", previews...)
	for _, want := range []string{"gpufreq", "gpu_state", "gpuload"} {
		if !slices.Contains(names, `"`+want+`" (text)`) {
			t.Errorf("actual finalizer lost %q: %q", want, names)
		}
	}
	for _, v := range measurementsFinalizerBoundaryViolationsHMC376(t, choices, prompt, startNs, endNs) {
		t.Errorf("right-boundary record leaked into finalizer: %s", v)
	}
	// Prompt previews are intentionally bounded. The native selectable tables
	// must still retain every row, including series outside the four-row preview.
	values := measurementsColumnHMC376(t, "原始值（单位未知）", measurementsNativePreviewsHMC376(choices, types.RuntimeMeasurementMembers)...)
	for _, want := range []string{"3.342000005e+08 (real)", "9007199254740993 (integer)", "未知（NULL）"} {
		if !slices.Contains(values, want) {
			t.Errorf("complete native handoff lost %q", want)
		}
	}
}

// measurementsPathNoiseHMC376 names the run directory of the path-noise
// control; it spells the raw value of the fixture record that starts exactly
// on the right boundary.
const measurementsPathNoiseHMC376 = "run-999000000"

// Controls for the right-boundary oracle of the measurement handoff. The
// retired guard searched the whole finalizer instruction for the boundary
// record's raw value; the instruction prints the prepared path, so that guard
// depended on the temporary directory.
func TestMeasurementsFinalizerBoundaryOracleControls(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	const startNs, endNs = 1_000_000_000, 2_000_000_000
	t.Run("path_noise", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), measurementsPathNoiseHMC376)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		ctx, _ := measurementsFinalizerContextHMC376(t, dir, path, 1, 2)
		choices, _ := splitSourceMetadataHMC287(t, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), map[string][3]string{path: {"0", "3000000000", "3000000000"}})
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		if !strings.Contains(prompt, measurementsPathNoiseHMC376) {
			t.Fatal("path noise no longer reaches the finalizer instruction; this control is vacuous")
		}
		measurementsFinalizerHandoffChecksHMC376(t, choices, prompt, startNs, endNs)
	})
	// Strong negative control: a handoff that really includes the record
	// starting on the requested right boundary must be rejected on both
	// surfaces, and accepted once the oracle is told the wider window.
	t.Run("widened_window", func(t *testing.T) {
		ctx, _ := measurementsFinalizerContextHMC376(t, t.TempDir(), path, 1, 2.05)
		choices, _ := splitSourceMetadataHMC287(t, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), map[string][3]string{path: {"0", "3000000000", "3000000000"}})
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		values := measurementsColumnHMC376(t, "原始值（单位未知）", measurementsNativePreviewsHMC376(choices, types.RuntimeMeasurementMembers)...)
		if !slices.Contains(values, "999000000 (integer)") {
			t.Fatalf("fixture changed: the widened window no longer includes the right-boundary record; re-derive this control: %q", values)
		}
		got := strings.Join(measurementsFinalizerBoundaryViolationsHMC376(t, choices, prompt, startNs, endNs), "\n")
		for _, want := range []string{
			"native members: row", "native timeline: row", "native summary: row",
			"previewed members: row", "previewed timeline: row", "previewed summary: row",
			"at or after the right boundary 2000000000", "outside the half-open query window",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("oracle missed %q on a real right-boundary record:\n%s", want, got)
			}
		}
		if bad := measurementsFinalizerBoundaryViolationsHMC376(t, choices, prompt, startNs, 2_050_000_000); len(bad) != 0 {
			t.Fatalf("oracle rejects a correct wider window, so it proves nothing: %v", bad)
		}
		// Coverage of the oracle's surface: when the record really is part of
		// the handoff, the only instruction lines that carry its typed value
		// cell are the previews the oracle parses. The needle is the whole
		// typed cell, which a directory or content id does not spell.
		carriers := 0
		for _, line := range strings.Split(prompt, "\n") {
			if !strings.Contains(line, "999000000 (integer)") {
				continue
			}
			carriers++
			if !measurementsPreviewLineHMC376.MatchString(line) {
				t.Errorf("the record reaches the finalizer on a line the oracle does not read: %.200s", line)
			}
		}
		if carriers == 0 {
			t.Fatal("the widened handoff never reached the instruction; this coverage check is vacuous")
		}
	})
}

// Default preparation and the real finalizer share native data. Raw values
// are displayable without becoming a GPU protocol, source obligation or cause.
func TestMeasurementsPublicFinalizerEmitPatchRender(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, rm := measurementsFinalizerContextHMC376(t, t.TempDir(), path, 1, 2)
	allChoices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	choices, metadata := splitSourceMetadataHMC287(t, allChoices, map[string][3]string{path: {"0", "3000000000", "3000000000"}})
	if len(choices) != 3 {
		t.Fatalf("expected raw summary/members/timeline: %+v", choices)
	}
	var defaultMembers *types.RuntimeMeasurementTable
	for i := range choices {
		if choices[i].DefaultPresentation {
			if defaultMembers != nil || choices[i].View != types.RuntimeMeasurementMembers {
				t.Fatal("raw measurements must publish exactly one members default")
			}
			defaultMembers = &choices[i]
		}
	}
	if defaultMembers == nil {
		t.Fatal("raw measurements lost their lossless default view")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// HMC376 re-base: these claims were searched in the whole instruction and
	// in the encoded tables, which also carry the prepared path. They are now
	// read from the teaching lines, typed cells and parsed previews; the
	// path-noise and widened-window controls are in
	// TestMeasurementsFinalizerBoundaryOracleControls.
	measurementsFinalizerHandoffChecksHMC376(t, choices, prompt, 1_000_000_000, 2_000_000_000)
	if t.Failed() {
		t.FailNow()
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projection := func() []byte {
		b, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
		return b
	}
	beforeProjection := projection()
	for _, table := range choices {
		if table.MemberSet != nil {
			t.Fatal("raw display acquired complete-population authority")
		}
		if table.View == types.RuntimeMeasurementMembers || table.View == types.RuntimeMeasurementTimeline {
			if len(table.Rows) != 13 {
				t.Fatalf("native window rows=%d, want 13", len(table.Rows))
			}
		}
		for _, patch := range []bool{false, true} {
			block := map[string]any{"id": "raw", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
			input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "原始量测尚不能直接确认GPU实际运行频段。"}, block}}
			schema := (&tool.EmitAnswerDocument{}).ParametersFor(ctx)
			if patch {
				input = map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead"}}
				schema = (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
			}
			raw, _ := json.Marshal(input)
			if err := toolparam.Validate(raw, schema); err != nil {
				t.Fatalf("native selector schema: %v", err)
			}
			var out types.ToolResult
			if patch {
				out, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
			} else {
				out, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			}
			if err != nil || !out.Success {
				t.Fatalf("emit/patch native table: %v / %s", err, out.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			// The explicit selector keeps its identity and values. Choosing a
			// summary/timeline additionally retains the producer's full members;
			// choosing members itself must not duplicate the same selector.
			seenRaw, seenLead, seenDefault, generated, seenMetadata := 0, 0, 0, 0, 0
			for _, saved := range doc.Blocks {
				switch saved.ID {
				case "lead":
					seenLead++
					if saved.Kind != types.BlockSummary || saved.Text != "原始量测尚不能直接确认GPU实际运行频段。" || saved.SystemGeneratedKind != "" {
						t.Fatal("native display changed the model's summary")
					}
				case "raw":
					seenRaw++
					if saved.RuntimeMeasurement == nil || saved.RuntimeMeasurement.ObservationID != table.ObservationID || saved.RuntimeMeasurement.View != table.View ||
						!reflect.DeepEqual(saved.RuntimeMeasurement.BoundTable, &table) || saved.SystemGeneratedKind != "" {
						t.Fatal("native data or explicit selector changed at emit/patch")
					}
				default:
					if saved.RuntimeMeasurement != nil && saved.RuntimeMeasurement.ObservationID == metadata[0].ObservationID {
						seenMetadata++
						if saved.SystemGeneratedKind != types.AnswerSystemGeneratedNativeFacts || !reflect.DeepEqual(saved.RuntimeMeasurement.BoundTable, &metadata[0]) {
							t.Fatal("automatic source metadata changed its exact native table")
						}
						continue
					}
					generated++
					if saved.SystemGeneratedKind != types.AnswerSystemGeneratedNativeFacts || saved.RuntimeMeasurement == nil ||
						!reflect.DeepEqual(saved.RuntimeMeasurement.BoundTable, defaultMembers) {
						t.Fatal("unexpected or reconstructed automatic native table")
					}
				}
				if saved.RuntimeMeasurement != nil && saved.RuntimeMeasurement.ObservationID == defaultMembers.ObservationID && saved.RuntimeMeasurement.View == defaultMembers.View {
					seenDefault++
				}
			}
			wantGenerated := 1
			if table.View == defaultMembers.View {
				wantGenerated = 0
			}
			if seenRaw != 1 || seenLead != 1 || seenDefault != 1 || seenMetadata != 1 || generated != wantGenerated || len(doc.Blocks) != 3+wantGenerated {
				t.Fatalf("explicit/default selectors lost or duplicated: raw=%d lead=%d default=%d generated=%d metadata=%d", seenRaw, seenLead, seenDefault, generated, seenMetadata)
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, row := range table.Rows {
				if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
					t.Fatalf("row not displayed exactly once: %v", row)
				}
			}
			for _, note := range table.Notes {
				if !strings.Contains(visible, note) {
					t.Fatalf("unknown/scope disclosure lost: %q", note)
				}
			}
			for _, row := range defaultMembers.Rows {
				if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
					t.Fatalf("default member row not displayed exactly once: %v", row)
				}
			}
			for _, row := range metadata[0].Rows {
				if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
					t.Fatalf("source metadata row not displayed exactly once: %v", row)
				}
			}
		}
	}
	if !bytes.Equal(beforeProjection, projection()) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("raw measurement presentation acquired causal authority")
	}
	endNarrow := 1.5
	rm.RuntimeArtifactScopeProfile.TimeEnd = &endNarrow
	ctx.AnalysisIR.RequestModel = rm
	ctx.Mutable.SetRequestModel(rm)
	if c := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract; c != nil {
		remaining, _ := splitSourceMetadataHMC287(t, c.Choices(), map[string][3]string{path: {"0", "3000000000", "3000000000"}})
		if len(remaining) != 0 {
			t.Fatal("wider source data relabelled as narrow request window")
		}
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	bad, _ := json.Marshal(map[string]any{"replace_blocks": []any{map[string]any{"id": "raw", "kind": "table", "runtime_measurement": map[string]any{"observation_id": choices[0].ObservationID, "view": choices[0].View}}}, "unchanged_block_ids": []string{"lead"}})
	for _, clear := range []bool{false, true} {
		if clear {
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			ctx.Mutable.ResetDispatchToolResults()
		}
		out, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, bad)
		if err != nil || out.Success || !reflect.DeepEqual(accepted, ctx.Mutable.AnswerDocumentV2()) {
			t.Fatalf("out-of-scope/missing receipt modified answer: %v / %s", err, out.Summary)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("read-only measurement query modified database")
	}
}
