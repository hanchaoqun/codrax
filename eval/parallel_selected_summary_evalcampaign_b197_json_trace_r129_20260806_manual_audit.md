# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T21:55:24Z
- sweep_start_ts: 20260806-145523
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260806-145524 | log_regex,answer_regex | none | 58s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 输出精确为 `{"ids":["u1","u3"]}`。首轮计划声明读取 instructions.md 但脚本没有实际消费，精确材料门触发一次 repair；模型随后读取两个输入并正确收敛。没有 DAG 重建、字段漂移、completion 越权或 JSON 通道矛盾。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-145524 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 34 | read=2,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、一次 bundle 查询、双轴、根因排序、唤醒链、可消除量、因果投影和自动补齐均在；但正文把 priority_inversion_candidate 推成线程“相互抢占”“推迟主线程”“嵌套阻塞”，又从目标 D/io_wait=0 推出 S 态“而非锁”等未获授权机理。系统尾部 caveat 虽正确，决策席行仅显示 kind=runnable 且缺少就地 candidate/sleep 权限，模型未把远端边界约束带入总结。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `EVAL-B196-DATAJSONTERM1`、`EVAL-B197-SPLITTERM1`、`EVAL-B198-EVALAUTH1` 在生产回放闭环；JSON 教学保持单一结果通道，系统没有替代 evaluator 的业务判断。
- `EVAL-B195-PIAUTH1` 连续第三次人工失败，已排除单轮模型波动。根因是 principal decision row 丢失 typed cause kind/候选权限，而 target-state 行没有就地声明 S-sleep 原因不可由状态分区判定。下一批只增强 prompt-owned typed handoff，不扫描答案、不硬拒、不改写模型正文。
- Trace 显式时间窗、因果投影、自动补采与双维根因能力均未退化。
