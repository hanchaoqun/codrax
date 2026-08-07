# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T04:43:35Z
- sweep_start_ts: 20260806-214333
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-214335 | answer_regex,answer_contains | none | 147s | 21 | read=5,repo_map=2,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | S36t is active: the prompt has exactly one sequence body+anchor array and no duplicate alias/per-edge JSON. The model still authors another graph and removes it after two rejects. Final prose explicitly says `resolve` returns the class without instantiating it, contradicting current `return cls()` and its own summary. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-214335 | answer_regex,answer_contains | none | 193s | 21 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | The single sequence template is present but not consumed; the optional graph is removed after two rejects. Final prose still invents the missing factory→Logger bridge, calls `unique_ptr` a raw pointer, and orders factory selection as a runtime hop after `Sink::write`. Explorer also produced a false typed binding at registry.cpp:32: that line calls `SinkRegistry::create(kind,path)` but does not name `ConsoleSink`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion and next batches

- `EVAL-B231-DIAGRAMVIEWSPLIT1` is production-closed: only the final effective sequence family is taught, and duplicate JSON forms are gone.
- `EVAL-B229-COPYGRAPH1` is now model-watch. The exact template remains a soft aid, but two replays show this model may ignore it. Do not add a prose hard gate or system-authored answer mutation merely to force diagram retention.
- `EVAL-B232-DISCONNECTAUTH1` remains P1: publish precise typed connected components and explicit missing-bridge status so prose synthesis sees the same boundary as the graph.
- `EVAL-B233-RETURNBODY1` is confirmed again and strengthened: Python final prose directly contradicts `return cls()` because no return/value row entered Primary Evidence.
- New P0/P1 `EVAL-B234-RELENDPOINT1`: model-authored relationship/registration evidence was relocated to a line that grounds only its anchor/subject call, while the claimed object endpoint (`ConsoleSink`) is absent. That relation then entered the typed capsule as `make_sink -> ConsoleSink`. Relationship authority must validate owner/direction/object against parser/grounder structure, not merely find one anchor token.
- Priority: B234 fail-closed endpoint authority first, B232 component boundary second, B233 cross-language return/value-flow promotion third. Trace runtime paths remain untouched.
