package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Actual TraceQuery -> dispatch -> TurnA -> initial finalizer instruction and
// shared semantic compact projection. Values are checked against the native
// payload first: an unconnected engine is not a context-specific RED.
func TestIOInFlightPublicFinalizerContext(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx, result, native := ioInFlightPublicContext(t, lang, scope, "")
				if native == nil || native.Window == nil {
					t.Fatal("engine prerequisite: successful public query has no measured IOInFlight window")
				}
				if native.Window.StartTs != 1 || native.Window.EndTs != 1.01 || native.Population != tracequery.IOInFlightPopulationCompletePairs || native.IssuerScope != tracequery.IOInFlightIssuerScopeAll {
					t.Fatalf("native scope/population drift: %+v", native)
				}
				ledger := answerDocObservationLedger(ctx)
				before, _ := json.Marshal(struct {
					Result  types.ToolResult
					Ledger  types.ObservationLedger
					Request types.RequestModel
				}{result, ledger, ctx.AnalysisIR.RequestModel})
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				semantic := types.ProjectObservationPromptRecords(ledger.Records, &ctx.AnalysisIR.RequestModel, nil, types.SemanticReviewObservationPromptProjectionOptions(types.ObservationPromptRecordLimit))
				var newRecords []types.ObservationRecord
				for _, want := range []struct {
					family, dev, op        string
					peak, mean, busy, area float64
				}{{"block_rq", "8,0", "R", 2, 1.4, 10, 14}, {"block_rq", "8,0", "W", 1, .6, 6, 6}, {"block_bio", "8,0", "R", 1, 1, 10, 10}, {"block_rq", "8,1", "R", 1, .4, 4, 4}} {
					var group *tracequery.IOInFlightGroup
					for i := range native.Groups {
						g := &native.Groups[i]
						if g.EndpointFamily == want.family && g.Dev == want.dev && g.Operation == want.op {
							group = g
						}
					}
					if group == nil || group.Values == nil {
						t.Fatalf("engine prerequisite: missing measured group %s/%s/%s", want.family, want.dev, want.op)
					}
					v := group.Values
					if float64(v.PeakRequests) != want.peak || math.Abs(v.MeanRequests-want.mean) > 1e-7 || math.Abs(v.BusyMs-want.busy) > 1e-7 || math.Abs(v.RequestMs-want.area) > 1e-7 {
						t.Fatalf("engine prerequisite: group %s/%s/%s values=%+v", want.family, want.dev, want.op, v)
					}
					var matches []types.ObservationRecord
					for _, row := range ledger.Records {
						if row.Predicate != "io_inflight" {
							continue
						}
						identity := strings.Join(row.RichNotes, " ")
						if strings.Contains(identity, "endpoint_family="+want.family) && strings.Contains(identity, "dev="+want.dev) && strings.Contains(identity, "op="+want.op) {
							matches = append(matches, row)
						}
					}
					if len(matches) != 1 {
						t.Errorf("typed context must retain one independent group %s/%s/%s; got %d", want.family, want.dev, want.op, len(matches))
						continue
					}
					row := matches[0]
					newRecords = append(newRecords, row)
					if row.Role != types.AnswerAggregateRoleSupportingCoverage || row.Origin != types.AnswerEvidenceOriginRuntimeArtifact || row.SourceRef.Path != group.SourcePath || row.SourceRef.QueryScopeID == "" || !row.SourceRef.QueryWindowKnown || row.SourceRef.QueryWindowStartTs != 1 || row.SourceRef.QueryWindowEndTs != 1.01 {
						t.Errorf("group source/window or support-only authority changed: %+v", row)
					}
					finalLine := ioInFlightPublicPromptLine(prompt, row.ID)
					semanticLine := ""
					for _, projected := range semantic {
						if projected.ID == row.ID {
							semanticLine = projected.Summary + " " + strings.Join(projected.Notes, " ")
							if len(projected.Notes) > 6 {
								t.Error("semantic projection expanded the existing six-note budget")
							}
						}
					}
					for surface, line := range map[string]string{"initial": finalLine, "semantic": semanticLine} {
						for _, token := range []string{"endpoint_family=" + want.family, "dev=" + want.dev, "op=" + want.op, "all_issuers", "accepted_complete_pairs", "1.000000..1.010000", "requests", "request·ms"} {
							if !strings.Contains(line, token) {
								t.Errorf("%s same-ID group %s lost %q: %s", surface, row.ID, token, line)
							}
						}
						for key, value := range map[string]float64{"peak_requests": want.peak, "mean_requests": want.mean, "busy_ms": want.busy, "request_ms": want.area} {
							ioInFlightPublicNumber(t, surface, line, key, value)
						}
					}
					timeline := traceQueryObservationSupplementNoteValue(row, "io_inflight_timeline")
					if timeline == "" || !strings.Contains(ioInFlightPublicAllPromptRows(prompt, row.ID), timeline) || !strings.Contains(semanticLine, timeline) {
						t.Errorf("same-ID initial/semantic context lost the producer's bounded timeline for %s: %q", row.ID, timeline)
					}
				}
				coverageFound := false
				for _, row := range ledger.Records {
					if row.Predicate != "io_inflight_coverage" || !strings.Contains(row.Summary+strings.Join(row.RichNotes, " "), "partial") {
						continue
					}
					coverageFound = true
					newRecords = append(newRecords, row)
					line := ioInFlightPublicPromptLine(prompt, row.ID)
					for _, key := range []string{"unpaired_start", "ambiguous_cohort", "pairing_suppressed"} {
						if !strings.Contains(line, key) {
							t.Errorf("actual initial context lost partial pairing coverage %s: %s", key, line)
						}
					}
				}
				if !coverageFound {
					t.Error("actual context lost separate partial pairing coverage")
				}
				projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: newRecords})
				if projection.PrimaryRootCause != nil || len(projection.PrimaryRootCauses)+len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.BackgroundCauses) != 0 || ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("in-flight observations acquired root-cause candidates or a sidecar")
				}
				after, _ := json.Marshal(struct {
					Result  types.ToolResult
					Ledger  types.ObservationLedger
					Request types.RequestModel
				}{result, answerDocObservationLedger(ctx), ctx.AnalysisIR.RequestModel})
				if string(before) != string(after) {
					t.Fatal("context rendering changed query, ledger or request")
				}
				// A user thread called "block" is not this all-issuer account's
				// owner, even though the producer uses block as a display subject.
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = "block"
				collision := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, row := range newRecords {
					lines := ioInFlightPublicAllPromptRows(collision, row.ID)
					if strings.Contains(lines, "owner_scope=`target_owned`") || !strings.Contains(lines, "owner_scope=`selected_window_context`") {
						t.Errorf("all-issuer account acquired matching thread-name ownership: %s", lines)
					}
				}
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = ""
				// A narrower request cannot borrow the original wider statistics.
				start, end := 1.002, 1.004
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				narrow := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, row := range newRecords {
					if ioInFlightPublicPromptLine(narrow, row.ID) != "" {
						t.Errorf("wider in-flight result %s leaked into narrower selected-window context", row.ID)
					}
				}
			})
		}
	}
}

