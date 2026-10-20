package tool

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// All credentials come from real preparation and TraceQuery. The reviewer
// never manufactures a source title, native seal, or table publication.
func sourceTitleReviewBlocksHMC269(table types.RuntimeMeasurementTable, kind, title string) []any {
	block := rawReviewSelectorHMC268(table, kind)
	if title != "" {
		block["title"] = title
	}
	if kind == "summary" {
		return []any{block}
	}
	return []any{
		map[string]any{"id": "lead", "kind": "summary", "text": "Recorded facts retain their original source and unknown physical meaning."},
		block,
	}
}

func TestHMC269ReviewSourceOwnedTitlesAreStructural(t *testing.T) {
	for _, kind := range []string{"summary", "table", "diagram"} {
		t.Run(kind, func(t *testing.T) {
			bus, _, _ := rawReviewContextHMC268(t, "current")
			members := rawReviewMembersHMC268(t, rawReviewChoicesHMC268(t, bus))
			positive := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": sourceTitleReviewBlocksHMC269(members, kind, "")}, false)
			if !positive.Success {
				t.Fatalf("real selector positive failed before title negative: %s", positive.Summary)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			// Both plausible and misleading titles conflict structurally. No
			// special baseline/current or prose-keyword matcher is acceptable.
			for _, title := range []string{"Current recorded measurements", "Other capture's values"} {
				t.Run(title, func(t *testing.T) {
					result := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": sourceTitleReviewBlocksHMC269(members, kind, title)}, false)
					if result.Success {
						t.Fatal("source-owned native selector accepted a competing model title")
					}
					after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
					if string(after) != string(before) {
						t.Fatal("rejected competing title changed the accepted document")
					}
				})
			}
		})
	}
}

func TestHMC269ReviewOriginalSourceIsSharedAcrossNativeSurfaces(t *testing.T) {
	bus, _, path := rawReviewContextHMC268(t, "current")
	choices := rawReviewChoicesHMC268(t, bus)
	members := rawReviewMembersHMC268(t, choices)
	var metadata types.RuntimeMeasurementTable
	for _, table := range choices {
		if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			metadata = table
		}
	}
	if metadata.ObservationID == "" {
		t.Fatal("real metadata positive missing")
	}
	for _, table := range []types.RuntimeMeasurementTable{members, metadata} {
		for _, kind := range []string{"summary", "table", "diagram"} {
			t.Run(table.ObservationID+"/"+kind, func(t *testing.T) {
				result := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": sourceTitleReviewBlocksHMC269(table, kind, "")}, false)
				if !result.Success {
					t.Fatal(result.Summary)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				block := blockByID(t, doc, "facts")
				if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() {
					t.Fatal("real current table did not bind")
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
				if !strings.Contains(visible, path) {
					t.Fatal("native surface lost its verified original path; derived capture basename alone is not source identity")
				}
				if strings.Contains(visible, "-->") || strings.Contains(visible, "->>") {
					t.Fatal("source presentation invented a relationship edge")
				}
			})
		}
	}
}

func TestHMC269ReviewSourceTitleRecoveryNeedsCurrentOriginal(t *testing.T) {
	bus, _, path := rawReviewContextHMC268(t, "current")
	members := rawReviewMembersHMC268(t, rawReviewChoicesHMC268(t, bus))
	result := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": sourceTitleReviewBlocksHMC269(members, "table", "")}, false)
	if !result.Success {
		t.Fatal(result.Summary)
	}
	wire, err := json.Marshal(bus.Mutable.AnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	restore := func() *types.AnswerDocumentV2 {
		t.Helper()
		var doc types.AnswerDocumentV2
		if err := json.Unmarshal(wire, &doc); err != nil {
			t.Fatal(err)
		}
		if blockByID(t, &doc, "facts").RuntimeMeasurement.IsBound() {
			t.Fatal("JSON restored private native source authority")
		}
		return &doc
	}
	current := restore()
	if !types.RebindRuntimeAnswerReceipts(current, types.BuildAnswerSemanticViewForBusContext(bus)) {
		t.Fatal("current original could not restore the legitimate exact selector")
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(current, "zh"))
	if !strings.Contains(visible, path) {
		t.Error("current rebind did not restore the source-owned original title")
	}
	// Same bytes in a different inode are a different source generation.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".replacement", body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".replacement", path); err != nil {
		t.Fatal(err)
	}
	stale := restore()
	if types.RebindRuntimeAnswerReceipts(stale, types.BuildAnswerSemanticViewForBusContext(bus)) {
		t.Fatal("same-name/same-byte replacement restored stale source title authority")
	}
	if blockByID(t, stale, "facts").RuntimeMeasurement.IsBound() {
		t.Fatal("failed rebind partially published stale native facts")
	}
}

func sourceTitleReviewPairHMC269(t *testing.T, pair, userRoles bool) (*types.BusContext, []string) {
	t.Helper()
	bus, _, _ := hmc17NamedPathContext(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/current.data")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(bus.RepoRoot, "one", "capture.data"), filepath.Join(bus.RepoRoot, "two", "capture.data")}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	start, end := 1.0, 2.0
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.Language = "en"
	rm.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds",
	}
	if userRoles {
		rm.RuntimeArtifactScopeProfile.Comparison = &types.RuntimeArtifactComparisonScope{
			Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: "first capture"}},
			Current:  types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: "second capture"}},
		}
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	bus.Mutable.SetRequestModel(rm)
	requests := []map[string]any{
		{"source": "path", "path": paths[0], "view": "measurements", "time_start": 1, "time_end": 2},
		{"source": "path", "path": paths[1], "view": "measurements", "time_start": 1, "time_end": 2},
	}
	var results []types.ToolResult
	if pair {
		results = append(results, hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{"baseline": requests[0], "current": requests[1]}}))
	} else {
		for _, request := range requests {
			results = append(results, hmc17NamedQuery(t, bus, request))
		}
	}
	for _, result := range results {
		if !result.Success {
			t.Fatal(result.Summary)
		}
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return bus, paths
}

