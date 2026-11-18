package tool

// HMC376 lane L5 — the reader-visible report of one capture must not depend on
// where the capture file lives, where the session keeps its stored results, or
// (for the coverage denominator) which of two rank queries ran first.
//
// Mechanism of the base-commit defect: an observation coordinate minted from a
// query receipt carries a digest of that receipt (capture path, stored-result
// location, query). The projection used the coordinate's lexical order as the
// last tie-break between two publications of one fact, so the digest decided
// which publication kept the row — and with it which rank board the row sat
// on. The coverage denominator then elected "the largest board" over that
// path-chosen split and printed 220.887 ms or 148.975 ms for the same bytes.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	hmc376DonghuFixture = "../../eval/fixtures/real_traces/donghu.ftrace"
	hmc376TiebaFixture  = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
)

// hmc376Recipe is the fixed part of a params fork: one target over one window.
// Only min_duration_ms differs between the boards.
type hmc376Recipe struct {
	fixture string
	params  map[string]any
}

var (
	hmc376DonghuRecipe = hmc376Recipe{fixture: hmc376DonghuFixture, params: map[string]any{
		"pid": 9163, "time_start": 13762.791708, "time_end": 13763.024898,
		"max_depth": 4, "max_branches": 8, "max_chain_nodes": 1, "limit": 12, "trace_flavor": "harmony_hitrace",
	}}
	hmc376TiebaRecipe = hmc376Recipe{fixture: hmc376TiebaFixture, params: map[string]any{
		"pid": 59843, "time_start": 34579.472865, "time_end": 34579.587805, "max_depth": 4, "limit": 12,
	}}
)

// hmc376ParamsForkRun is one complete public run of the params-fork recipe.
type hmc376ParamsForkRun struct {
	label       string
	rendered    string
	denominator string
	// overWindow is the per-board over-window sentence, "" when none is printed.
	overWindow string
	// firstRankCoordinateFirst reports whether the first rank query's
	// observation coordinates sort before the second rank query's. On the base
	// commit this lexical order chose the surviving publication.
	firstRankCoordinateFirst bool
	rankCoordinates          [2]string
}

// hmc376ParamsForkPublicRun asks the public trace_query tool for the wakeup
// chain and the root-cause ranking of thread 9163 over one fixed window, once
// per min_duration_ms value (two rank boards over the same target and window),
// then renders the report through the production mutation and render path.
func hmc376ParamsForkPublicRun(t *testing.T, label, tracePath, sessionDir string, minDurations ...float64) hmc376ParamsForkRun {
	t.Helper()
	return hmc376RecipePublicRun(t, hmc376DonghuRecipe, label, tracePath, sessionDir, minDurations...)
}

// hmc376RecipeResults runs the public queries of one recipe, one wakeup chain
// and one root-cause ranking per board.
func hmc376RecipeResults(t *testing.T, recipe hmc376Recipe, label, tracePath, sessionDir string, minDurations ...float64) []types.ToolResult {
	t.Helper()
	queryBus := &types.BusContext{RepoRoot: sessionDir, WorkDir: sessionDir}
	var results []types.ToolResult
	for _, minDuration := range minDurations {
		for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
			query := map[string]any{"source": "path", "path": tracePath, "view": view, "min_duration_ms": minDuration}
			for key, value := range recipe.params {
				query[key] = value
			}
			params, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&TraceQuery{}).Execute(queryBus, params)
			if err != nil || !result.Success {
				t.Fatalf("%s: public %s (min_duration_ms=%v): %v %s", label, view, minDuration, err, result.Summary)
			}
			results = append(results, result)
		}
	}
	return results
}

