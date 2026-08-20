# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T19:53:14Z
- sweep_start_ts: 20260820-125313
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-125314 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 312s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | 显式 2.000–2.020s 窗、Trace 因果投影、11ms 链上 IO 首席、三个 1ms runnable 调度席、主要占时/规则可消双轴和背景隔离完整，活动流无时间降级。模型明确披露目标直接阻塞者未建立，却仍称上游 IO 是“整条唤醒链延迟的核心驱动因素”，并在无对应竞争证据时建议绑同核/提优先级，B1253 再次复现。两次成文拒绝仅因模型把只允许 summary 的 trace_causal_claim_caliber 留在 section；schema/错误均已明确，当前按模型操作波动观察，不扩成正文硬门。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-125314 | answer_regex,answer_contains | none | 512s | 50 | read=17,repo_map=2,list=0,trace=0,source_lens=0 | midloop=13,inv=4/0,fin_reject=4,unavail=0,prune=1 | partial | 最终一张合法 sequenceDiagram 和一张列头完整的阶段表，Analyze→Explore→Extract→Finalize 主干、AnalysisIR/EvidenceItems/AnswerSymbols/AnswerDocumentV2 与 BusContext/Mutable 状态载体均可读；无降级稿。较 r784 的 1530s/20 rejects 降到 512s/4 rejects。第 5 轮 relation lease 建立后，四条无锚 BC 边由模型一次声明并被原子工具成功删除，B1254 生产转正；但混合 participant+relation 首拒只先建立 participant delta，第 3 轮提前做 relation edit 时仍报 exact-prior-anchor，下一轮才得到 relation lease。最终 BusContext 是孤立参与者，真实状态写入关系只在表/正文，不在图；记 B1257/B1258，不用系统补边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
