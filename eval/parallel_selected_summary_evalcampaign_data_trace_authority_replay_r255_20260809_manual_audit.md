# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T00:53:28Z
- sweep_start_ts: 20260809-175327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260809-175328 | log_regex,answer_regex | none | 76s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B436 生效：纯全量 sum 不再要求先补 decision ledger；typed contributions=2、reconcile=pass、最终严格单行 `17`。仍有 6 批/3 个历史 failed action：初始 custom_transform 被规则前置门改写，完整记录已知后仍插入 value_distribution；记 B438 效率债。 |
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-175328 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 103s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B437 主权限生效：#1 仅链上 threadpool io_wait 11ms，logger 明确为背景且无目标因果贡献；显式窗/自动补采/投影均在。新 B439：正文把 wakeup@2.020000 与窗外 switch-in@2.020020 混成“被唤醒”，把 typed 窗内 20.000ms 写成 20.020ms。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

1. `EVAL-B436-DECISIONAGGBOUND1` production replay closed. The workflow now reaches `compute_contributions` without requiring an item-level decision ledger for an all-record aggregation, and the user-visible answer is exactly the requested scalar.
2. `EVAL-B437-OFFCHAININFERENCESOFT1` production replay closed for root authority. The model no longer transfers the background logger's D-state into the app wakeup cause; deterministic projection and prose agree on the typed-on-chain root population.
3. `EVAL-B438-DATATYPEDINTENTCARRY1=P2` remains open. A prerequisite rewrite loses the already-declared aggregation intent and later chooses an exploratory value-distribution scaffold even though complete records and the amount field are materialized. The generalized remedy is typed downstream-intent carry across prerequisite batches, not a special case for this CSV or value 17.
4. `EVAL-B439-TRACEWAKEVSRUN1=P1` is confirmed. The final prompt carried selected-window authority, but the compact typed trace guidance omitted the generic t_sleep/t_wake/t_run semantics. This allowed the model to relabel switch-in as wakeup and substitute an out-of-window timestamp. The fix must be shared guidance for both typed and generic trace lanes; no prose scanner or answer rewrite is allowed.
5. Root-family coverage remained present in this case for IO/D-state and runnable scheduling supply. The next trace selection must exercise priority inversion, compute supply, and deterministic semantic work so their typed on-chain seats are audited rather than inferred from this fixture.
