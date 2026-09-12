# Selected Eval Manual Audit Scaffold

- date: 2026-09-12T09:32:47Z
- sweep_start_ts: 20260912-023247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260912-023247 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 243s | 60 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=1 | fail | 五段库存正确；模型漏算另两Binder对端、泛化长睡眠、误称供给/反转；另有系统非墙钟残差附注缺口，见下文。 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260912-023247 | write_apply,write_patch_oracle | none | 257s | 28 | read=9,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (requested finite inputs) | 原测试4/4、生产局部修改、原测试字节不变；最终1计划0probe，未触发B1561累计顺序；Python整块编辑教学缺scope限定另修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Frozen execution and priority

- Source `b6b5f1cf5f92`, post-commit build `2026-09-12T09:32:14Z`; snapshot `.codrax/tmp/codrax-selected-20260912-023247`.
- Start `09:32:47Z`, complete `09:37:04Z`. Exactly two parallel cases, each once; original case/oracle/CAP5/1200s unchanged. No third run, no source/build edits during live, no rewrites of original outputs or proof.
- Inventory: 243 cases, 215 read / 25 apply / 3 plan. Selection balances current proof-order witness, high-impact explicit-window protection, coverage age, and executable environment. H1 last ran in r1056. A new write run need not reproduce the previous model's plan/probe choices; absence of that branch is not verification of it.
- Outer sweep reports write 257s; case wall is 254s. Do not combine these different timers. Machine summary is preserved verbatim and does not include all planning/tool retries in its finalizer-reject column.

## Write: patch and verification

Result directory: `eval/results/github_issue_dateutil_relativedelta_float_symptom-20260912-023247`.

1. Final applied plan `plan-1789205792567571000-23903` changes only `relativedelta.py`, with a local constructor patch. Integer-valued floats become integers; finite fractional values raise `ValueError`; existing integer arithmetic remains unchanged. `test_relativedelta.py` and README retain their original bytes.
2. Product execution genuinely ran `python3 -m unittest "test_relativedelta.py" -v`, four native tests passed. Final verification is `passed`; proof ledger is `verified`, profile `strong`, with two covered path obligations and one advisory convention row. There are no declared hard behavior-contract obligations in this final plan: this is not evidence that every imaginable behavior was executed.
3. Final plan contains zero verification probes and no applied/reverified earlier plan. Therefore the newly delivered B1561 long-failure display and cumulative-before-advisory ordering were **not naturally exercised**. Original r1062 offline proof-order RED/GREEN remains the direct witness; its old true failures and failed verdict remain unchanged.
4. Planning history is not zero-retry: duplicate test path, missing range end, two `scope=micro`/whole-file `modify` rejections, and preservation of the old regression test each required repair. In the main log, L1991 duplicate path; L2062 missing `end_line`; L2093–2107 an accepted draft later sent back before apply; L2326–2327 original test statements had been refactored into temporary variables; L2513/L2569 the two micro-scope rejections. Do not label the first two as JSON-carrier rejections merely because the submitted payload also contained a string carrier. That existing conservative baseline gate was not proven incorrect by this audit. The model ultimately removed the unnecessary test rewrite.
5. Confirmed systemic teaching gap **B1665/P1**: `defaults.go` advises micro edits to use patch, immediately followed by Python indentation advice that offers full `modify` without a scope condition. The same unsafe suggestion exists in both plan schemas and the Python EOF retry diagnostic. This is conflicting guidance, not proof that the two deterministic micro-scope rejections were themselves contradictory. Fix all four outlets using one scope-aware edit explanation; retain the validator and broader-scope full-rewrite capability.
6. Independent post-hoc `.codrax/tmp/20260912-r1063-dateutil-posthoc.{py,log}` (Python 3.9.6) passed native 4/4, 280 date/datetime integer/whole-float combinations, 12 finite fractional-value cases, six zero/large finite constructions, and three non-date rejections. NaN raises `ValueError`, infinities raise `OverflowError`; do not claim all non-integral special values use one exception. This is **not product proof** and was not inserted into the original report. Nine original fixture/delivery/plan/report/final files had unchanged SHA before/after.

## Trace: original facts, context and answer

Output `.codrax/output/20260912-023647.624-23899.{md,html,root-causes.json}`. Main log is the result directory's `run-1.logs/codrax-20260912-023249-000-23899.log`. Original fixture physical lines below differ by one from the attached copy's coordinates; do not silently interchange them.

| Original interval (seconds) | ms | Peer TID / TGID | Original physical lines |
|---|---:|---|---|
| 13762.835861–13762.837270 | 1.409 | 10961 / 9743 | 4356 → 4443 |
| 13762.894496–13762.895420 | 0.924 | 10961 / 9743 | 12562 → 12618 |
| 13762.937527–13762.937595 | 0.068 | 10625 / 9432 | 16946 → 16976 |
| 13762.950638–13762.950758 | 0.120 | 10961 / 9743 | 18524 → 18549 |
| 13762.951138–13762.951711 | 0.573 | 11354 / 9743 | 18611 → 18668 |