func hmc376RecipePublicRun(t *testing.T, recipe hmc376Recipe, label, tracePath, sessionDir string, minDurations ...float64) hmc376ParamsForkRun {
	t.Helper()
	results := hmc376RecipeResults(t, recipe, label, tracePath, sessionDir, minDurations...)
	var rankCoordinates []string
	storedResults := map[string]string{}
	var storedOrder []string
	for i, result := range results {
		// The page may name the stored query result it read a fact from.
		// That file name is a digest of the result payload (which records
		// the capture path), so it is neutralised below — by the ORDINAL
		// of the query that produced it, so a report that attaches a fact
		// to another query's result, or lists the results in another
		// order, still differs.
		for _, record := range result.Observations {
			for _, ref := range []string{record.SourceRef.PayloadRef, record.SourceRef.RawRef} {
				name := filepath.Base(strings.TrimSpace(ref))
				if strings.TrimSpace(ref) == "" || storedResults[name] != "" {
					continue
				}
				storedResults[name] = "<stored-result-of-query-" + itoaHMC376(i+1) + ">"
				storedOrder = append(storedOrder, name)
			}
		}
		if i%2 == 0 {
			continue // the wakeup chain of this board
		}
		coordinate := ""
		for _, record := range result.Observations {
			if strings.HasPrefix(record.Predicate, "root_cause_") {
				coordinate, _, _ = strings.Cut(record.ID, "#")
				break
			}
		}
		if coordinate == "" {
			t.Fatalf("%s: rank query %d published no ranked observation", label, i)
		}
		rankCoordinates = append(rankCoordinates, coordinate)
	}
	if len(rankCoordinates) != len(minDurations) {
		t.Fatalf("%s: the recipe needs one rank board per min_duration_ms, got %d", label, len(rankCoordinates))
	}

	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = results
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "path identity witness。"}}}
	applied, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !applied.Success {
		t.Fatalf("%s: apply: %v %s", label, err, applied.Summary)
	}
	rendered := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	// The page may legitimately print where the material came from. Neutralise
	// exactly those locators — the capture directory, the session directory
	// and the stored-result file names — so the comparison below is about
	// every other reader-visible byte.
	rendered = strings.ReplaceAll(rendered, filepath.Dir(tracePath), "<capture-dir>")
	rendered = strings.ReplaceAll(rendered, sessionDir, "<session-dir>")
	for _, name := range storedOrder {
		rendered = strings.ReplaceAll(rendered, name, storedResults[name])
	}

	run := hmc376ParamsForkRun{label: label, rendered: rendered}
	if len(rankCoordinates) >= 2 {
		run.firstRankCoordinateFirst = rankCoordinates[0] < rankCoordinates[1]
		run.rankCoordinates = [2]string{rankCoordinates[0], rankCoordinates[1]}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if run.denominator == "" && strings.Contains(line, "关注线程等待(sleep/D-state/runnable)") {
			run.denominator = line
		}
		if strings.Contains(line, "有效归因合计") && strings.Contains(line, "超过窗长") {
			run.overWindow = line
		}
	}
	if run.denominator == "" {
		t.Fatalf("%s: the report carries no coverage denominator sentence:\n%s", label, rendered)
	}
	return run
}

func hmc376CopyFixture(t *testing.T, raw []byte, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "donghu.ftrace")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// hmc376FirstDifference names the first line on which two reports differ.
func hmc376FirstDifference(a, b string) string {
	left, right := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(left) || i < len(right); i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l != r {
			return "line " + itoaHMC376(i+1) + "\n  - " + l + "\n  + " + r
		}
	}
	return "(no line differs)"
}

