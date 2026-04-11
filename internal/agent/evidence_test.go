package agent

import (
	"strings"
	"testing"

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
	tests := []struct {
		question string
		wantAny  []string // at least these entities should be extracted
	}{
		{
			"有多少个agent可以调用subagent?",
			[]string{"agent", "subagent"},
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