func TestHMC269ReviewEqualNamedSourcesKeepIndependentRoles(t *testing.T) {
	for _, pair := range []bool{false, true} {
		for _, userRoles := range []bool{false, true} {
			name := map[bool]string{false: "separate", true: "combined"}[pair] + "/" + map[bool]string{false: "query_only", true: "accepted_user"}[userRoles]
			t.Run(name, func(t *testing.T) {
				bus, paths := sourceTitleReviewPairHMC269(t, pair, userRoles)
				var tables []types.RuntimeMeasurementTable
				for _, table := range rawReviewChoicesHMC268(t, bus) {
					if table.View == types.RuntimeMeasurementMembers {
						tables = append(tables, table)
					}
				}
				if len(tables) != 2 || tables[0].ObservationID == tables[1].ObservationID || len(tables[0].Rows) != 8 || len(tables[1].Rows) != 8 {
					t.Fatalf("same basename/value fixture did not retain two independent exact populations: %d tables", len(tables))
				}
				for i := range tables[0].Rows {
					// Column zero is intentionally the source-owned row identity,
					// not a measured value. Equal source bytes must not erase it.
					if tables[0].Rows[i][0] == tables[1].Rows[i][0] || !reflect.DeepEqual(tables[0].Rows[i][1:], tables[1].Rows[i][1:]) {
						t.Fatalf("row %d lacks distinct source identity or equal actual fields", i)
					}
				}
				seen := map[string]bool{}
				for _, table := range tables {
					title, notes, ok := table.NativeSourcePresentation("en")
					if !ok {
						t.Fatal("real current source lacks its private presentation")
					}
					side := -1
					for i, path := range paths {
						if strings.Contains(strings.Join(notes, "\n"), "Original source: "+path+"; ") {
							side = i
							seen[path] = true
						}
					}
					if side < 0 {
						t.Fatalf("full source identity lost: %v", notes)
					}
					role := []string{"Baseline", "Current"}[side]
					switch {
					case userRoles:
						if !strings.HasPrefix(title, role+" · ") {
							t.Fatalf("accepted original-source role did not survive %s: %q", name, title)
						}
					case pair:
						if !strings.HasPrefix(title, "Query "+role+" · ") {
							t.Fatalf("query-side role was lost or promoted to user role: %q", title)
						}
					default:
						if !strings.HasPrefix(title, "capture.data · ") {
							t.Fatalf("separate unassigned query invented a comparison role: %q", title)
						}
					}
					for _, kind := range []string{"summary", "table", "diagram"} {
						out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": sourceTitleReviewBlocksHMC269(table, kind, "")}, false)
						if !out.Success {
							t.Fatal(out.Summary)
						}
						visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "en"))
						if !strings.Contains(visible, title) || !strings.Contains(visible, paths[side]) {
							t.Fatalf("%s did not reuse exact source presentation", kind)
						}
					}
				}
				if len(seen) != 2 {
					t.Fatal("equal names/values collapsed independent source identity")
				}
			})
		}
	}
}

