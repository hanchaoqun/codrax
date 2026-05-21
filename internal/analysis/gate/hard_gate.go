package gate

// hard_gate.go — Phase 1 of the commercial-grade remediation
// (docs/design/commercial_grade_3_pattern_remediation.md).
//
// Declarative metadata-only registry for analyzer / coherence
// hard gates. Each gate listed here declares:
//   - Name           — gate identifier used in reject messages
//   - SourceFile     — file:line of the actual enforcement
//   - TriggerSummary — one-line plain-English description of when
//     the gate fires (used by both audit and skill-prompt advisory)
//   - CarveOuts      — every typed predicate that should LET A
//     STRUCTURALLY-LEGITIMATE-MATCH-TO-TRIGGER pass without firing
//     the reject (e.g. L0-B carves out IsRelationalLookup because
//     "filter set X by relation to Y" legitimately has 1 entity +
//     IsCategoryEnumeration=true). May be empty.
//   - LLMAdvisoryHint — an LLM-natural prose hint generated into
//     the analyzer skill prompt. R6-clean (no internal pipeline
//     terms). Empty when the gate is purely structural and the
//     LLM cannot fix it via classification.
//
// The registry is metadata-only: existing gate enforcement code
// (analyzer.go, coherence.go) keeps its inline check. The registry
// pins the contract via TestAllHardGates_ExplicitCarveOutDeclaration
// (build-time test scans source files and asserts the declared
// CarveOuts substrings appear in the trigger condition).
//
// This metadata+source-scan approach gives us declarative auditing
// without the behavior-drift risk of refactoring 6 gates to a
// runtime Run() interface. Future gates require:
//   1. Add an entry to RegisteredHardGates
//   2. The build-time test verifies the source-file substring match
//   3. The skill prompt advisory section auto-includes the new gate
//
// Skipping any of these steps fails the test or surfaces no LLM
// guidance — neither degrades silently.

// HardGate is the metadata descriptor for one analyzer / coherence
// hard reject gate. Construction is at package init (RegisteredHardGates
// is a package-level slice). Read-only at runtime — the registry is
// purely an audit + skill-prompt data source.
//
// Field-staleness discipline (2026-05-09 design pivot): every field
// here MUST either (a) have a build-time test that fails when the
// field drifts from reality, OR (b) be inherently stable across
// refactors. The original SourceLineHint field violated both — line
// numbers shift with every commit and no test caught the rot — so it
// was removed. SourceFile stays because TestAllHardGates_ExplicitCarveOutDeclaration
// reads it and grep-checks for the carve-out substring; moving a
// gate to a different file fails the test and forces a registry
// update, which is the correct enforcement mechanism.
type HardGate struct {
	// Name is the gate identifier. Must match the substring used in
	// the gate's reject Detail message so audit can correlate. Stable
	// across refactors — the gate name is a public contract surface.
	Name string

	// SourceFile is the file path (relative to repo root) where the
	// gate's enforcement code lives. Read by the build-time test
	// TestAllHardGates_ExplicitCarveOutDeclaration which scans the
	// file content for `!rm.Predicates.<CarveOuts[i].Predicate>`
	// substrings. Refactors that move a gate to a different file
	// fail the test until this field is updated — that test failure
	// IS the cross-developer sync enforcement.
	SourceFile string

	// TriggerSummary is a one-line plain-English description of when
	// the gate fires. Read by skill-prompt advisory generation. R6-
	// clean — no internal pipeline terms. May drift if the gate
	// condition is refactored substantively; the staleness risk is
	// bounded by the LLM-advisory R6 audit (TestPromptSnapshot) and
	// human review at the time of refactor.
	TriggerSummary string

	// CarveOuts lists every typed predicate that legitimately bypasses
	// the gate even when the trigger condition matches. Each entry
	// declares the predicate name (matching the JSON / Go field name
	// the analyzer LLM emits) and the structural reason. May be empty
	// when the gate is a direct structural contradiction with no
	// legitimate carve-out shape. Each CarveOut.Predicate is
	// enforced-in-source by the build-time test (the SourceFile must
	// contain `!rm.Predicates.<Predicate>` as a substring).
	CarveOuts []TypedPredicateCarveOut

	// LLMAdvisoryHint is an LLM-natural prose hint surfaced in the
	// analyzer skill prompt's carve-out advisory section. R6-clean
	// — no internal pipeline terms (no "gate" / "explore" / "reject").
	// Empty when the gate is purely structural and the LLM cannot
	// route around it via classification.
	LLMAdvisoryHint string
}

