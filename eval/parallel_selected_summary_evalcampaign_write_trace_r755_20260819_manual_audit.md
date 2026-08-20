# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T03:58:50Z
- sweep_start_ts: 20260819-205849
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260819-205850 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 10ms 窗、worker-200→app-100 已证链、链上 #1、runnable 8.300ms 可消与 9.000ms 累计、目标 sleep 10.000ms 实占、跨核角色、背景不晋升和 Trace 因果投影均在。正文却由 D/IO=0 推出“正常的睡眠等待”，同页 caveat 又明确 sleep 机制未证；并复制 `status=complete` 内部枚举。上游 perf-triage 自由叙事恰好错误主张“自身睡眠/无反转”，仍进入最终上下文，与 deterministic trace_query 争权。 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-205850 | log_regex,write_apply,answer_regex,answer_contains | none | 243s | 26 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只改 `fastlex/tokenizer.py`，保留 five-newline 测试原文；probe、精确 unittest fallback 两项与 changed-path 验证均通过。Planner 首次 insert 形被准确拒绝，第二次 full replacement 成功，仅 1 次失败，故 B1214 三次滚转分支未触发；243s 是改善信号但不是生产闭环。两条 satisfies 合同均带真实文件 evidence_ref，保持 required 符合 B1213 边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B1214-REPEATEDSTRUCTUREDEMITROLLOVER1=unit-covered/production-arm-not-exercised-r755`：本轮只有
   1 次失败 structured emit，第二次成功；不得用总时长下降冒充第三次阈值臂已经触发。
2. `B1216-PRETRIAGENARRATIVEAUTHORITY2=confirmed/P1-high`：perf-triage 输出“主要为自身睡眠等待、
   不存在优先级反转”，而后续 deterministic trace_query 给出相反的已证链上 runnable/priority candidate。
   最终上下文仍同时携带两者，模型正文留下“正常睡眠”与 mechanism-unproven 自冲突。根修应在同一
   artifact 已有 deterministic authority 时，把 pre-triage 自由 summary/因果判断降为导航，不删除其原始
   测量字段，不扫描或改写模型答案。
3. `B1215-SYSTEMSUPPLEMENTBUSINESSLANGUAGE1=confirmed/P2`：模型上下文仍直接提供
   `status=complete`、`state_partition_coverage=complete` 等内部枚举，终稿发生中英混用。应从 typed
   context/补充 renderer 提供读者词形与边界，内部枚举保留在诊断载体，不做最终 prose 关键词门。
4. Trace 主合同未退化：根因只来自 on-chain worker-200，背景压力不加冕；实际占用与现规则可消两轴、
   优先级反转候选的证据上限、调度供给、D/IO 零值边界和系统自动补齐均保留。
