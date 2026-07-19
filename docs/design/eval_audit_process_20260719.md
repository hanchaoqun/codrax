# Eval 过程审计 — 20260719 全量 10 案 × 2 run（main=2081d4cf2）

审计口径：过程面（执行机制），非答案面。禁 LLM-flake；每 FAIL 归因到机制。
结果面：16 PASS + 4 FAIL（zod ×2、data_multifile r2、其余全 PASS）。

---

## 1. FAIL 归因

### 1.1 zod_prefault_symptom 双 FAIL = 确定性机制 bug（verify-failure handoff 把 TestResult.Suite 当 make target 回灌 → `make make`）

verdict 词判定点：`eval/run.sh:1180` — MODE=apply 时读 `run-N.write-apply.json` 的 `report_passed`；两 run 均 `report_passed=false`（report 存在、channel=post_apply_verify、plan_id 匹配，仅 passed=false）→ `write_report_failed`。

**机制链（两 run 同构，确定性）：**

1. 计划阶段产出多变更 plan（r1: plan-48102 changes=4；r2: plan-48698 changes=2），但 apply dispatch 只投放 active slice（W1s）：coder 收到 "Active slice target paths: 1 total"，只 apply 实现修复 `to-json-schema.ts`。orchestrator 记 `apply stage: completed, 1/4 changes applied`（r1 log 48229:339）/ `1/2`（r2 log 48743:353）。
2. verify #1 跑 fixture oracle `make check`（tests/check_prefault_schema.py，要求实现修复+3 falsy 回归测试+default 断言全齐）→ 必然 FAIL（tests 尚未 apply）。失败 TestResult 记 `suite="make"`（make runner 的 Suite=runner 名，非 target）。
3. 控制器 replan_batch → planner 产出 test-only plan（r1: 48229 / r2: 48743）→ apply 测试补丁 → commit + tag `refs/codrax/applied/<plan>`。
4. **verify #2：verifier 按契约调 `run_tests({})`，但 `inheritRunTestsScopeFromVerifyFailureHandoff`（internal/tool/run_tests.go:288/1670）从 verify-failure handoff 继承 scope：`uniqueVerifyFailureSuite(handoff.FailingTests)` 返回 "make"（黑名单 `verifyFailureSuiteReusableAsSelector` 只挡 "make-test"，不挡 "make"），suite 注入后 make 分支（run_tests.go:6270-6283）把 suite 当 target 覆盖 detectMakeTestTarget 的 "check" → 执行 `make make` → "No rule to make target 'make'" exit 2 → reason_code=make_target_missing → verification_status=unavailable → passed=false → unverified 终态 → write_report_failed。**
   - witness r1: run-1.logs/codrax-…-48229.log:2426-2429 `inherited scoped verify target … suite="make"` → `make@. exec: make make`
   - witness r2: run-2.logs/codrax-…-48743.log:2032-2034 同形
5. 反事实证明：run-2 durable 链（seed→af4f6ea 实现修复→cd2c0a2 测试）materialize 后 `make check` **rc=0 全过**。即若 verify #2 跑 surface 选定的 `make check`（test_surface.selected candidate 明明就是 `make check`），run-2 必 PASS。双 FAIL 唯一 proximate cause = suite 继承 bug。

**伴生缺陷（run-1 独有，被 EXPECT 掩盖的交付正确性 bug）：**
- r1 slice-1 apply 后的 checkpoint commit `CommitChangesForPaths` 对 plan 全 paths 做 git add，未 apply 的 `check_prefault_schema.py` pathspec 不存在 → **整个 commit 失败**（仅 WARN，log 48229:340），实现修复留在 worktree 未提交。
- 随后 test-only plan 48229 的 commit 只含测试文件、parent=seed → **durable ref `refs/codrax/applied/plan-…48229` = 只有测试、没有实现修复**（git diff f3374bd..5680b1c 仅 test 文件 +21 行；materialize 后 `make check` FAIL "implementation still uses truthiness"）。用户按输出提示 `/merge` 或 cherry-pick 该 ref 会拿到红测试无修复的交付。
- EXPECT 之所以还 match：eval 在 verdict 时 worktree 尚存（write-apply.json worktree_exists=true），EXPECT 读的是活 worktree 字节（实现修复以未提交态存在）——**EXPECT 面掩盖 durable 交付面的丢修复**。
- 末轮 verifier 面板还向用户断言"本地验证环境缺少测试运行器或依赖"——不实（make 存在、check target 存在），诚实面失真。

