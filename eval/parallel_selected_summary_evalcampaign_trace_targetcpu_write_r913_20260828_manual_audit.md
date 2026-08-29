# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T00:08:09Z
- sweep_start_ts: 20260828-170809
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260828-170809 | write_apply,write_patch_oracle,answer_contains | none | 81s | 26 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只把 `main.go` 的 `retrun` 改为 `return`，应用树无额外改动；ChangePlan、fingerprint、apply ref、changed-path coverage 与 clean worktree audit 闭合，真实 `go test -json ./...` 1/1 通过。没有跳过风险、隔离 worktree 或验证门。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-170809 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 192s | 50 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=1 | partial | B1422 获生产正证：1697 条全零 `target_cpu` 只撤销同核/跨核强结论，36 次唤醒、四跳依赖、waker CPU、目标真实运行 CPU、链上排序与数值均保留；答案明确投递 CPU 未确认，未再与后置事实对照冲突。显式窗、Trace 因果投影、自动补采、链上-only 根因、实际占时/规则可消双轴、业务线索与背景隔离完整。新 B1424：final decision handoff 仍直接教授 `elected_wakeup_path=`，模型照抄进正文；已在 typed 上下文生产处改为自然语言路径标签，不扫描/拒绝/改写终稿。模型还把调用点词面扩写成文件系统缓存/页面锁对象，并把“直接阻塞未证”写成“并非 IO/锁竞争”，属于已登记 B1269/B1271 的软边界遵循重复，不新增硬 prose 门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
