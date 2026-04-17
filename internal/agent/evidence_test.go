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

// TestGroundEvidenceItems_AcceptsCorroboratedLine verifies the
// positive path: an evidence item whose Subject identifier
// actually appears on the cited line (reconstructed from gutter)
// is kept as-is.
func TestGroundEvidenceItems_AcceptsCorroboratedLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/explorer.go", 976,
			[]string{
				"func (e *explorerEvaluator) ContinuationPrompt(resp llm.Response) (string, bool) {",
				"\tif resp.Content != \"\" && e.phase == 1 {",
				"\t\te.investigationNotes = append(e.investigationNotes, resp.Content)",
			}, 5574),
	}
	items := []types.EvidenceItem{{
		Kind:      types.EvidenceMechanism,
		Subject:   "ContinuationPrompt",
		Source:    "internal/agent/explorer.go",
		LineStart: 976,
		Producer:  "explorer.llm",
		Summary:   "[MECHANISM] ContinuationPrompt line 976: entry point",
	}}
	out := groundEvidenceItems(items, nil, history)
	if len(out) != 1 {
		t.Fatalf("length: %d", len(out))
	}
	if out[0].LineStart != 976 {
		t.Errorf("grounded line cleared: got %d, want 976", out[0].LineStart)
	}
	if strings.Contains(out[0].Producer, ungroundedSuffix) {
		t.Errorf("grounded item was tagged ungrounded: %q", out[0].Producer)
	}
}

// TestGroundEvidenceItems_RejectsHallucinatedLine reproduces the
// df3 Pattern 2 failure: the LLM cites a line number that is
// inside a real function body, but the actual text at that line
// does not mention the subject. The grounder must clear
// LineStart and tag ungrounded. Neighbour radius ±2 means the
// cite is rejected only when none of the surrounding 5 lines
// mention the subject, not on an off-by-one.
func TestGroundEvidenceItems_RejectsHallucinatedLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/explorer.go", 1012,
			[]string{
				"\t\tusedOtherDiscovery = true",       // 1012
				"\t}",                                  // 1013
				"\tif !e.phase0ExtraRound {",          // 1014
				"\t\tdiscovered, _ = extractFileCoverage(history)", // 1015
				"\t\tusedGrep := false",               // 1016
				"\t\tusedOtherDiscovery := false",     // 1017
				"\t\tfor _, r := range history {",     // 1018 — no ContinuationPrompt
				"\t\t\tif r.Success {",                // 1019
				"\t\t\t\tswitch r.ToolName {",         // 1020
			}, 5574),
	}
	items := []types.EvidenceItem{{
		Kind:      types.EvidenceConditional,
		Subject:   "ContinuationPrompt",
		Source:    "internal/agent/explorer.go",
		LineStart: 1018,
		Producer:  "explorer.llm",
		Summary:   "[CONDITIONAL] ContinuationPrompt line 1018: phase transition",
	}}
	out := groundEvidenceItems(items, nil, history)
	if out[0].LineStart != 0 {
		t.Errorf("hallucinated line not cleared: got %d, want 0", out[0].LineStart)
	}
	if !strings.HasSuffix(out[0].Producer, ungroundedSuffix) {
		t.Errorf("hallucinated item not tagged ungrounded: %q", out[0].Producer)
	}
}

// TestGroundEvidenceItems_AcceptsSymbolStartFromGrep covers the
// tier-2 fallback: when the LLM cites a symbol declaration line
// without ever calling read_file on that file (the number came
// from a grep result), the grounder still accepts it because the
// repo graph confirms the symbol exists at that exact line.
func TestGroundEvidenceItems_AcceptsSymbolStartFromGrep(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/explorer.go": {
				RelPath: "internal/agent/explorer.go",
				Symbols: []repomap.Symbol{
					{Name: "ContinuationPrompt", Kind: "method", Line: 976, EndLine: 1413},
				},
			},
		},
	}
	items := []types.EvidenceItem{{
		Kind:      types.EvidenceMechanism,
		Subject:   "ContinuationPrompt",
		Source:    "internal/agent/explorer.go",
		LineStart: 976,
		Producer:  "explorer.llm",
	}}
	out := groundEvidenceItems(items, graph, nil)
	if out[0].LineStart != 976 {
		t.Errorf("tier-2 should have accepted symbol start line, cleared to %d", out[0].LineStart)
	}
	if strings.HasSuffix(out[0].Producer, ungroundedSuffix) {
		t.Errorf("tier-2 grounded item tagged ungrounded: %q", out[0].Producer)
	}
}