func itoaHMC376(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

// The denominator the recipe must print: the largest single board's own
// account of the target's wait rows. Hand check on the fixture — the
// min_duration_ms=0.5 board publishes five runnable accounts of thread 9163
// (48.519 + 40.071 + 31.728 + 17.928 + 32.877 = 171.123 ms); the 5.0 board
// publishes 48.519 + 31.728 + 17.928 + 32.877 (the same four facts) plus one
// aggregate 17.815 = 148.867 ms. The larger account plus the one admitted
// sleep view (49.764 ms) is 220.887 ms, equal to the single-query denominator
// of the 0.5 board. 148.975 ms is not any board's account: it is 99.211 ms —
// the part of the 5.0 board left after a path-chosen split of the shared
// facts — plus the same 49.764 ms.
const (
	hmc376OwnerDenominator = "关注线程等待(sleep/D-state/runnable) 220.887ms 中链路覆盖"
	hmc376SplitDenominator = "148.975ms"
)

// TestHMC376ParamsForkReportIsIndependentOfCapturePathPublic — the same capture
// bytes under at least eight directories, one shared session directory, the
// identical public recipe: every reader-visible byte must be the same.
//
// The directory set is completed until it holds both lexical orders of the two
// rank publications' coordinates. Those two orders are exactly the two classes
// that printed 220.887 ms and 148.975 ms on the base commit, so the set always
// contains a path of each class, whatever the machine's temporary root is.
func TestHMC376ParamsForkReportIsIndependentOfCapturePathPublic(t *testing.T) {
	raw, err := os.ReadFile(hmc376DonghuFixture)
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	session := filepath.Join(root, "session")
	names := []string{
		"a", "bb", "ccc", "dddd", "eeeee", "f6", "g7", "h8",
		// Reserve: only used while one coordinate order is still missing.
		"i9", "j10", "k11", "l12", "m13", "n14", "o15", "p16", "q17", "r18", "s19", "t20", "u21", "v22", "w23", "x24",
	}
	const minimumPaths = 8
	var runs []hmc376ParamsForkRun
	orders := map[bool]int{}
	for _, name := range names {
		if len(runs) >= minimumPaths && orders[true] > 0 && orders[false] > 0 {
			break
		}
		path := hmc376CopyFixture(t, raw, filepath.Join(root, "captures", name))
		run := hmc376ParamsForkPublicRun(t, name, path, session, 0.5, 5.0)
		orders[run.firstRankCoordinateFirst]++
		runs = append(runs, run)
	}
	// Control: the directory really reaches the observation coordinates, and
	// both of their lexical orders are in the set. Without this the test could
	// pass on eight paths that happen to fall into one class.
	if runs[0].rankCoordinates == runs[1].rankCoordinates {
		t.Fatalf("control: two capture directories minted the same rank coordinates %q; the path no longer reaches them and this test proves nothing",
			runs[0].rankCoordinates)
	}
	if orders[true] == 0 || orders[false] == 0 {
		t.Fatalf("control: %d directories produced only one lexical order of the rank coordinates (first-query-first=%d, second-first=%d)",
			len(runs), orders[true], orders[false])
	}

	var report []string
	for _, run := range runs {
		order := "first rank query's coordinate sorts first"
		if !run.firstRankCoordinateFirst {
			order = "second rank query's coordinate sorts first"
		}
		report = append(report, run.label+": "+order+" → "+run.denominator)
	}
	summary := strings.Join(report, "\n")
	for _, run := range runs {
		if strings.Contains(run.rendered, hmc376SplitDenominator) {
			t.Fatalf("%s: the denominator is a path-chosen split of the shared facts, not a board's own account:\n%s", run.label, summary)
		}
		if !strings.Contains(run.denominator, hmc376OwnerDenominator) {
			t.Fatalf("%s: the denominator must be the largest single board's own account (220.887 ms):\n%s", run.label, summary)
		}
		if !strings.Contains(run.denominator, "另有 1 条关注线程状态行未计入分母(单项最大 18.933ms)") {
			t.Fatalf("%s: the periodic-idle exclusion must stay disclosed:\n%s", run.label, summary)
		}
		if run.rendered != runs[0].rendered {
			t.Fatalf("the same capture bytes rendered differently under directories %q and %q (first difference %s)\n%s",
				runs[0].label, run.label, hmc376FirstDifference(runs[0].rendered, run.rendered), summary)
		}
	}
}

// TestHMC376ParamsForkReportIsIndependentOfSessionDirectoryPublic — sibling of
// the same class: the coordinate digest also hashes where the session stored
// the query result, so one capture file at one path must render identically
// from different session directories.
func TestHMC376ParamsForkReportIsIndependentOfSessionDirectoryPublic(t *testing.T) {
	raw, err := os.ReadFile(hmc376DonghuFixture)
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	path := hmc376CopyFixture(t, raw, filepath.Join(root, "capture"))
	names := []string{"s1", "s22", "s333", "s4444", "s5", "s6", "s7", "s8", "s9", "s10", "s11", "s12", "s13", "s14", "s15", "s16"}
	const minimumSessions = 2
	var runs []hmc376ParamsForkRun
	orders := map[bool]int{}
	for _, name := range names {
		if len(runs) >= minimumSessions && orders[true] > 0 && orders[false] > 0 {
			break
		}
		run := hmc376ParamsForkPublicRun(t, name, path, filepath.Join(root, "sessions", name), 0.5, 5.0)
		orders[run.firstRankCoordinateFirst]++
		runs = append(runs, run)
	}
	if orders[true] == 0 || orders[false] == 0 {
		t.Fatalf("control: %d session directories produced only one lexical order of the rank coordinates", len(runs))
	}
	for _, run := range runs {
		if !strings.Contains(run.denominator, hmc376OwnerDenominator) {
			t.Fatalf("session %s: the denominator must be the largest single board's own account (220.887 ms): %s", run.label, run.denominator)
		}
		if run.rendered != runs[0].rendered {
			t.Fatalf("one capture file rendered differently from session directories %q and %q (first difference %s)",
				runs[0].label, run.label, hmc376FirstDifference(runs[0].rendered, run.rendered))
		}
	}
}

// TestHMC376ParamsForkDenominatorOwnerIsIndependentOfQueryOrderPublic — the
// owner of the denominator is a typed role (the board whose own account of the
// target's wait rows is largest), not the board that happened to publish a
// shared fact first. Asking the 5.0 board before the 0.5 board must print the
// same denominator sentence.
func TestHMC376ParamsForkDenominatorOwnerIsIndependentOfQueryOrderPublic(t *testing.T) {
	raw, err := os.ReadFile(hmc376DonghuFixture)
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	path := hmc376CopyFixture(t, raw, filepath.Join(root, "capture"))
	fineFirst := hmc376ParamsForkPublicRun(t, "0.5 then 5.0", path, filepath.Join(root, "session"), 0.5, 5.0)
	coarseFirst := hmc376ParamsForkPublicRun(t, "5.0 then 0.5", path, filepath.Join(root, "session"), 5.0, 0.5)
	for _, run := range []hmc376ParamsForkRun{fineFirst, coarseFirst} {
		if !strings.Contains(run.denominator, hmc376OwnerDenominator) {
			t.Fatalf("%s: the denominator must be the largest single board's own account (220.887 ms): %s", run.label, run.denominator)
		}
	}
	if fineFirst.denominator != coarseFirst.denominator {
		t.Fatalf("the denominator sentence follows the query order:\n  0.5 then 5.0: %s\n  5.0 then 0.5: %s", fineFirst.denominator, coarseFirst.denominator)
	}
}

// hmc376Orders lists every order of n boards.
func hmc376Orders(n int) [][]int {
	if n == 1 {
		return [][]int{{0}}
	}
	var out [][]int
	for _, order := range hmc376Orders(n - 1) {
		for at := 0; at <= len(order); at++ {
			next := append(append(append([]int{}, order[:at]...), n-1), order[at:]...)
			out = append(out, next)
		}
	}
	return out
}

// hmc376DenominatorMS reads the value the coverage sentence divides by.
func hmc376DenominatorMS(t *testing.T, run hmc376ParamsForkRun) string {
	t.Helper()
	_, rest, ok := strings.Cut(run.denominator, "关注线程等待(sleep/D-state/runnable) ")
	if !ok {
		t.Fatalf("%s: no denominator in %q", run.label, run.denominator)
	}
	value, _, ok := strings.Cut(rest, "ms")
	if !ok {
		t.Fatalf("%s: no denominator value in %q", run.label, run.denominator)
	}
	return value
}

// TestHMC376CrossBoardMergeKeepsTheLargestBoardsOwnAccountPublic — the same-kind
// merge groups by thread and wait, not by board, so boards that measured one
// wait at their own thresholds fold into one row whose value adds every board's
// measurement. The coverage denominator must still be the largest single board's
// own account: what that board prints when it is queried alone, for every order
// the boards are asked in. The over-window sentence reads the same per-board
// accounts, so it must not follow the query order either.
func TestHMC376CrossBoardMergeKeepsTheLargestBoardsOwnAccountPublic(t *testing.T) {
	cases := []struct {
		name   string
		recipe hmc376Recipe
		boards []float64
		want   string
	}{
		// Row 91.375 ms ×3 = 40.071 + 33.489 + 17.815, one member per board.
		{"donghu three boards", hmc376DonghuRecipe, []float64{0.5, 2, 5}, "220.887"},
		// Row 44.544 ms ×4 carries members of both boards.
		{"tieba two boards", hmc376TiebaRecipe, []float64{0.5, 2}, "116.784"},
		{"tieba three boards", hmc376TiebaRecipe, []float64{0.05, 0.5, 2}, "117.523"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(tc.recipe.fixture)
			if err != nil {
				t.Fatalf("in-repo fixture: %v", err)
			}
			root := t.TempDir()
			dir := filepath.Join(root, "capture")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, filepath.Base(tc.recipe.fixture))
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			session := filepath.Join(root, "session")
			// Control: the expected value is the largest board's account as
			// that board prints it alone, not a number copied from a run.
			largest := ""
			for _, board := range tc.boards {
				alone := hmc376DenominatorMS(t, hmc376RecipePublicRun(t, tc.recipe, "alone", path, session, board))
				if largest == "" || hmc376ParseMS(t, alone) > hmc376ParseMS(t, largest) {
					largest = alone
				}
			}
			if largest != tc.want {
				t.Fatalf("control: the largest board alone prints %s ms, the case expects %s ms", largest, tc.want)
			}
			var first hmc376ParamsForkRun
			for i, order := range hmc376Orders(len(tc.boards)) {
				boards := make([]float64, 0, len(order))
				for _, at := range order {
					boards = append(boards, tc.boards[at])
				}
				label, _ := json.Marshal(boards)
				run := hmc376RecipePublicRun(t, tc.recipe, string(label), path, session, boards...)
				if got := hmc376DenominatorMS(t, run); got != tc.want {
					t.Fatalf("boards asked in order %s: the denominator must be the largest board's own account %s ms, got %s ms\n%s", label, tc.want, got, run.denominator)
				}
				if i == 0 {
					first = run
					continue
				}
				if run.denominator != first.denominator {
					t.Fatalf("the coverage sentence follows the query order:\n  %s: %s\n  %s: %s", first.label, first.denominator, run.label, run.denominator)
				}
				if run.overWindow != first.overWindow {
					t.Fatalf("the over-window sentence follows the query order:\n  %s: %q\n  %s: %q", first.label, first.overWindow, run.label, run.overWindow)
				}
			}
		})
	}
}

