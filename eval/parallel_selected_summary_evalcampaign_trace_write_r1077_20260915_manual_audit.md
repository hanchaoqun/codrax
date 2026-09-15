# r1077 人工审计：生成加载器写修复与流水线图

## 批次与证据边界

2026-09-15 05:01:17 America/Los_Angeles启动，清洁构建 revision `03ddd7e2defd`（B1702）。243例库存：215 read / 25 apply / 3 plan；按用户影响、证明风险、模式/表达覆盖、最近回放时间与成本排序。NAPI apply最近r947/08-30，原测试仅静态检查；pipeline read最近r999/08-31，补最近四批没有自然Mermaid的覆盖。严格 PARALLEL=2/CAP=5/TIMEOUT=1200，每例一次，没有第三例或追绿重跑。

机器原汇总 `eval/parallel_selected_summary_evalcampaign_trace_write_r1077_20260915.md`，runner `.codrax/tmp/20260915-r1077-runner.log`，未经人工修改。结束后 `20260915-r1077-cases-before.sha` 与 `-fixture-before.sha` 逐项一致，原case/fixture/oracle/测试期望/答案未改。

| 用例 | 机器结果 | 人工结论 | 保留问题 |
|---|---|---|---|
| qf_diagram_pipeline | PASS，196s（运行指标194s） | 四主阶段、职责与3条顺序关系完整，核心请求通过，但非全文零瑕疵 | 浏览器视觉审计不可用；部分绝对化表述和内部术语 |
| github_issue_napi_force_wasi_env_symptom | FAIL，499s（进程wall496s），plan_batch_followup_unverified | 实际补丁正确、独立生成JS执行36/36通过；正式证明未闭且最终状态文案错误，不能判完整交付通过 | B1703状态归属、B1704不可达修复教学，逐合同证明能力仍开放 |

## 读模式：关系、所有权与语法自愈

产物 `.codrax/output/20260915-050431.153-48692.{md,html}`；日志 `eval/results/qf_diagram_pipeline-20260915-050117/run-1.logs/codrax-20260915-050119-000-48692.log`。

1. 最终图四明确节点Analyze/Explore/Extract/Finalize、三条正向顺序边，无多余角色、虚构调用或丢边。日志3021原始提交中三条edge_anchors由模型选择precedence，身份是对应Agent，不是把阶段先后强铸为直接call。职责引用stage_binding.go:47/61/73/85指向对应定义，但单行声明不足以独立涵盖所有扩展说明。
2. 模型所获主序列、职责、确定性/模型边界准确（2533–2550），并区分顺序、调用、控制关系和显示标签（2442–2447）；主阶段与辅助函数关系分开。原JSON与最终Markdown逐字比较：摘要和四段正文保留；图只把四标签内字面量反斜杠n安全改为<br/>，节点、边、方向不变。同repair hash记两次不是两次改动；引用原文补齐及3项由既有枚举推导的claim-form metadata未产生新关系。
3. finalizer一次成文，0拒绝/patch/重写，无JSON恢复。Analyzer一次误归类用户明确提及的参与者、Explorer一次修正证据定位，原教学已明确，暂记局部遵循波动，不新增关键词门。
4. 保留精度缺陷：本例extract确定性跳过（1822），答案却概括四Agent依次执行；Analyzer本例使用repo_map（562），而答案称“不直接访问源码文件”，过于绝对。职责供给已准确，不用系统改写答案/叠加硬句式修正。Terminal=true等术语偏多，保留质量观察。
5. HTML存在；浏览器拒绝访问file URL。未另起服务/换浏览器绕过策略，**无真实浏览器视觉通过证据**。
6. P2 **B1705-DIAGRAMKINDRELATIONMINIMUM1**：DefaultEdgeRelationsForKind把所有flow默认映射guard Min1、sequence映射call Min1，纯precedence图不应因此补假条件/调用。检查当前为telemetry-only advisory，没有拒绝/重写。本次3105确有diagram_edges advisory，但完整detail未记录，具体归因只能标源码推定。后续按typed关系意图选语义最低要求，保持每条关系精确证据门，不按图形种类推导业务关系。

