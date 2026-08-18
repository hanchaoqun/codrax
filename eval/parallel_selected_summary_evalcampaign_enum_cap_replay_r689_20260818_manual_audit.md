# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T14:19:29Z
- sweep_start_ts: 20260818-071928
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-071929 | answer_regex,answer_contains | none | 463s | 39 | read=12,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=5/0,fin_reject=0,unavail=0,prune=0 | fail | B1076 的 owner/mechanism 补采已明显生效：终稿分成配置与 REPL Mermaid 两节，准确给出六级查找、`CODRAX_SETTINGS`、first-hit/default、`initApp -> LoadRuntimeSettings` 与 `renderRichResponse -> RenderMermaidBlocks`。仍有明确事实错误：把终端 Mermaid 成功形写成 SVG，源码实际输出确定性 ASCII；失败形也笼统写成“原始块或友好文本”，遗漏 unsupported/text fallback、fallback-rune 和 library-rejected 的 typed 分支。覆盖优先级只写泛化 CLI flags，未披露仓库限定的精确 override 子集。Runner PASS 不能覆盖这些人工正确性缺口；证据上下文已经包含 ASCII 与分支，主要是模型成文波动，不再是核心 owner 缺证。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-071929 | answer_regex,answer_contains | none | 482s | 42 | read=15,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=3/1,fin_reject=2,unavail=0,prune=1 | fail | B1081 获生产闭环：不再出现 typed enumeration supplement 与 `MaxCount=1` 自冲突，19 次拒绝/1200s 超时降为 2 次拒绝/482s PASS。首拒是模型自画未证/错向边，第二拒是 patch 替换 table 时漏行，validator 均正确。终稿的四阶段表与时序图基本回答问题，但系统在首稿和 patch 两轮分别追加 `principal-support-surface-terms` 与 `-2` 两张主展示表，标题为“系统按已验证证据补充可见标签”，暴露 `Terminal`、`MutableState` 等内部 support labels，既非用户要求又不幂等。该确定性红线记 B1082；活跃流持续推进，无固定 4ms 降级或静默 fallback。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings

1. `B1081-AUTOSUPPLEMENTTABLECARDINALITY1` 获生产闭环。相同双案下 pipeline 从 r688 的
   `TIMEOUT/1200s/19 finalizer rejects` 变为 `PASS/482s/2 rejects`，日志不再出现自动补表造成的
   table `MaxCount=1` 冲突。cap-aware yield 与旧 system carrier 清理均生效，关系/引用硬门未放松。
2. `B1076-MULTITOPICMECHANISMCLOSURE1` 第一批和第二批核心补采均获生产正证：两个问题各自拿到正确
   owner 与 load-bearing mechanism，六级 settings lookup 和 REPL Mermaid shipping 入口均进入终稿。
   剩余 SVG/typed outcome 错述是在精确证据已到达 Finalizer 后发生的模型成文波动；CLI override 子集
   仍可作为低一级 unit dimension closure 继续软补采，不得用答案词面硬门或系统改写纠正。
3. 新确认 `B1082-SURFACETERMSYSTEMAUTHORSHIP1=P0`。`normalizePrincipalSupportSurfaceTermSupplement`
   读取模型可见面后，把 evidence `surface_terms` 另铸为 `SurfacePrincipal` 系统表；patch 轮次再次执行时
   `nextPrincipalSupportSurfaceTermBlockID` 生成 `-2`，因此同一事实可重复进入最终答案。`SystemGeneratedKind`
   只标识所有权，不授予系统成文权；这违反既裁 `EVAL-B54-SURFACETERMAUTHOR1`：精确 surface term 应通过
   `preCheckModelSurfaceTerms` typed soft advisory 交给模型，不得拼入正文。
4. 最优根修是断开 full/patch/persist 共用 shipping normalization 对该补表器的调用，并把它加入 AST
   authorship tripwire；保留 Finalizer prompt 与 `preCheckModelSurfaceTerms` 软提示。禁止扫描用户请求、
   reasoning 或终稿关键词作 hard gate，也不删除/重写模型 blocks。需要回归证明重复 pre-emit 不生成
   `principal-support-surface-terms(-N)`，而 typed advisory 仍携带缺失标签。
5. 该施工只收回 source-code answer 的系统可见成文权。Trace 独立 owner 下的 typed runtime projection、
   显式时间窗自动补采与因果投影保持不变；链上-only 主因、背景 support-only、实际占用与规则可消除量
   双轴均不进入本函数。
