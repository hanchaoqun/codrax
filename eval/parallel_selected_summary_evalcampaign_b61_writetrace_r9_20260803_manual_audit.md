# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T04:40:48Z
- sweep_start_ts: 20260803-214045
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-214048 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗 1.000000..1.010000 与三次带目标过滤的 trace_query 均保留；正文分别总结目标 10.000ms 四态占用、net-300→worker-200→app-100 唤醒链、worker 优先级反转候选，以及链累计 9.000ms/有效可消除 8.300ms，明确“时间占用”和“现有规则可消除量”两轴不可互相替代。Trace 因果投影与证据索引仍由 typed 探索+系统补齐形成并置于模型结论之后，没有系统改写模型答案。 |
| 1 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260803-214048 | log_regex,write_apply,write_patch_oracle | none | 235s | 19 | read=9,repo_map=3,list=2,trace=0,source_lens=1 | fail | 补丁本身正确且只改 src/client.ts；B61 回退已生效：JavaScript probe typed runner_missing 后继续 node syntax fallback，并升级执行 make check，真实仓内检查通过。最终保持 unverified 是正确 fail-closed：make/source_static 只能证明静态契约，不能伪装成 POST/JSON 运行时行为权威。新 gap 是同一 unchanged plan/probe/env 随即创建 verify-only proof batch并原样第二次 run_tests，25 秒内重复相同 runner_missing+make pass，零新增证据；登记 EVAL-B62-PROOFRETRY1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
