# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T12:01:44Z
- sweep_start_ts: 20260802-050144
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-050144 | log_regex,answer_regex | none | 48s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | route=data；terminal script 首次漏读 instructions.md 被 AST guard 在执行前拒绝，repair 后真实读取两份材料；data_rounds=1、repair_rounds=1、action_failed=0，最终严格只有 `2`。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-050144 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 100s | 28 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | analyzer 接受 bounded_fact_set；仅一次 wakeup_chain，正文 PID/TID、transaction=42、直接 waker 与方向均正确，且无 full supplement/floor/根因排序/可消除量/因果投影。但系统仍追加 client-20 五态状态卡及同值 typed 附注，用户未请求 scheduler state。证明 B41-T20 的粗粒度 focused-mechanism fallback 仍把有限 IPC 事实误认成状态主值。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
