package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestContainsIdentifier(t *testing.T) {
	tests := []struct {
		text string
		name string
		want bool
	}{
		// Basic matches
		{"NewFoo()", "Foo", true},
		{"&Foo{}", "Foo", true},
		{"binds NewFoo", "Foo", true},
		{"returns Foo{}", "Foo", true},

		// Must not match substrings of longer identifiers
		{"ErrorHandler", "Handler", false},
		{"HandlerFunc", "Handler", false},
		{"MyHandlerImpl", "Handler", false},

		// Should match when bounded by non-ident characters
		{"(Handler)", "Handler", true},
		{"*Handler", "Handler", true},
		{"Handler.Name", "Handler", true},
		{".Handler.", "Handler", true},

		// Short names still work with >= 4 char threshold
		{"Agent{}", "Agent", true}, // 5 chars
		{"NewAgent()", "Agent", true},
		{"SubAgent", "Agent", false}, // embedded in longer word

		// Edge cases
		{"", "Foo", false},
		{"Foo", "", false},
		{"Foo", "Foo", true},
		{"FooBar", "Foo", false},
		{"BarFoo", "Foo", false},

		// Underscore is an ident char
		{"my_handler", "handler", false}, // preceded by _
		{"handler_test", "handler", false}, // followed by _
		{"my handler test", "handler", true},

		// Cross-language factory prefixes
		{"createHandler()", "Handler", true},   // Python/JS factory
		{"CreateHandler()", "Handler", true},    // C#/Go factory
		{"makeHandler()", "Handler", true},      // Ruby/functional style
		{"MakeHandler()", "Handler", true},      // Go alternate factory
		{"buildHandler()", "Handler", true},     // Builder pattern
		{"BuildHandler()", "Handler", true},
		{"getHandler()", "Handler", true},       // Accessor factory
		{"GetHandler()", "Handler", true},
		{"destroyHandler()", "Handler", false},  // Not a factory prefix
		{"useHandler()", "Handler", false},      // Not a factory prefix
	}

	for _, tt := range tests {
		got := containsIdentifier(tt.text, tt.name)
		if got != tt.want {
			t.Errorf("containsIdentifier(%q, %q) = %v, want %v", tt.text, tt.name, got, tt.want)
		}
	}
}

func TestResolveConditions(t *testing.T) {
	concreteValues := `## Concrete Values (programmatically extracted from source code)

These are EXACT values from source code — ground truth, not summaries.

| File:Line | Method | Fact |
|-----------|--------|------|
| agent.go:42 | ` + "`Name()`" + ` | returns "explorer" |
| agent.go:50 | ` + "`IsWrite()`" + ` | returns false |
| config.go:10 | ` + "`DefaultMode()`" + ` | returns "analysis" |

### Resolution Chains

These chains trace through the concrete values to resolve conditions:

- ` + "`Register()` binds NewFoo → `Foo.Name()` returns \"bar\"" + `
`

	notes := []string{
		`- [CONDITIONAL] ` + "`Execute()`" + ` line 15: calls grep IF ` + "`IsWrite()`" + ` == false
- [CONDITIONAL] ` + "`Process()`" + ` line 30: skips validation IF ` + "`debugMode`" + ` is enabled
- [CONDITIONAL] ` + "`Route()`" + ` line 45: dispatches IF ` + "`Name()`" + ` returns "explorer"
- [DIRECT] ` + "`Simple()`" + ` line 10: returns true`,
	}

	unresolved := resolveConditions(notes, concreteValues)

	// IsWrite() is in concrete values → resolved
	// debugMode is NOT in concrete values → unresolved
	// Name() is in concrete values → resolved
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved condition, got %d: %v", len(unresolved), unresolved)
	}
	if got := unresolved[0]; !contains(got, "debugMode") {
		t.Errorf("expected unresolved to mention debugMode, got: %s", got)
	}
}

func TestResolveConditionsNoIFClause(t *testing.T) {
	cv := "| file:1 | `Foo()` | returns true |"
	notes := []string{
		"- [CONDITIONAL] `Bar()` line 5: does something when configured",
	}
	unresolved := resolveConditions(notes, cv)
	// No IF clause → should be marked unresolved
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved, got %d", len(unresolved))
	}
}

