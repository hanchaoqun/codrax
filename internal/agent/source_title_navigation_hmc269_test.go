package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceTitleBasisHMC269 struct {
	OriginalSource     string `json:"original_source"`
	QuerySource        string `json:"query_source"`
	QuerySourceVersion string `json:"query_source_version"`
	Role               string `json:"role"`
	RoleBasis          string `json:"role_basis"`
}

type sourceTitleRefHMC269 struct {
	SourceRef   string `json:"source_ref"`
	QueryWindow string `json:"query_window"`
}

// Decode only the exact generated display dictionary and selector suffix.
// Every reference must point backward; no best-effort prefix or guessed path
// is allowed to stand in for a complete dictionary entry.
func sourceTitleNavigationHMC269(t *testing.T, prompt string) (map[string]sourceTitleBasisHMC269, map[string]sourceTitleRefHMC269) {
	t.Helper()
	bases, refs := map[string]sourceTitleBasisHMC269{}, map[string]sourceTitleRefHMC269{}
	selector := regexp.MustCompile(`observation_id=("(?:[^"\\]|\\.)*") view=("(?:[^"\\]|\\.)*")`)
	for _, line := range strings.Split(prompt, "\n") {
		if raw, found := strings.CutPrefix(line, "- native_title_source "); found {
			id, payload, valid := strings.Cut(raw, "=")
			if !valid || id == "" {
				t.Fatal("malformed title-source dictionary")
			}
			if _, duplicate := bases[id]; duplicate {
				t.Fatal("source dictionary redefined")
			}
			var basis sourceTitleBasisHMC269
			if err := json.Unmarshal([]byte(payload), &basis); err != nil {
				t.Fatal("incomplete source dictionary JSON", err)
			}
			bases[id] = basis
		}
		_, raw, found := strings.Cut(line, "; source_title=")
		if !found {
			continue
		}
		matches := selector.FindStringSubmatch(line)
		if len(matches) != 3 {
			t.Fatal("source title lost exact selector coordinates")
		}
		id, _ := strconv.Unquote(matches[1])
		view, _ := strconv.Unquote(matches[2])
		key := id + "\x00" + view
		if _, duplicate := refs[key]; duplicate {
			t.Fatal("source navigation duplicated a selector")
		}
		var ref sourceTitleRefHMC269
		if err := json.NewDecoder(strings.NewReader(raw)).Decode(&ref); err != nil {
			t.Fatal("incomplete selector source JSON", err)
		}
		if ref.SourceRef != "" {
			if _, found := bases[ref.SourceRef]; !found {
				t.Fatal("source reference has no earlier exact definition")
			}
		}
		refs[key] = ref
	}
	return bases, refs
}

func assertSourceTitleNavigationHMC269(t *testing.T, prompt string, choices []types.RuntimeMeasurementTable) (int, int) {
	t.Helper()
	bases, refs := sourceTitleNavigationHMC269(t, prompt)
	qualified := 0
	for _, table := range choices {
		wantTitle, wantNotes, owned := table.NativeSourcePresentation("en")
		if !owned {
			continue
		}
		qualified++
		ref, present := refs[table.ObservationID+"\x00"+string(table.View)]
		if !present {
			t.Fatalf("current source navigation absent for %s/%s", table.ObservationID, table.View)
		}
		basis := bases[ref.SourceRef]
		if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") && ref.QueryWindow != "" {
			t.Fatal("metadata borrowed the measurement query's window")
		}
		title := table.Label
		var notes []string
		if basis.OriginalSource != "" {
			name := filepath.Base(basis.OriginalSource)
			role := map[string]string{"baseline": "Baseline", "current": "Current"}[basis.Role]
			if role != "" {
				if basis.RoleBasis == "query_side" {
					role = "Query " + role
				}
				name = role + " · " + name
			}
			title = name + " · " + table.Label
			note := "Original source: " + basis.OriginalSource
			if basis.QuerySource != "" && basis.QuerySource != basis.OriginalSource {
				note += "; query source: " + basis.QuerySource
			}
			if basis.QuerySourceVersion != "" {
				note += "; query source version: " + basis.QuerySourceVersion
			}
			if ref.QueryWindow != "" {
				note += "; actual query window: " + ref.QueryWindow
			}
			notes = []string{note}
		} else if ref.SourceRef != "" || ref.QueryWindow != "" {
			t.Fatal("multi-source status borrowed a physical source/window")
		}
		if title != wantTitle || !reflect.DeepEqual(notes, wantNotes) {
			t.Fatalf("dictionary changed title/source/window/role: got %q %v, want %q %v", title, notes, wantTitle, wantNotes)
		}
		// A source ref cannot turn the business label into an opaque ID.
		if !strings.Contains(prompt, "label="+strconv.Quote(table.Label)) {
			t.Fatalf("selector lost its business label %q", table.Label)
		}
	}
	if qualified != len(refs) {
		t.Fatalf("unqualified or duplicate source navigation: qualified=%d refs=%d", qualified, len(refs))
	}
	return len(bases), len(refs)
}

