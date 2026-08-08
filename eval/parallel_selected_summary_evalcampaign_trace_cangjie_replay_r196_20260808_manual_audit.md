# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T07:43:41Z
- sweep_start_ts: 20260808-004340
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260808-004341 | typed_inventory_rowset,dimension_substring,answer_contains | none | 139s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | PASS | 两个同名 `native_add` 的最终引用分别稳定落在 Bridge.cj:6 与 07_foreign_ffi.cj:6，正确 citation 未再被 aggregate repair 覆盖；12 行清单、family、package 与计数完整。导语把 `.cj` 清单写成 “Cangjie/ArkTS” 属轻微语言范围泛化，记 B335 观察，不影响成员事实。 |
| 1 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260808-004341 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 185s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | PARTIAL | 系统投影中 logger 的 19.5/7ms 仍完整可见，但 Attribution/Chain total 都为 `—`，链上 11ms 正常加冕，B331 生产闭环。模型却从同时 D-state/IO 与压力分推断“共享 IO 资源竞争导致 threadpool 变慢”，并自行相加成 30.5ms；trace 未证明同一设备/资源/竞争边，记 B334。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### `cangjie_repomap`

- 模型再次发出两个正确 citation_ref；S37bh 在弱 aggregate repair 前分别铸造 exact `source_inventory_row_id`，最终两行可见引用与各自行文本一致，正确 corpus citation 未再被裁掉。
- 2 extend / 2 foreign func / 8 public class 的 typed rowset、package、file:line 和完整性均正确，零 finalizer reject、零修复循环。
- summary 把只含 `.cj` 的清单称作 “Cangjie/ArkTS 源文件”。这是模型范围措辞轻微扩张，未污染 typed 成员；暂记 `EVAL-B335-INVENTORYLANGUAGESCOPEWORD1=P2-observe`，等待异构语言复现，不为本 case 建硬门。

### `trace_query_wakeup_background_demotion`

- S37bi 生产生效：两条 logger background row 在关键指标表分别保留 19.500ms/7.000ms 的 Window projection，Chain total 与 Attribution 均为 `—`；按需图例明确根因归因只承载已证链上影响。
- 模型主结论继续以 `threadpool-400` 的链上 11ms 为首，保留完整 wakeup chain 与 runnable 3×1ms 次级项；显式窗口、自动补齐、双轴信息与系统投影都未受影响。
- 新 gap 不在系统选举，而在给模型的背景证据口径：模型把 logger 与 threadpool 的同时 D-state/IO 和 score=35.5 扩写为“共享 IO 资源竞争”及 threadpool 变慢的环境原因。当前没有共同 dev/inode/device/lock/core、资源身份或 typed competition edge；同时发生只证明独立背景。
- 同一段还自行把 19.5ms + 11ms 写成 30.5ms，违反既有 NO CROSS-ROW DURATION SUMS。最优修复应收紧现有 Trace query / background aggregate 两条软教学：无 exact shared-resource identity 或 typed dependency/competition edge 时，只能称独立背景与后续排查方向；不得跨线程相加。禁止答案词面硬扫或系统代写。

## Ruling

- runner：2/2 PASS；人工：1 PASS / 1 PARTIAL。
- `EVAL-B331=S37bi-production-positive/closed`：off-chain 数值仍完整，但根因排序列权限已撤回。
- `EVAL-B332=S37bh-production-positive/closed`：跨文件同名符号的模型正确 citation 与 exact row identity 均保真。
- `EVAL-B334-TRACEOFFCHAINRESOURCEINFERENCE1=P1-confirmed`：背景共现/同类/压力分不证明共同资源、竞争或对链上时长的因果贡献；不得自行跨线程求和。
- `EVAL-B335-INVENTORYLANGUAGESCOPEWORD1=P2-observe`：一次 summary 语言范围泛化，等待异构复现，不硬化。
- 本批零 malformed JSON、零 repair、零 finalizer reject、零“成文校验未通过”；系统没有替换或删除模型答案。
