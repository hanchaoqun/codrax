package context_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const artifactDenialCaller = "fscache_page_wait_on_page_bit"
const artifactDenialGhost = "pretriage_only_missing_symbol"

// Exercise the real pre-triage corroboration gate before any trace_query has
// supplied a rescue fact. The raw attachment and a model-authored candidate
// are different authorities even when they happen to contain the same token.
func TestRuntimeArtifactDenialPublicRawBeforeQuery(t *testing.T) {
	bus, _, _ := artifactDenialPublicBus(t, "capture.systrace", artifactDenialFixture(t), 2, "threadpool-400")
	if len(bus.ToolResults) != 0 || len(bus.Mutable.DispatchToolResults()) != 0 {
		t.Fatal("fixture must not have queried the trace before testing raw attachment preservation")
	}
	before := artifactDenialSnapshot(t, []any{bus.AttachedHitrace, bus.AttachedLog, bus.Mutable.PerfTrace(), bus.TypedDenials})
	ac := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	actual := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "explore-code"})
	clean := *ac
	clean.TypedDenials = nil
	baseline := ctxbuilder.BuildPromptContext(&clean, &skill.Config{Name: "explore-code"})
	for _, title := range []string{ctxbuilder.SectionAttachedPerfTrace, ctxbuilder.SectionAttachedRuntimeLog} {
		t.Run(title, func(t *testing.T) {
			want := artifactDenialSection(t, baseline, title)
			if !strings.Contains(want, artifactDenialCaller) || !strings.Contains(want, "artifact-local") {
				t.Fatal("raw fixture must expose the literal caller and its non-source line boundary")
			}
			got := artifactDenialSection(t, actual, title)
			if got != want {
				t.Errorf("source denial rewrote raw artifact content before the first query; section=%s", title)
			}
		})
	}
	candidate := artifactDenialSection(t, actual, ctxbuilder.SectionPerfTriageExtraction)
	if strings.Contains(candidate, artifactDenialGhost) || strings.Contains(candidate, "threadpool-400") || !strings.Contains(candidate, "<unverified-external-source>") {
		t.Errorf("model-authored pre-triage candidate must still be sanitized, not inherit raw attachment authority: %s", candidate)
	}
	artifactDenialAssertSourceRefused(t, bus)
	if after := artifactDenialSnapshot(t, []any{bus.AttachedHitrace, bus.AttachedLog, bus.Mutable.PerfTrace(), bus.TypedDenials}); before != after {
		t.Fatal("prompt assembly changed raw bytes, pre-triage candidates, or repository denials")
	}
}

// Real query facts must retain their own artifact/window/subject identities.
// A caller on the target's dependency is not required to name the target itself.
func TestRuntimeArtifactDenialPublicTypedQueryContext(t *testing.T) {
	base := artifactDenialFixture(t)
	cases := []struct {
		name, file, trace, subject string
		start                      float64
		caller                     bool
	}{
		{"dependency", "capture.systrace", base, "threadpool-400", 2, true},
		{"other_capture_and_subject", "other.systrace", strings.ReplaceAll(base, "threadpool", "worker"), "worker-400", 2, true},
		{"other_window", "later.systrace", strings.ReplaceAll(base, "2.", "3."), "threadpool-400", 3, true},
		{"no_caller", "unknown.systrace", strings.ReplaceAll(base, " caller="+artifactDenialCaller, ""), "threadpool-400", 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus, path, rm := artifactDenialPublicBus(t, tc.file, tc.trace, tc.start, tc.subject)
			var results []types.ToolResult
			for _, view := range []string{"wakeup_chain", "window_stats", "root_cause_rank"} {
				params := artifactDenialJSON(t, map[string]any{"source": "path", "path": path, "view": view, "pid": 100,
					"time_start": tc.start, "time_end": tc.start + .020, "trace_flavor": "harmony_hitrace"})
				result, err := (&tool.TraceQuery{}).Execute(bus, params)
				if err != nil || !result.Success || len(result.Observations) == 0 {
					t.Fatalf("fixture public %s query failed: %v; %s", view, err, result.Summary)
				}
				results = append(results, result)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
			projection := types.CompileTraceCausalProjectionSet(ledger)
			artifactDenialAssertQueryFacts(t, projection, tc.subject, tc.start, tc.caller)
			for _, r := range ledger.Records {
				if !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) {
					continue
				}
				if r.SourceRef.Path != path || !r.SourceRef.QueryWindowKnown ||
					r.SourceRef.QueryWindowStartTs != tc.start || r.SourceRef.QueryWindowEndTs != tc.start+.020 {
					t.Fatalf("fixture query record lost its own capture/query window: %+v", r.SourceRef)
				}
			}
			before := artifactDenialSnapshot(t, []any{results, ledger, projection, rm, bus.Mutable.PerfTrace(), bus.TypedDenials})
			for _, stage := range []types.PipelineStage{types.StageExplore, types.StageFinalize} {
				for _, lang := range []string{"zh", "en"} {
					t.Run(string(stage)+"/"+lang, func(t *testing.T) {
						bus.Language = lang
						ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
						if stage == types.StageExplore {
							ac.AgentName = types.AgentExplorer
						}
						if !strings.Contains(ac.TraceRootCauseBoard, tc.subject) || !strings.Contains(ac.TraceWaitEvidence, tc.subject) {
							t.Fatal("fixture did not reach the native typed board and wait-context producers")
						}
						if strings.Contains(ac.TraceWaitEvidence, artifactDenialCaller) != tc.caller {
							t.Fatal("fixture caller presence must originate in the selected native query, not pre-triage file")
						}
						pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
						for _, section := range []struct{ title, text string }{
							{ctxbuilder.SectionTraceRootCauseBoard, ac.TraceRootCauseBoard},
							{ctxbuilder.SectionTraceWaitEvidence, ac.TraceWaitEvidence},
						} {
							got := artifactDenialSection(t, pc, section.title)
							if got != section.text {
								t.Errorf("source denial rewrote native runtime facts in %s; target=app-100, dependency=%s", section.title, tc.subject)
							}
						}
					})
				}
			}
			afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
			if after := artifactDenialSnapshot(t, []any{results, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), rm, bus.Mutable.PerfTrace(), bus.TypedDenials}); before != after {
				t.Fatal("context rendering changed native values, identities, windows, or source denials")
			}
			artifactDenialAssertSourceRefused(t, bus)
		})
	}
}

