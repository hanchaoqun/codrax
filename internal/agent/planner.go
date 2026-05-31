package agent

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// plannerEvaluator drives the planner agent's single-emit ReAct loop
// for B0 write-mode plan stage. The agent's one structured
// contribution is a call to emit_change_plan, which installs the
// ChangePlan on BusContext.Mutable. ParseOutput reads the installed
// plan and returns a success StageOutput; when the LLM fails to emit
// the plan within the loop cap, a clean error surfaces upstream so
// the plan stage hook writes a diagnostic to TaskState.LastError instead of
// a silent no-op.
//
// Shape is closest to the finalizer's answerDocumentEvaluator: one
// tool is expected, success is signaled by a populated slot on
// Mutable, and the loop should terminate as soon as the slot fills.
// Unlike the finalizer, planner does not support correction retries
// in B0 — if the first emit_change_plan call is rejected by the
// tool's schema validator, ParseOutput reports the failure instead
// of nudging for a retry. B1 will add retry-hint semantics once the
// plan content can be richer (open-question #1).
//
// Red-line discipline (L3): plannerEvaluator MUST NOT reference the
// ground.* package. Plan content is a structured proposal, not
// citation-bearing evidence; grounding has no meaning here.
type plannerEvaluator struct {
	// mu is captured in BuildInitialInstruction so ShouldStop can
	// check whether the emit_change_plan tool has installed a plan
	// without needing ctx at every Evaluator entry. Mirror of
	// answerDocumentEvaluator.mu.
	mu *types.MutableState

	// Default iteration caps from AgentSettings — used as the floor
	// when no per-dispatch override has been computed by the
	// orchestrator (e.g. unit tests that bypass orchestrator wiring,
	// or runs where the analyzer's IR is unavailable).
	defaultSoftIterCap int
	defaultHardIterCap int

	// Per-dispatch caps captured in BuildInitialInstruction from
	// ctx.MaxIterOverride. Zero means "no override seen — fall back
	// to the defaults above." This mirrors the per-dispatch
	// MaxIterOverride mechanism the explorer already consumes from
	// AgentContext (see internal/agent/agent.go::maxIter resolution
	// and orchestrator.go's per-dispatch scaling block). The single
	// MaxIterOverride channel carries the orchestrator's complexity-
	// aware budget decision; the planner interprets it as its inner-
	// cap soft floor (recovery slack added on top).
	dispatchSoftIterCap int
	dispatchHardIterCap int

	// Pre-emit investigation cache (Module A). Same fingerprint
	// pattern the explorer uses (explorer.go:553-570): when the
	// planner re-dispatches within the same Run with identical
	// analyzer signal, skip the keyword-search rebuild and reuse
	// the cached result. The stored key is scoped by repo root,
	// sub-repo topology, and active-set pending list so a REPL turn in
	// another repository or multi-repo focus cannot reuse a stale rank.
	searchResult      *keywordSearchResult
	searchFingerprint string
}

// plannerInvestigationTopN bounds the file list rendered into the
// planner's initial prompt. Tuned to fit comfortably in the prompt
// without crowding out the user's actual instructions; the planner
// can always grep / read_file for more if these N aren't enough.
const plannerInvestigationTopN = 12

// plannerTestSurfaceMaxItems caps each list (manifests / test dirs /
// runner hints) in the rendered Test surface so a monorepo with 50
// pyproject.toml files doesn't bury the prompt.
const plannerTestSurfaceMaxItems = 8

