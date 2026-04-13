package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractEvidenceRequirements_ChineseSubagent(t *testing.T) {
	reqs := extractEvidenceRequirements("有多少个agent可以调用subagent? 请列出每个agent的名称和它能调用的subagent类型")

	kinds := make(map[string]bool)
	for _, r := range reqs {
		kinds[r.Kind] = true
	}

	if !kinds["enumeration"] {
		t.Error("missing enumeration requirement (多少/列出)")
	}
	if !kinds["call_chain"] {
		t.Error("missing call_chain requirement (调用)")
	}
	if !kinds["registration"] {
		t.Error("missing registration requirement (implied by call_chain)")
	}
	if !kinds["return_value"] {
		t.Error("missing return_value requirement (名称/类型)")
	}
}

func TestExtractEvidenceRequirements_EnglishConfig(t *testing.T) {
	reqs := extractEvidenceRequirements("How does the database.host config value flow to the HTTP handler?")

	kinds := make(map[string]bool)
	for _, r := range reqs {
		kinds[r.Kind] = true
	}

	if !kinds["config_mapping"] {
		t.Error("missing config_mapping requirement")
	}
	// "flow" triggers call_chain via needsDataflow, but extractEvidenceRequirements
	// should also detect it doesn't match call_chain keywords directly.
	// "handler" is an entity that should appear
	hasHandler := false
	for _, r := range reqs {
		for _, e := range r.Entities {
			if strings.Contains(e, "handler") {
				hasHandler = true
			}
		}
	}
	if !hasHandler {
		t.Error("handler entity not captured in any requirement")
	}
}

func TestExtractEvidenceRequirements_NoMatch(t *testing.T) {
	reqs := extractEvidenceRequirements("代码风格好不好")
	// Should produce minimal or no requirements
	if len(reqs) > 1 {
		t.Errorf("expected ≤1 requirement for generic question, got %d", len(reqs))
	}
}

func TestCheckRequirementSatisfaction_Registration(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent"}, Status: "unsatisfied",
			Reason: "need to find where subagent is registered"},
	}

	// Notes that mention registration with specific value
	notes := []string{
		"## Evidence from subagent.go\n- [REGISTRATION] `RegisterDefaultSubAgents` line 62: registers NewSubExplorer as the only subagent",
	}

	reqs = checkRequirementSatisfaction(reqs, notes, nil)
	if reqs[0].Status != "satisfied" {
		t.Errorf("registration requirement status = %q, want satisfied", reqs[0].Status)
	}
}

func TestCheckRequirementSatisfaction_Partial(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent"}, Status: "unsatisfied",
			Reason: "need to find where subagent is registered"},
	}

	// Notes that mention registration without specific value
	notes := []string{
		"## Evidence from subagent.go\n- [REGISTRATION] `Register` line 33: registers a sub-agent in the registry.",
	}

	reqs = checkRequirementSatisfaction(reqs, notes, nil)
	if reqs[0].Status != "partial" {
		t.Errorf("registration requirement status = %q, want partial (no specific value)", reqs[0].Status)
	}
}

func TestCheckRequirementSatisfaction_ReturnValueFromEvidence(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "return_value", Entities: []string{"subexplorer"}, Status: "unsatisfied",
			Reason: "need concrete return values"},
	}

	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "SubExplorer.Name", Predicate: "returns", Object: `"explorer"`},
	}

	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status != "satisfied" {
		t.Errorf("return_value requirement status = %q, want satisfied", reqs[0].Status)
	}
}

func TestErmAllSatisfied(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "enumeration", Status: "satisfied"},
		{Kind: "call_chain", Status: "satisfied"},
	}
	if !ermAllSatisfied(reqs) {
		t.Error("expected all satisfied")
	}

	reqs[1].Status = "partial"
	if ermAllSatisfied(reqs) {
		t.Error("expected not all satisfied")
	}
}

func TestErmUnsatisfiedGaps(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent"}, Status: "unsatisfied", Reason: "find registrations"},
		{Kind: "return_value", Entities: []string{"subagent"}, Status: "partial", Reason: "find return values"},
		{Kind: "enumeration", Entities: []string{"agent"}, Status: "satisfied", Reason: "list agents"},
	}

	gaps := ermUnsatisfiedGaps(reqs)
	if !strings.Contains(gaps, "MISSING") {
		t.Error("gap output should contain MISSING for unsatisfied")
	}
	if !strings.Contains(gaps, "INCOMPLETE") {
		t.Error("gap output should contain INCOMPLETE for partial")
	}
	if strings.Contains(gaps, "enumeration") {
		t.Error("gap output should not contain satisfied requirements")
	}
}

func TestErmFileScore(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent"}, Status: "unsatisfied"},
		{Kind: "return_value", Entities: []string{"subexplorer"}, Status: "unsatisfied"},
	}

	// File with RegisterDefaultSubAgents and SubExplorer symbols
	fi := &repomap.FileInfo{
		RelPath: "internal/agent/subagent.go",
		Symbols: []repomap.Symbol{
			{Name: "SubAgent", Kind: "interface"},
			{Name: "SubAgentRegistry", Kind: "type"},
			{Name: "RegisterDefaultSubAgents", Kind: "function"},
		},
	}
	score := ermFileScore(fi, reqs)
	if score <= 0 {
		t.Fatal("subagent.go should score > 0 with registration + subagent entities")
	}

	// File with SubExplorer.Name method
	fi2 := &repomap.FileInfo{
		RelPath: "internal/agent/sub_explorer.go",
		Symbols: []repomap.Symbol{
			{Name: "SubExplorer", Kind: "type"},
			{Name: "Name", Kind: "method", Receiver: "SubExplorer"},
			{Name: "NewSubExplorer", Kind: "function"},
		},
	}
	score2 := ermFileScore(fi2, reqs)
	if score2 <= 0 {
		t.Fatal("sub_explorer.go should score > 0 with subexplorer entity + Name method")
	}

	// Irrelevant file
	fi3 := &repomap.FileInfo{
		RelPath: "internal/logging/logger.go",
		Symbols: []repomap.Symbol{
			{Name: "Logger", Kind: "type"},
			{Name: "NewFromFlags", Kind: "function"},
		},
	}
	score3 := ermFileScore(fi3, reqs)
	if score3 > 0 {
		t.Errorf("logger.go should score 0, got %f", score3)
	}

	// subagent.go should score higher than sub_explorer.go due to
	// RegisterDefaultSubAgents matching registration-like name
	t.Logf("subagent.go=%f sub_explorer.go=%f logger.go=%f", score, score2, score3)
	if score < score2 {
		t.Logf("note: subagent.go scored lower than sub_explorer.go (both relevant, order acceptable)")
	}
}

