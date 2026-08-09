# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T01:31:28Z
- sweep_start_ts: 20260808-183127
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-183128 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 136s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S37dd 的候选机理包络已生效：开篇和链条把 lower-priority dependency 保持为供给候选，明确 `causal_conclusion=unproven`，没有加冕链外背景。但同一 ThreadPool 的上下文把 10.433ms 无 caller 的未证 remainder、7.386ms fscache caller、其他 caller 与 thread census 拼在一行，模型又把 caller/文件系统机理借给未证席，并把 page_lock callsite 升级为资源争用。确认 B399：不同 cause/caller/census 席必须逐行隔离，census 不绑定任何 cause seat。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-183128 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 252s | 30 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | S37dd 的阶段/线程角色分权和四尺生效：最终使用 7.000→7.040=40ms，不再把 37ms span sum 当包络，也未把 span-name 角色当独立 thread-role authority。新残余 B398：模型把 1ms uncovered gap 写成“无明显跨线程阻塞/效率正常”，而 gap 只证明区间补集。另有一次可避免的成文拒绝：模型用 self-arrow 表达 span，却只给跨线程箭头配锚；patch 后全部 5 箭头配锚通过。应软教 `Note over` 表 span、只给 typed temporal edge 画箭头并逐箭头配锚，不扫描 label 硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `2/2 PASS`; human correctness: `0/2 PASS`（均为 partial，精确值显著改善但仍越过证据权限）。
- `EVAL-B395/EVAL-B396` 生产闭环；`EVAL-B397` 的 priority/D-state 包络生效，但同线程旧 wait-summary 仍造成跨席 caller/机理污染，故保持 partial。
- 新增 `EVAL-B398-FRAMEGAPSEMANTICCALIBER1=P1/HIGH`：`uncovered_gap` 只能解释为 first-start→last-end 内未被 item interval 覆盖的区间，不能单独证明 scheduler latency、blocking、efficiency 或“无阻塞”。
- 新增 `EVAL-B399-WAITSEATCONTEXTCOLLISION1=P1/HIGH`：cause-unproven、caller-proven、window inventory、census 必须是独立 typed seats；同 subject 不是跨席借 caller、state、mechanism 或算术重组的授权。
- Mermaid 本轮实际产出并在一次修复后通过，证明 temporal relation owner 有效；首稿拒绝来自教学心智不足，不是 label/prose 可硬扫的依据。后续以 `Note over` 表 span、箭头逐 occurrence 配 typed anchor 的通用教学降低重试。
- 系统没有删除、替换或重写模型主稿；确定性 Trace 投影保持 typed on-chain 主因人口，adjacent/background 仅支撑与额外排查方向。
