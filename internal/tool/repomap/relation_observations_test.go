package repomap

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func relationObservationFixtureGraph(t *testing.T, root string) *Graph {
	t.Helper()
	files := []*FileInfo{
		{
			RelPath:  "internal/pipeline/analyzer.go",
			Language: LangGo,
			Package:  "pipeline",
			Symbols: []Symbol{
				{Name: "Run", Kind: "function", File: "internal/pipeline/analyzer.go", Line: 10, Exported: true},
			},
			Relations: []Relation{{
				Kind:       "call",
				File:       "internal/pipeline/analyzer.go",
				Line:       12,
				ToEP:       RelationEndpoint{Name: "Process"},
				Confidence: rmtypes.ConfidenceAST,
				Provenance: rmtypes.ProvenanceTreeSitter,
				ResolvedBy: "test_fixture",
			}},
		},
		{
			RelPath:  "internal/pipeline/process.go",
			Language: LangGo,
			Package:  "pipeline",
			Symbols: []Symbol{
				{Name: "Process", Kind: "function", File: "internal/pipeline/process.go", Line: 5, Exported: true},
			},
		},
		{
			RelPath:  "java/app/Child.java",
			Language: LangJava,
			Package:  "app",
			Symbols: []Symbol{
				{Name: "Child", Kind: "class", File: "java/app/Child.java", Line: 3, Exported: true},
			},
			Relations: []Relation{{
				Kind:       "inheritance",
				File:       "java/app/Child.java",
				Line:       3,
				ToEP:       RelationEndpoint{Name: "Base", File: "java/app/Base.java", Line: 2},
				Confidence: rmtypes.ConfidenceAST,
				Provenance: rmtypes.ProvenanceTreeSitter,
				ResolvedBy: "test_fixture",
			}},
		},
		{
			RelPath:  "java/app/Base.java",
			Language: LangJava,
			Package:  "app",
			Symbols: []Symbol{
				{Name: "Base", Kind: "class", File: "java/app/Base.java", Line: 2, Exported: true},
			},
		},
	}
	return BuildGraph(root, files)
}

