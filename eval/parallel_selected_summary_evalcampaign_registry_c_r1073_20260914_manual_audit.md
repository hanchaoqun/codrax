# r1073：注册名读例 / C 单行修复人工审计

- date: 2026-09-15T03:40:28Z
- sweep_start_ts: 20260914-204028
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁构建 revision=`cd83d996cbb5`，基于已推送 B1634c/B1692。严格两路并行、每例一次，未追加追绿。机器表保留原判；下面将代码正确性、证据供给、正式验证与展示质量分别审计，不以独立后验覆盖原生产报告。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260914-204028 | answer_regex,answer_contains | none | 102s | 30 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial：核心事实正确，返回值引用不精确、标题含内部编号 | 同一聚合在两处模型输入中被赋予不同证明资格，P1 待公共复现；本轮答案成员无误 |
| 2 | patch_c_typo | FAIL | eval/results/patch_c_typo-20260914-204028 | write_apply,write_patch_oracle,answer_contains | none | 103s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial：补丁及独立原生检查通过，正式逐合同证明未闭 | 保留 unverified；B1319 改后目标绑定与 B1561 原生证明继承债，不是 B1634c 回归 |

## 1. 批次选择与保真

- 当前 243 个 case：215 read、25 apply、3 plan。按用户价值、确定性错误风险、近期覆盖、语言/模式跨度和本机可验收性排序；注册名题上次 2026-08-15，C typo 上次 2026-09-09，避免紧接刚跑过的 Go/Rust 追绿。
- 读例覆盖“关系成员、注册名/返回值、总数”；写例覆盖 C 源码修改、构建/运行、正式交付身份。图与 Trace 均不是本轮必需输出，未强造图或把未触发通道算生产正证。
- `CAP=5 PARALLEL=2 TIMEOUT=1200`；20:40:28 启动，读例 20:42:10、写例 20:42:11 结束。表中 102/103s 是外层调度耗时，两例内部 `wall_seconds` 均为 100s。
- `.codrax/tmp/20260914-b1692-build.log` 保留构建版本，`.codrax/tmp/20260914-r1073-run.log` 保留不可变二进制快照路径、运行与结果；两 case 和两 C fixture 的 `.codrax/tmp/20260914-r1073-case-fixtures-before.sha` 复核通过。
- 原机器汇总、case、oracle、fixture、最终答案、计划、报告与日志均未改写；独立 C 检查前后六个正式工件 SHA 相同。人工审计不会将原 FAIL 改成 PASS。

## 2. 读例：事实正确，证据口径与展示仍有残差

实际问题要求默认注册成员的总数、完整名称、注册和 `Name()` 返回值证据。源码核对：

- `internal/agent/subagent.go:63–65` 只有 `r.Register(NewSubExplorer(deps))`。
- `internal/agent/sub_explorer.go:28–34` 构造 `SubExplorer`，`Name()` 在第 33 行返回 `"explorer"`。
- `SubAgentRegistry.Register` 在 `subagent.go:34–38` 用 `sa.Name()` 作为键；`Names` 返回该 map 的键。
- 因此总数 1、完整成员 `{explorer}` 正确；没有将另一套普通 Agent 注册表或配置路由名 `sub_explorer` 混入。工厂构造、实例注册和 `Register` 调 `Name()` 是不同关系，不能自行串成工厂直接调用 `Name()`。

最终答案：`.codrax/output/20260914-204208.230-79443.md`（同名 HTML 保留）。主要日志：`eval/results/qf_relation_subagent_registry-20260914-204028/run-1.logs/codrax-20260914-204029-000-79443.log`。

过程与实际供给：

1. 分析阶段曾错误声明 source inventory；日志 606–609 的既有关系正规化移除不适用的清单权威，保留非 scalar 成员集合/总数，不能说模型首次分类全部正确。本例 intent=enumerate 且 HasPerMemberTable=true，在原 roleBindingScalarShapeEligible 就已不适用，未进入 B1691 新增整题保护分支；该新分支生产覆盖 N/A，只能记兼容未回归。一个“注册证据”维度短语未通过原文连续出处校验，但原问与注册调查仍保留，暂未确认新增损害。
2. 源码读取与探索给出了两处真实实现；基本清单完成提示只催交接，不宣称全合同已证明。最终模型上下文含精确返回 `SubExplorer.Name returns "explorer"`，`ev-14947a22e1cbfda0` 指向第 33 行（日志 2097/2154/2301）。这是 B1692 返回值供给的生产正向覆盖，不代表全部语言语法被覆盖。
3. 最终首次结构稿后局部 patch 一次补总数，零成文拒绝、零语义重写、零工具不可用/剪枝。上下文 60197/200000（30%），没有预算挤压证据。
4. **引用精度 P2/观察**：模型实际选的是 `Name` 定义行 32 与注册行 64（日志 2538），最终引用行 32 只展示签名，未直接展示返回字面值的行 33。供给不是缺失，不能用 B1692 再造同类修复；先独立区分模型选证和结构引用绑定，不由系统替换引用或改写正文。
5. **B1696 / P2 教学与可见标签混杂**：答案标题为“第 4 维：总数”。修订提示日志 2556–2560 一面说只列用户标签，一面又发该编号前缀；初始/修订提示源位于 `answer_document_evaluator.go:8050/8081/16903/16913`。下一片应把内部索引与用户标签分开供给并验证中英文初始/修订面，不新增关键词硬门或末端标题替换。
6. **B1695 / P1 同一聚合的双资格**：日志 2285 的 `aggregate:0#current_source` 被标成 `principal_answer / hard / independently_proven`；日志 2385 同一 `member_set` 却是 `advisory_model_inference / not_authorized`。唯一 support_ref 为定义行 32，缺少成员关系的系统凭证。`aggregateFactHasIndependentTypedAuthority` 允许精确源码坐标抬权，`AnswerAggregateFactAuthorizesPrincipalContract` 则对关系/工作流成员要求独立证明，两个 consumer 不同源。此为真实模型输入矛盾，尚未证明造成本轮错误成员或完整公共入口复现；下一片先复现 `CompileObservationLedger` 到两处模型供给，再做共享、分字段的资格判定。不能简单让二者全部变绿或全部降级：保合法 typed relation/source inventory/current-source 值及 Trace 独立观测，不把定义坐标升级为成员、方向、顺序和因果证明。

