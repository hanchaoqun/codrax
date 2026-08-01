# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T02:51:07Z
- sweep_start_ts: 20260731-195105
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260731-195107 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 189s | 41 | read=1,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | AR1 covered：客户实形错误系统附注 `144.557ms / 0.440% → 100.000%` 完全消失。模型正文正确给出 3 次、0.635ms、约 0.44%，system principal authority 发布 complete 3-row roster、d_state=0/io_wait=3 与同一 caller；无 finalizer reject/patch。focused 无窗仍保持 projection_blocks=0。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-195107 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 210s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正式 UTF-8 中点 oracle 通过。projection_blocks=2；完整 Trace 因果投影、根因排序、窗内可消除量、VerifyClass/类校验 0.285ms、最晚边 34579.496810s、直接裸边和系统补采均在。模型 principal 同样把 semantic span 列为链上 #2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B15-AR1`：covered。
- `EVAL-B15-H8O1`：covered。
- 显式时间窗 Trace 因果投影、focused 无窗窄报告两种 report shape 均未回退。
- 下一批离开 C2/H8，选择久未回放的 perf off-CPU 证据质量与 repo typed
  caller relation，分别覆盖 runtime caliber 和代码关系枚举。
