package agent

// HMC376: what later stages and the reader see must not depend on the directory
// a capture file was read from. Observation coordinates and stored-result names
// carry digests of that location, so they may identify a publication but must
// never order one.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Both masks try a decimal first: the fractional digits of a published value
// look like a digest, and a value that moves with the capture's location must
// stay visible to the comparison.
var (
	hmc376DecimalValue = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	hmc376Digest       = regexp.MustCompile(`[0-9]+\.[0-9]+|[0-9a-f]{16,}`)
	// A file generation token ends in a sub-second change time of any length,
	// so the token is masked whole rather than field by field. A stored result's
	// name is a digest that the row budget may cut short.
	hmc376Locator = regexp.MustCompile(`[0-9]+\.[0-9]+|strong:[0-9a-z<>:]+|trace-query-result-[0-9a-f]*|[0-9a-f]{8,}`)
)

// hmc376MaskKeepingDecimals replaces what mask matches, except decimal values.
func hmc376MaskKeepingDecimals(mask *regexp.Regexp, text string) string {
	return mask.ReplaceAllStringFunc(text, func(token string) string {
		if hmc376DecimalValue.MatchString(token) {
			return token
		}
		return "<digest>"
	})
}

// hmc376MaskLocators masks every locator of a finalizer instruction.
func hmc376MaskLocators(text string) string {
	return hmc376MaskKeepingDecimals(hmc376Locator, text)
}

// hmc376NeutralLocators replaces the capture directory and every digest, leaving
// the order and the content of the text to compare.
func hmc376NeutralLocators(text string, dirs ...string) string {
	for _, dir := range dirs {
		text = strings.ReplaceAll(text, dir, "<dir>")
	}
	return hmc376MaskKeepingDecimals(hmc376Digest, text)
}

// The masks hide locators and nothing a reader is told as a number.
func TestHMC376LocatorMasksKeepDecimalValuesVisible(t *testing.T) {
	digest := strings.Repeat("0a", 32)
	for _, c := range []struct{ text, want string }{
		{"end±0.000500000", "end±0.000500000"},
		{"peak 0.00800000000822 ms", "peak 0.00800000000822 ms"},
		{"share 0.12345678901234567", "share 0.12345678901234567"},
		{"trace_query:publication-" + digest + "#rank:1", "trace_query:publication-<digest>#rank:1"},
	} {
		if got := hmc376MaskLocators(c.text); got != c.want {
			t.Errorf("hmc376MaskLocators(%q) = %q, want %q", c.text, got, c.want)
		}
		if got := hmc376NeutralLocators(c.text); got != c.want {
			t.Errorf("hmc376NeutralLocators(%q) = %q, want %q", c.text, got, c.want)
		}
	}
	for text, want := range map[string]string{
		"id=abcdef01":                  "id=<digest>",
		"trace-query-result-0a1b":      "<digest>",
		"token strong:12:ab<x>:3 kept": "token <digest> kept",
	} {
		if got := hmc376MaskLocators(text); got != want {
			t.Errorf("hmc376MaskLocators(%q) = %q, want %q", text, got, want)
		}
	}
}

func hmc376FirstDifferentLine(a, b string) string {
	left, right := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] != right[i] {
			at := 0
			for at < len(left[i]) && at < len(right[i]) && left[i][at] == right[i][at] {
				at++
			}
			from := max(at-80, 0)
			return "\n  - …" + strings.ToValidUTF8(left[i][from:min(at+160, len(left[i]))], "") +
				"\n  + …" + strings.ToValidUTF8(right[i][from:min(at+160, len(right[i]))], "")
		}
	}
	return "(one text is a prefix of the other)"
}

// TestHMC376BusinessAccountsAreIndependentOfCapturePathPublic — the finalizer's
// business-account block numbers its populations in query-group order. Eight
// copies of one capture must give the same groups in the same order.
func TestHMC376BusinessAccountsAreIndependentOfCapturePathPublic(t *testing.T) {
	var first, firstDir string
	for i := 0; i < 8; i++ {
		ctx, path := hmc323BusinessBudgetContext(t)
		for _, selectors := range []map[string]any{
			{"pid": 700, "time_start": 5.0, "time_end": 5.111487, "limit": 40},
			{"pid": 700, "time_start": 5.2, "time_end": 5.311487, "limit": 40},
			{"pid": 700, "time_start": 5.0, "time_end": 5.4, "limit": 12},
		} {
			hmc320BusinessQuery(t, ctx, path, selectors)
		}
		ledger := answerDocObservationLedger(ctx)
		plan := planAnswerDocBusinessTreeFacts(ctx, ledger)
		if plan.groups != 3 || plan.shownGroups != 3 {
			t.Fatalf("control: three independent queries must each keep a group, got groups=%d shown=%d", plan.groups, plan.shownGroups)
		}
		block := hmc376NeutralLocators(renderAnswerDocBusinessAccountPlan(plan), filepath.Dir(path), ctx.WorkDir)
		if !strings.Contains(block, "business-population-3") {
			t.Fatalf("control: the block must number three populations:\n%s", block)
		}
		if first == "" {
			first, firstDir = block, filepath.Dir(path)
			continue
		}
		if block != first {
			t.Fatalf("the business-account block differs between %s and %s: first difference%s", firstDir, filepath.Dir(path), hmc376FirstDifferentLine(first, block))
		}
	}
}

