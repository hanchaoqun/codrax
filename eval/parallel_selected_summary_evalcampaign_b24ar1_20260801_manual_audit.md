# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T18:21:25Z
- sweep_start_ts: 20260801-112123
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-112125 | write_apply,write_patch_oracle,answer_contains | none | 98s | 18 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划、补丁和验证一致；applied tree 仍只有 `retrun`→`return` 一行。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-112125 | answer_regex,answer_contains | none | 252s | 30 | read=7,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | 新 typed authority 准确检出 19 条无同向 call-site 证据的伪串联边，但复用软 `citation` 导致零 reject；analyzer 还把用户明确要求的 sequence 错分为 `call_dag`，答案以 `gate.RunWith` 替代精确终点 `gate.Run`，并重复发布 20+1+26 项清册。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
