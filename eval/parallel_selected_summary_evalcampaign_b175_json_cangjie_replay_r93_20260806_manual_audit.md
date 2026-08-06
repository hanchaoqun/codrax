# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T11:24:15Z
- sweep_start_ts: 20260806-042414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260806-042415 | log_regex,answer_regex | none | 50s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact answer `{"ids":["u1","u3"]}`. One admission repair remained because the model declared `instructions.md=script_consumed` but omitted it from the initial script; the typed guard correctly rejected that plan and the repair read both declared inputs. No failed action, malformed/decode event, semantic ledger expansion, or answer salvage. 50s versus r92 196s confirms the two deterministic contract loops are gone; one honest material-consumption repair remains model-plan variance, not a system contradiction. |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-042415 | typed_inventory_rowset,dimension_substring,answer_contains | none | 214s | 24 | read=8,repo_map=3,list=1,trace=0,source_lens=3 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | Final answer is complete and correct: 2 extend, 2 foreign func, 8 public class rows with package/path/line and 12 citations. Runner false red came from the first marker revision matching `extend ` inside the public-class Cart description; narrowed cell-start markers make this exact artifact pass and retain extra-row failure. Process audit found a real contract conflict: prompt Principal Enumeration Rows exposed partition row IDs (`enum-set-foreign-func-...`) while the validator required an unshown synthetic-global ID namespace (`enum-set-source-inventory-principal-rows-...`), causing two retries. The first call also used a JSON-encoded `blocks` string; lossless recovery preserved all blocks, so there was no missing answer or user-visible degradation. Two wrong row-local citation refs remained soft advisories; sources were all present, but exact row binding needs follow-up. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
