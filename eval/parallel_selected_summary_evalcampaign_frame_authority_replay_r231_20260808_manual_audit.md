# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T00:39:18Z
- sweep_start_ts: 20260808-173917
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-173918 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 117s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B390 生效：没有固定 16.67ms/60Hz/jank verdict；四个跨线程成员与 temporal/unproven ceiling 均保留。模型仍把 span 名扩写成“接收垂直同步、布局绘制、光栅化、硬件显示输出”等未证内部机理，并把 marker/stage role 提升成 UI/RenderService/GPU 硬件线程角色。无 diagram，故 B391 的生产 Mermaid 臂本轮未覆盖。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-173918 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 127s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 确定性投影正确保持 typed on-chain 根因、双轴、邻近/背景隔离与 unproven ceiling。模型却把同线程 #5 的 `fscache_page_wait_o` 借给原因未证的 #3 10.433ms 席，把无 absolute_level 的 604.528/551.600 称为偏高/较重，并把 lower-priority dependency candidate 扩写成已发生反转/供给不足。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner: `2/2 PASS`; human: `0/2 PASS`。两案均是第一次成文、`finalizer_reject=0`，没有畸形 JSON、JSON repair、Mermaid repair 或系统替换模型正文。
- B390 生产闭环：perf-triage 已明确当前 payload 没有 refresh/deadline carrier，测得 duration 被保留，最终答案没有固定预算、jank、丢帧或未丢帧 verdict。
- B391 代码与单测仍成立，但本轮模型没有发 diagram，生产图边关系门未被触发；保持 `pending-production-diagram-replay`，不能用 runner PASS 虚关。
- Donghu 的系统投影没有把邻近/背景升为主因：主根因席仍来自 typed on-chain population，链外压力只在背景板；真实占时与现有规则可消量仍为两条独立决策轴。失败发生在模型对 typed carrier 的消费，不应通过系统改写答案修复。

## Confirmed generalized gaps

1. `EVAL-B392-TRACESPANSEMANTICAUTH1=P1/HIGH`：span/marker 名称与 interval 被模型扩写为未证的内部工作、同步输入、跨线程 handoff、硬件操作和完成/显示语义；marker/stage role 也被当成 thread role。应提供语言/平台中立的 typed soft boundary：标签只证明名称、实测区间与 producer 给出的 stage role，内部机理与 owning-thread role 需要独立 typed evidence。
2. `EVAL-B393-TRACEROWLOCALCALLERAUTH1=P1/HIGH`：同 subject 的 proof-partition 席在长上下文里丢失 row identity，原因未证的 10.433ms 席借用了另一 7.386ms 席的 `fscache_page_wait_o`。应把 caller presence/absence 与 sibling-transfer forbidden 近席携带到最终 compact ledger；不根据 subject/name/value 猜关系。
3. `EVAL-B389-TRACECONTEXTOVERLOAD1=P1/CONFIRMED`：Finalizer 同时收到通用 Runtime Trace 13 项清单、完整 Trace Decision Inputs、Observation Ledger 和最终 boundary；精确 aggregate-scale/candidate/caller ceiling 虽在尾部存在，模型仍被早期通用叙事稀释。应由 `TraceEvidenceAuthority` 的存在选择 compact typed guidance，保留作用域、状态区间、thread/span role、causal ceiling；不再重复 Binder/IO/perf/root-cause 等未必相关的通用教学。触发只读 typed carrier，不扫描请求或模型原文。
4. `EVAL-B394-TRACEAGGREGATESEVERITY1=P1/SUBSUMED-B389`：无 `absolute_level` 的 aggregate score 仍被模型说成偏高/较重。最终 typed scale boundary 已正确，先通过 B389 的近席去重提升可见度；若复放仍失败，再考虑更短的 per-row calibration carrier，不新增正文硬门。

## Invariants for the next batch

- 不改变显式时间窗、自动补采、帧 timeline/flow、唤醒链、根因排序、因果投影或窗内可消量。
- 主因人口继续只能来自 typed on-chain 席；adjacent/background 只能支撑或给出额外排查方向。
- 不扫描用户问题、thinking、summary、final text；不按具体 span/caller/线程名拟合；不由系统删除、替换或补写模型结论。
- 所有新提示都是 typed soft guidance；任何 hard gate 仍只消费 schema-valid 精确信号。
