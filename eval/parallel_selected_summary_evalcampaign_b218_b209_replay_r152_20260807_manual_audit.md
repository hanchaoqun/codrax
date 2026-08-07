# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T07:23:04Z
- sweep_start_ts: 20260807-002302
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-002304 | answer_regex,answer_contains | none | 114s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Runner oracle is too weak. The final answer reaches the right broad mechanism but three principal citations are detached/misbound and it claims the factory return is bound to `Logger::sink_` without a proved value-handoff bridge. The accepted completion carried 9 source-located members plus only 6 observation-shaped refs; member normalization shifted those refs positionally and later reported 9 supported rows. `EVAL-B241-MEMLOCREF1` is confirmed; `EVAL-B236-PHASEBRIDGE1` remains partial. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-002304 | answer_regex,answer_contains | none | 126s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | The final prose correctly identifies `JsonPlugin`, `run_pipeline -> resolve`, registration, instantiation, callback execution and MRO; one minor wording typo remains. JSON was valid. Two finalizer rejects came from an optional sequence diagram that ignored the copy-ready typed recipe, then used invalid flowchart-style `-.->` syntax and return/reply anchors with the wrong endpoint semantics. The model-owned removal patch converged, but `EVAL-B242-OPTDIAGCONV1` remains a retry-efficiency/context-teaching gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `EVAL-B239-BLOCKCARD1` did not regress: the two r152 cases compiled as call-chain families with unbounded principal list carriers and no contradictory capped optional carrier. No block-cardinality reject occurred.
- `EVAL-B240-CITEREF1` is only partially production-confirmed. The new unique explicit support lane is present, but this C++ replay exposed an earlier carrier corruption: source locations embedded in `members[]` were overridden by a shorter positional `support_refs[]` list before final citation repair ran.
- `EVAL-B241-MEMLOCREF1` is a generalized structured-carrier defect, not C++ fitting. A member-owned explicit coordinate must outrank a conflicting bare positional slot; otherwise any supported language can shift citations when a model emits an observation list shorter than the roster.
- `EVAL-B242-OPTDIAGCONV1` is not malformed JSON. The prompt already contained a complete typed Mermaid recipe and anchor array, but the optional-diagram retry did not front-load the simple model-owned choice “copy that recipe unchanged or remove the optional block” until after another failed patch.
- The Python rejection also preserves two previously filed guards: display message parameters must not alter typed endpoint identity, and unlabeled/invalid flowchart-like arrows must not bypass strict relation anchors. Neither should be fixed with language names or raw-answer keyword gates.
- No Trace runtime contract changed or participated in this replay. Explicit windows, auto-supplement, causal projection, root-cause ranking, wakeup chains, eliminable impact and the dual root-analysis dimensions remain outside these fixes.
