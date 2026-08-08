# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T12:37:38Z
- sweep_start_ts: 20260808-053737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260808-053738 | write_plan,write_patch_oracle | none | 66s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only 结果精确定位 `Main.java:16`，唯一 patch 为 `retrun`→`return`；change/slice 均为 1，目标路径只有 Main.java，未改仓库。验收声明 `javac Main.java`，但 plan fixture 不执行编译；一处 outcome 文案把 `greet` 写成 `gre et`，不影响 patch 权威，记低优先级表述噪声。 |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260808-053738 | log_regex,answer_regex | none | 443s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B349 没有 production witness：本轮无 foreign-param/schema-invalid。业务 ledgers 已正确形成 10 decisions、4 rules、4 contributions（GroupA=17/GroupB=4/GroupC=5）、10 resolutions 与 reconcile=pass；失败在 complete-reference projection。round 22 的错误 `17,4,5` 被 typed `reference_ledger_domain_mismatch` 正确拦截，未发布；模型随后把稳定槽 ID `target_id` 继续当作 contribution-domain key，且多次把实际 3 行 targets 想成 4 行。系统给出的单一 `reference_key_field` 载体没有把“输出槽身份/行序”和“与 group_key 对齐的 reference 字段”冲突显式结构化；最终失败批的空 answer 又把终态图从精确 domain-mismatch 降成普通 incomplete-reference。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Exactly two cases ran concurrently; no third case was launched.
- Human result: `write=PASS`, `data=FAIL`.
- The data failure is not an ACTIONPARAMSCHEMA1 regression. Runtime parameter ownership was not exercised by this replay.
- No wrong data answer escaped: `17,4,5` was rejected by typed grounding and the terminal status remained failed.
- Filed two separate generalized gaps: `EVAL-B351-REFERENCEFIELDROLE1` for the typed reference-field role conflict/compatible-domain carrier, and `EVAL-B352-FAILRESULTAUTH1` for loss of the last precise output-projection diagnosis after a later action fails without an answer.
- Neither gap authorizes scanning planner prose, guessing expected numbers, or system-side answer replacement.
