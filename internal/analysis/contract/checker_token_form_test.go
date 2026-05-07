package contract

import "testing"

// TestContainsSymbol_TokenFormEquivalence locks the multi-language
// equivalence promise of containsSymbol's token-form-aware fallback.
//
// Root motivation: s5a r2 went into a 7-violation must_include retry
// because the analyzer extracted wire-format AgentName strings
// (snake_case "sub_explorer") while the finalizer answered with Go
// type names (camelCase "subExplorerEvaluator"). The plain substring
// matcher missed the equivalence and triggered a finalizer
// re-dispatch costing ~1.5 min of LLM time.
//
// The fix must generalise to every language repomap supports, not
// just Go. Each row asserts a needle in one canonical form matches
// an answer string in another canonical form, mimicking the kinds
// of token-form crossings the analyzer→finalizer handoff actually
// produces in the wild.
func TestContainsSymbol_TokenFormEquivalence(t *testing.T) {
	cases := []struct {
		lang   string
		needle string
		text   string
		want   bool
		note   string
	}{
		// ── Go ─────────────────────────────────────────────────────
		{"go", "sub_explorer", "subExplorerEvaluator implements LoopController", true,
			"snake → camelCase Go type name (the s5a r2 root case)"},
		{"go", "log_triager", "the logTriagerEvaluator drives the optional pre-stage", true,
			"snake → camelCase, embedded mid-prose"},
		{"go", "answer_document_evaluator", "answerDocumentEvaluator owns the V2 carrier", true,
			"multi-segment snake → camelCase"},
		{"go", "TaskList", "TaskListSize tracks the count", true,
			"PascalCase substring (legacy behaviour preserved)"},
		{"go", "TaskList", "the task list is small", false,
			"phrase 'task list' must NOT satisfy the symbol — run boundary"},

		// ── Python ─────────────────────────────────────────────────
		{"python", "data_loader", "class DataLoader: pass", true,
			"snake → PascalCase class"},
		{"python", "DataLoader", "from .loaders import data_loader", true,
			"PascalCase needle hitting snake_case usage"},
		{"python", "BaseClass", "BaseClass.__init__ called", true,
			"identifier separated by dot (method qualifier)"},

		// ── Rust ───────────────────────────────────────────────────
		{"rust", "task_graph", "struct TaskGraph { nodes: Vec<Node> }", true,
			"snake → PascalCase Rust type"},
		{"rust", "TaskGraph", "fn build_task_graph() -> TaskGraph {}", true,
			"Pascal needle hits both fn name and return type"},
		{"rust", "MAX_RETRY", "const max_retry: usize = 3;", true,
			"SCREAMING_SNAKE needle hits snake_case (case-fold + separator-strip)"},
		{"rust", "Item", "use crate::collections::Item;", true,
			"qualified path :: breaks runs, but Item still matches"},

		// ── Swift ──────────────────────────────────────────────────
		{"swift", "user_session", "let userSession = UserSession()", true,
			"snake → camelCase + PascalCase, both in same line"},
		{"swift", "UserSession", "struct user_session_token {}", true,
			"Pascal → snake_case substring (within the run)"},

		// ── Java / Kotlin ─────────────────────────────────────────
		{"java", "request_id", "long requestId = ctx.getRequestId();", true,
			"snake → camelCase Java field + accessor"},
		{"java", "RequestId", "package com.example.proto; class RequestId {}", true,
			"Pascal needle hits class declaration"},
		{"kotlin", "view_model", "class HomeViewModel : ViewModel()", true,
			"snake → PascalCase Kotlin class"},
		{"java", "MAX_BACKOFF", "private static final int maxBackoff = 5;", true,
			"SCREAMING_SNAKE → camelCase via case-fold + separator-strip"},

		// ── Ruby ───────────────────────────────────────────────────
		{"ruby", "user_account", "class UserAccount; end", true,
			"snake → PascalCase Ruby class"},
		{"ruby", "UserAccount", "user_account.save!", true,
			"Pascal → snake instance"},
		{"ruby", "save", "User#save called", true,
			"# breaks run, save matches its own run"},

		// ── JS / TS / ArkTS ───────────────────────────────────────
		{"ts", "request_handler", "export function requestHandler() {}", true,
			"snake → camelCase TS export"},
		{"arkts", "user_profile", "@Component struct UserProfile {}", true,
			"snake → PascalCase ArkTS @Component"},
		{"ts", "kebab-case", "const kebabCase = 'foo'", true,
			"kebab → camelCase needle equivalence"},

		// ── Cangjie ───────────────────────────────────────────────
		{"cangjie", "task_runner", "class TaskRunner <: Runnable {}", true,
			"snake → PascalCase Cangjie class"},
		{"cangjie", "TaskRunner", "func task_runner_init() {}", true,
			"Pascal → snake_case Cangjie fn"},

		// ── C ─────────────────────────────────────────────────────
		{"c", "pthread_create", "int pthread_create(pthread_t *t, ...);", true,
			"snake_case C / POSIX function"},
		{"c", "MAX_BUFFER_SIZE", "#define MAX_BUFFER_SIZE 4096", true,
			"SCREAMING_SNAKE C macro"},
		{"c", "size_t", "typedef unsigned long size_t;", true,
			"POSIX-style _t suffix typedef"},
		{"c", "kErrorCode", "static const int kErrorCode = -1;", true,
			"Google-style camel const"},

		// ── C++ ───────────────────────────────────────────────────
		{"cpp", "MyClass", "class MyClass : public Base {};", true,
			"PascalCase C++ class"},
		{"cpp", "thread_local", "thread_local int counter = 0;", true,
			"snake_case C++11 keyword"},
		{"cpp", "BufferImpl", "class buffer_impl { /* ... */ };", true,
			"Pascal → snake C++ implementation class"},
		{"cpp_qualified", "vector_size", "size_t std_vector_size = nums.size();", true,
			"qualified-name run `std_vector_size` flat contains `vectorsize`"},

		// ── Objective-C ──────────────────────────────────────────
		{"objc", "NSString", "@interface NSString : NSObject", true,
			"PascalCase Obj-C class (NS prefix)"},
		{"objc", "did_finish_launching", "- (void)applicationDidFinishLaunching:", true,
			"snake → camelCase Obj-C method"},

		// ── Lua ───────────────────────────────────────────────────
		{"lua", "task_handler", "function TaskHandler:new() end", true,
			"snake → PascalCase Lua module"},
		{"lua", "process_request", "function M.processRequest(req) end", true,
			"snake → camelCase Lua method"},

		// ── Proto / gRPC ──────────────────────────────────────────
		{"proto", "user_account", "message UserAccount { string id = 1; }", true,
			"snake → PascalCase proto message"},
		{"proto", "RpcService", "service rpc_service { rpc Get(...); }", true,
			"Pascal → snake proto service"},

		// ── CUDA (uses cpp grammar) ───────────────────────────────
		{"cuda", "kernel_launch", "__global__ void kernelLaunch(float *x);", true,
			"snake → camel CUDA kernel"},

		// ── Filename anchors (`.` kept inside run) ────────────────
		{"go", "sub_explorer", "see sub_explorer.go for details", true,
			"snake_case filename with extension still satisfies the needle"},
		{"kotlin", "user_account", "see UserAccount.kt", true,
			"PascalCase filename with extension still satisfies the snake_case needle"},
		{"python", "data_loader", "see data_loader.py", true,
			"snake_case filename with .py extension"},

		// ── Negative cases (run-boundary safety) ──────────────────
		{"prose", "sub_explorer", "the sub explorer reads files", false,
			"prose 'sub explorer' (space-separated) must NOT match"},
		{"prose", "task_graph", "I built a task. Graph helps.", false,
			"sentence boundary . is run-internal but breaks via space"},
		{"short", "ab", "abc", true, "short needle keeps plain substring"},
		{"phrase", "loop controller", "the LoopController interface", false,
			"multi-word phrase needle skips identifier fallback"},
	}

	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.needle, func(t *testing.T) {
			got := containsSymbol(tc.text, tc.needle)
			if got != tc.want {
				t.Errorf("containsSymbol(%q, %q) = %v, want %v\n  note: %s",
					tc.text, tc.needle, got, tc.want, tc.note)
			}
		})
	}
}

