# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T14:34:43Z
- sweep_start_ts: 20260813-073442
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-073444 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 179s | 37 | read=1,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Analyzer 最终正确发出 `explicit_time_window + bounded_fact_set` 以及 state/wait/reason/count 四个 typed fact families；模型一次 window_stats 已拿到 11 段、36.757ms、caller=dma_fence_default_w，正文也回答了这些值。Runner 表面只缺陈旧固定词形“等待对象”，且正文把 kernel caller 误扩为等待对象/DMA fence 写等待并把 12 census vs 11 intervals 解释为“正常测量误差”，与现有 typed caller/口径教学冲突。更深的系统 GAP 是 shared authority 让 explicit window 抢在 bounded profile 前，系统自动注入 1000+ 行全量因果投影、根因榜、业务 spans、metrics/next steps；有限四字段问答被错误扩域。两个 analyzer reject 分别来自旧 count→scalar 自冲突和 target profile 修补，仍有合同心智成本。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-073443 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 257s | 44 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B726 获生产正证：Analyzer 一次成功发出 `explicit_time_window + causal_diagnosis + required causal_attribution`；4 次带窗 trace_query 后 final projection=1。最终完整保留主根因 65.912ms、D-state 36.757ms、优先级反转/调度与算力供给、小贡献、业务 span、链上榜、邻近 support-only、枚举边界及实际占用/规则可消双轴。唯一成文 reject 是 table cells 8 对 columns 7，模型一次 patch 自修；没有系统替写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusion

- `B726-RUNTIMEBOUNDEDCAUSALDIMENSIONCONFLICT1` is production-positive on H7.
  The new role did not suppress the explicit-window causal report.
- New `B727-RUNTIMEARTIFACTRANGEANSWERBREADTHINVERSION1` is confirmed. Exact
  time endpoints constrain evidence range; they do not themselves request root ranking,
  wakeup topology, optimization boards, or a whole report. Typed question breadth must
  outrank typed artifact range.
- H2's stale “等待对象” oracle conflicts with the newer caller-caliber contract. The
  trace proves a kernel blocked call-site, not the waited resource identity. Do not add
  answer-prose matching or system-authored wording to satisfy it; update the eval only
  after the product fix is replayed and audited.
- Both active streams ran for 179s/257s and completed with model answers. No fixed-age
  4ms degradation, empty-answer fallback, or malformed-JSON recovery occurred.
