package agent

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestParseEvidenceItems(t *testing.T) {
	notes := []string{
		"## Evidence from [internal/agent/explorer.go]\n" +
			"- [DIRECT] `Name()` line 12: returns \"explorer\"\n" +
			"- [CONDITIONAL] `Register()` line 20: dispatches IF `Enabled()` == true\n" +
			"- [RELATIONSHIP] `Register` -> `NewExplorer`: calls constructor\n" +
			"- [ABSENT] Expected `LegacyHandler` in registry but NOT found",
	}

	items := parseEvidenceItems(notes, "explorer.llm")
	if len(items) != 4 {
		t.Fatalf("parseEvidenceItems count = %d, want 4", len(items))
	}

	if items[0].Source != "internal/agent/explorer.go" {
		t.Fatalf("first evidence source = %q", items[0].Source)
	}

	var sawConditional bool
	var sawRelationship bool
	var sawAbsent bool
	for _, item := range items {
		switch item.Kind {
		case types.EvidenceConditional:
			sawConditional = item.Condition != ""
		case types.EvidenceRelationship:
			sawRelationship = item.Subject != "" && item.Object != ""
		case types.EvidenceAbsent:
			sawAbsent = item.Predicate == "absent"
		}
	}
	if !sawConditional {
		t.Fatal("expected parsed conditional evidence with condition text")
	}
	if !sawRelationship {
		t.Fatal("expected parsed relationship evidence with subject/object")
	}
	if !sawAbsent {
		t.Fatal("expected parsed absent evidence")
	}
}

func TestParseEvidenceItems_BareTagLinesWithoutBullet(t *testing.T) {
	notes := []string{
		"[DIRECT] `Name()` line 12: returns \"explorer\"\n" +
			"[CONDITIONAL] `Register()` line 20: dispatches IF `Enabled()` == true",
	}

	items := parseEvidenceItems(notes, "explorer.llm")
	if len(items) != 2 {
		t.Fatalf("parseEvidenceItems bare-tag count = %d, want 2", len(items))
	}
	if items[0].Kind != types.EvidenceDirect {
		t.Fatalf("first bare-tag kind = %q, want %q", items[0].Kind, types.EvidenceDirect)
	}
	if items[1].Kind != types.EvidenceConditional {
		t.Fatalf("second bare-tag kind = %q, want %q", items[1].Kind, types.EvidenceConditional)
	}
}

func TestMergeEvidenceItemsDedupesByStableID(t *testing.T) {
	a := types.EvidenceItem{
		Kind:       types.EvidenceConcrete,
		Subject:    "Handler.Name",
		Predicate:  "returns",
		Object:     "\"explorer\"",
		Source:     "internal/agent/explorer.go",
		LineStart:  12,
		LineEnd:    12,
		Confidence: 0.7,
	}
	a.ID = types.StableEvidenceID(a.Kind, a.Subject, a.Predicate, a.Object, a.Condition, a.Source, a.LineStart, a.LineEnd)
	b := a
	b.Confidence = 0.9
	b.EvidenceRef = "blob://trace/result.txt"

	merged := mergeEvidenceItems([]types.EvidenceItem{a}, []types.EvidenceItem{b})
	if len(merged) != 1 {
		t.Fatalf("mergeEvidenceItems count = %d, want 1", len(merged))
	}
	if merged[0].Confidence != 0.9 {
		t.Fatalf("merged confidence = %.2f, want 0.9", merged[0].Confidence)
	}
	if merged[0].EvidenceRef != "blob://trace/result.txt" {
		t.Fatalf("merged evidence ref = %q", merged[0].EvidenceRef)
	}
}

func TestEntityHitsBoundsShortGenericEntities(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		entity   string
		want     bool
	}{
		{
			name:     "agent does not match subagent",
			haystack: normalizeEntityHaystack("subagent"),
			entity:   "agent",
			want:     false,
		},
		{
			name:     "handler does not match routehandler",
			haystack: normalizeEntityHaystack("routehandler"),
			entity:   "handler",
			want:     false,
		},
		{
			name:     "agent matches standalone token",
			haystack: normalizeEntityHaystack("base agent runtime"),
			entity:   "agent",
			want:     true,
		},
		{
			name:     "agent matches simple plural",
			haystack: normalizeEntityHaystack("agents coordinate"),
			entity:   "agent",
			want:     true,
		},
		{
			name:     "compound entity keeps substring behavior",
			haystack: normalizeEntityHaystack("sub_agents runtime"),
			entity:   "subagent",
			want:     true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := entityHits(c.haystack, c.entity); got != c.want {
				t.Fatalf("entityHits(%q, %q) = %v, want %v", c.haystack, c.entity, got, c.want)
			}
		})
	}
}

