# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T03:29:06Z
- sweep_start_ts: 20260816-202905
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260816-202907 | log_regex,answer_regex | none | 62s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Planner 首稿含多个串行结构错误。schema 先只报告 `actions[7].script missing`；唯一 compact repair 已给 actions 7/8 补 script，随后才暴露较早 join action 的 `left_fields/left_key` 冲突。框架在第二个精确 typed locus 尚未获修补机会时终止，未进入执行/对账/答案阶段。确认 B958：一次修补预算无法收敛 fail-closed validator 逐个暴露的独立错误；不是数据计算或最终 CSV 错。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260816-202907 | answer_regex,answer_contains,mermaid_edge_count | none | 347s | 40 | read=16,repo_map=2,list=1,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail-safe-partial | 最终图正确保留 Analyzer→Explorer→Extractor→Finalizer 三条 precedence，以及 `BuildInitialInstruction→TurnAArtifacts`、`dispatchStage→applyStageOutput` 两条已证局部 call；无证 `BusContext/Mutable` 保持断开并显式披露，没有系统补边。它仍未回答用户要求的四 Agent 与 BusContext/Mutable 完整数据流，正文却继续概括共享传递。B955 qualified-handoff 排序仅获 partial 正证；runner 的名字+一条边 oracle 仍是假阳性，关系完备性 gap 继续统一立案，不按本 case 补硬边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
