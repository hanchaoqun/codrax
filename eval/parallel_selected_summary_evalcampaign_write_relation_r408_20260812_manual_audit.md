# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T22:46:05Z
- sweep_start_ts: 20260812-154603
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-154605 | write_apply,answer_regex | none | 132s | 23 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Source and three regression tests are correct; no duplicate test block was reinserted and no cross-language Go probe was executed. `make check` passed but both changed paths carried capability=source_static, so the final unverified verdict is honest. New B678: controller prompt exposed only report status=passed, and the deterministic proof scheduler did not turn inline-capable static-only production coverage into a bounded probe batch. |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-154605 | answer_regex | none | 189s | 23 | read=1,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | The relation carrier now supplied four exact typed edges, including `tokenize_bytes -> best_merge`, and the final prose/list preserved all hops plus the fallback. The first diagram ignored the copy-ready node topology and invented five unsupported edges; validator correctly rejected it. The model then removed the optional diagram despite recognizing the supplied skeleton. B672 relation handoff is production-positive, but diagram retention remains model-authoring partial rather than a missing system relation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and next batch

1. B675/B677 are production-positive: incompatible proof code is stopped before
   execution, and the already-applied falsy-prefault tests are not appended again.
2. B678 is a typed context and scheduling gap. `verification_evidence=passed`
   reached the controller without its per-path capability; the report's two
   `source_static` rows were consulted only after the model selected
   `all_verified`, at which point the system downgraded instead of scheduling
   executable proof.
3. The generic repair renders bounded changed-path status/caliber/capability in
   controller context and deterministically schedules one direct probe-authoring
   batch only for source families with an actual inline runtime. TypeScript uses
   JavaScript; Go/Python/JavaScript/Ruby/Java use their direct runtime. Native-only
   C/C++/Rust/ArkTS/Cangjie/Kotlin/Swift/Lua/Proto remain honest unverified and
   must use project-native runners—no wrapper probe is fabricated.
4. B672 is now production-positive at the typed handoff: the Finalizer received
   the exact four-edge skeleton. The model's choice to discard an optional graph
   after one rejected draft is not repaired through answer rewriting or a keep-
   diagram hard gate. Continue cross-family evals to decide whether shorter patch
   teaching has generic ROI.
5. No fixed-age active-stream fallback was observed. Four milliseconds without a
   complete answer cannot trigger degradation while bytes remain active.
