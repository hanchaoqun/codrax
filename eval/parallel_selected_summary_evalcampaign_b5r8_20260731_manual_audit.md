# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T15:59:23Z
- sweep_start_ts: 20260731-085923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-085923 | log_regex,answer_regex,answer_contains | none | 135s | 36 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | T11 remains covered and all measured values are correct, but the typed pair authority reached the finalizer only as guidance. The answer still turns `unproven` into “time bases are different”, says numeric offset correction is the prerequisite for joint analysis, and ends with “does not share a calibration anchor” instead of “no shared anchor is present in evidence”. This is a product authority-publication gap despite runner PASS. |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260731-085923 | typed_inventory_rowset,dimension_substring,answer_contains | none | 160s | 20 | read=8,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Batch L is covered: the first source-inventory result and final answer contain exactly 2 keyword `extend` rows, 2 `foreign func` rows, and 8 `public class` rows; `@Extend(Text) highlight` is absent from the `extend` family but remains available to explicit marker-family lookup. The mixed English sentence “has 8 item(s)” is isolated model phrasing variance, not a factual or coverage failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### real_trace_e2_cross_trace_asymmetry

- Correct: `144.557ms` versus `0.556ms`; timestamp numeric difference about `31637s`/`8.8h`; `cpu_frequency=90` and VSync only in the first artifact; no source-code tools; no Trace causal projection or full causal report for this non-windowed generic comparison.
- Correct control plane: the finalizer prompt contains one typed pair with `shared_clock_origin=unproven`, `direct_time_alignment=unproven`, `shared_device=unproven`, and `shared_capture_session=unproven`; both local `alignment=identity` values are explicitly marked endpoint-local.
- Incorrect publication: the model nevertheless states “时间基准不相同”, “必须以数值偏移修正为前提”, and “不共享校准锚点”. The available evidence proves only that no shared anchor was supplied; it cannot prove same or different, and a raw timestamp subtraction is not a calibration transform.
- Generalized diagnosis: typed pair authority is prompt-only. A model can ignore it, while the accepted document has no deterministic pair-level authority block. Fix at the typed document materialization boundary; do not scan or rewrite user/model prose.

### cangjie_repomap

- The authoritative member sets and final answer close exactly at `extend=2`, `foreign func=2`, `public class=8`.
- The former ArkTS marker row no longer contaminates the bare keyword family. No guessed required file or invalid optional role narrowed the search.
- Eight `read_file` calls and three mid-loop injections are acceptable for this full-repository exact enumeration; there were no rejects, unavailable tools, or context-pressure symptoms.
- The sentence `public class has 8 item(s)` is a bilingual style wobble only. It did not alter any member, package, location, count, or citation and is therefore recorded as model variance rather than a new production hard gate.