func hmc376ParseMS(t *testing.T, value string) float64 {
	t.Helper()
	var parsed float64
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		t.Fatalf("not a millisecond value %q: %v", value, err)
	}
	return parsed
}

// TestHMC376ThreeBoardReportIsIndependentOfCapturePathPublic — the cross-board
// merge under eight capture directories: every reader-visible byte is the same.
func TestHMC376ThreeBoardReportIsIndependentOfCapturePathPublic(t *testing.T) {
	raw, err := os.ReadFile(hmc376DonghuFixture)
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	session := filepath.Join(root, "session")
	var runs []hmc376ParamsForkRun
	for _, name := range []string{"a", "bb", "ccc", "dddd", "eeeee", "f6", "g7", "h8"} {
		path := hmc376CopyFixture(t, raw, filepath.Join(root, "captures", name))
		runs = append(runs, hmc376RecipePublicRun(t, hmc376DonghuRecipe, name, path, session, 0.5, 2, 5))
	}
	if runs[0].rankCoordinates == runs[1].rankCoordinates {
		t.Fatalf("control: two capture directories minted the same rank coordinates; the path no longer reaches them")
	}
	for _, run := range runs {
		if got := hmc376DenominatorMS(t, run); got != "220.887" {
			t.Fatalf("%s: the denominator must be 220.887 ms, got %s ms", run.label, got)
		}
		if run.rendered != runs[0].rendered {
			t.Fatalf("the same capture bytes rendered differently under directories %q and %q (first difference %s)",
				runs[0].label, run.label, hmc376FirstDifference(runs[0].rendered, run.rendered))
		}
	}
}

