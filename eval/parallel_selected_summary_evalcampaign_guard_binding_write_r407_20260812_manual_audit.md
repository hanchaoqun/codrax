# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T22:15:54Z
- sweep_start_ts: 20260812-151553
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-151554 | answer_regex | none | 165s | 23 | read=3,repo_map=3,list=1,trace=0,source_lens=1 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | B673 production-positive: guard now cites line 20 rather than fallback line 22. B674 drafting fragment absent. The first diagram still invented unsupported member edges and was correctly rejected; the repaired answer removed the graph. Principal member_set eventually appeared, but no principal-member call handoff was observed and the final still omits the read Rust helper body, so B672 is not production-closed. |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-151554 | write_apply,answer_regex | none | 303s | 23 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | The source fix was reasonable, but replan appended the same three falsy-prefault tests a second time. Project make check is only source-static proof, so the controller correctly requested behavioural proof. The proof-only planner emitted Go probes for a TypeScript target; changes=[] bypassed language compatibility until execution, leaving the final verdict unverified. The valid probe-only plan was also rejected by disk persistence as an ordinary empty plan. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generic gaps and disposition

1. `B673-GUARDCONDITIONCITATION1` is production-closed by the polyglot replay.
   The correction came from schema-valid guard role + grounded Condition, not from
   scanning final prose.
2. `B674-DRAFTMARKERLEAK1` has a positive replay, but remains soft-teaching-only;
   no string-specific deletion gate was added.
3. `B672-PRINCIPALMEMBERSETEDGECOVERAGE1` remains pending production closure.
   Relation completeness is still weaker than node completeness; the system must
   continue to hand the model parser-owned typed edges, never draw or conclude them.
4. `B675-PROBEONLYTARGETLANGUAGE1` is confirmed. Full plans validate probe runtime
   against change paths; proof-follow-up plans intentionally have no changes and
   therefore lost the same authority. The generic repair validates exact batch
   TargetPaths before installation in both emitters.
5. `B676-PROBEONLYDURABILITY1` is confirmed. A controller-owned no-change plan with
   non-empty typed probes and target paths is a valid verification artifact, not an
   ordinary empty source plan. Persistence must admit only that strict shape.
6. `B677-REPLANALREADYINSERTEDBLOCK1` is confirmed. A verify-failure replan can
   append an insertion-only block that is already present in the current worktree;
   the existing duplicate gate only compares added runs inside the new patch. The
   next batch will use typed verify-failure state plus exact current bytes and an
   insertion-only hunk shape, without semantic/prose heuristics.

## Red-line audit

- No raw request, thinking, final-answer, or user/model keyword scan is proposed.
- Behavioural proof is not weakened: a static project command remains insufficient
  for explicit runtime contracts, and an incompatible probe is repaired before run.
- Read mode and Trace code paths are untouched. Explicit windows, causal projection,
  auto-supplement, typed on-chain-only roots, dual occupancy/eliminability axes and
  background-only adjacent evidence retain their existing authority.
- Active byte-producing streams have no fixed-age degradation path. Four
  milliseconds without a complete answer is not a fallback signal; only caller
  cancellation/deadline, no-first-byte, byte-stall, transport, or decode failure may
  terminate or recover the stream.
