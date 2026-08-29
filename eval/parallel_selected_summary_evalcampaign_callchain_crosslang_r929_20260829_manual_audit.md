# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T07:07:27Z
- sweep_start_ts: 20260829-000726
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260829-000727 | primary_answer | none | 175s | 31 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 五条调用、容量 guard、内存 `rows.add` 与 `AuditLog.record -> System.out.println` 均准确；终稿明确写“内存列表”和“标准输出”，没有把主体操作宣称成数据库写入。runner 仍要求一句固定顺序的“标准输出不等于落库/持久化”显式否定，属于更严格的表达 oracle，不应据此新增终稿词面硬门。两次拒绝后模型移除可选图，列表关系仍完整。 |
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260829-000727 | answer_regex,answer_contains | none | 393s | 38 | read=15,repo_map=4,list=0,trace=0,source_lens=1 | midloop=7,inv=3/0,fin_reject=8,unavail=0,prune=0 | pass | `run -> ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce -> fetch`、重试 guard/delay 与 `@app/core` 的 tsconfig 继承/paths 均有源码证据，最终答案可用。但 8 次拒绝不是模型随机波动：混合图/非图修复租约回退到手工坐标并禁止删除无关摘要，模型反复猜 `match/body_occurrence`，最终只得删除可选图，确认 B1443。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Findings

1. 本轮从已推送 `8ccf2e4a7` 干净构建严格并发恰好 2 路。TypeScript runner/human PASS；Java runner FAIL、human PASS。两路均正常等待活跃流，没有固定 4ms、4m、首字节、累计活动年龄或上下文比例降级，也没有旧稿恢复、系统代写结论或关系。
2. Java 的终稿已把逐跳真实操作与概念词拆开：`VisitRepository.insert` 明确为加入内存列表，`AuditLog.record` 明确为打印标准输出；完整五跳与容量检查位置准确。runner 的正则还要求同一句里显式出现“不/未/only/not”再连接“落库/持久/数据库”，因此 runner FAIL 不等于事实错误。继续保留为表达遵循观察，不新增请求/模型/终稿关键词硬门。
3. B1442 的一般 aggregate authority 上限进入 Finalizer，但本轮 Analyzer 没有激活 source-operation-site 专用合同，因此不能把本轮 Java 改善记为 B1442 生产转正；B1442 仍待一个 typed `RequiresSourceOperationSiteMemberSetHandoff=true` 的自然生产用例验证。B1441 的 observed-operation 边界三次到达，获得继续正证。
4. 新 P1 `B1443-MIXEDREPAIRCAPABILITYINTERSECTION1` 是 TypeScript 8 次“成文校验未通过”的确定性根因。首稿同时包含：普通结构错误（两个 summary、非图 relation list 缺 endpoint identity）与图错误（一个 label pair、一个无 anchor 的 reply）。relation lease 的 target selector 只收实际 diagram block，却要求 lease 的全部 failure 都在 target set；非图 failure 使窄 schema 整体回退到兼容 schema，重新暴露 `block_id/match/body_occurrence` 手工坐标。
5. 同一租约的执行门又允许无关块 replace、却禁止任何无关块 remove。模型需要合并并删除多余 summary，却第一次被 `whole_remove_not_authorized` 拒绝；随后在未提供不可变 prior-anchor 精确载荷的情况下反复猜坐标，连续触发 exact-prior-anchor/body-occurrence 错误，最后只能删除可选图。最终事实答案 PASS，但关系图和约 220 秒额外时延被系统合同冲突消耗。
6. B1443 采用权限并集根修：实际 diagram failure 始终投影为 generation-scoped `failure_ref/action` 精确分支，混在同一 lease 的非图 relation failure 继续由精确 unrelated whole-block replacement 修；不再因混合 carrier 回退到 legacy 坐标。当前草稿中非系统生成、非图目标块的 replace/remove ID 都以 enum 精确投影，使同一事务能处理独立结构错误；新增块继续关闭，必需图不能删除，租约目标不能 whole replace，可选目标只有 typed allowance 才能删除。
7. 新测试覆盖混合 diagram+list lease 的精确 schema、无 legacy selector、无关替换/删除 roster，以及“一个事务删除无关 summary + 用 failure_ref 原子修图”的生产执行。系统不替模型选择 edge/action/label/layout；普通 validation 与 lease post-merge topology 仍完整运行。

状态：

`r929=runner-1/2-pass+human-2/2-pass`；
`B1441=production-positive/core-closed`；
`B1442=implemented/full-suite-pass/pending-natural-activation-replay`；
`B1443=implemented+full-suite-pass+build-pass/pending-production-replay`；
`mixed-repair-capability=typed-union-not-intersection`；
`legacy-diagram-coordinate-selector=hidden-when-live-diagram-ref-exists`；
`request/model/final-prose-hard-gate=forbidden/none`；
`system-answer/conclusion/relation/node/label/layout-authorship=none`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/production-positive-r929`。