// TestContainsSymbol_S5aR2Regression pins the exact production
// scenario that motivated the fix. If this test ever regresses,
// finalizer dispatches will jump from 1 to 2 on enumeration cases
// where analyzer extracted wire-format AgentName strings.
func TestContainsSymbol_S5aR2Regression(t *testing.T) {
	answer := `## Concrete Implementations of LoopController

The following 8 types in internal/agent/ implement LoopController:

| Type Name | File |
| --- | --- |
| analyzerEvaluator | analyzer.go |
| answerDocumentEvaluator | answer_document_evaluator.go |
| explorerEvaluator | explorer.go |
| extractorEvaluator | extractor.go |
| logTriagerEvaluator | log_triager.go |
| perfTriagerEvaluator | perf_triager.go |
| subExplorerEvaluator | sub_explorer.go |
| writeAnalyzerEvaluator | write_analyzer.go |
`

	// These are the literal MustInclude entries the s5a r2
	// analyzer produced (snake_case wire-format AgentName values).
	mustInclude := []string{
		"sub_explorer",
		"log_triager",
		"perf_triager",
		"write_analyzer",
		"answer_document_evaluator",
	}
	for _, term := range mustInclude {
		if !containsSymbol(answer, term) {
			t.Errorf("must_include term %q falsely missed in s5a-style answer — \n"+
				"this is the exact regression that caused the s5a r2 retry storm.", term)
		}
	}
}

// TestFlattenIdentifier_LanguageInvariants documents the canonical
// outputs across the major languages we support.
func TestFlattenIdentifier_LanguageInvariants(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"sub_explorer", "subexplorer"},
		{"subExplorer", "subexplorer"},
		{"SubExplorer", "subexplorer"},
		{"SUB_EXPLORER", "subexplorer"},
		{"sub-explorer", "subexplorer"},
		{"subexplorer", "subexplorer"},
		{"Sub-Explorer", "subexplorer"},
		{"answer_document_evaluator", "answerdocumentevaluator"},
		{"answerDocumentEvaluator", "answerdocumentevaluator"},
		{"AnswerDocumentEvaluator", "answerdocumentevaluator"},
		{"_leading", "leading"},
		{"trailing_", "trailing"},
		{"-kebab-", "kebab"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := flattenIdentifier(tc.in); got != tc.want {
				t.Errorf("flattenIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsIdentifierShaped(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"sub_explorer", true},
		{"subExplorer", true},
		{"sub-explorer", true},
		{"sub_explorer123", true},
		{"sub explorer", false},
		{"sub.explorer", false},
		{"sub::explorer", false},
		{"sub#explorer", false},
		{"", false},
		{"a", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isIdentifierShaped(tc.in); got != tc.want {
				t.Errorf("isIdentifierShaped(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
