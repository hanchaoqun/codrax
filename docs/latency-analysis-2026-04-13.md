# End-to-end latency analysis — 2026-04-13

Source: B5b-α eval grid run at HEAD `7166e49` + local IR read-cutover
patch (not yet committed). Logs under `eval/results/{df1,df3,t1,t2,t3}-20260413-1*`.

This document is a working notes snapshot for optimizing explorer-stage
latency. It is descriptive, not prescriptive — the fix designs come later.

## 1. Where time goes

### 1.1 Per-stage wall time (case × run-1 sample)

| case | analyzer | explorer | finalizer | total  | notes |
|------|---------:|---------:|----------:|-------:|-------|
| df1  |    5.3 s |   52.7 s |     5.8 s |  63.8 s | single explorer dispatch, 12 iters |
| df3  |    5.3 s |  124.5 s |     6.4 s | 136.2 s | **2× explorer dispatch**, 39 rounds |
| t1   |    5.5 s |  305.4 s |     9.0 s | 320.0 s | **2× explorer dispatch**, 57 rounds |
| t2   |    3.5 s |   86.0 s |    12.9 s | 102.4 s | single dispatch, 27 iters |
| t3   |    5.5 s |  135.9 s |     4.9 s | 146.3 s | single dispatch (run-1), 29 iters |

Analyzer and finalizer are effectively constant: **~5 s + ~6 s = 11 s**
per run regardless of case. Explorer dominates: **82–95%** of total wall
time for every case.

### 1.2 Within explorer: LLM wait vs tool execution

Measured via iter timing diff on df1 run-1 (48 s explorer wall):

| component              | seconds | share |
|------------------------|--------:|------:|
| LLM turn latency (sum of gaps between tool-result ends and next assistant turns) | 36.2 s | 75% |
| In-iter tool execution + framework overhead | ~12 s | 25% |

Tool calls themselves (grep, read_file, repo_map) finish in <1 s locally.
The 25% framework overhead is mostly startup, keyword search, IR
construction, and logging — fixed cost per iter.

**LLM round-trip latency is the dominant cost — roughly 3 s per iter in
this sample.** 12 iters × ~3 s = ~36 s of pure LLM wait.

## 2. The self-dispatch problem

Count of explorer re-dispatches per run across 5 cases × 3 runs each
(15 runs sampled):

| case | dispatch count distribution | re-dispatch rate |
|------|-----------------------------|------------------:|
| df1  | 1,1,1                       | 0/3  |
| df3  | 2,1,2                       | 2/3  |
| t1   | 2,2,2                       | **3/3** |
| t2   | 1,1,1                       | 0/3  |
| t3   | 1,2,1                       | 1/3  |

Raw assistant-turn counts correlate strongly with dispatch count:
- Single-dispatch runs: 12–30 rounds
- Double-dispatch runs: 39–58 rounds (≈2× single)

Each re-dispatch re-runs Phase 0 (keyword search) + Phase 1 (breadth
scan) + Phase 2 (depth read) from scratch, doubling wall time.

### 2.1 Root cause: unsatisfiable ERM requirements

From `t1 run-1` debug log, the explorer soft-stops its first dispatch at
iter=18 with this state:

```
call_chain(BaseAgent,Execute,evaluator,...)=satisfied
mechanism(...)=satisfied
registration(BaseAgent)=satisfied
registration(evaluator)=satisfied
registration(Execute)=partial
registration(ContinuingEvaluator)=partial
registration(SynthesizingEvaluator)=unsatisfied
registration(synthesis)=unsatisfied
registration(continuation)=partial
```

All of the call_chain and mechanism reqs are green, but **several
`registration(*)` reqs cannot be satisfied** because the referenced
terms (`synthesis`, `continuation`, `SynthesizingEvaluator`) either
are not registered as symbols in the codebase, or the registration is
through interface embedding rather than a literal registration call.

