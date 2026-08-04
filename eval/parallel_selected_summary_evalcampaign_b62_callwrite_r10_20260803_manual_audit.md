# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T04:59:48Z
- sweep_start_ts: 20260803-215945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-215948 | answer_regex,answer_contains | none | 369s | 36 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=7/2,fin_reject=1,unavail=0,prune=0 | fail | Explorer eventually froze the correct typed boundary: `buildAnalysisIR -> gate.RunWith`, while `gate.Run -> RunWith` is the reverse wrapper and there is no requested-direction path to `gate.Run`. Finalizer first drew the false `RunWith -> Run` edge; the call-edge validator correctly rejected and the patch removed that edge, but the untouched summary still says RunWith calls Run, and system-added roster sections still place `gate.Run` inside a “call chain” list. Runner regex is too weak. Context/typed projection gap, not a reason to scan answer prose. |
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260803-215948 | log_regex,write_apply,write_patch_oracle | none | 378s | 19 | read=10,repo_map=3,list=5,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | First plan correctly fixed `src/client.ts`, but its Python probe used a stale relative test path and failed before assertions. During replan, a fresh typed planner probe passed against the already-applied worktree and the planner explicitly observed that the source was correct, yet the protocol still accepted a second same-path structured replace. It left duplicate `return res.json();` plus extra braces, producing invalid TypeScript. Node/tsc was unavailable and the Python source scan lacked typed target/contract authority, so final verification stayed incomplete. This is a write-safety P0; the output tree itself is corrupted, not merely unverified. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
