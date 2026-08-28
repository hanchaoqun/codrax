# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T22:49:42Z
- sweep_start_ts: 20260828-154941
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-154942 | answer_regex,answer_contains | none | 106s | 28 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 默认值 50、YAML 字段、CLI `Changed` 守卫和总体优先级均正确，B1412 重复 atom 未回归，B1413 同义系统补充本轮也未复现。但正文把 YAML 合并赋值定位为 `internal/config/runtime.go:1702`；该行实际是旧键 rename map，真实赋值在 `cmd/root.go:2590-2591`。错误不是纯 finalizer 波动：Turn A handoff 把 repo-map 导航候选 `ev-55827fd22546dd4b` 以 `grounding=unspecified assignment_fact` 混入 accepted evidence，模型从该 typed 表面误取。新立 B1417。 |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260828-154942 | answer_regex,answer_contains | none | 428s | 41 | read=8,repo_map=8,list=0,trace=0,source_lens=8 | midloop=12,inv=5/0,fin_reject=4,unavail=0,prune=0 | pass | 最终答案准确给出 3 个类型、5 个函数、30 个 Kind 常量和唯一 const block，逐成员表完整且无 invented rows。finalizer 前的精确审查两次均为 `members=30 principal_items=30 missing=0 unexpected=0`，证明 B1415 exact row-set 与 B1416 accepted-closure 单调恢复都获得生产正证。过程仍有 4 次成文结构拒绝（成员载体、principal 元数据、row bucket、空表 patch），答案未受损但模型心智/时延偏高，作为 B1418 过程观察项留档，不用系统代选布局。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `B1415/B1416` 生产闭环：错误 retry 没有擦除已验收的 5 函数/30 常量，也没有把错误成员带回；exact `source_inventory_row_id` 绑定在 30 行常量席上得到 `30/30` 正证。
- 新确认 `B1417-UNSPECIFIEDNAVASEVIDENCE1/P1`：最终成文的 typed handoff 把 `grounding=unspecified` 导航候选与 grounded/recovered 证据并列在 `accepted_evidence_handoff`。相似前缀旧键映射因此污染配置主路径定位。最优方案只在 finalizer prompt 投影剔除 unspecified/ungrounded accepted refs；完整 Turn A、证据账本和硬门输入不删，不扫描请求或答案 prose。
- `B1418-FINALIZERENUMSCHEMACHURN1/P2-observe`：事实已齐但仍经历 4 次结构 patch 拒绝。暂记为 JSON 教学/合同分阶段披露的效率观察项；先跨异构回放确认泛化，不为本例硬编码表格或系统代写模型布局。
