# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T01:34:51Z
- sweep_start_ts: 20260731-183449
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260731-183451 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 210s | 46 | read=4,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | HG1/PV1/CAP1 真实生效：finalizer reject 8→0；system lead 逐条发布 3 次/0.635ms/完整 caller，模型正文也收敛；trace_query_final_projection_blocks=0，未恢复全量因果投影。残余：同一 attached_trace.txt 被两个 artifact_id 别名铸成“自己↔自己”的跨工件关系表，另立 REL1。 |
| 1 | real_trace_h3_iofam_one_seat | PASS | eval/results/real_trace_h3_iofam_one_seat-20260731-183451 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 217s | 43 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗非回退通过：final projection blocks=2，完整因果投影/root/wakeup/可消除量与系统补采均在，IOFAM 硬词完整；但模型把 storage_latency_by_layer 的 85-op avg=0.343ms 绑定成可见 6 次的平均值（6 条已列值平均约 1.076ms），直接数值口径存在跨集合错误，按 P2 模型残余留档。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### C2：WS2 收账

- `finalizer_rejects=0`、`finalizer_rewrites=0`，r2 的 8 次错误硬拒绝与 degraded
  export 已消失。
- system lead 发布
  `target_wait_occurrence_roster=complete`、
  `roster_scope=producer_paired_complete`、`occurrences=3`、
  `occurrence_wall_clock_sum=0.635ms`，以及三条精确
  start/end/duration/state/iowait/caller。
- 模型正文也给出相同三行，不再附带“capacity 截断所以实际次数可能更高”的
  错误 caveat。
- `trace_query_final_projection_blocks=0`，日志没有
  `materialized runtime trace causal projection`；无窗 focused 查询仍是窄
  principal-value 答案。
- 新系统 GAP：答案出现同一路径
  `attached_trace.txt ↔ attached_trace.txt` 的跨工件关系边界。ledger 中同一
  immutable blob path 被不同 artifact_id 别名分桶；pair builder 以 artifact_id
  优先，未以 canonical typed path 合并。

### H3：显式窗非回退通过，模型跨集合平均值绑定错误

- `trace_query_final_projection_blocks=2`，日志同时出现 scope canonicalization、
  target principal card 和 causal projection materialization。
- 完整 Trace 因果投影、root rank、wakeup chain、critical blocking、窗内
  可消除量，以及 `完成端到端·IO延迟（io_latency）` /
  `块设备层·块设备IO(inode)` / `综合评分,非墙钟` 均在。
- 模型列出的 6 条 io_latency 为
  0.865/0.884/1.056/1.058/1.248/1.347ms，随后却写“6 次平均 0.343ms”。
  `0.343ms` 属于 `storage_latency_by_layer` 的 85 条全层操作平均，不是这
  6 条可见子集的平均；子集算术平均约 1.076ms。
- typed system projection没有该错误。当前只证明模型把 aggregate 的 denominator
  错绑到 visible subset；先按模型波动/P2 留档，不增加输出原文扫描或算术硬门。
