# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T17:03:10Z
- sweep_start_ts: 20260828-100309
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-100310 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | The explicit 34579.472865..34579.587805 frame window, five typed trace queries, four-hop ThreadPoolForeg→NetworkService→CookieMonsterCl→target wakeup dependency, on-chain priority/scheduler, D/IO, supply, deterministic VerifyClass clue, actual-time versus rule-eliminable ledgers, business identities, adjacent/background separation, representative windows, and full Trace causal projection all remain present. The model also supplies a useful ranked summary and repair directions instead of leaving only a system evidence board. Two unsupported promotions remain: it says the target is repeatedly woken but cannot obtain scheduling, while the dominant runnable delays belong to upstream chain nodes; and it calls the target's CPU0/1/2 running-time sum (~26.2ms inside a 114.9ms window) “close to full”, which is not a utilization conclusion. These are B1388 observation-to-inference authority witnesses, not reasons to scan or rewrite final prose. No fixed 4ms/4m/stream-age downgrade occurred. |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-100310 | answer_regex,answer_contains | none | 186s | 28 | read=19,repo_map=1,list=0,trace=0,source_lens=1 | midloop=14,inv=5/0,fin_reject=0,unavail=0,prune=0 | partial | B1389 is production-positive: after the typed per-dimension closure rejection, the explorer read the real LoadRuntimeSettings body and emitted separate operation evidence for yaml.NewDecoder/KnownFields/Decode and the CLI Changed branch; the final answer no longer invents Viper and correctly reports default 50 and code→YAML→explicit CLI precedence. Process remains partial: the model first emitted ten unowned rows and attempted completion three times before learning to attach requested_dimension_indices, resulting in 19 reads/14 midloop turns. The answer also imprecisely says an explicit CLI value ultimately enters mergedMaxSteps; the implementation retains that value in flagMaxSteps while mergedMaxSteps is the code+YAML carrier. Record B1390 as initial typed-ownership soft-teaching debt; it must be driven by the existing dimension roster, not request or answer wording, and must not let the system author the explanation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
