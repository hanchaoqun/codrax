# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T17:55:47Z
- sweep_start_ts: 20260801-105545
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-105547 | write_apply,write_patch_oracle,answer_contains | none | 91s | 19 | read=2,repo_map=0,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan→apply→verify 全链通过；applied tree 仅 `retrun`→`return` 一行，编译与 diff 验收通过。计划摘要有一次“误写成 retrun”自反措辞，rationale/patch 均正确，按模型文案波动记观察项，不立系统 GAP。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-105547 | answer_regex,answer_contains | none | 216s | 36 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=4/2,fin_reject=0,unavail=0,prune=0 | fail | `analyzer.go:2663` 只证明 `buildAnalysisIR -> gate.RunWith`；`gate.go:134-135` 真值是 `gate.Run -> RunWith`。答案、principal item 与 Mermaid 均反写为 `RunWith -> Run`，definition citation 被当成调用方向证据。runner 只钉名称存在而漏掉方向。另有 18 主项 + 9 补项 + 27 完整项三重清单和条件调用未标条件。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
