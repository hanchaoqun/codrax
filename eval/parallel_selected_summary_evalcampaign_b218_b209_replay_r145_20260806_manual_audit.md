# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T03:34:18Z
- sweep_start_ts: 20260806-203416
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-203418 | answer_regex,answer_contains | none | 181s | 21 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | Correctly keeps `@register("json")` selector separate from `content_type() -> "application/json"`, identifies `JsonPlugin`, and explains `run_pipeline -> resolve -> callback handle` plus the cooperative mixin chain. Both retries came from an optional sequence diagram that promoted lookup/return/MRO candidates into direct calls; the model removed the diagram without losing the prose answer. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-203418 | answer_regex,answer_contains | none | 199s | 23 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | Citation metadata and participant parsing are fixed, but four diagram retries end in an authority escape: the unproved `Sink.write -> ConsoleSink.write` dispatch is relabelled `precedence`, and `kind -> create` is asserted as guard despite typed evidence being `SinkRegistry.create -> kind`. Final prose also overstates an unseen caller-to-Logger injection bridge. This is a deterministic validator gap, not model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- S36o participant-declaration isolation is effective: `participant SW as sink_->write` no longer creates a phantom `sink_ -> write` edge.
- S36p citation metadata binding is effective: the C++ four-hop list points to the correct call/declaration/override/library-call lines.
- S36q condition classification is effective: the typed guard evidence is present and no longer appears as a constructor binding.
- New P0 `EVAL-B227-RELABELAUTH1`: in a strict source call diagram, `relation_kind` was treated as proof for `guard/import/precedence/contain/observe`. A failed call could therefore be renamed to one of those enums and pass. The repair must compare each declared relation with same-direction typed evidence, keep `contain` as subgraph-only until a directed carrier exists, and must not inspect labels, user input, model reasoning, or final prose.
- Python's two optional-diagram retries remain a P1 context/recipe optimization. Its final answer is correct, so this run does not justify weakening evidence gates or adding case-specific answer rewriting.
