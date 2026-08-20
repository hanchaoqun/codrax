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
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260819-205850 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 10ms 窗、worker-200→app-100 已证链、链上 #1、runnable 8.300ms 可消与 9.000ms 累计、目标 sleep 10.000ms 实占、跨核角色、背景不晋升和 Trace 因果投影均在。正文却由 D/IO=0 推出“正常的睡眠等待”，同页 caveat 又明确 sleep 机制未证；并复制 `status=complete` 内部枚举。冷读 finalizer prompt 后确认 pre-triage 自由 observation 已被裁掉，且精确写明 zero D/IO 不分类 sleep 原因；前一项属模型波动，不新增 prose 硬门。 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-205850 | log_regex,write_apply,answer_regex,answer_contains | none | 243s | 26 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只改 `fastlex/tokenizer.py`，保留 five-newline 测试原文；probe、精确 unittest fallback 两项与 changed-path 验证均通过。Planner 首次 insert 形被准确拒绝，第二次 full replacement 成功，仅 1 次失败，故 B1214 三次滚转分支未触发；243s 是改善信号但不是生产闭环。两条 satisfies 合同均带真实文件 evidence_ref，保持 required 符合 B1213 边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. `B1214-REPEATEDSTRUCTUREDEMITROLLOVER1=unit-covered/production-arm-not-exercised-r755`：本轮只有
   1 次失败 structured emit，第二次成功；不得用总时长下降冒充第三次阈值臂已经触发。
2. `B1216-PRETRIAGENARRATIVEAUTHORITY2=refuted/non-gap`：perf-triage 的早期自由叙事确有错误，但
   finalizer 的 `Perf Triage — Validated Extraction` 只剩三条 deterministic-validator 行，navigation-only
   observations 与 meta summary 均未进入成文权威；Prior Stage Findings 也从确定性 root-cause board 构造。
   Prompt 还逐字给出 `zero_d_state_or_iowait_does_not_classify_sleep_reason=true`。因此“正常睡眠”是模型
   违背现有精确上下文的波动，保留回放观察，不加答案原文硬门、不由系统改写结论。
3. `B1215-SYSTEMSUPPLEMENTBUSINESSLANGUAGE1=confirmed/P2`：模型上下文仍直接提供
   `status=complete`、`state_partition_coverage=complete` 等内部枚举，终稿发生中英混用。应从 typed
   context/补充 renderer 提供读者词形与边界，内部枚举保留在诊断载体，不做最终 prose 关键词门。
4. Trace 主合同未退化：根因只来自 on-chain worker-200，背景压力不加冕；实际占用与现规则可消两轴、
   优先级反转候选的证据上限、调度供给、D/IO 零值边界和系统自动补齐均保留。
