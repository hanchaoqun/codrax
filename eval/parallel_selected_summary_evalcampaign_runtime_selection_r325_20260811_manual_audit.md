# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T17:46:07Z
- sweep_start_ts: 20260811-104606
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-104607 | answer_regex,answer_contains | none | 146s | 23 | read=2,repo_map=2,list=1,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B551-A production-positive: typed selection obligation forced registration/return/call evidence. The first model draft contained the conceptual registry/lookup/callback/MRO route, but the hard relation repair supplied a call-only skeleton and the patch removed the diagram instead of preserving typed registration/return facts. B551-B confirmed. The final generic answer-facet supplement is not actionable; B550 remains open. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-104607 | answer_regex,answer_contains | none | 158s | 22 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | B551-A production-positive: Explorer could not close until it emitted typed factory return/assignment evidence. B551-B confirmed independently: repair reduced selection, construction, injection and virtual dispatch to two direct calls, then the final answer shipped without a relationship diagram. B552: the accepted factory-return citation points at the function signature rather than the return statement. B553: two title-only phantom blocks caused avoidable missing-id retries. The prose also incorrectly says there is no branch while the registry has a kind guard. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B551-A=production-positive`: runtime-selection authority now survives endpoint normalization and forced both Explorers to collect a concrete selection fact before completion.
- `B551-B=P0-confirmed`: the recovery renderer projects a mixed typed relation set into call/callback-only sequence messages. Guard, return/construction, registration/type/binding and assignment facts are silently omitted from the copy-ready diagram, so a correct conceptual graph is repeatedly rejected and ultimately removed. The repair must preserve non-message facts as typed diagram annotations/notes or another family-native carrier; it must not relabel them as calls or author new model relations.
- `B550=P1-confirmed`: both generic supplements came from typed `answer_facet_coverage`, with soft `branch_guard` and `diagram_spine` obligations visible in the prompt. The user-facing caveat must name the exact missing typed facet instead of saying only “some dimensions may be incomplete”.
- `B552=P1-confirmed`: selection completion currently accepts a citable return row whose source line does not own the returned object. Tighten the evidence/source join without scanning answer prose.
- `B553=P1-confirmed`: a structurally empty title-only block can be discarded mechanically before ID validation; only blocks with no semantic payload are eligible, so model conclusions and visible content remain untouched.
- Both tasks stayed active until their model-authored answers completed. No fixed four-minute fallback or system-authored answer occurred. Active stream age alone remains forbidden as a degradation trigger; only typed first-byte/stall/termination/exhaustion conditions may enter retry/recovery, and recovery may preserve an existing model draft rather than synthesize a conclusion.
- This batch did not exercise Trace. Explicit-window causal projection, automatic supplementation, on-chain root-cause authority, and the measured-cost/rule-eliminable dual view remain unchanged.
