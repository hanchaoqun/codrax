# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T16:38:01Z
- sweep_start_ts: 20260822-093759
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260822-093801 | write_apply,write_patch_oracle,answer_contains | none | 106s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只把 `main.c` 的 `retrun buf;` 改为 `return buf;`；计划、应用、fingerprint 与 post-apply verify 闭合，`make test` 真实编译并运行两次均通过。验证生成的未跟踪二进制 `main` 被如实披露，未提交或伪装为源码改动。Analyzer 首次把 `is_dimensioned_answer=true` 与空 dimensions 同时提交，系统以单一精确合同拒绝，第二次即纠正；这是可恢复模型表单错误，不是互斥教学/执行合同。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260822-093801 | answer_regex,answer_contains,mermaid_edge_count | none | 352s | 35 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=6/0,fin_reject=1,unavail=1,prune=0 | partial | 正文对四阶段责任和三条 typed precedence 基本准确，图也诚实保留 `Mutable` 的未证边界；但最低关键字/边数 oracle 掩盖了图层缺口。第一稿同时声明 `BC["BusContext"]` 与同 ID/同标题的 `subgraph BC["BusContext"]`；typed participant 修补随后强引导使用 canonical `BusContext` 作为新端点，终图又隐式创建第二个 BusContext 节点，只接到孤立的 `BuildAgentContext`。共享状态与阶段仍未形成用户要求的数据流，且不同 Mermaid 查看器会对 node/subgraph ID 冲突产生拒绝或不一致渲染。确认 B1353，非单纯模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r868 结论

- runner：2/2 PASS；人工：write pass，read partial。
- read 的一次 finalizer reject 不是“缺证关系被系统硬画”，而是系统在已有 `BC` 可见参与者后仍要求模型另用 `BusContext` 端点，制造同一业务身份的第二节点；这是 typed 修补提示与模型既有可见拓扑未对齐。
- Explorer 已读取 `internal/context/builder.go:59` 并产出 `bus.Mutable -> AgentContext.Mutable` 的局部 typed 值传递，但该候选在完整 requested relation 未闭合时保持 local-only，最终模型没有选择展示。不能把它强制升级为端到端关系；本轮只记人工完整性 partial，不用答案关键词硬门。
- B1353 的泛化修复边界：Mermaid 源码仅在“唯一同 ID、同可见名称、独立节点无边”时移除冗余节点并保留 subgraph；typed retry 同时发布模型已写的精确 node/subgraph ID 供复用。系统不选择关系、方向、标签或布局；有边、异名、重复/歧义 carrier 一律不自动处理。