// TypedPredicateCarveOut declares one carve-out: a typed predicate
// that legitimately bypasses a gate when set to the carve-out value.
type TypedPredicateCarveOut struct {
	// Predicate is the field name the analyzer LLM emits (matches
	// the JSON-tag naming convention used in skill prompt schemas).
	// Examples: "IsRelationalLookup" / "IsCountQuestion".
	Predicate string

	// CarveOutValue is the predicate value that bypasses the gate.
	// Most carve-outs are "true" (the predicate being true exempts
	// the gate); rare carve-outs may exempt on false. Stored as a
	// string for declaration clarity.
	CarveOutValue string

	// Reason is a brief explanation of WHY this predicate carves
	// out the gate — surfaced in error messages and skill prompt
	// hints. R6-clean.
	Reason string
}

// RegisteredHardGates is the canonical list of all analyzer /
// coherence hard reject gates. Adding a new hard gate to the codebase
// requires adding an entry here so:
//  1. The build-time test verifies the source contains the carve-out
//  2. The skill prompt advisory auto-includes the new gate
//  3. Future audits can iterate the registry instead of grepping
//
// Order is documentation-driven (functional → structural). Tests
// index by Name.
var RegisteredHardGates = []HardGate{
	{
		Name:           "L0-B",
		SourceFile:     "internal/agent/analyzer.go",
		TriggerSummary: "Question is classified as a categorical enumeration but only one named entity is supplied, with no relational lookup or scoped source-inventory carrier to explain where the members will be discovered.",
		CarveOuts: []TypedPredicateCarveOut{
			{
				Predicate:     "IsRelationalLookup",
				CarveOutValue: "true",
				Reason:        "Filter-set-by-relation-to-target questions (e.g. 'which packages import X', 'list X's exports') legitimately carry one entity (the relation target); the values come from later investigation.",
			},
		},
		LLMAdvisoryHint: "If the question filters a set by a relationship to a named target (e.g. 'which packages import X', 'list X's exports'), set BOTH `is_category_enumeration=true` AND `is_relational_lookup=true` even when entities contains only the relation target. If it asks for an inventory under a package/path/source scope, keep the scope entity and fill `required_files` plus `source_scope_profile` when known — the values are looked up after classification.",
	},
	{
		Name:           "R2.2",
		SourceFile:     "internal/analysis/gate/coherence.go",
		TriggerSummary: "The compiled answer view expects a long-form payload (prose / call-chain / enumeration) but the answer subject is declared as a single literal value at high confidence.",
		CarveOuts: []TypedPredicateCarveOut{
			{
				Predicate:     "IsCountQuestion",
				CarveOutValue: "true",
				Reason:        "Count questions ('how many X', 'total LOC of Y') legitimately combine a long-form explanation surface with a numeric scalar answer — the count itself is the literal, the explanation is the derivation.",
			},
		},
		LLMAdvisoryHint: "If the question asks for a single computed number that aggregates across multiple source units (e.g. 'how many lines', 'total of N items', 'count of registered handlers'), set BOTH `is_count_question=true` AND `is_scalar_answer=true`. Without `is_count_question=true` the long-form-explanation surface conflicts with the numeric subject and the question is rejected.",
	},
	{
		Name:            "R1.2",
		SourceFile:      "internal/analysis/gate/coherence.go",
		TriggerSummary:  "The question is declared as cross-component (comparing or relating two distinct subsystems) yet zero or one sub-topic is emitted — a cross-component answer requires at least two component sub-topics.",
		CarveOuts:       nil, // direct contradiction
		LLMAdvisoryHint: "",
	},
	{
		Name:            "R1.4",
		SourceFile:      "internal/analysis/gate/coherence.go",
		TriggerSummary:  "Two or more sub-topics are emitted but the question is neither cross-component nor a categorical enumeration — the multiple sub-topics have no structural axis to organise them.",
		CarveOuts:       nil, // structural inconsistency check
		LLMAdvisoryHint: "",
	},
	{
		Name:            "R1.5",
		SourceFile:      "internal/analysis/gate/coherence.go",
		TriggerSummary:  "Sub-topic entity resolution is inconsistent — at least one sub-topic mixes resolver-verified and unresolved entities or shares an entity with another sub-topic.",
		CarveOuts:       nil, // structural check
		LLMAdvisoryHint: "",
	},
}

// HardGateByName returns the registered HardGate with the given
// name, or nil if not found. Used by audit tests and skill-prompt
// generation.
func HardGateByName(name string) *HardGate {
	for i := range RegisteredHardGates {
		if RegisteredHardGates[i].Name == name {
			return &RegisteredHardGates[i]
		}
	}
	return nil
}

// CarveOutPredicates returns the deduplicated list of typed
// predicate names that appear as carve-outs across all registered
// gates. Used by skill-prompt advisory generation to enumerate all
// LLM-emit-able predicates that can route around at-risk gates.
func CarveOutPredicates() []string {
	seen := make(map[string]bool)
	var out []string
	for _, g := range RegisteredHardGates {
		for _, co := range g.CarveOuts {
			if !seen[co.Predicate] {
				seen[co.Predicate] = true
				out = append(out, co.Predicate)
			}
		}
	}
	return out
}
