# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T20:56:47Z
- sweep_start_ts: 20260808-135646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-135647 | answer_regex,answer_contains,mermaid_edge_count | none | 438s | 31 | read=7,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=3/0,fin_reject=6,unavail=0,prune=0 | fail | 正文说 4 个主 stage 构成 Analyze→Explore→Extract→Finalize 的单向顺序 DAG，但最终图只画出 runAnalyzePhase→dispatchStage、runReadSchedulerLoop→dispatchExploreWindow 和 runReadSchedulerLoop→StageExtract 三个局部事实，四阶段主链在图中缺席。6 次成文拒绝主要来自模型先画无证 Start/End、dispatch→stage 或无 owner 的 stage 顺序边，再逐步删图。多段标签错误已不再出现，B379 的 typed endpoint 唯一消歧在生产生效；runner 仅因“至少一条 Mermaid 边”而误判 PASS。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-135647 | answer_regex,answer_contains,mermaid_edge_count | none | 680s | 44 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=8,unavail=0,prune=0 | fail | Analyzer 正确提取 analyzer/explorer/extractor/finalizer/Mutable/BusContext 六个参与者，soft no-incident checklist 也准确提示缺边；模型仍把局部定义和 assignment 扩写为完整 pipeline 结论。最终图只剩 explorerEvaluator→MutableState 一条 assignment，未表达所求组件关系。8 次成文拒绝后靠删边通过，runner 的单边 oracle 再次假阳性；这证明 soft PrimaryEntities 提示不足，但 PrimaryEntities 本身仍是嘈声 shortlist，不能直接升级为硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS; human audit: 0/2. 两案都满足了“存在 Mermaid 边”的弱 oracle，却没有完成用户要求的主关系视图。
- `EVAL-B379-DIAGRAMMULTISURFACEIDENTITY1` 获得生产闭环：校验错误始终使用 `StageAnalyze`、`StageExplore` 等 canonical endpoint，没有再把 `analyze` 等显示值当成 typed identity。
- `EVAL-B377-FLOWPARTICIPANTROLE1` 从 soft-layer witness 升级为明确架构缺口：系统需要 analyzer-authored、schema-valid 的参与者—角色—接入义务载体，区分必须接入主关系、独立/容器/上下文参与者和允许披露 unproven 的边界。不能把 `PrimaryEntities`、用户/模型原文、标签关键词、实体相似度或“边数”改造成生产硬门。
- eval 侧另有 typed oracle 缺口：`mermaid_edge_count>=1` 只能证明图非空，不能证明 case 声明的关系参与者已接入。后续 oracle 只能消费 case 自身的结构化期望，不能反向污染 production 判定。
- 本批没有 malformed JSON、没有系统重写/删除模型结论，也没有 Trace 查询。后续载体必须显式排除 runtime/root-cause Trace；Trace 显式窗、系统自动补采、因果投影、唤醒链、根因排序、窗内可消除量及真实占时/规则可消双维保持不变，主因仍只能来自 typed on-chain 席，邻近区域和背景只作额外排查方向。
