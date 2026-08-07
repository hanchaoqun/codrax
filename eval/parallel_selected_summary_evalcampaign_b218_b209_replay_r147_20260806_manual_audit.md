# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T04:29:02Z
- sweep_start_ts: 20260806-212858
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-212904 | answer_regex,answer_contains | none | 153s | 21 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | `source_line` is consumed and the prior stdout error is fixed: prose and citation now both say stderr. The model nevertheless ignores the copy-ready graph, authors an unsupported story sequence, retries twice, then removes it. Final prose also invents an unseen `make_sink(kind) -> Logger` construction bridge; the fixture only proves the factory and Logger initializer as disconnected facts. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-212902 | answer_regex,answer_contains | none | 182s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | Selector/value roles remain distinct, but the model again ignores the exact graph and removes its story diagram after three rejects. More importantly, it says `resolve` returns the registered class although current source returns `cls()` (an instance); explorer read line 34 but emitted only the definition line, so the exact return/instantiation fact did not reach Primary Evidence. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion and task ordering

1. `EVAL-B228-CALLLINECTX1` is production-closed: grounded call/callback lines appear in both finalizer prompts; C++ preserved `std::fputs(line.c_str(), stderr)` and no longer contradicted its citation.
2. New P0-near `EVAL-B231-DIAGRAMVIEWSPLIT1`: the copy-ready capsule chose the analyzer/raw contract's flow/call-DAG carrier while the same final prompt's effective call-chain Required Answer Blocks taught sequence. This is a system-authored contradictory tutorial. S36t changes the capsule to consume the same final `AnswerSemanticView.DiagramPlan` and removes duplicate alias/per-edge JSON when a complete body+array is present.
3. `EVAL-B229-COPYGRAPH1` remains partial pending replay after S36t. The first implementation was structurally valid but duplicated multiple authoring forms and disagreed with the effective diagram family, so non-consumption cannot be attributed solely to model variance.
4. New P1 `EVAL-B232-DISCONNECTAUTH1`: a model can still narrate disconnected typed components as one continuous setup/runtime chain. General solution: publish the exact typed connected-component partition and missing-bridge boundary as high-salience guidance; do not scan answer prose or have the system rewrite the conclusion.
5. New P1 `EVAL-B233-RETURNBODY1`: an already-read return/instantiation line may never become a typed evidence row, leaving the finalizer with only a function definition and causing class/value/instance substitution. General solution requires parser/grounder-authored return/value-flow promotion across languages, not a `return cls()` or Python-only rule.
6. `EVAL-B230-MEMBERMULTIREF1` remains separately open. No Trace runtime authority, explicit-window causal projection, auto-supplement, root ranking, wakeup chain, or eliminable-amount code was touched by this batch.
