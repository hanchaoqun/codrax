# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T10:18:28Z
- sweep_start_ts: 20260829-031826
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260829-031828 | primary_answer | none | 126s | 28 | read=8,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=3/1,fin_reject=1,unavail=0,prune=0 | partial | The call path, capacity guard, in-memory `rows` write, and terminal `System.out.println` are all identified correctly. The answer does not directly resolve the request's false “audit persistence” premise with an explicit no-database/no-durability statement, so the strict semantic oracle fails. The finalizer context already carried the exact terminal-body operation and effect boundary; this is model adherence, not missing system context. |
| 2 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260829-031828 | write_apply,write_patch_oracle | none | 171s | 27 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | The production-only patch adds value-based `equals`/`hashCode` exactly and leaves tests unchanged. A direct Java behavior probe that references `LazilyParsedNumber` was accepted, proving B1449 did not block a legal target-coupled probe. The source-static `make check` passed; Java execution remained honestly unverified because the host has no JDK. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Java read mode

- The answer reconstructs the grounded path from `VisitController.create` through `VisitService.schedule`, the capacity reads/check, `VisitRepository.insert`, and `AuditLog.record`. It correctly places the guard at `VisitService.schedule:18` and says the visit record is stored only in the repository's in-memory `rows` list.
- The terminal implementation was available as typed current-source evidence: `AuditLog.record -> System.out.println` at `AuditLog.java:6`. The finalizer context also stated that this exact call does not prove storage or durability. The model used the exact operation in the final answer, but did not explicitly say that audit events are not persisted to a database. Because the user premise says “审计落库”, the human verdict is partial and the strict oracle failure is meaningful.
- This is not a missing-context or validator-contract GAP. Adding a prose keyword gate or forcing one negative sentence would overfit the fixture. The existing typed terminal-effect context is precise; the remaining miss is model adherence and stays under heterogeneous replay observation.
- The sole finalizer rejection was structural: the model declared directed `call_edge` claims without `edge_anchors`. One patch added typed anchors and succeeded. It did not remove relations, recover an old draft, or trigger repeated form churn.

### Gson write mode

- The applied tree changes only `LazilyParsedNumber.java`, adding value-based `equals(Object)` and `hashCode()`. The native test source and expected values are unchanged.
- The emitted probe directly constructs and exercises `LazilyParsedNumber` instances and a `HashMap`; it does not wrap a compiler/test command. The post-B1449 coupling gate accepted this executable type carrier, which is the intended positive production arm.
- `make check` passed its source-static assertions. Java behavior execution and manifestless direct-main compilation were unavailable because this host has no Java runtime, and the workflow truthfully finished `accept_unverified/runner_missing`. The eval runner's FAIL therefore records verification caliber, not a bad patch or an authority bypass.
- The baseline probe attempt is the expected pre/post behavior comparison path, not an accidental duplicate current-state proof. No new high-ROI system GAP was found in this write run.

### Resolution

- `B1449-PROBECOUPLINGLITERALAUTHORITY1` is now production-positive/core-closed: r935's command-string false coupling is rejected by tests, while r936's real Java target reference is accepted in an apply/verify workflow.
- Keep `sr_java_call_chain` in the high-priority heterogeneous pool. A later pass should test whether the model consistently states the terminal effect boundary; do not change the answer validator or scan final prose to force fixture wording from one miss.
