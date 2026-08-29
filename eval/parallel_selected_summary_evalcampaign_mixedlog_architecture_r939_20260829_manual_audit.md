# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T11:27:30Z
- sweep_start_ts: 20260829-042728
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-042730 | log_attachment,answer_contains | log_triage | 161s | 28 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 两种语言的第一帧、各自栈内调用者以及“两个顶层错误没有已证跨栈因果”均正确，附件路径不再被重复质疑。但 analyzer 被共享上下文中的 `Intent hint: root_cause` 连续带偏：先 root-cause，再与 bounded-fact 合同冲突，合计 5 次 emit 才通过；最终答案又仅凭 `A0c0d/JsApp`/`CjApp` 标签臆造两处“JVM 线程”。这是附件能力被误呈现为请求意图、opaque 日志标签缺权威边界的系统 gap，不是单纯模型波动。 |
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260829-042730 | answer_regex,answer_contains | none | 209s | 31 | read=11,repo_map=3,list=0,trace=0,source_lens=1 | midloop=9,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | 正文正确列出 conditional pre-stages、四个主阶段、绑定与事件机制；首稿图中三条主阶段 precedence 有证，其余 dispatch/event/context 边无对应 typed relation，validator 正确拒绝。模型修补时没有只删除被点名的无证边，而是删掉整张可选图，连三条已证主链也一起丢失。当前定性为关系图修补心智/模型遵循残余；系统不得替模型选择或改写边，后续用异构 architecture/flow 图验证是否需把“允许边集合”进一步结构化前置。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
