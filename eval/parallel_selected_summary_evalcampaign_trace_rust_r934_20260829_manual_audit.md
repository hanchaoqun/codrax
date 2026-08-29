# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T09:20:38Z
- sweep_start_ts: 20260829-022038
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260829-022038 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 190s | 39 | read=5,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 34579.472865..34579.587805 窗、四跳唤醒链、链上主席、实际占时/规则可消双轴、确定性工作与业务下钻均在；完整 Trace 因果投影和自动补齐保留，帧因果未证边界明确，邻近/背景没有升为根因。target_cpu 1697 条全 0 被诚实降级，未拿退化字段猜放置。 |
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260829-022038 | answer_regex | none | 455s | 41 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=6,unavail=0,prune=0 | partial | 列表与图正确表达 run 的 sibling fan-out：run→collect_files→walk，另一路 run→index_file→Matcher::is_match；结尾却误串成 walk→index_file 的连续链，并把 6 个节点/5 条边写成“6 个跳跃”。typed 图和 evidence 上下文充分，属模型遵循波动；但前 3 次关系修补被系统 alias/candidate 合同冲突反复拒绝，是 B1448 精确系统 gap，已由 2ef3fdd96 根修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit decision

- `B1448-RELATIONREPAIRALIASPARITY1`：confirmed、implemented、full-suite/build green、pushed；待后续异构生产回放自然触发。
- 修复边界：只把当前图中已声明、且能由 accepted typed evidence 唯一解析到候选端点的 node id 加入同一 `allowed_additions`；不读取用户请求、Mermaid message、模型推理或答案正文，不选择关系、方向、标签、布局或结论。多个 qualified alias 同时匹配一个短端点时继续 fail-closed。
- Rust 终稿的分支线性化与 6/5 算术矛盾没有据此新增 prose 硬门或系统改写；继续作为跨模型、跨语言遵循波动观察项。
