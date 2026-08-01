# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T00:59:04Z
- sweep_start_ts: 20260731-175902
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260731-175904 | answer_regex | none | 136s | 18 | read=4,repo_map=1,list=1,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Core chain is correct and the typed ledger contains all three call edges plus the fallback edge, but the rendered citation refs are shifted: the FastTokenizer definition row cites `text.encode`, the native call row cites the Rust module declaration, and the fallback is mentioned only in the summary rather than emitted as an evidenced branch node. Runner regex did not detect the relation-to-citation mismatch. |
| 1 | real_trace_h3_iofam_one_seat | FAIL | eval/results/real_trace_h3_iofam_one_seat-20260731-175904 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 184s | 43 | read=2,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The analyzer emitted valid exact `time_start/time_end` and an anchored quote but mislabeled the scope `bounded_selector`. Parsing then discarded the times. Supplement skipped as `families_present`; focused-runtime-fact publication suppressed the deterministic projection/IO-family block. The model consequently replaced typed IOFAM display with free prose, omitted all hard composed words, mislabeled the blocked-reason duration caliber, and treated ranking absence as a metric. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## B14 r1 Findings

### H3 — typed scope contradiction suppresses exact-window authority

The raw analyzer emit carried all of the following at once:

- `requested_scope=bounded_selector`;
- `time_start=13762.791708`;
- `time_end=13763.024898`;
- an exact, current-request-anchored window quote.

`parseRuntimeArtifactScopeProfile` accepts the enum arm first and clears both
times for `bounded_selector`. Downstream `ExplicitTimeWindow()` therefore
returns false. This is a typed-shape contradiction, not absent evidence.
It suppresses the exact-window override in both supplement selection and
answer-report materialization. No raw request or final-answer scan is needed
to fix it: anchored `bounded_selector` plus a structurally valid typed
start/end pair is the precise signal for canonicalizing the profile to
`explicit_time_window`.

The final answer contains trace-derived values, but it lacks the deterministic
IO family seat and therefore fails every hard IOFAM display invariant:
`IO延迟`, `块设备层`, `块设备IO(inode)`, `综合评分,非墙钟`, and
`完成端到端·IO延迟（io_latency）`. It also calls the
blocked-reason interval sum a non-wall-clock internal estimate; that conclusion
does not follow from the typed caliber. This is a downstream symptom of losing
the authority block, not a reason to add prose keywords or case-specific
rewrites.

### Polyglot — relation ledger complete, rendered grounding incomplete

Exploration and the finalizer prompt carry:

- `FastTokenizer.tokenize -> encode`;
- `FastTokenizer.tokenize -> _fastlex.tokenize_bytes`;
- `_fastlex.tokenize_bytes wrapper -> core tokenize_bytes`;
- `FastTokenizer.tokenize -> _tokenize_slow` under the false native guard;
- `_fastlex` registration.

The answer states the correct mechanism, so the runner passes. Human grounding
still fails: ordered rows use citation indexes that point at neighboring
claims, and the fallback edge never becomes its own evidenced branch row.
This is a generic relation-node/citation semantic-alignment debt across
call-chain answers. It is lower priority than H3 because the conclusion is
correct and this run shows no destructive source/write consequence.
