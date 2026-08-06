# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T21:23:43Z
- sweep_start_ts: 20260806-142341
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-142343 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 30 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S27 生产回放通过：未再出现窗口总长/百分比伪配对；显式窗、双轴、根因排序、wakeup chain、因果投影、自动补采完整。模型仍把 candidate 写成“阻滞主线程及时响应”，并由目标 D/io_wait=0 推出“等待均为正常 S 态 VSync 等待”；typed 输入只支持候选有效归因和目标状态，不支持把所有 sleep 定性为正常 VSync，人工不签绿。 |
| 2 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260806-142343 | log_regex,answer_regex | none | 255s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 第一批 custom_transform 已精确生成 `{\"ids\":[\"u1\",\"u3\"]}`，但 ActionRunner 把终态子计划强制降为 freeform，Runner 只能保存 emitted_payload、不能按外层 json_only 合同提升为 Answer。之后系统一面要求 assemble_answer，一面只允许 compute-stage actions，触发 6 次 repair、8 批重建，最终仍因 contested answer 失败。确定性系统合同 gap，非模型 JSON 畸形。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B193-ARITHPAIR1` 生产闭环；S27 没有侵入 Trace runtime causal 能力。
- `EVAL-B196-DATAJSONTERM1=P0/system-contract-contradiction`：终态 custom transform 的外层 strict output contract 在子 Runner 入口丢失，已正确计算的 payload 无法成为答案；后续 DAG 修复合同互相冲突并造成高重试。
- `EVAL-B195-PIAUTH1` 仍开放。r127 没有虚构持锁，但仍把候选升级成确定阻滞，并把全部 sleep 推成正常 VSync 等待。系统不得改写正文；应把 typed seat 的 caliber/未授权机理就近携带到 principal decision row，减少模型跨长 prompt 关联负担。
