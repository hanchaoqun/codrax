# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T01:28:16Z
- sweep_start_ts: 20260809-182814
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260809-182816 | write_apply,write_patch_oracle,answer_contains | none | 68s | 21 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Full write path 正常：计划为 `kind=patch`，隔离 worktree 只把 `main.c` 的 `retrun buf;` 改成 `return buf;` 一行；自动选择 `make test`，1/1 通过，最终 `verified`，未清空累计验证域、未扩文件、未触发 JSON/成文修复。 |
| 1 | trace_query_core_topology_supply | PASS | eval/results/trace_query_core_topology_supply-20260809-182816 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 139s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | 状态查询正确且未误触发因果投影：CPU4=big、typed 频点2200000kHz、app-20 running 40ms/runnable 10ms、worker-30 running 10ms、low_freq_loss=0。R+ 与同 CPU 更高 prio 支持抢占候选。过程同参数 `window_stats` 被三节点重复调用；analyzer 先铸 `intent=trace + is_diagnostic_question=true` 被合同拒后自修。正文把单个频率状态写成“固定/接近最高”，随后 caveat 又称持续性为推断，按口径波动记观察项；不扫描正文硬拒。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- 严格 `PARALLEL=2`，runner `PASS/PASS`；两例 `finalizer_reject=0`，没有“成文校验未通过”重试。
- 写模式 production pass：单行补丁、自动测试、最终验证与应用树一致。
- Trace 状态查询不需要 Trace 因果投影；系统没有套入 root-cause/call-chain 合同，显式 10.000..10.060 窗与 typed 算力供给事实完整。
- `EVAL-B443-TRACEDAGIDENTICALQUERYREUSE1=P2`：三个独立调查节点重复相同 source/view/window/core_topology 查询；应评估 typed tool-result memo/共享观察，而不是靠关键词缩减图。
- `EVAL-B444-TRACEDIAGNOSTICINTENTCOMPAT1=P2`：询问“是否有 pressure/低频”是运行时诊断事实，但 `intent=trace` 与 `is_diagnostic_question=true` 当前合同互斥，触发一次 analyzer 自修；应从 typed intent/profile 关系修合同，不扫描原问题。
- 单次频率状态的“固定/最高”措辞先记模型/上下文观察项；已有 typed `low_freq_loss=0` 可回答低频信号，不因一例新增结论硬门。
