# r1082 人工审计：Python 多仓写修复 + 显式时序图

- 从已推送 `e02a24760` 清洁构建，版本 `0.1.20260916/e02a247604bf/2026-09-16T07:36:23Z`；2026-09-16 07:36:48Z 启动，07:44:36Z 结束。两例并行、各一次，无第三例或追绿重跑。
- `PARALLEL=2 / TIMEOUT=1200`；Python原plan15+apply24，sequence原read15。保留CAP5环境，但两例均未命中read-multirepo cap分支。
- 当前243例（215read/25apply/3plan），按业务影响、稀疏模式、明确图需求、最近覆盖、oracle强度及成本选本对：Python最近r1068，sequence最近r1054。
- [机器原汇总](parallel_selected_summary_evalcampaign_memoclaw_sequence_r1082_20260916.md)：**1 PASS / 1 FAIL**，原判不改。

| 用例 | 机器判定 | 人工判定 | 时间 / context | 关键事实 |
|---|---|---|---|---|
| `github_issue_memoclaw_text_search_multirepo_py` | FAIL：正式验证未闭合 | 补丁行为pass；端到端验证交付partial | runner281s / metric277s；28% | sync/async正确，独立实际执行160/160通过；系统如实标未完全验证 |
| `qf_sequence_analyzer_gate` | PASS | FAIL | runner468s / metric465s；42% | 原图可解析渲染，方向正确但同caller时序倒置，阶段表达不足；存在系统提示域冲突 |

## 1. Python：交付恢复正确不等于正式验证完成

结果目录：`eval/results/github_issue_memoclaw_text_search_multirepo_py-20260916-003649`。

1. 原请求以仓内API reference为准，只修Python SDK同步/异步接口，不改reference或弱化测试。最终 `run-1.applied-tree/memoclaw/client.py` 移除urlencode，两方法均POST `/v1/search`，JSON包含query/limit及非空namespace；默认limit10、返回对象、异常与async await保留。只有该源文件变更，原fixture及兄弟仓未被污染。
2. B1693自然触发 **一个源码owner + 后续纯验证计划**：seed `fd414621f0f4b19125aae667d0cdab680c38bc53`；源码计划 `plan-1789544347328168000-29744`、commit `053f13ceaea65893627d2456b97fa66c55dbffe2`；最终 `plan-1789544449929605000-29945` 为proof-only。系统materialization收据available，消费者resolved，只选择实际源码owner，没有被无源码尾计划覆盖回初始树。多owner/restore/同秒分支本轮 **N/A**，仍仅有确定性测试，不能冒称生产全覆盖。
3. 最终final.json记录三条验证结果passed，但整体 `completion.verdict=unverified / verification_proof_incomplete`，不矛盾：通过的是已运行检查，尚未获得完整行为/逐合同证明。changed_path_coverage的capability仍为unknown，不能说系统已精确识别为static-only。原Make检查实际为AST/文本判断；尾计划继续复制断言，并未调用text_search。自行打印VERIFIED不授正式证明。
4. 未闭原因含 `project_test_assertion_not_observed`、`verification_probe_target_execution_unobserved`、`verification_probe_missing_soft_contract_ref`。继承B1561/B1678原生目标执行与逐断言证明债，不降低证明要求，不对最后文字标签另设硬门。source-only/import-only probe的拒绝与最终成文/JSON错误分开计。
5. 独立后验只在 `.codrax/tmp/20260916-r1082-memoclaw-native/`，不送模型、不改计划/报告/正式证明。check_text_search.py使用mock transport执行真实方法：6query×3limit×4namespace×2方法=144，加8base URL和8异常=160。旧fixture请求契约0/160，新交付160/160；返回identity152/152、异常identity8/8、真实await80/80、执行错误0。收据original-red.json/after-delivery.json，源码前后SHA相同。有限后验不回填正式报告，不冒称所有输入完备证明。
6. 上下文含正确reference、两个方法、原测试和焦点仓，定位实现未缺证。context28%，read10/repo_map1、finalizer0、JSON修复0；不是上下文挤压或成文合同冲突。CLI明确“未完全验证”，没有空答案或伪装全通过。

主要原件：run-1.out、run-1.materialization.json、run-1.delivery-seed.json、两组原plan/report/final；日志 `run-1.logs/codrax-20260916-003650-000-29744.log` 与 `run-1.logs/codrax-20260916-003907-000-29945.log`。本例无PLAN_EXPECT，没有plan-oracle.json属N/A。

## 2. 时序图：解析通过，业务时序仍不正确

结果目录：`eval/results/qf_sequence_analyzer_gate-20260916-003649`；原过程日志 `run-1.logs/codrax-20260916-003650-000-29704.log`（下称Q）；终稿 `.codrax/output/20260916-004434.521-29704.md` / `.html`，均未改写。

1. 源码为 `buildAnalysisIR → gate.RunWith ← gate.Run`，并无buildAnalysisIR到gate.Run的已证有向路径。终稿保留此边界，两条方向及图后清单位置正确，未重演reverse-edge错误。
2. MD15先画buildAnalysisIR→RunWith，MD17才画buildAnalysisIR→analyzerGraphForNormalize；同caller源码位点为analyzer.go:1921在前、2725在后。单条边存在不等于时序正确；两入口共享callee也不证明两次调用发生于同一次运行。
3. 图仅两条边界关系加一个早期helper；Normalize2323、Amplify2348、Compile2530等虽部分列于图后，不能替代图中时序。不把某一固定helper名单设成通用硬门；应保留模型基于证据选择、表达有序阶段的通道。
4. MD26将Amplify称为扩展实体/补隐含关联，实际analyzer.go:2332–2339说明其补齐模型省略的可选请求字段，且正确说明已在handoff Q2201。这是模型成文职责误述。RecomputeBudget直接依据实际hypothesis数量的解释亦不足。源码/方向证据已供给，不以缺上下文解释全部错误。
5. 原图用仓内 `internal/preview/assets/mermaid.min.js` 实际离线parse/render成功，sequence、SVG22334bytes；两名审计者复核同一未修改原件，不拿弃稿结果作最终收据。语法合法与语义正确分开判。

