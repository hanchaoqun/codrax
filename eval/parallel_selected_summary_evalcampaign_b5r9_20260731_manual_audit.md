# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T16:14:33Z
- sweep_start_ts: 20260731-091433
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-091433 | log_regex,answer_regex,answer_contains | none | 125s | 42 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Batch N is covered: the visible deterministic table publishes one artifact pair and four independent `未证明` statuses, explicitly states that local identity and timestamp offsets do not prove a shared clock, and the generic comparison still has no Trace causal projection/full report. A secondary system cross-check falsely calls typed `clock_alignment` unsupported; filed separately. |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260731-091433 | typed_inventory_rowset,dimension_substring,answer_contains | none | 146s | 20 | read=8,repo_map=3,list=1,trace=0,source_lens=3 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Product answer is exactly correct at 2/2/8 with the requested symbol names, paths, packages, and citations. Runner false-FAILs because the case expected construct display labels `extend String/Cart` while deterministic row normalization correctly renders the symbol-name column as `String/Cart`; the current count is matched-expected rows rather than actual visible rows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### real_trace_e2_cross_trace_asymmetry

- Correct measured facts: `144.557ms` versus `0.556ms`, about `260×`; CPU-frequency rows `90` versus `0`; VSync `32` versus `0`.
- The user-visible `跨工件关系边界（系统确定性）` table contains exactly one pair and independently marks shared clock origin, direct time alignment, same device, and same capture session as `未证明`.
- The boundary text correctly says matching `trace_seconds`, endpoint-local `alignment=identity`, and a timestamp subtraction cannot establish a shared clock or direct alignment. No Trace causal projection, root-cause report, wakeup chain, or other unrequested report block reappeared.
- The model's hidden reasoning still speculated about different hardware/session, but accepted answer text does not publish that claim. No answer-prose hard gate is warranted.
- New secondary gap: the system cross-check appendix says `clock_alignment` is absent from the report evidence even though accepted deterministic SourceRef metadata and the system pair block both carry it. The lexicon board is not ingesting typed source-reference clock fields.

### cangjie_repomap

- Final visible rows are factually exact: 2 keyword `extend` symbols (`String`, `Cart`), 2 `foreign func` declarations, and 8 `public class` declarations, each with the correct path/package/citation.
- The runner's expected `extend String` and `extend Cart` tokens are construct display labels, not the symbol names the user requested. The system-normalized table correctly uses `String` and `Cart` in its `符号名称` column.
- The existing `EXPECT_INVENTORY_COUNT_*` compares the number of expected rows that matched, so it both false-fails legitimate alternate structured presentation and cannot detect an unexpected extra row if all expected rows are still present.
- Batch O therefore changes the case identity tokens to the requested symbol names and makes exact count consume actual table/list row cardinality inside the matching Markdown or bold section. When no structured scoped section exists it retains the legacy matched-count fallback.
- Analyzer did three source-inventory calls plus one list pass in this run versus one lens in r8. Since the final answer is correct and the variation did not repeat yet, this is recorded as model/process variance rather than a new hard route.
