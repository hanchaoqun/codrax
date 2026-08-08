# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T13:29:04Z
- sweep_start_ts: 20260808-062903
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-062904 | answer_regex,answer_contains | none | 138s | 31 | read=7,repo_map=5,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Classification was repaired (`is_role_locate_lookup=false`, role profile false), but the evidence set never grounded canonical stage binding/transfer. Finalizer was identified by `tryAutoRepairFinalizerAnswerDocument`; `BuildAgentContext -> BusContext` is reversed and BusContext/Mutable arrows to every stage are unproved. `FlowFindings=0` is structural: explain/define runs lookup with findings disabled. |
| 2 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260808-062904 | answer_regex,answer_contains | none | 205s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | Correctly discloses no directed `buildAnalysisIR -> gate.Run` path and shows both real calls into `RunWith`, but does not enumerate the key ordered operations in the function body. Five closure attempts reflect endpoint reachability being used for an intraprocedural sequence. The sole finalizer reject was malformed structured JSON (`kind=""`) and recovered on the next round. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decisions

- `EVAL-B354-ARCHCOMPONENTFLOWCLOSURE1=P1`: architecture/component-flow has no typed flow/transfer predicate. `predicate_axis=define` permits definition-only closure, while deterministic dataflow is deliberately run as lookup with `SkipFindings=true`. Soft teaching that requires FlowFindings is therefore unfulfillable. Add a schema-enum flow axis and make closure/prompt consume typed relation carriers; do not inspect raw request or answer prose.
- `EVAL-B355-INTRAPROCSEQUENCEAUTH1=P1`: ordered calls/steps within one function are not the same contract as a directed source-to-sink call chain. Preserve honest `no_directed_path`, but separately collect and render the ordered operations inside the named owner so a stale/wrapper sink does not erase the requested middle sequence.
- Trace red lines were not exercised by either case and remain unchanged: explicit-window projection and supplementation stay active; only typed on-chain rows may be ranked as roots; adjacent/background evidence is support only; the system does not rewrite model conclusions.
