package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stageToolCapabilityQuery describes a question whose primary
// authority lane is the system's stage -> agent -> skill ->
// ToolSuggestions -> buildToolSchemas capability surface, rather than
// helper subsets or exact-target absence logic.
type stageToolCapabilityQuery struct {
	Binding        types.StageBinding
	Tool           string
	StageMention   string
	ToolMention    string
	StageMentionAt int
	ToolMentionAt  int
}

var (
	capabilitySkillsOnce sync.Once
	capabilitySkills     *skill.Registry
)

func capabilitySkillRegistry() *skill.Registry {
	capabilitySkillsOnce.Do(func() {
		reg := skill.NewRegistry()
		skill.RegisterDefaults(reg)
		capabilitySkills = reg
	})
	return capabilitySkills
}

func capabilityKnownTools() []string {
	reg := capabilitySkillRegistry()
	if reg == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, name := range reg.List() {
		cfg, err := reg.Get(name)
		if err != nil || cfg == nil {
			continue
		}
		for _, toolName := range cfg.ToolSuggestions {
			toolName = strings.TrimSpace(toolName)
			if toolName == "" || seen[toolName] {
				continue
			}
			seen[toolName] = true
			out = append(out, toolName)
		}
	}
	return out
}

func capabilitySurfaceHintFromQuery(q *stageToolCapabilityQuery) *types.CapabilitySurfaceHint {
	if q == nil {
		return nil
	}
	return types.NormalizeCapabilitySurfaceHint(&types.CapabilitySurfaceHint{
		Binding:        q.Binding,
		Tool:           q.Tool,
		AuthorityFiles: capabilityAuthorityFiles(q),
	})
}

func capabilityQueryFromHint(hint *types.CapabilitySurfaceHint) *stageToolCapabilityQuery {
	hint = types.NormalizeCapabilitySurfaceHint(hint)
	if hint == nil {
		return nil
	}
	return &stageToolCapabilityQuery{
		Binding: hint.Binding,
		Tool:    hint.Tool,
	}
}

func detectStageToolCapabilityQuery(rm types.RequestModel) *stageToolCapabilityQuery {
	// User wording is intentionally not inspected here. Capability-surface
	// rewrites are hard pipeline decisions, so they may only consume the
	// analyzer's typed hint.
	return capabilityQueryFromHint(rm.AnalyzerHints.CapabilitySurface)
}

// detectStageToolCapabilityQueryAdvisory is a diagnostic-only raw-text helper.
// Do not wire it into RequestModel reconciliation, evidence planning, explorer
// routing, or finalizer prompting; those paths must consume the typed analyzer
// hint through detectStageToolCapabilityQuery.
func detectStageToolCapabilityQueryAdvisory(rm types.RequestModel) *stageToolCapabilityQuery {
	raw := strings.TrimSpace(rm.RawRequest)
	if raw == "" {
		return nil
	}
	toolMentions := capabilityToolMentions(raw)
	if len(toolMentions) != 1 {
		if types.IsScalarSourceLiteralLookup(rm) {
			return nil
		}
		return nil
	}
	stageMentions := capabilityStageMentions(raw)
	if len(stageMentions) == 0 {
		if types.IsScalarSourceLiteralLookup(rm) {
			return nil
		}
		return nil
	}
	tool := toolMentions[0]
	best := stageMentions[0]
	bestDistance := absInt(best.start - tool.start)
	for _, candidate := range stageMentions[1:] {
		distance := absInt(candidate.start - tool.start)
		if distance < bestDistance || (distance == bestDistance && candidate.start > best.start) {
			best = candidate
			bestDistance = distance
		}
	}
	if rm.AnswerSubject.Kind == types.SubjectReturnValue {
		return nil
	}
	if !rm.Predicates.IsScalarAnswer &&
		(strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") ||
			rm.PredicateAxis == types.AxisReturn ||
			rm.Intent == types.IntentReturnValue) {
		return nil
	}
	return &stageToolCapabilityQuery{
		Binding:        best.binding,
		Tool:           tool.value,
		StageMention:   best.alias,
		ToolMention:    tool.value,
		StageMentionAt: best.start,
		ToolMentionAt:  tool.start,
	}
}

func detectStageToolCapabilityQueryFromContext(ctx *types.AgentContext) *stageToolCapabilityQuery {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil {
		return detectStageToolCapabilityQuery(ctx.AnalysisIR.RequestModel)
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return detectStageToolCapabilityQuery(*rm)
		}
	}
	return nil
}

func capabilityAuthorityFiles(q *stageToolCapabilityQuery) []string {
	if q == nil {
		return nil
	}
	files := []string{
		"internal/orchestrator/topology.go",
		capabilitySkillSourceFile(q.Binding.Skill),
		"internal/agent/agent.go",
	}
	if q.Binding.Stage == types.StageAnalyze {
		files = append(files, "internal/agent/analyzer.go")
	}
	return dedupeSurfaceFiles(files)
}

