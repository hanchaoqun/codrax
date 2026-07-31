# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T04:46:56Z
- sweep_start_ts: 20260730-214656
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260730-214656 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 132s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | Principal lead adopts authoritative count=3 and total=0.635ms, but the timeline lists only two waits (0.168+0.183=0.351ms) and invents that the missing 0.284ms may be outside the preview or a different kernel accounting method. Ground truth is three intervals 0.138+0.147+0.350=0.635ms. Analyzer emitted call_chain for one runtime focus identity (thread label + PID split into two entities), so the narrow-fact materializer guard was bypassed and an unrelated full causal report was appended. Automatic anchors do not detect this internal contradiction. |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-214656 | write_apply,answer_regex | none | 537s | 17 | read=22,repo_map=4,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Product patch and falsy tests are correct, but process is unhealthy: the first correct plan embedded a Python verification probe that shells out to missing `npx`; that infrastructure absence was classified as code verification failure and triggered repeated replan/explore/verify cycles. The replan needlessly changed the already-correct `"_prefault" in schema` implementation to `!== undefined`; only the final deterministic TestSurface fallback (`npm` missing → `make check`) proved the work in 42ms. Missing probe dependencies must be typed unavailable and fall through, not replan product code. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