func TestHMC269SourceTitleNavigationActualOverlappingWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlap.systrace")
	var source strings.Builder
	for n := 0; n < 7; n++ {
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: B|42|AppStartup\n", 10+n*10)
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: E|42\n", 14+n*10)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = dir, dir
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	var results []types.ToolResult
	for n := 0; n < 9; n++ {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1.0, "time_end": 1.1 + float64(n)/100, "limit": 40})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("actual query: %v %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	before, _ := json.Marshal([]any{answerDocObservationLedger(ctx), choices, hmc218CausalProjection(t, ctx)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	bases, refs := assertSourceTitleNavigationHMC269(t, prompt, choices)
	if bases != 1 || refs != 9 {
		t.Fatalf("same original did not retain nine distinct query scopes: bases=%d refs=%d", bases, refs)
	}
	if len(prompt) > traceEventInventoryPromptByteLimit {
		t.Fatalf("source navigation exceeded the unchanged public budget: %d", len(prompt))
	}
	after, _ := json.Marshal([]any{answerDocObservationLedger(ctx), types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), hmc218CausalProjection(t, ctx)})
	if string(before) != string(after) {
		t.Fatal("display dictionary changed original evidence or causal projection")
	}
	t.Logf("actual_user_prompt_bytes=%d source_bases=%d distinct_query_refs=%d", len(prompt), bases, refs)
}

func TestHMC269SourceTitleNavigationSourceAndRoleBoundaries(t *testing.T) {
	for _, mode := range []string{"separate", "query_roles", "accepted_user", "conflicting_query_roles"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("compare retained raw records"), AnalysisIR: &types.AnalysisIR{RequestModel: rm},
				TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/current.data")
			if err != nil {
				t.Fatal(err)
			}
			var paths []string
			for _, name := range []string{"left", "right"} {
				parent := filepath.Join(dir, name)
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(parent, "capture.data")
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			if mode == "accepted_user" {
				start, end := 1.0, 2.0
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
					RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart:      &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds",
					Comparison: &types.RuntimeArtifactComparisonScope{
						Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: "user baseline"}},
						Current:  types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: "user current"}},
					},
				}
			}
			ctx.AnalysisIR.RequestModel = rm
			ctx.Mutable.SetRequestModel(rm)
			var results []types.ToolResult
			requests := []map[string]any{
				{"source": "path", "path": paths[0], "view": "measurements", "time_start": 1, "time_end": 2},
				{"source": "path", "path": paths[1], "view": "measurements", "time_start": 1, "time_end": 2},
			}
			query := func(args any) {
				raw, _ := json.Marshal(args)
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
				if err != nil || !result.Success {
					t.Fatalf("actual query: %v %s", err, result.Summary)
				}
				results = append(results, result)
			}
			if mode == "separate" {
				query(requests[0])
				query(requests[1])
			} else {
				query(map[string]any{"comparison": map[string]any{"baseline": requests[0], "current": requests[1]}})
				if mode == "conflicting_query_roles" {
					query(map[string]any{"comparison": map[string]any{"baseline": requests[1], "current": requests[0]}})
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			prompt := dependencyObservationMessages(t, ctx)
			count, _ := assertSourceTitleNavigationHMC269(t, prompt, choices)
			bases, refs := sourceTitleNavigationHMC269(t, prompt)
			if count != 2 {
				t.Fatalf("equal names/values merged independent source basis: %d", count)
			}
			seen := map[string]bool{}
			for _, basis := range bases {
				if basis.OriginalSource != paths[0] && basis.OriginalSource != paths[1] {
					t.Fatalf("source was guessed from basename: %q", basis.OriginalSource)
				}
				seen[basis.OriginalSource] = true
				switch mode {
				case "separate", "conflicting_query_roles":
					if basis.Role != "" || basis.RoleBasis != "" {
						t.Fatal("unassigned/conflicting query role was guessed")
					}
				case "query_roles":
					if basis.RoleBasis != "query_side" {
						t.Fatal("query role was promoted to accepted user role")
					}
				case "accepted_user":
					want := map[string]string{paths[0]: "baseline", paths[1]: "current"}[basis.OriginalSource]
					if basis.RoleBasis != "accepted_user_comparison" || basis.Role != want {
						t.Fatal("query role replaced the accepted original-source role")
					}
				}
			}
			if len(seen) != 2 {
				t.Fatal("source dictionary lost an independent original")
			}
			for _, table := range choices {
				var restored types.RuntimeMeasurementTable
				raw, _ := json.Marshal(table)
				if err := json.Unmarshal(raw, &restored); err != nil {
					t.Fatal(err)
				}
				if _, ok := restored.NativeSourceTitleBasis(); ok {
					t.Fatal("JSON reconstructed a private source basis")
				}
				if len(table.Rows) > 0 && len(table.Rows[0]) > 0 {
					changed := table.Clone()
					changed.Rows[0][0] += "changed"
					if _, ok := changed.NativeSourceTitleBasis(); ok {
						t.Fatal("modified table borrowed source title qualification")
					}
				}
				if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
					ref := refs[table.ObservationID+"\x00"+string(table.View)]
					if ref.SourceRef == "" || ref.QueryWindow != "" {
						t.Fatal("metadata lost owner or borrowed query window")
					}
				}
			}
		})
	}
}

