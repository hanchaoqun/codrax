# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T10:24:27Z
- sweep_start_ts: 20260814-032425
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260814-032427 | answer_regex,answer_contains | none | 225s | 35 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / system panic | No answer or Mermaid was produced because the process panicked after the explorer emitted 56 evidence rows. `callChainPrincipalControlSelectionEntries` deduplicated `append(base,out...)` and then sliced by the raw `len(base)`; one repeated evidence identity made the deduplicated result 40 entries while the raw prefix was 41, causing `[41:40]`. B785 slices by the deduplicated prefix and pins the duplicate-base shape. The runner's banned/no-Mermaid reasons are downstream symptoms, not the cause. |
| 2 | github_issue_commons_lang_random_ascii | FAIL | eval/results/github_issue_commons_lang_random_ascii-20260814-032427 | write_apply,answer_regex | none | 346s | 25 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | partial / honest-unverified | Applied tree is correct: production fast-path and range shrinking are both gated by `end <= 0x7f`; tests cover CJK, Greek, Cyrillic, full-width/Arabic digits, exact 0x7f and >0x7f boundaries. The host exposes `/usr/bin/java`/`javac` stubs but no installed JVM, while `make check` is explicitly source-static. The controller correctly refused to call this behavior-verified and preserved the delivery as `verification_proof_incomplete`. This is an environment capability boundary, not evidence to lower the verification bar. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner: 0/2 PASS. Human: one system fail, one correct implementation with honest unverified status.
- B785 is a generalized cardinality bug in a shared call-chain support compiler. It is independent of Go, Mermaid, the named endpoints and answer prose: any language/runtime whose support base contains a duplicate evidence identity can hit the same panic.
- The repair preserves the existing evidence selection and relation authority. It only computes the slice boundary from the same deduplicated base used to build the combined set; it does not fabricate edges, force a diagram, or rewrite model content.
- The Java work should remain unverified until a real JVM/project runner exists. Source-static success must not be promoted to target behavior proof, and the eval runner should continue reporting that boundary.
- This batch does not alter Trace explicit windows, automatic supplementation, causal projection, on-chain root election, adjacent/background support roles, or active-stream liveness/recovery.