func TestFindingRelevanceScoreDoesNotTreatSubAgentAsAgent(t *testing.T) {
	f := types.FlowFindingDigest{
		Path:       []string{"SubAgentRuntime", "execute"},
		Confidence: 1.0,
	}
	score := findingRelevanceScore(f, []string{"agent"})
	if score != 0 {
		t.Fatalf("short entity Agent must not match SubAgentRuntime, score=%v", score)
	}
}

// TestMergeEvidenceItems_LLMEmitOutranksDataflow locks the C16 fix:
// items produced by emit_evidence (or other question-targeted
// explorer analysis) must sort ahead of dataflow.* items, regardless
// of file path. The s1a eval regression showed dataflow scanning
// cmd/root.go fill the downstream top-18 Structured Evidence slot
// (cmd/ sorts before internal/) and burying the real gate.go
// checkXxx items the LLM had just emitted. Rank-before-path fixes
// it without giving up deterministic output.
func TestMergeEvidenceItems_LLMEmitOutranksDataflow(t *testing.T) {
	// Alphabetically-early dataflow item (the failure case's shape).
	df := types.EvidenceItem{
		Kind:      types.EvidenceDirect,
		Subject:   "init",
		Predicate: "calls",
		Object:    "PersistentFlags",
		Source:    "cmd/root.go",
		LineStart: 134,
		Producer:  "dataflow.engine",
	}
	// Alphabetically-later LLM emit item.
	llm := types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Subject:      "Run",
		Predicate:    "calls",
		Object:       "checkCoverage",
		Source:       "internal/analysis/gate/gate.go",
		LineStart:    106,
		AnchorSymbol: "checkCoverage",
		Producer:     "explorer.emit_evidence",
	}

	merged := mergeEvidenceItems([]types.EvidenceItem{df}, []types.EvidenceItem{llm})
	if len(merged) != 2 {
		t.Fatalf("merged len = %d, want 2", len(merged))
	}
	if merged[0].Producer != "explorer.emit_evidence" {
		t.Errorf("merged[0].Producer = %q, want explorer.emit_evidence (LLM emit must outrank dataflow)", merged[0].Producer)
	}
	if merged[1].Producer != "dataflow.engine" {
		t.Errorf("merged[1].Producer = %q, want dataflow.engine", merged[1].Producer)
	}
}

// TestMergeEvidenceItems_WithinRankKeepsPathSort verifies the
// secondary tiebreaker: within a single rank (all LLM emits, or all
// dataflow), sort is still (Source, LineStart, ID) — the historical
// deterministic output callers depend on.
func TestMergeEvidenceItems_WithinRankKeepsPathSort(t *testing.T) {
	earlier := types.EvidenceItem{
		Kind:      types.EvidenceDirect,
		Subject:   "A",
		Source:    "cmd/root.go",
		LineStart: 100,
		Producer:  "dataflow.engine",
	}
	later := types.EvidenceItem{
		Kind:      types.EvidenceDirect,
		Subject:   "B",
		Source:    "internal/orchestrator/orchestrator.go",
		LineStart: 50,
		Producer:  "dataflow.finding",
	}
	merged := mergeEvidenceItems([]types.EvidenceItem{later}, []types.EvidenceItem{earlier})
	if merged[0].Source != "cmd/root.go" {
		t.Errorf("merged[0].Source = %q, want cmd/root.go (within-rank alphabetical tiebreaker)", merged[0].Source)
	}
}

// TestMergeEvidenceItems_CollidingIDPromotesRank verifies the edge
// case where a dataflow.* item and an LLM emit happen to hash to the
// same stable ID: the merged entry must carry the LLM-emit Producer
// so it still ranks ahead of pure-dataflow entries. Without this
// promotion the merged item would keep whichever Producer arrived
// first and potentially downrank itself.
func TestMergeEvidenceItems_CollidingIDPromotesRank(t *testing.T) {
	id := types.StableEvidenceID(types.EvidenceDirect, "Run", "calls", "checkCoverage", "", "internal/analysis/gate/gate.go", 106, 0)
	llm := types.EvidenceItem{
		ID:        id,
		Kind:      types.EvidenceDirect,
		Subject:   "Run",
		Predicate: "calls",
		Object:    "checkCoverage",
		Source:    "internal/analysis/gate/gate.go",
		LineStart: 106,
		Producer:  "explorer.emit_evidence",
	}
	df := llm
	df.Producer = "dataflow.engine"

	// Feed dataflow first so it lands in the map with dataflow.Producer
	// first. The LLM item arrives second — rank-promotion must kick in.
	merged := mergeEvidenceItems([]types.EvidenceItem{df}, []types.EvidenceItem{llm})
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].Producer != "explorer.emit_evidence" {
		t.Errorf("collided-ID merge must promote Producer to the LLM-emit band, got %q", merged[0].Producer)
	}
}