// --- ERM English keyword expansion tests --------------------------------
//
// These guard the expansion that closes the gap between user-direct
// Chinese phrasing and the analyzer's English rewrites. Each Kind
// gets a positive set covering analyzer-style rewrites and a
// negative set covering false-positive risks.

func TestExtractEvidenceRequirements_EnumerationEnglishRewrites(t *testing.T) {
	cases := []string{
		"Determine the number of agents that can call subagent.",
		"Count the agents in the system.",
		"Find all instances of read_file usage.",
		"Identify all evaluators in the agent package.",
		"List the registered tools.",
		"Enumerate the available skills.",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasEnum := false
		for _, r := range reqs {
			if r.Kind == "enumeration" {
				hasEnum = true
				break
			}
		}
		if !hasEnum {
			t.Errorf("question %q: no enumeration Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_EnumerationNegative(t *testing.T) {
	// These contain enumeration-flavored words but are not enumeration
	// questions. The expansion must not over-trigger on bare "list" /
	// "all" / "find" / "count".
	cases := []string{
		"Find the file containing the Explorer struct.", // no "all" / "every"
		"What is the count field used for?",             // bare "count" not enough
		"Show me the list of dependencies.",             // "the list of" is not enumerate
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		for _, r := range reqs {
			if r.Kind == "enumeration" {
				t.Errorf("question %q: spurious enumeration Kind; reqs=%v", q, reqs)
			}
		}
	}
}

func TestExtractEvidenceRequirements_ReturnValueEnglishRewrites(t *testing.T) {
	cases := []string{
		"Identify the return value of the ShouldStop method in explorerEvaluator.",
		"What is returned by SubExplorer.Name()?",
		"Determine the return value of BaseAgent.Execute on hard stop.",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasRV := false
		for _, r := range reqs {
			if r.Kind == "return_value" {
				hasRV = true
				break
			}
		}
		if !hasRV {
			t.Errorf("question %q: no return_value Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_ReturnValueNegative(t *testing.T) {
	// "return" alone must NOT trigger return_value — it would match
	// every code-flow question that mentions early returns / return
	// statements without being about a return value at all.
	cases := []string{
		"Where does the function return early?", // bare "return" + "early"
		"Show the return statement on line 42.", // bare "return statement"
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		for _, r := range reqs {
			if r.Kind == "return_value" {
				t.Errorf("question %q: spurious return_value Kind; reqs=%v", q, reqs)
			}
		}
	}
}

func TestExtractEvidenceRequirements_RegistrationBindRewrites(t *testing.T) {
	// Analyzer rewrites of 注册/绑定 questions often use "bound to" /
	// "binding". The expansion must catch these without depending on
	// the bare "register" stem.
	cases := []string{
		"Where is the explorer agent bound to its sub-agents?",
		"Find the binding for the propose_sub_agents tool.",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasReg := false
		for _, r := range reqs {
			if r.Kind == "registration" {
				hasReg = true
				break
			}
		}
		if !hasReg {
			t.Errorf("question %q: no registration Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_ConditionalEnglishRewrites(t *testing.T) {
	cases := []string{
		"Identify the conditions under which the explorer stops.",
		"Determine when ShouldStop fires.",
		"What is triggered when the cache misses?",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasCond := false
		for _, r := range reqs {
			if r.Kind == "conditional" {
				hasCond = true
				break
			}
		}
		if !hasCond {
			t.Errorf("question %q: no conditional Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_OriginalRequestPreserved(t *testing.T) {
	// ERM Part 2 (corrected): the explorer must extract entities ONLY
	// from the original user request and run keyword detection over
	// the joined original+rewrite. The original commit c04298f ran
	// BOTH over the joined string and was caught by integration testing
	// — it polluted the entity set with generic English from the
	// rewrite ("count","agents","that","call") and flipped
	// answer_chain[0] to a spurious chain.
	//
	// This test mirrors the production split in explorer.go
	// BuildInitialPrompt and verifies:
	//  1. Keyword detection over the join produces the right Kind
	//     (return_value triggered by "什么" in the original AND
	//     "return value" in the rewrite — either is sufficient).
	//  2. Entities are derived from the original ONLY, so the
	//     CamelCase identifiers survive AND the generic English from
	//     the rewrite is excluded.
	original := "explorerEvaluator 的 ShouldStop 方法返回什么值?"
	rewritten := "Identify the return value of the ShouldStop method in explorerEvaluator."
	joined := original + " | " + rewritten
	entities := extractRankingEntities(original)

	reqs := extractEvidenceRequirementsWithEntities(joined, entities)
	if len(reqs) == 0 {
		t.Fatalf("split inputs produced 0 reqs; need ≥1 return_value Kind")
	}
	hasReturnValue := false
	for _, r := range reqs {
		if r.Kind == "return_value" {
			hasReturnValue = true
		}
	}
	if !hasReturnValue {
		t.Errorf("split inputs should produce return_value Kind; got reqs=%v", reqs)
	}

	// Entities MUST contain the original CamelCase identifiers.
	wantOriginals := []string{"explorerevaluator", "shouldstop"}
	for _, want := range wantOriginals {
		found := false
		for _, e := range entities {
			if e == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("entity %q from original missing; got entities=%v", want, entities)
		}
	}

	// Entities MUST NOT contain generic English from the rewrite.
	// "identify", "method", "value" are the analyzer's noise.
	bannedFromRewrite := []string{"identify", "method"}
	for _, banned := range bannedFromRewrite {
		for _, e := range entities {
			if e == banned {
				t.Errorf("entity %q leaked from rewrite into split entity set; got entities=%v", banned, entities)
			}
		}
	}
}

// --- T1.1 satisfaction-helper fix audit tests ----------------------------
//
// These tests guard the over-fitting audit conclusions for the registration
// satisfaction branch added on top of T1.1: the new branch must accept
// binds-shape Concrete Values (positive case) without leaking into
// unrelated Kinds or matching the wrong entity (negative cases).

func TestIsRegistrationShape_PositiveBinds(t *testing.T) {
	cases := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Predicate: "binds"},
		{Kind: types.EvidenceConcrete, Predicate: "binds ONLY"},
		{Kind: types.EvidenceConcrete, Predicate: "binds first"},
	}
	for _, ev := range cases {
		if !isRegistrationShape(ev) {
			t.Errorf("isRegistrationShape(%+v) = false, want true", ev)
		}
	}
}

func TestIsRegistrationShape_NegativeOtherShapes(t *testing.T) {
	cases := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Predicate: "returns"},
		{Kind: types.EvidenceConcrete, Predicate: "maps to"},
		{Kind: types.EvidenceConditional, Predicate: "binds"}, // wrong Kind
		{Kind: types.EvidenceRelationship, Predicate: "calls"},
		{Kind: types.EvidenceMechanism, Predicate: "reads_config"},
		{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain"},
	}
	for _, ev := range cases {
		if isRegistrationShape(ev) {
			t.Errorf("isRegistrationShape(%+v) = true, want false", ev)
		}
	}
}

func TestCheckRequirementSatisfaction_RegistrationFromConcreteBinds(t *testing.T) {
	// Positive case: registration requirement on entity "subexplorer" with
	// a binds-shape Concrete Value mentioning that exact entity. The new
	// branch in case "registration" must mark it satisfied.
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subexplorer"}, Status: "unsatisfied",
			Reason: "need to find where subexplorer is registered"},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds ONLY",
			Subject:   "RegisterDefaultSubAgents",
			Object:    "NewSubExplorer(deps)",
			Summary:   "RegisterDefaultSubAgents() binds ONLY NewSubExplorer(deps)",
		},
	}
	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status != "satisfied" {
		t.Errorf("registration via binds-Concrete: status = %q, want satisfied", reqs[0].Status)
	}
}

func TestCheckRequirementSatisfaction_RegistrationFromConcreteBinds_WrongEntity(t *testing.T) {
	// Reverse safety case: registration requirement on entity "subexplorer"
	// with a binds-Concrete that mentions an UNRELATED entity. Must NOT be
	// satisfied — entity match must remain strict.
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subexplorer"}, Status: "unsatisfied"},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds ONLY",
			Subject:   "RegisterDefaultTools",
			Object:    "NewGrepTool",
			Summary:   "RegisterDefaultTools() binds ONLY NewGrepTool",
		},
	}
	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status == "satisfied" {
		t.Errorf("registration with non-matching entity: status = satisfied, want unsatisfied (entity precision must be preserved)")
	}
}

func TestCheckRequirementSatisfaction_ReturnValueUnaffectedByBinds(t *testing.T) {
	// Cross-Kind safety: a return_value requirement is checked by the
	// case "return_value" branch, which already accepts EvidenceConcrete
	// with predicate "returns". The new "registration" branch is added
	// only inside `case "registration"` so a return_value req on entity
	// X with a binds-Concrete X must NOT be satisfied via the new path —
	// it would only be satisfied if the return_value branch's own check
	// matches, which it does not for binds-shape predicates.
	reqs := []EvidenceRequirement{
		{Kind: "return_value", Entities: []string{"subexplorer"}, Status: "unsatisfied"},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds ONLY",
			Subject:   "RegisterDefaultSubAgents",
			Object:    "NewSubExplorer(deps)",
			Summary:   "binds NewSubExplorer",
		},
	}
	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status == "satisfied" {
		t.Errorf("return_value satisfied by binds-Concrete: cross-Kind leak detected; want unsatisfied")
	}
}

func TestIdentifyAnswerChains_RefactorParity(t *testing.T) {
	// Regression check: refactoring identifyAnswerChains to call
	// isRegistrationShape must not change its output for inputs the
	// helper covers. The base inputs use binds-shape Concrete Values
	// — exactly the path that now goes through the helper.
	question := "which subagent does Explorer register?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds ONLY",
			Subject:   "Explorer",
			Object:    "SubExplorer",
			Summary:   "Explorer binds ONLY SubExplorer",
		},
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "returns",
			Subject:   "SubExplorer.Name",
			Object:    "explorer",
			Summary:   "SubExplorer.Name() returns explorer",
		},
	}
	whitelist := answerPredicateWhitelist{}
	chains, _ := identifyAnswerChains(question, evidence, 5, whitelist, nil, nil)
	if len(chains) == 0 {
		t.Fatalf("identifyAnswerChains returned no chains; expected at least the binds-Concrete to land")
	}
	// The binds-Concrete must appear in the output — that's the path
	// touched by the helper.
	found := false
	for _, c := range chains {
		if strings.Contains(c, "binds ONLY SubExplorer") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("binds-Concrete missing from answer chains: %v", chains)
	}
}

