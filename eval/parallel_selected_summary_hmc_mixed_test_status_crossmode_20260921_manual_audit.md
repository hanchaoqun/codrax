# Mixed-invocation verification: fixed cross-mode manual audit

- date: 2026-09-21T10:44:24Z
- sweep_start_ts: 20260921-034424
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_mixed_test_status_crossmode_20260921

Frozen build: `1b9185049056`, built `2026-09-21T10:44:09Z`. Runner session77298 formally exited 0; machine 1/2, manual 1/2. Two concurrent cases, one attempt each; no oracle changes or third same-version case. This audit does not close broader source-free proof binding or previous Trace failures.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | nested_python_increment | PASS | eval/results/hmc_mixed_test_status_crossmode_20260921/nested_python_increment-20260921-034424 | write_apply,write_patch_oracle,answer_contains | none | 116s | 28 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS, functional delivery only | Native three-assertion PASS coexists with root discovery zero; controller and final verdict agree. All nine PTO contracts remain planning-only; this is not successful required-proof binding. |
| 2 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_mixed_test_status_crossmode_20260921/trace_query_business_marker_io_chain-20260921-034424 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 279s | 46 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | Missing 35ms request duration and LoadDocumentIndex business clue; whole-query 6ms Running misdescribed as post-wakeup running. Projection survives; background backup stays outside the causal chain. |

## 1. Python: delivered change and actual §84 branch

The only delivered source change is `packages/widget/widget.py:2`, `return value` → `return value + 1`. Seed HEAD `ab2be09b659906e6b8a0040db21ae907fd436885` is unchanged; isolated applied commit is `1a7f0b44121c7bc5e40fcfeaa206a6ba880d47dc`. Original tests, setup and test initialization bytes remain unchanged (test SHA256 `504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`). Runtime `.gitignore` gained `.codrax/` in the seed working directory; this is disclosed and is not part of the delivered source patch.

Plan `plan-1789987539657384000-3732` executed `python3 -m unittest "tests/test_widget.py" -v` from `packages/widget`: three methods/seven inputs PASS, exit 0. Root syntax compilation passes; root discovery reports zero tests. The exact empty invocation remains in the report, but no longer overrides the actual nested assertions. The controller receives passed verification, enters `batch_verified`, and the final result truthfully reports three passing native tests. One emit, one apply, one RunTests; no inline probe or repeated verification. Actual tool output is at `run-1.logs.all.log:2553`, controller input at `:2674`, final result in `run-1.out:105`.

Proof limitation: all nine declared contracts are `planning_only_ungrounded`, required=0. The assertion IDs still omit the non-root qualified prefix; the short suite is legal under existing suffix matching. No required contract was satisfied and no missing required proof was waived. This functional PASS verifies the mixed empty/native status fix, not HMC B2–B6 or successful exact-identity teaching. Successful full identities remain insufficiently visible in controller/planner context; keep that follow-up open.

## 2. Trace: retained causal chain, incomplete and partly incorrect primary answer

Ground truth from `eval/fixtures/hmosperf_business_io_chain/events.systrace`:

- `OpenDocument`: 1.000–1.050s, 50ms; its own states are 5ms Running + 1ms runnable + 44ms S.
- The expanded query 1.000–1.051s instead contains 6 + 1 + 44 = 51ms. These are different denominators, not interchangeable response accounting.
- `LoadDocumentIndex`: 1.0045–1.0445s, 40ms; states 8 + 1 + 31ms.
- Request issue→complete: 1.005–1.040s = 35ms. The worker's S interval ends at its wakeup: 1.009010–1.040010s = 31ms. Each subsequent worker/main runnable interval is 1ms.
- Backup request 1.002–1.049s = 47ms is a separate background request, not a root cause of this response.

The primary answer correctly retains the 50ms outer marker, worker's 31ms on-chain IO wait, two 1ms scheduling intervals, and `storage-irq → worker → app-main` chain; it excludes backup from direct attribution. It fails to state the 35ms request lifetime or name `LoadDocumentIndex`, despite reporting the child interval's 40ms and 8/1/31 state values. It uses the 51ms query breakdown instead of the business-window breakdown, and calls all 6ms of query Running “after wakeup,” which is false. It also exposes `completion_closed` and `issuing thread` without ordinary-language explanation. No answer-text hard gate is introduced to compensate.

Process: nine Trace tool calls, five emit_analysis attempts/four rejections, one final work-relationship patch; no hard finalizer rejection. The accepted analyzer object remains `causal_diagnosis` with `work_attribution_required=true`, and the final artifact contains a Trace causal projection. Thus this is not another confirmed loss of causal classification. The first analyzer failure was an illegal top-level `type` property; later feedback exposed a distinct teaching problem: it offered a unique bounded-effect repair even while exact root-cause classifications conflicted. Following only that partial repair would fail the next consistency check. The model eventually supplied a causal role, but relabeled the previous effect role instead of retaining both; the prose nevertheless answered the backup-effect question. Fix the contradictory hint without granting new causal authority or forcing finite questions into full diagnosis.

Final receipt: markdown SHA256 `9ad6a94d209eb1983ee6227f5369e885814120bbf16cfcca1525a1199c2b6ccd`, primary/answer SHA256 `015e9aa00796f5c246ebd9f7b2fd79fdfe4119f4c4a00e075c508e2893786db6`. The machine correctly reports the two missing primary facts.

Actual final-context audit: `run-1.logs.all.log:3084–3085` includes both named business intervals and their own state partitions (50=5+1+44, 40=8+1+31); `:3098` includes request 35ms, issue/complete endpoints, completion-to-issuer wake proof, blocked 31ms with separate endpoints, and a no-addition instruction. `:3263` separately identifies the query's 51ms=6+1+44. The finalizer initially receives four messages and retains them when appending to seven (`:3350→3424`), with no prune. The omissions and 6ms misuse are therefore model errors after adequate evidence supply, not lost collection/context. The model-authored root description also calls the request approximately 31ms; do not silently repair that prose from a keyword match.

System artifacts: `.codrax/output/20260921-034901.351-3723.root-causes.json` is schema2/available, with three on-chain selections, 0.031/0.001/0.001 seconds of effective attribution, the correct query scope 1.000–1.051, and no backup selection. Its model-owned description at line18 is still wrong about request duration, so the complete sidecar is not signed semantically clean. The system projection retains both business names/50ms/40ms in its business-clue section, and the background 47ms remains separate (`run-1.answer-transcript.md:114–125`). It reveals a separate deterministic wording gap: the overview maps all `io_latency` rows to “IO阻塞·设备延迟,” although this row's 31ms is issuer blocking rather than device/request service time. Preserve the established IO阻塞 family root, values, ranking and authority; review the unsupported device suffix independently, not as a workaround for the model's missing 35ms sentence.

Reference re-read: `core/preprocess/sleep_ops.py:558–625` follows exact wake anchors and clips each S/D segment to its own recursive window before computing its duration. That supports separate request/state windows; it does not justify merging 35ms and 31ms or replacing child business names. Its unknown/interrupt-waker dropping behavior is not copied into Codrax, which has typed completion-closure evidence here.

## Disposition

Keep historical FAILs and the parent 13/79 delivered, 66 open unchanged. Close only the narrowly verified mixed-invocation defect after the reviewed line-budget adjustment and replacement full suite pass. First fix the exact contradictory analyzer repair teaching, then continue native identity teaching/visibility and B2–B6; use context evidence to decide whether the Trace omissions require a producer fix or are model failures. No same-version third live case is authorized by this receipt.
