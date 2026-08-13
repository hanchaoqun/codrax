# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T00:13:09Z
- sweep_start_ts: 20260812-171308
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-171309 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 47 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、链上排名、Trace 因果投影、实际占用/业务 span 与规则可消双轴完整，背景未晋升主因；B684 已阻止把 12/11 两口径差异说成 precision，但模型仍从 caller 名形扩写为 DMA Fence/GPU release 修向，typed 证据只授权内核调用点，故继续收窄软教学。 |
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-171309 | write_apply,answer_regex | none | 186s | 23 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | B683 生产正证：controller-owned proof batch 的 purpose/goal/paths/criteria 未再被模型覆盖，planner 也只发 changes=[]+4 probes。四个 JS probe 却被 runtime 以 `verification_probe_language_target_mismatch` 拒绝，而 plan validator 明确允许 JS→TS；随后 make check 的 source_static 结果把 proof 批签成 verified。另有 runner 只看最后 probe-only plan，误报 durable_apply_ref_missing。均为系统合同 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B683 已获生产正证：控制器生成的补证批 typed 元数据在整个 plan→verify 链保持不变；
   planner 没有重复写生产或测试文件。
2. B685（P0，已修）：inline probe 的 direct-source provider 关系此前有多份实现，计划校验允许
   JavaScript→TypeScript，运行时 mismatch 与 changed-path coverage 却只认精确 family，形成
   “同一计划先接收、后确定性拒绝”的合同冲突。现由
   `types.VerificationProbeDirectSourceFamilies` 单源供计划校验、执行检查、覆盖投影和
   follow-up runtime 判断。JS→TS 只表示允许通过 package entrypoint/loader/compiled module
   尝试执行，不表示 provider 已存在；执行失败仍 typed unavailable 且不能被静态套件掩盖。
   ArkTS、Cangjie 等不借跨语言 wrapper 硬签。
3. B686（P0，已修）：source-static follow-up 为了先让 planner 编写 probe 而暂不标 verify-only，
   但 probe-only plan 落地后没有切回严格权威；即使四个探针均 unavailable，独立 `make check`
   仍可把批次标 verified。现在 probe-only plan 生成后立即 typed promote 为 verify-only，且
   reconcile 同时检查 obligation 与 capability 的 unavailable/failed 计数。普通实现批仍可让
   独立项目套件覆盖非权威 probe-authoring 失败，不扩大硬门。
4. B687（eval，已修）：最终 probe-only plan 天生没有自己的 apply ref；runner 只按最后 plan ID
   查 durable commit，漏掉前序生产批的唯一 `refs/codrax/applied/*`。现在仅对精确
   `changes=[] + verification_probes>0` 形物化前序 durable checkpoint；普通变更无 ref 仍 fail-loud。
5. B684 再收窄但保持软教学：caller 名称形态只能作为 code-location/search clue，不能自行铸造
   subsystem、mechanism、wait object 或直接修向。未扫描用户/模型/答案原文，未由系统改写结论。
6. 活跃流审计新增 Reader-boundary liveness：过去完整 SSE line 才刷新时钟，慢分片且未换行的
   活跃连接仍可能被误判 silence。现在任意成功 Read 的字节刷新；4ms 间隔的 partial-frame
   回归越过 stall 阈值后仍取回原模型答案。固定年龄绝不授权降级，只有 caller cancel/deadline、
   no-first-byte、真实 byte-stall、transport/decode failure 才进入重试/披露降级。
