package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- SubAgentValidator ---

// SubAgentValidator validates a SubAgentProposal from the LLM and converts it
// to a list of SubAgentRequests. Rejects invalid proposals so the orchestrator
// can fall back to the original agent output.
type SubAgentValidator struct {
	maxSubTasks int
	registry    *SubAgentRegistry
}

// NewSubAgentValidator creates a new validator.
func NewSubAgentValidator(registry *SubAgentRegistry) *SubAgentValidator {
	return &SubAgentValidator{
		maxSubTasks: 8,
		registry:    registry,
	}
}

// Validate checks the proposal and produces SubAgentRequests.
// All requests share the same ReadView (bus) for shared read access.
func (v *SubAgentValidator) Validate(bus *types.BusContext, proposal *types.SubAgentProposal) ([]*types.SubAgentRequest, error) {
	if len(proposal.SubTasks) == 0 {
		return nil, fmt.Errorf("proposal has no sub_tasks")
	}
	if len(proposal.SubTasks) > v.maxSubTasks {
		return nil, fmt.Errorf("too many sub_tasks: %d > %d", len(proposal.SubTasks), v.maxSubTasks)
	}

	seen := make(map[string]bool)
	requests := make([]*types.SubAgentRequest, 0, len(proposal.SubTasks))

	for i, st := range proposal.SubTasks {
		// ID uniqueness
		if st.ID == "" {
			return nil, fmt.Errorf("sub_task[%d]: id is required", i)
		}
		if seen[st.ID] {
			return nil, fmt.Errorf("sub_task[%d]: duplicate id %q", i, st.ID)
		}
		seen[st.ID] = true

		// SubAgent must be registered
		if _, err := v.registry.Get(st.SubAgent); err != nil {
			return nil, fmt.Errorf("sub_task[%d]: %w", i, err)
		}

		// Scope is required to prevent unbounded operations. Normalize
		// it before execution so later tools see stable repo-relative
		// paths and unsafe parent/absolute escapes never reach file
		// scanners.
		scope, err := normalizeSubAgentScopes(bus, st.Scope)
		if err != nil {
			return nil, fmt.Errorf("sub_task[%d]: %w", i, err)
		}
		if len(scope) == 0 {
			return nil, fmt.Errorf("sub_task[%d]: scope is required", i)
		}

		// Objective is required
		if st.Objective == "" {
			return nil, fmt.Errorf("sub_task[%d]: objective is required", i)
		}

		requests = append(requests, &types.SubAgentRequest{
			ID:          fmt.Sprintf("%s-%s", bus.TraceID, st.ID),
			SubAgent:    st.SubAgent,
			Objective:   st.Objective,
			Scope:       scope,
			Constraints: st.Constraints,
			InputData:   st.InputData,
			ReadView:    bus, // shared read — same pointer for all
		})
	}

	return requests, nil
}

func normalizeSubAgentScopes(bus *types.BusContext, rawScopes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawScopes))
	out := make([]string, 0, len(rawScopes))
	for _, raw := range rawScopes {
		normalized, err := normalizeSubAgentScope(bus, raw)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	logSubAgentScopeOverlap(out)
	return out, nil
}

func normalizeSubAgentScope(bus *types.BusContext, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if subAgentScopeHasParentTraversal(trimmed) {
		return "", fmt.Errorf("scope %q escapes the current repository; use a repo-relative path inside the active scope", raw)
	}

	if bus != nil {
		if gater, ok := bus.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetPath(bus, "propose_sub_agents", trimmed, nil)
			if !gate.Allowed {
				return "", fmt.Errorf("scope %q is outside the active repository boundary: %s", raw, gate.RefusalProse)
			}
			if strings.TrimSpace(gate.ResolvedPath) != "" {
				trimmed = strings.TrimSpace(gate.ResolvedPath)
				if subAgentScopeHasParentTraversal(trimmed) {
					return "", fmt.Errorf("scope %q resolves outside the current repository; use a repo-relative path inside the active scope", raw)
				}
			}
		}
	}

	pathForClean := strings.ReplaceAll(trimmed, "\\", string(filepath.Separator))
	cleaned := filepath.Clean(pathForClean)
	if cleaned == "" {
		return "", nil
	}
	if filepath.IsAbs(cleaned) {
		if bus == nil || strings.TrimSpace(bus.RepoRoot) == "" {
			return "", fmt.Errorf("scope %q is absolute; use a repo-relative path inside the active scope", raw)
		}
		rel, err := filepath.Rel(bus.RepoRoot, cleaned)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("scope %q escapes the current repository; use a repo-relative path inside the active scope", raw)
		}
		cleaned = rel
	}
	return filepath.ToSlash(cleaned), nil
}

