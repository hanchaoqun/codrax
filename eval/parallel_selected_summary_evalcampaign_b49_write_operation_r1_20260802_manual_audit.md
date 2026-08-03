# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T01:00:06Z
- sweep_start_ts: 20260802-180005
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260802-180006 | write_apply,write_patch_oracle,answer_contains | none | 106s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | applied tree 只有 `main.c` 一行 `retrun`→`return`；`make test` exit=0，path caliber=`project_runner`、capability=`target_behavior`，proof=`strong`，final=`verified`。 |
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-180006 | log_regex,answer_regex | none | 141s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | runner 只验标题/使用词/URL。第 5 轮 Python 明确只输出前 300 行，完整抽取停在 §3.1；final 却宣称八章完整提取。末轮达到固定预算后 material evaluator 被条件短路，typed 覆盖裁定缺席。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `patch_c_typo`

- patch 与请求完全一致，applied tree 相对 fixture 只有 `main.c` 第 19 行一处变更。
- repository-declared `make test` 真实执行并通过；ChangeReport 将唯一 changed path 标成
  `caliber=project_runner / capability=target_behavior`。
- FinalReport 为 `proof.status=strong`、`completion.verdict=verified`、delivery/source authority
  全部指向同一 plan 和 `main.c`。CAPCAL1 的成功路径获得 live 正证。
- planner 对不支持的 C probe 和 tab/space old_text 各经历一次精确 typed 修复，未产生额外改动。

### `operation_web_manual_summary`

- 首页 → `user_guide.html` 导航正确，link inventory、payload ref、source/excerpt 双截断上下文均准确。
- 原始手册 HTML 为 248,161 bytes / 4,103 行。最后的 Python extractor 明写“输出前 300 行”，
  生成 13,687-byte 工件，内容实际止于 §3.1；§3.2–§8 只有目录标题，没有正文覆盖。
- 最终答案却称“所有章节”“完整提取”，并给出未实际读取章节的概括。runner 的宽松 regex
  只看手册/使用/URL，构成 false green。
- 深层原因 `EVAL-B49-OPERMAX1`：CLI/REPL 都只在
  `commandRounds < commandOperationMaxCommandRounds` 时调用 material evaluator。第 5 个命令执行后
  直接进入 finalizer，既没有 complete/partial/budget typed 裁定，也没有给长材料继续分页的机会。
- 既有 `EVAL-B44-HTMLBODY1` 仍成立：任意 shell 抽取没有 source hash/range/page/remaining lineage；
  即使适当增加预算，模型仍无法机械证明多个页面已经覆盖完整源材料。

## 本批施工裁定

1. 末轮也必须运行 material evaluator；如果 typed 状态仍要求继续而没有扩展额度，发布
   `budget_exhausted`，finalizer 只能给部分结果。
2. 固定 5 轮改为 typed 自适应：只有 base-limit 轮存在 payload、evaluator 返回
   `continue_command` 且 material coverage 为 `partial/not_evaluated` 时，授予一次 5→8 的有界扩展。
3. 扩展只增加调查容量；每个 continuation plan 仍走原风险/审批策略，不能扩大执行权限。
4. 不扫描用户请求或最终答案，不替模型生成结论；HTMLBODY1 的 first-class bounded material
   reader/coverage ledger 继续作为下一独立批次。
