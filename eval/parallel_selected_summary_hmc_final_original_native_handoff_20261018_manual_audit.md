# HMC261 人工审计：原件交付与采样解释分开验收

机器2/2；按本批各自限定合同，人工1/2：A完整采样答案FAIL，B真实最终说明及只读登记终态PASS。不倒签§260缺失的最终说明，不把B说成完整CLI/GUI验收。79唯一＝24交付＋55开放保持。

## A：真实sample-only入口与成文

原件目录：[A运行](results/hmc_final_original_native_handoff_20261018/trace_binary_sample_only-20261018-200047/)。完整独立报告归档于本批`verification-receipts-v1/codrax-hmc261-sample-manual-audit-v1.md`，SHA`6d6d38b69eee32101c571f8b0448ed98b28c29404f1c6ae9e66fead4b514cbc7`。

| 核对范围 | 结论 |
|---|---|
| 两级不可列举目录、完整真实原件、默认准备 | PASS：30,062,585B原件→12000记录，非预览代替全集 |
| 请求窗、目标、独立统计 | PASS：200ms窗1393记录；目标164条/259权重；两热点各82条、174/85cycles，记录CPU1/CPU0 |
| 当前解释交接 | PASS：1679B interpretation完整进入finalizer，恰1份；407B reason未重传，旧Current model synthesis为0份 |
| 原生视图完整性 | 如实披露1/2完整；过滤统计有效，过滤树不可用不等于真零；自动补齐无过滤树完整164/259 |
| 完整正文 | FAIL：采样分布推执行时间、排除过载/死循环；分组双峰推反复进出内核；业务符号推已发生的注册/IPC关系 |
| 图与根因投影 | 未触发：无图；附件为`trace_root_cause_contract_not_active`，不是链筛后无候选；同窗window_stats自动补齐实际通过 |

本次没有把67.18%数值直接写为实际耗时比，也没有重现4ms执行/196ms停顿；旧失败保留，本次精准记录定性的执行/因果越权。不能把改善抹去，也不能因表格正确抵销正文错误。

完整查询、未知sample_kind、记录CPU、非耗时边界均已进入成文上下文。实际初轮finalizer user消息108307B，完整独立解释可逐字匹配；错误已出现在interpretation，finalizer又升级为肯定结论，不能归咎旧reason再回流或事实未到场，也不足以认定纯随机波动。max_context_tokens_est=82925，仅作本次成本观察，不从跨次差异推因果。

最终MD第27/46行把分布写成“CPU时间”并排除死循环/过载；第29/46行从双峰推反复进出内核；第40行肯定注册/IPC机制。MD、HTML可见正文、primary/principal同错；第50行和原生表的正确边界不能抵销这些断言。原件SHA：MD`fda07cac7015eecd1018445e32b430dbc24f3ccec65f7ca863ce6ec63b9f57aa`，HTML`4b467336a50f68a23c68bca9ca76c6658ed94c934b38a7640e9ea01f0d914a9e`。这是内容审计，没有浏览器像素验收。

两份164/259质量块来自不同查询：首份perf_stats带pattern，第二份自动window_stats无pattern且按精确线程选择。不能以相同数值删除人口身份或合并分母。元数据降噪/事实与解释一致性仍归16.4；18.3原有限sample-only诚实解释出口未通过，不增任意格式/平台/图作为新退出门。

## B：真实最终说明、原件与恢复终态

原件目录：[B运行](results/hmc_final_original_native_handoff_20261018/native-registration-natural/)。独立报告归档于本批`verification-receipts-v1/codrax-hmc261-registration-manual-audit-v1.md`，SHA`8b8b9cceaff98d2e67d25afb1516e17153a23c658e3f561aab959783f851b56c`。

真实父controller/planner与独立恢复子controller均调用模型；测试由公开run_tests执行。operator准备初始修复、合同、Git和历史运行，不宣称自然完成最初源码修复或从零CLI。

父`history-report-artifacts`、子`restart-child-delivery-artifacts`各7件：3个精确ChangeReport原件、2个current alias、2个真实WriteFinalReport。两份manifest均完整无issues，逐件字节/SHA/类型/计划核对一致；源临时目录清理后原字节仍留存。两个final各自的`verification.report_id`、`artifacts.report_path`及owning VerifyRef准确：旧source绑定`12723c…`代，本轮登记绑定`b5e394…`代，不因mtime或同PlanID取最新成功。

旧source final保留`in_progress/weak`，当前final为`complete/verified`。中间attempt仍`unverified/verification_proof_incomplete`，后续补证只改变其batch终态关系，没有涂改历史attempt。旧名`final-report.json`仍是兼容ChangeReport，不作为最终说明；实际证据取`.final.json`。

三种计数分别核对，不能相加或互换：

| 量尺 | 实际结果 |
|---|---|
| 当前验证报告/最终说明 | 1条断言结果，passed1/failed0；2条命令（语法检查＋unittest） |
| 累计证明 | 2份贡献报告、2条断言结果、4条命令；不是2个不同测试 |
| 原生独立执行 | operator准备1次＋正式工作流3次；父子14份材料不是14次执行 |

恢复前登记batch为verifying且无VerifyRef/verify attempt；子进程真实新执行后才finish/all_verified。无changes/verification_probes；源码、测试、配置和依赖未改，子Git diff/status为空、HEAD一致，断言和源码文件摘要与新执行绑定。6个上下文包重存无引用累积。初始fixture缺完整apply checkpoint等边界仍诚实披露，没有借只读验证伪造源码交付。

本次闭环18.4“最终说明原件留存及精确链接”子缺口；18.4全领域矩阵、18.5轮换不关闭。不宣称网页下载/点击、完整CLI或多框架全验。四次过早accept_unverified倾向均被确定性控制层挡住，记为教学/路由效率观察，不无限阻塞已具事实凭证的交付。

## 本批交付口径

完整能力新增0；生产子能力1组（单一当前解释交接），验收保障子能力1组（真实最终说明原件/精确链接/父子失败留存）。本批剩余人工验收缺陷1组：采样解释与事实边界一致性，含不同查询人口的展示降噪，归既有16.4/18.3/18.4，不重复新增编号。

下一能力轨优先17.6可公开复现的格式接入接缝，旧采样失败保原，不加第三例或关键词硬门追绿。全仓/发布及完整原证索引见[批次汇总](parallel_selected_summary_hmc_final_original_native_handoff_20261018.md)与统一账本§261。
