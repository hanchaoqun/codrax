package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This file is the extended characterization grid for the
// `extractAnswerSymbols` / `answerSymbolFromEvidence` audit documented
// in memory/project_answer_symbol_extraction_audit.md.
//
// Structure
// ---------
// Each gap from the audit is covered by TWO tests sharing one fixture:
//
//   - Test<Name>_CurrentBroken   — passes today, asserts the exact
//                                  (usually wrong) output of the
//                                  current extraction. A reviewer who
//                                  flips this assertion is saying
//                                  "I've changed the extraction" and
//                                  must show it in the diff.
//
//   - Test<Name>_TargetCorrect   — skipped today via t.Skip, asserts
//                                  the post-fix behavior. Un-skip
//                                  when the Phase 2/3 fix in the
//                                  audit plan lands.
//
// Tests never call the analyzer or the LLM — they drive
// `extractAnswerSymbols` directly with synthesized `EvidenceItem`s
// built to mirror exactly what the explorer's Concrete Values
// extractor would produce for the matching source-code fixture. That
// keeps every test deterministic and independent of any single eval
// case.
//
// Over-fit audit
// --------------
// None of these fixtures encode eval-specific symbol names as magic
// constants in the assertion: the extractor's target output for each
// fixture is derivable from the fixture's *structural* fields
// (Subject, Object, Summary) — e.g. "the identifier immediately
// after `def ` in the decorator's target line". Flipping the
// fixtures to a different app name produces a different expected
// answer the same way.

// ============================================================
// Cell (R, binds, Go)  — reverse-reference enumeration
// ============================================================
// See reverse_reference_repro_test.go for the primary repro — that
// lives in its own file because it's the specific 2026-04-12
// triggering case. The grid test below uses a second fixture with
// different symbol names to confirm the gap is about the *shape*,
// not about SubExplorer in particular.

func evidenceGoRegistrationHopLiteral() types.EvidenceItem {
	// Go: a Register-style function binds a constructor whose
	// resulting type's Name() returns a literal string. Canonical
	// reverse-reference shape: the question asks "who uses the name
	// 'foo'", the answer is the literal "foo".
	return types.EvidenceItem{
		ID:        "ev-audit-R-binds-go",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterHandlers()",
		Predicate: "binds ONLY",
		Object:    "NewUserHandler(deps)",
		Summary:   "`RegisterHandlers()` binds ONLY NewUserHandler(deps) → `UserHandler.Name()` returns \"users\"",
		Source:    "internal/handlers/register.go",
		LineStart: 12,
		Producer:  "concrete_values",
	}
}

func TestAudit_ReverseBindsGo_Phase2Fixed(t *testing.T) {
	// Reverse-enumeration question — "how many handlers can serve
	// the users endpoint" — triggers classifyAnswerRole → RoleAnchor
	// (count + relationship verb), which walks the chain right-to-left
	// for a string literal and finds "users" in the terminal hop.
	question := "how many handlers can serve users"
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceGoRegistrationHopLiteral()}, "enumeration", question, nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "users" {
		t.Errorf("Phase 2: RoleAnchor should extract the terminal literal %q, got %q",
			"users", syms[0].Name)
	}
}

// ============================================================
// Cell (I, returns, Go)  — identity / return-value question
// ============================================================

func evidenceGoReturnsLiteral() types.EvidenceItem {
	// Go short function: `func (h *UserHandler) Name() string { return "users" }`
	// The concrete-values extractor represents this as:
	//   Subject = "UserHandler.Name"   (receiver.method)
	//   Object  = "\"users\""          (the returned literal)
	//   Summary = "`UserHandler.Name()` returns \"users\""
	return types.EvidenceItem{
		ID:        "ev-audit-I-returns-go",
		Kind:      types.EvidenceConcrete,
		Subject:   "UserHandler.Name",
		Predicate: "returns",
		Object:    "\"users\"",
		Summary:   "`UserHandler.Name()` returns \"users\"",
		Source:    "internal/handlers/user.go",
		LineStart: 20,
		Producer:  "concrete_values",
	}
}

func TestAudit_IdentityReturnsGo_Phase2Fixed(t *testing.T) {
	// "what does X return?" matches the RoleAnchor cue
	// ("what does" + "return"), so pickHop walks for a string
	// literal. The evidence is arrow-less so splitHops falls back
	// to [Subject, Object] = ["UserHandler.Name", "\"users\""];
	// the Object hop contains a quoted literal, extractQuotedLiteral
	// returns "users".
	question := "what does UserHandler.Name return?"
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceGoReturnsLiteral()}, "return_value", question, nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "users" {
		t.Errorf("Phase 2: return-value question should yield literal %q, got %q", "users", syms[0].Name)
	}
}

// ============================================================
// Cell (R, decorates, Python)  — unrouted shape + lowercase ident
// ============================================================

