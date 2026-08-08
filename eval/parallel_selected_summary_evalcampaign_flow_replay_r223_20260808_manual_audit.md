# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T19:48:19Z
- sweep_start_ts: 20260808-124818
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-124819 | answer_regex,answer_contains,mermaid_edge_count | none | 141s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 四个主 stage 的顺序、职责、引用与 3 条 `precedence` 边均正确。pre-emit 与 post-contract 对相同 `AllMainStages` carrier 保持一致，B376 获得生产闭环。唯一 reject 来自模型额外给两个条件 pre-stage 画无证 `precedence` 边，模型只删除这两条而保留主图；最终答案没有“系统保留内容/第一稿答案”整页重复，B369 获得生产闭环。soft participant checklist 正确标出四个主 stage incident，并把 pre-stage 保持 no-incident，没有自动造桥。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260808-124819 | answer_regex,answer_contains,mermaid_edge_count | none | 393s | 36 | read=14,repo_map=4,list=0,trace=0,source_lens=0 | fail | Explorer 虽读了 14 个文件并得到两个局部 assignment segment，但没有形成 Analyzer/Explorer/Extractor/Finalizer 与 Mutable/BusContext 的已证关系载体；soft checklist 诚实显示全部 named participants 无 incident relation。Finalizer 先为完整数据流画 7 条无 anchor 边，被拒后改成 3 条无证 precedence，再照另一条合同的“presentation-only 无 typed relation”措辞删除 anchors，立刻被 typed flow body-ownership 合同再次拒绝；第 4 轮只能删光箭头，最终图 0 边，runner 诚实 FAIL。确认 `EVAL-B378-FLOWPRESENTATIONESCAPECONFLICT1=P0/REDLINE`：同一 typed flow 图同时被教导可删 metadata、又被硬门要求 metadata。正文仍把局部事实扩写成完整组件流，B377 的 soft 层有效披露但不能保证模型遵循，硬 participant carrier 仍未获授权。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `1/2 PASS`; human full correctness: `1/2` (`pass + fail`).
- B376 生产闭环：相同 precedence 稿件不再出现 pre-emit 通过、post-contract 拒绝；post-contract 只有 soft advisory。
- B369 生产闭环：两份最终可见答案均没有同 carrier 的“第一稿答案”整页重复；日志中的“第一稿答案”只是交互式首稿展示，不是最终附件。
- B377 soft 层按设计生效：只披露 incident/no-incident，不拒绝、不补边、不替模型结论；logic 案证明它尚不能替代真正的 typed participant-role carrier。
- 新 P0 是 typed-flow 合同自冲突，不是模型波动、JSON 畸形或 Mermaid parser 问题；3 次成文拒绝与 393 秒延迟由同一矛盾放大。
- 两案无 malformed JSON、无用户/模型原文关键词硬门、无系统替写答案、无 Trace 查询。后续修复继续排除 `QFRootCauseTrace`，保持显式窗因果投影、自动补采和链上根因权限；链外邻近/背景仍只能作为额外排查方向。
