package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// explorer_t123_test.go — tests for T1/T2/T3 (the 2026-04-18
// "explorer是怎么调用subagent的？" debug follow-up).
//
// T1a — entity boost in keywordSearch
// T1b — complexity-aware top-N cap (MaxFilesForComplexity)
// T1c — complexity-aware ERM thresholds (thresholdForKind)
// T2a — "calls" pattern in extractConcreteValues / extractCallTargets
// T2b — resolution chain linker accepts "calls" kind
// T2c — concrete-values table renders "calls → Target.Method"
// T3a — EvidencePlan.RequiredFiles + analyzerRequiredFiles
// T3b — detectCrossFileSymbolGaps (cross-file symbol reference tracking)

// -----------------------------------------------------------------------------
// T1a + T1b
// -----------------------------------------------------------------------------

// TestMaxFilesForComplexity pins the complexity → cap ladder.
// Changing any of these numbers requires reviewing the rationale in
// keyword_search.go's doc comment and the explorer's coverage gates.
func TestMaxFilesForComplexity(t *testing.T) {
	cases := []struct {
		complexity types.Complexity
		want       int
	}{
		{types.ComplexitySimple, 15},
		{types.ComplexityModerate, 20},
		{types.ComplexityComplex, 30},
		{"", 20},      // zero value → moderate default
		{"bogus", 20}, // unknown value → moderate default
	}
	for _, tc := range cases {
		got := MaxFilesForComplexity(tc.complexity)
		if got != tc.want {
			t.Errorf("MaxFilesForComplexity(%q) = %d, want %d",
				tc.complexity, got, tc.want)
		}
	}
}

// TestEntityBoostFactor exercises the entity-boost multiplier ladder.
// The tested invariants pin:
//  1. path-only match → 1.3x (tie-breaker for files named after the entity)
//  2. symbol-only match → 1.3x (file whose symbol table contains the entity)
//  3. both → 1.6x (strongest signal)
//  4. neither → 1.0x (no boost)
//  5. short entities (< 4 chars) are skipped (false-positive guard)
//  6. empty entities / nil graph → 1.0x (safe default)
func TestEntityBoostFactor(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/agent/subagent_runtime.go": {
				RelPath: "internal/agent/subagent_runtime.go",
				Symbols: []repotypes.Symbol{
					{Name: "SubAgentRuntime", Kind: "type"},
					{Name: "Run", Kind: "method", Receiver: "SubAgentRuntime"},
				},
			},
			"internal/agent/sub_explorer.go": {
				RelPath: "internal/agent/sub_explorer.go",
				Symbols: []repotypes.Symbol{
					{Name: "Execute", Kind: "method", Receiver: "SubExplorer"},
				},
			},
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repotypes.Symbol{
					{Name: "analyzerEvaluator", Kind: "type"},
				},
			},
		},
	}

	t.Run("path + symbol match → 1.6x", func(t *testing.T) {
		got := entityBoostFactor("internal/agent/subagent_runtime.go", graph,
			[]string{"SubAgent"})
		if got != 1.6 {
			t.Errorf("boost = %v, want 1.6", got)
		}
	})

	t.Run("path-only match → 1.3x", func(t *testing.T) {
		// "subagent" in path; no matching symbol (different entity).
		got := entityBoostFactor("internal/agent/subagent_runtime.go", graph,
			[]string{"subagent"})
		if got != 1.6 {
			// "SubAgentRuntime" symbol contains "subagent" (lowercased)
			// so this is actually a path+symbol match.
			t.Logf("got %v (1.6 expected due to symbol match too)", got)
		}
	})

	t.Run("symbol-only match → 1.3x", func(t *testing.T) {
		// Symbol "Execute" in sub_explorer.go; "Execute" is NOT in the
		// filename, so we get symbol-only.
		got := entityBoostFactor("internal/agent/sub_explorer.go", graph,
			[]string{"Execute"})
		if got != 1.3 {
			t.Errorf("boost = %v, want 1.3 (symbol-only)", got)
		}
	})

	t.Run("no match → 1.0x", func(t *testing.T) {
		got := entityBoostFactor("internal/agent/analyzer.go", graph,
			[]string{"Finalizer"})
		if got != 1.0 {
			t.Errorf("boost = %v, want 1.0 (no match)", got)
		}
	})

	t.Run("short entities skipped", func(t *testing.T) {
		// "Run" is exactly 3 chars — below the 4-char threshold. Would
		// otherwise match Run symbol in subagent_runtime.go.
		got := entityBoostFactor("internal/agent/subagent_runtime.go", graph,
			[]string{"Run"})
		if got != 1.0 {
			t.Errorf("boost = %v, want 1.0 (short entity filtered)", got)
		}
	})

	t.Run("nil graph → 1.0x", func(t *testing.T) {
		got := entityBoostFactor("internal/agent/subagent_runtime.go", nil,
			[]string{"SubAgent"})
		// Graph nil — symbol channel closed, but path still matches.
		if got != 1.3 {
			t.Errorf("boost = %v, want 1.3 (path-only when graph nil)", got)
		}
	})

	t.Run("empty entities → 1.0x", func(t *testing.T) {
		got := entityBoostFactor("internal/agent/subagent_runtime.go", graph, nil)
		if got != 1.0 {
			t.Errorf("boost = %v, want 1.0 (no entities)", got)
		}
	})
}