func artifactDenialPublicBus(t *testing.T, file, trace string, start float64, dependency string) (*types.BusContext, string, types.RequestModel) {
	t.Helper()
	dir := t.TempDir()
	// TraceQuery resolves symlinks; compare physical identities on macOS too
	// (/var and /private/var refer to the same temporary directory).
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, file)
	for name, contents := range map[string]string{file: trace, artifactDenialCaller: "package example\nfunc unrelated() {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	end := start + .020
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
	}
	mu := types.NewMutableState("Inspect the target's measured waiting and wakeup dependencies.")
	mu.SetRequestModel(rm)
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu, TypedDenials: types.NewTypedDenialSet(),
		AttachedHitrace: trace, AttachedLog: "runtime sample caller=" + artifactDenialCaller + "\n",
		AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}},
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
	}
	params := artifactDenialJSON(t, map[string]any{
		"meta": map[string]any{"source": "hitrace", "summary": "Pre-triage navigation only"},
		"stalls": []map[string]any{
			{"start_ts_ms": (start + .003) * 1000, "duration_ms": 11, "kind": "io", "file": artifactDenialCaller, "symbol": dependency, "line": 8},
			{"start_ts_ms": start * 1000, "duration_ms": 99, "kind": "io", "file": "missing_backend.go", "symbol": artifactDenialGhost, "line": 1},
		},
	})
	result, err := (&tool.EmitPerfTrace{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("fixture pre-triage failed: %v; %s", err, result.Summary)
	}
	perf := mu.PerfTrace()
	if perf == nil || len(perf.Stalls) != 2 || perf.Stalls[0].File != "" || perf.Stalls[1].File != "" || !perf.Stalls[0].IsNavigationOnly() {
		t.Fatalf("fixture must retain candidates while rejecting both real-but-unrelated and missing source files: %+v", perf)
	}
	if !bus.TypedDenials.IsSymbolDenied(dependency) || !bus.TypedDenials.IsSymbolDenied(artifactDenialGhost) {
		t.Fatal("fixture did not create real pre-triage source denials")
	}
	artifactDenialAssertSourceRefused(t, bus)
	return bus, path, rm
}

func artifactDenialAssertSourceRefused(t *testing.T, bus *types.BusContext) {
	t.Helper()
	if !bus.TypedDenials.IsPathDenied(artifactDenialCaller) || !bus.TypedDenials.IsPathDenied("missing_backend.go") {
		t.Fatal("runtime observation must not remove independent source denials")
	}
	result, err := (&tool.ReadFile{}).Execute(bus, artifactDenialJSON(t, map[string]any{"path": artifactDenialCaller}))
	if err != nil || result.Success {
		t.Fatalf("same-name unrelated source read must remain refused: %v; %+v", err, result)
	}
}

func artifactDenialAssertQueryFacts(t *testing.T, set types.TraceCausalProjectionSet, dependency string, start float64, caller bool) {
	t.Helper()
	io, path := false, false
	runnable := map[string]bool{}
	for _, p := range set.Projections {
		if math.Abs(p.WindowStartTs-start) > 1e-9 || math.Abs(p.WindowEndTs-start-.020) > 1e-9 {
			t.Fatal("fixture projection changed the requested window")
		}
		path = path || reflect.DeepEqual(p.WakeupPath, []string{dependency, "network-300", "cookie-200", "app-100"})
		for _, n := range p.RankedSeats {
			if n.Subject == dependency && n.TypeToken == "io_wait" && math.Abs(n.EffectiveImpactMS-11) < 1e-6 {
				io = true
				if (n.BlockedReasonCaller == artifactDenialCaller) != caller {
					t.Fatalf("native caller was invented/lost before the presentation test: %+v", n)
				}
			}
			if n.TypeToken == "priority_inversion_candidate" && math.Abs(n.EffectiveImpactMS-1) < 1e-6 {
				runnable[n.Subject] = true
			}
		}
	}
	if !io || !path || !runnable[dependency] || !runnable["cookie-200"] || !runnable["network-300"] {
		t.Fatalf("native fixture admission failed: IO11=%t path=%t Runnable1=%v", io, path, runnable)
	}
}

func artifactDenialSection(t *testing.T, pc *types.PromptContext, title string) string {
	t.Helper()
	for _, section := range pc.UserSections {
		if section.Title != title {
			continue
		}
		for _, message := range ctxbuilder.ToMessages(pc) {
			if message.Role == "user" && strings.Contains(message.Content, "## "+title+"\n"+section.Content) {
				return section.Content
			}
		}
		t.Fatalf("public ToMessages lost section %s", title)
	}
	t.Fatalf("fixture did not publish section %s", title)
	return ""
}

func artifactDenialFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("fixture HTRACE missing")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok || len(strings.Split(trace, "\n")) != 17 {
		t.Fatal("fixture must retain all 17 physical lines")
	}
	return trace + "\n"
}

func artifactDenialJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func artifactDenialSnapshot(t *testing.T, value any) string {
	t.Helper()
	return fmt.Sprintf("%s", artifactDenialJSON(t, value))
}
