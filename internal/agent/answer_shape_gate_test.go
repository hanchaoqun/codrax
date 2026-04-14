package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This file locks the Phase 4 (2026-04-13) answer-shape-gated
// literal-termination promotion inside extractAnswerSymbols.
//
// Why Phase 4 exists
// ------------------
// The Phase 2 NL classifier (classifyAnswerRole) reads analyzer-
// rewritten surface forms and can miss reverse-reference questions
// when the analyzer picks a rewrite that doesn't match any keyword
// cue. Real 2026-04-13 repro captured in the log at
// /tmp/codrax-verify-repl3-.../logs/*.log:
//
//   user:     "which agents can invoke subagent"
//   analyzer: title="Identify subagent-invoking agents",
//             question_kind="call_chain",
//             answer_shape="list_of_symbols"
//   classifier: no "which" prefix + no count cue ("identify" is not
//             in the countEn list) + declaredKind != "enumeration"
//             → defaults to RoleTerminal
//   extractor: walks binding LHS via pickTerminalLegacy → picks the
//             callee class `SubExplorer` instead of the bridging
//             literal "explorer".
//
// The 2026-04-12 audit's F3 declared-kind fast path was designed for
// the Chinese→English "List agents that..." rewrite which DOES land
// on question_kind=enumeration. The English→English "Identify ...-
// invoking agents" rewrite bypasses F3 because the analyzer chooses
// call_chain instead. Adding more English keywords to countEn is
// playing whack-a-mole with an LLM's rewrite surface area.
//
// Phase 4 fix
// -----------
// Use answer_shape — a DIFFERENT analyzer field with an explicit
// documented contract (analyzer.go hard-rule 3): list_of_symbols
// means "user is asking for a SET of names they want to see listed".
// When the analyzer declared list_of_symbols AND the classifier
// fell through to RoleTerminal AND the evidence set actually
// contains a literal in some hop, promote the role to RoleAnchor.
// Anchor's strict right-to-left literal walk then picks the
// bridging literal and drops items without one.
//
// Over-fit audit (run against every test in this file)
// ----------------------------------------------------
//  1. Reverse: swap list_of_symbols ↔ value → promotion flips off.
//  2. Deletion: no specific eval-case identifier appears in the
//     gate logic; removing any single fixture does not weaken the
//     rule.
//  3. Class/name specificity: fixtures use generic names
//     (Registry/Handler/Widget) and one uses the real-scenario
//     shape. The gate logic is shape-only.
//  4. No-bait: the gate requires BOTH answer_shape AND a literal in
//     the evidence; neither alone triggers.
//  5. No-contamination: no identifier from any eval case is
//     hardcoded in the production path.

// ============================================================
// Positive: the Phase 4 gate fires on the documented shape
// ============================================================

// evidencePhase4BridgeLiteral mirrors the real 2026-04-13 repro
// shape: a Register-family caller binds a constructor whose
// resulting type's Name()/Key() method returns a string literal
// that is the bridging identity. The classifier would see no
// recognizable English cue in the analyzer's rewritten title and
// return RoleTerminal; pickTerminalLegacy would walk the binding
// LHS and return "Widget" — wrong. Phase 4 routes through Anchor
// instead, walking right-to-left, extracting "widget-handler".
func evidencePhase4BridgeLiteral() types.EvidenceItem {
	return types.EvidenceItem{
		ID:        "ev-phase4-bridge-literal",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterDefaultWidgets()",
		Predicate: "binds ONLY",
		Object:    "NewWidget(deps)",
		Summary:   "`RegisterDefaultWidgets()` binds ONLY NewWidget(deps) → `Widget.Name()` returns \"widget-handler\"",
		Source:    "internal/widget/register.go",
		LineStart: 42,
		Producer:  "concrete_values",
	}
}

func TestPhase4_AnalyzerRewriteThatBypassesKeywordClassifier(t *testing.T) {
	// Simulates the 2026-04-13 REPL turn 1 path: analyzer rewrites
	// a "which X can Y" question to a verb-less "Identify ...-ing
	// agents" title AND classifies it as call_chain (both are wrong
	// NL signals). The keyword classifier returns RoleTerminal. Old
	// behavior would extract the callee class "Widget". Phase 4's
	// gate sees answer_shape=list_of_symbols + terminal literal
	// present → routes through RoleAnchor → extracts the literal.
	question := "Identify widget-handling agents" // no "which", no "how many", no "list the"
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{evidencePhase4BridgeLiteral()},
		"call_chain",       // analyzer's (wrong) question_kind
		question,           // analyzer's (bypassing) title
		"list_of_symbols",  // analyzer's (correct) answer_shape
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "widget-handler" {
		t.Errorf("Phase 4 promotion to Anchor should pick the bridging literal; got %q want %q",
			syms[0].Name, "widget-handler")
	}
}