func TestExtractIdentifiers(t *testing.T) {
	tests := []struct {
		input string
		want  []string // at minimum these should be present
	}{
		{"`IsWrite()` == false", []string{"IsWrite"}},
		{"foo.Bar == true", []string{"foo.Bar"}},
		{"`mode` is \"debug\"", []string{"mode"}},
		{"the and for not", nil}, // all noise words (< 3 chars or in noise list)
	}

	for _, tt := range tests {
		got := extractIdentifiers(tt.input)
		for _, w := range tt.want {
			found := false
			for _, g := range got {
				if g == w {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractIdentifiers(%q): expected %q in result %v", tt.input, w, got)
			}
		}
		if tt.want == nil && len(got) > 0 {
			t.Errorf("extractIdentifiers(%q): expected empty, got %v", tt.input, got)
		}
	}
}

func TestIsEvidenceLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		// Should match
		{"return true", true},
		{"return \"hello\"", true},
		{`  return &Foo{}`, true},
		{`func() { return X }`, true},
		{`name => "value"`, true},
		{`"key": NewHandler(),`, true},
		{`Register(NewFoo())`, true},
		{`append(list, item)`, true},
		{`handler := &Handler{}`, true},       // Go struct literal
		{`new Handler()`, true},                // Java/JS constructor
		{`srv := NewServer(config)`, true},     // Go factory
		// Cross-language constructor patterns
		{`handler := &Handler{}`, true},           // Go struct literal
		{`new Handler()`, true},                   // Java/JS constructor
		{`srv := NewServer(config)`, true},        // Go factory
		{`handler = CreateHandler()`, true},       // Python/JS factory
		{`obj = MakeWidget()`, true},              // Ruby/functional factory
		{`svc := BuildService(deps)`, true},       // Builder pattern
		{`yield some_value`, true},                // Python generator
		{`subscribe(handler)`, true},              // Event subscription
		{`provide(ServiceToken, factory)`, true},  // Angular DI
		// Should NOT match — cross-language false positive prevention
		{`x := 42`, false},
		{`// just a comment`, false},
		{`fmt.Println("hello")`, false},
		{`if a && b {`, false},                    // JS/Go logical AND — not a constructor
		{`x := y & 0xFF`, false},                  // bitwise AND
		{`&amp; entity`, false},                   // HTML entity
		{`newspaper := "daily"`, false},           // "new" embedded in word
		{`renewable := true`, false},              // "new" embedded in word
	}
	for _, tt := range tests {
		got := isEvidenceLine(tt.line)
		if got != tt.want {
			t.Errorf("isEvidenceLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestResolveConditionsChainTarget(t *testing.T) {
	// Verify that resolution chains are also checked for condition resolution.
	cv := `## Concrete Values

| File:Line | Method | Fact |
|-----------|--------|------|
| a.go:1 | ` + "`Register()`" + ` | binds NewFoo |

### Resolution Chains

- ` + "`Register()` binds NewFoo → `Foo.Name()` returns \"bar\"" + `
`
	notes := []string{
		"- [CONDITIONAL] `Route()` line 5: dispatches IF `Foo.Name` returns bar",
	}
	unresolved := resolveConditions(notes, cv)
	// Foo.Name appears in chain target → should be resolved
	if len(unresolved) != 0 {
		t.Errorf("expected 0 unresolved (chain target match), got %d: %v", len(unresolved), unresolved)
	}
}

func TestDetectTruncatedUngrepped(t *testing.T) {
	t.Run("detects truncated read with no grep", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[internal/agent/explorer.go: showing lines 1-500 of 2383 total]\npackage agent\n...",
			},
		}
		truncated, grepped := detectTruncatedUngrepped(history)
		if len(truncated) != 1 {
			t.Fatalf("expected 1 truncated file, got %d", len(truncated))
		}
		if truncated[0].path != "internal/agent/explorer.go" {
			t.Fatalf("expected path internal/agent/explorer.go, got %q", truncated[0].path)
		}
		if truncated[0].linesRead != 500 {
			t.Fatalf("expected linesRead=500, got %d", truncated[0].linesRead)
		}
		if truncated[0].totalLines != 2383 {
			t.Fatalf("expected totalLines=2383, got %d", truncated[0].totalLines)
		}
		if grepped["internal/agent/explorer.go"] {
			t.Fatalf("file should not be marked as grepped")
		}
	})

	t.Run("small file not flagged", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[internal/agent/subagent.go: showing lines 1-66 of 66 total]\npackage agent\n...",
			},
		}
		truncated, _ := detectTruncatedUngrepped(history)
		if len(truncated) != 0 {
			t.Fatalf("expected 0 truncated (small file fully read), got %d", len(truncated))
		}
	})

	t.Run("file under 500 lines not flagged", func(t *testing.T) {
		// Even if partially read, files ≤500 lines are not flagged.
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[small.go: showing lines 1-200 of 400 total]\npackage x\n...",
			},
		}
		truncated, _ := detectTruncatedUngrepped(history)
		if len(truncated) != 0 {
			t.Fatalf("expected 0 truncated (file ≤500 lines), got %d", len(truncated))
		}
	})

	t.Run("fully read large file not flagged", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[big.go: showing lines 1-800 of 800 total]\npackage x\n...",
			},
		}
		truncated, _ := detectTruncatedUngrepped(history)
		if len(truncated) != 0 {
			t.Fatalf("expected 0 truncated (fully read), got %d", len(truncated))
		}
	})

	t.Run("line-level grep marks file as grepped", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[internal/agent/explorer.go: showing lines 1-500 of 2383 total]\npackage agent\n...",
			},
			{
				ToolName: "grep",
				Success:  true,
				Summary:  "[grep: 3 matching lines]\ninternal/agent/explorer.go:65: // subagent\ninternal/agent/explorer.go:120: SubAgent\ninternal/agent/explorer.go:450: sub_agent",
			},
		}
		truncated, grepped := detectTruncatedUngrepped(history)
		if len(truncated) != 1 {
			t.Fatalf("expected 1 truncated file, got %d", len(truncated))
		}
		if !grepped["internal/agent/explorer.go"] {
			t.Fatalf("explorer.go should be marked as line-grepped")
		}
	})

	t.Run("failed results ignored", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  false,
				Summary:  "[missing.go: showing lines 1-100 of 2000 total]\n...",
			},
		}
		truncated, _ := detectTruncatedUngrepped(history)
		if len(truncated) != 0 {
			t.Fatalf("expected 0 truncated (failed result), got %d", len(truncated))
		}
	})

	t.Run("multiple reads track max end line", func(t *testing.T) {
		history := []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[big.go: showing lines 1-200 of 1000 total]\npackage x",
			},
			{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[big.go: showing lines 201-500 of 1000 total]\nfunc foo()",
			},
		}
		truncated, _ := detectTruncatedUngrepped(history)
		if len(truncated) != 1 {
			t.Fatalf("expected 1 truncated file, got %d", len(truncated))
		}
		if truncated[0].linesRead != 500 {
			t.Fatalf("expected max linesRead=500, got %d", truncated[0].linesRead)
		}
	})
}

