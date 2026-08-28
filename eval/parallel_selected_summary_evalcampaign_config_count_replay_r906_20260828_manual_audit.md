# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T21:40:15Z
- sweep_start_ts: 20260828-144014
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-144015 | answer_regex,answer_contains | none | 208s | 28 | read=15,repo_map=0,list=0,trace=0,source_lens=0 | midloop=9,inv=6/0,fin_reject=0,unavail=0,prune=0 | partial | B1407 production-positive：默认 scalar=50 未再借位成优先级集合基数。主体正确给出 YAML Decode、默认 50、merge 与 Changed 覆盖顺序；但 Decode/merge/Changed 主张只引用字段定义和 flag 注册，B1409 仍开。系统又追加同一 3 项优先级清单，RuntimeSettings/flag 两项备注分别重复 6 次；确认 B1412 exact-note merge 无去重与 B1413 已充分主体仍重复 supplement。15 次 read、26 个 explorer iteration 也偏重。 |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260828-144015 | answer_regex,answer_contains | none | 388s | 39 | read=5,repo_map=7,list=0,trace=0,source_lens=7 | midloop=12,inv=4/0,fin_reject=5,unavail=0,prune=0 | pass（事实）/partial（过程） | B1410 production-positive：最终恢复 3 type、5 function（含 SetExternalArtifactFloor）、30 Kind constants，共 38 项且逐项可复核。r905 的 visible_count=28 本轮未复现；最终 generic 弱证据 caveat 来自早期 truncated source-inventory advisory 未被后续完整 30/30 typed rows 清除，B1411 重裁为 stale advisory。5 次 reject 中首稿 table shape/首列身份是模型结构错误；但 source_inventory_family 的“删字段”修向没有已发布 field-edit、以及系统要求只补 facet metadata 而 replace_blocks 又要求整表重发，是 B1414 patch 合同不可执行/教学不完整。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

1. `B1410-PRINCIPALMEMBERSETSUPERSETLABELDRIFT1` 已获得生产正证并可 core-close。accepted 5 项 complete principal set 不再被后来的 4 项窄标签集合覆盖；系统没有生成、补猜或重写任何成员。
2. `B1411` 需从“结构化 roster 计数 28/30”改判为两层问题：r905 的 summary 文本计数解析曾误读 28；r906 已无该计数 mismatch，但早期 source-inventory `followup_debt/truncated=true` 仍在最终 30/30 完整证据后物化 generic enumeration caveat。施工应读取 typed 后代完整性/证据覆盖来消除陈腐 advisory，不能扫描终稿字符串。
3. 新 P1 `B1412-AGGREGATEMEMBERNOTEEXACTDEDUP1`：同一 accepted member 的等义/重复备注在多轮 aggregate merge 中持续由 `MergeEvidenceSummaries` 拼接，最终用户面出现 6 次重复。最小根修先对规范化后完全相同的备注做 byte-exact 去重；相近但不相同的模型说明仍保留，系统不裁语义优劣。
4. 新 P2 `B1413-AGGREGATESUPPLEMENTREDUNDANTWITHMODELROSTER1`：模型主体已经给出完整优先级表，系统仍附加结构化清单。后续应基于 block 的 typed member-set/facet coverage 判定“已承载”，只抑制重复 supplement，不读取可见措辞，不删除模型块。
5. 新 P1 `B1414-PATCHMETADATAEDITUNPUBLISHED1`：系统的 reject/hint 要求删除 `source_inventory_family` 或只增 `facet_ids`，但当轮发布的 patch surface 不允许对应 field edit，`replace_blocks` 又要求完整重发 38 行表。应让每条系统修向都具有同轮可执行的精确 patch 分支，或者在教学中明确完整块事务形；禁止发布不可执行修向。
6. 本批没有触及 Trace query、显式时间窗、因果投影、自动补齐、链上根因选举、业务线索或双账户模型；没有 4ms/4m/首字节/活跃流年龄降级，也没有系统代写答案结论。