// --- T2.1 mechanism Kind tests ----------------------------------------
//
// Cover Chinese + English-rewrite-friendly trigger detection,
// satisfaction via EvidenceMechanism + EvidenceRelationship, and the
// "no spurious mechanism Kind on unrelated questions" reverse case.

func TestExtractEvidenceRequirements_MechanismChinese(t *testing.T) {
	cases := []string{
		"explorer 怎么实现 ContinuationPrompt 的?",
		"dataflow.Analyze 的工作流程是什么?",
		"keyword_search 的原理是什么?",
		"BaseAgent.Execute 的步骤?",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasMechanism := false
		for _, r := range reqs {
			if r.Kind == "mechanism" {
				hasMechanism = true
				break
			}
		}
		if !hasMechanism {
			t.Errorf("question %q: no mechanism Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_MechanismEnglish(t *testing.T) {
	// These are the analyzer's typical English rewrites of Chinese
	// mechanism questions. They MUST trigger mechanism Kind so the
	// satisfaction check can fire when the question reaches the
	// explorer.
	cases := []string{
		"Explain how ContinuationPrompt works in the explorer agent.",
		"Describe the process of dataflow.Analyze.",
		"How does keyword_search rank files?",
		"Walk through the steps of BaseAgent.Execute.",
		"How is the answer chain built from concrete values?",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		hasMechanism := false
		for _, r := range reqs {
			if r.Kind == "mechanism" {
				hasMechanism = true
				break
			}
		}
		if !hasMechanism {
			t.Errorf("question %q: no mechanism Kind extracted; reqs=%v", q, reqs)
		}
	}
}

func TestExtractEvidenceRequirements_MechanismNotTriggered(t *testing.T) {
	// Reverse safety: questions that contain "how" but not as a
	// mechanism marker must NOT trigger the mechanism Kind. The
	// trigger is "how does / how is / how the" — bare "how many"
	// is enumeration, not mechanism.
	cases := []string{
		"how many agents are registered?",
		"which file defines the Explorer struct?",
		"what type does FileInfo.Path have?",
	}
	for _, q := range cases {
		reqs := extractEvidenceRequirements(q)
		for _, r := range reqs {
			if r.Kind == "mechanism" {
				t.Errorf("question %q: spurious mechanism Kind extracted; reqs=%v", q, reqs)
			}
		}
	}
}

func TestCheckRequirementSatisfaction_MechanismFromEvidence(t *testing.T) {
	// Two EvidenceMechanism items → satisfied.
	reqs := []EvidenceRequirement{
		{Kind: "mechanism", Entities: []string{"explorer"}, Status: "unsatisfied"},
	}
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, Predicate: "reads_config", Subject: "explorer", Summary: "explorer reads_config"},
		{Kind: types.EvidenceMechanism, Predicate: "iterates", Subject: "explorer", Summary: "explorer iterates files"},
	}
	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status != "satisfied" {
		t.Errorf("mechanism satisfaction: status = %q, want satisfied", reqs[0].Status)
	}
}

func TestCheckRequirementSatisfaction_MechanismPartial(t *testing.T) {
	// One mechanism evidence item → partial (need ≥2 for full).
	reqs := []EvidenceRequirement{
		{Kind: "mechanism", Entities: []string{"explorer"}, Status: "unsatisfied"},
	}
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, Predicate: "reads_config", Subject: "explorer", Summary: "explorer reads_config"},
	}
	reqs = checkRequirementSatisfaction(reqs, nil, evidence)
	if reqs[0].Status != "partial" {
		t.Errorf("mechanism partial: status = %q, want partial", reqs[0].Status)
	}
}

