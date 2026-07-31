# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T22:54:19Z
- sweep_start_ts: 20260731-155417
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-155419 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 45 | read=2,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=1,prune=1 | fail-presentation | 显式窗、完整 Trace 因果投影、自动补采、根因榜、5 sync/10 oneway、1.409ms occurrence 均正确；AG2/AG3 caveat 也准确，但位于约 1100 行报告尾部，导语仍写“唯一/其余未阻塞”，关键口径的展示优先级不足。 |
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | PASS | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-155419 | write_apply,answer_regex | none | 420s | 18 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | AF1/AG1 全链生效。首个 patch 用 saturating/raw arithmetic，被真实 `make check` 拦截；durable replan 后 checked_add/checked_sub、越界耗尽和 usize::MAX 测试正确，第二次 Make oracle 通过，3 个 Rust 路径均为 project_runner covered，最终 verified。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Residual and disposition

- `EVAL-B12-AH1`（P1，typed correction placement）：确定性 coverage
  caveat 在文档尾部，无法及时纠正摘要中的 exhaustive overclaim。最优方案
  是把同一 typed 内容做成带不可伪造 system marker 的前置 authority block，
  固定在 summary 后、模型明细与完整 Trace 因果投影前；不扫描或重写模型
  文字。
- PyO3 第一计划偏离明确 checked arithmetic 要求属于模型波动；真实项目
  oracle + typed replan 已正确闭环。当前不为 Rust token、函数名或该 case
  建专用 hard gate，保留 420s 效率观察项，优先继续更高风险 eval。