// TestHMC376FinalizerInstructionIsIndependentOfCapturePathPublic — the whole
// instruction the finalizer receives: business accounts, nesting relations and
// event inventories of eight copies of one capture read the same, in the same
// order, once the locators themselves are masked.
func TestHMC376FinalizerInstructionIsIndependentOfCapturePathPublic(t *testing.T) {
	observedAt := regexp.MustCompile(`20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9:]+[-+][0-9:]+`)
	// A locator the row budget cut can end in the sequence number of its copy.
	cutCopySequence := regexp.MustCompile(`/[0-9]{1,3}(\.\.\.|…)\[truncated\]`)
	// Prompt budgets charge locator bytes. One short root keeps every copy
	// inside the same budget whatever temporary directory the run was given;
	// t.TempDir prefers GOTMPDIR to TMPDIR.
	root, err := os.MkdirTemp("/tmp", "h376")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("TMPDIR", root)
	t.Setenv("GOTMPDIR", root)
	var first, firstDir string
	for i := 0; i < 8; i++ {
		ctx, path := hmc323BusinessBudgetContext(t)
		for _, selectors := range []map[string]any{
			{"pid": 700, "time_start": 5.0, "time_end": 5.111487, "limit": 40},
			{"pid": 700, "time_start": 5.2, "time_end": 5.311487, "limit": 40},
			{"pid": 700, "time_start": 5.0, "time_end": 5.4, "limit": 12},
			{"pid": 700, "view": "event_search", "time_start": 5.0, "time_end": 5.4, "event_names": []string{"tracing_mark_write"}, "limit": 5},
		} {
			hmc320BusinessQuery(t, ctx, path, selectors)
		}
		system, user := finalizerReaderFieldMessages(t, ctx)
		text := hmc376NeutralLocators(system+"\n"+user, filepath.Dir(path), ctx.WorkDir)
		text = observedAt.ReplaceAllString(hmc376MaskLocators(text), "<time>")
		text = cutCopySequence.ReplaceAllString(text, "/<seq>${1}[truncated]")
		for _, control := range []string{"business-population-3", "### Trace Event Search Inventories"} {
			if !strings.Contains(text, control) {
				t.Fatalf("control: the instruction must carry %q", control)
			}
		}
		if first == "" {
			first, firstDir = text, filepath.Dir(path)
			continue
		}
		if text != first {
			t.Fatalf("the finalizer instruction differs between %s and %s: first difference%s", firstDir, filepath.Dir(path), hmc376FirstDifferentLine(first, text))
		}
	}
}

// TestHMC376TraceHandoffsAreIndependentOfCapturePathPublic — two hand-offs of one
// real capture, read from eight directories. The stage report gives external
// observations to later model stages; the business-account block spends its slots
// across four queries whose measured accounts are equal, so the order of those
// queries decides which accounts are shown at all.
func TestHMC376TraceHandoffsAreIndependentOfCapturePathPublic(t *testing.T) {
	raw, err := os.ReadFile("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatalf("in-repo fixture: %v", err)
	}
	root := t.TempDir()
	var firstReport, firstAccounts, firstName string
	for _, name := range []string{"a", "bb", "ccc", "dddd", "eeeee", "f6", "g7", "h8"} {
		dir := filepath.Join(root, "captures", name)
		work := filepath.Join(root, "work", name)
		for _, d := range []string{dir, work} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(dir, "donghu.ftrace")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		rm := types.RequestModel{Language: "zh", Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck, PerfTrace: &types.PerfBundle{}}
		ctx := &types.AgentContext{RepoRoot: dir, WorkDir: work, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
			Mutable: types.NewMutableState("why is thread 9163 slow"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
		ctx.Mutable.SetRequestModel(rm)
		var results []types.ToolResult
		for _, minDuration := range []float64{0.5, 5} {
			for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
				params, err := json.Marshal(map[string]any{
					"source": "path", "path": path, "view": view, "pid": 9163,
					"time_start": 13762.791708, "time_end": 13763.024898, "max_depth": 4, "max_branches": 8,
					"max_chain_nodes": 1, "min_duration_ms": minDuration, "limit": 12, "trace_flavor": "harmony_hitrace",
				})
				if err != nil {
					t.Fatal(err)
				}
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !result.Success {
					t.Fatalf("%s: public %s: %v %s", name, view, err, result.Summary)
				}
				results = append(results, result)
				ctx.Mutable.AppendDispatchToolResult(result)
			}
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})

		ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results})
		observations := types.PrioritizeObservationRecords(ledger.Records, nil, nil, 16)
		report := renderExplorerStageReport("", "", nil, nil, nil, nil, nil, nil, false, observations...)
		if !strings.Contains(report, "## External Observations") {
			t.Fatalf("control: the report must list the external observations:\n%s", report)
		}
		report = hmc376NeutralLocators(report, dir, work)

		plan := planAnswerDocBusinessTreeFacts(ctx, answerDocObservationLedger(ctx))
		if plan.groups != 4 || plan.shownGroups != 4 {
			t.Fatalf("control: four queries must each keep a group, got groups=%d shown=%d", plan.groups, plan.shownGroups)
		}
		accounts := hmc376NeutralLocators(renderAnswerDocBusinessAccountPlan(plan), dir, work)

		if firstName == "" {
			firstReport, firstAccounts, firstName = report, accounts, name
			continue
		}
		if report != firstReport {
			t.Fatalf("the stage report differs between capture directories %q and %q: first difference%s", firstName, name, hmc376FirstDifferentLine(firstReport, report))
		}
		if accounts != firstAccounts {
			t.Fatalf("the business-account block differs between capture directories %q and %q: first difference%s", firstName, name, hmc376FirstDifferentLine(firstAccounts, accounts))
		}
	}
}
