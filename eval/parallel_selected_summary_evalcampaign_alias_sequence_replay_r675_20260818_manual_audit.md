# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T09:01:52Z
- sweep_start_ts: 20260818-020149
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-020152 | answer_regex,answer_contains | none | 112s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 七条 selected-definition body call 继续完整进入证据池，最终答案覆盖 Windows/macOS/POSIX API 与 `cmd_sleep`；本轮模型未画图、也未发 standalone relation，因此没有内部 alias 泄漏但不能单独验证修复分支。新见 B1062：13 条 citation pool 最终只保留 4 条，多 API 行大量引用定义首行而非实际调用行，Windows 行无来源，`handlers.c:38` 的结束时间又被修到 `handlers.c:34`，属于多操作机制的引用覆盖/修补完整性缺口。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-020152 | answer_regex,answer_contains | none | 351s | 37 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=6/0,fin_reject=1,unavail=0,prune=0 | pass | 最终时序图准确显示 `buildAnalysisIR -> RunWith <- gate.Run`，不虚构到 `gate.Run` 的有向路径；图外关系虽然 carrier 使用 `buildIR/RW/GR`，渲染结果稳定为 `buildAnalysisIR → RunWith` 与 `gate.Run → RunWith`，B1060 获生产正证。一次 finalizer 拒绝只要求把两条 principal edge 与 15 条 supporting call 分层，修补保留图和关系。探索 25 轮、completion 6 次暴露 B1061：限定调用名被 compact-relation 解析错误套用文件轴，精确同源码行仍被拒。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1060 production closure

- `qf_sequence_analyzer_gate` 的 Mermaid declaration 使用 `buildIR/RW/GR` 作为内部 alias，节点可见标签为
  `buildAnalysisIR/RunWith/gate.Run`；同一答案的 standalone edge carrier 仍携 alias。
- renderer 只依赖同一文档内唯一、同向、可见的真实边，将图外显示解析为模型写下的可见标签。最终答案未出现
  `buildIR → RW` 或 `GR → RW`，也没有修改 endpoint identity、图体、关系类型或模型结论。
- 因而 `B1060-STANDALONERELATIONALIASDISPLAY1` 可由 pending replay 更新为 production closed。

### B1061 qualified code identity versus compact relation

- Explorer 已在 `internal/agent/analyzer.go:2323` 和 `:2530` 读到
  `normalizer.Normalize()`、`compiler.Compile(...)` 的精确调用行，却被 completion 多次判 support-ref 不兼容。
- 根因不是证据不足：`AnswerAggregateMemberRelationParts` 将 `normalizer.Normalize` 拆成关系两端，随后
  `aggregateSupportLocationCompatibleWithMember` 错误要求 caller 文件路径包含 `normalizer`。
- 通用修向只能在“完整 language-qualified identity 出现在同一 exact grounded snippet/read line”时绕过
  relation file-axis；不得用短尾、近邻行、请求/答案正文，也不得把显式 `A -> B` 关系当限定名。

### B1062 citation coverage observation

- C 案的机制事实本身完整，缺口发生在 citation selection/repair 层：多项 API 和多次调用被压缩为少量定义行，
  甚至出现正文点名 `handlers.c:38` 而来源落到 `:34` 的错位。
- 该问题与 B1061 的 completion admission 不同，应独立立案；后续方案须维护模型事实与精确来源的对应关系，
  不能由系统改写事实句或简单强制“每行一个引用”。

### Red-line review

- 本轮无 Trace 代码改动；显式时间窗、Trace 因果投影、自动补齐和链上-only 主因均不受影响。
- 未观察活跃流因固定 4ms 未形成完整答案而降级；两案均由模型最终成文，系统只校验/渲染结构，没有接管结论。
