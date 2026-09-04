package skill_test

import (
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInPrompts scans every LLM-facing string inside
// every registered skill config for implementation-jargon tokens from
// the glossary (InternalTermsBlocklist + ProjectSpecificIdentifierBlocklist)
// through the single shared scanner, glossarylint.ScanText. It is the
// registry-rendered lane of the jargon gate family; the renderer
// packages (agent, context, tool, orchestrator, …) are covered by
// glossarylint.RendererRoster and its totality census.
//
// Batch 1 (report-only): violations were reported via t.Log. Batch 2A
// cleaned the skill-config surfaces and batch 4B flipped this gate to
// t.Fatal so any regression aborts the test run. §40.52 moved the file
// to the external test package so it can import glossarylint (which
// imports skill for the vocabulary) and retired the local matcher copy.
//
// Scope:
//   - skill.BuildAnalysisSkill() — the analyzer's declarative contract
//   - every entry the default registry carries (explore / extract /
//     answer-document / log-triage / log-segmentation)
//
// Each skill contributes Goal, OutputFormat, and the Workflow /
// Prohibitions / Tier B / ToolSuggestions slices. ToolSuggestions is
// scanned for completeness but is very unlikely to contain jargon
// because its entries are tool names; including it keeps the lint
// surface uniform with the rest of the Config.
func TestNoInternalTermsInPrompts(t *testing.T) {
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)

	names := reg.List()
	if len(names) == 0 {
		t.Fatalf("empty skill registry — RegisterDefaults did not register any skill")
	}

	var hits []glossarylint.Hit
	for _, name := range names {
		sk, err := reg.Get(name)
		if err != nil {
			t.Fatalf("Registry.Get(%q): %v", name, err)
		}
		hits = append(hits, scanConfig(sk)...)
	}

	if len(hits) == 0 {
		return
	}

	// Adding a new blocklist token without first cleaning the existing
	// surfaces fails here — by design, so a one-sided cleanup cannot
	// land. Per-violation lines via t.Errorf, summary via t.Fatalf so
	// the full list is visible in a single run.
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoInternalTermsInPrompts found %d violation(s); rephrase in user-facing language or extend internal/skill/glossary.go :: InternalTermsBlocklist", len(hits))
}

// scanConfig walks every LLM-facing string in a Config and returns
// one hit per glossary token found.
func scanConfig(sk *skill.Config) []glossarylint.Hit {
	var out []glossarylint.Hit
	prefix := sk.Name + "."

	out = append(out, glossarylint.ScanText(prefix+"Goal", sk.Goal)...)
	for i, w := range sk.Workflow {
		out = append(out, glossarylint.ScanText(prefix+"Workflow["+strconv.Itoa(i)+"]", w)...)
	}
	out = append(out, glossarylint.ScanText(prefix+"OutputFormat", sk.OutputFormat)...)
	for i, p := range sk.Prohibitions {
		out = append(out, glossarylint.ScanText(prefix+"Prohibitions["+strconv.Itoa(i)+"]", p)...)
	}
	// Tier B bodies are LLM-facing exactly like their Tier A
	// counterparts — the renderer splices them into the same Workflow /
	// Prohibitions surface when the applicability filter matches
	// (CMP-C F2, adversarial review 2026-07-04: WorkflowTierB was a
	// lint blind spot, so conditionally-rendered jargon could ship
	// unscanned). Mechanical guard across ALL skills.
	for i, item := range sk.WorkflowTierB {
		out = append(out, glossarylint.ScanText(prefix+"WorkflowTierB["+strconv.Itoa(i)+"]", item.Body)...)
	}
	for i, item := range sk.ProhibitionsTierB {
		out = append(out, glossarylint.ScanText(prefix+"ProhibitionsTierB["+strconv.Itoa(i)+"]", item.Body)...)
	}
	for i, ts := range sk.ToolSuggestions {
		out = append(out, glossarylint.ScanText(prefix+"ToolSuggestions["+strconv.Itoa(i)+"]", ts)...)
	}
	return out
}
