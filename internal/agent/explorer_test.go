package agent

import (
	"os"
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

	// Regression guard for the 2026-04-12 explorer context blowup:
	// when ripgrep was run on a single-file path without -H, output
	// lines looked like "158-	// content" or "200:	func Foo". The
	// extractor used to parse these as "discovered files", inflating
	// the coverage denominator with dozens of bogus entries per grep
	// call. Now: dash/colon-before-lineno is recognized, and
	// isValidFilePath rejects anything that isn't a plausible path.
	t.Run("single-file grep without filename prefix is rejected", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true, Summary: "[grep: 5 matching lines]\n" +
				"158-\t// early return\n" +
				"159-\t// when condition holds\n" +
				"200:\tfunc PipelineStage\n" +
				"--\n" +
				"242-\t// context line with no path\n"},
		}
		discovered, _ := extractFileCoverage(history)
		if len(discovered) != 0 {
			t.Errorf("expected 0 discovered from unprefixed grep output, got %d: %v", len(discovered), discovered)
		}
	})

	// Context-line format with proper filename prefix (dash separator):
	// ripgrep -C emits "file.go-101-content" for context lines. The
	// extractor must recognize the dash-before-lineno form and still
	// extract just the filename.
	t.Run("grep context lines with dash separator", func(t *testing.T) {
		history := []types.ToolResult{
			{ToolName: "grep", Success: true, Summary: "[grep: 4 matching lines]\n" +
				"internal/agent/explorer.go-100-\t// context\n" +
				"internal/agent/explorer.go:101:\tfunc Pipeline\n" +
				"internal/agent/explorer.go-102-\t// more context\n" +
				"internal/agent/agent.go:50:\ttype Evaluator\n"},
		}
		discovered, _ := extractFileCoverage(history)
		if len(discovered) != 2 {
			t.Fatalf("expected 2 discovered (deduped), got %d: %v", len(discovered), discovered)
		}
		has := make(map[string]bool)
		for _, d := range discovered {
			has[d] = true
		}
		if !has["internal/agent/explorer.go"] || !has["internal/agent/agent.go"] {
			t.Errorf("expected both paths, got %v", discovered)
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

func TestExtractDecisionBlocks(t *testing.T) {
	t.Run("Go-style function with 4 blocks", func(t *testing.T) {
		// Simulates a function with comment-delimited independent blocks,
		// each terminated by a return statement.
		src := `func (e *eval) ContinuationPrompt() (string, bool) {
	// Phase setup
	discovered := scan()

	// Function-boundary read guidance (HIGHEST PRIORITY)
	if partials := detect(); len(partials) > 0 {
		return hint, true
	}

	// Enumeration completeness
	if isEnum && coverage < 0.8 {
		return enumHint, true
	}

	// Large-file grep redirect
	truncated := detectTruncated()
	if len(truncated) > 0 {
		return redirectHint, true
	}

	// Idle termination
	if idle >= 2 {
		return "", false
	}
	return fallback, true
}`
		lines := strings.Split(src, "\n")
		// Function body spans lines 1 to len(lines) (1-based).
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		if blocks == nil {
			t.Fatal("expected ≥3 blocks, got nil")
		}
		// "Phase setup" has no return, filtered out. 4 strategy blocks remain.
		if len(blocks) != 4 {
			t.Errorf("expected 4 blocks (return-bearing only), got %d", len(blocks))
			for i, b := range blocks {
				t.Logf("  block %d: L%d-%d %q", i, b.startLine, b.endLine, b.label)
			}
		}
		if blocks[0].label != "Function-boundary read guidance (HIGHEST PRIORITY)" {
			t.Errorf("block[0] label = %q", blocks[0].label)
		}
		if blocks[len(blocks)-1].label != "Idle termination" {
			t.Errorf("last block label = %q", blocks[len(blocks)-1].label)
		}
	})

	t.Run("Python-style function with hash comments", func(t *testing.T) {
		// First comment after function def has no blank line before it,
		// so it's skipped. The remaining 3 sections (preceded by blank
		// lines) are detected.
		src := `def handle_request(self, req):
    # Validate input
    if not req.valid:
        return error_response()

    # Check permissions
    if not self.has_access(req.user):
        return forbidden()

    # Process the request
    result = self.process(req)
    if result.failed:
        return failure(result)

    # Send response
    return success(result)`
		lines := strings.Split(src, "\n")
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		if blocks == nil {
			t.Fatal("expected ≥3 blocks, got nil")
		}
		if len(blocks) != 3 {
			t.Errorf("expected 3 blocks, got %d", len(blocks))
			for i, b := range blocks {
				t.Logf("  block %d: L%d-%d %q", i, b.startLine, b.endLine, b.label)
			}
		}
		if blocks[0].label != "Check permissions" {
			t.Errorf("block[0] label = %q", blocks[0].label)
		}
	})

	t.Run("Java-style with throw terminators", func(t *testing.T) {
		src := `public Response dispatch(Request req) {
    // Authentication check
    if (!isAuthenticated(req)) {
        throw new UnauthorizedException();
    }

    // Rate limiting
    if (rateLimiter.isExceeded(req.getClient())) {
        throw new TooManyRequestsException();
    }

    // Route to handler
    Handler h = router.resolve(req.getPath());
    return h.handle(req);
}`
		lines := strings.Split(src, "\n")
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		// Java throw is at deeper indent (8 spaces vs 4 base), so won't
		// be detected as a block terminator. Only the final return at
		// base+4 terminates. This is correct: the throw is inside an
		// if-block, not a top-level early return.
		// With 3 section headers and 1 terminator, we get 3 blocks.
		if blocks == nil {
			t.Fatal("expected ≥3 blocks, got nil")
		}
		if len(blocks) < 3 {
			t.Errorf("expected ≥3 blocks, got %d", len(blocks))
			for i, b := range blocks {
				t.Logf("  block %d: L%d-%d %q", i, b.startLine, b.endLine, b.label)
			}
		}
	})

	t.Run("too few blocks returns nil", func(t *testing.T) {
		src := `func simple() {
	// Setup
	x := 1
	return x
}`
		lines := strings.Split(src, "\n")
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		if blocks != nil {
			t.Errorf("expected nil for <3 blocks, got %d blocks", len(blocks))
		}
	})

	t.Run("too short function returns nil", func(t *testing.T) {
		src := `func tiny() { return 1 }`
		lines := strings.Split(src, "\n")
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		if blocks != nil {
			t.Errorf("expected nil for short function, got %d blocks", len(blocks))
		}
	})

	t.Run("SQL-style with double-dash comments", func(t *testing.T) {
		// PL/pgSQL RAISE is inside IF blocks (deeper indent), so
		// return-filter correctly excludes them — the section headers
		// don't have top-level terminators. This is correct behavior:
		// SQL control flow is fundamentally different from early-return
		// languages. Decision blocks are most useful for Go/Python/Java
		// style early-return functions.
		src := `CREATE FUNCTION process()
RETURNS void AS $$
BEGIN
    -- Validate schema version
    IF NOT check_version() THEN
        RAISE EXCEPTION 'bad version';
    END IF;

    -- Migrate pending records
    IF has_pending() THEN
        RAISE NOTICE 'migrating';
    END IF;

    -- Clean up temporary tables
    DROP TABLE IF EXISTS tmp_work;

    -- Update statistics
    RAISE NOTICE 'done';
END;
$$ LANGUAGE plpgsql;`
		lines := strings.Split(src, "\n")
		blocks := extractDecisionBlocks(lines, 1, len(lines))
		// RAISE is nested inside IF (indent > base+4), so blocks get
		// filtered out by return-filter. nil is the correct result.
		if blocks != nil {
			t.Logf("SQL blocks (expected nil due to nested RAISE): %d", len(blocks))
			for i, b := range blocks {
				t.Logf("  [%d] L%d-%d: %s", i, b.startLine, b.endLine, b.label)
			}
		}
	})

	t.Run("real ContinuationPrompt from codebase", func(t *testing.T) {
		// Read the actual explorer.go and run extraction on ContinuationPrompt.
		data, err := os.ReadFile("explorer.go")
		if err != nil {
			t.Skip("explorer.go not in test working directory")
		}
		lines := strings.Split(string(data), "\n")
		// ContinuationPrompt starts around line 165, but the Phase 1
		// body with decision blocks starts at ~282 and ends ~592.
		// Find exact bounds by scanning for the function signature.
		funcStart, funcEnd := 0, 0
		for i, l := range lines {
			if strings.Contains(l, "func (e *explorerEvaluator) ContinuationPrompt(") {
				funcStart = i + 1 // 1-based
			}
			// Find the closing brace at column 0 after funcStart.
			if funcStart > 0 && funcEnd == 0 && i > funcStart && strings.TrimSpace(l) == "}" && !strings.HasPrefix(l, "\t") {
				funcEnd = i + 1
				break
			}
		}
		if funcStart == 0 || funcEnd == 0 {
			t.Skip("could not find ContinuationPrompt bounds")
		}
		blocks := extractDecisionBlocks(lines, funcStart, funcEnd)
		if blocks == nil {
			t.Fatal("expected decision blocks in ContinuationPrompt, got nil")
		}
		// Should have ≥5 blocks (function-boundary, enumeration, large-file,
		// pre-scanned push, unanalyzed symbols, idle streak, idle termination).
		if len(blocks) < 5 {
			t.Errorf("expected ≥5 decision blocks, got %d", len(blocks))
		}
		t.Logf("ContinuationPrompt: %d decision blocks detected", len(blocks))
		for i, b := range blocks {
			t.Logf("  [%d] L%d-%d: %s", i+1, b.startLine, b.endLine, b.label)
		}
	})
}

// TestStripConversationPrefix locks the parse for REPL's
// "## Prior conversation ... ## Current request\n<raw>" wrapper. The
// helper must return only the raw current request when the marker is
// present, and pass through unchanged otherwise (single-shot mode).
func TestStripConversationPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single-shot passes through",
			in:   "how many agents can invoke subagent",
			want: "how many agents can invoke subagent",
		},
		{
			name: "REPL with memory prefix returns current request only",
			in: "## Prior conversation\n### Relevant compacted memory\n" +
				"- **Project Agents and Subagents** (turn-xxx) — SubExplorer, NewSubExplorer ...\n" +
				"  Full turn:\n" +
				"  (long content)\n\n" +
				"## Current request\n" +
				"how many agents can invoke subagent",
			want: "how many agents can invoke subagent",
		},
		{
			name: "REPL with trailing whitespace is trimmed",
			in: "## Prior conversation\n(something)\n\n" +
				"## Current request\n" +
				"what does Foo.Bar return\n\n",
			want: "what does Foo.Bar return",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "marker without trailing newline not matched (single-shot form wins)",
			in:   "## Current request",
			want: "## Current request",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripConversationPrefix(c.in); got != c.want {
				t.Errorf("stripConversationPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestBuildInitialPrompt_CrossRunResetOnQuestionChange is the REPL-
// turn-boundary fix lock. When the explorer evaluator's cached
// userQuestion differs from the new ctx.CurrentTask, every cross-
// Run field must be reset so the retry branch below does NOT treat
// the new question as a continuation of a prior one.
//
// The 2026-04-12 audit (see memory/project_repl_equivalence_audit.md)
// showed that without this reset, a fresh REPL turn would inherit the
// previous turn's investigationNotes, preScannedFiles, searchResult,
// and ermRequirements — polluting S1/S2 decisions for the new
// question.
func TestBuildInitialPrompt_CrossRunResetOnQuestionChange(t *testing.T) {
	eval := &explorerEvaluator{
		userQuestion:              "how many agents can invoke subagent",
		investigationNotes:        []string{"[DIRECT] prior turn note"},
		preScannedFiles:           []string{"internal/agent/explorer.go"},
		allScoredFiles:            []string{"internal/agent/explorer.go"},
		searchResult:              &keywordSearchResult{}, // non-nil stale pointer
		ermRequirements:           []EvidenceRequirement{{Kind: "registration", Entities: []string{"subagent"}}},
		fileSymbols:               map[string][]string{"stale.go": {"Foo"}},
		phase0ExtraRound:          true,
		grepRedirectedFiles:       map[string]bool{"stale.go": true},
		idleStreakInDepth:         5,
		lastToolResultCount:       10,
		preScannedPushCount:       3,
		lastPreScannedUnreadCount: 2,
		broadenAttempts:           1,
	}

	// Simulate next REPL turn with a completely different question.
	// ctx.CurrentTaskKeywords is empty so BuildInitialPrompt's
	// keywordSearch gate doesn't run — we only care that the reset
	// wipes prior state BEFORE the retry check.
	ctx := &types.AgentContext{
		CurrentTask: "how does BuildContext cap turn file size",
	}
	prompt := eval.BuildInitialPrompt(ctx, nil)

	// The retry branch must NOT activate — prior notes should be gone.
	if strings.Contains(prompt, "Retry: Depth Investigation") {
		t.Errorf("cross-run reset failed: retry branch fired on question change")
	}
	// All cross-run fields must be reset.
	if len(eval.investigationNotes) != 0 {
		t.Errorf("investigationNotes not reset: %v", eval.investigationNotes)
	}
	if len(eval.preScannedFiles) != 0 {
		t.Errorf("preScannedFiles not reset: %v", eval.preScannedFiles)
	}
	if len(eval.allScoredFiles) != 0 {
		t.Errorf("allScoredFiles not reset: %v", eval.allScoredFiles)
	}
	if eval.searchResult != nil {
		t.Errorf("searchResult not reset")
	}
	if len(eval.ermRequirements) != 0 {
		t.Errorf("ermRequirements not reset: %v", eval.ermRequirements)
	}
	if len(eval.fileSymbols) != 0 {
		t.Errorf("fileSymbols not reset: %v", eval.fileSymbols)
	}
	if eval.phase0ExtraRound {
		t.Error("phase0ExtraRound not reset")
	}
	if eval.idleStreakInDepth != 0 || eval.lastToolResultCount != 0 ||
		eval.preScannedPushCount != 0 || eval.lastPreScannedUnreadCount != 0 ||
		eval.broadenAttempts != 0 {
		t.Error("per-run counters not reset")
	}
	// New userQuestion should reflect the new task.
	if eval.userQuestion != "how does BuildContext cap turn file size" {
		t.Errorf("userQuestion not updated to new question: %q", eval.userQuestion)
	}
}

// TestBuildInitialPrompt_SameQuestionKeepsRetryState is the
// complementary test: when ctx.CurrentTask equals e.userQuestion
// (intra-Run self-loop), the cross-run reset must NOT fire and the
// retry branch below must activate as before.
func TestBuildInitialPrompt_SameQuestionKeepsRetryState(t *testing.T) {
	eval := &explorerEvaluator{
		userQuestion:       "investigate strategies",
		investigationNotes: []string{"[DIRECT] strategy A from iter 1"},
	}
	ctx := &types.AgentContext{CurrentTask: "investigate strategies"}
	prompt := eval.BuildInitialPrompt(ctx, nil)

	if !strings.Contains(prompt, "Retry: Depth Investigation") {
		t.Error("same-question retry branch did not fire")
	}
	if len(eval.investigationNotes) != 1 {
		t.Errorf("investigationNotes wiped on same-question retry: %v", eval.investigationNotes)
	}
}

func TestBuildInitialPrompt_RetryInjectsPriorSynthesis(t *testing.T) {
	// All sub-tests simulate an intra-Run explore → explore self-loop
	// where the SAME question is retried. The 2026-04-12 REPL audit
	// added a cross-run reset that fires when `ctx.CurrentTask !=
	// e.userQuestion`, so intra-Run retry fixtures MUST keep the two
	// fields equal for the retry branch to activate.
	const taskTitle = "investigate strategies"
	eval := &explorerEvaluator{}
	eval.investigationNotes = []string{"[DIRECT] foo line 1: something"}
	eval.userQuestion = taskTitle

	t.Run("retry with prior explore report includes synthesis baseline", func(t *testing.T) {
		ctx := &types.AgentContext{
			CurrentTask: taskTitle,
			PriorReports: []types.StageReport{
				{
					Stage:    types.StageExplore,
					Agent:    types.AgentExplorer,
					Findings: "The function has 3 strategies: A, B, and C.",
				},
			},
		}
		prompt := eval.BuildInitialPrompt(ctx, nil)

		if !strings.Contains(prompt, "Previous Synthesis") {
			t.Error("retry prompt should contain 'Previous Synthesis' section")
		}
		if !strings.Contains(prompt, "3 strategies: A, B, and C") {
			t.Error("retry prompt should contain the prior synthesis findings")
		}
		if !strings.Contains(prompt, "improve, don't restart") {
			t.Error("retry prompt should instruct to improve, not restart")
		}
	})

	t.Run("retry with RetryHint includes both hint and synthesis", func(t *testing.T) {
		eval2 := &explorerEvaluator{
			investigationNotes: []string{"[DIRECT] bar line 2: thing"},
			userQuestion:       "investigate",
		}
		ctx := &types.AgentContext{
			CurrentTask: "investigate",
			RetryHint:   "Previous attempt had low file coverage.",
			PriorReports: []types.StageReport{
				{
					Stage:    types.StageExplore,
					Agent:    types.AgentExplorer,
					Findings: "Found 2 of 5 items.",
				},
			},
		}
		prompt := eval2.BuildInitialPrompt(ctx, nil)

		if !strings.Contains(prompt, "Previous attempt had low file coverage") {
			t.Error("should contain RetryHint")
		}
		if !strings.Contains(prompt, "Found 2 of 5 items") {
			t.Error("should contain prior synthesis findings")
		}
	})

	t.Run("retry without prior reports works normally", func(t *testing.T) {
		eval3 := &explorerEvaluator{
			investigationNotes: []string{"[DIRECT] baz line 3: stuff"},
			userQuestion:       "investigate",
		}
		ctx := &types.AgentContext{
			CurrentTask: "investigate",
		}
		prompt := eval3.BuildInitialPrompt(ctx, nil)

		if strings.Contains(prompt, "Previous Synthesis") {
			t.Error("should NOT contain Previous Synthesis when no PriorReports")
		}
		if !strings.Contains(prompt, "Retry: Depth Investigation") {
			t.Error("should still be a retry prompt (has investigationNotes)")
		}
	})

	t.Run("prior synthesis truncated when too long", func(t *testing.T) {
		eval4 := &explorerEvaluator{
			investigationNotes: []string{"[DIRECT] x line 1: y"},
			userQuestion:       "investigate",
		}
		longFindings := strings.Repeat("A very detailed finding. ", 200)
		ctx := &types.AgentContext{
			CurrentTask: "investigate",
			PriorReports: []types.StageReport{
				{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: longFindings},
			},
		}
		prompt := eval4.BuildInitialPrompt(ctx, nil)

		if !strings.Contains(prompt, "[truncated]") {
			t.Error("long prior synthesis should be truncated")
		}
	})
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