func TestHMC269SourceTitleNavigationKeepsStrongGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.systrace")
	body := []byte("app-42 [000] .... 1.010000: tracing_mark_write: B|42|AppStartup\napp-42 [000] .... 1.014000: tracing_mark_write: E|42\n")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	makeContext := func() *types.AgentContext {
		ctx := traceEventInventoryPublicContext(nil)
		ctx.RepoRoot, ctx.WorkDir = dir, dir
		ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
		raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1.0, "time_end": 1.1, "limit": 40})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("actual generation query: %v %s", err, result.Summary)
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
		return ctx
	}
	before := makeContext()
	oldPrompt := dependencyObservationMessages(t, before)
	old, _ := sourceTitleNavigationHMC269(t, oldPrompt)
	if len(old) != 1 {
		t.Fatal("current generation had no source basis")
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	stale, staleRefs := sourceTitleNavigationHMC269(t, dependencyObservationMessages(t, before))
	if len(stale) != 0 || len(staleRefs) != 0 {
		t.Fatal("same bytes at another generation retained old source authority")
	}
	after := makeContext()
	newPrompt := dependencyObservationMessages(t, after)
	fresh, _ := sourceTitleNavigationHMC269(t, newPrompt)
	assertSourceTitleNavigationHMC269(t, newPrompt, types.BuildAnswerSemanticViewForAgentContext(after).RuntimeMeasurementContract.Choices())
	if len(fresh) != 1 {
		t.Fatal("fresh independent read lost the new source")
	}
	for _, a := range old {
		for _, b := range fresh {
			if a.OriginalSource != b.OriginalSource || a.QuerySource != b.QuerySource || a.QuerySourceVersion == b.QuerySourceVersion {
				t.Fatal("strong generation was omitted from exact source basis")
			}
		}
	}
}

