# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T10:10:34Z
- sweep_start_ts: 20260808-031032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-031034 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 137s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Typed projection preserves requested 34579.472865..34579.587805 window, wakeup path, on-chain-only crown, two-dimensional occupancy/eliminable surfaces, and adjacent/background separation. System coverage caveat nevertheless says all scheduler/IO/frequency observations are only background while the same projection has typed on-chain rows; duplicate alias publications also emit the same VerifyClass span twice. Model prose has one 59843→61839 typo and adds overlapping IO calibers, but the system projection does not copy either error. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260808-031034 | answer_regex,answer_contains | none | 191s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Source facts and uncertainty boundary are correct. The strict gate correctly rejects an optional diagram that mixed unsupported inferred/guard/value/call edges; model removes the diagram on one patch and retains the fully grounded prose/list answer. This is a diagram-guidance observation, not a reason to weaken typed relation evidence. |

Follow-up status (S37bt): the Trace partial's system-side duplicate alias publication is closed by exact typed semantic-event identity. The fold retains both E24/E25 and both locators, while distinct intervals/query domains remain separate; production replay remains pending. The model prose typo/non-additive IO sum remains a soft-context/model observation and is not rewritten or prose-gated by the system.

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