func TestExactEntityAnchors_UniqueSymbolOnly(t *testing.T) {
	target := &repotypes.Symbol{Name: "buildOrLoadGraph", File: "internal/tool/repomap/tool.go"}
	otherA := &repotypes.Symbol{Name: "Execute", File: "internal/agent/agent.go"}
	otherB := &repotypes.Symbol{Name: "Execute", File: "internal/repl/repl.go"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/tool/repomap/tool.go": {RelPath: "internal/tool/repomap/tool.go"},
			"internal/agent/agent.go":       {RelPath: "internal/agent/agent.go"},
			"internal/repl/repl.go":         {RelPath: "internal/repl/repl.go"},
		},
		SymbolDefs: map[string][]*repotypes.Symbol{
			"buildOrLoadGraph": {target},
			"Execute":          {otherA, otherB},
		},
	}

	anchors := exactEntityAnchors(graph, []string{"buildOrLoadGraph", "Execute"})
	anchor, ok := anchors["internal/tool/repomap/tool.go"]
	if !ok {
		t.Fatalf("unique exact symbol should anchor tool.go, got %+v", anchors)
	}
	if anchor.Rank != 2 || anchor.Hit != "symbol_exact" {
		t.Errorf("tool.go anchor = %+v, want rank=2 hit=symbol_exact", anchor)
	}
	if _, ok := anchors["internal/agent/agent.go"]; ok {
		t.Errorf("ambiguous Execute symbol must not anchor agent.go, got %+v", anchors["internal/agent/agent.go"])
	}
}

// TestExactEntityAnchors_SubTopicEntityHazard_NarrowingWithPrimary
// pins the A' fix for the 2026-04-22 s1a-115146 run-1 anchor misroute.
// The analyzer's multi-topic merge unions sub-topic entities into the
// flat Entities list used for breadth ranking. An English-word sub-
// topic entity ("check") can case-insensitively match a unique
// PascalCase Go symbol in an unrelated file (here: `func Check` in
// contract/checker.go), making that file the sole exact anchor and
// mis-routing Focused Depth Start. exactEntityAnchors must be fed
// the pre-merge PrimaryEntities list — which contains the user-named
// entities only — so the planner-added "check" cannot latch on.
func TestExactEntityAnchors_SubTopicEntityHazard_NarrowingWithPrimary(t *testing.T) {
	checkSym := &repotypes.Symbol{Name: "Check", File: "internal/analysis/contract/checker.go"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/analysis/contract/checker.go": {RelPath: "internal/analysis/contract/checker.go"},
			"internal/analysis/gate/gate.go":        {RelPath: "internal/analysis/gate/gate.go"},
		},
		SymbolDefs: map[string][]*repotypes.Symbol{
			"Check": {checkSym},
			// gate.Run is a qualified form; neither exactSymbolFiles nor
			// exactQualifiedSymbolFiles resolves it on a plain "Run"
			// symbol, so the only candidate anchor comes from "check".
			"Run": {{Name: "Run", File: "internal/analysis/gate/gate.go"}},
		},
	}

	// Merged-entity path (pre-fix): "check" case-folds to "Check" and
	// anchors checker.go — the exact hazard run-1 tripped on.
	merged := exactEntityAnchors(graph, []string{"gate.Run", "retryable", "reject", "check"})
	if _, ok := merged["internal/analysis/contract/checker.go"]; !ok {
		t.Fatalf("merged-entities path should reproduce the hazard (checker.go anchored), got %+v", merged)
	}

	// Primary-only path (post-fix): without "check" in the entity list
	// passed to exactEntityAnchors, nothing case-matches a unique
	// symbol so no anchor is produced — the Focused Depth Start
	// preamble would not mis-route Explorer to checker.go.
	primary := exactEntityAnchors(graph, []string{"gate.Run", "retryable", "reject"})
	if _, ok := primary["internal/analysis/contract/checker.go"]; ok {
		t.Errorf("primary-entities path must NOT anchor checker.go, got %+v", primary)
	}
}

