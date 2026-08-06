# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T03:37:37Z
- sweep_start_ts: 20260805-203735
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-203737 | log_regex,answer_regex | none | 37s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 三条只读命令一次完成，OS/CPU/内存/GPU 与完整 payload 一致，128 GiB 换算正确；零重试、零答案丢失。运行日志没有 structured-param/compat repair，但当前 operation 日志不保留 planner 原始两个数组字段，故只判“未观察到修复”，不虚称原始 JSON 数组形已被直接证明。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-203737 | answer_regex,answer_contains | none | 94s | 22 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 4 members 只发 4 个 support_refs，均对齐 StageBinding 责任行族，不再出现双批 8 refs；顺序、职责、图和 precedence edge 正确，零成文拒绝。但 StageAnalyze 从第 44 行 `{` 借邻行 anchor 通过 Tier-1，最终引用显示 `{`；其余行也仍以 scope=line 承载跨字段摘要。数量教学闭环，精确证据 span 未闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B158-SUPPREFMULTI1` 生产闭环：completion carrier 为 4 members / 4 positional refs，顺序一致，无第二批裸 refs。
- `EVAL-B158-OPJSONARRAY1` 本轮零可见 repair 且答案正确；因 operation 计划日志缺 raw-shape witness，状态为 `production-no-repair-observed`，不升级成“直接数组已证”。`EVAL-B158-OPJSONMETRIC1` 仍开放。
- 新确认 `EVAL-B159-TIER1LINE1=P1 evidence-coordinate`：Tier-1 在 `line_start±2` 找到精确 anchor 后只判 grounded、未把 LineStart 归一到命中行，导致相邻 `{` 成为可见引用。最优方案是以 grounder 已返回的精确 matched line 规范坐标和 snippet；不读 summary/答案 prose，不重写模型结论。
- `EVAL-B157-EVSPAN1` 仍为 partial：精确行归一只能修坐标，不能把 identity 行升级成跨字段职责证明；后续维持 typed span/soft-teaching 路线，禁止摘要相似度硬门。