func executeRelationObservationFixture(t *testing.T, view string) types.ToolResult {
	t.Helper()
	repo := t.TempDir()
	mut := types.NewMutableState("relation observation fixture")
	mut.SetSearchGraph(relationObservationFixtureGraph(t, repo))
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}
	params, err := json.Marshal(map[string]any{
		"path":           ".",
		"view":           view,
		"query":          "Run Child",
		"scopes":         []string{"internal", "java"},
		"relation_kinds": []string{"call", "inheritance"},
		"top_n":          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&RepoMapV2{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("repo_map %s returned error: %v", view, err)
	}
	if !res.Success {
		t.Fatalf("repo_map %s should succeed: %s", view, res.Summary)
	}
	return res
}

// TestRepoMapRelationMapExecuteAttachesTypedObservations pins the H2 producer
// contract through the real Execute path: the graph-backed rows the
// relation_map renderer printed are attached as typed observation rows on the
// ToolResult, classified as index-derived navigation facts (the same family
// the observation ledger assigns repo_map / source-inventory lens facts):
// supporting-coverage role, the canonical soft grounding policy for that
// (origin, role) pair, and never anything that can become a current-source
// citation. The rendered prose itself is unchanged — the typed rows are a
// side-channel, asserted against the exact row lines in the Summary.
func TestRepoMapRelationMapExecuteAttachesTypedObservations(t *testing.T) {
	res := executeRelationObservationFixture(t, "relation_map")

	// Rendered TEXT unchanged: the exact pre-existing relation row lines
	// (including the bracketed diagnostics) are still present verbatim.
	wantLines := []string{
		"call `Run @ internal/pipeline/analyzer.go:10` → Process @ internal/pipeline/process.go:5 — observed @ internal/pipeline/analyzer.go:12 [tree_sitter/test_fixture, confidence=1.00, resolution=typed]",
		"inheritance `Child @ java/app/Child.java:3` → Base @ java/app/Base.java:2 — observed @ java/app/Child.java:3 [tree_sitter/test_fixture, confidence=1.00]",
	}
	for _, want := range wantLines {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("relation_map rendered text drifted, missing %q:\n%s", want, res.Summary)
		}
	}

	if len(res.Observations) != 2 {
		t.Fatalf("expected 2 typed relation observations, got %d: %+v", len(res.Observations), res.Observations)
	}
	wantPolicy := types.AnswerClaimBindingGroundingPolicy(
		types.AnswerEvidenceOriginCrossRepoIndex, types.AnswerAggregateRoleSupportingCoverage)
	if wantPolicy != types.ClaimGroundingSoft {
		t.Fatalf("canonical navigation grounding policy drifted: %q", wantPolicy)
	}
	for _, record := range res.Observations {
		if !strings.HasPrefix(record.ID, "repo_map:") {
			t.Fatalf("typed row ID missing producer namespace: %q", record.ID)
		}
		if record.Origin != types.AnswerEvidenceOriginCrossRepoIndex {
			t.Fatalf("typed row %s origin %q is not the repo_map index origin", record.ID, record.Origin)
		}
		if record.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("typed row %s role %q is not supporting coverage", record.ID, record.Role)
		}
		if record.GroundingPolicy != wantPolicy {
			t.Fatalf("typed row %s grounding policy %q, want %q", record.ID, record.GroundingPolicy, wantPolicy)
		}
		if record.SourceRef.Kind != types.ObservationSourceCrossRepoIndex {
			t.Fatalf("typed row %s source kind %q is not the index namespace", record.ID, record.SourceRef.Kind)
		}
		// Citation red line: a navigation row must never be eligible for the
		// current-source citation lane.
		if record.Origin == types.AnswerEvidenceOriginCurrentSource {
			t.Fatalf("typed row %s declared current-source origin", record.ID)
		}
		if types.ObservationRecordHasCurrentSourceLineSpan(record) {
			t.Fatalf("typed row %s is current-source citation eligible: %+v", record.ID, record)
		}
		if record.Producer != "repo_map" {
			t.Fatalf("typed row %s producer %q", record.ID, record.Producer)
		}
		if strings.TrimSpace(record.Summary) == "" || strings.Contains(record.Summary, "\n") {
			t.Fatalf("typed row %s summary must be one compact line: %q", record.ID, record.Summary)
		}
		if record.ObservedAt == "" {
			t.Fatalf("typed row %s missing observed_at", record.ID)
		}
	}

	byPredicate := map[string]types.ObservationRecord{}
	for _, record := range res.Observations {
		byPredicate[record.Predicate] = record
	}
	call, ok := byPredicate["call"]
	if !ok {
		t.Fatalf("call row missing: %+v", res.Observations)
	}
	if call.Subject != "Run" || call.Object != "Process" {
		t.Fatalf("call row triple drifted: %+v", call)
	}
	if call.SourceRef.Path != "internal/pipeline/analyzer.go" || call.Span.LineStart != 12 {
		t.Fatalf("call row coordinates drifted: %+v", call)
	}
	if call.Confidence != rmtypes.ConfidenceAST {
		t.Fatalf("call row confidence %v, want %v", call.Confidence, rmtypes.ConfidenceAST)
	}
	inh, ok := byPredicate["inheritance"]
	if !ok {
		t.Fatalf("inheritance row missing: %+v", res.Observations)
	}
	if inh.Subject != "Child" || inh.Object != "Base" {
		t.Fatalf("inheritance row triple drifted: %+v", inh)
	}

	// Consumer acceptance: the ledger compiles producer rows of this family
	// directly (unlike current-source rows, which it rejects from this
	// channel) and keeps origin/policy intact.
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{res},
	})
	compiled := 0
	for _, record := range ledger.Records {
		if strings.HasPrefix(record.ID, "repo_map:") && strings.Contains(record.ID, "#relation_map:") {
			compiled++
			if record.Origin != types.AnswerEvidenceOriginCrossRepoIndex {
				t.Fatalf("ledger drifted typed row origin: %+v", record)
			}
			if record.GroundingPolicy != wantPolicy {
				t.Fatalf("ledger drifted typed row grounding policy: %+v", record)
			}
		}
	}
	if compiled != 2 {
		t.Fatalf("ledger compiled %d repo_map relation rows, want 2: %+v", compiled, ledger.Records)
	}
}