`ermAutoSatisfyUnresolvable` is supposed to catch this but clearly
misses the interface-method and concept-term cases. The orchestrator
then interprets "unsatisfied reqs" as "explorer did not finish" and
dispatches it a second time. The second dispatch finds the same
unsatisfiable state and soft-stops identically — 143 s burned for zero
new evidence.

### 2.2 Scale of the waste

Rough count across the 15 sampled runs: 8 re-dispatches × ~120 s each
= **~960 s of redundant explorer work** out of ~3000 s total eval
time. That is a **30–35% latency floor** that could be unlocked by
fixing the unsatisfiable-requirement detection.

## 3. Other observations

### 3.1 iter=0 is parallel, iter>0 is serial

```
iter=0: 3-8 tool calls dispatched together (batched ReAct)
iter>0: typically 1-2 tool calls per round
```

The LLM batches tool calls aggressively at the very start of Phase 1
(keyword-scan seed files), then degenerates into single-call ReAct
rounds as it follows references. Serial rounds pay full LLM latency
per tool call. If mid-loop rounds could batch 2-3 related reads the
latency would drop proportionally.

### 3.2 Prompt size grows with iter

t1 run-1 SYNTHESIS prompt was 37556 chars (1st dispatch) / 41817
(2nd dispatch). Explorer's per-iter prompt grows roughly linearly
with the number of tool results accumulated. Longer prompts are
slower to serve — this is a compounding effect on top of the iter
count growth.

### 3.3 Synthesis calls are cheap relative to ReAct

df1: SYNTHESIS 17622 chars → 3 s response
t1:  SYNTHESIS 37556 chars → 14 s response (1st), 9 s (2nd)

SYNTHESIS is one LLM call per explorer dispatch, so even at 14 s per
call the contribution to total latency is bounded at ~15 s per
dispatch. ReAct loop latency dominates.

## 4. Optimization levers (ranked by expected impact)

| # | Lever | Expected win | Risk |
|---|-------|-------------|------|
| 1 | Detect unsatisfiable `registration(*)` reqs for concept/interface-method terms and auto-satisfy them → **kill self-dispatch** | ~30% total grid latency | Low — detection is deterministic; correctness unaffected because the reqs can never be met |
| 2 | Prompt caching (Anthropic prompt cache) on the static system/instruction portion of explorer's prompt | ~30–50% of LLM-side latency per iter | Low — provider-side, transparent |
| 3 | Batch mid-loop reads: when the LLM asks to read a file, pre-emptively include 1-2 referenced files in the same response | 2-3× effective rounds | Medium — changes ReAct contract, may confuse the LLM |
| 4 | Hard-cap explorer to 1 dispatch per case; force finalizer to work with partial evidence if soft-stop fires | Same as #1 but blunter | Medium — may degrade df3/t1 answer quality |
| 5 | Parallel tool-call encouragement in mid-loop prompt | 10–20% | Low — already works on iter=0, just needs stronger prompt cues |
| 6 | Move keyword search + IR build off the critical path by speculatively starting explorer while analyzer is still finishing | ~5 s per run | Low — tree-sitter/grep is local, cost is bounded |
| 7 | Smaller synthesis prompt (drop low-rank evidence items above a configurable floor) | ~5 s on long-tail cases | Low — already capped; just needs tuning |

## 5. Recommendation

**Attack self-dispatch first (#1).** It is the highest-leverage single
fix: deterministic root cause (unsatisfiable ERM registration reqs
on non-symbol terms), independent of LLM behavior, verifiable on t1
and df3 (both have 100%/66% re-dispatch rates).

The fix path is to tighten `ermAutoSatisfyUnresolvable` so that any
`registration(X)` req whose target X is a concept term, interface
method name, or synonym for a verb (e.g. `synthesis`, `continuation`)
is short-circuited. Target: bring t1 total from 320 s to ~160 s and
df3 from 136 s to ~70 s.

Prompt caching (#2) is the next-best step because it is provider-side
and compounds with every other optimization.

B5b-α's IR read cutover is neutral to all of the above — it is a
data-routing refactor, not a latency optimization. These notes should
inform a later latency-focused branch, not block B5b shipping.
