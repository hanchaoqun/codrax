# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T08:06:22Z
- sweep_start_ts: 20260814-010620
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260814-010622 | answer_regex,answer_contains,mermaid_edge_count | none | 291s | 38 | read=7,repo_map=4,list=0,trace=0,source_lens=1 | midloop=5,inv=4/0,fin_reject=1,unavail=0,prune=1 | partial | B773 is production-positive for display selection: the accepted principal diagram removes r473's disconnected append/ToolResults/RepoFacts fragment, keeps all six requested participants visible, and invents no bridge. One precise retry was still needed because the first model draft omitted BusContext/Mutable boundaries. The prose continues to describe a fuller stage-to-carrier flow than the typed graph proves, despite accurate unproven-spine context, so the answer is not a complete human pass. |
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260814-010622 | write_apply,answer_regex | none | 506s | 24 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail-safe | The first applied plan failed the repository's Python source-shape checker and correctly replanned. The second plan passed that checker, but no Rust compiler/runtime exists and the Make target declares Python execution only, so all Rust paths remain source_static and the controller correctly refuses verified delivery. Manual code audit finds a real exact-boundary bug: `current_length.checked_sub(n)` yields `Some(0)` when n equals the remaining length, then `idx.checked_sub(1)?` returns before `self.length = self.index`; a subsequent next can still yield an item. The honest unverified verdict prevented a false green. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Process and answer audit

### qf_logic_view_read_pipeline

- The B773 soft guidance reached the Finalizer only from typed `requested_relation_spine_status=unproven`; it did not scan request or answer prose and did not alter validation.
- Compared with r473, the final model-authored graph now prioritizes the requested participant slate: Orchestrator, Analyzer, Explorer, Extractor, Finalizer, BusContext, and Mutable are visible. Only the proved `BuildAgentContext` call and four-stage precedence subset are connected. BusContext/Mutable remain disconnected and are disclosed as unproven; no stage-to-carrier edge is synthesized.
- The first draft still omitted the two participant boundaries and was rejected once. The patch added the exact visible disconnected nodes and boundaries without rewriting other blocks. This is normal precise repair, but it proves the soft guidance is not a deterministic authoring substitute.
- The narrative claims that stage outputs and several artifacts flow through BusContext.Mutable as a complete pipeline, while the same answer's typed diagram says those participant relations are unproved. Finalizer already received explicit “do not call this the complete requested flow” guidance. Treat the remaining prose overclaim as repeated model-adherence/context-salience observation, not permission for a prose keyword gate or system rewrite.
- Explorer still closes after proving local operations incident to carriers rather than a request-spine bridge. The completion-vs-finalizer authority divergence remains a separate design item; do not add unbounded bridge searches until both layers share a typed request-spine provider.

### github_issue_pyo3_iter_nth_overflow_symptom

- The write route preserved controller risk/approval/worktree gates. Medium risk auto-executed; main was untouched. The first plan attempted an unsupported `verification_probes[].language=rust` despite the projected enum and explicit teaching. The exact schema rejection recovered by removing that probe; this is model adherence/cost noise, not a contradictory contract.
- First verification ran repository-owned `make check` and failed because `nth` used `checked_add(n)?`, returning before iterator exhaustion. The controller carried the exact failure into `replan_batch`, applied a second bounded plan, and reran the same project target successfully.
- The passing command executes `python3 tests/check_iterators.py`, a source/text oracle. `TestSurface.declared_execution_language_families=[python]`; changed Rust files therefore have `caliber=declared_project_check, capability=source_static`. The controller's `production_verification_source_static_only` downgrade is correct and must not be relaxed merely to satisfy Runner PASS.
- Static success is also insufficient in fact. For `nth_back(n)` with three remaining items and `n=3`, `current_length.checked_sub(n)` is `Some(0)` and `idx.checked_sub(1)?` returns `None` before the exhaustion assignment. The checker covers `n=10` and `usize::MAX` but misses the exact remaining-length boundary. The cumulative patch is therefore not ready to merge even though the shape checker passes.
- Register `B774-WRITEEVALNATIVEPROOF1/P1-eval`: correctness campaigns should pair write cases with an available native execution surface when judging implementation quality. Source-static fixtures remain valuable fail-closed tests, but cannot produce a human correctness pass or authorize a product change that promotes their caliber. The generalized remedy is a real language-native project runner/fixture or an explicitly unavailable verdict, not case-specific source regex hardening.

## Verdict

- Runner: 1 PASS / 1 FAIL. Human: QF partial; write fail-safe with an incorrect cumulative patch correctly withheld from verified delivery.
- B773 display selection is production-positive and materially reduces internal implementation clutter, while model prose adherence and Explorer request-spine closure remain open observations.
- The write FAIL is an honest verification boundary, not an empty answer, workflow loss, or reason to weaken gates. No model/system answer takeover, malformed JSON fallback, or active-stream fixed-age degradation occurred.
- Next write eval should use a Go/Python/JavaScript fixture with a real runtime runner so implementation correctness and workflow closure can both be audited. Trace explicit-window causal projection and auto-supplement were not involved or changed.
