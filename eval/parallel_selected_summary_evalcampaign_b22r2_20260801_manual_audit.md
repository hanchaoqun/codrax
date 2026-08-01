# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T15:38:06Z
- sweep_start_ts: 20260801-083804
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-083806 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B22-A live：确定性优化表同时显示 raw span=5.000ms 与 typed 可消除=4.600ms/65.7%；显式 5.000..5.007 窗、根因排序、唤醒链、完整 Trace 因果投影和自动补采均保留。frame_causality=unproven 仍接管最终主结论，未把候选写成已证丢帧因果。 |
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260801-083806 | write_apply,write_patch_oracle,answer_contains | none | 158s | 19 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单行 patch 在隔离 worktree 正确落地，deterministic go test 1/1 通过，主仓未改。过程暴露通用 EVAL-B22-GOMAINPROBE1：Go 探针只接受外部 import，被改包为 package main/目标为未导出符号时无法表达同包探针，连续 3 次 coupling reject 后只能删探针；功能结论仍正确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