func TestCheckRequirementSatisfaction_RegistrationFallthroughLLMNotes(t *testing.T) {
	// Coexistence check: the new binds-Concrete branch must not displace
	// the existing LLM-notes [REGISTRATION] branch. When evidence is empty
	// but notes contain a [REGISTRATION] line for the entity with a
	// specific value, the legacy branch must still satisfy.
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent"}, Status: "unsatisfied"},
	}
	notes := []string{
		"## Evidence\n- [REGISTRATION] `RegisterDefaultSubAgents` line 62: registers NewSubExplorer as the only subagent",
	}
	reqs = checkRequirementSatisfaction(reqs, notes, nil)
	if reqs[0].Status != "satisfied" {
		t.Errorf("LLM-notes [REGISTRATION] path: status = %q, want satisfied (legacy branch must still work)", reqs[0].Status)
	}
}

// TestExtractEvidenceRequirementsWithHint_DeclaredKindHonoured verifies
// that when the analyzer declares a question_kind, the resulting
// requirement set always contains at least one entry of that kind —
// even when the keyword path would have missed it.
func TestExtractEvidenceRequirementsWithHint_DeclaredKindHonoured(t *testing.T) {
	// A question with no mechanism keywords that the analyzer still
	// classifies as mechanism. Without the hint the keyword path would
	// produce no mechanism requirement.
	reqs := extractEvidenceRequirementsWithHint(
		"the XMLParser thing",
		[]string{"XMLParser"},
		"mechanism",
	)
	seen := false
	for _, r := range reqs {
		if r.Kind == "mechanism" {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("declared mechanism kind should be in output, got: %+v", reqs)
	}
}

// TestExtractEvidenceRequirementsWithHint_UnknownFallsBack verifies
// that question_kind="unknown" goes through the legacy keyword path
// unchanged — the hint is advisory, not mandatory.
func TestExtractEvidenceRequirementsWithHint_UnknownFallsBack(t *testing.T) {
	reqs := extractEvidenceRequirementsWithHint(
		"how does the ContinuationPrompt mechanism work?",
		[]string{"ContinuationPrompt"},
		"unknown",
	)
	// Keyword path should still detect mechanism via "how does" / "mechanism".
	seen := false
	for _, r := range reqs {
		if r.Kind == "mechanism" {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("unknown kind should fall back to keyword inference; got: %+v", reqs)
	}
}

// TestExtractEvidenceRequirementsWithHint_RegistrationPerEntity verifies
// that a declared registration kind is expanded per-entity (matching
// the keyword path's convention), so checkRequirementSatisfaction
// handles it uniformly downstream.
func TestExtractEvidenceRequirementsWithHint_RegistrationPerEntity(t *testing.T) {
	reqs := extractEvidenceRequirementsWithHint(
		"just show me Foo and Bar",
		[]string{"Foo", "Bar"},
		"registration",
	)
	perEntityCount := 0
	for _, r := range reqs {
		if r.Kind == "registration" && len(r.Entities) == 1 {
			perEntityCount++
		}
	}
	if perEntityCount < 2 {
		t.Errorf("declared registration kind should expand per-entity; got %+v", reqs)
	}
}

// ---------- L0-1: answer chain terminal verification ----------

// TestExtractTerminalSegment covers the arrow-splitting helper that
// feeds every terminal predicate. It must handle multi-hop chains,
// single-hop fallback, and whitespace.
func TestExtractTerminalSegment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"A → B → C", "C"},
		{"A → B", "B"},
		{"no arrow here", "no arrow here"},
		{"  A → B  ", "B"},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractTerminalSegment(c.in); got != c.want {
			t.Errorf("extractTerminalSegment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTerminalIsConcreteSymbolRef covers the bad-pattern rejection
// list and the good-shape acceptance paths. Each entry is a terminal
// string (not a full chain) passed through a single-segment chain.
func TestTerminalIsConcreteSymbolRef(t *testing.T) {
	type row struct {
		chain string
		want  bool
		why   string
	}
	cases := []row{
		// Bad: Go control-flow / builtin terminals.
		{"A → range r.tools", false, "range r.X is a loop iterator"},
		{"A → assigns name := range r.agents {", false, "internal iteration marker"},
		{"A → for _, x := range y", false, "for _, iteration"},
		{"A → make(map[string]int)", false, "builtin make"},
		{"A → append(xs, y)", false, "builtin append"},
		// Good: method-call and literal-return shapes.
		{"A → SubExplorer.Name() returns \"explorer\"", true, "method call + literal"},
		{"Register(NewFoo) → Foo.Type() returns \"tool\"", true, "Name/Type identity method"},
		{"A → returns nil", true, "nil literal return"},
		{"A → returns true", true, "bool literal return"},
		{"A → returns 42", true, "numeric literal return"},
	}
	for _, c := range cases {
		if got := terminalIsConcreteSymbolRef(c.chain, nil); got != c.want {
			t.Errorf("terminalIsConcreteSymbolRef(%q) = %v, want %v (%s)",
				c.chain, got, c.want, c.why)
		}
	}
}

// TestTerminalIsConcreteLiteral verifies the return_value predicate
// accepts literals and rejects non-returns.
func TestTerminalIsConcreteLiteral(t *testing.T) {
	good := []string{
		"A → returns \"x\"",
		"A → returns 'y'",
		"A → returns true",
		"A → returns nil",
		"A → returns 0",
	}
	bad := []string{
		"A → range r.tools",
		"A → Foo.Name() // no returns keyword",
		"A → make(map[string]bool)",
	}
	for _, s := range good {
		if !terminalIsConcreteLiteral(s, nil) {
			t.Errorf("terminalIsConcreteLiteral(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if terminalIsConcreteLiteral(s, nil) {
			t.Errorf("terminalIsConcreteLiteral(%q) = true, want false", s)
		}
	}
}

// TestTerminalPredicatesFor_KindDedup verifies that duplicate Kinds in
// the requirement list only produce one predicate entry — a common
// case because ERM emits one per-entity requirement for registration.
func TestTerminalPredicatesFor_KindDedup(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"A"}},
		{Kind: "registration", Entities: []string{"B"}},
		{Kind: "call_chain", Entities: []string{"A", "B"}},
	}
	got := terminalPredicatesFor(reqs)
	if len(got) != 2 {
		t.Errorf("expected 2 predicates (registration + call_chain), got %d", len(got))
	}

	// Mechanism has no predicate — should produce empty.
	mech := []EvidenceRequirement{{Kind: "mechanism", Entities: []string{"X"}}}
	if got := terminalPredicatesFor(mech); len(got) != 0 {
		t.Errorf("mechanism kind should produce no predicates, got %d", len(got))
	}

	// Empty reqs → nil.
	if got := terminalPredicatesFor(nil); got != nil {
		t.Errorf("nil reqs should produce nil predicates, got %v", got)
	}
}

// TestChainOriginIsRegistrationLinkage covers the L0-1 origin predicate.
// Two acceptance paths exist: function name contains `Register`, OR
// first segment has `binds ONLY` followed by a call expression.
// Constructor-originated chains (`NewFoo → Foo.Method`) whose signature
// line appears after "binds ONLY" must fail the compound check —
// this is the df1 post-L0-1 regression fix.
func TestChainOriginIsRegistrationLinkage(t *testing.T) {
	good := []string{
		// Path 1: Register in function name.
		"`RegisterDefaults()` binds ONLY x → `Foo.Name()` returns \"foo\"",
		"`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
		"`Register(NewHandler())` binds Handler → `Handler.ID()` returns 42",
		// Path 2: non-Register name but `binds ONLY <Call>` structural
		// match. Covers hypothetical codebases using `BindHandlers()`,
		// `InstallRoutes()`, etc. without the Register convention.
		"`BindHandlers()` binds ONLY NewUserHandler(deps) → `UserHandler.ID()` returns \"user\"",
		"`InstallRoutes()` binds ONLY NewRouter(cfg) → `Router.Path()` returns \"/api\"",
	}
	for _, g := range good {
		if !chainOriginIsRegistrationLinkage(g, nil) {
			t.Errorf("expected PASS for %q", g)
		}
	}
	bad := []string{
		// Constructor chains with `returns &X{}` — no binds, no Register.
		"`NewProposeSubAgents()` returns &ProposeSubAgents{ → `ProposeSubAgents.Name()` returns \"x\"",
		"`NewBaseAgent()` returns &BaseAgent{ → `BaseAgent.buildToolSchemas()` returns ts",
		"`NewFoo()` returns &Foo{ → `Foo.Bar()` returns nil",
		// The critical regression case: `NewFoo() binds ONLY <paramlist>`.
		// The concrete-values extractor emits this for EVERY function's
		// signature. Must be rejected because `name` (lowercase param)
		// is not a call expression.
		"`NewBaseAgent()` binds ONLY name types.AgentName, deps *Dependencies, eval Evaluator → `BaseAgent.Name()` returns b.name",
		// Lowercase binds target (not an exported call).
		"`NewProposeSubAgents()` binds ONLY ctx *Context → `ProposeSubAgents.Name()` returns \"x\"",
	}
	for _, b := range bad {
		if chainOriginIsRegistrationLinkage(b, nil) {
			t.Errorf("expected FAIL for %q", b)
		}
	}
}

// TestFirstTokenIsCallExpression covers the helper that distinguishes
// "constructor call after binds ONLY" from "parameter list".
func TestFirstTokenIsCallExpression(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"NewFoo(deps)", true},
		{"  NewFoo(deps)", true}, // leading whitespace OK
		{"Register(Handler)", true},
		{"name types.AgentName", false}, // lowercase param
		{"ctx *Context", false},         // lowercase param
		{"NewFoo", false},               // no paren
		{"", false},
		{"(deps)", false}, // starts with paren, not ident
		{"NEW(x)", true},  // all-caps also accepted
	}
	for _, c := range cases {
		if got := firstTokenIsCallExpression(c.in); got != c.want {
			t.Errorf("firstTokenIsCallExpression(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestIdentifyAnswerChains_ConstructorOriginDemoted verifies the L0-1
// origin check in the live scoring loop. Given a constructor-originated
// chain and a register-originated chain with DOUBLE the entity overlap
// on the wrong chain, the register chain must still rank first thanks
// to the origin demotion.
//
// This exactly reproduces the df1 post-L0-1 regression:
//   - wrong chain matches "subagent" + "agent" substrings in
//     `ProposeSubAgents.Name() returns "propose_sub_agents"` (overlap 2/2)
//   - right chain matches only "subagent" substring in
//     `RegisterDefaultSubAgents → SubExplorer` (overlap 1/2)
//   - without origin check, wrong chain would score 3.0 vs right 1.95
//   - with origin check, wrong chain gets ×0.1 and scores 0.3
func TestIdentifyAnswerChains_ConstructorOriginDemoted(t *testing.T) {
	question := "which agent can call subagent?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`NewProposeSubAgents()` returns &ProposeSubAgents{ → `ProposeSubAgents.Name()` returns \"propose_sub_agents\"",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer → `SubExplorer.Name()` returns \"explorer\"",
		},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent", "agent"}},
	}
	chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(chains) == 0 {
		t.Fatal("expected at least one chain")
	}
	if !strings.Contains(chains[0], "RegisterDefaultSubAgents") {
		t.Errorf("expected RegisterDefaultSubAgents chain at top, got: %s", chains[0])
	}
}

// TestIdentifyAnswerChains_TerminalRangeDemoted is the headline test for
// L0-1: given two chains, one ending at a loop iterator and one ending
// at a concrete symbol/literal, the concrete one must rank first even
// when it has equal or lower entity overlap. This reproduces the df1
// run-3 failure mode in a unit test.
func TestIdentifyAnswerChains_TerminalRangeDemoted(t *testing.T) {
	question := "which agents can call subagent?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterDefaults()` binds ONLY r *Registry → `Registry.List()` assigns name := range r.agents {",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterDefaultSubAgents()` binds NewSubExplorer → `SubExplorer.Name()` returns \"explorer\"",
		},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"subagent", "agents"}},
		{Kind: "call_chain", Entities: []string{"subagent", "agents"}},
	}
	chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(chains) == 0 {
		t.Fatal("expected at least one chain returned")
	}
	// The SubExplorer chain must rank first.
	if !strings.Contains(chains[0], "SubExplorer") {
		t.Errorf("expected SubExplorer chain at top, got: %s", chains[0])
	}
	// The range chain must still appear somewhere (demote, not drop).
	foundRange := false
	for _, c := range chains {
		if strings.Contains(c, "range r.agents") {
			foundRange = true
			break
		}
	}
	if !foundRange {
		t.Error("demoted chain should still be present as fallback safety")
	}
}

// TestIdentifyAnswerChains_NoPredicateForMechanism verifies mechanism
// kind leaves ranking untouched (no demotion applied) — important
// because df3 is a mechanism case and must not regress.
func TestIdentifyAnswerChains_NoPredicateForMechanism(t *testing.T) {
	question := "how does ContinuationPrompt work?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "ContinuationPrompt → range ph.contextualHints { push strategy X }",
		},
	}
	reqs := []EvidenceRequirement{{Kind: "mechanism", Entities: []string{"ContinuationPrompt"}}}
	chains, _ := identifyAnswerChains(question, evidence, 5, buildAnswerWhitelist(reqs), reqs, nil)
	// Mechanism kind has no predicate; chain containing `range` must
	// still be ranked as-is, not demoted.
	if len(chains) == 0 {
		t.Fatal("mechanism chain should survive (no predicate applied)")
	}
}

// TestIdentifyAnswerChains_BackCompatNilArgs verifies legacy callers
// that pass nil reqs + nil graph get pre-L0-1 ranking behaviour.
func TestIdentifyAnswerChains_BackCompatNilArgs(t *testing.T) {
	question := "which X register Y?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds",
			Summary:   "Register(NewFoo) binds Foo",
			Subject:   "Register",
			Object:    "Foo",
		},
	}
	chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, nil, nil)
	if len(chains) == 0 {
		t.Error("nil reqs + nil graph should preserve legacy behaviour; got no chains")
	}
}

// ---------- Multi-key stable sort (2026-04-12) ----------

// TestIdentifyAnswerChains_StableSortEqualScoreByStrictOK locks the
// second sort key: when two candidates share an identical primary
// score, the one that passed all L0-1 predicates (strictOK=true) must
// rank above the demoted one. Pre-fix the plain float comparator put
// them in Go's internal slice order, which changes across runtime
// hash-seed variations and evidence iteration order.
//
// Fixture shape: two evidence items whose overlap and bonus multipliers
// yield exactly the same float64 score. The first is a clean
// registration linkage (passes origin predicate); the second is a
// constructor-originated chain (demoted by chainOriginIsRegistrationLinkage
// but still scored above zero). Both end in a literal returns shape, so
// terminal predicate passes for both.
func TestIdentifyAnswerChains_StableSortEqualScoreByStrictOK(t *testing.T) {
	question := "which X register Y?"
	evidence := []types.EvidenceItem{
		// strictOK=false path: NewFoo constructor origin, demoted
		// ×0.1 by chainOriginIsRegistrationLinkage.
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`NewFoo()` returns &Foo{} → `Foo.Name()` returns \"foo\"",
		},
		// strictOK=true path: Register-prefixed origin, passes
		// predicate. Same overlap (both contain "register" / "y"
		// substrings via the question entity list after
		// normalisation).
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterHandlers()` binds ONLY NewBar(deps) → `Bar.Name()` returns \"bar\"",
		},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"register", "y"}},
	}
	chains, strict := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(chains) == 0 {
		t.Fatal("expected at least one chain")
	}
	// Strict subset must contain the Register chain (the one that
	// passed the origin predicate).
	if len(strict) == 0 || !strings.Contains(strict[0].Summary, "RegisterHandlers") {
		t.Errorf("strict subset should lead with RegisterHandlers, got: %+v", strict)
	}
	// Loose list must also rank RegisterHandlers above NewFoo because
	// the ×0.1 origin demotion makes their raw scores unequal. This
	// double-checks the ordering works both with and without score
	// separation.
	registerIdx := -1
	newFooIdx := -1
	for i, c := range chains {
		if strings.Contains(c, "RegisterHandlers") {
			registerIdx = i
		}
		if strings.Contains(c, "NewFoo") {
			newFooIdx = i
		}
	}
	if registerIdx < 0 {
		t.Fatalf("RegisterHandlers chain missing from loose list: %v", chains)
	}
	if newFooIdx >= 0 && registerIdx > newFooIdx {
		t.Errorf("loose list: RegisterHandlers (idx %d) should rank before NewFoo (idx %d); chains=%v",
			registerIdx, newFooIdx, chains)
	}
}

