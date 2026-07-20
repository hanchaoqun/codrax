package tool

// answer_document_projection_p3measure_test.go — P3MEASURE-1 双不可见 face
// A/B (§29.169 hard condition 1, 2026-07-20): the four flagship boards plus
// the 17874 report shape (the donghu_17267 board — the same board the
// 20260719-161405.439-17874.md customer report rendered) replay the FULL
// production face pipeline twice — once from the live measured rank result,
// once from a clone whose four P3M* fields are cleared — and every user face
// and every model face must compare BYTE-IDENTICAL:
//
//	user face  — the rendered answer document (projection tree + ◎ overview
//	             + detail + evidence index), zh AND en;
//	model face — the trace_query tool Summary banner (the rank-row text face
//	             the LLM reads) and every ObservationRecord field EXCEPT
//	             RichNotes; the RichNotes delta must be EXACTLY the p3m_*
//	             display-only keys (the silent wire itself).
//
// TestP3MeasureAdvisoryOnlyConsumerAbsence is the structural closure: the
// p3m_* key literals and the P3M* field identifiers appear in NO consumer
// source anywhere — display_only IS enforced, not just declared (advisory-
// only red line, supply_pressure 分离先例: promotion to any gate/face first
// reddens this pin and the registry carrier pin, which is the review surface
// the future ruling must pass through).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var p3mFaceBoards = []struct {
	name  string
	trace string
	q     tracequery.Query
}{
	{"tieba_flag", "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace",
		tracequery.Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.595184,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}},
	{"tieba_trace", "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace",
		tracequery.Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
			TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace}},
	{"donghu_2955", "../../eval/fixtures/real_traces/donghu.ftrace",
		tracequery.Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}},
	// The 17874 report shape (§29.169 spec: "17874 复放").
	{"donghu_17267_17874shape", "../../eval/fixtures/real_traces/donghu.ftrace",
		tracequery.Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace}},
}

// p3mClearRankMeasurement deep-copies a Result far enough to clear the four
// P3M* fields on every published rank item (the B arm of the face A/B).
func p3mClearRankMeasurement(result tracequery.Result) tracequery.Result {
	if result.RootCauseRank == nil {
		return result
	}
	rank := *result.RootCauseRank
	rank.Items = append([]tracequery.RootCauseRankItem(nil), rank.Items...)
	for i := range rank.Items {
		rank.Items[i].P3MCounterfactualValidMs = 0
		rank.Items[i].P3MCounterfactualInvalidMs = 0
		rank.Items[i].P3MEdgeWitnessedMs = 0
		rank.Items[i].P3MDisposition = ""
	}
	result.RootCauseRank = &rank
	return result
}

func p3mMustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func p3mRenderUserFace(t *testing.T, obs []types.ObservationRecord, lang string) string {
	t.Helper()
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "p3measure A/B."}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1753000000, 0).UTC())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), lang)
}

