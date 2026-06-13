package skill

import (
	"strings"
	"testing"
)

// TestTraceQueryViewTeachings_TableShape locks the shared view-teaching
// table: one row per view, no duplicates, every row carries a when-clause,
// and the three previously-untaught views are present.
func TestTraceQueryViewTeachings_TableShape(t *testing.T) {
	rows := TraceQueryViewTeachings()
	if len(rows) != 17 {
		t.Fatalf("expected 17 trace_query view rows, got %d", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.View == "" {
			t.Fatalf("row with empty view: %+v", row)
		}
		if seen[row.View] {
			t.Fatalf("duplicate view row %q", row.View)
		}
		seen[row.View] = true
		if strings.TrimSpace(row.When) == "" {
			t.Fatalf("view %q has no when-to-use clause", row.View)
		}
	}
	for _, want := range []string{"frame_timeline", "frame_flow", "frame_root_cause_bundle", "evidence_pack"} {
		if !seen[want] {
			t.Fatalf("previously-untaught view %q missing from shared table", want)
		}
	}
}

// TestTraceQueryViewTeachings_EvidencePackDistinguishedFromRecipe pins the
// one-clause distinction: recipe adapts its included views, evidence_pack is
// the fixed bundle.
func TestTraceQueryViewTeachings_EvidencePackDistinguishedFromRecipe(t *testing.T) {
	var recipeWhen, packWhen string
	for _, row := range TraceQueryViewTeachings() {
		switch row.View {
		case "recipe":
			recipeWhen = row.When
		case "evidence_pack":
			packWhen = row.When
		}
	}
	if !strings.Contains(recipeWhen, "adapts") {
		t.Fatalf("recipe when-clause should state the included views adapt: %q", recipeWhen)
	}
	if !strings.Contains(packWhen, "`recipe`") || !strings.Contains(packWhen, "do not adapt") {
		t.Fatalf("evidence_pack when-clause should contrast with recipe: %q", packWhen)
	}
	if !strings.Contains(packWhen, "fixed") {
		t.Fatalf("evidence_pack when-clause should name the fixed bundle: %q", packWhen)
	}
}

// TestRenderTraceQueryViewMatrix_PreservesPinnedPromptPhrases keeps the
// rendered matrix compatible with the prompt phrases that explorer and skill
// tests pin at the consuming sites.
func TestRenderTraceQueryViewMatrix_PreservesPinnedPromptPhrases(t *testing.T) {
	matrix := RenderTraceQueryViewMatrix()
	for _, want := range []string{
		"`state_churn` context",
		"fragmented state-churn causes",
		"`view=\"frame_root_cause_bundle\"`",
		"handoff-safe frame/jank root-cause bundles",
		"structured row lookup",
		"`view=\"event_search\"`",
		"`view=\"evidence_pack\"`",
		", and ",
	} {
		if !strings.Contains(matrix, want) {
			t.Fatalf("rendered view matrix missing %q:\n%s", want, matrix)
		}
	}
	names := RenderTraceQueryViewNameList()
	for _, row := range TraceQueryViewTeachings() {
		if !strings.Contains(names, "`"+row.View+"`") {
			t.Fatalf("compact name list missing %q: %s", row.View, names)
		}
	}
}

// TestExploreSkill_TraceQueryViewListRendersFromSharedTable verifies the
// explore-skill workflow item embeds the shared matrix instead of a
// hand-written copy, so every view (including frame_timeline / frame_flow /
// evidence_pack) is taught.
func TestExploreSkill_TraceQueryViewListRendersFromSharedTable(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	if !strings.Contains(corpus, RenderTraceQueryViewMatrix()) {
		t.Fatalf("explore-skill should embed the shared trace_query view matrix verbatim:\n%s", corpus)
	}
}
