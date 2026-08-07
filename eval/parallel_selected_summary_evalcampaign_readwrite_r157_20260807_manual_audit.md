# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T09:36:56Z
- sweep_start_ts: 20260807-023655
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260807-023656 | write_apply,answer_regex | none | 244s | 21 | read=11,repo_map=4,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | The one-line TypeScript patch is semantically correct and the repository-owned Python oracle passes, but no Node runtime is available and the six TypeScript assertions were not executed. The final `proof_weak` disclosure is therefore honest, not a false code-failure verdict. The write analyzer did waste one model round by serializing `behavior_contracts[]` as malformed string JSON before retrying; record as `EVAL-B253-WRITEJSONCARRIER1`. |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260807-023656 | answer_regex,answer_contains | none | 319s | 36 | read=4,repo_map=3,list=1,trace=0,source_lens=3 | midloop=8,inv=2/0,fin_reject=4,unavail=0,prune=2 | fail | Literal names and retry semantics are correct, and the final Mermaid survives, but the user-required comparison table is gone. The source-inventory extraneous-row gate repeatedly treated comparison dimensions (`适用时机`, output form, description) as fake principal members; after four rejects the model deleted the table and the structural contract still accepted the answer. Record `EVAL-B251-PERMEMBERTABLE1` plus `EVAL-B252-COMPMATRIXAXIS1`. The analyzer also opened an over-broad repo-wide source-inventory lane for a bounded two-tool comparison, causing 19 exploration rounds / 319s; retain as `EVAL-B254-COMPINVROUTE1` observation rather than fitting these tool names. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner PASS on the read case is not a human pass: the regex required Mermaid but did not assert the separately requested table.
- The write patch itself is correct. `proof_weak` is the right final authority because `make check` ran the Python structural/behavioral oracle but the Node probe was unavailable and `tests/js-binding.test.ts` did not execute.
- JSON shape failure was recoverable by retry and did not erase the answer, but the write-analysis prompt lacked the compact shape-first instruction already used by other structured emitters.
- No Trace/runtime case ran in this pair. Trace explicit-window causal projection and auto-supplement behavior were therefore guard-audited from code boundaries, not re-certified by r157 production data.