func evidencePythonDecoratorRoute() types.EvidenceItem {
	// Python:
	//   @app.route("/api/users")
	//   def list_users(): ...
	//
	// `extractConcreteValues` emits a concreteValueEntry with
	// kind="decorates" and value="@app.route(\"/api/users\") → list_users",
	// which becomes:
	return types.EvidenceItem{
		ID:        "ev-audit-R-decorates-py",
		Kind:      types.EvidenceConcrete,
		Subject:   "", // decorator scan doesn't set a Subject method
		Predicate: "decorates",
		Object:    "@app.route(\"/api/users\") → list_users",
		Summary:   "@app.route(\"/api/users\") → list_users",
		Source:    "api/users.py",
		LineStart: 7,
		Producer:  "concrete_values",
	}
}

func TestAudit_ReverseDecoratorPython_Phase3Fixed(t *testing.T) {
	// Phase 3: `decorates` predicate is now routed. hasTerminalEvidence
	// admits it; pickTerminalLegacy splits the Object on `→` and picks
	// firstIdent of the right-hand side — the decorated function name.
	// `list_users` is a snake_case identifier, so firstIdent (not
	// firstUppercaseIdent) does the picking.
	syms := extractAnswerSymbols([]types.EvidenceItem{evidencePythonDecoratorRoute()}, "enumeration", "", nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "list_users" {
		t.Errorf("Phase 3: expected %q (handler function from decorator), got %q", "list_users", syms[0].Name)
	}
}

// ============================================================
// Cell (R, decorates, Java)  — Spring-style annotation
// ============================================================

func evidenceJavaAnnotationRoute() types.EvidenceItem {
	// Java:
	//   @GetMapping("/api/foo")
	//   public ResponseEntity<Foo> getFoo() { ... }
	return types.EvidenceItem{
		ID:        "ev-audit-R-decorates-java",
		Kind:      types.EvidenceConcrete,
		Predicate: "decorates",
		Object:    "@GetMapping(\"/api/foo\") → getFoo",
		Summary:   "@GetMapping(\"/api/foo\") → getFoo",
		Source:    "src/main/java/app/FooController.java",
		LineStart: 34,
		Producer:  "concrete_values",
	}
}

func TestAudit_ReverseAnnotationJava_Phase3Fixed(t *testing.T) {
	// Same decorates-case routing as the Python test, proving the
	// fix is language-agnostic: lowercase-first camelCase (`getFoo`)
	// is accepted by firstIdent just like snake_case.
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceJavaAnnotationRoute()}, "call_chain", "", nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "getFoo" {
		t.Errorf("Phase 3: expected %q (Spring @GetMapping handler), got %q", "getFoo", syms[0].Name)
	}
}

// ============================================================
// Cell (F, maps, Go)  — map-literal routing table
// ============================================================

func evidenceGoMapDispatchTable() types.EvidenceItem {
	// Go: `var routes = map[string]http.Handler{ "/api/users": NewUserHandler() }`
	// `extractConcreteValues` emits kind="maps" with "key → value" pairs
	// joined by "; ".
	return types.EvidenceItem{
		ID:        "ev-audit-F-maps-go",
		Kind:      types.EvidenceConcrete,
		Predicate: "maps",
		Object:    "\"/api/users\" → NewUserHandler()",
		Summary:   "\"/api/users\" → NewUserHandler()",
		Source:    "internal/routes/table.go",
		LineStart: 8,
		Producer:  "concrete_values",
	}
}

func TestAudit_ForwardMapDispatchGo_Phase3Fixed(t *testing.T) {
	// Phase 3: `maps` predicate is now routed. Rightmost-arrow hop
	// is `NewUserHandler()`; firstUppercaseIdent picks `NewUserHandler`
	// and stripNewPrefix strips the constructor prefix. Any map entry
	// whose value is a Go constructor follows this path.
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceGoMapDispatchTable()}, "registration", "", nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "UserHandler" {
		t.Errorf("Phase 3: expected %q (constructor with New- stripped), got %q", "UserHandler", syms[0].Name)
	}
}

// ============================================================
// Cell (R, resolution_chain, Go)  — multi-hop reverse
// ============================================================

func evidenceGoResolutionChainMultihop() types.EvidenceItem {
	// A 3-hop chain where the user's question is "who INITIALIZES
	// TaskRunner"? The origin (leftmost hop, before the first →) is
	// the caller we want to surface; the current code always returns
	// the terminal (rightmost hop).
	return types.EvidenceItem{
		ID:        "ev-audit-R-chain-go",
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Summary:   "`NewOrchestrator()` binds ONLY NewTaskRunner(deps) → `TaskRunner.Name()` returns \"runner\" → `RunnerRegistry.Add()` assigns r[name] := runner",
		Producer:  "concrete_values",
	}
}

func TestAudit_ReverseChainGo_Phase2Fixed(t *testing.T) {
	// "who initializes X" matches classifyAnswerRole → RoleOrigin
	// (via the reverse-verb "initialize"), which takes the leftmost
	// hop of the resolution chain and pulls the caller name out of
	// the first "<Ident>(" pattern — NewOrchestrator, stripped of
	// its New prefix to "Orchestrator".
	question := "who initializes TaskRunner"
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceGoResolutionChainMultihop()}, "call_chain", question, nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Orchestrator" {
		t.Errorf("Phase 2: RoleOrigin should extract leftmost-hop caller %q, got %q",
			"Orchestrator", syms[0].Name)
	}
}

