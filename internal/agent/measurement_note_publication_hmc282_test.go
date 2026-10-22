package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Parse actual prompt navigation, not receipt internals. Definitions must
// precede each reference, and a selector may reference only its own notes.
func hmc282SelectorNotes(t *testing.T, text string) map[string][]string {
	t.Helper()
	definitions := map[string]string{}
	out := map[string][]string{}
	key := ""
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "- native_measurement_note "); ok {
			id, value, found := strings.Cut(rest, "=")
			var note string
			if !found || json.Unmarshal([]byte(value), &note) != nil {
				t.Fatalf("invalid note definition: %s", line)
			}
			definitions[id] = note
		}
		if strings.HasPrefix(line, "- observation_id=") {
			var id, view string
			if _, err := fmt.Sscanf(line, "- observation_id=%q view=%q", &id, &view); err != nil {
				t.Fatal(err)
			}
			key = id + "/" + view
		} else if strings.HasPrefix(line, "- native_table_reuse ") {
			key = "" // Aliases are independently checked against their representative.
		}
		for _, prefix := range []string{"  scope_refs=", "  boundary_refs=", "  additional_note_refs="} {
			if raw, found := strings.CutPrefix(line, prefix); found && key != "" {
				var refs []string
				if err := json.Unmarshal([]byte(raw), &refs); err != nil {
					t.Fatal(err)
				}
				for _, ref := range refs {
					note, exists := definitions[ref]
					if !exists {
						t.Fatalf("selector %s used undefined note %s", key, ref)
					}
					out[key] = append(out[key], note)
				}
			}
		}
	}
	return out
}

func hmc282AcceptNativeNotes(t *testing.T, ctx *types.AgentContext, selections []types.AnswerRuntimeMeasurementReceipt) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"reason": "HMC282_LIFECYCLE_ONLY", "confidence": "high", "result_kind": "resolved",
		"native_fact_handoff": map[string]any{
			"runtime_measurements": selections, "interpretation_role": "independent_analysis",
			"interpretation_units": []types.InvestigationInterpretationUnit{{Text: "HMC282_INDEPENDENT_CHAIN_BUSINESS_CLUE", Boundaries: []string{"Not verified as a causal relation."}}},
		},
	})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatalf("public handoff was not accepted: %v %s", err, r.Summary)
	}
}

