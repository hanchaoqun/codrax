# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T04:10:48Z
- sweep_start_ts: 20260803-211045
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-211048 | answer_regex,answer_contains | none | 206s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=3/1,fin_reject=2,unavail=0,prune=0 | pass | 最终图只画源码已证的 `buildAnalysisIR -> callee` 顺序边，并明确裸 `gate.Run` 不存在、真实终点为 `gate.RunWith`；两次拒绝分别纠正虚构的中间函数互调和精确锚点遗漏，未发现矛盾合同。 |
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260803-211048 | log_regex,write_apply,write_patch_oracle | none | 288s | 20 | read=8,repo_map=4,list=7,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | 补丁本身正确且仅改 `src/client.ts`；系统已发现并选择 `make check`，却因首选 JavaScript verification probe 缺 runner，在项目 suite 执行前以 `unverified:runner_missing` 结束。确认 `EVAL-B61-PROBEFALLBACK1`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