func subAgentScopeHasParentTraversal(raw string) bool {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func logSubAgentScopeOverlap(scopes []string) {
	for i := 0; i < len(scopes); i++ {
		for j := i + 1; j < len(scopes); j++ {
			if subAgentScopesOverlap(scopes[i], scopes[j]) {
				logging.Warning("[subagent-runtime] overlapping sub-task scopes kept advisory: %q and %q", scopes[i], scopes[j])
			}
		}
	}
}

func subAgentScopesOverlap(a, b string) bool {
	a = strings.Trim(filepath.ToSlash(filepath.Clean(a)), "/")
	b = strings.Trim(filepath.ToSlash(filepath.Clean(b)), "/")
	if a == "" || b == "" || a == "." || b == "." {
		return false
	}
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// --- SubAgentRuntime ---

// SubAgentRuntime validates, executes, and reduces SubAgent proposals.
// Orchestrator calls Run() with a proposal; Runtime handles the rest internally.
type SubAgentRuntime struct {
	registry       *SubAgentRegistry
	validator      *SubAgentValidator
	reducer        *SubAgentReducer
	maxParallelism int
	observer       reasoninggraph.Observer
}

// NewSubAgentRuntime creates a new runtime with built-in validator and reducer.
func NewSubAgentRuntime(registry *SubAgentRegistry) *SubAgentRuntime {
	return &SubAgentRuntime{
		registry:       registry,
		validator:      NewSubAgentValidator(registry),
		reducer:        &SubAgentReducer{},
		maxParallelism: types.DefaultPipelineMaxParallelism,
	}
}

func (r *SubAgentRuntime) SetMaxParallelism(v int) {
	if r == nil {
		return
	}
	if v < 0 {
		v = types.DefaultPipelineMaxParallelism
	}
	if v > types.MaxPipelineParallelismCeil {
		v = types.MaxPipelineParallelismCeil
	}
	r.maxParallelism = v
}

func (r *SubAgentRuntime) SetReasoningObserver(observer reasoninggraph.Observer) {
	if r == nil {
		return
	}
	r.observer = observer
}

// Run is the single entry point for the Orchestrator.
// It validates the proposal, executes SubAgents in parallel, and reduces results.
func (r *SubAgentRuntime) Run(bus *types.BusContext, proposal *types.SubAgentProposal) (*StageOutput, error) {
	// 1. Validate
	requests, err := r.validator.Validate(bus, proposal)
	if err != nil {
		return nil, err
	}
	scopeCount, overlapPairs := subAgentRequestScopeStats(requests)
	logging.Info(
		"[subagent-runtime] proposal branches=%d parallelism=%d scopes=%d scope_overlap_pairs=%d",
		len(requests),
		subAgentEffectiveParallelism(r.maxParallelism, len(requests)),
		scopeCount,
		overlapPairs,
	)

	// 2. Execute parallel
	results, execErr := r.execute(requests)
	r.observeSubAgentEvents(requests, results)

	// 3. Reduce
	merged := r.reducer.Reduce(results)
	stats := collectSubAgentRuntimeStats(results)
	logging.Info(
		"[subagent-runtime] reduced branches=%d ok=%d errors=%d tools=%d success=%d fail=%d duplicate_tool_results=%d duplicate_read_file=%d duplicate_grep=%d repo_map=%d facts=%d evidence=%d flow=%d advisory_notes=%d outputs=%d",
		stats.Branches,
		stats.OK,
		stats.Errors,
		stats.Tools,
		stats.SuccessfulTools,
		stats.FailedTools,
		stats.DuplicateToolResults,
		stats.DuplicateReadFile,
		stats.DuplicateGrep,
		stats.RepoMapTools,
		stats.Facts,
		stats.Evidence,
		stats.Flow,
		stats.AdvisoryNotes,
		stats.Outputs,
	)

	return merged, execErr
}

func (r *SubAgentRuntime) observeSubAgentEvents(requests []*types.SubAgentRequest, results []*types.SubAgentResult) {
	if r == nil || r.observer == nil {
		return
	}
	for i, req := range requests {
		var result *types.SubAgentResult
		if i >= 0 && i < len(results) {
			result = results[i]
		}
		for _, event := range reasoninggraph.EventsFromSubAgentRequestResult(req, result) {
			r.observer.ObserveReasoningEvent(event)
		}
	}
}

// execute runs all requests in parallel and returns results in the same order.
func (r *SubAgentRuntime) execute(requests []*types.SubAgentRequest) ([]*types.SubAgentResult, error) {
	results := make([]*types.SubAgentResult, len(requests))
	errs := make([]error, len(requests))
	workers := subAgentEffectiveParallelism(r.maxParallelism, len(requests))
	if workers <= 1 {
		for i, req := range requests {
			results[i], errs[i] = r.executeOne(i, req)
		}
		return results, combineErrors(errs)
	}

	var wg sync.WaitGroup
	jobs := make(chan int)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i], errs[i] = r.executeOne(i, requests[i])
			}
		}()
	}
	for i := range requests {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results, combineErrors(errs)
}