// TestHMC376RootCauseSideFileIsIndependentOfCapturePathPublic — the root-cause
// side file carries no path-derived identity, and neither its roster order nor
// the contract scope (taken from the first candidate) may follow the directory
// the capture was read from.
func TestHMC376RootCauseSideFileIsIndependentOfCapturePathPublic(t *testing.T) {
	raw, err := os.ReadFile(hmc376DonghuFixture)
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	session := filepath.Join(root, "session")
	type sideFile struct{ name, body string }
	var files []sideFile
	for _, name := range []string{"a", "bb", "ccc", "dddd", "eeeee", "f6", "g7", "h8"} {
		path := hmc376CopyFixture(t, raw, filepath.Join(root, "captures", name))
		results := hmc376RecipeResults(t, hmc376DonghuRecipe, name, path, session, 0.5, 5)
		ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results})
		contract, err := tracefinding.CompileCandidateContract(ledger, types.CompileTraceCausalProjectionSet(ledger), tracefinding.SeatFrameCausalityAuthority{})
		if err != nil {
			t.Fatal(err)
		}
		contract.RootCauseReportEnabled = true
		selectable := tracefinding.SelectableRootCauseCandidates(contract)
		ranks := map[int]int{}
		for _, candidate := range selectable {
			ranks[candidate.Decision.Rank]++
		}
		shared := 0
		for _, count := range ranks {
			if count > 1 {
				shared++
			}
		}
		if len(selectable) < 2 || shared == 0 {
			t.Fatalf("control: the recipe must publish candidates that share a rank, got %d candidates over ranks %v", len(selectable), ranks)
		}
		var body strings.Builder
		scope, _ := json.Marshal(contract.Scope)
		body.Write(scope)
		for _, candidate := range selectable {
			bound, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
				SchemaVersion: types.TraceRootCauseReportSchemaVersion,
				RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: candidate.Decision.CandidateID}},
			}, contract)
			if err != nil {
				t.Fatalf("%s: bind %s: %v", name, candidate.Decision.SubjectName, err)
			}
			item, _ := json.Marshal(bound)
			body.WriteString("\n")
			body.Write(item)
		}
		files = append(files, sideFile{name, strings.ReplaceAll(body.String(), filepath.Dir(path), "<capture-dir>")})
	}
	for _, file := range files {
		if file.body != files[0].body {
			t.Fatalf("the side file differs between capture directories %q and %q (first difference %s)",
				files[0].name, file.name, hmc376FirstDifference(files[0].body, file.body))
		}
	}
}