// TestRepoMapNonRelationViewsAttachNoTypedObservations pins default
// stability: every view other than relation_map keeps an empty typed
// side-channel (and therefore a byte-identical ToolResult shape).
func TestRepoMapNonRelationViewsAttachNoTypedObservations(t *testing.T) {
	for _, view := range []string{"overview", "file_map", "task_map"} {
		res := executeRelationObservationFixture(t, view)
		if len(res.Observations) != 0 {
			t.Fatalf("view %s attached typed observations: %+v", view, res.Observations)
		}
	}
}

// TestRelationMapTypedObservationsCapDedupAndIdentity pins the builder
// contract: dedup uses the shared typed-relation member key family (same
// kind/source/target/target-file member collapses regardless of observed
// line), identity-incomplete rows are skipped instead of weak-keyed, and
// publication is hard-capped at 50 rows.
func TestRelationMapTypedObservationsCapDedupAndIdentity(t *testing.T) {
	now := time.Now()

	var rows []render.RelationMapRow
	// Three copies of the same member at different observed lines → one row.
	for line := 10; line <= 12; line++ {
		rows = append(rows, render.RelationMapRow{
			Kind: "call", Source: "Run", Target: "Process",
			TargetFile: "b.go", ObservedFile: "a.go", ObservedLine: line,
			Text: fmt.Sprintf("call Run → Process observed a.go:%d", line),
		})
	}
	// Identity-incomplete row (no target name) → skipped, never weak-keyed.
	rows = append(rows, render.RelationMapRow{
		Kind: "call", Source: "Run", ObservedFile: "a.go", ObservedLine: 99,
		Text: "call Run → (unresolved)",
	})
	out := relationMapTypedObservations(rows, "", "render-a", now)
	if len(out) != 1 {
		t.Fatalf("dedup did not collapse same-member rows: %+v", out)
	}
	if out[0].Span.LineStart != 10 {
		t.Fatalf("first rendered copy should win: %+v", out[0])
	}
	wantKey := types.TypedRelationMemberSurfaceKey(
		"call", "Run", types.TypedRelationMember{Name: "Process", File: "b.go"})
	if wantKey == "" {
		t.Fatal("shared member key unexpectedly empty")
	}

	// 60 distinct members → capped at exactly 50, IDs ordinal and unique.
	rows = rows[:0]
	for i := 0; i < 60; i++ {
		rows = append(rows, render.RelationMapRow{
			Kind:   "call",
			Source: fmt.Sprintf("Fn%d", i), Target: fmt.Sprintf("Callee%d", i),
			TargetFile: "b.go", ObservedFile: "a.go", ObservedLine: i + 1,
			Text: fmt.Sprintf("call Fn%d → Callee%d", i, i),
		})
	}
	out = relationMapTypedObservations(rows, "", "render-b", now)
	if len(out) != relationMapObservationRowCap {
		t.Fatalf("cap drifted: got %d rows, want %d", len(out), relationMapObservationRowCap)
	}
	ids := map[string]bool{}
	for _, record := range out {
		if ids[record.ID] {
			t.Fatalf("duplicate typed row ID %q", record.ID)
		}
		ids[record.ID] = true
	}

	// ID namespace is deterministic per rendered output and distinct across
	// different rendered outputs when no blob reference exists.
	again := relationMapTypedObservations(rows, "", "render-b", now)
	if again[0].ID != out[0].ID {
		t.Fatalf("typed row IDs not deterministic: %q vs %q", again[0].ID, out[0].ID)
	}
	other := relationMapTypedObservations(rows, "", "render-c", now)
	if other[0].ID == out[0].ID {
		t.Fatalf("distinct lenses share typed row ID %q", out[0].ID)
	}
}