func TestBuildCrossReferenceMapFromEvidenceUsesFlowFindings(t *testing.T) {
	findings := []types.FlowFindingDigest{
		{
			ID:         "flow-1",
			Path:       []string{"config.handlers.explorer", "NewExplorer", "Register"},
			Conditions: []string{"enabled == true"},
			Confidence: 0.8,
		},
	}

	out := buildCrossReferenceMapFromEvidence(nil, findings)
	if out == "" {
		t.Fatal("expected cross-reference output from flow findings")
	}
	if want := "config.handlers.explorer -> NewExplorer -> Register"; !containsString(out, want) {
		t.Fatalf("cross-reference output missing %q:\n%s", want, out)
	}
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestExtractRankingEntities(t *testing.T) {
	// The no-graph variant accepts structural identifiers
	// (CamelCase, snake_case, qualified names) and long lowercase
	// tokens (≥ 8 chars). Pure short lowercase words — including
	// the domain term `agent` — are rejected because the extractor
	// has no way to distinguish them from generic English prose.
	// Callers that hold a repomap.Graph should use
	// extractRankingEntitiesWithGraph so short lowercase tokens that
	// exactly match a repo symbol survive.
	tests := []struct {
		question string
		wantAny  []string // at least these entities should be extracted
	}{
		{
			"有多少个agent可以调用subagent?",
			[]string{"subagent"},
		},
		{
			"How does `SubExplorer.Name` work?",
			[]string{"subexplorer.name"},
		},
		{
			"What is RegisterDefaultSubAgents?",
			[]string{"registerdefaultsubagents"},
		},
	}
	for _, tt := range tests {
		entities := extractRankingEntitiesWithGraph(tt.question, nil)
		for _, want := range tt.wantAny {
			found := false
			for _, got := range entities {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractRankingEntitiesWithGraph(%q) missing %q, got %v", tt.question, want, entities)
			}
		}
	}
}

func TestExtractRankingEntities_DropsGenericLowercaseProse(t *testing.T) {
	// Regression pin for the REPL-audit follow-up #3: the df1 question
	// used to yield {many, agents, invoke, subagent}, polluting every
	// downstream ranking and ERM relevance calculation with three
	// generic English verbs/nouns. After the 2026-04-13 tightening
	// only `subagent` (8 chars, pure lowercase but long enough to
	// stand on length alone) should survive the no-graph path.
	got := extractRankingEntitiesWithGraph("how many agents can invoke subagent", nil)
	sort.Strings(got)
	want := []string{"subagent"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected only %v, got %v", want, got)
	}
	banned := []string{"many", "agents", "invoke", "how", "can"}
	for _, b := range banned {
		for _, g := range got {
			if g == b {
				t.Errorf("tightened filter still admits generic prose %q", b)
			}
		}
	}
}

func TestExtractRankingEntitiesWithGraph_AcceptsShortSymbolMatches(t *testing.T) {
	// When a repomap.Graph is available, pure-lowercase short tokens
	// are accepted if their lowercased form exactly matches a symbol
	// name in the repo. This lets genuine short domain terms like
	// `Agent` (as a type) survive even though they would be filtered
	// out by the no-graph length rule. The test uses a synthetic
	// graph so the rule is independent of whatever real symbols
	// codrax happens to ship today.
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{
			"Agent":   {{Name: "Agent", Kind: "type"}},
			"Handler": {{Name: "Handler", Kind: "type"}},
		},
	}
	got := extractRankingEntitiesWithGraph("how many agents invoke an Agent handler", graph)
	// `agents` (plural) still has no symbol match → rejected.
	// `invoke`  (no symbol named "invoke")          → rejected.
	// `Agent`   (capitalised, structural)           → accepted as "agent".
	// `handler` (symbol "Handler" lowercased match) → accepted.
	// `many`    (4 chars, lowercase, no match)      → rejected.
	// `how`     (3 chars)                           → rejected (min length).
	wantSet := map[string]bool{"agent": true, "handler": true}
	if len(got) != len(wantSet) {
		t.Fatalf("got %v, want exactly %v", got, wantSet)
	}
	for _, e := range got {
		if !wantSet[e] {
			t.Errorf("unexpected entity %q in %v", e, got)
		}
	}
}

