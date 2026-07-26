# 读/写模式代表性 eval 过程审计(2026-07-26)

基线 `main=c36e4f940`(审计期间零代码修改;本文档只落 GAP 与冻结修向)。方法:`make` 后经 `eval/parallel_selected.sh`(快照二进制,PARALLEL=2,每例 1 跑)分两批执行;逐例对照 `run-1.out`(模型过程回显)与 `run-1.logs.all.log`(控制面 debug 日志)与产线代码。

## 1. 选例与结果矩阵

| 批 | 案例 | 模式 | 判定 | 用时 | 过程判词 |
|---|------|------|------|-----:|----------|
| 读 | qf_architecture | read | PASS | 273s | ana/exp/fin 各 1 轮,过程干净 |
| 读 | data_json_strict_ids | read(data 工作流) | **FAIL** | 244s | **GAP-EVAL-D1**:正确答案早产出,账本义务螺旋 18 批后 error 退出 |
| 读 | logtri_go | read(log_triage) | PASS | 97s | 锚点对齐+版本漂移 caveat 诚实(250→1820/320→1525 行映射并声明旧构建不可证),健康标本 |
| 读 | read_combo_trace_current_code_boundary | read(perf_triage+trace) | PASS | 143s | 答案健康;**GAP-EVAL-R1**:analyzer 首轮字段名+编码双误烧一次重试 |
| 写 | github_issue_zod_prefault | apply(TS) | PASS | 257s | 写全链(分析→计划→应用→回归测试)健康 |
| 写 | github_issue_gson_lazy_number | apply(Java) | **FAIL** | 98s | **GAP-EVAL-W1** 所杀(非模型错):被 zod 案的 active run 挡死 |
| 写 | patch_java_typo | plan | **FAIL** | 30s | 同 GAP-EVAL-W1;写前分析已正确定位 typo,计划阶段被拒 |

真实模型能力面:读 4/4 内容正确(data 案的**答案**其实正确);写 1/1 真正到达执行的案例通过。三个 FAIL 全部是**系统机制**产物,分述如下。

## 2. GAP-EVAL-W1(P1,写模式):身份不匹配的 active run 阻塞一切新写(并行/共 CWD 串扰)

**实锤链**:gson 与 patch_java_typo 两案均死于同一错误——`write workflow auto-resume refused (fail closed): found active run "wf-1785041767818069000"(goal: 修复 finalizeDefault…zod…) … workflow_repo_root_mismatch: stored repo root ".../github_issue_zod_prefault-.../run-1.repo", current repo root ".../patch_java_typo-.../run-1.repo"`。zod 案先启动,其 run 尚 active;并行的另两案在**同一 CWD**(仓库根)运行 codrax,共享 `<CWD>/.codrax` 持久层(架构文档:runtimeAnchor=<CWD>/.codrax,`--repo` 不改锚)。

**机制定位**(`internal/orchestrator/write_controller_scheduler.go:733-739`):身份感知 finder 无匹配 run 但存在 active run 时,`len(skips)>0` 臂以第一个 mismatch fail-close 拒绝——注释语义为「不在其上静默播种竞争 run」。**判词**:WFID-1 对「防错误续跑」正确;但 `workflow_repo_root_mismatch`(canonical repo root 不同)的 active run 与新写**天然不竞争**(worktree/计划/refs 全部按 repo 隔离),把它纳入阻塞域是保护半径过宽;错误文案还把 `/workflow resume` 推荐给明显无关场景(引导词面误导)。

**冻结修向(不实施,待批)**:①产品侧——阻塞域收窄到同 canonical repo root 的 active run(repo_root_mismatch 的 skips 不再触发拒绝,只在披露里列出);base/goal mismatch(同 repo)维持 fail-close 现状;②eval 基建侧——parallel runner 为每案例隔离运行时锚(独立 CWD 或 CODRAX 运行时目录),使写案例并行互不共享 `.codrax`(本次 PARALLEL=2 写批必然串扰=eval 基建自身缺陷,单跑写案例不受影响);③错误文案在 repo_root_mismatch 形下不应推荐 resume,应提示「该 run 属于其它仓库上下文」。

## 3. GAP-EVAL-D1(P1,data 工作流):正确答案在手,账本义务螺旋输掉终局