读例无图，关系渲染/语法恢复记 N/A；没有图不是本题关系遗漏的证据。人工结论为核心事实 pass、整题引用/展示 partial，不把机器 PASS 等同于所有质量项闭环。

## 3. C 写例：补丁正确，正式证明确实尚未闭合

正式计划 `plan-1789443674115965000-79456`，最终源码提交 `f0734f44b34eab139dd3f28553c9b4baf578e66a`，applied ref、checkpoint、final/report/source owner 均一致；本轮无 proof-only 尾计划或多 owner，同路径同秒缺口并未触发。

实际修改仅 `main.c:19` 的 `retrun buf; → return buf;`，单文件 1+/1-，Makefile、测试与其期望完全不变。正式提交的源码字节、权限（100644）与 `run-1.applied-tree` 一致，未把验证生成的未跟踪 `main` 程序加入交付。计划摘要有拼写解释不准确的模型措辞，但未影响精确补丁；不以此增加正文门。

计划/执行全过程：

1. 第一次 `emit_change_plan` 提交 Python probe，因不能直接执行 C 目标被精确拒绝一次（计划日志 1173，`verification_probe_target_language_mismatch`）。系统提示已解释原生断言及 `project_test_observations` 路径；模型随后移除 probe，仅保留 acceptance_tests。不是 JSON 畸形，也不是成文校验重试。
2. 两次正式 `run_tests {}` 都跑 `make test`。聚合测试通过，记录命令退出 0、覆盖 `main.c`；原 Makefile 编译并运行默认/指定名称两次，只检查退出码，不断言精确 stdout。不能把 aggregate 成功签成每条声明都已证明。
3. 两条 required 合同 `compile-fixed` 与 `no-typo` 仍无各自合法观察。最终 `completion=unverified / verification_proof_incomplete`，机器据此 FAIL。用户末卡明确区分“测试通过”和“未完全验证”，没有静默宣称全部成功。
4. **B1319 继承新见证**：源码观察拒绝被替换旧行的原因是 `post_apply_source_contract_line_unresolved`。旧行映射 `patch_effect_line_remap.go:128` 在删除时返回 `removed_by_patch`；`run_tests_source_contract.go:81` 不借相邻新行作旧证。该保守边界和原测试应保留。计划虽有准确新旧字节（计划 207–229），合同只有旧 `evidence_ref`，缺少合同选择的改后目标/版本/替换关系。
5. **不能只补行映射追绿**：本轮 `no-typo.expected` 实际是整句“main.c 第 19 行不存在字符串 retrun”，不是字面值 `retrun`。schema 已要求精确 expected；若只让 `not_contains` 在新行比较整句，即使拼写错误仍在，也可能通过。需模型明确选择精确 literal，并与当前代次源码/改后目标绑定；未知只披露，禁止从说明文字抽词或把一删一增冒充旧行存续。
6. **B1561 原生逐合同证明债**：编译轴须精确命令/断言与 contract_ref、期望值及交付代次绑定。重复执行同一 Make 聚合不能新增这些证明，不能与 B1634c 的评测计划归属混号。

独立人工检查在新临时目录 `.codrax/tmp/r1073-c-native.oI73tu/` 编译实际交付源码（`cc -Wall -Wextra -Werror -O0`），默认/命名/空参数/双参数四组精确输出均通过。检查日志 `probe.log` 与正式工件 before SHA 保留；该后验未导入 runtime 报告，不替原 case 补证明、不改正式 FAIL。

## 4. 后续按 ROI 排队，不积压隐性债务

| 优先级 | 项目 | 下一步与闭环条件 |
|---|---|---|
| P1 / 下一小批 | B1695 聚合事实资格双源 | 公共 ledger→两处最终提示复现；同一事实/字段共用证明资格，合法来源与 Trace 反控，不能把当前源码坐标当关系证明 |
| P1 / 独立施工 | B1693 同秒物化、B1694 同坐标异语义合并 | 保留确定性原失败；前者需正式全交付 owner+seed+拓扑，后者需 claim 身份，不任取历史 ref 或简单增重复证据 |
| P1 / 继承能力 | B1319 改后源码目标、B1561 原生逐合同证明 | 先设计精确代次/目标/literal 与原生断言关联，保删除行/聚合测试边界；不能只修本例文字 |
| P2 | B1696 标签与引用精度观察 | 初始/修订中英文教学分离内部索引；引用独立公共审计，不强改模型正文/标题/引用 |
| P1 / 原开放项 | 完整 occurrence 库存 A/B、自递归解析及图表达 | 沿既有分类矩阵继续，不以本轮无图或单个返回值正证销账 |

本轮未触发 Trace 生产问题，不新增其生产覆盖声明。代码冻结全仓中 tracequery/tracediag/转换相关包已通过；显式时间窗、链上根因、占时/可消双轴、IO/业务线索、投影/补齐不改。默认首响应 600s、真实字节静默 300s、非流式 600s 仍在；reasoning/tool/heartbeat 等真实流活动不会因 4ms/旧 4m 无可见正文降级，调用方取消和显式总预算保持独立。本轮没有长连接超过旧 4m 的生产见证，不能拿两例 100s 时长冒充专项验证。