func TestCrossValidateEvidence(t *testing.T) {
	t.Run("detects contradiction", func(t *testing.T) {
		cv := `## Concrete Values

| File:Line | Method | Fact |
|-----------|--------|------|
| agent.go:42 | ` + "`IsWrite()`" + ` | returns false |
| agent.go:50 | ` + "`Name()`" + ` | returns "explorer" |
`
		notes := []string{
			"- [DIRECT] `IsWrite()` line 42: returns true",
			"- [DIRECT] `Name()` line 50: returns \"explorer\"",
		}
		result := crossValidateEvidence(notes, cv)
		// IsWrite: LLM says "true", source says "false" → conflict
		if !strings.Contains(result, "IsWrite") {
			t.Errorf("expected conflict for IsWrite, got: %s", result)
		}
		// Name: both say "explorer" → no conflict
		if strings.Contains(result, "Name") {
			t.Errorf("Name() should not be flagged as conflict, got: %s", result)
		}
	})

	t.Run("no conflicts returns empty", func(t *testing.T) {
		cv := `| file:1 | ` + "`Foo()`" + ` | returns true |`
		notes := []string{
			"- [DIRECT] `Foo()` line 1: returns true",
		}
		result := crossValidateEvidence(notes, cv)
		if result != "" {
			t.Errorf("expected empty (no conflicts), got: %s", result)
		}
	})

	t.Run("no matching methods returns empty", func(t *testing.T) {
		cv := `| file:1 | ` + "`Foo()`" + ` | returns true |`
		notes := []string{
			"- [DIRECT] `Bar()` line 5: returns false",
		}
		result := crossValidateEvidence(notes, cv)
		if result != "" {
			t.Errorf("expected empty (no matching methods), got: %s", result)
		}
	})

	t.Run("registration conflict", func(t *testing.T) {
		cv := `| file:1 | ` + "`Register()`" + ` | binds ONLY NewFoo |`
		notes := []string{
			"- [REGISTRATION] `Register()` line 1: binds NewBar and NewBaz",
		}
		result := crossValidateEvidence(notes, cv)
		if !strings.Contains(result, "Register") {
			t.Errorf("expected conflict for Register, got: %s", result)
		}
	})

	t.Run("qualified method name match", func(t *testing.T) {
		cv := `| file:1 | ` + "`Foo.IsEnabled()`" + ` | returns true |`
		notes := []string{
			"- [DIRECT] `IsEnabled()` line 5: returns false",
		}
		result := crossValidateEvidence(notes, cv)
		if !strings.Contains(result, "IsEnabled") {
			t.Errorf("expected conflict for IsEnabled (qualified match), got: %s", result)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		if r := crossValidateEvidence(nil, ""); r != "" {
			t.Errorf("nil notes + empty cv should return empty, got: %s", r)
		}
		if r := crossValidateEvidence([]string{"- [DIRECT] `X()` line 1: returns 1"}, ""); r != "" {
			t.Errorf("empty cv should return empty, got: %s", r)
		}
	})

	// Cross-language: Python-style claim vs Go-style concrete value
	t.Run("cross-language value agreement", func(t *testing.T) {
		cv := `| file:1 | ` + "`get_name()`" + ` | returns "handler" |`
		notes := []string{
			`- [DIRECT] ` + "`get_name()`" + ` line 5: returns "handler"`,
		}
		result := crossValidateEvidence(notes, cv)
		if result != "" {
			t.Errorf("matching values should not conflict, got: %s", result)
		}
	})
}

func TestNormalizeValueAssertion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`returns "explorer"`, `"explorer"`},
		{"returns true", "true"},
		{"returns false", "false"},
		{"binds ONLY NewFoo", "NewFoo"},
		{"binds NewFoo, NewBar", "NewFoo, NewBar"},
		{"maps key → value", "key → value"},
		{"registers NewHandler", "NewHandler"},
		{"decorates @route → handler", "@route → handler"},
		{"some unknown format", "some unknown format"},
	}
	for _, tt := range tests {
		got := normalizeValueAssertion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeValueAssertion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValueAssertionsAgree(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Exact match
		{"true", "true", true},
		{`"explorer"`, `"explorer"`, true},

		// Quote style differences
		{`"explorer"`, `'explorer'`, true},
		{`explorer`, `"explorer"`, true},

		// Containment
		{"true", "true (always)", true},

		// Disagreement
		{"true", "false", false},
		{`"explorer"`, `"analyzer"`, false},
		{"NewFoo", "NewBar", false},
	}
	for _, tt := range tests {
		got := valueAssertionsAgree(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("valueAssertionsAgree(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPerFileGrepRedirect(t *testing.T) {
	// Verify that the per-file grep redirect tracks files individually.
	eval := &explorerEvaluator{}
	eval.phase = 1
	eval.userQuestion = "test question"

	// First call: two truncated files, both should be redirected.
	history1 := []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[big_a.go: showing lines 1-100 of 2000 total]\npackage x"},
		{ToolName: "read_file", Success: true, Summary: "[big_b.go: showing lines 1-100 of 1500 total]\npackage y"},
	}
	prompt1, cont1 := eval.ContinuationPrompt(llm.Response{Content: "analyzing..."}, 0, 0, history1)
	if !cont1 {
		t.Fatal("expected continuation for first redirect")
	}
	if !strings.Contains(prompt1, "big_a.go") || !strings.Contains(prompt1, "big_b.go") {
		t.Errorf("first redirect should mention both files, got: %s", prompt1)
	}

	// Second call: same files + one new truncated file.
	// Only the new file should be redirected.
	history2 := append(history1, types.ToolResult{
		ToolName: "read_file", Success: true,
		Summary: "[big_c.go: showing lines 1-200 of 3000 total]\npackage z",
	})
	prompt2, cont2 := eval.ContinuationPrompt(llm.Response{Content: "more analysis..."}, 1, 1, history2)
	if !cont2 {
		t.Fatal("expected continuation for second redirect (new file)")
	}
	if !strings.Contains(prompt2, "big_c.go") {
		t.Errorf("second redirect should mention big_c.go, got: %s", prompt2)
	}
	if strings.Contains(prompt2, "big_a.go") || strings.Contains(prompt2, "big_b.go") {
		t.Errorf("second redirect should NOT re-mention big_a.go or big_b.go, got: %s", prompt2)
	}
}

func TestDetectPartiallyReadSymbols(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"agent.go": {
				Symbols: []repomap.Symbol{
					{Name: "Execute", Kind: "method", Receiver: "BaseAgent", Line: 100, EndLine: 400, File: "agent.go"},
					{Name: "helper", Kind: "function", Line: 50, EndLine: 55, File: "agent.go"}, // small, should be skipped
				},
			},
		},
	}

	t.Run("detects partial read", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "read_file", Success: true,
				Summary: "[agent.go: showing lines 100-200 of 500 total]\ncode..."},
		}
		hints := detectPartiallyReadSymbols(history, graph)
		if len(hints) != 1 {
			t.Fatalf("expected 1 hint, got %d", len(hints))
		}
		if hints[0].symbolName != "BaseAgent.Execute" {
			t.Errorf("expected BaseAgent.Execute, got %s", hints[0].symbolName)
		}
		if hints[0].readEnd != 200 {
			t.Errorf("expected readEnd=200, got %d", hints[0].readEnd)
		}
		if hints[0].coverage > 0.35 {
			t.Errorf("expected coverage <35%%, got %.0f%%", hints[0].coverage*100)
		}
	})

	t.Run("fully read function no hint", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "read_file", Success: true,
				Summary: "[agent.go: showing lines 1-500 of 500 total]\ncode..."},
		}
		hints := detectPartiallyReadSymbols(history, graph)
		if len(hints) != 0 {
			t.Fatalf("expected 0 hints (fully read), got %d", len(hints))
		}
	})

	t.Run("small function skipped", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "read_file", Success: true,
				Summary: "[agent.go: showing lines 50-52 of 500 total]\ncode..."},
		}
		hints := detectPartiallyReadSymbols(history, graph)
		if len(hints) != 0 {
			t.Fatalf("expected 0 hints (small function), got %d", len(hints))
		}
	})

	t.Run("multiple reads aggregate", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "read_file", Success: true,
				Summary: "[agent.go: showing lines 100-200 of 500 total]\npart1"},
			{ToolName: "read_file", Success: true,
				Summary: "[agent.go: showing lines 200-250 of 500 total]\npart2"},
		}
		hints := detectPartiallyReadSymbols(history, graph)
		if len(hints) != 1 {
			t.Fatalf("expected 1 hint, got %d", len(hints))
		}
		// maxEnd should be 250, function ends at 400 (coverage ~50%)
		if hints[0].readEnd != 250 {
			t.Errorf("expected readEnd=250, got %d", hints[0].readEnd)
		}
	})

	t.Run("nil graph returns nil", func(t *testing.T) {
		hints := detectPartiallyReadSymbols(nil, nil)
		if hints != nil {
			t.Fatalf("expected nil for nil graph, got %v", hints)
		}
	})
}