// TestP3MeasureFlagshipFaceABIdentity — the 双不可见 face proof on the four
// flagship boards + the 17874 shape.
func TestP3MeasureFlagshipFaceABIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace battery")
	}
	at := time.Unix(1753000000, 0).UTC()
	indexes := map[string]*tracequery.Index{}
	for _, board := range p3mFaceBoards {
		idx, ok := indexes[board.trace]
		if !ok {
			if _, err := os.Stat(board.trace); err != nil {
				t.Skipf("real-trace fixture absent: %v", err)
			}
			var err error
			idx, err = tracequery.BuildIndex(context.Background(), board.trace)
			if err != nil {
				t.Fatal(err)
			}
			indexes[board.trace] = idx
		}
		var obsWith, obsCleared []types.ObservationRecord
		for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
			q := board.q
			q.View = view
			result := tracequery.Run(idx, q)
			cleared := p3mClearRankMeasurement(result)

			// ── model face 1: the tool Summary banner ──────────────────────
			p := traceQueryParams{View: view}
			summaryWith := traceQuerySummary(result, p, "flagship.systrace", "blob:p3m")
			summaryCleared := traceQuerySummary(cleared, p, "flagship.systrace", "blob:p3m")
			if summaryWith != summaryCleared {
				t.Fatalf("%s/%s: the tool Summary model face must be byte-identical with the measurement cleared", board.name, view)
			}

			with := traceQueryTypedObservations(result, "flagship.systrace", "p-"+view, "r", "", at)
			without := traceQueryTypedObservations(cleared, "flagship.systrace", "p-"+view, "r", "", at)
			if len(with) != len(without) {
				t.Fatalf("%s/%s: observation census moved: %d vs %d", board.name, view, len(with), len(without))
			}
			// ── model face 2: records equal except the p3m_* silent wire ───
			sawMeasurementNote := false
			for i := range with {
				a, b := with[i], without[i]
				aNotes, bNotes := a.RichNotes, b.RichNotes
				a.RichNotes, b.RichNotes = nil, nil
				aJSON, bJSON := p3mMustJSON(t, a), p3mMustJSON(t, b)
				if aJSON != bJSON {
					t.Fatalf("%s/%s: record %d differs outside RichNotes:\n%s\nvs\n%s", board.name, view, i, aJSON, bJSON)
				}
				bSet := map[string]bool{}
				for _, note := range bNotes {
					bSet[note] = true
				}
				for _, note := range aNotes {
					if bSet[note] {
						continue
					}
					if !strings.HasPrefix(note, "p3m_") {
						t.Fatalf("%s/%s: record %d gained a non-p3m note from the measurement: %q", board.name, view, i, note)
					}
					sawMeasurementNote = true
				}
				aSet := map[string]bool{}
				for _, note := range aNotes {
					aSet[note] = true
				}
				for _, note := range bNotes {
					if !aSet[note] {
						t.Fatalf("%s/%s: record %d LOST note %q when the measurement cleared", board.name, view, i, note)
					}
				}
			}
			if view == "root_cause_rank" && !sawMeasurementNote {
				t.Fatalf("%s: the flagship rank face must carry at least one silent p3m_* note (A must be meaningful)", board.name)
			}
			obsWith = append(obsWith, with...)
			obsCleared = append(obsCleared, without...)
		}
		// ── user face: full rendered answer document, zh + en ──────────────
		for _, lang := range []string{"zh", "en"} {
			mdWith := p3mRenderUserFace(t, obsWith, lang)
			mdCleared := p3mRenderUserFace(t, obsCleared, lang)
			if mdWith != mdCleared {
				t.Fatalf("%s: the rendered %s user face must be byte-identical with the measurement cleared", board.name, lang)
			}
			if strings.Contains(mdWith, "p3m_") {
				t.Fatalf("%s: the %s user face leaks a p3m_ token", board.name, lang)
			}
		}
	}
}

// TestP3MeasureAdvisoryOnlyConsumerAbsence — the mechanical advisory-only
// red line: outside the four authorized write/registry sites (engine stamp,
// engine wire struct, tool emitter, key registry), NO non-test source in
// internal/ references the p3m_* keys or the P3M* fields. A future consumer
// (gate, face, parser) MUST redden this pin first — that red is the review
// surface the §29.169 stage-two ruling has to pass through.
func TestP3MeasureAdvisoryOnlyConsumerAbsence(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("internal", "tracequery", "rank_p3_measure.go"): true, // the measurement itself
		filepath.Join("internal", "tracequery", "types.go"):           true, // the wire struct fields
		filepath.Join("internal", "tracequery", "query.go"):           true, // the two mount hooks (ctx stash + tail stamp)
		filepath.Join("internal", "tool", "trace_query.go"):           true, // the display-only note emitter
		filepath.Join("internal", "types", "trace_note_keys.go"):      true, // the key registry
		// The tracediag schema-hash pin site (R2' 第 7 处): references the
		// fields only in its adjudication comment; the diag dump renders
		// reflectively (the audit face — NOT an answer-pipeline consumer).
		filepath.Join("internal", "tracediag", "render_key_first.go"): true,
	}
	pattern := regexp.MustCompile(`p3m_|P3MCounterfactual|P3MEdgeWitnessed|P3MDisposition|P3MeasureCoverageFamilies|TraceNoteKeyP3M|stampP3CounterfactualMeasure|buildP3MeasureContext|p3MeasureCtx`)
	// 复核收编 (对抗官 P3-1, 2026-07-20): walk the REPO root, not just
	// internal/ — cmd/ and top-level packages are inside the red line too.
	root := "../.."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == ".claude" || base == ".codrax" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if allowed[rel] {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if loc := pattern.Find(raw); loc != nil {
			t.Errorf("advisory-only red line: %s references the P3MEASURE wire (%q) — display_only keys admit no consumer without a new user ruling", rel, loc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
