# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T07:35:30Z
- sweep_start_ts: 20260807-003529
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-003531 | answer_regex,answer_contains | none | 181s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=3,unavail=0,prune=0 | pass | Core conclusion and registry/resolve/MRO mechanism are correct, but the optional diagram paid three rejects before model-owned removal. The first specialized hint was prefixed by a contradictory full-emit directive; the next patches still redrew instead of copying the capsule. `JsonPlugin.handle` also ends on a class-declaration citation rather than the callback/MRO lines. `EVAL-B243-RECIPESELF1`, `EVAL-B244-RETRYCONFLICT1`, and the existing citation/multi-ref debt remain material quality/efficiency gaps. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-003531 | answer_regex,answer_contains | none | 221s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | The final answer still invents the unproved factory-return -> Logger ownership bridge, duplicates the two principal rosters into several system-generated supplement blocks, and detaches/misbinds citations. More seriously, on retry the model copied the system's copy-ready capsule byte-for-byte, but that capsule drew register/guard/return as sequence message arrows and the validator rejected those system-authored edges. This is deterministic system contract self-conflict, not model fluctuation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `EVAL-B241-MEMLOCREF1` passed unit/full-suite validation but this replay did not exercise its accepted production lane: the first two C++ completion attempts carried correct per-member coordinates, then the member-note evidence-strength gate rejected them; the final accepted attempt removed all refs and landed as soft `system_inference`.
- `EVAL-B243-RECIPESELF1` is P0/P1. A copy-ready sequence recipe cannot use a generic `->>` message for guard/register/assignment/return/type facts. It must contain only diagram-family-expressible relations and one unambiguous relation per endpoint pair; the complete typed capsule can keep the omitted sibling facts for prose/Notes.
- `EVAL-B244-RETRYCONFLICT1` is P0/P1. The optional-diagram patch hint was mechanically prefixed with “re-emit the full previous document” after it had already selected patch-only recovery. Both instructions cannot be true in the same turn.
- `EVAL-B245-AGGSUPPDUP1` is P1. The accepted C++ document already had two model-authored principal lists; aggregate carrier normalization appended eight rows and persistence retained several duplicate system supplement blocks. System evidence supplementation must not duplicate an already represented model-owned principal roster.
- `EVAL-B236-PHASEBRIDGE1` remains the highest answer-quality debt after the red-line self-conflicts: the system correctly says the factory/guard/return and invocation components have no proved bridge, but the final answer still narrates constructor injection as established.
- No raw user/model-prose hard gate, Trace contract change, or system-authored conclusion was introduced. Trace windows, auto-supplement, causal projection, root ranking, wakeup chains, eliminable impact and dual-dimension root analysis remain isolated.