**修向候选（P0）：**
a) `verifyFailureSuiteReusableAsSelector` 排除 suite==runner 名（至少 "make"），或 make 车道的 TestResult.Suite 改记实际 target（"check"）；
b) 继承 scope 执行失败（reason=make_target_missing/runner_missing）时回落 TestSurface selected candidate 重跑一次，而非直接 unavailable 定罪；
c) `source=llm_choice` 标签失实（实为 handoff 继承）——诊断面应记 `verify_failure_handoff`；
d) checkpoint commit 按 AppliedSet∩OwnedPaths 提交而非 plan 全 paths（部分 apply 时保住已落地字节的 durable 链）。

### 1.2 data_multifile r2 FAIL = 数据车道义务集自矛盾（entity_resolutions 必需 vs 终段无合法生产者）——DL 家族新形（§7.12 同族）

r1 PASS / r2 FAIL 分叉点：r1 计划路径走了 `normalize_entities`（round4-5 resolutions=5→10）；r2 走 join/derive 路径全程 resolutions=0，但 join 产物使 `EntityStageMaterialized=true`。

**机制链：**
1. `dataTaskArtifactMaterializesEntityStage`（internal/repl/data_task_workflow.go:4248）：任何 kind 含 normalize_entities/enrich_records/**join_records** 的成功 artifact 即判 entity 阶段"已实体化"。
2. 阶段机 `NextStage`（internal/dataworkflow/stage.go:87）：`EntityResolutionRequired && records==0 && !EntityStageMaterialized` 才回 normalize 阶段 → r2 被 materialized 短路，直通 contributions→reconcile→emit_output_contract_answer（ledger_graph.go:52 的 Present 定义同样 `records>0 || materialized`）。
3. 终段校验（internal/dataquery/dataquery.go:2074）却只认 `records>0`：`entity_resolution_required=true but result.entity_resolutions is empty` → repair。
4. **死锁：** emit 段 allowed=[reconcile_artifacts, assemble_answer]，assemble_answer 是纯投影（"projects existing reconcile groups … without changing computed values"，非脚本、无 add_resolution 车道）；修复 planner 两条路全被硬拒——(a) 终段 custom_transform 读原材料 → 被 final-stage material 重读守卫拒（r2 log repair-r1 rejected）；(b) `normalize_entities` → 被 stage gate 拒 `action_outside_allowed_next_stage`（r2 log rejected-r6）。6 轮 repair 耗尽 → terminal failed → read_exit:1 + no_regex_match。
5. 诚实面保住：r2 round-8 已算出正确答案（answer_len=6 = "17,0,5"），系统拒发布而非发错——fail-loud 成立，损失的是可用性。
6. 该死锁族本应被 `TestEveryRequiredLedgerHasReachableProducer_NoTypedDeadlock`（stage.go 注释）钉死，但 pin 只查 NextStage 路由的 producer 可达性；**逃逸点 = 图/阶段机的 "Present=records||materialized" 与 validator 的 "satisfied=records>0" 两个权威定义不一致**（发布面与判定面字节分叉，UXR-1 病根同形）。

**witness：** run-2.logs.all.log（[cli/data] 行）：r1-r6 resolutions=0 贯穿；repair-r1 rejected（custom_transform 读材料）；repair-r6 rejected（normalize_entities 越段）；terminal status=failed data_rounds=13 repair_rounds=6。

**修向候选（P1，需裁定取向）：**
a) 单一权威：validator 的满足判定与阶段机 Present 用同一 typed 谓词（materialized 要么两边都算满足、要么两边都不算）；
b) 若坚持 records>0 才算满足：NextStage 的 materialized 短路必须同时要求 records>0（删 EntityStageMaterialized 逃逸），使欠账在 entity 阶段就还；
c) repair 死锁探测：required-ledger 在当前段无合法 producer 时，允许 typed 回退（重开 entity 阶段）而非让 planner 盲撞 6 轮。

---

## 2. 全 20 run 过程面扫要点

### 2.1 完成门（emit_investigation_complete）
- 10 个 trace run：全部 emit ok=true、0 次 tool 级拒绝、0 downgrade、0 burn —— LENSBURN 修后零烧轮在 8/10 run 成立。
- **例外：donghu_real_frame_multicausal 两 run 各 3 次 emit**：emit 被接受后 orchestrator `accepted investigation closure cannot auto-complete mixed-origin explore window; missing_origin_lanes=current_source`（internal/orchestrator/orchestrator.go:7454）→ 重派 explorer ×2，第三轮仍未满足（rf=0，问题原文明说"只分析这份 trace，不分析代码"），最终按 `current_source_soft/downgradable` 走 runtime_only_caveat 放行。每轮 ~90s：r1 燃 ~180s/318s、trace_query 21 次（其他 trace 案 1-4 次）；r2 同形。**软车道（soft+downgradable）却驱动了 2 轮硬重派**——违反"嘈声/软信号只作软引导"精神；且这是 memory 里 CSP#63 关账时留的"orchestrator:7540 同族备案"残口的实测代价 witness。
- gson（write 案）无 emit_investigation_complete（write 管线不走该门），正常。

### 2.2 门触发 / 降级 / breaker
- 全 20 run：0 hard-reject、0 breaker、CGEC summary 全部 `pre_complete_downgrades=0 strict_findings=0`（仅 1-2 条 advisory）。非致命不硬拦合规（§29.104.13）。
- data r2 的 6 连 repair-reject 是唯一硬拒集群，归因见 1.2（结构死锁，非误伤）。

### 2.3 tool 序列健康（TOOLWIN）
- 10 个 trace run：首次 trace_query 全部 ok=true、全程 trace_query 0 失败、attached-trace 全携（source=attached_trace）。TOOLWIN 修后行为成立。
- 无重试风暴：全 20 run 无 `attempt=2/6` LLM 重试；无 >60s 单轮 LLM（STREAM-WAIT 修后首见全绿窗）。

### 2.4 write 生命周期（gson / zod / patch_go / patch_python）
- patch_go（apply）两 run：plan→apply 1/1→checkpoint→tag ref→verify `go test -json ./...` PASSED 一次成。patch_python（plan-only）两 run 干净。
- gson 两 run：apply 1/1；r1 verify mvn 缺 → 自动升级 make check PASS；r2 bounded verification probe PASS 跳过 suite。单变更 plan = 不进 verify-failure handoff 路径，故未踩 zod 的坑。
- zod：见 1.1（slice 部分 apply→必败 verify→replan 弃剩余 slices→suite 继承 bug）。
- L5：审计时 `.codrax/worktrees/` 已空，无泄漏。
- 备忘：zod r1 出现 `write controller decision normalized: explore_code → apply_plan (auto_executable_plan)` 确定性改写模型决策——按 L2 风险门设计行为，非缺陷，备案。

### 2.5 耗时
- 无 run >8min。最长 zod r1 405s（两整轮 plan/apply/verify + replan）。frame_multicausal r1 318s 中 ~180s 为 2.1 的重派烧耗。c2 r1 217s vs r2 128s = tq 调用数差（10 vs 6），正常方差。

---

## 3. mechanism metrics 汇总（wall/exit/关键计数）

| case | run | verdict | wall(s) | exit | trace_query | read_file | data r/repair | 备注 |
|---|---|---|---|---|---|---|---|---|
| donghu_frame_multicausal | 1 | PASS | 318 | 0 | 21 | 0 | - | 3×emit，2 重派烧 ~180s |
| donghu_frame_multicausal | 2 | PASS | 241 | 0 | 9 | 0 | - | 同形 3×emit |
| donghu_short_runnable | 1/2 | PASS | 117/102 | 0 | 3/3 | 0 | - | 单 emit 干净 |
| c2_dstate_iowait | 1/2 | PASS | 217/128 | 0 | 10/6 | 1/3 | - | 干净 |
| a3_whole_trace_overview | 1/2 | PASS | 77/113 | 0 | 1/2 | 0 | - | 干净 |
| frame_semantic_span_opt | 1/2 | PASS | 147/108 | 0 | 3/1 | 0 | - | 干净 |
| gson_lazy_number (apply) | 1/2 | PASS | 115/105 | 0 | 0 | 5/4 | - | 1 变更 1 plan 一次成 |
| zod_prefault (apply) | 1 | FAIL | 405 | 0 | 0 | 15 | - | make make；durable ref 丢实现修复 |
| zod_prefault (apply) | 2 | FAIL | 276 | 0 | 0 | 10 | - | make make；durable 链完整、check 可过 |
| data_multifile | 1 | PASS | 256 | 0 | 0 | 0 | 12/3 | 走 normalize_entities 路径 |
| data_multifile | 2 | FAIL | 331 | 1 | 0 | 0 | 13/6 | 义务死锁；答案已算对被扣发 |
| patch_go_typo (apply) | 1/2 | PASS | 119/82 | 0 | 0 | 3/2 | - | 一次成 |
| patch_python_typo (plan) | 1/2 | PASS | 50/65 | 0 | 0 | 2/3 | - | 干净 |

---

## 4. GAP 清单（按严重度）

**GAP-1 [P0][确定性] verify-failure handoff 的 suite 继承污染 make 车道 → `make make`**
形：re-verify 恒跑 `make <runner名>`，verify 永 unavailable，多变更 make-oracle 写案永 FAIL。
witness：zod r1 log 48229:2426-2429、r2 log 48743:2032-2034；两 run 同因。
机制：run_tests.go:1670 `inheritRunTestsScopeFromVerifyFailureHandoff` + `uniqueVerifyFailureSuite` 取失败 TestResult.Suite("make")，黑名单漏 "make"；run_tests.go:6270 make 分支 suite 覆写 target。附加：`source=llm_choice` 标签失实；verifier prompt 明令"不许传 suite"却被系统侧静默注入（发布面与执行面矛盾）。
修向：黑名单加 runner 名等值拒 / make 车道 Suite 记真 target / 继承 scope 失败回落 surface candidate 重跑 / 诊断 source 改真值。

**GAP-2 [P1][交付正确性][被 EXPECT 掩盖] 部分 apply + 全 paths checkpoint commit → durable ref 丢已落地修复**
形：slice 部分 apply 后 `CommitChangesForPaths(plan 全 paths)` 因未 apply path 的 git add 128 整体失败（仅 WARN）；后续 replan plan 的 commit parent=seed，`refs/codrax/applied/<最终plan>` 不含前一轮已 apply 的实现修复；/merge、cherry-pick 交付面破损。
witness：zod r1 log 48229:340（pathspec 'check_prefault_schema.py'）；run-1.repo `git diff f3374bd..5680b1c` 仅测试文件；materialize 该 ref 跑 make check FAIL"implementation still uses truthiness"。
机制：stage_hooks.go applyPostHook → writeApplyCommitOwnedPaths 用 plan 全 paths 而非 AppliedSet；commit 失败降级为 WARN 不阻断、不重试子集。
修向：按 AppliedSet∩OwnedPaths 提交；commit 失败升为 typed 信号（阻断 tag/或 partial 语义）；eval 侧 EXPECT 应对 durable ref 而非活 worktree 取字节（防掩盖）。

**GAP-3 [P1][确定性] 数据车道 entity_resolutions 义务两权威定义分叉 → 结构死锁（DL/§7.12 家族新形）**
形：join_records 使 EntityStageMaterialized=true → 阶段机放行至 emit 段；validator 只认 records>0；emit 段 allowed 集无 producer；repair 6 轮全被硬拒；正确答案被扣发，终态 failed。
witness：data r2 terminal + rejected-r1/rejected-r6（路径见 1.2）；r1 同 case 走 normalize_entities 路径 PASS = 分叉实锤。
机制：data_task_workflow.go:4248（join_records 计入 materialized）/ stage.go:87 短路 / ledger_graph.go:52 Present 定义 / dataquery.go:2074 校验定义——两权威不同判。既有 no-deadlock pin 只 pin 路由可达性，未 pin "Present 与 satisfied 同判"。
修向：单一 typed 谓词双面共用；或删 materialized 短路；或死锁探测重开 entity 阶段。需用户裁定取向（materialized 是否算义务已偿）。

**GAP-4 [P2][烧耗][软信号驱动硬重派] mixed-origin 完成门对 pure-trace 问题重派 explorer ×2**
形：用户明言"只分析这份 trace，不分析代码"，emit 被接受后 auto-complete 因 missing_origin_lanes=current_source（soft、downgradable）拒绝 ×2，重派两整轮（~180s/run，trace_query 21 vs 常态 3），第三轮仍靠 runtime_only_caveat 放行——burn 后结局与第一轮相同。
witness：frame_multicausal r1 log:1155/2042、r2 log:1072/1630；其余 4 个 trace 案 0 次。
机制：orchestrator.go:7454 `acceptedClosureMissingRequiredOriginsForAutoComplete`——软车道 lane 缺席被当硬重派条件；即 memory 中 CSP#63 关账残口"orchestrator:7540 同族"的实测代价。
修向：missing lane 为 soft+downgradable 且（问题显式排除代码 / 重派不可能补上该 lane）时首轮即走 caveat 放行；至多重派一次。

**GAP-5 [P2][诚实面] verify unavailable 文案把"目标名错"说成"环境缺 runner/依赖"**
形：`make make` No-rule（自造目标不存在）被归类 environment/unavailable，用户面板称"本地验证环境缺少测试运行器或依赖"——make 与 check target 均存在。
witness：zod r1/r2 final 输出（run-1.out 尾部）；report verification_confidence detail。
机制：run_tests_parsers.go:376 make_target_missing → unavailable 语义桶未区分"surface 有可跑 candidate 未被尝试"。
修向：与 GAP-1(b) 回落联动；unavailable 结论前置条件=无剩余可跑 candidate。

**GAP-6 [P3][观察] W1s slice 流程下 oracle 型 fixture 必踩 verify#1 失败**
形：多 slice plan 只 apply slice-1 即全量 verify，fixture oracle（要求整组交付）必败 → replan 弃剩余 slices 重规划（zod r1 三次 planner dispatch，其一 plan-49192 被 localization critique 拒）。功能上被 replan 兜住，但烧 2-3 轮 planner/verify，且是 GAP-2 的诱发条件。
修向（候选）：plan 有未 apply slices 时 controller 优先"继续 apply 下一 slice"而非 replan；或 verify 失败证据带"剩余 slices 未 apply"typed 提示给 planner。

（附）eval 基建观察：run-N.plan.json 被 apply 步覆写为最终 replan plan（plan 步原 plan 只存 plan-*.final.json）——summary.md 的 "Plan artifacts" 表因此只显示末代 plan，审计时易误读；建议 plan 步产物独立留档。
