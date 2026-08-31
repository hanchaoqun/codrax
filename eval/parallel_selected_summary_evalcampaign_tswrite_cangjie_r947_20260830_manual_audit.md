# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T02:32:11Z
- sweep_start_ts: 20260830-193210
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260830-193211 | write_apply,answer_regex | none | 193s | 27 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-honest-unverified-boundary | 单行补丁把非空字符串 truthy 判断收窄为仅 `true`/`error` 强制 WASI，`false` 与正常 native fallback 语义正确；`make check` 通过，但报告明确执行语言只有 Python、能力为 `source_static`，没有执行 TypeScript 目标行为。因此 `unverified:production_verification_source_static_only` 是正确的 fail-closed 边界，不能通过把静态 oracle 冒充行为证明来改绿。 |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260830-193211 | typed_inventory_rowset,dimension_substring,answer_contains | none | 592s | 42 | read=13,repo_map=2,list=0,trace=0,source_lens=2 | midloop=5,inv=6/2,fin_reject=9,unavail=11,prune=0 | fail-system-contract | 模型已形成正确 2 个 extend、2 个 foreign func、8 个 public class 的 12 行表；系统把 synthetic global set 的内部标签 `source inventory principal rows` 写进每个 typed row 的 `SetLabel`，随后硬要求该内部集合名成为每行可见 bucket，造成同一正确答案 9 次拒绝并最终降级。不是模型波动；生产根修见 `5722d5d69`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
