# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T20:14:56Z
- sweep_start_ts: 20260815-131454
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-131456 | write_plan,write_patch_oracle | none | 71s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct one-line `retrun -> return` plan. The planner emitted no verification probe, and the plan passed on its first structured emission with zero validation rejects/retries. Verify-owned Python dry-build passed. |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-131456 | answer_regex,answer_contains | none | 154s | 29 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | The answer correctly gives total `1`, member `explorer`, registration call, Name return, and current-scope boundary. B846 is closed: no unrelated naked-number citation was added. However the model chose a Markdown table in `block.text` without citation sidecar `items[]`, so three selected registration/return/init anchors were pruned and only the Name definition survived in the formal citation list. A generic system coverage caveat was also appended despite an already localized model boundary block. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B846-PATCHCITATIONIDENTITYREMAP1` is production-closed. The exact prior witness `internal/agent/explorer.go:19917` is absent from the emit, citation pool, and rendered answer. The derived count remains visible without being reinterpreted as a source literal.
2. `B850-PROBEOMISSIONGUIDANCECHURN1` is production-closed. This replay adopted the soft verify-ownership guidance completely: no probe, no plan rejection, no retry, and deterministic Python dry-build remained active.
3. `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1` remains partial through a broader transport shape. The schema legitimately permits a complete Markdown table in `block.text`, while citations live only on `items[]`; a cited Markdown row therefore needs a non-rendered item sidecar. The existing rule is present but easy to miss, and three consecutive production replays have now omitted registration support. The next safe improvement is one canonical soft rule: when visible table rows need citations, prefer structured rows, or add one citation sidecar per Markdown row; never synthesize a sidecar from table prose and never impose a citation-count hard gate.
4. Context audit found an independent analyzer error: `answer_role_profile` required `function,method` even though the principal members are registered subagents. This injected irrelevant candidate-role work and produced a soft advisory. It did not alter the answer, so it remains a model/context observation pending heterogeneous recurrence; do not hard-rewrite it from request words.
5. The model emitted a concrete uncertainty boundary block but omitted its typed facet metadata. The accepted path then materialized the generic answer-coverage family instead of a localized residual. This is a repeated UX debt, but a fix must use typed facet/violation data rather than scan the caveat prose; it is not closed by suppressing all uncertainty warnings.
6. No malformed JSON, draft fallback, empty answer, Trace mutation, system-authored conclusion, or active-stream fixed-age degradation occurred.
