# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T13:25:19Z
- sweep_start_ts: 20260822-062517
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-062519 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 211s | 40 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000–2.020s 窗、四跳唤醒链、11.000ms 链上 IO 首因、三个 1.000ms 反转候选、实际占时/规则可消双账、链上业务下钻、邻近/背景隔离与完整 Trace 因果投影均保留；零成文拒绝。模型把 fscache 调用点扩写为“实际瓶颈”仍属带边界的软措辞观察，不由系统硬扫或改写。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-062519 | answer_regex,answer_contains | none | 296s | 36 | read=6,repo_map=0,list=1,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=4,unavail=0,prune=0 | partial | 最终正文正确覆盖 JsonPlugin、导入期注册、REGISTRY 查找、cls() 实例化、executor callback 与 MRO；普通关系列表最终保留四条精确边。但 Mermaid 被连续修补缩成单 participant+Note，几乎失去关系表达。首轮 live delta 的所有 failure 仅允许 remove/replace，系统仍讲解 attach；模型把互不配对的 failure_ref/addition_ref 合并为 attach 后被同一执行器拒绝，触发原子回滚及后续级联修补。B1346 候选 evidence 未自然触发：机械引用修复先消化了候选，因此仍只有单测正证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B1347-ATTACHCAPABILITYPROJECTION1/P1`：确定性系统 gap，不是模型随机波动。宽兼容 patch schema 的 action enum 不含 `attach`，本代 failures 也无 `allowed_actions:[attach]`，但共享 retry 教学仍主动介绍 attach；并置 failure/addition roster 又没有精确配对载体，诱导模型组合两个互不兼容 ref。
- 根修原则：只有当前 typed lease 与动态 schema 共同发布的精确双-ref branch 才讲解 attach；宽 schema 或无配对代际明确披露“不存在 attach 能力”，不得从相邻 JSON 行推断配对。系统仍不选择操作、关系、端点、标签、布局或答案结论。
- chain1 后续两轮空 anchor 拒绝是首个非法 attach 导致整批原子回滚后、模型没有重放已正确写出的 sibling replacement 的级联。先修 B1347 再复放；若首个原子 patch 能直接提交，则不另立针对该单例的自动代写修复。
- 最终图关系缩水另记为后续跨图族审计项：检查 assignment/data-flow/register/callback 等非 call typed recipe 是否完整进入 diagram-local candidates。不得按 Python/JsonPlugin 特例补边，也不得由系统替模型生成关系。
