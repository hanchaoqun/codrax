# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T22:28:50Z
- sweep_start_ts: 20260817-152849
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-152851 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 222s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 用户窗固定为 2.000..2.020s；typed 唤醒链 threadpool-400→network-300→cookie-200→app-100 完整，11ms IO 链上首席与 3×1ms 调度供给分账，邻近 sleep/背景 IO 未晋升主因；frame/deadline 因果未证被明确披露。无固定 4ms 降级。 |
| 1 | read_combo_config_two_knobs_precedence | PASS | eval/results/read_combo_config_two_knobs_precedence-20260817-152851 | answer_regex,answer_contains | none | 244s | 29 | read=11,repo_map=0,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 主体答案已正确给出 code default 50/3、sample 值 50/2、CLI inherit sentinel 0 及 code<yaml<CLI；但系统前置行又称“未找到完全一致目标、仅别名”，与正文直接矛盾。Explorer 已读生产 schema/consumer，却把正向源码窗口误铸 absence_support，随后把 resolved 自动归一成 absence。另有重复空表/清单补充的展示债。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- Runner 为 `2/2 PASS`，人工为 `1 PASS / 1 PARTIAL`。Config 的数值主结论已修正，但可见的
  exact-resolution 头行与正文互相否定，不能按全绿收账。
- Config 共 14 个探索轮、11 次读取、9 条 mid-loop 提示。B1017 的失败 grep 修复信号本轮没有触发，
  因为本轮检索均成功；它仍只有代码 pin，没有生产触发正证。
- 新 gap B1019 是 typed 状态机问题，不是模型波动：正向源码窗口只因 `anchor_symbol` 不是 exact
  target 就被改标 `absence_support`；completion 再据此把模型的 `resolved` 改成 `absence`；最终
  semantic view 被迫要求可见 exact carrier，模型只能填写 `alias_match`。修复必须让源码正向出现
  否定全局 absence，同时不把相关证据越权晋升为定义证明。
- Trace 本轮人工通过。模型负责根因判断与修向；系统只发布 typed 查询、精确窗投影和证据附录。