// Independent query windows must not be fused merely to fit a display budget.
// The real producer publishes two coverage families per query: those records
// must not consume all ten handoff slots before any measured group is visible.
func TestIOInFlightPublicRepeatedQueryHandoffBudget(t *testing.T) {
	ctx, first, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeCausalDiagnosis, "")
	path := first.Observations[0].SourceRef.Path
	for i := 0; i < 5; i++ {
		// i=0 repeats the initial query; the remaining four have independent
		// contained windows. No result ID, measurement or receipt is forged.
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1 + float64(i)/1000, "time_end": 1.01})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("actual additional query failed: %v %+v", err, result)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	rows, omitted := answerDocIOInFlightMeasurementRows(ledger, &ctx.AnalysisIR.RequestModel)
	eligible := 0
	byID := map[string]types.ObservationRecord{}
	for _, row := range ledger.Records {
		if row.Predicate == "io_inflight" || row.Predicate == "io_inflight_coverage" {
			eligible++
			byID[row.ID] = row
		}
	}
	if eligible <= 10 || len(rows) != 10 || omitted != eligible-len(rows) {
		t.Fatalf("display cap/omissions drift: eligible=%d rows=%d omitted=%d", eligible, len(rows), omitted)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	_, handoff, found := strings.Cut(prompt, "#### IO In-Flight Measurements\n")
	if !found {
		t.Fatal("actual initial context omitted the bounded measurement handoff")
	}
	handoff, _, _ = strings.Cut(handoff, "\n###")
	if !strings.Contains(handoff, fmt.Sprintf("rendered_source_rows=10 omitted_source_rows=%d", omitted)) {
		t.Fatal("actual handoff lost its exact omission disclosure")
	}
	groupCount, seen := 0, map[string]bool{}
	for _, row := range rows {
		if seen[row.ID] || !reflect.DeepEqual(row, byID[row.ID]) {
			t.Fatalf("selected row repeated or changed its original receipt: %+v", row)
		}
		seen[row.ID] = true
		line := ioInFlightPublicPromptLine(handoff, row.ID)
		if line == "" || !strings.Contains(line, fmt.Sprintf("source_path=%q", row.SourceRef.Path)) ||
			!strings.Contains(line, fmt.Sprintf("%.6f..%.6f", row.SourceRef.QueryWindowStartTs, row.SourceRef.QueryWindowEndTs)) {
			t.Errorf("same-ID source/window missing from actual handoff: %s", line)
		}
		if row.Predicate == "io_inflight" {
			groupCount++
			if !strings.Contains(line, "peak_requests=") || !strings.Contains(line, "request·ms") {
				t.Errorf("actual measured group lost its numbers/units: %s", line)
			}
		}
	}
	if groupCount == 0 {
		t.Error("independent coverage rows crowded every measured group out of the actual handoff")
	}
	// Exact duplicate publication is safe to coalesce; distinct query receipts
	// above are not. This is an explicit selector-level check, not a new query.
	replayed := types.ObservationLedger{Records: append(append([]types.ObservationRecord(nil), rows...), rows...)}
	replayRows, replayOmitted := answerDocIOInFlightMeasurementRows(replayed, &ctx.AnalysisIR.RequestModel)
	if len(replayRows) != len(rows) || replayOmitted != len(rows) {
		t.Fatalf("identical publication replay changed accounting: rows=%d omitted=%d", len(replayRows), replayOmitted)
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("bounded display changed the source observations")
	}
}