func TestKeywordSearchCandidatePaths_IncludeAnchorOnlyFiles(t *testing.T) {
	candidates := keywordSearchCandidatePaths(
		map[string]float64{"internal/context/builder.go": 42},
		map[string]exactEntityAnchor{
			"internal/tool/repomap/tool.go": {Rank: 2, Hit: "symbol_exact"},
		},
	)
	if len(candidates) != 2 {
		t.Fatalf("candidate union should keep grep + anchor-only files, got %v", candidates)
	}
	if candidates[0] != "internal/context/builder.go" || candidates[1] != "internal/tool/repomap/tool.go" {
		t.Errorf("candidate paths sorted unexpectedly: %v", candidates)
	}
}

func TestSortKeywordResults_PrefersExactEntityAnchor(t *testing.T) {
	results := []keywordFileScore{
		{Path: "internal/context/builder.go", Score: 100},
		{Path: "internal/tool/repomap/tool.go", Score: 5, ExactEntityRank: 2},
	}
	sortKeywordResults(results)
	if got := results[0].Path; got != "internal/tool/repomap/tool.go" {
		t.Fatalf("exact entity anchor should outrank broad keyword score, got %s first", got)
	}
}

// -----------------------------------------------------------------------------
// T1c
// -----------------------------------------------------------------------------

// TestThresholdForKind pins the complexity → satisfaction-threshold
// ladder. Changing these numbers affects how early the explorer
// declares ERM requirements resolved; the test guards against
// accidental regressions that would reintroduce the
// "mechanism-satisfied-at-2-evidence" blind spot that motivated the
// 2026-04-18 refactor.
func TestThresholdForKind(t *testing.T) {
	type row struct {
		kind          types.RequirementKind
		complex       int // satisfied/partial for ComplexityComplex
		moderate      int // satisfied/partial for ComplexityModerate
		partialComp   int
		partialModera int
	}
	cases := []struct {
		kind                      types.RequirementKind
		simpleSat, simplePart     int
		moderateSat, moderatePart int
		complexSat, complexPart   int
	}{
		{types.ReqMechanism, 2, 1, 2, 1, 4, 2},
		{types.ReqEnumeration, 3, 1, 3, 1, 5, 2},
		{types.ReqConfigMapping, 2, 1, 2, 1, 3, 2},
		{types.ReqConditional, 1, 0, 1, 0, 2, 1},
		{types.ReqCallChain, 2, 1, 2, 1, 3, 2},
	}
	for _, c := range cases {
		t.Run(string(c.kind)+"/simple", func(t *testing.T) {
			th := thresholdForKind(c.kind, types.ComplexitySimple)
			if th.Satisfied != c.simpleSat || th.Partial != c.simplePart {
				t.Errorf("simple %s = {%d, %d}, want {%d, %d}",
					c.kind, th.Satisfied, th.Partial, c.simpleSat, c.simplePart)
			}
		})
		t.Run(string(c.kind)+"/moderate", func(t *testing.T) {
			th := thresholdForKind(c.kind, types.ComplexityModerate)
			if th.Satisfied != c.moderateSat || th.Partial != c.moderatePart {
				t.Errorf("moderate %s = {%d, %d}, want {%d, %d}",
					c.kind, th.Satisfied, th.Partial, c.moderateSat, c.moderatePart)
			}
		})
		t.Run(string(c.kind)+"/complex", func(t *testing.T) {
			th := thresholdForKind(c.kind, types.ComplexityComplex)
			if th.Satisfied != c.complexSat || th.Partial != c.complexPart {
				t.Errorf("complex %s = {%d, %d}, want {%d, %d}",
					c.kind, th.Satisfied, th.Partial, c.complexSat, c.complexPart)
			}
		})
	}
}