// ============================================================
// Negative: gate stays off when inputs don't match the contract
// ============================================================

func TestPhase4_NoPromotion_WhenAnswerShapeIsValue(t *testing.T) {
	// Reverse of the positive test: same evidence, same unrewritable
	// title, but analyzer declared answer_shape=value (a single
	// literal value is expected, not a set of names). Promotion
	// must NOT fire. The existing classifier returns RoleTerminal
	// and pickTerminalLegacy extracts the callee class.
	question := "Identify widget-handling agents"
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{evidencePhase4BridgeLiteral()},
		"call_chain",
		question,
		"value", // NOT list_of_symbols
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Widget" {
		t.Errorf("answer_shape=value must NOT promote; legacy Terminal should pick class name; got %q want %q",
			syms[0].Name, "Widget")
	}
}

func TestPhase4_NoPromotion_WhenEvidenceHasNoLiteral(t *testing.T) {
	// Positive answer_shape but the evidence chain carries NO
	// literal in any hop. Promoting to Anchor would just drop the
	// item (strict walk). Phase 4 stays on Terminal so the class
	// name is still returned.
	ev := types.EvidenceItem{
		ID:        "ev-phase4-no-literal",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterDefaultWidgets()",
		Predicate: "binds ONLY",
		Object:    "NewWidget(deps)",
		Summary:   "`RegisterDefaultWidgets()` binds ONLY NewWidget(deps) → `Widget.Run()` assigns cfg := ...",
		Source:    "internal/widget/register.go",
		LineStart: 42,
		Producer:  "concrete_values",
	}
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{ev},
		"call_chain",
		"Identify widget-handling agents",
		"list_of_symbols", // gate would fire, but literal absent
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol (legacy Terminal), got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Widget" {
		t.Errorf("no literal in any hop → no promotion → legacy class pick; got %q want %q",
			syms[0].Name, "Widget")
	}
}

func TestPhase4_NoPromotion_WhenClassifierAlreadyAnchor(t *testing.T) {
	// The existing classifier already routes this question to
	// RoleAnchor via its count-cue table. Phase 4 must leave that
	// path alone — the gate only applies when role == RoleTerminal.
	// The result here must be identical to the classic Phase 2
	// behavior exercised by the audit test grid.
	question := "how many handlers can serve users" // countEn + verb → Anchor
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{evidenceGoRegistrationHopLiteral()},
		"enumeration",
		question,
		"list_of_symbols",
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "users" {
		t.Errorf("RoleAnchor path unchanged; got %q want %q", syms[0].Name, "users")
	}
}

func TestPhase4_NoPromotion_WhenClassifierReturnsOrigin(t *testing.T) {
	// "who binds X" is a reverse-reference-by-origin form. The
	// classifier returns RoleOrigin; Phase 4's gate is strictly
	// guarded on RoleTerminal so this path must also be left alone.
	// We don't assert a specific Name here because Origin's hop
	// walker has its own semantics; we only assert that promotion
	// did NOT silently fire (Anchor would pick "widget-handler"
	// which would be visible if the gate wrongly overrode Origin).
	question := "who binds widget-handler"
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{evidencePhase4BridgeLiteral()},
		"call_chain",
		question,
		"list_of_symbols",
		nil,
	)
	// Origin walks LHS hops and returns the caller name.
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol from Origin path, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name == "widget-handler" {
		t.Errorf("Phase 4 gate wrongly overrode RoleOrigin; got Anchor literal %q", syms[0].Name)
	}
}

// ============================================================
// Real-scenario fixture (SubExplorer → "explorer")
// ============================================================
// This is the exact chain shape captured in the 2026-04-13 REPL
// turn 1 debug log (chain[0] of the 5 produced by the explorer).
// It's in this file rather than reverse_reference_repro_test.go
// because the triggering analyzer path is the English→English
// "Identify ...-ing agents" rewrite that the Phase 2 classifier
// cannot distinguish from forward call-chain questions, which is
// what Phase 4 specifically fixes.

func evidencePhase4RealReproSubExplorer() types.EvidenceItem {
	return types.EvidenceItem{
		ID:        "ev-phase4-real-repro",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterDefaultSubAgents()",
		Predicate: "binds ONLY",
		Object:    "NewSubExplorer(deps)",
		Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
		Source:    "internal/agent/registry.go",
		LineStart: 18,
		Producer:  "concrete_values",
	}
}