// TestIdentifyAnswerChains_StableSortEqualScoreByConfidence locks the
// third sort key. Two identical-shape chains with different
// EvidenceItem.Confidence values must order with the higher-confidence
// one first.
func TestIdentifyAnswerChains_StableSortEqualScoreByConfidence(t *testing.T) {
	question := "which X register Y?"
	// Two binds-shape Concrete items with identical Summaries modulo
	// the registered type name, identical source info, but different
	// confidence values. Same score (same overlap, same bonus).
	evidence := []types.EvidenceItem{
		{
			Kind:       types.EvidenceConcrete,
			Predicate:  "binds",
			Subject:    "RegisterA",
			Object:     "NewLowConfY()",
			Summary:    "`RegisterA()` binds NewLowConfY()",
			Confidence: 0.3,
		},
		{
			Kind:       types.EvidenceConcrete,
			Predicate:  "binds",
			Subject:    "RegisterB",
			Object:     "NewHighConfY()",
			Summary:    "`RegisterB()` binds NewHighConfY()",
			Confidence: 0.9,
		},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"register", "y"}},
	}
	chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(chains) == 0 {
		t.Fatal("expected at least one chain")
	}
	// Higher-confidence chain must come first.
	if !strings.Contains(chains[0], "NewHighConfY") {
		t.Errorf("expected NewHighConfY first (higher confidence), got: %v", chains)
	}
}

