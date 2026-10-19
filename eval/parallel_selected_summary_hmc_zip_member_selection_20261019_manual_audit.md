# HMC263双例人工审计

## 判定与限制

两例各一次并行、同一冻结；机器1/2，限定人工1/2。A无附件普通ZIP自然入口FAIL；B既有测试只读登记、独立恢复、新执行和历史原件/最终说明的有限合同PASS。完整能力新增0，55开放不变；旧采样执行状态/权重耗时错误未被本次覆盖，不销账。主控复读实际A终稿、原调用日志与B执行报告/状态/独立审计，不以脚本exit0代替验收。

## A：没有取到指定成员的原生事实

原件目录：`results/hmc_zip_member_selection_20261019/trace_zip_selected_member-20261019-002123`。自然问题只有ZIP路径、成员、时间窗、线程及热点/调用栈需求，没有附加系统护栏、`--htrace`或操作者预选。

正式runner PID42835，UTC07:21:23.625568–07:25:26.568815，exit0；eval判卷实际FAIL（`no_successful_tool_result:trace_query missing_primary:OnDataEvent_`）。日志SHA`f24941758df1826e7954d276d9f2957924c57a9dbefef9beb59afb22d46da077`。

已确认系统断点：运输探测仅给未知域容器，未把已知二进制候选交接到typed trace入口；路由理由写artifact/optional，实际有效模式却是repo/required并注入source_explanation。探索没有trace_query可用面，5次尝试全unavailable，目录发现因仓外边界拒绝；反复读取/grep二进制并不能替代完整准备。最后仅说明无法取得目标数据，没有成功原生查询、热点表或栈帧结果。保留目录权限边界本身是正确行为，修复应限定用户命名原件，不放开任意外部目录。

所以本例不能验收本批精确成员API的自然贯通，也未命中relation_path原生教学（实际分析为member_set且没有原生事实）。旧unknown→执行系统调用错误未复现不是修复证据；图/原生统计未出现亦不能签覆盖。离线真实原件公开查询PASS和此次自然入口FAIL分别保留。

独立A报告SHA`8c6b8fac7f30aa5bf2ea7a9d1cd573175bfe322ff0f0b8c84b15641d99178fe8`；实际MD SHA`0b18282c45765e7c71ace22a583191dc8dd148acacbbdd5b13eda0d642ec8d75`。原件清单含91个工具blob和MD/HTML/answer-surfaces/read_run共95件，清单SHA`4e07d041ae2268c11d5e35d01599e92fe97bf2a4849ef619e00ada2373c40f98`；它们不是成功查询产物或95次执行。正式ZIP另保存原字节，不能把operator包装/展开目录称为应用准备成功。

## B：本轮与历史状态、原件和计数一致

目录：`results/hmc_zip_member_selection_20261019/native-registration-natural`。真实parent/controller→planner→fresh child/controller，操作者仅准备原实现/契约/历史结果；不是完整CLI或从零修改验收。PID42950，恢复子51778；UTC07:21:24.694367–07:23:15.792537，exit0，日志SHA`6ae4b6aa999087ba719979586cb1476fdb75be5dce42c131aba95c84c9833981`。

父控制器两次过早finish被控制层转为证明补充，planner实际读取源码与现有测试后发零文件变更登记。恢复子重新执行原生测试后才complete/verified。旧proof-repair尝试保持unverified；其后由已验证followup覆盖当前完成义务，不重写历史记录。

| 核对项 | 结果 |
| --- | --- |
| 只读 | 源码/测试SHA、HEAD一致，整个夹具git status/diff为空 |
| 本轮身份 | 新invocation与操作者旧执行、父source验证及未验证修补尝试全部不同 |
| 当前计数 | 1 assertion / 1 passed / 2 commands（语法预检＋现有测试执行） |
| 累计计数 | 2贡献报告 / 2断言结果 / 4命令，不计兼容副本或归档文件数 |
| 原件 | 父/子各7份文件，共14次字节SHA及10次canonical报告SHA复算一致 |
| 最终链接 | 两侧各2条真正WriteFinalReport→精确版本报告，共4条一致 |
| 恢复上下文 | 五份快照均6逻辑包，内容不增殖；持久化往返稳定，不冒称内存到落盘所有物理ID字面相同 |

根目录`final-report.json`是ChangeReport快照，真正完成说明在`originals/*.final.json`。旧说明保in_progress/weak与自有`63c9…report`，新说明complete/verified指向`cdb9…report`；中间`042f…`原件仍保未验证。保留source-localization weak、apply-checkpoint unavailable，不扩签源码定位/应用凭证能力。

独立B报告`codrax-hmc263-registration-manual-audit-v1.md` SHA`536f12903fb4c6422c859b64a3576d40332dafb0b00d61da2c48bb52cdc6a57d`。完整独立报告与实际终稿/状态/原日志已在verification-receipts-v1封存；227文件索引SHA`114719ff58f5a5fec617567c6203bae41a1ca78c45bd5a8aea5cfb63d8c5336b`。末版全仓只同步一个旧schema测试清单和正反例，生产与实际评测binary未变，未追加模型重跑；全仓通过不改变本页人工判定。
