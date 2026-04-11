package agent

import (
	"testing"

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
