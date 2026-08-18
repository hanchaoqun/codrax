# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T15:38:52Z
- sweep_start_ts: 20260818-083851
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-083852 | answer_regex,answer_contains | none | 158s | 28 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 非 stage 对照正向：0 finalizer reject，合法 sequenceDiagram 保留两条真实同向 call，正确说明 `buildAnalysisIR` 与 `gate.Run` 没有直连而分别调用 `gate.RunWith`，证明 B1085 未误收窄普通调用图。人工扣分：正文和尾注把 typed 内部枚举 `shared_callee_boundary` 原样展示给用户，且末句“gate.Run 作为 gate.Run 的无参变体”措辞自指。与客户已报 `bounded_window_candidate` 同类，记 B1088。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-083852 | answer_regex,answer_contains | none | 688s | 42 | read=27,repo_map=2,list=0,trace=0,source_lens=1 | midloop=17,inv=9/2,fin_reject=3,unavail=0,prune=3 | fail | B1085 生产正证：Analyzer roster 仅 `analyze/finalizer`；prompt 发出 3-edge principal spine，首拒后明确走 complete principal-only repair；终图由 r691 的 17 个 participant 收敛为 4 个业务 stage，未混入 helper calls，read 51→27、ctx 54%→42%、reject 4→3。人工仍 fail：正文/表格把 `dataflow.Analyze` 混入 StageAnalyze 主流程，把 `emit_investigation_complete` 错归 StageExtract，状态载体列称 MutableState 写 AnalysisIR，结论还重复 `StageExplore`；表头退化为“列 1…列 4”且正文重复。关系图根修有效，但证据角色→模型总结仍有 B1089。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

1. `B1085-STAGEENDPOINTSPANCOMPLETENESS1` 获生产正证：typed endpoint span 被标为 complete principal relation spine，repair payload 不再含 supporting helper call 全池；主图显著收敛且正确。
2. 三轮拒绝均来自模型首稿/补丁继续保留 principal recipe 外的无证 precedence/return/reply 边，而不是系统继续发错全池。最终第四稿删除额外边并保留 3 条正确 stage precedence；先不降低 gate。
3. 新确认 `B1088-INTERNALENUMCUSTOMERSURFACE1`：`shared_callee_boundary` 与客户此前看到的 `bounded_window_candidate` 都是 typed 枚举直接泄漏。应在生产者处提供 locale-aware reader label/description 并把 raw enum 明确降为 metadata；不得扫描或替换终稿字符串。
4. 新确认 `B1089-STAGEEVIDENCEROLETOFINAL1`：stage authority 已给出 canonical responsibility，终稿仍把同名 `internal/analysis/dataflow.Analyze` 支撑事实晋升为 StageAnalyze 主过程，并误分配工具阶段。应审计 EvidenceItem 的 request-spine/supporting role 是否以结构字段进入 finalizer，不能靠禁止函数名或系统改答案。
5. 两案均无固定 4ms 降级、空答案或系统代写图。Trace 能力未进入本批且保持不变。
