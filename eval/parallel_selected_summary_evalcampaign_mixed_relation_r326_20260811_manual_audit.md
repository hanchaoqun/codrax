# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T18:09:04Z
- sweep_start_ts: 20260811-110903
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-110904 | answer_regex,answer_contains | none | 141s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Typed capsule contained two call/callback edges plus one factory-result Note, but the optional-diagram repair offered deletion as an equal shortcut. The model removed the graph; the final prose remained correct and the B550 supplement precisely named only the missing relationship spine. B553 title-shell retries did not recur. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-110904 | answer_regex,answer_contains | none | 157s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass-with-caveat | The patched sequence retained four exact call edges and the factory-return fact as an unanchored Note. The runtime guard was separately grounded at `src/registry.cpp:17`, but the family-neutral visual projection dropped it because unary guard evidence has no object endpoint. The disconnected components remained honest and no bridge was invented. B553 title-shell retries did not recur. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: 2/2; human: 1/2 (`pass-with-caveat + fail`).
- `B551-B` has a production-positive branch: sequence recovery can retain non-message binary facts as unanchored Notes without minting call authority. It is not fully closed because unary guard/condition facts never enter the binary relation recipe graph.
- `B550` has a production-positive witness: the Python fallback names only `关系图主干及其已证关系`, instead of globally weakening the otherwise grounded answer.
- `B553` has a no-recurrence production witness: neither case retried a duplicate title-only shell.
- `B552` is reclassified from an active source-line escape to a production self-correction witness. The attempted registration on the function signature was rejected as ungrounded, and the next round emitted the exact guard line plus exact return line. Keep the source-owned predicate under regression coverage; no additional hard gate is justified by this run.
- New `B554-UNARYSEMANTICNOTE1/P1-high`: the typed visual projection requires two endpoints before it can create any recipe, so citable unary control facts such as a branch guard disappear from the visual capsule. Preserve such facts as one-participant unanchored Notes; never synthesize a second endpoint or an edge.
- New `B555-OPTIONALDIAGRAMESCAPE1/P1`: when a verified optional skeleton already contains useful typed structure, retry teaching still presents deletion as the easiest equal choice. Prefer local skeleton replacement through soft guidance, while retaining deletion only when the model judges the visual adds no useful relationship structure. Diagram optionality must remain model-owned.
- Both runs stayed below four minutes. The fixed-age rule remains explicit: an active stream with heartbeat/reasoning/assistant/tool progress must wait even beyond four minutes; it must not trigger a system-written fallback answer.