func TestHMC282NativeNotesPublicFinalizerExactReuse(t *testing.T) {
	ctx, result, _ := schedulerMeasurementPublicContext(t, "")
	before, _ := json.Marshal(result)
	causal := hmc218CausalProjection(t, ctx)
	choices := schedulerMeasurementPublicChoices(t, ctx, result)
	scheduler := map[string]bool{}
	for _, r := range result.Observations {
		if r.Predicate == "scheduler_concurrency" {
			scheduler[r.ID] = true
		}
	}
	var selections []types.AnswerRuntimeMeasurementReceipt
	for _, table := range choices {
		if scheduler[table.ObservationID] {
			selections = append(selections, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		}
	}
	if len(selections) != 8 {
		t.Fatalf("public fixture lost its independent scheduler views: %d", len(selections))
	}
	hmc282AcceptNativeNotes(t, ctx, selections)
	prompt := dependencyObservationMessages(t, ctx.ShallowClone())
	notes := hmc282SelectorNotes(t, prompt)
	for _, table := range choices {
		if !scheduler[table.ObservationID] {
			continue
		}
		got := notes[table.ObservationID+"/"+string(table.View)]
		for _, expected := range table.Notes {
			if !containsExactHMC282(got, expected) {
				t.Errorf("actual Finalizer selector %s/%s did not bind its already published note %q", table.ObservationID, table.View, expected)
			}
		}
	}
	if strings.Contains(prompt, "Other selected views remain partial or absent") || strings.Contains(prompt, "Native reference publication: 2 of 8") {
		t.Error("complete rows and exact reusable notes still reported as partial views")
	}
	if !strings.Contains(prompt, "HMC282_INDEPENDENT_CHAIN_BUSINESS_CLUE") || strings.Contains(prompt, "HMC282_LIFECYCLE_ONLY") {
		t.Error("note reuse changed independent interpretation ownership")
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed original evidence or Trace causal projection")
	}
}

func containsExactHMC282(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestHMC282NativeNotesSeparateCoverageAndBudget(t *testing.T) {
	for _, mode := range []string{"navigation_exhausted", "not_presented", "wrong_window", "wrong_source"} {
		t.Run(mode, func(t *testing.T) {
			ctx, result, _ := schedulerMeasurementPublicContext(t, "")
			choices := schedulerMeasurementPublicChoices(t, ctx, result)
			var selected types.RuntimeMeasurementTable
			for _, table := range choices {
				if table.View != types.RuntimeMeasurementTimeline {
					continue
				}
				isScheduler := false
				for _, r := range result.Observations {
					isScheduler = isScheduler || r.ID == table.ObservationID && r.Predicate == "scheduler_concurrency"
				}
				if isScheduler {
					selected = table
					break
				}
			}
			if selected.ObservationID == "" {
				t.Fatal("missing real target view")
			}
			hmc282AcceptNativeNotes(t, ctx, []types.AnswerRuntimeMeasurementReceipt{{ObservationID: selected.ObservationID, View: selected.View}})
			p := &answerDocNativePresentationReceipt{}
			if mode == "navigation_exhausted" {
				p.navigationBytes = 16 << 10
			}
			if mode != "not_presented" {
				renderAnswerDocRuntimeMeasurementChoices(ctx, p)
			}
			if mode == "wrong_window" {
				start, end := 5.0, 6.0
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5..6"}
			}
			if mode == "wrong_source" {
				ta := ctx.Mutable.TurnAArtifacts()
				for i := range ta.ToolResults {
					for j := range ta.ToolResults[i].Observations {
						ta.ToolResults[i].Observations[j].SourceRef.Path += "-other-source"
					}
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(*ta)
			}
			projection := answerDocSeparatedNativeFactHandoff(ctx.ShallowClone(), p)
			if mode == "wrong_window" || mode == "wrong_source" {
				if projection.ownsCurrentNarrative {
					t.Fatal("previous display receipt survived a source/window change")
				}
				return
			}
			if projection.nativeViewsComplete() || p.measurementViews[nativeFactHandoffViewKey(selected)] {
				t.Fatal("partial or missing presentation signed full view completion")
			}
			if mode == "navigation_exhausted" {
				if !strings.Contains(projection.text, "1 of 1 selected views have all retained rows represented") || !strings.Contains(projection.text, "0 of 1 have all producer notes represented") {
					t.Fatalf("complete rows versus incomplete notes not disclosed: %s", projection.text)
				}
			}
			if p.navigationBytes > 16<<10 || !strings.Contains(projection.text, "HMC282_INDEPENDENT_CHAIN_BUSINESS_CLUE") {
				t.Fatal("navigation budget or independent business interpretation changed")
			}
		})
	}
}

func TestHMC282NativeNoteReferencesDoNotBorrowSourceOrWindow(t *testing.T) {
	for _, differentSource := range []bool{false, true} {
		first, second := hmc275IOPath(t), ""
		second, end := first, 1.009
		if differentSource {
			second, end = hmc275IOPath(t), 1.01
		}
		ctx, _ := hmc275IOContext(t, []string{first, second}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {"time_start": 1.0, "time_end": end}})
		prompt := dependencyObservationMessages(t, ctx)
		bound := hmc282SelectorNotes(t, prompt)
		for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
			got := bound[table.ObservationID+"/"+string(table.View)]
			if len(got) < 2 || !reflect.DeepEqual(got[:2], table.Notes[:2]) {
				t.Fatal("selector lost its precise original source/window notes")
			}
			for _, note := range got {
				if !containsExactHMC282(table.Notes, note) {
					t.Fatal("another selector lent a note absent from this original table")
				}
			}
		}
		rows := 0
		for _, preview := range hmc275MessagePreviews(t, prompt) {
			rows += len(preview.Rows)
		}
		if rows > 128 || strings.Contains(prompt, "native_table_reuse") {
			t.Fatal("note reuse enlarged row budget or merged independent source/query previews")
		}
	}
}

func TestHMC282NativeNoteReuseRequiresExistingExactDefinitionsAndAtomicBudget(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		var b strings.Builder
		navigation := runtimeMeasurementNoteNavigation{}
		original := []string{"", "same text", " line\n\"quoted\" ", "another source/window only"}
		navigation.references(&b, original)
		definitions := b.String()
		// The last dictionary note is not part of this selector. Similar text
		// with different whitespace is not an exact source note either.
		own := []string{"", "same text ", " line\n\"quoted\" ", "unpublished", " line\n\"quoted\" "}
		p := &answerDocNativePresentationReceipt{}
		if exhausted {
			p.navigationBytes = 16 << 10
		}
		written := map[string]bool{"": true}
		published := navigation.publishExisting(&b, own, written, p)
		if exhausted {
			if len(published) != 0 || b.String() != definitions || p.navigationOmitted != 1 {
				t.Fatal("failed atomic reference write published a partial note or invented its receipt")
			}
			continue
		}
		if !reflect.DeepEqual(published, []string{" line\n\"quoted\" "}) {
			t.Fatalf("borrowed another selector's note, normalized bytes or invented a definition: %#v", published)
		}
		if len(navigation) != len(original) || len(written) != 1 || p.navigationBytes != len(b.String())-len(definitions) {
			t.Fatal("reference reuse mutated definitions/receipt or escaped shared navigation accounting")
		}
	}
}