### 2.1 B1708：确认系统呈现域与参与者候选不一致

Q2983–2994要求必需图只画exact endpoint-boundary subgraph；Q3291排除另外57条已证关系进入principal diagram。但Q3298给buildAnalysisIR的首两项候选只有早期helper analyzerGraphForNormalize/analyzerSymbolResolver，随后要求每个有候选incident participant选择一项；它们不在两边模板内。这是同轮供给域冲突，不是模型箭头错，更不能将所有拒绝都算波动。

代码来源：`types/answer_semantic_view_compile.go::projectCallChainEndpointBoundaryFacetAuthority` 收窄图指引；`agent/answer_document_evaluator.go::renderAnswerDocMechanismRelationAuthority` 仅供边界模板；`agent/answer_document_flow_participant_coverage.go` 候选却读取全量evidence。第二supporting图理论上可避开requirement facet域计数，但“exactly1”教学未解释，不能靠模型猜出口。

根修方向：共享区分“必需端点边界关系”与“可独立呈现有证调用”，统一初稿/修补供给；保留principal_path_edge成员约束、原sender/精确方向/不可达结论。静态源码顺序不能授运行时共执行、callee串联或并发。公开BuildInitialInstruction overlay已连续3次复现候选域交集为空，端点方向/无路径保护3次通过，收据 `.codrax/tmp/20260916-b1708-overlay-red-count3.log`，复现源/overlay/patch同前缀留档。失败针不进入正常suite，未修改产品；本审计不冒称已修。

### 2.2 重试须分因，不把10拒绝/9patch混为一种

| Q位置 | 观察 | 定性 |
|---|---|---|
| 1874–1877 | 带位置修饰成员已有refs却报无法解析 | 引用身份诊断观察项，待复现，非已证JSON错 |
| 1906/1981/2067 | 未证有向路径、错误waiver、端点体读取未闭 | 现有拒绝有依据，探索与成文次数分列 |
| 3382–3470 | blocks双编码，一轮内层JSON损坏；另一轮缺identity/元数据 | 模型编码/载体错误；安全结构恢复不授事实，拒绝有损恢复正确 |
| 3509/3575/3611 | 同一s2同时whole replace与atomic edit | 重复模型协议错误，未staged且旧base保留 |
| 3657–3730 | s2/s3 anchors及participant待补 | B1707六字段Action在3678–3686自然出现且含visible_label，不是仍缺教学 |
| 3775 | 把helper目标绑定为buildAnalysisIR | 模型端点错配 |
| 3820–3826 | 选当前发布n3→n2 add后，同边报removed/expanded | 候选/identity恢复/租约一致性疑点，待公共复现，未签根因 |
| 3871及终稿 | 补边固定追加图尾，形成倒序 | 活跃该图租约时无插入位置；租约解决后完整body通道恢复，不能声称全程无修正通道 |
| 3946–3969 | 修好s2，s3改独立事实 | 接受 |
| 3975–4004 | 接受后advisory仍将数组放不支持的field edit | 最后1拒保留已接受答案，非答案消失/超时降级 |

独立公开patch探针进一步证实：active diagram lease下，add将早期Prepare追加在Finish后且同图whole replacement被拒；图义务解决、只剩列表租约时，schema恢复diagram完整替换，公开Execute接受模型自写Prepare-before-Finish且原样保存。因此时序补边存在局部表达摩擦，但不能冒称没有恢复出口，也不擅自自动排序模型图。r1082后期s1退出目标，模型选择保持s1不变。测试与命令收据在 `/tmp/codrax-r1082-sequence-audit/sequence_position_audit_test.go` / `receipt.txt`，公开ParametersFor+Execute PASS1.130s，不是新live。

机器PASS仅覆盖端点、图存在及部分带行号关系，不能签时序/职责完备。Q有61条grounded source facts、59条typed关系但仅2条进主模板；context42%、无工具裁剪，不以context满解释缺口。

## 3. 封存与保护面

- `20260916-r1082-{cases,fixtures}-before.sha` 核原case/全部本多仓fixture不变，B1693 Go/build和eval冻结清单亦复核通过。
- `20260916-r1082-results-audit.sha` 封存16个明确列出的结果，`20260916-r1082-answer-audit.sha` 封存MD/HTML；不冒称全部缓存/工作树快照。机器汇总SHA256=`9eebe21901676176f1272bc64196f521ead9bd9fc91de44813e7438b199a0887`。
- 本对无运行时附件，Trace投影/IO/链上主因及完整图矩阵均N/A；全仓确定性保护见B1693验收，不能借无触发签全能力通过。
- 活跃流六类边界count3与超时默认值测试通过：首响应600s、中途真实静默300s、非流式600s；不因4ms或旧4m没有可见正文而降级。显式取消/独立任务预算仍另管，不承诺无限等待。
- 下一高ROI先B1708公开复现与同源范围，再核原子补边位置/identity租约；B1561/B1678原生证明、旧图矩阵及模型职责解释债仍留账。原答案/oracle/正式验证结论不改。
