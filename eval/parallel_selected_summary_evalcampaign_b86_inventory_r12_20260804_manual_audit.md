# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T16:00:56Z
- sweep_start_ts: 20260804-090054
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260804-090056 | dimension_substring,answer_contains | none | 64s | 19 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | 五个复合 declaration rows、package、精确 file:line 与五条 citation 全部一致；`principal_support_member_coverage=0`，没有“部分证据较弱”或“补充定位未执行”。B85-C production 见证通过。唯一 unavailable attempt 是 explorer 请求本轮不存在的 `emit_evidence`，不影响答案。 |
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260804-090056 | typed_inventory_rowset,answer_contains | none | 142s | 20 | read=3,repo_map=3,list=2,trace=0,source_lens=3 | midloop=1,inv=1/0,fin_reject=1,unavail=3,prune=0 | fail | runner 假 PASS。正确集合是四个 `@Entry` 页面与两个 `@Builder` 片段；答案错误加入无 decorator 的 `EntryAbility`，并被系统完整 role 投影强制追加 20 个无关 function/method rows。首稿本已只回答两个请求集合，pre-emit reject 才造成扩域。`entry_page` rowset ID 未绑定可见 `@Entry` section，count=4 退化为仅检查四个期望行存在，漏掉额外行。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `EVAL-B59-INVROW1=production-proven/closed`：Cangjie 原 witness 已验证复合行身份与机械定位 authority 修复生效。
- `EVAL-B86-SURFFAM1=confirmed/implemented-tests-pass/replay-next`：parser 已提供 `surface=@Builder`、四个 `@Entry` 行也有独立 marker，但旧 family 铸造只识别 base+specific 二元组，单 marker/并列 marker 无 family；coarse role 因此扩成 20 行。产品根修按 typed multi-family intersection 处理，不读取请求或答案 prose。
- `EVAL-B86-EVALROW1=confirmed/eval-fix-next`：typed inventory oracle 在 rowset ID 与可见 section 无同名标题时未执行 exact extra-row count，造成 runner 假 PASS。应由 case 显式声明 section anchor，不能用 runner 猜 `@Entry` 关键词。