// BuildInitialInstruction captures the Mutable pointer + per-dispatch
// iteration cap override for later ShouldStop inspection, and returns
// the PlanningHint supplement when the orchestrator's verify→plan
// retry loop (B2.3) has seeded one. The hint carries failure context
// from the previous ChangeReport so the planner knows what to avoid
// on this retry. When no hint is set (first attempt, or retry
// disabled), returns empty string and the skill's Workflow/Goal/
// Prohibitions drive the dispatch unchanged.
//
// PlannerSoftIterCapOverride consumption: the orchestrator's
// per-dispatch scaling block (orchestrator.go's StagePlan branch)
// writes the analyzer-complexity-scaled soft cap to
// ctx.PlannerSoftIterCapOverride. The hard cap is derived as soft +
// the agent-settings recovery slack (default hard - default soft).
// Zero override = no analyzer signal available → fall back to
// agent-settings defaults so unit tests and degraded paths still
// work. Distinct from MaxIterOverride (outer ReAct loop ceiling) so
// the outer cap remains a strict superset of the inner soft cap and
// the soft→hard recovery window can actually run.
func (e *plannerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	_ = sk
	if ctx != nil {
		e.mu = ctx.Mutable
		if ctx.PlannerSoftIterCapOverride > 0 {
			e.dispatchSoftIterCap = ctx.PlannerSoftIterCapOverride
			recoverySlack := e.defaultHardIterCap - e.defaultSoftIterCap
			if recoverySlack < 1 {
				recoverySlack = 1
			}
			e.dispatchHardIterCap = e.dispatchSoftIterCap + recoverySlack
		} else {
			e.dispatchSoftIterCap = 0
			e.dispatchHardIterCap = 0
		}
	}
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}

	// Compose sections in this order so the planner reads its
	// orientation BEFORE the situational directive: task framing
	// (Module 0) → top-ranked files (Module A) → test infrastructure
	// facts (Module B) → planning context / retry hint (existing).
	// All sections are pure data — no "you should do X" prescriptions.
	var sections []string
	if framing := e.buildTaskFramingSection(ctx); framing != "" {
		sections = append(sections, framing)
	}
	if seed := e.buildInvestigationSeed(ctx); seed != "" {
		sections = append(sections, seed)
	}
	if surface := e.buildTestSurfaceSection(ctx); surface != "" {
		sections = append(sections, surface)
	}
	if probes := e.buildProbeHistorySection(ctx); probes != "" {
		sections = append(sections, probes)
	}
	if history := e.buildIterationHistorySection(ctx); history != "" {
		sections = append(sections, history)
	}
	if pitfalls := e.buildActivePitfallsSection(ctx); pitfalls != "" {
		sections = append(sections, pitfalls)
	}
	if planning := e.buildPlanningContextSection(ctx); planning != "" {
		sections = append(sections, planning)
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}