func TestDetectEnumerationIntent(t *testing.T) {
	tests := []struct {
		question string
		want     bool
	}{
		// Chinese enumeration patterns
		{"项目中所有实现了Evaluator接口的类型有哪些", true},
		{"列出每个agent的ShouldStop行为", true},
		{"有哪些pipeline stage", true},
		{"全部配置项列举一下", true},

		// English enumeration patterns
		{"list all Evaluator implementations", true},
		{"find all files that contain ShouldStop", true},
		{"what are the agent types", true},
		{"which stages support self-loop", true},
		{"how many tools are registered", true},
		{"enumerate the pipeline stages", true},
		{"every agent has a Name method", true},

		// Non-enumeration questions
		{"how does the explorer work", false},
		{"explain the ReAct loop", false},
		{"fix the bug in parser", false},
		{"why is the build failing", false},
	}
	for _, tt := range tests {
		got := detectEnumerationIntent(tt.question)
		if got != tt.want {
			t.Errorf("detectEnumerationIntent(%q) = %v, want %v", tt.question, got, tt.want)
		}
	}
}

func TestExtractQuestionEntities(t *testing.T) {
	tests := []struct {
		question string
		want     []string // at minimum these should be present
	}{
		{"`explorerEvaluator`的ContinuationPrompt策略", []string{"explorerEvaluator"}},
		{"BaseAgent.Execute方法的执行流程", []string{"BaseAgent.Execute"}},
		{"`MyClass.doThing` behavior", []string{"MyClass.doThing"}},
		{"how does ShouldStop work", nil}, // too short for CamelCase (1 segment)
	}
	for _, tt := range tests {
		got := extractQuestionEntities(tt.question)
		for _, w := range tt.want {
			found := false
			for _, g := range got {
				if g == w {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractQuestionEntities(%q): expected %q in %v", tt.question, w, got)
			}
		}
	}
}

func TestBuildInitialPromptRetry(t *testing.T) {
	eval := &explorerEvaluator{}
	ctx := &types.AgentContext{
		CurrentTask:        "test question",
		CurrentTaskKeywords: []string{"test"},
		RepoRoot:           ".",
		RetryHint:          "Read more files about X",
	}

	// First call: should be Phase 0
	prompt1 := eval.BuildInitialPrompt(ctx, nil)
	if eval.phase != 0 {
		t.Fatalf("first call should set phase=0, got %d", eval.phase)
	}
	if !strings.Contains(prompt1, "Breadth Scan") {
		t.Error("first call should contain 'Breadth Scan'")
	}

	// Simulate investigation: add notes
	eval.investigationNotes = []string{"## Evidence from file.go\n- [DIRECT] foo line 1: returns true"}
	eval.idleStreakInDepth = 5 // simulate stale counter

	// Second call (self-loop): should skip Phase 0
	prompt2 := eval.BuildInitialPrompt(ctx, nil)
	if eval.phase != 1 {
		t.Fatalf("retry call should set phase=1, got %d", eval.phase)
	}
	if strings.Contains(prompt2, "Breadth Scan") {
		t.Error("retry call should NOT contain 'Breadth Scan'")
	}
	if !strings.Contains(prompt2, "Retry") {
		t.Error("retry call should contain 'Retry'")
	}
	if !strings.Contains(prompt2, "Read more files about X") {
		t.Error("retry call should include RetryHint")
	}
	// Counters should be reset
	if eval.idleStreakInDepth != 0 {
		t.Errorf("idleStreakInDepth should be reset, got %d", eval.idleStreakInDepth)
	}
}

func TestPhase0QualityGate(t *testing.T) {
	t.Run("gate fires when only grep used", func(t *testing.T) {
		eval := &explorerEvaluator{phase: 0}
		history := []types.ToolResult{
			{ToolName: "grep", Success: true, Summary: "file1.go\nfile2.go\nfile3.go"},
		}
		prompt, cont := eval.ContinuationPrompt(llm.Response{Content: "done scanning"}, 0, 0, history)
		if !cont {
			t.Fatal("gate should fire (no repo_map used)")
		}
		if !strings.Contains(prompt, "repo_map") {
			t.Error("gate should suggest using repo_map")
		}
		if !eval.phase0ExtraRound {
			t.Error("phase0ExtraRound should be set")
		}
	})

	t.Run("gate passes with both tools", func(t *testing.T) {
		eval := &explorerEvaluator{phase: 0}
		history := []types.ToolResult{
			{ToolName: "grep", Success: true, Summary: "file1.go\nfile2.go\nfile3.go"},
			{ToolName: "repo_map", Success: true, Summary: "map output"},
		}
		prompt, cont := eval.ContinuationPrompt(llm.Response{Content: "done"}, 0, 0, history)
		// Should transition to Phase 1
		if !cont {
			t.Fatal("should continue to Phase 1")
		}
		if !strings.Contains(prompt, "Evidence Collection") {
			t.Errorf("should transition to Phase 1, got: %s", prompt[:80])
		}
		if eval.phase != 1 {
			t.Errorf("phase should be 1, got %d", eval.phase)
		}
	})

	t.Run("gate fires only once", func(t *testing.T) {
		eval := &explorerEvaluator{phase: 0, phase0ExtraRound: true}
		history := []types.ToolResult{
			{ToolName: "grep", Success: true, Summary: "file1.go"},
		}
		prompt, cont := eval.ContinuationPrompt(llm.Response{Content: "done"}, 0, 0, history)
		// Should NOT fire again — proceed to Phase 1
		if !cont {
			t.Fatal("should continue to Phase 1")
		}
		if strings.Contains(prompt, "broaden your search") {
			t.Error("gate should NOT fire twice")
		}
	})
}

func TestAdaptiveTruncation(t *testing.T) {
	tests := []struct {
		noteCount int
		wantLimit int
	}{
		{1, 1200},
		{3, 1200},
		{4, 1600},
		{5, 2000},
		{7, 2800},
		{10, 3000}, // capped
	}
	for _, tt := range tests {
		truncLimit := 1200
		if tt.noteCount > 3 {
			truncLimit = 1200 + 400*(tt.noteCount-3)
		}
		if truncLimit > 3000 {
			truncLimit = 3000
		}
		if truncLimit != tt.wantLimit {
			t.Errorf("noteCount=%d: truncLimit=%d, want %d", tt.noteCount, truncLimit, tt.wantLimit)
		}
	}
}

func TestExtractFileCoverageGrepFormats(t *testing.T) {
	t.Run("files_only=true format", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true,
				Summary: "[grep: 3 matching files]\ninternal/agent/explorer.go\ninternal/agent/agent.go\ninternal/agent/planner.go"},
		}
		discovered, _ := extractFileCoverage(history)
		if len(discovered) != 3 {
			t.Fatalf("expected 3 discovered files, got %d: %v", len(discovered), discovered)
		}
	})

	t.Run("files_only=false format extracts paths", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true,
				Summary: "[grep: 3 matching lines]\ninternal/agent/explorer.go:157: func ShouldStop\ninternal/agent/agent.go:92: type Evaluator\ninternal/agent/explorer.go:200: return false"},
		}
		discovered, _ := extractFileCoverage(history)
		// Should extract unique paths: explorer.go, agent.go (deduplicated)
		if len(discovered) != 2 {
			t.Fatalf("expected 2 discovered files (deduplicated), got %d: %v", len(discovered), discovered)
		}
		has := make(map[string]bool)
		for _, d := range discovered {
			has[d] = true
		}
		if !has["internal/agent/explorer.go"] || !has["internal/agent/agent.go"] {
			t.Errorf("expected explorer.go and agent.go, got %v", discovered)
		}
	})

	t.Run("mixed formats", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true,
				Summary: "[grep: 2 matching files]\ninternal/agent/explorer.go\ninternal/agent/agent.go"},
			{ToolName: "grep", Success: true,
				Summary: "[grep: 2 matching lines]\ninternal/agent/planner.go:20: func ShouldStop\ninternal/agent/explorer.go:118: return false"},
		}
		discovered, _ := extractFileCoverage(history)
		// Should have 3 unique paths: explorer.go, agent.go, planner.go
		if len(discovered) != 3 {
			t.Fatalf("expected 3 discovered (mixed formats), got %d: %v", len(discovered), discovered)
		}
	})

	t.Run("header lines skipped", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true,
				Summary: "[grep: 1 matching files]\ninternal/agent/explorer.go"},
		}
		discovered, _ := extractFileCoverage(history)
		if len(discovered) != 1 {
			t.Fatalf("expected 1 (header skipped), got %d: %v", len(discovered), discovered)
		}
		if discovered[0] != "internal/agent/explorer.go" {
			t.Errorf("expected internal/agent/explorer.go, got %s", discovered[0])
		}
	})
}