func (r *SubAgentRuntime) executeOne(i int, req *types.SubAgentRequest) (*types.SubAgentResult, error) {
	sub, err := r.registry.Get(req.SubAgent)
	if err != nil {
		return nil, err
	}
	logging.Info("[subagent-runtime] start[%d] %s: %s", i, req.SubAgent, req.Objective)
	result, err := sub.Run(req)
	logging.Info("[subagent-runtime] done[%d] %s: err=%v", i, req.SubAgent, err)
	return result, err
}

func subAgentEffectiveParallelism(configured, candidateCount int) int {
	if candidateCount <= 0 {
		return 0
	}
	if configured < 0 {
		configured = types.DefaultPipelineMaxParallelism
	}
	if configured == 0 || configured > candidateCount {
		return candidateCount
	}
	return configured
}

func combineErrors(errs []error) error {
	var msgs []string
	for _, e := range errs {
		if e != nil {
			msgs = append(msgs, e.Error())
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("subagent errors: %v", msgs)
}

func subAgentRequestScopeStats(requests []*types.SubAgentRequest) (scopeCount, overlapPairs int) {
	for _, req := range requests {
		if req == nil {
			continue
		}
		scopeCount += len(req.Scope)
		for i := 0; i < len(req.Scope); i++ {
			for j := i + 1; j < len(req.Scope); j++ {
				if subAgentScopesOverlap(req.Scope[i], req.Scope[j]) {
					overlapPairs++
				}
			}
		}
	}
	return scopeCount, overlapPairs
}

type subAgentRuntimeStats struct {
	Branches             int
	OK                   int
	Errors               int
	Tools                int
	SuccessfulTools      int
	FailedTools          int
	DuplicateToolResults int
	DuplicateReadFile    int
	DuplicateGrep        int
	RepoMapTools         int
	Facts                int
	Evidence             int
	Flow                 int
	AdvisoryNotes        int
	Outputs              int
}

func collectSubAgentRuntimeStats(results []*types.SubAgentResult) subAgentRuntimeStats {
	var stats subAgentRuntimeStats
	stats.Branches = len(results)
	seenTools := make(map[string]struct{})
	for _, res := range results {
		if res == nil {
			stats.Errors++
			continue
		}
		if strings.TrimSpace(res.Error) != "" {
			stats.Errors++
		} else {
			stats.OK++
		}
		stats.Facts += len(res.Facts)
		stats.Evidence += len(res.EvidenceItems)
		stats.Flow += len(res.FlowFindings)
		stats.AdvisoryNotes += len(res.InvestigationNotes)
		if len(res.Output) > 0 {
			stats.Outputs++
		}
		for _, tr := range res.Tools {
			stats.Tools++
			if tr.Success {
				stats.SuccessfulTools++
			} else {
				stats.FailedTools++
			}
			if tr.ToolName == "repo_map" {
				stats.RepoMapTools++
			}
			key := subAgentToolResultKey(tr)
			if key == "" {
				continue
			}
			if _, ok := seenTools[key]; ok {
				stats.DuplicateToolResults++
				switch tr.ToolName {
				case "read_file":
					stats.DuplicateReadFile++
				case "grep":
					stats.DuplicateGrep++
				}
				continue
			}
			seenTools[key] = struct{}{}
		}
	}
	return stats
}

func subAgentToolResultKey(result types.ToolResult) string {
	name := strings.TrimSpace(result.ToolName)
	summary := strings.TrimSpace(result.Summary)
	if name == "" || summary == "" {
		return ""
	}
	const maxSummaryKeyBytes = 512
	if len(summary) > maxSummaryKeyBytes {
		summary = summary[:maxSummaryKeyBytes]
	}
	return name + "\x00" + summary
}

// --- SubAgentReducer ---

// SubAgentReducer merges multiple isolated SubAgentResults into a single StageOutput.
// Merge strategy:
//   - Facts/Tools/MCP: append (accumulate)
//   - Signals: OR semantics (any sub-agent sets it → it's set)
//   - InvestigationNotes: append bounded advisory notes for Turn A handoff
//   - Output: merged into JSON array
//   - Errors: concatenated
type SubAgentReducer struct{}

const maxReducedSubAgentAdvisoryNotes = 12

// Reduce merges all results into one StageOutput for the orchestrator.
func (r *SubAgentReducer) Reduce(results []*types.SubAgentResult) *StageOutput {
	merged := &StageOutput{}
	signals := &types.ExecutionSignals{}
	var outputs []json.RawMessage
	var advisoryNotes []string

	for _, res := range results {
		if res == nil {
			continue
		}

		if res.Error != "" {
			if merged.Error != "" {
				merged.Error += "; "
			}
			merged.Error += fmt.Sprintf("[%s] %s", res.RequestID, res.Error)
			continue
		}

		// Accumulate
		merged.NewFacts = append(merged.NewFacts, res.Facts...)
		merged.EvidenceItems = append(merged.EvidenceItems, res.EvidenceItems...)
		merged.FlowFindings = append(merged.FlowFindings, res.FlowFindings...)
		advisoryNotes = appendSubAgentAdvisoryNotes(advisoryNotes, res.InvestigationNotes...)
		merged.ToolResults = append(merged.ToolResults, res.Tools...)
		merged.MCPResponses = append(merged.MCPResponses, res.MCPResps...)

		// OR-merge signals — only HasEnoughFacts survives after the
		// write-pipeline deletion.
		if res.Signals != nil && res.Signals.HasEnoughFacts {
			signals.HasEnoughFacts = true
		}

		if res.Output != nil {
			outputs = append(outputs, res.Output)
		}
	}

	merged.SignalUpdates = signals
	merged.InvestigationNotes = advisoryNotes
	if len(outputs) > 0 {
		merged.Data, _ = json.Marshal(outputs)
	}

	return merged
}

func appendSubAgentAdvisoryNotes(existing []string, incoming ...string) []string {
	if len(incoming) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, note := range existing {
		key := strings.TrimSpace(note)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, note := range incoming {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		if _, ok := seen[note]; ok {
			continue
		}
		seen[note] = struct{}{}
		existing = append(existing, note)
		if len(existing) >= maxReducedSubAgentAdvisoryNotes {
			break
		}
	}
	return existing
}
