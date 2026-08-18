# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T19:16:23Z
- sweep_start_ts: 20260818-121622
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260818-121624 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 11ms threadpool IO 是链上 #1，三个 1ms runnable 为调度供给次级；精确窗、投影、自动补齐、实际占用/可消双轴均保留，邻近 sleep 未升主因。正文与审计附录仍泄露 status=complete、*_authority=not_provided、tier/causality/predicate 等内部值，确认 B1105 为统一读者语言投影 gap。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-121624 | answer_regex,answer_contains | none | 349s | 30 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=13,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | B1103 已让初始 prompt 带精确 boundary rows，但 anchors 与 boundaries 是两个独立复制载体，模型复制图/anchors 时仍漏 boundary，触发 1 次硬拒；修补后把“未证关系边界”写进正文，并仍把 shared-callee 图叙述成完整调用顺序。确认 B1104 需要原子化 typed diagram sibling carrier，系统不能代画或改写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `trace_query_wakeup_causal_io_chain`

1. 请求窗精确保持 `2.000000..2.020000`。app-100 的 20.000ms S 态是目标等待症状；链上首席为
   threadpool-400 的 `fscache_page_wait_on_page_bit` IO 等待 11.000ms，随后 threadpool/network/cookie
   三个 runnable 席各 1.000ms。邻近 sleep 与 IO pressure 均只作上下文，没有进入根因席。
2. `Trace 因果投影`、确定性补采、主要时间占用与窗内可消除量双轴完整；模型也披露 app-100 的直接
   阻塞资源/业务同步关系未证。活跃流没有固定 4ms 降级，系统没有替换模型结论。
3. 新确认 `B1105-TYPEDREADERLANGUAGEPROJECTION1`：模型正文直接复制
   `target_window_wait_occurrences=0`、`status=complete`、`holder_identity_authority=not_provided`、
   `target_direct_blocking_authority=not_provided`；系统证据索引/补充又发布 `tier`、`causality`、
   `predicate` 等内部枚举。应由 typed 字段各自提供 locale-aware display label / boundary sentence，
   机器 token 只留诊断工件，不扫描或改写模型最终 prose。

### `qf_sequence_analyzer_gate`

1. 源码真相仍是两条 shared-callee 局部边：`buildAnalysisIR -> gate.RunWith <- gate.Run`，不存在
   `buildAnalysisIR -> gate.Run`。最终 Mermaid 的箭头方向正确，16 项中间函数清单也未丢。
2. B1103 获部分生产正证：初始 Finalizer prompt 已发布两个精确 participant boundary rows，不再是教学
   缺席；但 copy-ready Mermaid/`edge_anchors_json` 与 `participant_boundaries_json` 是相邻却独立的
   两个复制载体。模型完整复制前者、遗漏后者，仍触发 1 次 participant hard reject（r699 为 2 次）。
3. 修补后模型把审计元数据扩写成读者可见“未证关系边界”，同时摘要仍称“完整调用顺序/通过
   gate.RunWith 汇入 gate.Run”，与 shared-callee 真相冲突。不能靠系统改写答案闭环；B1104 将同一 typed
   producer 的 anchors + boundaries 合成一个完整 block-sibling JSON 对象，模型仍自行决定可见图与结论。
4. Explorer 仍有 18 轮、13 次 midloop，属于后续效率/上下文精度观察项；本批不因一个 case 增加函数名、
   标题或最终答案关键词硬门。
