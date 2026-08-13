# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T15:11:01Z
- sweep_start_ts: 20260813-081100
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-081101 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 42 | read=1,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | partial | B727 生产正证：bounded exact-window 只发布 11 段/36.757ms/caller 窄账，没有全量因果报告。runner 失败源于旧 oracle 把 caller 称为等待对象；正文仍把调用点扩写为 devhost 持有的 DMA fence，随后又承认 holder 未知，形成模型自相矛盾。系统 typed 附注正确只称内核调用点；不得以答案关键词硬改。 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-081101 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 因果投影仍在，但主席席丢失。模型先发 target-free root_cause_rank；Analyzer 的 thread-only `CompThread_0-2955` 与后续 cursor `{pid:2955,name:CompThread_0}` 被视为两目标，省略 selector 的查询不继承；补齐器又只按时间窗把全局 rank 当作目标 rank，skip=families_present。最终未出现 65.912ms 自身 running 折算和 49.623/0.033 同源分账，背景候选还被模型正文称“根因排序”。确认 B728，非模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