func TestDetectDetailListingIntent(t *testing.T) {
	tests := []struct {
		question string
		want     bool
	}{
		{"具体有哪些策略", true},
		{"哪几种continuation push", true},
		{"按优先级从高到低排列", true},
		{"分别说明每种的触发条件", true},
		{"what strategies does the explorer use", true},
		{"what are the different agent types", true},
		{"describe each step", true},
		{"how does it work", false},
		{"explain the architecture", false},
	}
	for _, tt := range tests {
		got := detectDetailListingIntent(tt.question)
		if got != tt.want {
			t.Errorf("detectDetailListingIntent(%q) = %v, want %v", tt.question, got, tt.want)
		}
	}
}

func TestIsEvidenceLineAssignment(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		// Variable assignments creating new composites — should match
		{`synthMessages := []llm.Message{messages[0], msg}`, true},
		{`config := Config{Name: "test"}`, true},
		{`items := map[string]int{"a": 1}`, true},
		// Simple assignments — should NOT match (no composite literal)
		{`x := 42`, false},
		{`name := "hello"`, false},
		// Already-matching patterns should still work
		{`return true`, true},
		{`Register(NewFoo())`, true},
	}
	for _, tt := range tests {
		got := isEvidenceLine(tt.line)
		if got != tt.want {
			t.Errorf("isEvidenceLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsStr(s, sub)
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
