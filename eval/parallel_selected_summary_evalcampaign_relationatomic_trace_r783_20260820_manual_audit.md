# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T18:41:02Z
- sweep_start_ts: 20260820-114100
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-114102 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 224s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000–2.020s 窗、自动补采、Trace 因果投影、链上 11ms IO 首因、三个 1ms runnable 调度席、实际占时/规则可消双轴和背景隔离均完整，且活动流无时间降级。正文主段却把 wakeup path 直接叙述成 app-100 被上游完成依赖阻塞，并说 fscache IO 是整条链延迟源头；同页 typed caveat 明确 `wakeup_path_blocking_authority=not_implied`、`target_direct_blocking_authority=not_provided`，构成主要结论与证据边界冲突。另有“三个 runnable 合计 3ms”虽算术成立，但不能与 11ms IO 跨席直接补足目标 20ms sleep。B1253 从 observe 升 confirmed。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-114102 | answer_regex,answer_contains | none | 947s | 50 | read=41,repo_map=3,list=0,trace=0,source_lens=1 | midloop=25,inv=7/0,fin_reject=9,unavail=0,prune=5 | partial | 最终仅一张合法 sequenceDiagram 和一张有正确列头的阶段表，Analyze→Explore→Extract→Finalize 三条 typed precedence 均保留，较 r782 恢复 Extractor/Finalizer 主段。但原子协议首轮 17 edits 被固定 max=16 拒绝；后续校验要求删除无 anchor 的 visible bad edge，而原子工具只允许命中 exact prior anchor，形成不可执行合同，最终第 10 次才退回 whole-block replace 成功。图因此删去了 Orchestrator/BusContext 的真实交互，仅保留阶段顺序；正文还把 `FinalAnswer` 含混写成 `AnswerDocumentV2` 内字段。runner PASS 不能掩盖 947s、9 rejects、7 次 completion 尝试和关系表达缩水。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