func TestHMC282NativeNotesAliasInheritsOnlyActualComponents(t *testing.T) {
	var trace strings.Builder
	trace.WriteString("# tracer: nop\n")
	for round := 0; round < 10; round++ {
		for device := 0; device < 8; device++ {
			start := 1 + float64(round)*.0008 + float64(device)*.00002
			sector := 1000 + round*100 + device*8
			fmt.Fprintf(&trace, "reader-40 (40) [001] .... %.6f: block_rq_issue: 8,%d R 4096 () %d + 8 [reader]\n", start, device, sector)
			fmt.Fprintf(&trace, "irq-80 (2) [001] .... %.6f: block_rq_complete: 8,%d R () %d + 8 [0]\n", start+.00001, device, sector)
		}
	}
	path := filepath.Join(t.TempDir(), "many-requests.systrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _ := hmc275IOContext(t, []string{path, path}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {}})
	choices, _ := runtimeMeasurementHandoffRoster(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), 32)
	for _, exhausted := range []bool{false, true} {
		p := &answerDocNativePresentationReceipt{}
		if exhausted {
			p.navigationBytes = 16 << 10
		}
		text := renderAnswerDocRuntimeMeasurementChoices(ctx, p)
		keys := runtimeMeasurementPreviewIdentities(ctx, choices, p)
		seen := map[string]int{}
		aliases, partialRows := 0, 0
		for i, table := range choices {
			if keys[i] == "" {
				continue
			}
			prior, found := seen[keys[i]]
			if !found {
				seen[keys[i]] = i
				continue
			}
			aliases++
			shown := strings.Contains(text, fmt.Sprintf("native_table_reuse label=%q observation_id=%q view=%q:", table.Label, table.ObservationID, table.View))
			want := answerDocMeasurementViewPresentation{}
			if shown {
				want = p.measurementViewPresentation(choices[prior])
				if !want.rowsComplete {
					partialRows++
				}
			}
			got := p.measurementViewPresentation(table)
			if got != want || p.measurementViews[nativeFactHandoffViewKey(table)] != (got.rowsComplete && got.notesComplete) {
				t.Fatalf("alias broadened actual publication: shown=%t got=%+v want=%+v", shown, got, want)
			}
		}
		if aliases == 0 || !exhausted && partialRows == 0 || p.navigationBytes > 16<<10 {
			t.Fatalf("missing alias/budget witness: aliases=%d partial=%d", aliases, partialRows)
		}
	}
}

func TestHMC282NativeSamplingPartialRowsRetainAllAtomicNotes(t *testing.T) {
	ctx, first := samplingFactPublicationContextHMC243(t)
	results := []types.ToolResult{first}
	for i := 1; i < 8; i++ {
		params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "window_stats", "pid": 11, "time_start": 1 + float64(i)/1000, "time_end": 2})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !r.Success {
			t.Fatalf("public sampling query: %v / %s", err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	p := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocRuntimeMeasurementChoices(ctx, p)
	previews := hmc275MessagePreviews(t, text)
	partial := 0
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if !table.HasNativeSamplingFacts() {
			continue
		}
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		for header, preview := range previews {
			if !strings.Contains(header, selector) || len(preview.Rows) >= len(table.Rows) {
				continue
			}
			partial++
			for _, note := range table.Notes {
				if !strings.Contains(text, "  boundary: "+note+"\n") {
					t.Fatal("atomic sampling preview lost a producer note")
				}
			}
			got := p.measurementViewPresentation(table)
			if got.rowsComplete || !got.notesComplete || p.measurementViews[nativeFactHandoffViewKey(table)] {
				t.Fatalf("partial sampling rows were confused with full atomic notes: %+v", got)
			}
		}
	}
	if partial == 0 {
		t.Fatal("public sampling fixture lacks a shown partial-row preview")
	}
}

func TestHMC282NativeNotesComponentsRemainConservativeAndDispatchLocal(t *testing.T) {
	ctx, result, _ := schedulerMeasurementPublicContext(t, "")
	table := schedulerMeasurementPublicChoices(t, ctx, result)[0]
	for _, first := range []answerDocMeasurementViewPresentation{{true, false}, {false, true}, {false, false}} {
		p := &answerDocNativePresentationReceipt{}
		p.observeMeasurementViewDetails(table, first.rowsComplete, first.notesComplete)
		p.observeMeasurementViewDetails(table, true, true)
		if got := p.measurementViewPresentation(table); got != first || p.measurementViews[nativeFactHandoffViewKey(table)] {
			t.Fatalf("a later complete view retroactively authorized a partial publication: %+v %+v", got, first)
		}
		other := table.Clone()
		other.Notes = append(other.Notes, "a changed source/window note")
		if got := p.measurementViewPresentation(other); got != (answerDocMeasurementViewPresentation{}) {
			t.Fatal("component receipt was keyed by selector instead of exact table")
		}
	}
	p := &answerDocNativePresentationReceipt{}
	p.observeMeasurementViewDetails(table, true, true)
	p.observeMeasurementView(table, false)
	if got := p.measurementViewPresentation(table); got != (answerDocMeasurementViewPresentation{}) {
		t.Fatal("later failed alias/navigation left optimistic diagnostic components")
	}
	if got := (&answerDocNativePresentationReceipt{}).measurementViewPresentation(table); got != (answerDocMeasurementViewPresentation{}) {
		t.Fatal("fresh dispatch inherited previous publication")
	}
}
