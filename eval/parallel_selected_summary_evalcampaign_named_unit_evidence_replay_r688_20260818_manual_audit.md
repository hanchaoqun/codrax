# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T13:40:54Z
- sweep_start_ts: 20260818-064053
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260818-064054 | answer_regex,answer_contains | none | 246s | 32 | read=15,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=0,unavail=0,prune=0 | fail | B1076 第一批获生产正证：Unit 2 的首屏证据已从 browser preview 洪泛切回 `REPL.renderRichResponse -> RenderMermaidBlocks/RenderMarkdown`，终稿不再声称 HTTP 500 且无回退，改为开关关闭透传、库/语法失败保留或 text 围栏降级。仍不合格：两题没有清晰分节，Mermaid 结论未绑定其源码引用；配置部分继续错称仓库根/工作目录查找，遗漏六级 lookup、first-hit-wins 与 CLI 精确 override 子集，并把 strict decode 行为误列成覆盖优先级。Runner 正则失败与人工失败同向，但不是完整正确性判据。 |
| 2 | read_combo_pipeline_sequence_table | TIMEOUT | eval/results/read_combo_pipeline_sequence_table-20260818-064054 | answer_regex,answer_contains | none | 1200s | 76 | read=41,repo_map=2,list=0,trace=0,source_lens=0 | midloop=20,inv=9/0,fin_reject=19,unavail=1,prune=1 | fail | 未产出最终答案。确定性 P0：模型首稿只有一个 model table；`appendPrincipalEnumerationTypedSupplements` 在预校验前追加系统 table，随后同一 typed semantic view 又要求 table `MaxCount=1`。模型删除/合并后，pre-emit 每轮把补充表重新加回，计数长期为 2，后续草稿一度为 3，形成不可满足合同并耗尽 20 分钟。关系证据门同时拒绝若干未证/错向边是正确行为，不应为降重试而放松。活跃流持续有字节与工具进展，没有固定 4ms 降级或静默 fallback。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings

1. `B1076-MULTITOPICMECHANISMCLOSURE1` 第一批已获生产正证：typed 业务身份优先于预扫描路径的有界排序把正确 REPL 机制送回首屏，未依赖用户/终稿关键词，也未改模型答案。第二批仍开放：每个 unit 需要 owner/load-bearing mechanism 的软补采与闭合；配置 unit 本轮仍未读齐 `cmd/root.go` 的六级查找和 CLI override 接线。
2. 新立 `B1081-AUTOSUPPLEMENTTABLECARDINALITY1=P0/production-confirmed-r688`。系统的 typed enumeration supplement 与同一 compiled semantic view 的 `MaxCount=1` 自冲突；这是“系统自动写入后又要求模型删除”的答案所有权/合同红线，不是模型波动。
3. 最优根修冻结为 typed cap-aware yield：自动补充拟新增载体前，读取同一个 `AnswerSemanticView.RequiredBlocks`；若候选会让已达上限的 requirement 超限，则不追加、不改写/合并 model table。重试草稿若已携带旧轮系统补充，只删除造成 typed 上限溢出的系统自有载体，再让缺行事实由既有 typed completeness 校验和 Finalizer prompt 提示模型补到原表。无上限枚举与合法的同 set 系统补充原位幂等合并保持。
4. Pipeline 的关系门拒绝仍基于 typed edge/citation/participant contract，不能因该 P0 一并放松。修复后需要原样双案回放，区分表格合同闭环、图关系 authoring 与 B1076 unit mechanism closure。
5. Combo 的 runner 正则还要求两个题面出现明确标题/序号；本轮虽有两个 principal blocks，但渲染成连续内容。这是 runner boundary 与回答可读性同向的提示，不以正则反推语义硬门。
6. 本批无 Trace 输入；显式时间窗、Trace 因果投影、自动补齐、链上-only 主因、实际占用/业务语义与规则可消除量双轴均未改。活跃流没有 4ms 固定年龄降级。