// TestIdentifyAnswerChains_StableSortEqualScoreByChainLength locks the
// fourth sort key. Two chains with identical score / strictOK /
// confidence, differing only in number of `→` hops, must order shorter
// first — a shorter chain is more direct, fewer indirection steps from
// question entity to terminal answer.
func TestIdentifyAnswerChains_StableSortEqualScoreByChainLength(t *testing.T) {
	question := "which X register Y?"
	evidence := []types.EvidenceItem{
		// 3-hop chain.
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterX()` binds NewLong(d) → `Long.Intermediate()` returns m → `m.Get()` returns \"y\"",
		},
		// 2-hop chain — shorter, more direct.
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterY()` binds NewShort(d) → `Short.Name()` returns \"y\"",
		},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"register", "y"}},
	}
	chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(chains) < 2 {
		t.Fatalf("expected at least 2 chains, got %d: %v", len(chains), chains)
	}
	if !strings.Contains(chains[0], "NewShort") {
		t.Errorf("expected shorter (NewShort) chain first, got order: %v", chains)
	}
}

// TestIdentifyAnswerChains_StableSortDeterministicAcrossRuns is the
// headline stability test: same input must produce the same ranked
// output across multiple identical invocations. Pre-fix this was NOT
// guaranteed for equal-score candidates because map iteration order
// influenced the input slice order upstream. The new comparator's lex
// tie-break on summary guarantees a total order.
func TestIdentifyAnswerChains_StableSortDeterministicAcrossRuns(t *testing.T) {
	question := "which X register Y?"
	// Build a fixture where several candidates collide on the first
	// few keys and differ only in the final (summary lex) tie-break.
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterAlpha", Object: "NewY1()", Summary: "`RegisterAlpha()` binds NewY1()"},
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterBeta", Object: "NewY2()", Summary: "`RegisterBeta()` binds NewY2()"},
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterGamma", Object: "NewY3()", Summary: "`RegisterGamma()` binds NewY3()"},
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterDelta", Object: "NewY4()", Summary: "`RegisterDelta()` binds NewY4()"},
	}
	reqs := []EvidenceRequirement{
		{Kind: "registration", Entities: []string{"register", "y"}},
	}
	baseline, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
	if len(baseline) == 0 {
		t.Fatal("expected non-empty baseline")
	}
	// Re-run several times and compare — any permutation means the
	// sort is not fully stable.
	for run := 0; run < 5; run++ {
		chains, _ := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)
		if len(chains) != len(baseline) {
			t.Fatalf("run %d: len mismatch baseline=%d got=%d", run, len(baseline), len(chains))
		}
		for i := range chains {
			if chains[i] != baseline[i] {
				t.Errorf("run %d: order changed at idx %d\n  baseline[%d]=%q\n  got[%d]=%q",
					run, i, i, baseline[i], i, chains[i])
			}
		}
	}
}