func TestHMC269ReviewConflictingQueryRolesKeepSourceWithoutRole(t *testing.T) {
	bus, paths := sourceTitleReviewPairHMC269(t, true, false)
	previous := bus.Mutable.TurnAArtifacts().ToolResults
	reversed := hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{
		"baseline": map[string]any{"source": "path", "path": paths[1], "view": "measurements", "time_start": 1, "time_end": 2},
		"current":  map[string]any{"source": "path", "path": paths[0], "view": "measurements", "time_start": 1, "time_end": 2},
	}})
	if !reversed.Success {
		t.Fatal(reversed.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: append(previous, reversed)})
	count := 0
	for _, table := range rawReviewChoicesHMC268(t, bus) {
		if table.View != types.RuntimeMeasurementMembers {
			continue
		}
		count++
		title, notes, ok := table.NativeSourcePresentation("en")
		if !ok || !strings.HasPrefix(title, "capture.data · ") || len(notes) == 0 {
			t.Fatalf("conflicting query roles were guessed or source omitted: %q / %v / %t", title, notes, ok)
		}
	}
	if count != 2 {
		t.Fatalf("role conflict removed legitimate same-publication facts: %d", count)
	}
}

func TestHMC269ReviewTitleSchemaMatchesAdapterAndLegacy(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	table := rawReviewMembersHMC268(t, view.RuntimeMeasurementContract.Choices())
	for _, patch := range []bool{false, true} {
		for _, title := range []string{"", "independent competing heading"} {
			schema, field := BuildAnswerDocumentParametersFor(view), "blocks"
			if patch {
				schema, field = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
			}
			payload := map[string]any{field: sourceTitleReviewBlocksHMC269(table, "table", title)}
			raw, _ := json.Marshal(payload)
			err := toolparam.Validate(raw, schema)
			if (err == nil) != (title == "") {
				t.Fatalf("schema title contract differs patch=%t title=%q err=%v", patch, title, err)
			}
		}
	}
	// Ordinary prose remains author-owned, while legacy unqualified measured
	// projections do not acquire the new private source-heading contract.
	legacy := runtimeMeasurementTestView()
	for _, view := range []*types.AnswerSemanticView{view, legacy} {
		raw := []byte(`{"blocks":[{"id":"lead","kind":"summary","title":"Business explanation","text":"The records alone do not prove causality."}]}`)
		if err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(view)); err != nil {
			t.Fatal("source-title contract spilled into ordinary explanation", err)
		}
	}
	payload := []byte(`{"blocks":[{"id":"m","kind":"table","title":"Legacy measured facts","runtime_measurement":{"observation_id":"source-A:window-1:block:R","view":"summary"}}]}`)
	if err := toolparam.Validate(payload, BuildAnswerDocumentParametersFor(legacy)); err != nil {
		t.Fatal("unqualified legacy source gained the new title prohibition", err)
	}
	// A separate legacy-only schema cannot detect a conditional that silently
	// matches every selector. Exercise both qualifications in one dispatch.
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, legacy.RuntimeMeasurementContract.Tables...)
	for _, patch := range []bool{false, true} {
		schema, field := BuildAnswerDocumentParametersFor(view), "blocks"
		if patch {
			schema, field = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
		}
		for _, choice := range []types.RuntimeMeasurementTable{table, legacy.RuntimeMeasurementContract.Tables[0]} {
			block := rawReviewSelectorHMC268(choice, "table")
			block["title"] = "Ordinary measured heading"
			raw, _ := json.Marshal(map[string]any{field: []any{block}})
			err := toolparam.Validate(raw, schema)
			wantAllowed := choice.ObservationID != table.ObservationID
			if (err == nil) != wantAllowed {
				t.Fatalf("mixed-dispatch schema applied source qualification globally: patch=%t id=%s err=%v", patch, choice.ObservationID, err)
			}
			var wire emitAnswerBlockV2
			blockJSON, _ := json.Marshal(block)
			if err := json.Unmarshal(blockJSON, &wire); err != nil {
				t.Fatal(err)
			}
			typed, err := NormalizeEmitAnswerBlock(wire, field+"[0]")
			if err != nil || runtimeMeasurementBindingValid(typed, view) != wantAllowed {
				t.Fatalf("mixed-dispatch executable binding disagrees with exact source qualification: %v", err)
			}
		}
	}
}
