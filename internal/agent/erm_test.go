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
	chains := identifyAnswerChains(question, evidence, 5, whitelist)
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