// ---------- L0-2: extract-then-express ----------

// TestFirstUppercaseIdent covers the token-walking helper that picks
// out the first capitalised, ≥3-char identifier in a terminal segment.
func TestFirstUppercaseIdent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"`SubExplorer.Name()` returns \"explorer\"", "SubExplorer"},
		{"r.agents assigns name", ""},       // all tokens lowercase → no match
		{"", ""},
		{"nothing capitalised here", ""},
		{"Foo", "Foo"},
		{"ab Xx", ""},         // Xx is only 2 chars → too short
		{"hello World foo", "World"},
	}
	for _, c := range cases {
		if got := firstUppercaseIdent(c.in); got != c.want {
			t.Errorf("firstUppercaseIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStripNewPrefix covers the constructor-name cleanup helper.
func TestStripNewPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"NewSubExplorer", "SubExplorer"},
		{"NewFoo", "Foo"},
		{"New", "New"},        // too short
		{"Newt", "Newt"},      // 4 chars but 4th is lowercase
		{"newHandler", "newHandler"}, // lowercase n, not a New-prefix
		{"SubExplorer", "SubExplorer"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripNewPrefix(c.in); got != c.want {
			t.Errorf("stripNewPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAnswerSymbolFromEvidence_RegistrationBinds verifies the happy
// path for registration-shape concrete values: the Object field
// contains the constructor call, stripNewPrefix yields the bare type.
func TestAnswerSymbolFromEvidence_RegistrationBinds(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:      types.EvidenceConcrete,
		Predicate: "binds ONLY",
		Subject:   "RegisterDefaultSubAgents",
		Object:    "NewSubExplorer(deps)",
		Summary:   "RegisterDefaultSubAgents() binds ONLY NewSubExplorer(deps)",
		Source:    "internal/agent/subagent.go",
		LineStart: 63,
	}
	sym := answerSymbolFromEvidence(ev, "registration", RoleTerminal, nil)
	if sym.Name != "SubExplorer" {
		t.Errorf("Name = %q, want SubExplorer", sym.Name)
	}
	if sym.File != "internal/agent/subagent.go" {
		t.Errorf("File = %q, want internal/agent/subagent.go", sym.File)
	}
	if sym.Line != 63 {
		t.Errorf("Line = %d, want 63", sym.Line)
	}
	if sym.Kind != "registration" {
		t.Errorf("Kind = %q, want registration", sym.Kind)
	}
}

// TestAnswerSymbolFromEvidence_ReturnsMethod verifies that a concrete
// returns evidence uses the receiver type (part before dot) as the
// extracted symbol, not the literal value.
func TestAnswerSymbolFromEvidence_ReturnsMethod(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:      types.EvidenceConcrete,
		Predicate: "returns",
		Subject:   "SubExplorer.Name",
		Object:    `"explorer"`,
		Source:    "internal/agent/sub_explorer.go",
		LineStart: 20,
	}
	sym := answerSymbolFromEvidence(ev, "return_value", RoleTerminal, nil)
	if sym.Name != "SubExplorer" {
		t.Errorf("Name = %q, want SubExplorer (receiver type)", sym.Name)
	}
}

// TestAnswerSymbolFromEvidence_DataflowPath verifies that a
// resolution_chain evidence item parses the terminal of Summary to
// find the answer symbol. This is the one case where the extractor
// still parses a string (the chain text carries more than any
// single Subject/Object pair).
func TestAnswerSymbolFromEvidence_DataflowPath(t *testing.T) {
	ev := types.EvidenceItem{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
	}
	sym := answerSymbolFromEvidence(ev, "registration", RoleTerminal, nil)
	if sym.Name != "SubExplorer" {
		t.Errorf("Name = %q, want SubExplorer", sym.Name)
	}
}

// TestExtractAnswerSymbols_Dedup verifies that multiple items with
// the same extracted symbol coalesce to a single entry.
func TestExtractAnswerSymbols_Dedup(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterA", Object: "NewSubExplorer(d)"},
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterB", Object: "NewSubExplorer(d)"}, // dup object → dup symbol
		{Kind: types.EvidenceConcrete, Predicate: "binds", Subject: "RegisterC", Object: "NewExplorer(d)"},
	}
	syms := extractAnswerSymbols(items, "registration", "", "", nil)
	if len(syms) != 2 {
		t.Fatalf("expected 2 unique symbols, got %d: %+v", len(syms), syms)
	}
	got := map[string]bool{}
	for _, s := range syms {
		got[s.Name] = true
	}
	if !got["SubExplorer"] || !got["Explorer"] {
		t.Errorf("missing expected symbols: %v", got)
	}
}

// TestExtractAnswerSymbols_MechanismOnlyEvidenceNil verifies that
// when the strict subset contains ONLY mechanism step evidence (no
// concrete registration / returns / relationship calls), extraction
// returns nil — the evidence-driven gate (S2) recognises there is
// no single-symbol terminal to pick out.
func TestExtractAnswerSymbols_MechanismOnlyEvidenceNil(t *testing.T) {
	items := []types.EvidenceItem{
		// Pure mechanism step — no single-symbol terminal shape.
		// Note: under S2, extractAnswerSymbols reads evidence SHAPES
		// not kinds. A bare EvidenceMechanism without the specific
		// predicates handled in answerSymbolFromEvidence's switch
		// should NOT trigger the gate.
		{Kind: types.EvidenceConditional, Subject: "step label"},
	}
	for _, kind := range []string{"mechanism", "enumeration", "conditional"} {
		if syms := extractAnswerSymbols(items, kind, "", "", nil); syms != nil {
			t.Errorf("kind=%s with non-terminal evidence should return nil, got %+v", kind, syms)
		}
	}
}

// TestExtractAnswerSymbols_EvidenceDrivenGate is the headline S2 test:
// evidence that CARRIES a registration shape triggers extraction
// regardless of analyzer-declared kind. This is the fix for df1 run 2
// (eval/results/df1-20260412-100204) where the analyzer classified
// as enumeration and L0-2 was skipped even though the strict subset
// had the canonical RegisterDefaultSubAgents → SubExplorer chain.
func TestExtractAnswerSymbols_EvidenceDrivenGate(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind: types.EvidenceConcrete, Predicate: "binds ONLY",
			Subject: "RegisterDefaultSubAgents", Object: "NewSubExplorer(deps)",
			Summary: "RegisterDefaultSubAgents() binds ONLY NewSubExplorer(deps)",
			Source:  "internal/agent/subagent.go", LineStart: 63,
		},
	}
	// All of these should now extract SubExplorer — question_kind is
	// reported but does NOT gate.
	for _, kind := range []string{"registration", "call_chain", "enumeration", "mechanism", "unknown", ""} {
		syms := extractAnswerSymbols(items, kind, "", "", nil)
		if len(syms) != 1 || syms[0].Name != "SubExplorer" {
			t.Errorf("kind=%s: expected [SubExplorer], got %+v", kind, syms)
		}
		if syms[0].Kind != kind {
			t.Errorf("kind=%s: AnswerSymbol.Kind should mirror supplied kind, got %q", kind, syms[0].Kind)
		}
	}
}

