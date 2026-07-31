# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T07:24:28Z
- sweep_start_ts: 20260731-002428
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260731-002428 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 133s | 35 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | analyzer 连续发出 `kind=process` 但无 `pid` 的 runtime target；emit_analysis 仅 WARN 后丢掉目标。随后 full-artifact 最小补采和 R15 authority 均无有效 typed target，系统按模型 20ms 窗补根因族，正文只写 2 次/0.285ms；真实 artifact 为 3 次/0.635ms。R15/R16 本轮未获得有效上游载体，不能判为回放覆盖。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260731-002428 | write_apply,answer_regex | none | 323s | 18 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 单一 ChangePlan、一次 apply、一次真实 verify；实现为 `_prefault !== undefined` 且保留 `default ??=`，false/0/空串与 existing-default 负例齐全。无 cumulative-review 修改批。效率残余：write analysis 5 轮 schema 修复、planner 8 轮 probe/insert-anchor 修复，且一次不可用 grep；最终控制器又尝试对已 complete batch 发 verify，确定性 transition 才改为 finish。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized Findings

1. `EVAL-B1-R17 / P0`：关键 typed carrier 采用 warn-and-drop。`runtime_targets` 中身份结构不合法的项被接受并静默移除；所有目标选择、目标优先、full-artifact 补采与答案 authority 因而一起失权。最优修复是对已提供的 malformed target fail-loud，使 analyzer 当轮修正；非法 `source` 仍可安全清空并警告，因为身份本身完整。
2. `EVAL-B1-R18 / P0`：窄状态事实识别把 `intent=explain + question_kind=mechanism` 排除在外，即使它是单一 runtime target、非 call、非 diagnostic 的状态/时间/内核原因事实。最优修复是扩展同一 typed focused-runtime-fact 谓词，让 family、supplement 和 materializer 共享；不能读取 D-state 题面或 case ID 做硬门。
3. `EVAL-B1-W5 / P2`：小修计划前的 schema/probe 修复摩擦过大。产品正确性未受损，但 23 行 patch 用时 323s；应从失败类型统计中抽象 write-analysis schema 示例和 verification-probe changed-symbol/path contract，避免继续给某个 TS case 增加特例。