// ============================================================
// Cell (I, returns, Python)  — lowercase identifier blindness
// ============================================================

func evidencePythonMethodReturnsLiteral() types.EvidenceItem {
	// Python:
	//   class list_users:
	//       def name(self): return "users"
	//
	// (A class with a snake_case name is unusual in Python but NOT
	// rejected by the extractor; for test purposes it's the easiest
	// way to exercise the lowercase-identifier code path without
	// inventing a new evidence shape.)
	return types.EvidenceItem{
		ID:        "ev-audit-I-returns-py",
		Kind:      types.EvidenceConcrete,
		Subject:   "list_users.name",
		Predicate: "returns",
		Object:    "\"users\"",
		Summary:   "`list_users.name()` returns \"users\"",
		Source:    "api/users.py",
		LineStart: 14,
		Producer:  "concrete_values",
	}
}

func TestAudit_IdentityReturnsPython_Phase3Fixed(t *testing.T) {
	// Phase 3: the returns case in pickTerminalLegacy now falls back
	// to firstIdent when firstUppercaseIdent draws a blank. For
	// Subject="list_users.name" the Go-strict picker returns "";
	// firstIdent accepts `list_users` as a snake_case identifier
	// (≥3 chars, isIdentStart-passing). This keeps legacy Go tests
	// unchanged and adds Python/Ruby receiver-type coverage.
	syms := extractAnswerSymbols([]types.EvidenceItem{evidencePythonMethodReturnsLiteral()}, "return_value", "", nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "list_users" {
		t.Errorf("Phase 3: expected %q (snake_case receiver), got %q", "list_users", syms[0].Name)
	}
}

// ============================================================
// classifyAnswerRole: language + rewrite coverage
// ============================================================
// Direct tests on the classifier. These lock the Chinese + English
// keyword sets so a future edit that accidentally drops a synonym
// (or adds one that over-triggers) is visible immediately.
//
// The fixtures mix raw user phrasings and analyzer-rewritten forms
// because the classifier runs on whatever text the call site hands
// it, which in production is the analyzer task title (rewritten),
// NOT the raw user question. A classifier that only understood
// the raw form would silently fail the end-to-end repro — see the
// "统计...数量" case below, which is exactly what the 2026-04-12
// re-test surfaced when only the raw list was covered.

func TestClassifyAnswerRole_ReverseReferenceChinese(t *testing.T) {
	cases := []struct {
		name string
		q    string
		want AnswerRole
	}{
		{"raw 多少 + 调用", "有多少个agent可以调用subagent", RoleAnchor},
		{"analyzer 统计 + 数量", "统计可以调用subagent的agent数量", RoleAnchor},
		{"几个 + 使用", "有几个模块使用cache", RoleAnchor},
		{"谁 + verb → RoleOrigin", "谁调用了这个方法", RoleOrigin},
		{"哪些 + 注册", "哪些组件注册了handler", RoleAnchor},
		{"返回什么 → RoleAnchor", "这个方法返回什么", RoleAnchor},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyAnswerRole(c.q, "")
			if got != c.want {
				t.Errorf("classifyAnswerRole(%q) = %v, want %v", c.q, got, c.want)
			}
		})
	}
}

func TestClassifyAnswerRole_ReverseReferenceEnglish(t *testing.T) {
	cases := []struct {
		name string
		q    string
		want AnswerRole
	}{
		{"raw how many + call", "How many agents can call subagents?", RoleAnchor},
		{"analyzer rewrite count the", "Count the agents that invoke sub-agents", RoleAnchor},
		{"determine the number of", "Determine the number of handlers that register routes", RoleAnchor},
		{"who + verb → Origin", "Who calls UserHandler.Name", RoleOrigin},
		{"which component + initializes → Origin", "Which component initializes TaskRunner", RoleOrigin},
		{"what does X return → Anchor", "What does SubExplorer.Name return", RoleAnchor},
		{"empty → Terminal (legacy)", "", RoleTerminal},
		{"forward property → Terminal default", "Show me the definition of Foo", RoleTerminal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyAnswerRole(c.q, "")
			if got != c.want {
				t.Errorf("classifyAnswerRole(%q) = %v, want %v", c.q, got, c.want)
			}
		})
	}
}

// ============================================================
// Sanity non-regression: cell (F, binds, Go) — currently-working case
// ============================================================
// This is NOT a gap. It's here to make sure Phase 2/3 fixes don't
// break the currently-green path. Plain forward registration: "what
// does RegisterHandlers register?" → UserHandler ✓.

func TestAudit_ForwardBindsGo_SanityNonRegression(t *testing.T) {
	syms := extractAnswerSymbols([]types.EvidenceItem{evidenceGoRegistrationHopLiteral()}, "registration", "", nil)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	// This must stay UserHandler both before AND after the fix,
	// because a forward-registration question should still resolve
	// to the registered class.
	if syms[0].Name != "UserHandler" {
		t.Errorf("non-regression: forward registration should yield class name; want %q, got %q",
			"UserHandler", syms[0].Name)
	}
}
