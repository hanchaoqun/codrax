# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T14:38:45Z
- sweep_start_ts: 20260807-073844
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260807-073846 | write_plan,write_patch_oracle | none | 52s | 20 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确一行 `retrun`→`return`；计划、补丁和验证域一致，零拒绝/repair，read-mode call-chain 教学未污染 write 计划。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-073846 | answer_regex,answer_contains | none | 309s | 29 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | S37x 边界导航被消费：模型经 scoped grep 读取并自行发射 `gate.Run -> RunWith @ gate.go:135`，没有系统铸证据；但最终 prose 把 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> RunWith` 两条汇合边说成“从 buildAnalysisIR 到 gate.Run 的路径经过 gate.RunWith”，与同页 no-path 披露和 Mermaid 自相矛盾。首稿还把 `blocks` 错编码为 JSON 字符串，既有无损 recovery 正常工作；随后 exact-anchor 门不承认 typed edge label 的端点，造成两次 patch 和冗余端点行。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r169 审计结论

- `EVAL-B276-ENDPOINTBOUNDARYFRONTIER1` 已获得 production witness：日志和工具轨迹证明 exact sink boundary advisory 到达 Explorer；模型使用 scoped targeted grep 检查 `gate.go:135`，随后以自己的 `emit_evidence` 铸造关系行。系统未创建关系、未改写结论。
- QF 的 runner PASS 不能覆盖人工语义失败。typed capsule 已有 `call_graph_status=parallel_convergence` 与两条方向正确的边，但拓扑展示仍不足以阻止模型把共同下游误串成 source→sink。需要从同一 typed capsule提供“双入箭头”软拓扑形，不扫描最终 prose、不接管结论。
- 两次 finalizer patch 的可泛化根因是 structured identity 合同过窄：带 `claim_form=call_edge` 的 `A -> B` 标签仍不能承载 A/B exact endpoint，迫使模型添加重复 standalone 行。修复应只解析 typed block 的单边结构标签；自由 Text/summary、无 typed claim、多跳标签、C/C++ `receiver->method` 均不得参与。
- 首次 `blocks` 字符串编码属于模型波动，已有 canonical JSON 教学明确 native array、既有 recovery 无损恢复并披露。没有发现教学/schema 自冲突，本批不复制第二套 JSON 规则。
- C++ write 案证明该 read/call-chain 改动没有污染精确写模式。显式窗 Trace、因果投影、自动补齐、根因排序、唤醒链、窗内可消除量和双维根因分析未进入本轮路径。