func TestIOInFlightPublicEightGroupsKeepCompleteHandoffBudget(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&body, "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,%d R 4096 () 123 + 8 [io]\n", i)
	}
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&body, "irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,%d R () 123 + 8 [0]\n", i)
	}
	path := filepath.Join(t.TempDir(), "eight-groups.systrace")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	// Match the producer's physical locator, including macOS /var aliases.
	var err error
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, native := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeCausalDiagnosis, path)
	if native == nil || len(native.Groups) != 8 || native.OmittedGroups != 0 || len(native.Coverage) != 2 {
		t.Fatalf("native premise did not publish eight groups and two family diagnostics: %+v", native)
	}
	rows, omitted := answerDocIOInFlightMeasurementRows(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) != 10 || omitted != 0 {
		t.Fatalf("single-query roster changed capacity: rows=%d omitted=%d", len(rows), omitted)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	_, handoff, found := strings.Cut(prompt, "#### IO In-Flight Measurements\n")
	if !found {
		t.Fatal("actual initial context lost the eight-group handoff")
	}
	for _, row := range rows {
		line := ioInFlightPublicPromptLine(handoff, row.ID)
		if line == "" || row.SourceRef.Path != path {
			t.Fatalf("single-query source row missing or changed: %+v", row)
		}
		if row.Predicate == "io_inflight" {
			for key, want := range map[string]float64{"peak_requests": 1, "mean_requests": .3, "busy_ms": 3, "request_ms": 3} {
				ioInFlightPublicNumber(t, "eight-group handoff", line, key, want)
			}
		}
	}
}

func ioInFlightPublicContext(t *testing.T, lang string, scope types.RuntimeQuestionScope, path string) (*types.AgentContext, types.ToolResult, *tracequery.IOInFlightStats) {
	t.Helper()
	if path == "" {
		var err error
		path, err = filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_inflight", "events.systrace"))
		if err != nil {
			t.Fatal(err)
		}
	}
	start, end := 1.0, 1.01
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: lang, AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Describe IO in-flight measurements, their scope and limits"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: lang, PerfTrace: &types.PerfBundle{},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 40, Source: "user_request"}},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: scope, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000000..1.010000 seconds"},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("actual TraceQuery failed: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	payloadRef := result.RawRef
	for _, row := range result.Observations {
		if row.SourceRef.PayloadRef != "" {
			payloadRef = row.SourceRef.PayloadRef
			break
		}
	}
	body, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.WindowStats == nil {
		t.Fatal("actual query lost window_stats")
	}
	return ctx, result, payload.WindowStats.IOInFlight
}

func ioInFlightPublicAllPromptRows(prompt, id string) string {
	var rows []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "- `"+id+"`:") || strings.HasPrefix(line, "  - id=`"+id+"`;") {
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n")
}

func ioInFlightPublicPromptLine(prompt, id string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "- `"+id+"`:") {
			return line
		}
	}
	for _, line := range strings.Split(prompt, "\n") {
		// A generic finite-fact card also uses this ID prefix but has no
		// measurement summary. Require the complete handoff row structure.
		if strings.HasPrefix(line, "  - id=`"+id+"`;") && strings.Contains(line, "; summary=") {
			return line
		}
	}
	return ""
}

func ioInFlightPublicNumber(t *testing.T, surface, text, field string, want float64) {
	t.Helper()
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(field) + `=([0-9]+(?:\.[0-9]+)?)`)
	match := pattern.FindStringSubmatch(text)
	if len(match) != 2 {
		t.Errorf("%s lost numeric field %s in %s", surface, field, text)
		return
	}
	got, err := strconv.ParseFloat(match[1], 64)
	if err != nil || math.Abs(got-want) > 1e-7 {
		t.Errorf("%s %s = %s, want %s", surface, field, match[1], fmt.Sprint(want))
	}
}