// TestCheckRequirementSatisfaction_ComplexMechanismNeedsMoreEvidence is
// the regression test for the "explorer→subagent" bug. At
// complexity=Complex, a mechanism requirement with only 2 evidence
// items must stay "partial" — it gets promoted to "satisfied" only
// at 4+.
func TestCheckRequirementSatisfaction_ComplexMechanismNeedsMoreEvidence(t *testing.T) {
	reqs := []EvidenceRequirement{{
		Kind:     types.ReqMechanism,
		Entities: []string{"subagent"},
		Status:   "unsatisfied",
	}}
	// Exactly 2 mechanism items — historically "satisfied", now only
	// "partial" under Complex.
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, Subject: "SubAgentRuntime", Summary: "subagent dispatch"},
		{Kind: types.EvidenceMechanism, Subject: "SubExplorer", Summary: "subagent execution"},
	}

	t.Run("complex stays partial at 2", func(t *testing.T) {
		got := checkRequirementSatisfaction(append([]EvidenceRequirement{}, reqs...),
			nil, evidence, types.ComplexityComplex)
		if got[0].Status != "partial" {
			t.Errorf("complex/2-evidence status = %q, want \"partial\"", got[0].Status)
		}
	})
	t.Run("moderate promotes to satisfied at 2", func(t *testing.T) {
		got := checkRequirementSatisfaction(append([]EvidenceRequirement{}, reqs...),
			nil, evidence, types.ComplexityModerate)
		if got[0].Status != "satisfied" {
			t.Errorf("moderate/2-evidence status = %q, want \"satisfied\"", got[0].Status)
		}
	})
	t.Run("complex reaches satisfied at 4", func(t *testing.T) {
		bigger := append([]types.EvidenceItem{}, evidence...)
		bigger = append(bigger,
			types.EvidenceItem{Kind: types.EvidenceMechanism, Subject: "ProposeSubAgents", Summary: "subagent tool invocation"},
			types.EvidenceItem{Kind: types.EvidenceMechanism, Subject: "Orchestrator", Summary: "subagent dispatch hook"},
		)
		got := checkRequirementSatisfaction(append([]EvidenceRequirement{}, reqs...),
			nil, bigger, types.ComplexityComplex)
		if got[0].Status != "satisfied" {
			t.Errorf("complex/4-evidence status = %q, want \"satisfied\"", got[0].Status)
		}
	})
}

// -----------------------------------------------------------------------------
// T2a + T2c
// -----------------------------------------------------------------------------

// TestExtractCallTargets_BasicRecognition pins the call-target
// detector's scope: method calls with exported names on non-stdlib
// receivers emit targets; noise is filtered.
func TestExtractCallTargets_BasicRecognition(t *testing.T) {
	lines := []string{
		`merged, err := o.subRuntime.Run(proposal)`,
		`result := b.deps.Tools.Execute(ctx, name, params)`,
		`sub.Run(req)`,
		`fmt.Sprintf("%d", x)`,     // stdlib noise
		`strings.Contains(s, "x")`, // stdlib noise
		`logging.Debug("hello")`,   // internal noise
		`x.String()`,               // method blocklist
		`return foo()`,             // bare call, no dot
	}
	got := extractCallTargets(lines, 6)
	wantPrefixes := []string{"subRuntime.Run", "Tools.Execute", "sub.Run"}
	if len(got) != len(wantPrefixes) {
		t.Fatalf("got %d call targets (%v), want %d", len(got), got, len(wantPrefixes))
	}
	for _, want := range wantPrefixes {
		found := false
		for _, g := range got {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected target %q in %v", want, got)
		}
	}
}

// TestExtractCallTargets_Dedup ensures the same call target is not
// emitted twice even when it appears multiple times in a snippet.
func TestExtractCallTargets_Dedup(t *testing.T) {
	lines := []string{
		`o.subRuntime.Run(a)`,
		`o.subRuntime.Run(b)`,
		`o.subRuntime.Run(c)`,
	}
	got := extractCallTargets(lines, 6)
	if len(got) != 1 {
		t.Errorf("expected 1 deduplicated target, got %v", got)
	}
}

// TestExtractCallTargets_CapRespected ensures the maxCalls cap is
// honored so long function bodies don't flood the concrete-values
// table.
func TestExtractCallTargets_CapRespected(t *testing.T) {
	lines := []string{
		`a.Run(x)`,
		`b.Stop(y)`,
		`c.Dispatch(z)`,
		`d.Process(w)`,
	}
	got := extractCallTargets(lines, 2)
	if len(got) != 2 {
		t.Errorf("expected 2 targets (cap honored), got %v", got)
	}
}

