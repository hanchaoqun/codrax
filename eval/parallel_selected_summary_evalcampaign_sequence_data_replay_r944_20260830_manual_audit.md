# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T01:34:57Z
- sweep_start_ts: 20260830-183455
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260830-183457 | log_regex,answer_regex | none | 335s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-process-gap | Final JSON is exactly `{"ids":["u1","u3"]}` and preserves source order. The path is unnecessarily expensive: the dynamically narrowed rank excludes `custom_transform` but the shared action schema still exposes `script`; the planner first emits `custom_transform`, then emits `derive_rules` with a script, receives three structured repairs, and only then follows the legal typed DAG. This is a schema/teaching mind-load gap, not an answer error. |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260830-183457 | answer_regex,answer_contains | none | 495s | 34 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=5/1,fin_reject=8,unavail=0,prune=0 | fail-system-contract | Source investigation is correct: `buildAnalysisIR -> gate.RunWith` and independently `gate.Run -> RunWith`; no directed `buildAnalysisIR -> gate.Run` path exists. The first draft honestly shows the disconnected boundary, but one list mixes member-set and principal-path responsibilities. Repair then deadlocks: a stale-anchor failure carries `body_occurrence=1`, the resolver copies it into a carrier whose executor requires zero; in parallel the same block must move off-facet items while its local relation lease forbids whole replacement. Eight rejects end in degraded recovery despite a usable factual answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion and batches

- `B1457/P0`: make opaque `stale_anchor` and `prior_anchor_metadata` refs omit visible-body occurrence during resolution; keep occurrence only for carriers that own a visible Mermaid edge. Add a production-shaped non-zero producer-occurrence regression.
- `B1458/P1`: compile a required member roster into an explicit sibling `member_set` block when QFCallChain also owns `principal_path_edge`; the two typed responsibilities must not rely on prose-only separation or an impossible post-reject whole-block mutation.
- `B1459/P1`: project action-kind-specific fields into the executable-rank data schema. Typed actions must not expose `script`; `custom_transform` exposes it only when that action kind is currently admitted. Runtime guards remain fail-closed.
- None of these batches reads request/model/final prose for a hard gate, selects a relation or conclusion, weakens typed relation evidence, or touches explicit-window Trace projection/auto-supplement/root-cause authority.
