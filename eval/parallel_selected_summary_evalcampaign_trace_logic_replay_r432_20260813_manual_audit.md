# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T11:25:48Z
- sweep_start_ts: 20260813-042546
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-042548 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 205s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 五次 typed 查询均保持用户显式窗；链上 running 65.912ms、D-state 36.757ms、反转/调度/算力/IO 与业务 span 双轴齐全。邻近/背景不加冕，不跨席相加，系统补齐 wakeup 分支与投影明细。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-042548 | answer_regex,answer_contains,mermaid_edge_count | none | 301s | 39 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=6,unavail=0,prune=1 | partial | 最终 Mermaid 语法合法且没有降级，阶段 precedence 保留，BusContext/Mutable 未证边界诚实。仍有 6 次成文拒绝：patch schema 把 replace/add block 仅暴露为裸 object，模型在重试中反复漏掉 edge identity 或把技术端点改成组件名，最终删掉全部局部调用关系才过门。确认 B721。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Conclusion

1. `20260813-035440.459-11882.md` 的非法 `] -->|` 出厂问题已闭环：r432 的正常答案发出合法 Mermaid；B720 的降级专测保证失败草稿保留正文与原始图源码，但把非法图改为带 `# ⚠` 原因的 `text` fence。正常答案不经该门。
2. B719 的 scope 分权生效：局部 evaluator→Mutable 调用不再被 participant gate 强迫冒充完整请求数据流；最后可诚实保留 `BusContext`、`Mutable` 的 requested-relation unproven boundary。
3. 不能仅凭 runner PASS 收账关系能力。新确认 `B721-PATCHBLOCKSCHEMAPARITY1`（P0）：full emit 的 block schema 完整暴露 `edge_anchors.from_identity/to_identity` 与 boundary，patch 的 replace/add item 却是裸 object。模型从结构化低心智合同退回依赖长提示文本，造成 6 次重试和关系缩水。
4. 泛化根修让 patch delta envelope 保持不变，但 replace_blocks/add_blocks 的 item 直接复用本 dispatch 的 full-emit projected block item。图、claim、Trace、source inventory 等未来新增字段也自动同源；不扫描用户/答案原文，不代画图，不降低 evidence gate。
5. active stream 没有因 4ms 降级；本轮 205s/301s 持续活跃。Trace lane 零改动并获生产回归正证。
