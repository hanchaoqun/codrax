package agent

import (
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
		entities := extractRankingEntities(tt.question)
		for _, want := range tt.wantAny {
			found := false
			for _, got := range entities {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractRankingEntities(%q) missing %q, got %v", tt.question, want, entities)
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
	got := extractRankingEntities("how many agents can invoke subagent")
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
			Source: "internal/agent/agent.go",
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
			Path: []string{"Logger.Close", "TestMultilineThreeLines"},
			Sources: []string{"logging/logger.go"}, Sinks: []string{"repl_test.go"},
			Confidence: 0.81,
		},
		{ // Relevant: subagent registration chain
			Path: []string{"RegisterDefaultSubAgents", "NewSubExplorer"},
			Sources: []string{"internal/agent/subagent.go"}, Sinks: []string{"SubExplorer"},
			Confidence: 0.84,
		},
		{ // Irrelevant long chain
			Path: []string{"expandKeywords", "trySplitConcatenated", "normalizeStrings", "mergeStrings"},
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

func TestRankEvidenceDiversity(t *testing.T) {
	question := "What does SubExplorer do?"
	// 5 items from same source+subject — only 2 should survive per key
	items := make([]types.EvidenceItem, 5)
	for i := range items {
		items[i] = types.EvidenceItem{
			Kind:    types.EvidenceConcrete,
			Subject: "SubExplorer.Run",
			Object:  strings.Repeat("x", i+1),
			Summary: "SubExplorer runs sub tasks",
			Source:  "sub_explorer.go",
		}
	}
	ranked := rankEvidenceByRelevance(question, items, nil)
	if len(ranked) != 2 {
		t.Errorf("diversity constraint: got %d items, want 2 (max 2 per source+subject)", len(ranked))
	}
}

func TestNeedsDataflowAnalysis_ChineseKeywords(t *testing.T) {
	tests := []struct {
		question string
		want     bool
	}{
		{"有多少个agent可以调用subagent?", true},           // 调用 + 多少
		{"请列出哪些agent注册了subagent", true},              // 哪些 + 注册
		{"这个配置项是怎么传播到handler的?", true},              // 配置 + 传播 + 怎么
		{"What is the project name?", false},             // no trigger keywords
		{"How does the value flow through the pipeline?", true}, // flow + through
		{"列出所有注册的路由", true},                           // 注册 + 路由
		{"代码风格好不好", false},                             // no trigger keywords
	}
	for _, tt := range tests {
		got := needsDataflowAnalysis(tt.question, nil)
		if got != tt.want {
			t.Errorf("needsDataflowAnalysis(%q) = %v, want %v", tt.question, got, tt.want)
		}
	}
}
