# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T03:23:30Z
- sweep_start_ts: 20260819-202329
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260819-202330 | answer_regex,answer_contains | none | 207s | 36 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 终稿保留 12 条 production `implements` 边与对应文件表，关系方向和引用正确；一次表形修补未删边。探索 20 轮、13 次读取仍偏重，系统补充还暴露 `typed owner/evidence anchor` 等内部术语，记 P2。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-202330 | log_regex,write_apply,answer_regex,answer_contains | none | 895s | 37 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只改 `fastlex/tokenizer.py`，连续换行段预折叠为一个 rank token，普通 BPE 路径保留，测试文件未改；pytest 不可用后精确 unittest fallback 两项均过。首轮 planner 连续 14 次结构化计划失败并出现虚构 Python `}`/重叠编辑，触发 typed stutter 后外层重派 3 轮即解，记通用收敛 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B1210-REPORTLOCALPROOFRECONCILE1=production-positive-r754`：probe 没有单独携带
   `regression-test-preserved`，但同一合同引用已被精确项目测试 receipt 覆盖，单报告 proof 正确签绿。
2. `B1211-MODELPROSEWORKFLOWAUTHORITY1=production-positive-r754`：fallback outcomes 的来源为
   `expected_outcome_fallback;quality_repaired:planning_only_ungrounded`，没有进入 required 权威。
3. `B1213-EXPLICITUNGROUNDEDSATISFIESAUTHORITY1=confirmed/P0`：本轮 analyzer 直接输出四条
   `operator=satisfies`、`source=write_analyzer`、无 `evidence_ref` 的合同；它们格式合法，旧入口只在
   `writeAnalysisIRQualityRejection` 非空时调用修复，因而仍能以 `required=true` 进入 workflow。
   根修必须对每份有效 IR 无条件执行 item-local 权威校准，不比较请求/模型/答案 prose。
4. `B1214-REPEATEDSTRUCTUREDEMITROLLOVER1=confirmed/P1`：首轮 planner 连续产生结构化非法编辑，
   当前只在较晚的 stutter/iteration cap 后外层重派；第二次携带 compact typed repair pack 很快成功。
   最优方案是按连续失败的 `emit_change_plan` typed result 计数并提前滚转，成功 emit 清零；禁止按
   4ms、4m、总年龄或自然语言关键词终止活跃字节流。
5. `B1215-SYSTEMSUPPLEMENTBUSINESSLANGUAGE1=confirmed/P2`：类型关系答案的系统补充仍把
   `typed owner/evidence anchor` 等实现术语直接暴露给用户。后续应在补充自身的 typed renderer
   做业务化显示，不扫描或改写模型答案，不删除可追溯证据。
6. 本轮不触碰 Trace：显式时间窗因果投影、自动补齐、链上根因权威、背景支撑分层，以及实际占用与
   规则计价可消除量双轴保持不变。
