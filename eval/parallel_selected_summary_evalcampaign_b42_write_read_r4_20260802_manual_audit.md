# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T15:06:03Z
- sweep_start_ts: 20260802-080602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260802-080603 | log_attachment,answer_regex | log_triage | 216s | 33 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 日志边界正确，但把 timeout classifier 的 false/complement 再次冒充 content validation 实现，并把 soft warning rule 未绑定到 model=demo 的 runtime instance；源码引用也有错位。三轮 analyzer 原本均发出 explain/architecture/mechanism，系统却因 diagnostic bool 静默重写成 root_cause，形成 65k explorer context、root_cause_trace 合同与不必要枚举，这是独立的 typed context-route GAP。现有机制软指导已足够精准，剩余结论错误记 model-variance watch，不追加答案 hard gate/替换。 |
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-080603 | write_apply,answer_regex | none | 307s | 20 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (proof boundary unverified) | 生产补丁和既有生成物行为测试正确；typed final report 诚实为 unverified，runner 本轮正确 fail。计划把 extensionless module import 放进 changed_symbol_refs，后续误铸 changed_symbol obligation，触发 source proof false negative；应在唯一匹配 changes[].path 时归一为 path identity，真实 symbol 与歧义路径保持 fail-closed。Node runner 缺失仍是合法未验证边界，不得为绿灯而放宽。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Context precision audit

- read analyzer 连续三次都声明 `intent=explain / scenario=architecture_explain / question_kind=mechanism`，同时误置 `is_diagnostic_question=true`。产品没有 fail-loud 要求模型修正矛盾，而是连续三次把 intent/scenario 改写为 root cause。
- 改写后 family 固定为 `root_cause_trace`，explorer 扩至 8 轮、7 次 `read_file`，上下文峰值约 65k/200k（33%）；对一个“两套当前源码机制 + 附件日志边界”问题，根因 family 的 required facets、diagram 与 enumeration 是不精确上下文。
- 新增的 independent-mechanism、producer-role 与 runtime-rule-instantiation 指令都准确到达模型；模型仍未打开 validation 的实际 check/requeue 路径，属于模型波动，不再用显示层重写或原文关键词 gate 纠正。
- 架构修复应删除 diagnostic route 的静默语义改写，保留 typed 自洽拒绝与重试。真正的 root-cause/performance intent、runtime causal scope，以及显式 Trace 时间窗均保持原权限。