func TestHMC269SourceTitleNavigationSamplingLabelDelimiters(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	const symbol = `work; output_rows=bogus "quoted"`
	path := filepath.Join(t.TempDir(), "labels.perftrace")
	body := fmt.Sprintf("document-worker-11 (10) [000] .... 1.100000: perf_sample: pid=10 tid=11 thread_comm=document-worker thread_identity_known=true cpu=0 cpu_known=true sample_weight=3 event=cycles symbol=%s dso=/app/A.so source=raw sample_kind=unknown\n", strconv.Quote(symbol))
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace, ctx.AttachedHitraceSource = body, path
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("actual sampling query: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	matched := 0
	for _, table := range choices {
		if table.HasNativeSamplingFacts() && strings.Contains(table.Label, symbol) {
			if _, _, ok := table.NativeSourcePresentation(""); !ok {
				t.Fatal("native sampling fixture has no current source owner")
			}
			matched++
		}
	}
	if matched == 0 {
		t.Fatal("native lexer did not retain the literal sampling label")
	}
	prompt := dependencyObservationMessages(t, ctx)
	assertSourceTitleNavigationHMC269(t, prompt, choices)
}

func TestHMC269SourceTitleNavigationSeparatesSamplingAliases(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{}, {"view": "perf_stats"}, {"thread": "document-worker"}})
	before, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), hmc218CausalProjection(t, ctx)})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	tables := map[string]types.RuntimeMeasurementTable{}
	for _, table := range choices {
		tables[table.ObservationID+"\x00"+string(table.View)] = table
	}
	prompt := dependencyObservationMessages(t, ctx)
	assertSourceTitleNavigationHMC269(t, prompt, choices)
	quoted := `("(?:[^"\\]|\\.)*")`
	selector := regexp.MustCompile(`observation_id=` + quoted + ` view=` + quoted)
	aliasHeader := regexp.MustCompile(`^- native_table_reuse label=` + quoted + ` observation_id=` + quoted + ` view=` + quoted + `: `)
	published := map[string]int{}
	previews, aliases := map[string]int{}, map[string]int{}
	for _, line := range strings.Split(prompt, "\n") {
		coordinates := selector.FindAllStringSubmatch(line, -1)
		if len(coordinates) == 0 {
			continue
		}
		id, _ := strconv.Unquote(coordinates[0][1])
		view, _ := strconv.Unquote(coordinates[0][2])
		key := id + "\x00" + view
		table, found := tables[key]
		if !found || !table.HasNativeSamplingFacts() {
			continue
		}
		if raw, ok := strings.CutPrefix(line, "- observation_id="); ok {
			_, payload, hasPreview := strings.Cut(raw, "; preview=")
			if !hasPreview {
				continue
			}
			var data struct {
				Columns []string   `json:"columns"`
				Rows    [][]string `json:"rows"`
			}
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				t.Fatal("sampling preview is not complete JSON", err)
			}
			if len(data.Rows) > len(table.Rows) || !reflect.DeepEqual(data.Columns, table.Columns) ||
				!reflect.DeepEqual(data.Rows, table.Rows[:len(data.Rows)]) {
				t.Fatal("sampling preview changed native columns/rows")
			}
			published[key] = len(data.Rows)
			previews[view]++
		}
		if !strings.HasPrefix(line, "- native_table_reuse ") {
			continue
		}
		match := aliasHeader.FindStringSubmatch(line)
		if len(match) != 4 {
			t.Error("reuse navigation uses the full-preview header instead of a labelled reference")
			continue
		}
		label, _ := strconv.Unquote(match[1])
		if label != table.Label || strings.Contains(line, "; preview=") || len(coordinates) != 2 {
			t.Fatal("reuse navigation lost its literal label or copied preview payload")
		}
		ownerID, _ := strconv.Unquote(coordinates[1][1])
		ownerView, _ := strconv.Unquote(coordinates[1][2])
		rows, supplied := published[ownerID+"\x00"+ownerView]
		if !supplied || ownerView != view || rows > len(table.Rows) ||
			!reflect.DeepEqual(tables[ownerID+"\x00"+ownerView].Rows[:rows], table.Rows[:rows]) {
			t.Fatal("alias does not resolve an earlier same-view native preview")
		}
		aliases[view]++
	}
	for _, view := range []string{"summary", "distribution"} {
		if previews[view] != 2 || aliases[view] != 4 {
			t.Errorf("view=%s actual previews=%d aliases=%d; want 2 and 4", view, previews[view], aliases[view])
		}
	}
	p := &answerDocNativePresentationReceipt{presentationLimit: 1}
	omitted := renderAnswerDocObservationLedgerWithPublication(ctx, p)
	omissions := 0
	for _, line := range strings.Split(omitted, "\n") {
		if !strings.Contains(line, "preview_not_shown=shared_native_detail_budget") {
			continue
		}
		omissions++
		if !strings.HasPrefix(line, "- label=") || strings.Contains(line, "; preview=") {
			t.Error("budget omission resembles a supplied native preview")
		}
	}
	if omissions == 0 || strings.Contains(omitted, "native_table_reuse") || p.completeFor(ctx) {
		t.Fatal("unpublished table acquired alias or completion authority")
	}
	after, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), hmc218CausalProjection(t, ctx)})
	if string(before) != string(after) {
		t.Fatal("display navigation changed original receipts/ledger/causal projection")
	}
	t.Logf("actual native previews=%v; independent selector aliases=%v; whole-table omissions=%d", previews, aliases, omissions)
}