1. The five closed intervals are disjoint and sum to **3.094ms**, three peer threads across two TGIDs. Final MD L27–31 correctly lists them. Input context L1819–1823 explicitly provides raw wakeups 3+1+1; L2447–2452 and L2707–2712 provide the complete five-entry inventory plus the boundary that an unassociated wait does not prove non-Binder/voluntary sleep.
2. Model MD L15/L21/L1027 nevertheless claims all peers are 10961 or only three Binder wakeups. The error already exists in the model's original structured emit (log L2954), not a system rewrite. This is an answer-consumption error, not missing trace evidence. Do not add a prose-keyword gate or silently correct its conclusion.
3. MD L39 extrapolates the 15.758ms app-9511-woken interval (original L23091→24430) to all long sleeps being voluntary pacing. Another **14.302ms** interval is woken by DetectViewRect-17679 (L24820→27467) and explicitly supplied at log L1809. The answer omits it. Transaction 12145963 ends at 13762.895420, not in the claimed 13762.937–13762.952 range. These are model errors; no current inventory gap was demonstrated.
4. Protected system capabilities remain: exact 233.190ms window; state account 157.248 running + 5.604 runnable + 70.338 sleep; actual-occupancy and rule-based recoverable-time axes kept separate (MD L60+); one Trace causal projection (L88+); business span clues; adjacent/background sections (L148–158) not promoted into root ranking. IO-marker zero correctly does not exclude independently completion-proven S-state IO waits.
5. Default root-cause sidecar exists, `status=available`, six model-selected entries, 10,640 bytes. Ordered impact values are 58.320 / 12.658 / 7.405 / 4.710 / 3.956 / 3.605ms. Model descriptions incorrectly call 2.34GHz a policy cap and runnable candidate components proven inversion wait; accompanying system evidence retains thermal-versus-policy and unproven-inversion qualifications. Selection/persistence works; descriptions are not fully correct.
6. Format/repair: analyzer rejects twice (stringified object shape, then inconsistent causal scope/fact families); finalizer rejects once because two tables have headers but no visible rows. Model then emits complete rows and later applies one metadata patch. Exact outer `schema_version` re-homing succeeds twice; no missing answer or missing root selection. The root table's generic “列1…6” is **model header omission**: original emit L2954 had columns/no rows; full retry L3011 had rows/no columns; final patch L3065 restores only Binder headers and preserves the root table. No Mermaid is emitted, so Mermaid parse/render is **N/A**, not a rendering pass.
7. All five model trace queries use the requested exact window; one read inspects an IPC result, not client source. Runtime supplementation and projection are retained; this single-window case does not cover B1626 multi-request-window identity.
8. **B1666/P1 confirmed system gap:** MD L98 claims up to 30.697ms residual is explained by own IO row `[E46]`; E46 at L655–666 is page-cache churn, explicitly a non-wall-clock count equivalent (raw member sum 119.100). `trace-query-result-a6c3ba0a.json` item 16 carries `type=page_cache_churn`, `tier=caliber_side`, `chain_relevance=self_caliber_side`, `member_fold_caliber=count_sum`; 84.300+34.800 is capped to 81.616, still not time. `runtimeTraceProjOwnCaliberIOPrimaryRow` selects the broad IO-family value without dimensional exclusion; `runtimeTraceProjResidualOwnCaliberNote` prints min(81.616,30.697) as residual milliseconds. Thus a quantity correctly labeled non-time elsewhere is still consumed by the residual-overlap explanation. This is not a model statement and not dismissed as model fluctuation. Scope of the fix: use existing typed caliber/registry authority at the shared selector, keep count/score rows visible but out of residual-time explanation, preserve wall-clock positive controls. Root-cause ranking, original accounts, and model prose must not be altered. B1439's already-correct input units and B1622's physical-count issue are distinct; do not reopen or mislabel them. Broader mathematical sufficiency of legacy wall-clock overlap claims remains separately auditable.

## Stream protection and next work

- This batch's LLM requests report first-response 10m, true stall 5m, non-stream request 10m. Existing active-stream regressions were independently rerun: `.codrax/tmp/20260912-r1063-active-stream-count3.log`, pass 37.699s. Heartbeat/hidden reasoning remains progress; 4ms or the retired four-minute no-visible-answer age is not a downgrade trigger. Real silence, explicit caller cancellation/deadline and independent outer budgets still apply. These short live calls are not a live >4-minute single-stream witness.
- Priorities: close B1665 shared teaching outlets; verify the residual non-wall-clock display source separately; continue B1626's complete member-window vertical slice. B1561 exact-contract/execution residuals, B1651b/B1662 native-runner debts and other existing open items remain open. Original machine 2/2 is preserved; human verdict is finite-input write pass / Trace fail. No third eval is added to seek green.
