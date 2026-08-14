# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T08:21:07Z
- sweep_start_ts: 20260814-012106
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260814-012107 | write_apply,write_patch_oracle | none | 139s | 24 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One bounded production edit normalizes whole-valued float years/months to int and rejects fractional floats. Native `python3 -m unittest discover -v` ran all four behavior tests and passed 4/4; changed-path verification is `covered/project_runner/target_behavior`, then the controller finished `all_verified`. No test edit, replan, malformed JSON, fallback, unavailable tool, or degraded-answer path. |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260814-012107 | answer_regex,answer_contains | none | 140s | 25 | read=13,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | First structured final emit was accepted with zero reject. The model authored all 12 production implementers and all 12 grounded `implements` relations; renderer compatibility converted class syntax into a valid flowchart without losing edges. Two prose defects remain: it repeated an unsupported/wrong “9 files” count although 12 distinct support paths were present, and it explained literal `<\|..` syntax after the visible graph had been normalized to labeled `implements` arrows. The former originated in explorer advisory reason and is model adherence/arithmetic drift, not typed authority; the latter is the generalized B775 display-language gap fixed after this replay by soft cross-kind teaching only. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner: 2/2 PASS. Human: one pass, one partial.
- The Python write case supplies the native-runtime positive witness required after r474's source-static Rust fail-safe: implementation behavior, not source shape, was executed and verified.
- The QF type-relation diagram is structurally complete and relation-preserving across the class-to-flowchart compatibility rewrite. The system did not author a node, edge, label, relation, or conclusion.
- The “9 files” statement was first minted in explorer's model-authored closure reason. The finalizer prompt explicitly marked that reason advisory and the typed member set authoritative; therefore this replay does not justify a new final-prose number scanner or system rewrite. Keep it as model adherence/arithmetic variance unless repeated typed-context evidence reveals a producer gap.
- Registered and implemented `B775-DIAGRAMSEMANTICPROSE1`: every diagram kind now receives soft instruction to describe relations in reader-facing domain terms and not narrate literal Mermaid operator tokens/arrow glyphs that a compatibility renderer can legitimately normalize. No validation gate or answer mutation was added.
- No empty answer, stale-draft recovery, fixed-age fallback, or active-stream-at-4ms degradation occurred. Trace explicit-window causal projection, auto-supplement, chain-only root-cause authority, and background/support separation were untouched.
