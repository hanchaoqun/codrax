# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T15:21:14Z
- sweep_start_ts: 20260808-082112
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-082114 | answer_regex,answer_contains | none | 213s | 34 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer 对明确要求“数据流”的 architecture 请求漏发 `predicate_axis=flow`，严格关系 owner 车道未启动；答案继而把 `applyStageOutput` 说成 Finalizer 调用、把 Explorer 说成唯一 Mutable 写者，并画出多条无 typed owner 的读写箭头。runner 的字符串/图形 oracle 未覆盖机制正确性。 |
| 2 | qf_diagram_pipeline | FAIL | eval/results/qf_diagram_pipeline-20260808-082114 | answer_regex,answer_contains | none | 829s | 46 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=20,unavail=1,prune=0 | fail | typed flow 正确进入严格车道，但系统没有把已读 `AllMainStages` 返回列表接成 adjacent precedence 权威；validator 在 missing owner 与 unproven precedence 间往返 20 次。降级保留了四阶段图且未再泄漏 repair `<think>`，但 829 秒与失败说明不可接受。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. S37cg 的降级安全修复已获得生产 witness：diagram 案 20 次失败后只恢复上一版结构化草稿，最终答案没有“模型最后一轮原文”、失败 repair `<think>` 或 schema 猜测；模型失败和降级状态仍明确披露。`EVAL-B362` 可关闭。
2. S37cf 的 pre-emit precedence 入口在单测成立，生产却没有候选 range：`AllMainStages` 的 citable definition 只锚定函数签名一行，明确有序的 returned slice 位于后续 7 行。临时复核只消费 `LineEnd > LineStart` 的 evidence，导致 exact source carrier 已读却不可达。立 `EVAL-B363-PRECEDENCERANGEHANDOFF1=P0/REDLINE`：只能从 citable definition/initializer 的 exact line 向后扩展已读、最多 64 行的候选，再交同一个 grounder；不得扫描请求/答案或写回 ledger。
3. Explorer 自行发出的 precedence 还暴露了相反的权限过宽：它把 `const (...)` 中 `StageAnalyze/Explore/Extract/Finalize` 的声明书写顺序当成 pipeline DAG 顺序，其中首条甚至铸成 `StageAnalyze -> StageFinalize`。现有“同 delimiter + top-level comma”仍会把 const/enum/type namespace 当有序值。立 `EVAL-B364-PRECEDENCEDECLARATIONAUTH1=P0/HIGH`：权威载体必须是 returned/bound ordered value（array/slice/tuple/list/rule chain）；声明组、参数表、任意 sibling statements 和 comments 均 fail-closed。
4. diagram 案先后尝试 precedence、call、assignment、删 metadata；每条修复都被另一合同拒绝，最终 20 次 reject。根因是 typed fact 的生产/消费断链，而非模型 JSON 或 Mermaid 格式。继续加 JSON/箭头教学只会加重模型心智。
5. logic 案的零 reject 不是好信号：其 analyzer payload 明确遗漏 `predicate_axis`，尽管请求字面要求“组件之间的数据流”，于是同一事实 topology 没有进入 S37ce 的 owner gate。立 `EVAL-B365-EXPLICITFLOWAXISOMISSION1=P1/HIGH`；修复应在 schema/analysis 合同层处理 typed request shape，不能靠原始请求或答案关键词硬门。
6. logic 案上下文支持了组件存在，却没有把“谁写 Mutable、谁调用 applyStageOutput、阶段输出由谁合并”以精确 owner/edge 形式交给 finalizer，模型把 orchestrator 责任归给 Finalizer。立 `EVAL-B366-MECHANISMOWNERCONTEXT1=P1/HIGH`，优先从 typed call/assignment/return evidence 提升关系可见性，不由系统代写结论。
7. 两案均未运行 Trace。所有 source-flow 修复继续在 `QFRootCauseTrace` 前 fail-open/提前排除；显式窗、系统补采、因果投影、唤醒链、根因排序和窗内可消除量保持原路径。Trace 根因只允许 typed on-chain 席，链外邻近/背景只能作为额外排查方向。

Batch result: runner `1/2 PASS`; human `0/2`。B362 生产关闭；B363/B364 为下一立即施工批，B365/B366 为随后独立批。
