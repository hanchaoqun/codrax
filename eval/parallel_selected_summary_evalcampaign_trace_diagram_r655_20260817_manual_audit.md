# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T01:13:54Z
- sweep_start_ts: 20260817-181353
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-181355 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 189s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Exact 1.000000..1.010000 supplementation, on-chain worker-200 #1 (8.300ms effective), target sleep as symptom, background-only pressure, causal projection, and no fixed-4ms fallback all remain correct. The model's prose still leaks raw enums (`priority_inversion_candidate`, `rank #2`) and overstates a cross-CPU dependency as direct scheduling blockage; the deterministic note correctly says the CPU2→CPU1 wake edge does not prove same-core occupation/preemption/direct competition. Improve typed customer-language/caliber context, but do not rewrite the model conclusion or hard-scan prose. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-181355 | answer_regex,answer_contains,mermaid_edge_count | none | 419s | 44 | read=18,repo_map=3,list=1,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=3,unavail=0,prune=2 | partial | The model took a different exact path and grounded `Mutable: bus.Mutable` plus several `ctx.Mutable.*` reads, so B1032's forward handoff→callee-body branch was not exercised and remains pending production replay. New B1033 is deterministic: once a callee-body initializer/formal-parameter flow is grounded, repair cannot walk backward to an exact caller argument handoff, so real local edges remain disconnected from the requested pipeline component. The final Mermaid is syntactically valid but still declares BusContext/Mutable unproven while prose claims a shared instance; three final rejects and 419s show the unresolved relation-component/collision contract. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized disposition

- `B1032-CALLEEBODYRELATIONFRONTIER1`: implemented and pinned, but r655 did not exercise its trigger because no grounded caller argument-flow row preceded the callee-body evidence. Keep status pending production replay.
- `B1033-CALLEEBODYREVERSECALLFRONTIER1`: confirmed P1. From an exact grounded operation inside one uniquely owned callable, use parser formal-parameter identity plus exact caller relations/arguments to select one bounded reverse frontier. This remains navigation-only and ambiguity-fail-closed; the model must emit the caller handoff.
- `B756-RUNTIMEENUMCUSTOMERLANGUAGE1`: remains open. Raw audit enums and an insufficiently emphasized cross-CPU caliber boundary influenced model wording; solve with typed localized display/caliber context, never output rewriting.
- Trace explicit-window causal projection and supplementation stayed protected; active streaming produced a full answer and never used a fixed-time fallback.