**过程时间线**(data_json_strict_ids,244s/18 批):
1. 第 1 轮 custom_transform 脚本只读 users.json → 引擎拒:required material instructions.md 未消费(coverage 门,正确);
2. 模型修复脚本重跑成功,`emitted_payload={"ids":["u1","u3"]}` —— **正确答案自此在工件图里**;
3. 但答案未投影(`final_projection missing`/`no_answer`),期间 custom_transform 被禁用(runtime_failure 后的 plan_guard 停摆保护),模型转 typed 动作;
4. 随后陷入账本义务链:`rule_coverage → decisions → contributions → reconcile` 全 missing/blocked_by_prerequisite,校验器每轮只报 `first_missing` 一个账本,修复循环一轮解锁一个;
5. 第 18 批完成 derive_rules(rule_coverage 落账)后终态校验仍报 `required ledger decisions is missing(producer_actions=[filter_records, qualify_records, compute_contributions])` → error 退出(exit 1)。

**三层机制判词**(对照 `internal/dataworkflow/ledger_graph.go`/`state.go:FirstMissing`/`plan_guard.go`):
- **D1a 产出通道与账本义务不对等**:decisions/contributions 账本的 producer 闭集是 typed 动作族(filter_records/qualify_records/compute_contributions);custom_transform 成功产物**不铸任何一本**——脚本通道完成计算后,模型被迫用 typed 动作把同一工作重做一遍才能过账。义务是给「审计轨迹」设计的(合理),但没有为「答案已产出」的形提供快速通道或豁免裁定。
- **D1b first_missing 一次一报**(EMITBURN 同族形):账本缺口是静态可全清单披露的(ledger_graph 前置图确定),但校验器每轮只报第一个,模型每轮只能修一个——四本账=至少四轮纯开销;审计面其实打印过 `decision_next_actions` 全清单,说明信息在场、披露面窄。
- **D1c 前置链串行解锁**:blocked_by_prerequisite 使 decisions 在 rule_coverage 落账前不可生产,即便模型想并行铸账也不行。

**冻结修向(不实施,待批)**:①终态校验/修复提示一次披露**全部**缺失账本+各自 producer(参照 EMITBURN-2 全清单 reject 先例);②一轮允许多账本动作(plan 批内多 action 已支持,transition 只取 1 action 的策略对账本回填场景放宽);③裁定件:custom_transform 成功产物的账本豁免或自动铸账(emitted_payload 携带的 filter/qualify 语义可否自动折算 decisions/contributions)——涉及审计轨迹语义,需用户裁定;④`banned:\`\`\`` 判词澄清:围栏来自 think 回显内嵌(`Let me construct a proper plan: \`\`\`json`),非答案面违规——eval 断言域覆盖全 stdout,失败路径的过程回显必然携围栏;若保留该断言,应限定到答案区段。

## 4. GAP-EVAL-R1(P2,analyzer 首轮字段双误):requested_ 写成 required_ + 嵌套对象字符串化

trace combo 案 debug 日志实锤:iter=0 的 emit_analysis 携 `"required_answer_dimensions":"{\"confidence\":0.9,…}"`——字段名错(schema 为 `requested_answer_dimensions`)且值被字符串化;strict-decode 拒后 iter=1 以正确字段名+对象形重发成功。成本=一整轮 analyzer 重试(该案 ~16s)。**修向(R2' 第 5 处族)**:strict-decode 的 MisplacedFieldHint 表加 `required_answer_dimensions→requested_answer_dimensions` 别名提示;字符串化对象已有通用 hint 则确认其覆盖该字段。属既有 R2' 机制的增量登记,非新机制。

## 5. PASS 面过程观察(记档,无行动)

- **logtri_go**:历史 flake 家族本跑健康——日志行号→当前锚点映射(250→1820/320→1525)+「旧构建不可证 guard 在场性」caveat,正是 §29 口径纪律的正向标本。
- **read_combo_trace**:perf_triage+trace_query 双 runtime 车道协同正常;analyzer 双维度(trace 观察+源码机制)typed 拆分正确;答案对 50ms 阈值给出精确 86.111ms 判定+采集范围诚实边界。
- **zod apply**:写全链(写前分析→计划→应用→false/0/"" 三回归测试)一次通过,无 replan,257s。

## 6. 处置清单

| GAP | 级别 | 处置 |
|-----|------|------|
| GAP-EVAL-W1 产品侧(repo_root mismatch 阻塞域) | P1 | 冻结修向待批(动 WFID-1 身份门语义,属 L2 邻域需谨慎评审) |
| GAP-EVAL-W1 eval 侧(runner 无 per-case 运行时隔离) | P1(eval 基建) | 冻结修向待批;短期规避=写案例不并行 |
| GAP-EVAL-D1a/b/c(data 账本螺旋) | P1 | 冻结修向待批;③为裁定件 |
| GAP-EVAL-D1d(banned 断言域) | P3(eval 基建) | 记档 |
| GAP-EVAL-R1(字段别名 hint) | P2 | 冻结修向(R2' 第5处登记),可随下一小批走 |