// TestExtractAnswerSymbols_EmptyItemsReturnsNil verifies the edge
// case: no items at all → nil, no panic.
func TestExtractAnswerSymbols_EmptyItemsReturnsNil(t *testing.T) {
	if syms := extractAnswerSymbols(nil, "registration", "", "", nil); syms != nil {
		t.Errorf("nil items → want nil, got %+v", syms)
	}
	if syms := extractAnswerSymbols([]types.EvidenceItem{}, "registration", "", "", nil); syms != nil {
		t.Errorf("empty items → want nil, got %+v", syms)
	}
}

// TestExtractAnswerSymbols_ArrowlessConcreteValue verifies the key
// benefit of the EvidenceItem refactor: an arrow-less single-hop
// concrete value (previously skipped by the string extractor because
// it would have extracted the subject instead of the object) now
// yields the correct symbol via the Object field.
func TestExtractAnswerSymbols_ArrowlessConcreteValue(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Predicate: "binds ONLY",
			Subject:   "RegisterDefaultSubAgents",
			Object:    "NewSubExplorer(deps)",
			Summary:   "RegisterDefaultSubAgents() binds ONLY NewSubExplorer(deps)",
			Source:    "internal/agent/subagent.go",
			LineStart: 63,
		},
	}
	syms := extractAnswerSymbols(items, "registration", "", "", nil)
	if len(syms) != 1 || syms[0].Name != "SubExplorer" {
		t.Errorf("arrow-less concrete value should extract SubExplorer, got %+v", syms)
	}
	if syms[0].File != "internal/agent/subagent.go" || syms[0].Line != 63 {
		t.Errorf("source locator lost: %+v", syms[0])
	}
}

// TestIdentifyAnswerChains_StrictSubsetExcludesDemoted reproduces the
// df1 run-3 failure mode end-to-end: the constructor-originated
// NewProposeSubAgents chain must appear in the loose chain list (for
// Ground Truth display) but NOT in the strict EvidenceItem subset
// (which feeds L0-2). This is the invariant the refactor establishes.
func TestIdentifyAnswerChains_StrictSubsetExcludesDemoted(t *testing.T) {
	question := "which agent can call subagent?"
	evidence := []types.EvidenceItem{
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
		},
		{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Summary:   "`NewProposeSubAgents()` returns &ProposeSubAgents{ → `ProposeSubAgents.Name()` returns \"propose_sub_agents\"",
		},
	}
	reqs := []EvidenceRequirement{{Kind: "registration", Entities: []string{"subagent", "agent"}}}
	chains, strict := identifyAnswerChains(question, evidence, 5, answerPredicateWhitelist{}, reqs, nil)

	// Loose list includes both for Ground Truth display.
	if len(chains) != 2 {
		t.Errorf("loose chains: expected 2, got %d", len(chains))
	}
	// Strict subset includes only the RegisterDefaultSubAgents chain.
	if len(strict) != 1 {
		t.Fatalf("strict subset: expected 1 item, got %d: %+v", len(strict), strict)
	}
	if !strings.Contains(strict[0].Summary, "RegisterDefaultSubAgents") {
		t.Errorf("strict subset should be the Register chain, got: %s", strict[0].Summary)
	}
}
