# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T20:29:46Z
- sweep_start_ts: 20260814-132944
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260814-132946 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 31 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail-system | 四态、显式窗和投影均在，但系统把“链上时间重叠”晋升成 semantic mechanism 根因；typed handoff 同时明确没有 target-blocking relation，且 VerifyClass span 在 wakeup 后 0.4ms 才结束，正文“完成后触发唤醒”不可能成立。 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260814-132946 | write_apply,write_patch_oracle | none | 790s | 24 | read=15,repo_map=2,list=1,trace=0,source_lens=0 | midloop=3,inv=0/0,fin_reject=0,unavail=3,prune=0 | fail-system | 最终 patch 行为正确、fixture unittest 独立复核 4/4 PASS；但三轮 plan/replan 曾把已落地修复重复插入后再删除，且最终报告在 cumulative proof=weak、reason=verification_probe_expected_stdout_missing 时仍签 completion=verified/all_batches_verified。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### Write：正确代码被终态假绿与陈腐 replan 污染

1. 最终 `relativedelta.py` 用单一 `_normalize` 同时接受整数与整数值浮点、拒绝非整数浮点；系统内 8 条测试通过，
   applied tree 上独立运行 fixture 原有 unittest 为 4/4 PASS。机器 patch oracle 的 PASS 对代码结果本身成立。
2. 首轮正确修改已落地后，一次模型 probe 的 `expected_stdout` 合同写错。普通 controller finish 两次被 typed
   truth ledger 以 `truth_ledger_failed_requires_repair` 正确拦截；但预算耗尽后的 completion-verify 直接按最后一份
   report pass 把 active batch 和 run 聚合成 `verified/all_batches_verified`。最终 JSON 同页又明确
   `proof.status=weak`、`proof_ledger.state=low_confidence`、
   `verification_probe_expected_stdout_missing`，构成确定性 typed authority 自相矛盾。
3. 登记 `B827-TERMINALCUMULATIVEPROOFAUTH1/P0`：所有终态捷径必须消费与 WriteFinalReport 相同的累计 proof
   artifacts/ledger。failed、unavailable、low-confidence、unknown 均不得铸造 all-verified；终态预算不足以继续补证时
   只能持久化 `unverified` 和具体 typed obligation reason，不能根据测试摘要或正文猜测。
4. 本轮还登记 `B828-REPLANCURRENTSTATEHANDOFF1/P1`：replan 混用了原始源码与已应用工作树，先重复插入
   `_normalize`，下一计划再删除重复。validator 对 no-op/stale/create-existing 的拒绝是正确的，缺口在 planner 没有拿到
   清晰的 current-generation/current-worktree receipt。最优修复应绑定 PlanID、apply generation、当前 path fingerprint 与
   exact already-applied edit receipts，不按 Python 或 `_normalize` 特判。
5. r506 后续模型计划没有声明 `expects_baseline_failure`，所以 B826 新的 main-snapshot differential 没在本轮生产触发；
   只能保留 implementation/full-suite 结论，不能借本轮机器 PASS 宣称 production closed。

### Trace：链上成员资格不等于 semantic completion 因果

1. 答案保留 sleep=5.0ms、runnable=0.8ms、running=1.2ms，显式窗和 “Trace 因果投影” 均存在；邻近背景没有被
   直接升成主因，VerifyClass 也确实位于 worker 的链上运行区间。
2. 但模型前 typed boundary 明确写明：selected leader 只是 wakeup 前重叠的 on-chain work，**没有** typed
   target-blocking relation 证明目标等待该工作、等待其完成或被其直接阻塞；frame evidence absent、frame causality
   unproven。系统生成投影仍把 `worker-200 class_verification effective=4.600ms` 加冕为主根因，给模型冲突上下文。
3. 原始时间关系进一步否证“完成后唤醒”：worker 在 5.005000s 唤醒 app，VerifyClass span 到 5.005400s 才结束；
   span completion 比 wakeup 晚 0.4ms，不可能触发该 wakeup。
4. 登记 `B829-TRACESEMANTICBINDINGAUTH1/P0`：链上 occupancy/overlap、业务语义标签和 target wait dependency 必须分席。
   semantic span 只有 membership+intersection 时可进入链上业务排查/实际占用候选，但不能铸造 “等待该 span 完成”
   机制或规则可消除主因；只有 exact typed dependency/binding 能授权 mechanism seat。不得删掉 semantic/JIT/类校验/
   shader/texture/GC 等业务线索，也不得把背景晋升为主因，最终结论仍由模型形成。

## Active-stream / authorship audit

- 两案均未观察到按 4ms、4s、4m 或连接总年龄降级。持续到达字节时不得用“尚未形成完整 answer”作为终止权；仅
  caller cancel/deadline、无首字节、byte-stall、transport/decode failure 可终止或进入明确披露恢复。
- 本批不允许系统从 patch/答案关键词改写模型代码或结论。B827 只校准 typed 完成权威；B828 只改善当前态 handoff；
  B829 只拆分 typed causal caliber。显式时间窗、因果投影、自动补齐、链上-only 主因、实际占用/业务线索与规则可消除
  双轴继续保留。
