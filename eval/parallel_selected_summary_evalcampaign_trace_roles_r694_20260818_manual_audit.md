# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T16:43:11Z
- sweep_start_ts: 20260818-094311
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-094311 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 38 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Trace 因果投影、链上 #1 worker-200、有效归因/实测 runnable=8.300ms 与链累计=9.000ms 分账均正确，背景压力未晋升主因；但模型正文仍把窗外 1.010000..1.010020 的 app-100 runnable 写成“被唤醒后”行为，并声称与 worker 同 CPU、secondary rank=2，和所选窗口 1.000000..1.010000 及 typed reader roster（该行 unranked context）冲突。正文还泄漏 `priority_inversion_candidate`、`target_self_state`、`rank #1` 等内部控制词。B1092 生产闭环，B1091/B1093 仅 partial。无成文拒绝、无系统代写模型结论、无 active-stream 4ms 降级。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-094311 | answer_regex,answer_contains | none | 288s | 31 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | pass | 最终答案正确呈现 `buildAnalysisIR -> RunWith <- gate.Run` 扇入拓扑，Mermaid 可渲染，两条真边方向/证据均保留，未虚构从 buildAnalysisIR 到 gate.Run 的串联路径；B1090 的空 `diagram:{}` 本轮不再误拒。过程仍有 2 次关系修补：首稿 ordered_list 按 Submission Checklist 携 `claim_uses` 却未携同块 `edge_anchors`，第一次 patch 又把显示标签误作 node id，第二次才复制 typed skeleton。确认新的教学/合同同源 gap，不影响本轮最终正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner：2 PASS / 2；人工：1 pass / 1 fail。
- `B1090-EMPTYOPTIONALCOMPOSITENORMALIZE1`：production-positive，可收账。
- `B1092-TRACEVALUECOLUMNROLE1`：production-positive，可收账。
- `B1091-TRACECONTROLVOCABREADERSURFACE1`：production-partial；自然语言事实卡在场，但 raw typed 词仍从其他 Finalizer 上下文面进入正文。
- `B1093-SELECTEDWINDOWPOSTBOUNDARY1`：production-partial；边界事实卡在场，但模型仍消费窗外 observation 并把它写进所选窗正文。
- 新 gap：`B1094-RELATIONBLOCKANCHORTEACHING1`。同块关系所有权硬合同要求 `claim_uses + edge_anchors`，而 Submission Checklist 只要求前者；这会让遵循清单的首稿确定性被拒。修复应统一 schema/教学来源，不降低 typed 关系证据门，也不由系统补造关系。