## 写模式：补丁正确不等于正式验证完成

结果目录 `eval/results/github_issue_napi_force_wasi_env_symptom-20260915-050117`；下称apply日志为 `run-1.logs/codrax-20260915-050404-000-48919.log`。

1. 隔离交付commit d304f66425cf11fce0b37e29f7143387f6d1e61c仅改模板一行：truthy改为精确 === 'true' || === 'error'。Python/TS测试与Makefile不改。native缺失仍fallback，false/0/空/未设置不强制，true/error仍强制。源仓main保留seed提交，工作树清理，run-1.applied-tree完整保留交付。
2. 独立原生后验用Node24.19.0导入**实际交付TS模板**、执行整个生成JS，模拟native/WASI，核require顺序、exports身份及异常身份。9环境值×native有无×WASI有无=36格。原始夹具26/36（10失败），补丁36/36。脚本/收据：`.codrax/tmp/20260915-r1077-napi-native-postcheck.mjs`、`-napi-original-red.log`、`-napi-delivery-postcheck.log`。不回填正式计划证明/机器判定/用户答案，不声称TS类型检查。
3. 正式post-apply make check仅Python源文本检查。report.passed但proof明确weak/production_verification_source_static_only，不冒称JS行为证明；controller追加累计验证。final report保applied主计划归属/coherent交付/completion=unverified，机器FAIL诚实。
4. 后续5个实际planner探测：3个JS import/syntax失败，2个Python静态成功。首JS缺模块（1419–1420），后两为CJS node -e中ESM导入（2110–2111、2174–2175）。模型可见错误但仍用Python静态代替，跨语言证明/静态签绿被拒正确。早期模型自创native存在就绝不force覆盖的候选，系统标planning-only，最终补丁未采纳。
5. **B1702自然触发N/A**：无post-apply plan probe；planner dry-run无active PlanID，不虚构plan/execution身份强行触发。B1702仍以公共回归/count3/race/全仓为交付证据，不借本批宣称生产正证。
6. **B1703-PROOFFOLLOWUPSTATUSOWNERSHIP1/P1，确认**：planPostHook在后续planner未安装新plan时生成“本轮没生成改动方案/请把目标说具体”，写入Result。外层正确保留已应用主批并完成为unverified，却保留这条子阶段错误再加终态卡（3244、3252–3259）。应在系统状态生产者按当前typed workflow避免子阶段抢整次结论，不扫描/替换已有模型答案；初次plan/plan-only/已应用失败修复独立保原语义。进入下一小片。
7. **B1704-PROOFFOLLOWUPRECOVERYTEACHINGSCOPE1/P1，确认**：typed proof-only批禁止新增/编辑测试（1129），source-free计划禁止project_test_observations（1495/2323）；language-mismatch修复包却无条件推荐include test file + project_test_observations（1547/2404）。拒绝资格正确，教学未随当前计划能力收窄，增加无效重试。已有typed sentinel资格应选择软指导，普通编辑计划保native测试通道；不放宽字段/语言门、不为追绿造空计划。进入下一小片。

## 总体边界

- 本对无Trace，显式窗/因果投影/自动补齐/链上IO、供给、语义、业务线索live覆盖N/A。B1702末版全仓86有测试包通过守旧能力。
- 首响应600s、真实字节静默300s、非流式600s不变。读日志2987流活跃，3039的4ms是成功提交后解析耗时，非早停/降级阈值。
- B1703/B1704按统一机制修复，B1705 P2不升级硬门；B1694、B1626/EVAL-B36-SYSAUTH1、全图表达和原生逐合同证明债继续开放，不重复本批追机器PASS。
