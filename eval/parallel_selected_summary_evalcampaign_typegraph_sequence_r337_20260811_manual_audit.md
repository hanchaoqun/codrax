# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T21:12:49Z
- sweep_start_ts: 20260811-141248
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-141249 | answer_regex,answer_contains | none | 271s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | B569 已生效：classDiagram 的 12 条 realization 边均被解析为 canonical implementer -> interface，模型提交的 exact anchors 方向正确。后置 orchestrator 校验却没有消费 B568 exact provider bridge，把同一答案误报为 12 条 type_relation_edge_unproven，触发一次无效修补；恢复层最终保留正确第一稿，但追加了“检查仍需补充验证”的系统降级说明。立 B571/P1-high：前后置关系校验必须共享同一 exact typed evidence 投影。活动模型流 271s 正常完成，没有固定四分钟降级。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260811-141249 | answer_regex,answer_contains | none | 827s | 42 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=18,inv=18/1,fin_reject=0,unavail=0,prune=0 | pass | 最终答案正确区分 buildAnalysisIR -> gate.RunWith 与 gate.Run -> gate.RunWith 的 parallel convergence，没有伪造 buildAnalysisIR -> gate.Run；时序图方向和 typed anchors 一致。效率严重异常：no_directed_path 端点存在性诊断在 gate.Run definition 已发射后仍重复，随后又要求模型手工转交两条已读 parser call relation，形成 18 次中循环、827s。立 B572/P1：收敛精确端点诊断与 parser relation typed handoff，不能放松关系真值门或让系统代选图边。活动流全程有真实模型进展，超过四分钟仍等待原模型完成，无超时降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 2/2，人工 1/2；字符串 oracle 再次没有发现第一案的后置证据消费断桥。
- B571 是确定性双消费者不一致，不是模型波动：pre-emit 已把 exact provider rows 投影为 citable type-relation evidence，post-finalizer 仍只读原始 evidence slice。
- B572 的最终事实正确，但 827 秒和 18 次中循环不应接受为正常成本；后续先区分 endpoint ambiguity 与 parser handoff 两个精确信号，再决定是否分批修复。
- 两案分别连续运行 271 秒和 827 秒，均由原始活跃模型流完成。固定累计时长不得触发答案降级、替换或系统代答；只有 transport 断链、真实无进展、调用方取消或独立安全边界才能进入恢复。
