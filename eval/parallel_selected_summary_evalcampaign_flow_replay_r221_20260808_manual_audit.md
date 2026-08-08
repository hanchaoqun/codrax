# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T18:08:20Z
- sweep_start_ts: 20260808-110818
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | FAIL | eval/results/qf_diagram_pipeline-20260808-110820 | answer_regex,answer_contains,mermaid_edge_count | none | 129s | 26 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 正文对四阶段名称/职责基本准确，但最终 Mermaid 只有四个孤立节点，无法表达用户明确要求的 pipeline 顺序；新 edge oracle 正确报 `0<1`。Explorer 已发 `AllMainStages` definition，却只看到了签名行，未把 returned slice 读入 grounding line map；pre-emit precedence 恢复因此无法证明三条相邻边。validator 两次拒绝均正确，模型随后删边。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260808-110820 | answer_regex,answer_contains,mermaid_edge_count | none | 403s | 37 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | 最终图同样零边；正文把若干局部事实扩成完整组件流，例如把 `graphState` 说成驱动 Analyzer、把 Extractor/Finalizer 的全部产物与 Mutable 关系说成已证。validator 正确拒绝无证 assignment/observe/precedence。B372 的旧短名错拒未复现，但 authority 仍把 21 条不相关自动 dataflow 路径加冕为 `typed_flow_paths_present`：宽泛 AnswerSupportPlan 绕过了上一批 no-plan 收窄。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

1. `EVAL-B372-FLOWENDPOINTIDENTITY1`：实现与回归套件已通过；R221 未再出现 R220 的同一组 short↔qualified call 错拒，但本轮模型没有重发完全相同的三边图，生产闭环证据仍记为 partial，不虚报完全关闭。
2. `EVAL-B373-FLOWAUTHNOSUPPORT1`：no-support-plan 臂生效于 diagram 案，authority 为 `listed_edges_only`；logic 案存在由自动扩展关系填充的宽泛 AnswerSupportPlan，21 条无关路径重新获得 principal ordered authority。根因不是 rank 分值，而是确权边界选择了整份 support plan，需与本轮 model-authored grounded operation rows 取交集。
3. `EVAL-B374-DIAGRAMORACLEEDGES1`：生产见证闭环。两个历史 false PASS 均被 `mermaid_edges:0<1` 诚实拦截；该 oracle 只在显式关系图 eval 使用，没有接入生产答案硬门。
4. 新立 `EVAL-B375-FLOWCARRIERSCOPE1=P1/HIGH`：flow completion 的“至少一条 operation”只证明某处存在 operation，不能保证请求所需的 producer/transfer/consumer 或 ordered carrier 已读。diagram 案的 stage→agent assignment 足以让 completion 通过，但 `AllMainStages` returned slice 只读到签名，最终顺序关系无可复验。先补 typed 当前调查 operation 作用域与精确补读；不得用实体相似度、case 名、文件名或答案原文做硬门。
5. 两案没有 malformed JSON；diagram 首稿把 `blocks` 作为 JSON 字符串发出，被既有 flat tolerance 修复，未丢答案。没有系统替写/删除模型结论。所有关系拒绝来自 schema-valid edge anchors 与 typed evidence mismatch。
6. 两案均未调用 Trace。后续修复继续排除 `QFRootCauseTrace`；显式时间窗、自动补采、因果投影、唤醒链、根因排序、窗内可消除量和真实占时/规则可消双轴保持不变。Trace 主因只允许 typed on-chain 席，邻近/背景只能作额外排查方向。
