# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T09:34:19Z
- sweep_start_ts: 20260809-023417
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-023419 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 165s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、三次 trace_query、自动补采和 Trace 因果投影均保留。主根因仅来自已证链上的 threadpool-400 io_wait 11ms；logger-900/IO 压力明确为 background、不参与排序。一次 completion，无 relation_claims JSON 重试。 |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260809-023419 | log_regex,answer_regex | none | 462s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B426 生效：无 contributions 时未提前发布 reconcile/assemble，最终 typed DAG 可达 complete。但模型的 compute_contributions 同时声明 operation=count 与 value_field=id；执行器静默忽略 value_field，assemble 只能发布分组计数数组。16 批、5 repairs、4 action failures 后仍非 {"ids":[...]}，立案 B427。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `trace_query_wakeup_background_demotion`: human PASS。模型结论和系统投影都遵守 typed on-chain 权限；邻近 sleep、logger D-state 和聚合 IO 压力只作支撑/额外排查方向，没有因数值更大而越权成为主因。
- `data_json_strict_ids`: human FAIL，但 B426 的 stage reachability 已生产生效：流程依次形成 decisions、contributions、reconcile、assemble，没有再出现系统同轮“允许未来动作、执行又必拒”的合同冲突。
- 新 `EVAL-B427-COUNTIGNORESVALUE1=P1/HIGH` 是 typed action 自相矛盾：`operation=count` 的执行定义固定贡献值为 `1`，却允许同一 action 携带会被静默忽略的 `value_field=id`。模型 thinking 明确想收集成员，JSON 却把 count/value_field 同时发射；系统接受后不可逆地把成员语义降成行计数。
- 根修不扫描目标、rules、thinking 或最终答案，也不让系统代写 JSON：在 compute_contributions 的公共 typed 参数准入边界拒绝 `count + non-empty value_field`，明确要求“纯计数删除 value_field；成员集合改用 include/set/rank”。正确 member contribution 随后由既有 reconcile + declared output_field 投影为 JSON list。
