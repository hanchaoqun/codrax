# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T23:01:30Z
- sweep_start_ts: 20260812-160129
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-160130 | answer_regex | none | 164s | 24 | read=2,repo_map=5,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | The repaired answer retained a valid four-node sequence diagram and the principal native call path. B672 therefore has a production diagram-retention positive. The caveat nevertheless says `_tokenize_slow` still needs confirmation even though the same run had already read and summarized that branch; this is evidence-to-final context loss/authoring shrinkage, not a missing relation edge and not grounds for system-authored prose. |
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-160130 | write_apply,answer_regex | none | 208s | 23 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | partial | The applied TypeScript fix and falsy regression tests remained correct and `make check` passed. The new per-path capability context reached the controller, which correctly recognized `source_static`, but the cumulative actual-diff hook had already minted the follow-up as `verify_only`. It repeated the same static command instead of entering the direct JavaScript probe-plan lane, then honestly finished unverified. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and next batch

1. B679 is confirmed: proof-repair batches had two constructors. The ordinary
   post-verify constructor mapped a pure `production_path_source_static_only`
   queue to a direct ReadyForChangePlan probe batch, while the older cumulative
   actual-diff hook independently hard-coded `verify_only`. Discovery location,
   not typed obligation content, therefore changed execution semantics.
2. The generic repair uses one constructor for both entrypoints. Only a queue
   composed entirely of source-static production paths with an exact supported
   inline runtime becomes direct probe authoring; mixed behavior-contract or
   native-only queues retain verify-only/unverified behavior. This does not
   lower proof caliber.
3. B680 is confirmed at the plan seam: a pure proof follow-up could submit a
   production structured edit and consume edit compilation/repair rounds before
   the scheduler rejected the unauthorized mutation. A shared typed validator
   now rejects production source paths before structured edit compilation. Test,
   fixture, and documentation paths remain available; a same-batch typed verify
   failure reopens production repair.
4. B672 receives a production-positive graph-retention witness in this replay.
   The remaining fallback-detail shrinkage stays an answer-context/authoring
   observation; no raw-answer keyword gate or system rewrite is introduced.
5. Active-stream policy was re-audited across adapter, evaluator, and finalizer.
   The built-in stream has no absolute age cap; SSE bytes refresh liveness, and
   the analyzer's 4ms terminal emit-only budget is not installed as a deadline
   when a precise streaming watchdog owns first-byte and byte-stall detection.
   The strengthened seam test waits 40ms before returning, proving the response
   outlives 4ms intact. A live byte-producing connection never authorizes a
   degraded answer. On true failure the system may preserve a prior
   model-authored draft with disclosure, but may not synthesize conclusions from
   evidence.
