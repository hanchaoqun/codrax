# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T08:00:25Z
- sweep_start_ts: 20260808-010024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_java_typo | PASS | eval/results/patch_java_typo-20260808-010026 | write_plan,write_patch_oracle | none | 86s | 21 | read=1,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划只修改 `Main.java:16` 的 `retrun -> return`，kind=patch，preflight 通过且没有落地源码。write_analyzer 一次成功；但前置 read analyzer 仍把明确 micro bugfix 分类为 `explain/architecture_explain`，B328 在 Java 上得到跨语言复现，虽未污染最终计划仍属 typed 上下文噪声。 |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-010026 | answer_regex,answer_contains | none | 579s | 38 | read=5,repo_map=19,list=0,trace=0,source_lens=19 | midloop=24,inv=15/0,fin_reject=0,unavail=0,prune=0 | partial | 最终答案与 Mermaid 正确：四 stage、职责、三条链边均保留，source repair 只把 `\\n` 转为 `<br/>`，finalizer reject/repair=0，S37bg 生产正证。但 analyzer 违背教学把 bounded conceptual stages 发成 source inventory；完成门遂把固定 4 成员扩成全仓 constant/type census，向无关 scopes 发出重复 follow-up，19 轮/579s 才降级放行。新立 B336。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Human correctness: **1 PASS / 1 PARTIAL**。两份最终用户产物均正确，但图表流程存在确定性完成合同扩域，不能按 runner 2/2 PASS 收账。
- `EVAL-B329-MERMAIDCHAINEDGEAST1` 与 `EVAL-B330-DIAGRAMBODYTEACHINGDRIFT1` 获得生产正证：一行 `Analyze --> Explore --> Extract --> Finalize` 的三跳 edge anchors 全部通过，首轮成文即接受。
- 新确认 `EVAL-B336-BOUNDEDCONCEPTUALINVENTORYAUTHORITY1=P1`：typed 形为 `IntentExplain + architecture_explain + declared_count + required diagram + requested summary`，却因漏发 `has_per_member_table` 没命中已有架构成员降级臂。source declarations 只是支撑证据，不应拥有全仓 source-inventory 完成权。
- B336 最优修复在共享 typed authority boundary：让上述有界 diagram-member 形与已有 per-member-table 形同样把 source inventory 降为 support-only；不得扫描“stage”、问题原文、模型 thinking 或答案，也不得按 Go/本仓文件设例外。
- `EVAL-B328-WRITEROUTEANALYZERSCOPE1` 在 Java 上确认：read analyzer 没再出现 field-value profile 拒绝，但仍把 write micro bugfix 标为 architecture explanation；最终 write analyzer/plan 正确，故 B328 保持 P1 open，后续应用 structured TurnPolicy 的软上下文收窄。
- 本批不含 Trace 输入、也未修改 Trace 路径；显式时间窗因果投影、系统补采、链上根因资格、窗内可消除量与链外背景隔离不受影响。