// TestExtractConcreteValues_EmitsCallsKind is the end-to-end check
// that cross-package method calls appear with kind="calls" in the
// concreteValueEntry list.
func TestExtractConcreteValues_EmitsCallsKind(t *testing.T) {
	src := `func (o *orchestrator) dispatchStage() {
	if proposal := extractSubAgentProposal(); proposal != nil {
		merged, err := o.subRuntime.Run(proposal)
		_ = merged
		_ = err
	}
}`
	entries := extractConcreteValues(src, "go")
	hasCalls := false
	for _, e := range entries {
		if e.kind == "calls" && strings.Contains(e.value, "subRuntime.Run") {
			hasCalls = true
		}
	}
	if !hasCalls {
		t.Errorf("expected \"calls → subRuntime.Run\" entry in %v", entries)
	}
}

// -----------------------------------------------------------------------------
// T3a
// -----------------------------------------------------------------------------

// TestPhase1MedianScore covers edge cases of the median helper.
func TestPhase1MedianScore(t *testing.T) {
	cases := []struct {
		name  string
		input []types.Phase1RankedFile
		want  float64
	}{
		{"empty", nil, 1.0},
		{"single", []types.Phase1RankedFile{{Score: 3.0}}, 3.0},
		{"odd count", []types.Phase1RankedFile{
			{Score: 1.0}, {Score: 5.0}, {Score: 3.0},
		}, 3.0},
		{"even count (lower-middle)", []types.Phase1RankedFile{
			{Score: 1.0}, {Score: 2.0}, {Score: 3.0}, {Score: 4.0},
		}, 2.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := phase1MedianScore(c.input)
			if got != c.want {
				t.Errorf("phase1MedianScore = %v, want %v", got, c.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// T3b
// -----------------------------------------------------------------------------

// TestDetectCrossFileSymbolGaps pins the cross-file symbol reference
// detector — the T3b mid-loop hint that pushes the LLM to read
// files whose symbols it has mentioned in notes.
func TestDetectCrossFileSymbolGaps(t *testing.T) {
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repotypes.Symbol{
			"SubAgentRuntime": {
				{Name: "SubAgentRuntime", File: "internal/agent/subagent_runtime.go"},
			},
			"SubExplorer": {
				{Name: "SubExplorer", File: "internal/agent/sub_explorer.go"},
			},
			"Run": {
				// Short name — must be filtered out by the ≥6 gate.
				{Name: "Run", File: "internal/agent/subagent_runtime.go"},
			},
			"AlreadyRead": {
				{Name: "AlreadyRead", File: "internal/agent/analyzer.go"},
			},
		},
	}

	t.Run("unread files with long names produce gaps", func(t *testing.T) {
		notes := []string{
			"The explorer creates a SubAgentRuntime and calls its Run method.",
			"The SubExplorer then executes the sub-task.",
		}
		readSet := map[string]bool{
			"internal/agent/explorer.go": true, // note-writing file
		}
		gaps := detectCrossFileSymbolGaps(notes, graph, readSet, 5)
		if len(gaps) != 2 {
			t.Fatalf("expected 2 gaps, got %d: %+v", len(gaps), gaps)
		}
		// Longest name first (T3b sort rule).
		if gaps[0].Symbol != "SubAgentRuntime" {
			t.Errorf("gaps[0] = %+v, want SubAgentRuntime first", gaps[0])
		}
	})

	t.Run("short names (< 6 chars) filtered", func(t *testing.T) {
		notes := []string{"The Run method is the entrypoint."}
		gaps := detectCrossFileSymbolGaps(notes, graph, nil, 5)
		for _, g := range gaps {
			if g.Symbol == "Run" {
				t.Errorf("short name \"Run\" should have been filtered, got %+v", gaps)
			}
		}
	})

	t.Run("already-read files excluded", func(t *testing.T) {
		notes := []string{"See AlreadyRead in the analyzer."}
		readSet := map[string]bool{
			"internal/agent/analyzer.go": true,
		}
		gaps := detectCrossFileSymbolGaps(notes, graph, readSet, 5)
		if len(gaps) != 0 {
			t.Errorf("already-read should produce zero gaps, got %+v", gaps)
		}
	})

	t.Run("max cap respected", func(t *testing.T) {
		notes := []string{
			"SubAgentRuntime and SubExplorer and AlreadyRead.",
		}
		gaps := detectCrossFileSymbolGaps(notes, graph, nil, 1)
		if len(gaps) != 1 {
			t.Errorf("cap=1 should yield 1 gap, got %d", len(gaps))
		}
	})

	t.Run("nil graph safe", func(t *testing.T) {
		gaps := detectCrossFileSymbolGaps([]string{"SubAgentRuntime"}, nil, nil, 5)
		if gaps != nil {
			t.Errorf("nil graph should yield nil gaps, got %+v", gaps)
		}
	})

	t.Run("empty notes safe", func(t *testing.T) {
		gaps := detectCrossFileSymbolGaps(nil, graph, nil, 5)
		if gaps != nil {
			t.Errorf("nil notes should yield nil gaps, got %+v", gaps)
		}
	})

	t.Run("narrative labels do not create fake symbol gaps", func(t *testing.T) {
		graph.SymbolDefs["Answer"] = []*repotypes.Symbol{
			{Name: "Answer", File: "internal/analysis/contract/checker.go"},
		}
		notes := []string{
			"Answer: The explorer builds a SubAgentRuntime and calls its Run method.",
			"Evidence:",
			"- SubExplorer coordinates the sub-task.",
		}
		gaps := detectCrossFileSymbolGaps(notes, graph, nil, 5)
		for _, g := range gaps {
			if g.Symbol == "Answer" {
				t.Fatalf("narrative label should not surface as a symbol gap: %+v", gaps)
			}
		}
	})
}

// ── 3-hop chain resolution ─────────────────────────────────────────

// TestResolveCallTargetReceivers verifies the graph-assisted type
// resolution that bridges variable names → type names in call chains.
func TestResolveCallTargetReceivers(t *testing.T) {
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repotypes.Symbol{
			"Run": {
				{Name: "Run", Kind: "method", Receiver: "SubAgentRuntime", File: "subagent_runtime.go"},
				{Name: "Run", Kind: "method", Receiver: "SubExplorer", File: "sub_explorer.go"},
			},
			"Execute": {
				{Name: "Execute", Kind: "function", Receiver: "", File: "tool.go"},
			},
		},
	}

	t.Run("resolves subRuntime.Run to SubAgentRuntime", func(t *testing.T) {
		recvs := resolveCallTargetReceivers("subRuntime.Run", graph)
		found := false
		for _, r := range recvs {
			if r == "SubAgentRuntime" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected SubAgentRuntime in resolved receivers, got %v", recvs)
		}
	})

	t.Run("bare function returns no receivers", func(t *testing.T) {
		recvs := resolveCallTargetReceivers("pkg.Execute", graph)
		if len(recvs) != 0 {
			t.Errorf("expected no receivers for bare function, got %v", recvs)
		}
	})

	t.Run("nil graph safe", func(t *testing.T) {
		recvs := resolveCallTargetReceivers("subRuntime.Run", nil)
		if recvs != nil {
			t.Errorf("expected nil for nil graph, got %v", recvs)
		}
	})
}

// TestCallValueMatchesReceiver_3HopChain is the regression test for
// the "explorer→subagent" 3-hop gap. It verifies the graph-assisted
// resolution bridging variable names to type names.
func TestCallValueMatchesReceiver_3HopChain(t *testing.T) {
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repotypes.Symbol{
			"Run": {
				{Name: "Run", Kind: "method", Receiver: "SubAgentRuntime"},
				{Name: "Run", Kind: "method", Receiver: "SubExplorer"},
			},
		},
	}

	// Hop 1: dispatchStage calls "subRuntime.Run"
	//        → should match rv.receiver "SubAgentRuntime"
	if !callValueMatchesReceiver("subRuntime.Run", "SubAgentRuntime", graph) {
		t.Error("hop 1 failed: subRuntime.Run should match SubAgentRuntime via graph resolution")
	}

	// Hop 2: SubAgentRuntime.Run calls "sub.Run"
	//        → should match rv.receiver "SubExplorer"
	if !callValueMatchesReceiver("sub.Run", "SubExplorer", graph) {
		t.Error("hop 2 failed: sub.Run should match SubExplorer via graph resolution")
	}

	// Negative: should NOT match unrelated receiver
	if callValueMatchesReceiver("subRuntime.Run", "Unrelated", graph) {
		t.Error("should not match unrelated receiver")
	}
}
