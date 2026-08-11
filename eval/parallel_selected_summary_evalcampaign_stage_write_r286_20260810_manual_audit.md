# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T02:00:54Z
- sweep_start_ts: 20260810-190052
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260810-190054 | write_apply,answer_regex | none | 154s | 21 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | The one-line TypeScript patch is correct and preserves the existing six-case test. Write analysis used one dispatch, plan/apply had no JSON carrier repair or replan, and the final output truthfully retained the delivery as unverified. `make check` executed only a Python source-static oracle; Node/TypeScript behavior was not executed, so human review does not upgrade runtime proof to pass. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-190054 | answer_regex,answer_contains,mermaid_edge_count | none | 283s | 40 | read=8,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | Runner false positive. B495 is production-positive: no explicitly ungrounded `Mutable initializer/value_fact` row reaches finalizer enrichment. The final graph honestly preserves only three stage precedence edges and leaves BusContext unproven, but prose still claims complete Mutable/BusContext data flow and includes unsupported component details. Finalizer input is 82KB and contains unrelated typed lexical groups from `explorerSearchCache`/budget code plus an unrelated trace-admission FlowFinding selected only because it shares `context.go`; this typed context pollution is a generalized precision gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r286 judgement

- Runner: 1/2; human: 0 pass / 1 fail / 1 uncertain.
- `B495-UNGROUNDEDLANE1` closes with a direct production witness. The audit row remains available to Explorer, but no rejected initializer is replayed to Finalizer as factual value evidence.
- `B496-DECLREPAIR1` remains implemented/pending direct production witness: this run used genuine assignment lines, not declaration-as-initializer.
- `B498-SUPPORTSCOPECTX1/P1-high`: `renderAnswerDocLocalFactOrderCapsule` ignores the compiled support scope, so unrelated same-owner groups from any ranked evidence file enter every finalizer prompt. Flow enrichment does use support scope, but `allowsFlowFinding` accepts a finding solely because one path shares a support file; a trace-admission path in `context.go` therefore enters a read-pipeline architecture answer. Apply the existing support SSOT to lexical groups and require an evidence/anchor connection (not file-only coincidence when typed anchors exist) for optional FlowFindings.
- The repair must only reduce advisory context. It must not change relation evidence, diagram gates, model prose, write mode, or Trace query/projection/supplement behavior.
- Analyzer again omitted the explicitly named `Mutable` from `diagram_hint.participants` while retaining it in `entities`; this is a repeated same-case model adherence issue, but no precise typed current-request field independently says which entity must be a participant. Keep `B497` as P2 model variance; no raw-request hard gate.
