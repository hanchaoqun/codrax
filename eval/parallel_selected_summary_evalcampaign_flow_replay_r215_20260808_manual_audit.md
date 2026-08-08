# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T14:34:00Z
- sweep_start_ts: 20260808-073359
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-073400 | answer_regex,answer_contains | none | 183s | 28 | read=6,repo_map=4,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Flow handoff cap 生效，但系统的 copy-ready recovery 把用户要求的主 pipeline 图替换成 `NewFinalizerAgent -> NewBaseAgent` 无关 helper 图；正文仍把字段/容器存在扩写成广义数据流。runner PASS 不能覆盖主可视化缺失。 |
| 2 | qf_diagram_pipeline | FAIL | eval/results/qf_diagram_pipeline-20260808-073400 | answer_regex,answer_contains | none | 345s | 33 | read=2,repo_map=3,list=0,trace=0,source_lens=2 | midloop=5,inv=1/0,fin_reject=8,unavail=0,prune=0 | fail | 模型先画出正确四阶段顺序；Explorer 未铸 precedence rows。随后系统又把 deterministic `source:Symbol` call row 与裸符号端点判为不等价，8 次同类 reject 后降级，最终泄漏 `<think>` 原文且图仅剩一条 helper call。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `S37ce` 的规模控制在生产命中。logic 案相关 flow rows 从 `24,689` 压到 `128` 且明确 `complete=false`；diagram 案 `86/86 complete=true`。因此本轮失败不再是无界 handoff，而是关系身份和恢复相关性问题。
2. logic 案首稿仍包含多条无同向 typed owner 的箭头；严格校验拒绝是正确的。但 required-diagram recovery 随后把 evidence pool 中任意一条可复制关系 `NewFinalizerAgent -> NewBaseAgent` 当成整图替代品，主问题要求的 Analyzer/Explorer/Extractor/Finalizer/BusContext 流程图被删除。系统虽未直接改写 prose，却通过强恢复提示实质挤掉模型的主图，属于 `EVAL-B360-FLOWRECOVERYRELEVANCE1=P0/REDLINE`。
3. diagram 案 typed support 明确含 `ev-d462c467f52d224f`: `internal/types/stage_binding.go:ReadModeMainStageBindings -> AllMainStages`，producer 是 deterministic dataflow lowerer；validator 却用稳定 `file:Symbol` identity 与 Mermaid 裸 `Symbol` 做字面比较，产生 `call_edge_unproven`。这是 `EVAL-B359-FLOWSOURCEIDENTITY1=P0/HIGH`，不是模型波动或 JSON 畸形。
4. `AllMainStages` 的已读定义区间完整包含 `StageAnalyze, StageExplore, StageExtract, StageFinalize`，但 Explorer 只收到 authoring 教学，没有稳定铸造 pairwise precedence。硬合同依赖模型再发一遍本可由 exact source range 验证的关系，形成不必要心智和重试。与此同时 S37ce 的 grounder 只验证区间内位置先后，任意函数内两条独立语句也可能被误铸为 precedence。合并为 `EVAL-B361-PRECEDENCEPROOFCARRIER1=P1/HIGH`：必须收窄为同一显式逗号分隔载体，并允许 pre-emit 仅针对模型已声明的 typed precedence 在既有 grounded range 上复核；不写回答案或 evidence ledger。
5. 8 次 reject 后的 degraded recovery 把最后一轮 `<think>` 和错误修复猜测完整附到客户答案。它确实披露“结构化成文未完成”，但不应把内部推理当作有用字符串。立 `EVAL-B362-DEGRADEDTHINKDISCLOSURE1=P0/REDLINE`：降级应优先保留最后一个结构化草稿/可验证散文，并给简短模型格式/校验失败说明；thinking 标签内容必须剥离，不能做事实来源。
6. 两案均未运行 Trace。源码图 hard gate 与临时 precedence 复核必须继续显式排除 `QFRootCauseTrace`；显式窗、自动补采、因果投影、根因排序、唤醒链及窗内可消除量保持独立。Trace 主因只可来自 typed on-chain 席，链外邻近/背景只能作为额外排查方向。

Batch result: runner `1/2 PASS`; human `0/2`。`B357` 生产闭环；`B356/B358` 行为接通但暴露 B359–B361；B362 独立留作下一高优先级降级安全批。