func capabilityFocusedFiles(q *stageToolCapabilityQuery, required []string) []string {
	return dedupeSurfaceFiles(append(capabilityAuthorityFiles(q), required...))
}

func capabilitySkillSourceFile(skillName string) string {
	if strings.EqualFold(strings.TrimSpace(skillName), "analysis-skill") {
		return "internal/skill/analysis_contract.go"
	}
	return "internal/skill/defaults.go"
}

func capabilityAuthorityKeywords(q *stageToolCapabilityQuery) []string {
	if q == nil {
		return nil
	}
	return prependUniqueStrings(nil,
		q.Tool,
		string(q.Binding.Agent),
		string(q.Binding.Stage),
		q.Binding.Skill,
		"ToolSuggestions",
		"buildToolSchemas",
	)
}

func reconcileStageToolCapabilitySurface(rm types.RequestModel) (types.RequestModel, *stageToolCapabilityQuery, string) {
	q := detectStageToolCapabilityQuery(rm)
	if q == nil {
		return rm, nil, ""
	}
	rm.Intent = types.IntentExplain
	rm.PredicateAxis = types.AxisDefine
	rm.AnswerSubject = types.AnswerSubject{
		Kind: types.SubjectGeneric,
		EntityAxes: []string{
			fmt.Sprintf("%s -> %s", q.Binding.Stage, q.Binding.Skill),
			fmt.Sprintf("%s -> %s", q.Binding.Skill, q.Tool),
		},
		Confidence: 0.85,
	}
	rm.Scenario = types.ScenarioGeneric
	rm.AnalyzerHints.Kind = string(types.ReqMechanism)
	rm.AnalyzerHints.ExactTargets = nil
	rm.AnalyzerHints.ExactContextTerms = nil
	rm.AnalyzerHints.ExactContextRoles = nil
	rm.AnalyzerHints.CapabilitySurface = capabilitySurfaceHintFromQuery(q)
	rm.AnalyzerHints.Entities = prependUniqueStrings(
		rm.AnalyzerHints.Entities,
		capabilityAuthorityKeywords(q)...,
	)
	rm.AnalyzerHints.Keywords = prependUniqueStrings(
		rm.AnalyzerHints.Keywords,
		capabilityAuthorityKeywords(q)...,
	)
	return rm, q,
		"stage/tool capability query uses stage -> skill -> ToolSuggestions -> buildToolSchemas as the primary authority surface"
}

func renderCapabilityAuthoritySection(q *stageToolCapabilityQuery, header string) string {
	if q == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", header)
	fmt.Fprintf(&b, "- Treat `%s` on `%s` as a capability-surface question.\n", q.Tool, q.Binding.Stage)
	fmt.Fprintf(&b, "- Primary authority order: `%s` stage binding -> `%s` skill -> that skill's `ToolSuggestions` -> `buildToolSchemas` exposure rule.\n", q.Binding.Stage, q.Binding.Skill)
	b.WriteString("- Runtime helper subsets and validators can explain narrower checks, but they do not define the full tool-exposure surface by themselves.\n")
	b.WriteString("- If the user asked whether the stage can call the tool, answer that capability directly before drilling into helper functions or implementation detail.\n")
	b.WriteString("- If the user instead asked where or how the policy is implemented, keep the capability surface as the grounding frame and then cite the narrower validator/helper code as supporting detail.\n\n")
	return b.String()
}

type capabilityBindingMention struct {
	binding types.StageBinding
	alias   string
	start   int
}

type capabilitySurfaceMention struct {
	value string
	start int
}

func capabilityStageMentions(raw string) []capabilityBindingMention {
	var out []capabilityBindingMention
	for _, binding := range types.AllStageBindings() {
		for _, alias := range []string{string(binding.Stage), string(binding.Agent)} {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if idx := capabilityMentionIndex(raw, alias); idx >= 0 {
				out = append(out, capabilityBindingMention{
					binding: binding,
					alias:   alias,
					start:   idx,
				})
			}
		}
	}
	return out
}

func capabilityToolMentions(raw string) []capabilitySurfaceMention {
	var out []capabilitySurfaceMention
	for _, toolName := range capabilityKnownTools() {
		if idx := capabilityMentionIndex(raw, toolName); idx >= 0 {
			out = append(out, capabilitySurfaceMention{value: toolName, start: idx})
		}
	}
	return out
}

func capabilityMentionIndex(raw, candidate string) int {
	raw = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`))
	candidate = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(candidate), `\`, `/`))
	if raw == "" || candidate == "" {
		return -1
	}
	return strings.Index(raw, candidate)
}

func prependUniqueStrings(base []string, items ...string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(item string) {
		item = strings.TrimSpace(item)
		if item == "" {
			return
		}
		key := strings.ToLower(item)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, item)
	}
	for _, item := range items {
		add(item)
	}
	for _, item := range base {
		add(item)
	}
	return out
}

func dedupeSurfaceFiles(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, file := range in {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