func TestEntityQualifies_Rule(t *testing.T) {
	symSet := map[string]bool{"handler": true}
	cases := []struct {
		raw  string
		ok   bool
		note string
	}{
		{"many", false, "pure lowercase < 8 chars, no symbol match"},
		{"invoke", false, "pure lowercase 6 chars, no symbol match"},
		{"agents", false, "plural form, no symbol match"},
		{"subagent", true, "length 8 compound identifier"},
		{"Agent", true, "uppercase → structural"},
		{"sub_agent", true, "underscore → structural"},
		{"pkg.Foo", true, "dot → structural"},
		{"handler", true, "matches symbol table"},
		{"has", false, "below min length"},
		{"Orchestrator", true, "CamelCase"},
	}
	for _, c := range cases {
		if got := entityQualifies(c.raw, symSet); got != c.ok {
			t.Errorf("entityQualifies(%q, symSet)=%v, want %v (%s)", c.raw, got, c.ok, c.note)
		}
	}
}

func TestRankEvidenceByRelevance(t *testing.T) {
	question := "有多少个agent可以调用subagent?"

	items := []types.EvidenceItem{
		{ // Irrelevant noise
			Kind: types.EvidenceConditional, Subject: "Logger.Close", Object: "file handle",
			Summary: "Logger closes file", Source: "logging/logger.go",
		},
		{ // Highly relevant: registration of subagent
			Kind: types.EvidenceRegistration, Subject: "RegisterDefaultSubAgents", Object: "NewSubExplorer",
			Summary: "registers SubExplorer as the only subagent", Source: "internal/agent/subagent.go",
		},
		{ // Relevant: agent calls subagent
			Kind: types.EvidenceRelationship, Subject: "buildToolSchemas", Object: "SubAgents.Get",
			Summary: "injects propose_sub_agents tool only if a sub-agent with the same name is registered",
			Source:  "internal/agent/agent.go",
		},
		{ // Somewhat relevant: mentions agent
			Kind: types.EvidenceMechanism, Subject: "Orchestrator", Object: "dispatchStage",
			Summary: "dispatches agent to stage", Source: "orchestrator.go",
		},
		{ // Highly relevant: concrete return value
			Kind: types.EvidenceConcrete, Subject: "SubExplorer.Name", Predicate: "returns", Object: `"explorer"`,
			Summary: `SubExplorer.Name() returns "explorer"`, Source: "internal/agent/sub_explorer.go",
		},
	}

	ranked := rankEvidenceByRelevance(question, items, nil)
	if len(ranked) != 5 {
		t.Fatalf("rankEvidenceByRelevance returned %d items, want 5", len(ranked))
	}

	// The registration item (RegisterDefaultSubAgents) should rank #1:
	// it has the strongest entity overlap (agent+subagent) + registration kind weight.
	if ranked[0].Subject != "RegisterDefaultSubAgents" {
		t.Errorf("expected RegisterDefaultSubAgents at #1, got %q", ranked[0].Subject)
	}

	// The bridge item (buildToolSchemas→SubAgents.Get) should rank #2:
	// both subject and object match question entities → bridge bonus.
	if ranked[1].Subject != "buildToolSchemas" {
		t.Errorf("expected buildToolSchemas at #2, got %q", ranked[1].Subject)
	}

	// Logger (no entity overlap with agent/subagent) should be last.
	lastSubject := ranked[len(ranked)-1].Subject
	if lastSubject != "Logger.Close" && lastSubject != "SubExplorer.Name" {
		t.Errorf("expected irrelevant item at bottom, got %q", lastSubject)
	}
}

func TestRankFindingsByRelevance(t *testing.T) {
	question := "有多少个agent可以调用subagent?"

	findings := []types.FlowFindingDigest{
		{ // Irrelevant
			Path:    []string{"Logger.Close", "TestMultilineThreeLines"},
			Sources: []string{"logging/logger.go"}, Sinks: []string{"repl_test.go"},
			Confidence: 0.81,
		},
		{ // Relevant: subagent registration chain
			Path:    []string{"RegisterDefaultSubAgents", "NewSubExplorer"},
			Sources: []string{"internal/agent/subagent.go"}, Sinks: []string{"SubExplorer"},
			Confidence: 0.84,
		},
		{ // Irrelevant long chain
			Path:    []string{"expandKeywords", "trySplitConcatenated", "normalizeStrings", "mergeStrings"},
			Sources: []string{"keyword_search.go"}, Sinks: []string{"evidence.go"},
			Confidence: 0.82,
		},
	}

	ranked := rankFindingsByRelevance(question, findings)
	if len(ranked) != 3 {
		t.Fatalf("got %d findings, want 3", len(ranked))
	}

	// The subagent registration finding should be first
	if !strings.Contains(ranked[0].Path[0], "Register") {
		t.Errorf("expected RegisterDefaultSubAgents finding first, got path=%v", ranked[0].Path)
	}

	// Logger finding should be last (zero entity overlap)
	if !strings.Contains(ranked[len(ranked)-1].Path[0], "Logger") &&
		!strings.Contains(ranked[len(ranked)-1].Path[0], "expand") {
		t.Errorf("expected irrelevant finding at bottom, got path=%v", ranked[len(ranked)-1].Path)
	}
}