func TestPhase4_RealRepro_2026_04_13_REPLTurn1(t *testing.T) {
	// Reproduces the exact failing case: analyzer's English rewrite
	// says "Identify subagent-invoking agents" with kind=call_chain
	// and shape=list_of_symbols. Pre-Phase 4: extracts "SubExplorer"
	// (wrong). Post-Phase 4: extracts "explorer" (correct — the
	// Name() literal that the Go agent registry keys on).
	question := "Identify subagent-invoking agents"
	syms := extractAnswerSymbols(
		[]types.EvidenceItem{evidencePhase4RealReproSubExplorer()},
		"call_chain",
		question,
		"list_of_symbols",
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "explorer" {
		t.Errorf("real repro: want %q (Name literal), got %q (callee class)", "explorer", syms[0].Name)
	}
}

// ============================================================
// Drop semantics: mixed batch (some items with literals, some without)
// ============================================================

func TestPhase4_AnchorStrictDropsNonLiteralItems(t *testing.T) {
	// Mirror the real REPL run where identifyAnswerChains produced
	// 5 chains but only the first carried a terminal literal. Phase
	// 4 promotes to Anchor for the WHOLE batch; Anchor's strict
	// walk returns "" for items without a literal, which the caller
	// drops. The promoted batch yields exactly one symbol.
	itemsMixed := []types.EvidenceItem{
		evidencePhase4RealReproSubExplorer(), // has literal
		{
			ID:        "ev-phase4-no-lit-1",
			Kind:      types.EvidenceConcrete,
			Subject:   "RegisterDefaultSubAgents()",
			Predicate: "binds ONLY",
			Object:    "NewSubExplorer(deps)",
			Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Run()` assigns sk := &skill.Config{",
			Source:    "internal/agent/registry.go",
			LineStart: 19,
			Producer:  "concrete_values",
		},
		{
			ID:        "ev-phase4-no-lit-2",
			Kind:      types.EvidenceConcrete,
			Subject:   "NewBaseAgent()",
			Predicate: "returns",
			Object:    "&BaseAgent{",
			Summary:   "`NewBaseAgent()` returns &BaseAgent{ → `BaseAgent.buildToolSchemas()` assigns err := b.deps.SubAgents.Get(string(b.name))",
			Source:    "internal/agent/agent.go",
			LineStart: 100,
			Producer:  "concrete_values",
		},
	}
	syms := extractAnswerSymbols(
		itemsMixed,
		"call_chain",
		"Identify subagent-invoking agents",
		"list_of_symbols",
		nil,
	)
	// Phase 4's whole point: clean single-symbol result. The items
	// without literals are silently dropped by Anchor's strict walk
	// instead of re-polluting the answer set with class names.
	if len(syms) != 1 {
		t.Fatalf("expected exactly 1 symbol after strict-drop; got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "explorer" {
		t.Errorf("mixed batch: want %q, got %q", "explorer", syms[0].Name)
	}
}

// ============================================================
// hasTerminalLiteral direct coverage
// ============================================================

func TestPhase4_HasTerminalLiteral_Positive(t *testing.T) {
	items := []types.EvidenceItem{evidencePhase4BridgeLiteral()}
	if !hasTerminalLiteral(items) {
		t.Errorf("hasTerminalLiteral should detect the terminal literal in the fixture")
	}
}

func TestPhase4_HasTerminalLiteral_Negative(t *testing.T) {
	// Same shape but terminal hop assigns a struct literal, not a
	// quoted string. hasTerminalLiteral must report false.
	ev := types.EvidenceItem{
		ID:        "ev-phase4-no-literal",
		Kind:      types.EvidenceConcrete,
		Subject:   "Factory()",
		Predicate: "binds ONLY",
		Object:    "NewWidget(deps)",
		Summary:   "`Factory()` binds ONLY NewWidget(deps) → `Widget.Run()` assigns cfg := &Config{}",
		Source:    "internal/widget/factory.go",
		LineStart: 10,
		Producer:  "concrete_values",
	}
	if hasTerminalLiteral([]types.EvidenceItem{ev}) {
		t.Errorf("hasTerminalLiteral should not false-positive on struct literal assignment")
	}
}

func TestPhase4_HasTerminalLiteral_EmptyBatch(t *testing.T) {
	if hasTerminalLiteral(nil) || hasTerminalLiteral([]types.EvidenceItem{}) {
		t.Errorf("hasTerminalLiteral must return false for empty input")
	}
}

// ============================================================
// Identifier-shape filter (the docstring-literal no-bait case)
// ============================================================
// Real 2026-04-13 repro surfaced this failure mode: when L0-1
// ranking includes an unrelated chain whose terminal hop is a
// long narrative literal (`GrepTool.Description() returns "Search
// file contents by regex pattern..."`), the Anchor walker would
// return that literal verbatim. isIdentifierLiteral is the
// structural filter that keeps Anchor aligned with the analyzer's
// list_of_symbols contract ("SET OF IDENTIFIER NAMES"), skipping
// long narrative literals via length/whitespace rules only.

func TestPhase4_IsIdentifierLiteral_Accepts(t *testing.T) {
	accept := []string{
		"explorer",
		"propose_sub_agents",
		"widget-handler",
		"users",
		"/api/v1/users",
		"SomeReallyLongButAcceptablePascalCaseTypeName",
		"user.list",
		"a",
	}
	for _, s := range accept {
		if !isIdentifierLiteral(s) {
			t.Errorf("expected %q to be identifier-shaped", s)
		}
	}
}

func TestPhase4_IsIdentifierLiteral_Rejects(t *testing.T) {
	reject := []string{
		"",
		"a sentence with spaces",
		"Search file contents by regex pattern. Use this to find where a symbol or string appears...",
		"multi\nline",
		"tab\there",
		"   ",
		// > 64 characters, still identifier-ish but pathologically long:
		"ThisIdentifierIsWayTooLongToReasonablyRepresentARealIdentifierInAnyCodebase",
	}
	for _, s := range reject {
		if isIdentifierLiteral(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

// evidencePhase4GrepDescription mirrors the exact pollution shape
// from the 2026-04-13 real repro: a RegisterDefaults chain whose
// terminal hop is a GrepTool.Description() returning a long
// docstring. This chain legitimately passes L0-1 (it's a valid
// `returns "literal"` shape), but the literal is NOT an identifier
// name. Phase 4 must not promote this item into the answer set.
func evidencePhase4GrepDescription() types.EvidenceItem {
	longDoc := `Search file contents by regex pattern. Use this to find where a symbol or string appears. Case handling is smart by default: if the pattern contains no uppercase characters it is matched case-insensitively`
	return types.EvidenceItem{
		ID:        "ev-phase4-grep-doc",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterDefaults()",
		Predicate: "binds",
		Object:    "&GrepTool{}, &ReadFile{}, &ListFiles{}",
		Summary:   "`RegisterDefaults()` binds &GrepTool{}, &ReadFile{}, &ListFiles{} → `GrepTool.Description()` returns \"" + longDoc + "\"",
		Source:    "internal/tool/registry.go",
		LineStart: 22,
		Producer:  "concrete_values",
	}
}

func TestPhase4_DocstringLiteralDoesNotPollute(t *testing.T) {
	// A mixed batch with one bridge-literal item and one docstring-
	// literal item. Phase 4 gate fires (there IS an identifier
	// literal in the batch — "explorer" in the first item). Anchor
	// walks both items: first returns "explorer", second skips the
	// long docstring and finds no other literal, returning "" which
	// drops the item. Result: clean single-symbol output.
	items := []types.EvidenceItem{
		evidencePhase4RealReproSubExplorer(),
		evidencePhase4GrepDescription(),
	}
	syms := extractAnswerSymbols(
		items,
		"call_chain",
		"Identify subagent-invoking agents",
		"list_of_symbols",
		nil,
	)
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol (grep docstring must be dropped), got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "explorer" {
		t.Errorf("want %q, got %q", "explorer", syms[0].Name)
	}
}

func TestPhase4_GateDoesNotFireWhenOnlyDocstringLiteralsExist(t *testing.T) {
	// If every item in the batch carries ONLY non-identifier
	// literals, the Phase 4 gate must not fire — promoting to Anchor
	// would just drop every item via the strict walk, losing signal.
	// The legacy Terminal path stays active and yields class-name
	// picks for items it can handle.
	items := []types.EvidenceItem{evidencePhase4GrepDescription()}
	syms := extractAnswerSymbols(
		items,
		"call_chain",
		"Identify grep-related tools",
		"list_of_symbols",
		nil,
	)
	// Gate does NOT fire; legacy Terminal path walks the binding
	// LHS of the `registers` shape and picks the first capitalized
	// ident in Object, which is "GrepTool".
	if len(syms) != 1 {
		t.Fatalf("expected legacy Terminal to produce 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "GrepTool" {
		t.Errorf("expected legacy Terminal class pick %q, got %q", "GrepTool", syms[0].Name)
	}
}
