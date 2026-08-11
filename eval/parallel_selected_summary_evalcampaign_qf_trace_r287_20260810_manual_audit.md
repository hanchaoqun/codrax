# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T02:16:42Z
- sweep_start_ts: 20260810-191641
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260810-191642 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 142s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The model lead is useful and calibrated: it keeps the selected 114.940ms window, names the typed wakeup path, ranks only on-chain seats, separates adjacent/background load, and retains runnable/supply, frequency, D/IO and VerifyClass semantic work plus business-bearing thread/span names. Frame causality remains explicitly unproven. One independent system-output gap remains: the final generic supplement says to verify against source code despite the explicit trace-only boundary. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-191642 | answer_regex,answer_contains,mermaid_edge_count | none | 221s | 39 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=3,unavail=0,prune=0 | fail | Runner false positive. The accepted diagram honestly keeps only three typed stage-precedence edges and disconnects Orchestrator/BusContext/Mutable/StageOutput, but prose still asserts the unproved carrier flow. B498 v1 was falsified in production: the 105KB finalizer prompt still contains explorerSearchCache and write-policy lexical groups plus an unrelated BaseAgent flow because the full support plan admits same-file near-line and broad sibling endpoints. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r287 judgement

- Runner: 2/2; human: 1 pass / 1 fail. No model JSON recovery was needed. QF spent three finalizer rejects repairing an initially unsupported carrier graph; Trace emitted once.
- Trace preserves the explicit time window and the two requested performance axes: actual on-chain occupancy/semantic work and rule-derived eliminable impact. The crowned seat is CookieMonsterCl priority-inversion candidate at 23.994ms effective attribution; ThreadPoolForeg D/IO, target frequency deficit, runnable delay, and the 0.285ms VerifyClass span remain separate. Neighbor/background rows are visibly non-principal and frame causality is `unproven`.
- `B499-RUNTIMEGENCAVEAT1/P1`: after that correct trace-only answer, a system supplement still emits “建议结合源码进一步核对相关组件”. This is not model prose and directly contradicts the typed runtime-only/current-source-optional boundary. It is the concrete recurrence of the old open `E20260522-G13/G33/G153` family; suppress the generic caveat from precise runtime-only accepted answers rather than scanning request text.
- `B498-SUPPORTSCOPECTX1` v1 is not closed. Production showed that whole-plan scope was itself too broad: `explorerEvaluator` at line 69 made cache helpers at lines 45–64 pass the ±24 location lane, while broad facet endpoints admitted write-policy flows. The refined implementation now makes lexical order owner-local and, for required diagrams, derives optional flow/lexical scope from the same bounded `Principal Support Path` shown to the model. Full `internal/agent` suite is green; production replay remains required.
- The Trace model mentions CookieMonster/NetworkService/ThreadPool and LacUtils, but does not develop a separate business hypothesis beyond the typed names. Keep this as a soft model-quality watch: the current answer still preserves the clues, and no precise typed field authorizes the system to write the business conclusion for it.