// TestRankEvidence_MechanismConcretePromotion replays the 2026-04-17
// regression where a dozen LLM-emit conditional items from files the
// LLM happened to read buried the programmatically-extracted concrete
// value for `BaseAgent.buildToolSchemas → b.deps.SubAgents.Get(
// string(b.name))` — the actual gate that answers "how many agents
// can call subagent". Without mechanism-concrete promotion the
// correct evidence lived in the +318 overflow; with promotion it
// must surface in the top-N so the extractor digest carries it.
func TestRankEvidence_MechanismConcretePromotion(t *testing.T) {
	question := "有多少个agent可以调用subagent?"

	items := []types.EvidenceItem{
		// 8 LLM-emit conditional items about explorer meta-logic, all
		// with subject/object containing `agent`/`subagent` superficially.
		// This mirrors the real failure: explorer_erm.go and explorer.go
		// produce lots of "agent X subagent" conditionals that are
		// about the explorer's own detection heuristics, not about
		// how agents invoke subagents.
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "reset", Object: "subagent",
			Summary: "reset state across runs", Source: "internal/agent/explorer.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "sequential", Object: "subagent",
			Summary: "skip Phase 0 on retry", Source: "internal/agent/explorer.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "validate", Object: "subagent",
			Summary: "validate sub_tasks non-empty", Source: "internal/tool/propose_sub_agents.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "analyze", Object: "subagent",
			Summary: "trust analyzer if kind declared", Source: "internal/agent/explorer.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "retry", Object: "subagent",
			Summary: "depth read on retry", Source: "internal/agent/explorer.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "enumerate", Object: "subagent",
			Summary: "enumeration intent for subagents", Source: "internal/agent/explorer_erm.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "calls", Object: "subagent",
			Summary: "call-chain inferred", Source: "internal/agent/explorer_erm.go",
			Producer: "emit_evidence"},
		{Kind: types.EvidenceConditional, Subject: "agent", Predicate: "execute", Object: "subagent",
			Summary: "execute if env var matches", Source: "internal/tool/propose_sub_agents.go",
			Producer: "emit_evidence"},

		// The critical mechanism concrete value. Source is NOT in the
		// read files (mirrors real scenario: LLM skipped agent.go).
		// Subject contains `agent` (in `BaseAgent`); Object contains
		// `sub_agents` which must match `subagent` after underscore
		// normalization.
		{Kind: types.EvidenceConcrete,
			Subject:   "BaseAgent.buildToolSchemas",
			Predicate: "assigns",
			Object:    `err := b.deps.SubAgents.Get(string(b.name)); err == nil {`,
			Summary:   "`BaseAgent.buildToolSchemas()` assigns err := b.deps.SubAgents.Get(string(b.name)); err == nil {",
			Source:    "internal/agent/agent.go",
			Producer:  "concrete_values",
		},
	}

	// readFiles mirrors the real run: LLM read explorer.go, explorer_erm.go,
	// propose_sub_agents.go, subagent.go, types/subagent.go — NOT agent.go.
	readFiles := map[string]bool{
		"internal/agent/explorer.go":          true,
		"internal/agent/explorer_erm.go":      true,
		"internal/tool/propose_sub_agents.go": true,
		"internal/agent/subagent.go":          true,
		"internal/types/subagent.go":          true,
	}

	ranked := rankEvidenceByRelevance(question, items, readFiles)

	// The mechanism concrete MUST appear within the top 4. The
	// extractor's Primary Evidence slot is currently topN=12, so top-4
	// is a strict floor — if the mechanism concrete is beaten even by
	// 4 noise items the promotion is not strong enough.
	foundAt := -1
	for i, ev := range ranked {
		if ev.Subject == "BaseAgent.buildToolSchemas" {
			foundAt = i
			break
		}
	}
	if foundAt < 0 {
		t.Fatalf("buildToolSchemas mechanism concrete not present in ranked output (len=%d)", len(ranked))
	}
	if foundAt >= 4 {
		t.Errorf("buildToolSchemas ranked at position %d, want < 4 (mechanism-concrete promotion failed)", foundAt)
	}
}

