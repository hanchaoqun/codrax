# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T12:30:52Z
- sweep_start_ts: 20260809-053050
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-053052 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 205s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass-with-advisory | B434/B435 production closed: threadpool-400 on-chain iowait 11ms alone crowned; logger-900 remains background with effective column — and no root ordinal. Model nevertheless speculated an untyped indirect logger→threadpool contribution and misstated CPU5 as shared CPU4; typed projection stayed correct. |
| 1 | data_basic_sum_with_rules | FAIL | eval/results/data_basic_sum_with_rules-20260809-053052 | log_regex,answer_regex | none | 295s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | System teaching/schema told the model that aggregation requires decision_records_required=true. After amount_num=[10,7] materialized, that new sticky obligation withheld compute_contributions, while custom_transform was disabled; six batches/two repairs/four action failures ended blocked with the correct value 17 in hand. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### Trace authority replay

- The requested `2.000000..2.020000` window, target `app-100`, two model trace queries, deterministic root-cause supplement, and final causal projection all remained present.
- B434 is effective in production: the background `logger-900` rows retain 19.500ms/7.000ms occupancy context, but their effective-attribution cells are `—`; the typed-fact summary gives logger no rank seat.
- B435 is effective in production: only typed `on_chain` rows receive root ordinals. `threadpool-400 / iowait` is #1 at 11.000ms; the three 1.000ms runnable rows are secondary on-chain seats. Adjacent sleep and background IO remain outside root ranking.
- The model-owned prose still made one unsupported leap: it said logger background indirectly contributed to threadpool IO completion and claimed a shared CPU4 although logger was measured on CPU5. This did not alter the deterministic projection or crown. Existing soft guidance already forbids causality transfer from temporal overlap/shared CPU; the follow-up should sharpen the same typed-relation instruction, not scan or rewrite answer prose.
- There were zero finalizer rejects/rewrite rounds and no `成文校验未通过`. One investigation-complete schema repair was limited to the model omitting aggregate-fact `value` fields.

### Data workflow failure

- The deterministic runner had complete data: `order_records_numeric` contained `amount_num=[10,7]`; the answer was therefore 17.
- Before the faulty repair plan, reducer state correctly published `decision_next_actions=compute_contributions` and decisions were optional. B432 admitted the candidate against that committed snapshot as designed.
- The repair plan then explicitly set `decision_records_required=true` because both the tool schema and system prompt broadly taught “aggregation ⇒ decisions”. The first dependency rank (`derive_fields(parse_number)`) committed that obligation; the next transaction correctly enforced it and withheld deferred `compute_contributions` pending filter/qualify decisions.
- No eligibility/filter/classification decision actually existed: the rule explicitly used every row. Consequently there was no executable qualification scaffold, while free-form `custom_transform` had already been disabled. The workflow inspected schema again and terminated blocked after 6 batches, 2 repairs, and 4 action failures.
- This is not B432 regression and not model-only variance. It is `EVAL-B436-DECISIONTEACHINGSELFLOCK1=P0`: JSON schema and prose teaching contradicted the action capability registry, which deliberately states that compute produces derived audit rows but does not imply a decision prerequisite.
