# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T05:39:24Z
- sweep_start_ts: 20260818-223923
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260818-223924 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 269s | 38 | read=2,repo_map=0,list=0,trace=10,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Numeric state/frequency oracles pass and policy binding remains unproven. B1142 is not production-closed: despite the new typed caliber context, the final broadens scheduler-marked D/io_wait=0 into proof of no disk/IO blocking. The absent completion-closed ruler in this finite query is treated as zero instead of not assessed. B1139 enum leakage is production-reconfirmed; B1143 newly records unsupported Sleep mechanism prose. No causal projection was required for this bounded effect query. |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-223924 | answer_regex,answer_contains | none | 403s | 30 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=0,unavail=0,prune=0 | partial | The answer correctly refuses to invent buildAnalysisIR→gate.Run and renders two grounded edges converging on RunWith; Mermaid source repair is positive. B1144/P1: Analyzer drops the explicit “图后列出关键中间函数” clause, retaining only one 调用顺序 dimension. The final consequently puts a two-edge list before the diagram and omits the already-grounded normalizer/compiler/risk/plan/binder intermediate inventory. This is a typed multi-clause dimension/order gap, not a Mermaid syntax failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit Conclusions

- Runner status is 2/2 PASS, but human correctness is 2/2 partial. Numeric/regex success does not close reader semantics or requested presentation order.
- B1142 needs one more generalized soft-context batch: publish a copy-ready reader label for the narrow scheduler classifier and state that an absent completion-closed ruler means “not assessed by this state partition”, never zero. This must not inspect or rewrite final prose and must not classify ordinary S as IO.
- B1139 remains a display/context debt: raw keys and enums (`target_window_states`, `complete`, `target_window_wait_occurrences`, `unproven`, `full_window_all_cpu`) still leak into Chinese prose despite the localization hint. Do not solve it with an answer keyword rejection gate.
- B1143/P2: scheduler state `S` alone does not prove voluntary yield, idle, preemption, or any waiting mechanism. The typed state-caliber context needs an explicit non-mechanism boundary.
- B1144/P1: required diagram and sibling list/order clauses must be independent typed answer dimensions. A diagram request must not absorb or erase “after the diagram list X”. The system should guide layout from those typed dimensions while the model remains the author.
- No active-stream 4ms degradation, malformed-answer fallback, finalizer rejection, system conclusion replacement, or Trace causal-projection regression was observed.
