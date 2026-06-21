package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Phase 2 (2026-05-17) — global parallelism cap shared across every
// orchestrator-owned parallel fan-out surface (post-emit reviewer
// pair, multi-sub_topic explorer dispatch, future parallel surfaces).
//
// Design rationale. Before this commit, P2C wired the reviewer
// errgroup as "always parallel" and the E' Phase 1 explorer dispatch
// was "always serial". That worked but didn't give operators a single
// knob to tune for:
//
//   - LLM rate-limited environments (force serial via
//     pipeline_max_parallelism: 1)
//   - Debug reproducibility (force serial so logs are linear)
//   - High-throughput environments (uncap via
//     pipeline_max_parallelism: 0)
//
// The orchestrator now reads MaxParallelism off PipelineSettings and
// every parallel fan-out call site uses effectiveParallelism(...) to
// decide goroutine count.

// effectiveParallelism returns the actual goroutine count a fan-out
// surface should use given the operator-configured cap and the
// number of candidate work units. Semantics:
//
//   - configured == 0 → return candidateCount (unlimited; every
//     candidate runs concurrently)
//   - configured == 1 → return 1 (strict serial; the caller MUST use
//     a sequential for-loop, never an errgroup)
//   - configured >  1 → return min(configured, candidateCount) (cap
//     applies; surfaces with fewer candidates than the cap run all
//     of them in parallel)
//
// Negative configured values are treated as "use default":
// DefaultPipelineMaxParallelism (= 2). The default handling
// matches PipelineSettings.MaxParallelism's documented zero-default
// fallback for unset operator values.
//
// candidateCount <= 0 short-circuits to 0 (no work, nothing to
// schedule).
//
// The return value is the maximum number of goroutines the caller
// should spawn — NOT a per-iteration counter. A caller with
// candidateCount=5 and configured=2 spawns 2 goroutines initially
// and feeds the remaining 3 candidates through a worker pool;
// caller with configured=0 spawns 5 goroutines outright.
//
// Pure function, deterministic, no global state.
func effectiveParallelism(configured, candidateCount int) int {
	if candidateCount <= 0 {
		return 0
	}
	if configured < 0 {
		configured = types.DefaultPipelineMaxParallelism
	}
	if configured == 0 {
		return candidateCount
	}
	if configured == 1 {
		return 1
	}
	if configured > candidateCount {
		return candidateCount
	}
	return configured
}

// effectiveGraphParallelism intersects the operator cap with the
// analyzer-authored graph policy. Operator semantics stay identical to
// effectiveParallelism: 0 means unlimited, 1 means serial, negative
// falls back to the runtime default. Graph policy semantics are
// intentionally narrower: MaxParallelism <= 0 means "IR did not add an
// extra cap"; positive values cap the already operator-bounded fan-out.
func effectiveGraphParallelism(operatorCap int, policy types.ExecutionPolicy, candidateCount int) int {
	operatorEffective := effectiveParallelism(operatorCap, candidateCount)
	if operatorEffective <= 1 || candidateCount <= 0 {
		return operatorEffective
	}
	graphCap := policy.MaxParallelism
	if graphCap <= 0 {
		return operatorEffective
	}
	graphEffective := effectiveParallelism(graphCap, candidateCount)
	if graphEffective < operatorEffective {
		return graphEffective
	}
	return operatorEffective
}

// orchestratorMaxParallelism returns the operator-configured
// MaxParallelism for this orchestrator instance, defaulting to
// DefaultPipelineMaxParallelism when unset or out of range.
//
// Hard-capped by MaxPipelineParallelismCeil (= 16) to prevent a
// misconfigured "100" from spawning an orchestrator-killing
// goroutine flood. Negative values fall back to the default.
func (o *Orchestrator) orchestratorMaxParallelism() int {
	if o == nil {
		return types.DefaultPipelineMaxParallelism
	}
	v := o.settings.MaxParallelism
	if v < 0 {
		return types.DefaultPipelineMaxParallelism
	}
	if v > types.MaxPipelineParallelismCeil {
		return types.MaxPipelineParallelismCeil
	}
	return v
}