// buildTaskFramingSection renders the WriteAnalysisIR's task-shape
// fields as a "## Task framing" prompt section so the planner sees
// the user's classified intent BEFORE it reasons about
// keyword-ranked files. Returns "" when no WriteAnalysisIR is
// available (commit 1's write_analyzer either degraded or wasn't
// dispatched — read mode never gets here because read mode never
// invokes the planner). Pure data: kind, scope, summary, risk
// flags, scope anchors, constraints, expected outcomes — no
// "you should do X" framing.
func (e *plannerEvaluator) buildTaskFramingSection(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Task framing\n\n")
	if ir.Request.Task.Summary != "" {
		fmt.Fprintf(&b, "- summary: %s\n", ir.Request.Task.Summary)
	}
	if ir.Request.Task.Kind != "" {
		fmt.Fprintf(&b, "- kind: %s\n", ir.Request.Task.Kind)
	}
	if ir.Request.Task.Scope != "" {
		fmt.Fprintf(&b, "- scope: %s\n", ir.Request.Task.Scope)
	}
	risks := []string{}
	if ir.Request.Risk.AffectsPublicAPI {
		risks = append(risks, "affects-public-api")
	}
	if ir.Request.Risk.ChangesPersistence {
		risks = append(risks, "changes-persistence")
	}
	if ir.Request.Risk.ChangesBuildSystem {
		risks = append(risks, "changes-build-system")
	}
	if len(risks) > 0 || ir.Request.Risk.Overall != "" {
		fmt.Fprintf(&b, "- risk: %s", ir.Request.Risk.Overall)
		if len(risks) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(risks, ", "))
		}
		b.WriteString("\n")
	}
	if len(ir.Request.ScopeAnchors) > 0 {
		fmt.Fprintf(&b, "- scope anchors: %s\n", strings.Join(ir.Request.ScopeAnchors, ", "))
	}
	if len(ir.Request.Constraints) > 0 {
		b.WriteString("- constraints:\n")
		for _, c := range ir.Request.Constraints {
			line := c.Kind
			if c.Target != "" {
				line += " on " + c.Target
			}
			if c.Note != "" {
				line += " — " + c.Note
			}
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	if len(ir.Request.ExpectedOutcomes) > 0 {
		b.WriteString("- expected outcomes:\n")
		for _, o := range ir.Request.ExpectedOutcomes {
			fmt.Fprintf(&b, "  - %s\n", o)
		}
	}
	return b.String()
}

// buildInvestigationSeed implements Module A's pre-emit investigation
// orientation. Runs the same keywordSearchWithOptions API the
// read-mode explorer uses (explorer.go:553-570) so the planner sees
// the same structural-rank + IDF + entity-boost ranking. Caches the
// graph on Mutable.SearchGraph() if not already present so the
// emit_evidence grounder + later tool calls can re-rank without
// rebuilding. Returns a "## Likely-relevant files" section listing
// the top N paths — pure data, no prescription.
//
// Returns "" when the analyzer signal is absent (e.g. no keywords)
// or the repo has no rankable files. The planner falls through to
// its skill workflow without orientation in that case.
func (e *plannerEvaluator) buildInvestigationSeed(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	keywords := irKeywords(ctx)
	if len(keywords) == 0 || ctx.RepoRoot == "" {
		return ""
	}

	// Re-rank fingerprint mirrors explorer's pattern. Includes the
	// signal axes that change the ranking; matches what's passed to
	// keywordSearchWithOptions below.
	entities := irEntities(ctx)
	mentionedEntities := irMentionedEntities(ctx)
	primaryEntities := irPrimaryEntities(ctx)
	domainHints := irDomainHints(ctx)
	maxFiles := MaxFilesForComplexity(irComplexity(ctx))
	exactContract := irExactResolutionContract(ctx)
	sourceScope := irSourceScopeProfile(ctx)
	var exactTargets []string
	exactPolicy := ""
	if exactContract != nil {
		exactTargets = exactContract.Targets
		exactPolicy = string(exactContract.RelatedContextPolicy)
	}
	suppressExactAnchors := observationOnlyRuntimeArtifactForExplorer(ctx)
	fp := keywordSearchScopedFingerprint(ctx, keywordSearchFingerprint(keywords, entities, mentionedEntities, primaryEntities, domainHints, exactTargets, exactPolicy, maxFiles, suppressExactAnchors, sourceScope.Fingerprint()))

	var sr *keywordSearchResult
	switch {
	case e.searchResult != nil && e.searchFingerprint == fp:
		sr = e.searchResult
	default:
		sr = keywordSearchWithOptions(keywords, ctx.RepoRoot, keywordSearchOptions{
			Entities:                   entities,
			MentionedEntities:          mentionedEntities,
			PrimaryEntities:            primaryEntities,
			DomainHints:                domainHints,
			MaxFiles:                   maxFiles,
			ExactResolution:            exactContract,
			SourceScope:                sourceScope,
			SuppressExactEntityAnchors: suppressExactAnchors,
			MultiGraph:                 ctx.MultiGraph,
			SearchGraph:                keywordSearchGraphHandle(ctx, ctx.RepoRoot),
			PendingSubRepos:            ctx.PendingSubRepos,
		})
		e.searchResult = sr
		e.searchFingerprint = fp
		if sr != nil && sr.Graph != nil && ctx.Mutable != nil {
			// Publish so later tool calls in this dispatch (and
			// downstream Coder if SearchGraph stays alive across
			// stages) can read without rebuilding.
			ctx.Mutable.SetSearchGraph(sr.Graph)
		}
	}
	if sr == nil || len(sr.Files) == 0 {
		return ""
	}

	limit := plannerInvestigationTopN
	if limit > len(sr.Files) {
		limit = len(sr.Files)
	}
	var b strings.Builder
	b.WriteString("## Likely-relevant files\n\n")
	b.WriteString("Top files ranked by structural importance (repo_map) + keyword match (grep IDF) + entity boost. Use read_file / grep on these before deciding what to change. Use line_offset/limit on read_file to page through large files; line_offset is zero-based and line-based, and the response will tell you the line range.\n\n")
	for i := 0; i < limit; i++ {
		f := sr.Files[i]
		fmt.Fprintf(&b, "  - %s\n", f.Path)
	}
	if len(sr.Files) > limit {
		fmt.Fprintf(&b, "  - … (+%d more — call repo_map for the full list)\n", len(sr.Files)-limit)
	}
	return b.String()
}

// buildTestSurfaceSection implements Module B. Derives the
// "what testing capability does this repo have" answer from facts
// already in the cached repomap Graph (no new disk I/O). Renders a
// neutral "## Test surface" section that ENUMERATES manifests / test
// directories / likely runners. The planner reads these and decides
// whether its plan needs to update a manifest, add a test file, etc.
//
// Returns "" when no graph is available (analyzer signal too weak,
// or first dispatch in a non-explorer-led plan-only path) so the
// planner just falls through to its workflow without this hint.
func (e *plannerEvaluator) buildTestSurfaceSection(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	graph := plannerSearchGraph(ctx)
	if graph == nil {
		return ""
	}
	profile := extractTestProfile(graph)
	if profile.empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Test surface\n\n")
	b.WriteString("Read-only facts about this repo's testing layout (derived from repo_map, no disk probe).\n\n")
	if len(profile.Manifests) > 0 {
		b.WriteString("Manifests detected: ")
		b.WriteString(strings.Join(profile.Manifests, ", "))
		b.WriteString("\n")
	}
	if len(profile.TestDirs) > 0 {
		b.WriteString("Test directories observed: ")
		b.WriteString(strings.Join(profile.TestDirs, ", "))
		b.WriteString("\n")
	}
	if len(profile.RunnerHints) > 0 {
		b.WriteString("Likely test runners: ")
		b.WriteString(strings.Join(profile.RunnerHints, ", "))
		b.WriteString("\n")
	}
	return b.String()
}

// buildProbeHistorySection renders Module E's plan-stage probe
// results into the planner's initial prompt. Each probe is one
// run_tests(dry_run=true) call the planner made earlier in this
// dispatch (or in a prior dispatch within the same Run, before a
// retry). The section enumerates probes in order with verbatim
// pass/fail counts + failure summary so the planner can compare
// "what the existing suite reports today" against the changes it's
// about to propose.
//
// Empty when no probe has fired this Run.
func (e *plannerEvaluator) buildProbeHistorySection(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	probes := ctx.Mutable.PlanStageProbeReports()
	if len(probes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Probe results\n\n")
	b.WriteString("Each entry is a run_tests(dry_run=true) probe you fired earlier in this Run. The probes ran the EXISTING test suite (before any plan was applied) so you can see what passes / fails today.\n\n")
	for i, r := range probes {
		if r == nil {
			continue
		}
		passed := 0
		failed := 0
		for _, tr := range r.TestResults {
			if tr.Passed {
				passed++
			} else {
				failed++
			}
		}
		fmt.Fprintf(&b, "### Probe %d\n\n", i+1)
		fmt.Fprintf(&b, "Tests: %d passed, %d failed.\n\n", passed, failed)
		if r.FailureSummary != "" {
			b.WriteString("Failure summary (verbatim):\n")
			b.WriteString(r.FailureSummary)
			b.WriteString("\n")
			if r.FailureSummaryBlobRef != "" {
				fmt.Fprintf(&b, "(Full stderr at %s — call read_file with line_offset/limit to page through.)\n", r.FailureSummaryBlobRef)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildIterationHistorySection renders Module C's iteration ledger
// into the planner's initial prompt. Empty on the first plan attempt
// (ledger is empty until clearForReplan appends after attempt 1's
// verify failure). The section gives the planner the FULL history
// of prior attempts in this Run — every PlanSummary verbatim, every
// FailureSummary verbatim, every changed-files list. The planner
// reads them and decides; the system attaches no commentary.
func (e *plannerEvaluator) buildIterationHistorySection(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	return types.RenderIterationLedger(ctx.Mutable.IterationLedger())
}

// buildActivePitfallsSection renders the stage-3 Failure
// Taxonomy entries the orchestrator deemed relevant to this
// plan dispatch. Each pitfall shows up as a single bullet
// (name + 1-line description + trigger) so the planner sees
// the WHAT + WHY without losing context budget. Empty when
// no pitfalls apply (typical for first plan in a fresh repo,
// for tasks whose kind doesn't match any pattern, or when
// the failure-taxonomy feature is disabled).
//
// The framing is descriptive (these patterns happened
// before), not prescriptive (you must do X). The LLM reads
// the pattern + decides for itself whether THIS plan
// triggers it.
func (e *plannerEvaluator) buildActivePitfallsSection(ctx *types.AgentContext) string {
	if ctx == nil || len(ctx.ActivePitfalls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Known active pitfalls in this repo\n\n")
	b.WriteString("Past plans in this repo failed due to the patterns below. Each one is annotated with what triggers it. When emitting your plan, check whether THIS task could trip any of them; if so, structure the plan to avoid the trigger. The planner is the decider — these are observations, not instructions.\n\n")
	for _, p := range ctx.ActivePitfalls {
		fmt.Fprintf(&b, "- **%s** — %s\n", strings.TrimSpace(p.Name), strings.TrimSpace(p.Description))
		if t := strings.TrimSpace(p.Trigger); t != "" {
			fmt.Fprintf(&b, "  - Trigger: %s\n", t)
		}
		if c := strings.TrimSpace(p.Consequence); c != "" {
			fmt.Fprintf(&b, "  - Typical consequence: %s\n", c)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildPlanningContextSection renders the existing PlanningHint —
// verify→plan retry hint, scaffold directive, or stream-stall
// recovery directive. Unchanged behavior; just refactored out of
// BuildInitialInstruction so the three-section composition stays
// readable.
func (e *plannerEvaluator) buildPlanningContextSection(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	hint := ctx.Mutable.PlanningHint()
	if hint == "" {
		return ""
	}
	// Consume-once: clear the hint so a subsequent sub-dispatch
	// within the same retry iteration does not double-apply it.
	ctx.Mutable.ResetPlanningHint()
	// The header is intentionally generic ("Planning context"). The
	// hint body itself carries the situation tag:
	//   - verify→plan retry — body opens with "Previous attempt N
	//     verify failed ..." (buildRetryHint).
	//   - empty-repo scaffold — body opens with "SCAFFOLD DIRECTIVE
	//     —" (planPreHook proactive seed).
	//   - streaming-stall recovery — body opens with "RETRY
	//     DIRECTIVE —" (write_scheduler transient retry).
	// Keeping the wrapper neutral avoids hardcoding "verify failed"
	// under branches where it isn't true.
	return "## Planning context\n\n" + hint +
		"\n\nFollow the directive in the planning context above. It was added because the system observed a condition that affects how this dispatch must emit."
}

// plannerSearchGraph returns the repomap.Graph cached on Mutable
// (typically by the explorer's BuildInitialInstruction or our own
// buildInvestigationSeed). Returns nil when nothing was cached or
// the cached value isn't a recognised graph type — caller handles
// nil gracefully so the planner can dispatch without the hint.
func plannerSearchGraph(ctx *types.AgentContext) *repomaptypes.Graph {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	g, ok := ctx.Mutable.SearchGraph().(*repomaptypes.Graph)
	if !ok {
		return nil
	}
	return g
}

// repoTestProfile is the structured "what testing capability does
// this repo have" data the planner sees in the Test surface section.
// All fields are language-agnostic enumerations of facts derived
// from a repomap Graph — no recommendations attached.
type repoTestProfile struct {
	Manifests   []string
	TestDirs    []string
	RunnerHints []string
}

func (p *repoTestProfile) empty() bool {
	return len(p.Manifests) == 0 && len(p.TestDirs) == 0 && len(p.RunnerHints) == 0
}

// plannerManifestRunner maps recognised manifest filenames to the
// run_tests runner tag the verifier would pick. The mapping mirrors
// internal/tool/run_tests.go's detectRunnerPlans so the planner sees
// the same categorisation the verify stage will apply downstream.
// Single source of truth would be ideal but circular import would
// result; this tiny map stays in sync with run_tests.go's table.
var plannerManifestRunner = map[string]string{
	"go.mod":              "go (go test)",
	"package.json":        "node (npm test)",
	"pyproject.toml":      "python (pytest)",
	"pytest.ini":          "python (pytest)",
	"setup.py":            "python (pytest)",
	"Cargo.toml":          "rust (cargo test)",
	"pom.xml":             "java (mvn test)",
	"build.gradle":        "java (gradlew test)",
	"build.gradle.kts":    "java (gradlew test)",
	"Gemfile":             "ruby (rspec)",
	"CMakeLists.txt":      "cmake (ctest)",
	"meson.build":         "meson (meson test)",
	"Makefile":            "make (make check)",
	"oh-package.json5":    "hvigor (hvigorw test)",
	"build-profile.json5": "hvigor (hvigorw test)",
	"hvigorfile.ts":       "hvigor (hvigorw test)",
	"cjpm.toml":           "cjpm (cjpm test)",
	"Package.swift":       "swift (swift test)",
}

// extractTestProfile walks the repomap Graph (zero disk I/O — the
// graph already carries every file's RelPath from the scan) and
// emits a fact-only profile of the repo's test surface. Three
// categories:
//
//  1. Manifests: project-root manifest filenames that imply a runner
//     (one entry per distinct manifest file path).
//  2. TestDirs: directories named tests/test/spec/__tests__ or whose
//     path contains a test-segment hint.
//  3. RunnerHints: human-readable runner labels derived from the
//     manifest set, deduplicated.
//
// No prescription — the planner reads these and decides whether its
// plan needs to update a manifest or add a test file. Caps each list
// at plannerTestSurfaceMaxItems so a monorepo doesn't drown the
// prompt.
func extractTestProfile(g *repomaptypes.Graph) repoTestProfile {
	var profile repoTestProfile
	if g == nil {
		return profile
	}
	manifestSet := map[string]bool{}
	testDirSet := map[string]bool{}
	runnerSet := map[string]bool{}
	for _, fi := range g.Files {
		if fi == nil || fi.RelPath == "" {
			continue
		}
		base := filepath.Base(fi.RelPath)
		if hint, ok := plannerManifestRunner[base]; ok {
			manifestSet[fi.RelPath] = true
			runnerSet[hint] = true
		}
		// Test directory detection: walk path segments.
		dir := filepath.Dir(fi.RelPath)
		for _, seg := range strings.Split(filepath.ToSlash(dir), "/") {
			switch seg {
			case "tests", "test", "spec", "__tests__", "testdata":
				testDirSet[dir] = true
			}
		}
	}
	profile.Manifests = sortedKeysCapped(manifestSet, plannerTestSurfaceMaxItems)
	profile.TestDirs = sortedKeysCapped(testDirSet, plannerTestSurfaceMaxItems)
	profile.RunnerHints = sortedKeysCapped(runnerSet, plannerTestSurfaceMaxItems)
	return profile
}

// sortedKeysCapped is a small helper used by extractTestProfile so
// the rendered lists are deterministic across runs (sort) and
// bounded (cap). Cap of 0 returns the full list; positive cap
// truncates to the first N after sorting.
func sortedKeysCapped(m map[string]bool, cap int) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	if cap > 0 && len(out) > cap {
		out = out[:cap]
	}
	return out
}

// ShouldStop terminates the ReAct loop as soon as a ChangePlan has
// been installed on Mutable (any of the three emit paths' happy
// path) or when the loop has burned through its iteration cap.
//
// Three emit paths land a finished plan on Mutable.ChangePlan:
//
//  1. emit_change_plan single-shot — LLM emits the whole plan in one
//     tool call. Smallest LLM round count when the plan fits.
//
//  2. emit_plan_skeleton + emit_plan_change multi-round — skeleton
//     first (metadata only, payload-bounded), then per-file body
//     emits. The LAST emit_plan_change call promotes
//     PartialChangePlan → ChangePlan once every body slot is filled.
//
// All three names belong to the recovery list because every one of
// them is a structured emit whose function.arguments string can be
// truncated by a streaming gateway; the LLM's clean retry of any of
// them at the soft cap must be allowed to execute before the hard
// cap forces termination. The cap is the per-dispatch value captured
// in BuildInitialInstruction; when no per-dispatch override is
// present (zero value), the agent-settings default takes over.
func (e *plannerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	if e.mu != nil && e.mu.ChangePlan() != nil {
		return true
	}
	soft, hard := e.dispatchSoftIterCap, e.dispatchHardIterCap
	if soft <= 0 || hard <= soft {
		soft, hard = e.defaultSoftIterCap, e.defaultHardIterCap
	}
	// Two-stage cap: soft cap is the normal stop; the soft→hard window
	// spares one extra iteration whenever the LLM is actively calling
	// any of the planner's structured-emit tools. Streaming gateways
	// occasionally truncate the function.arguments of a large emit
	// payload, the tool rejects with `unexpected EOF`, and the LLM's
	// next iteration is a clean retry. ShouldStop runs BEFORE tool
	// execution, so a flat soft-cap break would discard that recovery
	// before it ever runs.
	return iterationCapShouldStop(resp, iteration,
		soft, hard,
		emitChangePlanToolName,
		emitPlanSkeletonToolName,
		emitPlanChangeToolName)
}

const (
	emitChangePlanToolName   = "emit_change_plan"
	emitPlanSkeletonToolName = "emit_plan_skeleton"
	emitPlanChangeToolName   = "emit_plan_change"
)

// ParseOutput reads the installed ChangePlan, or reports a clean
// failure if emit_change_plan was never called successfully. The
// returned StageOutput carries MissingPiece (required by
// orchestrator's decision path) and an Error field when the
// emission failed.
func (e *plannerEvaluator) ParseOutput(
	ctx *types.AgentContext,
	messages []llm.Message,
	toolResults []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	_ = messages
	out := &StageOutput{}

	var plan *types.ChangePlan
	if ctx != nil && ctx.Mutable != nil {
		plan = ctx.Mutable.ChangePlan()
	}

	if plan == nil {
		// Three failure shapes to distinguish so the caller's retry
		// hint and the user-visible LastError both carry actionable
		// information instead of "did not emit":
		//
		//  (a) Partial plan installed but file contents still missing
		//      — the outline landed, the model was midway through
		//      per-file content emits when the loop bound hit. List
		//      the missing paths so the next round knows what to do.
		//  (b) Most recent structured emit was rejected — surface the
		//      validator's reason verbatim.
		//  (c) Nothing emitted at all — generic message.
		//
		// User-facing wording deliberately avoids tool names
		// (emit_change_plan / emit_plan_skeleton / emit_plan_change)
		// and structural slot names (PartialChangePlan / ChangePlan)
		// because this string ends up on TaskState.LastError and is
		// surfaced verbatim to the user; internal tool names confuse
		// without adding actionable signal. Operator log lines below
		// keep enough breadcrumbs for ops debugging.
		var partial *types.ChangePlan
		if ctx != nil && ctx.Mutable != nil {
			partial = ctx.Mutable.PartialChangePlan()
		}
		var reason string
		if partial != nil {
			missing := plannerMissingBodies(partial)
			total := plannerNonDeleteSlotCount(partial)
			if len(missing) == 0 {
				// Outline + every body present yet ChangePlan slot
				// stayed empty. Internal mismatch — log details, give
				// the user a short non-jargon line.
				logging.Warning("[planner] partial plan id=%s has every body filled but promotion to ChangePlan never fired (internal state mismatch)", partial.ID)
				reason = "the change plan was outlined and every file body provided, but the plan was never finalized (please retry)"
			} else {
				preview := missing
				if len(preview) > 5 {
					preview = preview[:5]
				}
				logging.Warning("[planner] partial plan id=%s has %d/%d file bodies missing: %v", partial.ID, len(missing), total, missing)
				reason = fmt.Sprintf(
					"the change plan listed %d files but %d still need their contents written. Pending files: %s",
					total, len(missing),
					strings.Join(preview, ", "))
				if len(missing) > len(preview) {
					reason += fmt.Sprintf(" (and %d more)", len(missing)-len(preview))
				}
			}
		} else {
			// Default message stays neutral; operator log captures
			// the failed-tool diagnostics for ops investigation.
			reason = "no change plan was produced this round"
			for i := len(toolResults) - 1; i >= 0; i-- {
				tr := toolResults[i]
				if !tr.Success && (tr.ToolName == emitChangePlanToolName ||
					tr.ToolName == emitPlanSkeletonToolName ||
					tr.ToolName == emitPlanChangeToolName) {
					logging.Warning("[planner] structured emit rejected: tool=%s summary=%s", tr.ToolName, tr.Summary)
					reason = "the proposed change plan was rejected: " + tr.Summary
					break
				}
			}
		}
		logging.Warning("[planner] no plan installed: %s", reason)
		out.MissingPiece = types.MissingFacts
		out.Error = reason
		return out, fmt.Errorf("%s", reason)
	}

	logging.Info("[planner] plan installed: id=%s changes=%d target_paths=%d",
		plan.ID, len(plan.Changes), len(plan.TargetPaths))
	out.MissingPiece = types.MissingNone
	return out, nil
}

// DetermineMissingPiece returns MissingNone on success (plan emitted)
// and MissingFacts when the emit failed — matching ParseOutput's
// decision so upstream code can observe a consistent signal.
func (e *plannerEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// plannerMissingBodies returns the repo-relative paths of partial
// plan changes that still need their content written (kind in
// {create, modify} with empty NewContent, or kind=patch with empty
// Patch). delete kinds are body-trivial and excluded.
//
// Used by ParseOutput to surface the precise gap when the loop
// terminates with a partial plan installed but no completed plan.
// Pure read of partial.Changes — no mutation.
func plannerMissingBodies(partial *types.ChangePlan) []string {
	if partial == nil {
		return nil
	}
	var missing []string
	for _, c := range partial.Changes {
		switch strings.TrimSpace(c.Kind) {
		case "create", "modify":
			if strings.TrimSpace(c.NewContent) == "" {
				missing = append(missing, c.Path)
			}
		case "patch":
			if strings.TrimSpace(c.Patch) == "" {
				missing = append(missing, c.Path)
			}
		}
	}
	return missing
}

// plannerNonDeleteSlotCount returns the number of changes that
// require a body fill (kinds: create / modify / patch). Used as the
// denominator in ParseOutput's "X/Y files still need contents"
// message so the user sees a concrete progress fraction.
func plannerNonDeleteSlotCount(partial *types.ChangePlan) int {
	if partial == nil {
		return 0
	}
	n := 0
	for _, c := range partial.Changes {
		if strings.TrimSpace(c.Kind) != "delete" {
			n++
		}
	}
	return n
}

// NewPlannerAgent constructs the B0 plan-stage agent. Wiring
// mirrors NewFinalizerAgent: a single Evaluator impl on top of
// BaseAgent's ReAct loop. The change-plan-skill provides the
// prompt and ToolSuggestions (read_file / grep / list_files /
// repo_map / exec_command / emit_change_plan).
func NewPlannerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentPlanner, deps, &plannerEvaluator{
		defaultSoftIterCap: deps.AgentSettings.PlannerSoftIterCap,
		defaultHardIterCap: deps.AgentSettings.PlannerHardIterCap,
	})
}
