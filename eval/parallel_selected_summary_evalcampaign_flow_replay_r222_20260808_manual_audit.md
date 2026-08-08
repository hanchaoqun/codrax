# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T18:59:33Z
- sweep_start_ts: 20260808-115930
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-115933 | answer_regex,answer_contains,mermaid_edge_count | none | 299s | 33 | read=9,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 最终四阶段顺序和职责正确，且 Mermaid 有 3 条 `precedence` 边；但 pre-emit 依据严格已读 `AllMainStages` 相邻值接受这些边，post-finalizer 又因看不到该局部派生权威连续拒绝同一稿，FRCAP 最后携一条 hard violation 出厂。可见正文还在“系统保留内容/第一稿答案”下完整重复一次并声称仍需验证。确认 B376 pre-emit/post-contract typed authority 断裂，B369 相同首稿重复展示复现。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-115933 | answer_regex,answer_contains,mermaid_edge_count | none | 435s | 39 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=5,unavail=0,prune=2 | fail | Analyzer 正确要求 flow 轴 architecture 图；Explorer 最终只有 1 条 call 与 1 条 assignment。Finalizer 的完整数据流因缺 typed relation 被 5 次正确拒绝，最后只保留 `runAnalyzePhase --> dispatchStage`，Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 其余节点全部孤立，正文仍扩写为完整组件流。现有 oracle 只要求至少一条边而误签 PASS。确认 B377 required flow 图缺少 typed 请求关系覆盖义务；不能靠实体相似度、答案关键词或任意边数修补。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `2/2 PASS`; human full correctness: `0/2` (`partial + fail`).
- `required_diagram_edge_absent` 本轮没有获得生产触发 witness：两份最终图都至少有一条结构边；其精确零边分支仍由单测与全仓套件覆盖。
- 新确认：`EVAL-B376-PREEMITPOSTCONTRACTPARITY1`、`EVAL-B377-RELATIONDIAGRAMCOVERAGE1`；重现：`EVAL-B369-FIRSTDRAFTREPEAT1`。
- 两案均无 malformed JSON、无 Trace 查询、无用户/模型原文关键词硬门、无系统替写模型结论。后续修复必须继续排除 `QFRootCauseTrace`，保持显式窗因果投影、自动补采和链上根因权限不变。