// TestGroundEvidenceItems_RejectsUnknownSymbolOutsideAnyRead is
// the combined miss: no read_file history covers the line, and
// the graph has no symbol with a matching name. Both tiers fail.
func TestGroundEvidenceItems_RejectsUnknownSymbolOutsideAnyRead(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/explorer.go": {
				RelPath: "internal/agent/explorer.go",
				Symbols: []repomap.Symbol{
					{Name: "extractFileCoverage", Kind: "function", Line: 4433, EndLine: 4505},
				},
			},
		},
	}
	items := []types.EvidenceItem{{
		Kind:      types.EvidenceDirect,
		Subject:   "NonExistentSymbol",
		Source:    "internal/agent/explorer.go",
		LineStart: 9999,
		Producer:  "explorer.llm",
	}}
	out := groundEvidenceItems(items, graph, nil)
	if out[0].LineStart != 0 {
		t.Errorf("unknown subject at unknown line not cleared: %d", out[0].LineStart)
	}
	if !strings.HasSuffix(out[0].Producer, ungroundedSuffix) {
		t.Errorf("unknown subject not tagged ungrounded: %q", out[0].Producer)
	}
}

// TestGroundEvidenceItems_PassThroughWhenNoLineCited verifies that
// evidence items without a LineStart (relationship or absent
// entries that don't carry a line number) are left untouched —
// grounding only fires on committed citations.
func TestGroundEvidenceItems_PassThroughWhenNoLineCited(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:     types.EvidenceRelationship,
			Subject:  "Register",
			Object:   "NewExplorer",
			Source:   "internal/agent/explorer.go",
			Producer: "explorer.llm",
		},
		{
			Kind:     types.EvidenceAbsent,
			Subject:  "LegacyHandler",
			Producer: "explorer.llm",
		},
	}
	out := groundEvidenceItems(items, nil, nil)
	for i, it := range out {
		if strings.Contains(it.Producer, ungroundedSuffix) {
			t.Errorf("item %d tagged ungrounded without line cite: %q", i, it.Producer)
		}
	}
}

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

// TestLooksLikeCodeIdentifier_StructuralRules is the over-fitting
// audit test for the filter that replaces the hardcoded stopword
// list. Each case isolates one structural rule; if a rule drifts
// only the relevant case fails.
//
// Cases explicitly cover:
//   - CamelCase / lowerCamelCase / snake_case / SCREAMING_SNAKE
//     as positive-by-shape (no graph needed)
//   - common English prose words rejected on structure alone
//   - lowercase token accepted iff the repo graph knows it
//   - lowercase token rejected when the graph does NOT know it
//
// The final two cases together are the anti-stopword-list
// contract — no word is universally rejected; the repo decides.
func TestLooksLikeCodeIdentifier_StructuralRules(t *testing.T) {
	cases := []struct {
		tok   string
		graph *repomap.Graph
		want  bool
		why   string
	}{
		{"ContinuationPrompt", nil, true, "CamelCase"},
		{"readSet", nil, true, "lowerCamelCase"},
		{"read_file", nil, true, "snake_case"},
		{"HTTP_STATUS", nil, true, "SCREAMING_SNAKE"},
		{"return", nil, false, "lowercase prose word, no graph"},
		{"method", nil, false, "lowercase prose word, no graph"},
		{"function", nil, false, "lowercase prose word, no graph"},
		{
			"explorer",
			&repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{
				"explorer": {{Name: "explorer"}},
			}},
			true,
			"lowercase accepted because repo graph defines it",
		},
		{
			"explorer",
			&repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}},
			false,
			"lowercase rejected when the repo graph does NOT define it",
		},
	}
	for _, c := range cases {
		if got := looksLikeCodeIdentifier(c.tok, c.graph); got != c.want {
			t.Errorf("%s: looksLikeCodeIdentifier(%q) = %v, want %v",
				c.why, c.tok, got, c.want)
		}
	}
}
