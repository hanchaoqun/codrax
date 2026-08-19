# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T23:41:08Z
- sweep_start_ts: 20260818-164107
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-164108 | log_regex,write_apply,answer_regex,answer_contains | none | 408s | 25 | read=12,repo_map=3,list=1,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | The final worktree preserves `tests/test_tokenizer.py` byte-for-byte and implements the requested one-token newline-run collapse in `fastlex/tokenizer.py`. The first plan accidentally created a duplicate method and `make check` correctly failed. The typed handoff reversed Python unittest's unlabeled `left != right` operands into `expected/actual`, so the replanner initially reasoned from a false role assignment. It later repaired the implementation, and a focused probe passed, but the second verify skipped the project suite; the prior failed `make check` therefore remained unsuperseded and the final `unverified` verdict was correctly fail-closed. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-164108 | answer_regex,answer_contains,mermaid_edge_count | none | 995s | 53 | read=72,repo_map=6,list=0,trace=0,source_lens=1 | midloop=62,inv=36/0,fin_reject=2,unavail=0,prune=10 | fail | Exploration expanded one exact `bus.Mutable != nil` guard into every sibling call in its enclosing function, causing 646 evidence rows, repeated forced reads, and false descent into unrelated trace helpers and a test helper named `rm`. The final diagram also retargeted the typed `o.busCtx -> ctxbuilder.BuildAgentContext` argument-flow edge to the visible claim `BusContext -> Analyzer`; preserving technical identities in metadata did not prevent the opposite endpoint's reader label from changing the relation's meaning. The answer therefore passed structural oracles while remaining materially misleading. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized gaps and disposition

1. **B1117 — unlabeled comparison operands are not expected/actual.** A bare runner message such as
   `AssertionError: X != Y` proves an operator and two ordered operands, but not their semantic roles. Preserve
   `left/right + roles=unlabeled`; populate expected/actual only from an explicitly labelled runner format.
2. **B1118 — cumulative replan verification must re-run the project suite.** A passing local probe cannot
   supersede an earlier exact project-suite failure. When the active plan carries controller-owned cumulative
   source-plan IDs and a project suite is available, continue from the probe into that suite; keep the proof bar.
3. **B1119 — exact-operation descent must not authorize sibling calls.** Guard/assignment/return fallback
   seeds are body-only. Only parser-owned typed call/flow edges can open child traversal. This is language-neutral
   and must not special-case `BuildAgentContext`, trace helpers, or `rm`.
4. **B1120 — visible endpoint labels must remain semantically bound to typed identities.** A participant-side
   reader alias may replace only the side explicitly authorized by the candidate. Every other endpoint must retain
   its exact technical identity or a parser-owned canonical reader identity; metadata must not license a diagram
   body that visibly claims a different component edge.

No case used a fixed 4ms total-age fallback. This batch did not touch Trace querying, explicit-window causal
projection, deterministic supplements, on-chain-only root-cause authority, or model ownership of conclusions.
