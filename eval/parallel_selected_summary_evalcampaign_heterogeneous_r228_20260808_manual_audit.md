# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T22:46:01Z
- sweep_start_ts: 20260808-154559
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260808-154601 | write_apply,write_patch_oracle,answer_contains | none | 88s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 隔离 worktree 内仅 `main.go` 一行 `retrun -> return`；ChangePlan kind=patch、git_apply、medium auto-safe、patch review、`go test -json ./...` 与 finish/all_verified 均有 typed 工件，原 fixture 未被修改。无 replan、无 JSON 修复、无验证域清空。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-154601 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 200s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | typed 五态、唤醒路径、链上根因榜、邻近/背景分层、实际占用/规则可消双轴均在；但模型把 `pre_wakeup_dependency + lower_priority_dependency_only` 擅自写成“持有主线程需要的锁、唤醒后等待释放/调度延迟”，并用窗外 onVsync 预览解释本窗帧完成。两者均与最终 typed authority（holder/waiter 未证、post-wakeup 未证、frame absent、窗外仅导航）直接冲突。runner PASS 为假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- runner `2/2 PASS`，人工 `1/2`；写模式通过，Trace 失败。
- Trace 的因果人口没有错位：榜首、#2、#3 都是 typed on-chain 席，邻近与背景没有被 deterministic 投影加冕。失败是模型在链上候选之上虚构了更强机理，以及越界消费窗外 frame marker。
- 新 GAP `EVAL-B384-TRACECLAIMENVELOPE1=P0/HIGH`：最终上下文同时暴露 `fix_direction=lock_priority` 与“holder/waiter 未证”，词面诱导和权限边界相互拉扯；需要按席位输出单一、精确、短小的 claim envelope，候选席只允许解释为下游唤醒前的低优先级依赖供给候选。
- 新 GAP `EVAL-B385-TRACEFRAMEPREVIEWCLAIM1=P1/HIGH`：已有 selected-window authority 正确把窗外 marker 降为 navigation-only，但最终提示过长，模型仍把窗外 onVsync 升成窗内帧边界/完成解释。应在同一个最终 checklist 重放 `frame absent/unavailable -> 窗外 marker 仅导航`。
- 最优方案为 prompt-only typed 收敛：不扫描用户输入、thinking、summary 或最终答案，不做关键词硬门，不自动增删/替写模型正文，不更改根因排序、可消量、自动补采或 Trace 因果投影。最终 checklist 同时钉住：主因人口仅 typed on-chain；adjacent/background 仅额外排查；两轴分开；candidate 的 holder/waiter 与 post-wakeup 语义未证；窗外 marker 不铸 frame authority。