// TestLooksLikeMechanismConcreteValue exercises the detector directly
// to lock down its decision boundary. Promotion is intentionally
// tight — it triggers only when the Object shows a registry-call
// pattern AND the text overlaps a question entity.
func TestLooksLikeMechanismConcreteValue(t *testing.T) {
	entities := []string{"agent", "subagent"}

	cases := []struct {
		name string
		item types.EvidenceItem
		want bool
	}{
		{
			name: "buildToolSchemas SubAgents.Get — promote",
			item: types.EvidenceItem{
				Kind:     types.EvidenceConcrete,
				Producer: "concrete_values",
				Subject:  "BaseAgent.buildToolSchemas",
				Object:   `err := b.deps.SubAgents.Get(string(b.name)); err == nil {`,
			},
			want: true,
		},
		{
			name: "Tools.Get literal — promote (underscore-normalized entity match)",
			item: types.EvidenceItem{
				Kind:     types.EvidenceConcrete,
				Producer: "concrete_values",
				Subject:  "BaseAgent.buildToolSchemas",
				Object:   `err := b.deps.Tools.Get("propose_sub_agents"); err == nil {`,
			},
			want: true,
		},
		{
			name: "LLM-emit item — NOT promoted (promotion is concrete-only)",
			item: types.EvidenceItem{
				Kind:     types.EvidenceConditional,
				Producer: "emit_evidence",
				Subject:  "agent",
				Object:   "subagent",
				Summary:  "calls_subagent via .Get(key)",
			},
			want: false,
		},
		{
			name: "unrelated registry call — no entity overlap",
			item: types.EvidenceItem{
				Kind:     types.EvidenceConcrete,
				Producer: "concrete_values",
				Subject:  "Logger.configure",
				Object:   `h := registry.Get("default")`,
			},
			want: false,
		},
		{
			name: "simple return value — no mechanism pattern",
			item: types.EvidenceItem{
				Kind:     types.EvidenceConcrete,
				Producer: "concrete_values",
				Subject:  "SubAgentRegistry.Name",
				Object:   `"explorer"`,
				Summary:  `SubAgentRegistry.Name() returns "explorer"`,
			},
			want: false,
		},
		{
			name: "dataflow_path item — NOT a concrete, never promoted",
			item: types.EvidenceItem{
				Kind:     types.EvidenceDataflowPath,
				Producer: "concrete_values",
				Subject:  "RegisterDefaultSubAgents() binds NewSubExplorer",
				Object:   `chains via SubAgents.Get(...)`,
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := normalizeEntityHaystack(strings.ToLower(
				c.item.Subject + " " + c.item.Object + " " + c.item.Summary + " " + c.item.Predicate))
			got := looksLikeMechanismConcreteValue(c.item, text, entities)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRankEvidenceDiversity(t *testing.T) {
	question := "What does SubExplorer do?"
	// 5 items from same source+subject+anchor — only 2 should survive per key.
	// AnchorSymbol intentionally shared so the distinct-anchor escape hatch
	// does not kick in; that case is covered by
	// TestRankEvidenceDiversity_DistinctAnchorsBypass.
	items := make([]types.EvidenceItem, 5)
	for i := range items {
		items[i] = types.EvidenceItem{
			Kind:         types.EvidenceConcrete,
			Subject:      "SubExplorer.Run",
			Object:       strings.Repeat("x", i+1),
			Summary:      "SubExplorer runs sub tasks",
			Source:       "sub_explorer.go",
			AnchorSymbol: "Run",
		}
	}
	ranked := rankEvidenceByRelevance(question, items, nil)
	if len(ranked) != 2 {
		t.Errorf("diversity constraint: got %d items, want 2 (max 2 per source+subject+anchor)", len(ranked))
	}
}

// TestRankEvidenceDiversity_DistinctAnchorsBypass pins the s1a-20260422
// regression: the LLM correctly emit_evidence'd 7 call points at gate.go:
// 106-112 with subject="Run" and distinct anchor_symbols (checkCoverage,
// checkDAGClosure, ..., checkPendingFieldsWellformed) representing 7
// genuinely different facts, and the diversity cap silently dropped 5/7
// because the key was (source, subject) only. Distinct AnchorSymbol must
// yield distinct dedup keys so every call point survives into the top-N.
func TestRankEvidenceDiversity_DistinctAnchorsBypass(t *testing.T) {
	question := "gate.Run 的 7 项检查是按什么顺序跑的？"
	callees := []string{
		"checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage",
		"checkCriterionResolvable", "checkPendingFieldsWellformed",
	}
	items := make([]types.EvidenceItem, len(callees))
	for i, callee := range callees {
		items[i] = types.EvidenceItem{
			Kind:         types.EvidenceDirect,
			Subject:      "Run",
			Predicate:    "calls",
			Object:       callee,
			Source:       "internal/analysis/gate/gate.go",
			LineStart:    106 + i,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: callee,
			Summary:      "第" + strings.Repeat("x", i+1) + "项检查",
		}
	}
	ranked := rankEvidenceByRelevance(question, items, nil)
	if len(ranked) != len(callees) {
		t.Fatalf("distinct anchors: got %d survivors, want %d (diversity cap collapsed distinct call points)",
			len(ranked), len(callees))
	}
	// Every callee must appear exactly once in the survivors.
	seen := make(map[string]int, len(callees))
	for _, it := range ranked {
		seen[it.AnchorSymbol]++
	}
	for _, c := range callees {
		if seen[c] != 1 {
			t.Errorf("callee %q: got %d occurrences, want 1", c, seen[c])
		}
	}
}

func TestNeedsDataflowAnalysis_ChineseKeywords(t *testing.T) {
	tests := []struct {
		question string
		want     bool
	}{
		{"有多少个agent可以调用subagent?", true},                        // 调用 + 多少
		{"请列出哪些agent注册了subagent", true},                         // 哪些 + 注册
		{"这个配置项是怎么传播到handler的?", true},                          // 配置 + 传播 + 怎么
		{"What is the project name?", false},                    // no trigger keywords
		{"How does the value flow through the pipeline?", true}, // flow + through
		{"列出所有注册的路由", true},                                     // 注册 + 路由
		{"代码风格好不好", false},                                      // no trigger keywords
	}
	for _, tt := range tests {
		got := needsDataflowAnalysis(tt.question, nil)
		if got != tt.want {
			t.Errorf("needsDataflowAnalysis(%q) = %v, want %v", tt.question, got, tt.want)
		}
	}
}

// buildGutterReadResult constructs a synthetic read_file ToolResult
// that matches the exact banner + per-line gutter format produced
// by tool.renderWithLineGutter. Used by the grounder tests to
// drive buildReadFileLineIndex without hitting disk.
func buildGutterReadResult(path string, startLine int, lines []string, totalLines int) types.ToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s: showing lines %d-%d of %d total]\n",
		path, startLine, startLine+len(lines)-1, totalLines)
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%6d│ %s", startLine+i, l)
	}
	return types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  b.String(),
	}
}

