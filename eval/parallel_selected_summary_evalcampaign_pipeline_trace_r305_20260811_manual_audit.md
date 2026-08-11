# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T10:22:49Z
- sweep_start_ts: 20260811-032247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260811-032249 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 111s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 主链 threadpool-400 → network-300 → cookie-200 → app-100、#1 `fscache_page_wait_on_page_bit` IO wait=11ms、三个 runnable=各 1ms 均与 typed trace 一致；邻近 sleep 与 IO pressure 保持 context/background，没有越权加冕，也未误报优先级反转。但模型称 20ms 总延迟由 11ms+3×1ms“共同构成”，实际仅 14ms；系统 handoff 已列 actual occupancy、priced seats 与未计价占用，却未高显著度禁止在无 exact additive carrier 时把 priced 子集写成完整分解。另有 target sleep E1/E2 以及 cookie/network sleep 跨 lane 重复，需独立去重审计。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-032249 | answer_regex,answer_contains | none | 532s | 44 | read=23,repo_map=5,list=0,trace=0,source_lens=1 | midloop=9,inv=5/0,fin_reject=2,unavail=0,prune=4 | fail | B518 有部分正效：最终图不再把源码行号写进箭头，exact 8 条关系与 5 个不连通分量保住。仍有三类问题：可见箭头继续使用 `stage precedence`/函数名而非业务动作；模型把 4 个 canonical stage + 结果写回误称“5 个阶段”，表头退化为“项目/列 2..5”；第一次 relation patch 把 participant_boundaries 放到带嵌套 diagram 的 `kind=section`，触发第二次确定性 reject。后者根因是局部合同要求 boundary row，却没有教授“嵌套 Mermaid 必须拆成独立 kind=diagram block”，立 B520；前两项暂记模型波动/soft-guidance effectiveness 未闭环，不追加 prose hard gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
