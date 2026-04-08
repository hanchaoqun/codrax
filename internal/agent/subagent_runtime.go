package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/hanchaoqun/design/internal/types"
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

		// Scope is required to prevent unbounded operations
		if len(st.Scope) == 0 {
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
			Scope:       st.Scope,
			Constraints: st.Constraints,
			InputData:   st.InputData,
			ReadView:    bus, // shared read — same pointer for all
		})
	}

	return requests, nil
}

// --- SubAgentRuntime ---

// SubAgentRuntime validates, executes, and reduces SubAgent proposals.
// Orchestrator calls Run() with a proposal; Runtime handles the rest internally.
type SubAgentRuntime struct {
	registry  *SubAgentRegistry
	validator *SubAgentValidator
	reducer   *SubAgentReducer
}

// NewSubAgentRuntime creates a new runtime with built-in validator and reducer.
func NewSubAgentRuntime(registry *SubAgentRegistry) *SubAgentRuntime {
	return &SubAgentRuntime{
		registry:  registry,
		validator: NewSubAgentValidator(registry),
		reducer:   &SubAgentReducer{},
	}
}

// Run is the single entry point for the Orchestrator.
// It validates the proposal, executes SubAgents in parallel, and reduces results.
func (r *SubAgentRuntime) Run(bus *types.BusContext, proposal *types.SubAgentProposal) (*StageOutput, error) {
	// 1. Validate
	requests, err := r.validator.Validate(bus, proposal)
	if err != nil {
		return nil, err
	}

	// 2. Execute parallel
	results, execErr := r.execute(requests)

	// 3. Reduce
	merged := r.reducer.Reduce(results)

	return merged, execErr
}

// execute runs all requests in parallel and returns results in the same order.
func (r *SubAgentRuntime) execute(requests []*types.SubAgentRequest) ([]*types.SubAgentResult, error) {
	results := make([]*types.SubAgentResult, len(requests))
	errs := make([]error, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(i int, req *types.SubAgentRequest) {
			defer wg.Done()

			sub, err := r.registry.Get(req.SubAgent)
			if err != nil {
				errs[i] = err
				return
			}

			log.Printf("[subagent-runtime] start[%d] %s: %s", i, req.SubAgent, req.Objective)
			results[i], errs[i] = sub.Run(req)
			log.Printf("[subagent-runtime] done[%d] %s: err=%v", i, req.SubAgent, errs[i])
		}(i, req)
	}
	wg.Wait()

	return results, combineErrors(errs)
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

// --- SubAgentReducer ---

// SubAgentReducer merges multiple isolated SubAgentResults into a single StageOutput.
// Merge strategy:
//   - Facts/Tools/MCP: append (accumulate)
//   - Signals: OR semantics (any sub-agent sets it → it's set)
//   - Output: merged into JSON array
//   - Errors: concatenated
type SubAgentReducer struct{}

// Reduce merges all results into one StageOutput for the orchestrator.
func (r *SubAgentReducer) Reduce(results []*types.SubAgentResult) *StageOutput {
	merged := &StageOutput{}
	signals := &types.ExecutionSignals{}
	var outputs []json.RawMessage

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
		merged.ToolResults = append(merged.ToolResults, res.Tools...)
		merged.MCPResponses = append(merged.MCPResponses, res.MCPResps...)

		// OR-merge signals
		if res.Signals != nil {
			if res.Signals.HasEnoughFacts {
				signals.HasEnoughFacts = true
			}
			if res.Signals.HasPlan {
				signals.HasPlan = true
			}
			if res.Signals.HasPatch {
				signals.HasPatch = true
			}
			if res.Signals.DesignReviewPassed {
				signals.DesignReviewPassed = true
			}
			if res.Signals.CodeReviewPassed {
				signals.CodeReviewPassed = true
			}
			if res.Signals.VerificationPassed {
				signals.VerificationPassed = true
			}
		}

		if res.Output != nil {
			outputs = append(outputs, res.Output)
		}
	}

	merged.SignalUpdates = signals
	if len(outputs) > 0 {
		merged.Data, _ = json.Marshal(outputs)
	}

	return merged
}