// Grounding tests migrated to internal/tool/ground/ground_test.go in
// the 2026-04-17 redesign. The old two-tier grounder (output:
// LineStart=0 + Producer /ungrounded) was replaced by the three-state
// ground.GroundItem (GroundingStatus: grounded / recovered / ungrounded).

// TestParseEvidenceHeaderSource_StripsMarkdownBackticks locks the
// parser behaviour that the LLM's markdown-prose wrapper around
// the file path does NOT leak into EvidenceItem.Source.
//
// Observed in df1-20260414-011749: the LLM wrote
//
//	## Evidence from `internal/agent/sub_explorer.go`
//
// and the parser carried the backticks through, so the grounder
// looked up `` `internal/agent/sub_explorer.go` `` in its per-
// file line index and missed every legitimate cite. The fix
// strips both the legacy `[...]` wrapper and the markdown `` `...` ``
// wrapper in a single TrimCutset pass.
func TestParseEvidenceHeaderSource_StripsMarkdownBackticks(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"## Evidence from `internal/agent/sub_explorer.go`", "internal/agent/sub_explorer.go"},
		{"## Evidence from [internal/agent/sub_explorer.go]", "internal/agent/sub_explorer.go"},
		{"## Evidence from internal/agent/sub_explorer.go", "internal/agent/sub_explorer.go"},
		{"## Evidence from `internal/agent/sub_explorer.go:25`", "internal/agent/sub_explorer.go"},
		{"## Evidence from ``", ""},
	}
	for _, c := range cases {
		got := parseEvidenceHeaderSource(c.in)
		if got != c.want {
			t.Errorf("parseEvidenceHeaderSource(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLooksLikeCodeIdentifier_StructuralRules moved with the rest of
// the grounding helpers to internal/tool/ground (2026-04-17 redesign).

// TestRankEvidenceWithAxis_CallBreaksTieOverRegistration is the
// regression test for the 2026-04-18 "explorer是如何调用subagent的"
// bug class: when the question carries AxisCall and two evidence
// items have comparable base scores (equal entity overlap, equal
// kind weight, equal bridge bonus), the axis × anchor_kind affinity
// must break the tie in favour of the call-anchored item.
//
// Balanced construction: both items carry the same entities
// ("explorer", "subagent"), the same kind (direct), the same source
// weight (neither read-through-explorer), and both trigger the
// bridge bonus. The ONLY differentiator is AnchorKind, so any
// ordering flip observed is solely the axis boost at work.
func TestRankEvidenceWithAxis_CallBreaksTieOverRegistration(t *testing.T) {
	question := "explorer是如何调用subagent的？"
	items := []types.EvidenceItem{
		{
			// Assignment anchor: "registered SubExplorer against the
			// SubAgentRegistry". Both entities present in subject+object
			// → bridgeBonus fires.
			Kind: types.EvidenceDirect, Subject: "SubAgentRegistry",
			Predicate: "binds", Object: "NewSubExplorer",
			Summary:      "SubAgentRegistry holds the sub-agent map; SubExplorer is the explorer-targeted entry",
			Source:       "internal/agent/subagent.go",
			AnchorKind:   types.AnchorAssignment,
			AnchorSymbol: "SubAgentRegistry",
			LineStart:    55,
		},
		{
			// Call anchor: "SubAgentRuntime invokes SubExplorer.Run".
			// Balanced entity overlap — both entities in subject+object.
			Kind: types.EvidenceDirect, Subject: "SubAgentRuntime",
			Predicate: "invokes", Object: "SubExplorer.Run",
			Summary:      "SubAgentRuntime executes sub-agents including the explorer branch by calling Run",
			Source:       "internal/agent/subagent_runtime.go",
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "Run",
			LineStart:    136,
		},
	}

	// With AxisCall: the call-anchored item MUST float above the
	// assignment-anchored one. With AxisUnknown the two items have
	// equal (or near-equal) scores; axis is the decisive signal.
	withAxis := rankEvidenceByRelevanceWithSubject(
		question, items, nil, types.AnswerSubject{}, nil, types.AxisCall)
	if len(withAxis) != 2 {
		t.Fatalf("axis: got %d items, want 2", len(withAxis))
	}
	if withAxis[0].Subject != "SubAgentRuntime" {
		t.Errorf("AxisCall should float call-anchored SubAgentRuntime to #1; got %q (all=%v)",
			withAxis[0].Subject, subjectSlice(withAxis))
	}
	if withAxis[1].Subject != "SubAgentRegistry" {
		t.Errorf("AxisCall should demote assignment-anchored SubAgentRegistry to #2; got %q at #2",
			withAxis[1].Subject)
	}

	// Sanity: AxisRegister (the converse question) should FLIP the
	// result — assignment-anchored item now wins.
	withRegister := rankEvidenceByRelevanceWithSubject(
		question, items, nil, types.AnswerSubject{}, nil, types.AxisRegister)
	if withRegister[0].Subject != "SubAgentRegistry" {
		t.Errorf("AxisRegister should float assignment-anchored SubAgentRegistry to #1; got %q (all=%v)",
			withRegister[0].Subject, subjectSlice(withRegister))
	}
}

func subjectSlice(items []types.EvidenceItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Subject
	}
	return out
}

// TestRankEvidenceWithAxis_UnknownAxisIsNoOp confirms the zero-axis
// path leaves ranking identical to the plain ranker.
func TestRankEvidenceWithAxis_UnknownAxisIsNoOp(t *testing.T) {
	question := "how does X work"
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, Subject: "A", Object: "x",
			Source: "a.go", AnchorKind: types.AnchorCall, LineStart: 1},
		{Kind: types.EvidenceDirect, Subject: "B", Object: "x",
			Source: "b.go", AnchorKind: types.AnchorDefinition, LineStart: 1},
	}
	base := rankEvidenceByRelevance(question, items, nil)
	withUnknown := rankEvidenceByRelevanceWithSubject(
		question, items, nil, types.AnswerSubject{}, nil, types.AxisUnknown)
	if len(base) != len(withUnknown) {
		t.Fatalf("length mismatch: base=%d axis=%d", len(base), len(withUnknown))
	}
	for i := range base {
		if base[i].Subject != withUnknown[i].Subject {
			t.Errorf("AxisUnknown should not change order; pos %d: base=%q axis=%q",
				i, base[i].Subject, withUnknown[i].Subject)
		}
	}
}
