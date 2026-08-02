# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T11:39:15Z
- sweep_start_ts: 20260802-043915
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-043915 | log_regex,answer_regex | none | 71s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `2`；初始脚本漏读 instructions 仍在执行前被 AST guard 拦下，repair 后一次执行成功，`data_rounds=1 / repair_rounds=1 / consumed=2`。evaluator 一度把当前批 freeform 误读成 effective strict contract 不满足并重复权衡，但 deterministic projection 已满足且最终无错误；记录为 P3 模型波动 watch，不扩本批。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-043915 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 94s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 与 r3 相反，analyzer 首次把同一有限三字段请求标成 `causal_diagnosis`，同时又声明 intent=trace、scenario=generic、is_diagnostic=false。共享 authority 因此合法放行；模型跑 ipc_graph+wakeup_chain，系统随后补 root_cause_rank+critical_blocking，生成约 36.6K 完整因果报告。不是刚修旁路回退，而是 breadth profile 与其他 typed diagnosis lanes 自相矛盾。登记 PROFILECOHERENCE1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2 PASS；human: 1/2 PASS。
- Data 的执行前消费证明连续两轮稳定生效。
- Binder 暴露 analyzer typed schema coherence gap：`causal_diagnosis` 不应在所有诊断/
  attribution carrier 均为 negative 时被接受。修复必须 fail-loud 让 analyzer 自己重发，
  不能由系统把 scope 静默改成期望值，也不能读取 Binder/问题/答案关键词。
