# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T08:42:11Z
- sweep_start_ts: 20260807-014209
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | FAIL | eval/results/sr_cpp_virtual_chain-20260807-014211 | answer_regex,answer_contains | none | 139s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | Local facts/citations are mostly correct, but the answer still narrates setup-time `make_sink/create` and log-time virtual dispatch as one consecutive path despite the typed `unproven_between_components` boundary. It also never states virtual/dynamic dispatch explicitly, so the declared oracle fails. JSON stayed valid; the one finalizer reject was an optional diagram repair. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-014211 | answer_regex,answer_contains | none | 161s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | Core class, registry lookup, decorator binding, instantiation, and MRO conclusion are correct. However, the model copied the system-provided `callback` sequence capsule byte-for-byte and the validator reclassified that edge as a missing call anchor; the model had to delete the optional diagram on the next patch. This is deterministic system contract churn, not malformed JSON or model fluctuation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `EVAL-B236-PHASEBRIDGE1` is not production-closed. Exact per-member source coordinates made the C++ call-chain `member_set` evidence-authorized even though those coordinates prove nodes, not relation/order/bridge. The stricter relation authority predicate covered relation enumerations but not the typed `QFCallChain` narrative family.
- `EVAL-B246-CALLSEGCONTRACT1`: the Required Answer Blocks copy itself says to walk one call chain in actual-control-flow order, while the later typed capsule says the components are unbridged and must not be consecutive. This contradictory soft context predictably pulls the model toward a false end-to-end narrative.
- `EVAL-B247-SEQUENCECALLBACK1`: the copy-ready sequence recipe emits a solid Mermaid message with `relation_kind=callback`; `diagramParsedEdgeRequiresCallAuthority` nevertheless treats every non-reply sequence message as a call. The separate callback evidence validator is therefore unreachable for an otherwise correct sequence callback edge.
- No raw user text, model thinking, or final-answer keyword gate is an acceptable repair. The generalized fix is typed family authority + segment-safe block teaching + callback relation ownership in the shared diagram validator.
