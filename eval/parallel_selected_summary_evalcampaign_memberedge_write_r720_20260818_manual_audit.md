# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T06:20:03Z
- sweep_start_ts: 20260818-232001
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260818-232003 | write_apply,answer_regex | none | 271s | 26 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-fail-safe | The first patch added the explicit true/error force variable and new branch but left the old truthiness branch; the repository check caught it, controller replanned, and the second patch deleted exactly that stale line. The cumulative result is correct and preserves false/0/empty/undefined fallback plus true/error forcing. `make check` then passed. The runner FAIL is the honest final `source_static_only` verdict: the Python checker parses the generated template and TS regression source but never executes `renderNativeBinding`; Make does not run the .ts test. Do not promote this to target behavior merely to make the eval green. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-232003 | answer_regex,answer_contains | none | 295s | 30 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | B1144 and B1145 are production-positive. Analyzer emits diagram then key-function roster as two dimensions; the final renders the truthful shared-callee diagram first and the 18-item grounded roster afterward. The first draft already separates the member_set roster from the exact endpoint-edge carrier. Finalizer rejects fall from 3 to 1; the sole repair adds omitted edge_anchors to the already-selected endpoint-edge list and preserves all content. No synthetic buildAnalysisIR→gate.Run relation appears. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Conclusions

- Machine status is 1 PASS / 1 FAIL; human audit is one read pass and one correct fail-safe write delivery. A machine FAIL is not evidence that the code fix is wrong when the verifier intentionally refuses to overstate a static checker as target execution.
- B1144 is production-closed for the requested layout: the diagram is rendered before the independently requested key-function roster. The system uses typed dimension order as authoring context and does not rewrite or reorder final blocks itself.
- B1145 is production-closed: the first draft uses one exact endpoint-edge block and a separate descriptive member_set block. The prior three-round facet ownership collision is absent.
- One finalizer reject remains because the model omitted edge_anchors from a standalone endpoint-edge list while keeping the directed claim. The exact typed gate repaired this in one patch without deleting relations or the roster. Treat it as a replay observation, not yet a new hard-gate batch.
- The TypeScript write result is semantically correct across two cumulative plans. Verification correctly detects the stale first-patch branch, drives a replan, and passes after deletion. Its final unverified verdict is also correct: the Python checker performs strong source/test-shape analysis but does not execute the TypeScript target behavior. Cross-language declared project checks must stay source_static unless a separate typed target-runtime/probe result exists.
- No malformed JSON recovery, empty answer, system-authored conclusion, active-stream fixed-age fallback, or Trace-path regression occurred. Read/write separation remains intact.
