# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T06:23:53Z
- sweep_start_ts: 20260731-232351
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260731-232353 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 109s | 33 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 无用户时间窗的精确等待事实继续走窄报告：0 root/wakeup/blocking view、`trace_query_final_projection_blocks=0`，没有因果投影或代表窗泄漏。模型探索曾在 2段/0.351ms 与 3段/19.671ms 间摆动，但 finalizer 消费 typed complete roster，系统主值和正文都正确发布 3段 `0.138+0.147+0.350=0.635ms`。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260731-232353 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B19c 接线通过：显式窗、根因排序、唤醒链、可消除量、因果投影、coverage、系统补采均在，且 lead 后出现 3 行 deterministic 代表性时间窗表。人工仍失败：typed coverage 为 `frame_causality=unproven/frame_evidence_status=absent`，模型却断言 VSync 落后“造成视觉卡顿”；模型还把两个可能重叠席相加为 42.9ms。属于既登记 FRAME1/ARITH2 的 authority-order 同根残余。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### B19c 正负接线覆盖

- 正例的系统投影顺序为 `Trace 因果投影 lead → 代表性时间窗 →
  因果投影关键指标`。三行均来自 typed ranked seats，明确标注“单次代表
  occurrence”与“全查询窗聚合不可当作单窗时长”。
- 正例仍执行 5 次 windowed trace query，覆盖 root、wakeup、blocking、
  timeline 和 resource 五个维度；完整显式窗合同没有因新发布面缩窄。
- 负例没有用户时间窗，只有 timeline/resource 事实视图；最终
  `trace_query_final_projection_blocks=0`。这证明代表窗 builder 不会把普通
  状态查询重新升级为全量因果报告。

### 权限顺序残余

- 正例开头直接声称“发生严重卡顿”，并把窗后 7.3ms 的
  `Choreographer#onVsync` 解释为“主线程仍处理上一帧、造成视觉卡顿”。
  尾部系统 coverage 却明确没有可绑定到目标的 frame/deadline 证据。
- 同一正文把 #1/#2 两个根因席相加成 42.9ms，并写成“主导超过窗口一半”；
  系统指标说明根因席可能重叠、修向收益不可相加。
- 两者都不是缺少 typed 真值，而是 exact authority 晚于模型叙事。下一批应
  以 typed 结论权限块前置解决，不能扩展模型 prose regex、删除模型正文，
  也不能抑制已有因果投影或自动补采。
