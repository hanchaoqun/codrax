# HMC265：命名材料查询入口与来源/执行状态分层

2026-10-19；基线`2a0ac2787`。79唯一＝24交付＋55开放、重复0。本批完整能力新增0、已交付子能力2组，剩余验收2组含于55；自然ZIP完整入口和旧采样解释均不销账。代码`5820b6f35be62082e4016af79dc7c51255d5e4bb`已普通推送，文档另笔结算。

## 实现与参考

参考仓`server.py::get_hitrace_path`的候选发现及`core/hiperf_converter.py`共享真实准备，借鉴发现/准备/执行分层，不复制扩展名授权、首成员选择或mtime判成功。

- 安全可选导航与强制准备/源码解释义务正交；通用Explorer复用当前受限工具面，mixed仍须源码证据，source-only坏fixture不强制解码。实际查询仍核完整成员、CRC和代次。
- 共享成文分开来源存在、原件读取、原生结果、源码支持和目标事实；观察记录不冒充调用数。只对已产事实生成卡，未知不补零；合法日志/MCP和真零保留。Extractor/Finalizer共用whole-record历史操作JSON，12行/16KiB，省略有数；工具不可见/目录范围拒绝不等于文件不可读。

不新增decoder或硬门，不扫描用户/答案关键词，不改Trace因果资格/链上业务/显式窗和自动补齐。

## 两例各一次，并行

| 例 | 范围 | 机器 | 独立人工 |
| --- | --- | --- | --- |
| A | 正常命名ZIP指定成员、真实CLI热点/调用栈问题，无预选附件 | FAIL | FAIL：选择缺口误升终态，正确成员也被阻断，0成功查询 |
| B | 自然只读登记、真实父/planner/独立恢复子 | PASS | 限定PASS：实际新执行、精确版本/计数和最终说明一致 |

A PID18964，UTC09:39:21.156521–09:42:42.435968，wrapper exit0；B PID18952，UTC09:39:21.066276–09:41:35.982884，exit0。区间真实重叠，没有第三例。B有operator初始实现/旧结果，不能扩签从零完整CLI。

A已命中artifact/optional量测分类、2成员完整导航与可见trace_query；10次调用＝2schema拒绝＋1多候选误升SourceUnavailable＋7终止门拒绝，后续正确archive_member仍未执行。最终“需要格式转换”没有decoder/CRC/权限失败证据。实际成文不再有combined_ready或空等待卡，但模型negative_observation仍遇到通用verified-zero教学冲突，继续留账。自动补齐未到达；旧采样unknown/CPU/权重/完整栈问题未覆盖，不按缺数据时未误述销账。

输入为同一参考PERFILE2真实原件的两个人工ZIP成员，不是两份采集或实机ZIP。原件30062585B/SHA`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；独立oracle12000全量、1393窗、164目标、259权重、CPU0/1为82/82条和85/174权重，未注入模型。本轮A均未取得这些数据。

B当前1断言/2命令，累计2贡献报告/2结果/4命令；operator1＋流程3实际invocation，工件副本不算执行。父子各7原件、5canonical、3verify refs、2精确final链接复核匹配。旧未验证尝试和旧in_progress说明保原，当前complete绑定新报告；只读HEAD/源码/测试/clean及6逻辑包不增殖均通过。source-localization weak和apply-checkpoint unavailable仍披露。

## 冻结与验证

v1 UTC09:39:15.417141冻结7136输入，SHA`62ed39fcb6abb48f62a1fe627dbde8d7ee82dcec4f5c594b03214ced457c5336`。实际CLI SHA`71a25a29b88a5c8c5cf49be76b76bc43541e29e32abcb7d7cdce7f6761eef716`；B测试binary SHA`503fe89db8654c805baebb46c76d85ac480d806db25f2d11fcb39e7fe2c8f4e6`。

入口末版race58根/194含子、readiness相邻race46根/85含子、shared-handoff三包相邻race均正式通过；窗口/链上IO与业务/L1等保护另过。真实classifier parser→正常Run→Explorer→查询的三策略接线及正常accepted-completion真实补齐通过；补齐使用成功成员、0.95–1.05秒显式窗，不冒称BusinessFocus测试或自然模型PASS。

首轮全仓旧测试要求marker-only也生成量测卡，与本次无空卡合同冲突。只改`answer_document_reader_field_scope_public_test.go`，保所有原字段/身份/来源/因果断言，新增真实事实卡正控；RED/GREEN与相邻race13根/35含子通过。v2 UTC09:49:01.893441冻结仍7136输入，仅此测试不同，SHA`8906e9c5d0e80e1e27e122b96cc34fa09f791003eb824befc24bb9f9d9d780bc`。生产、实际模型binary、问题/原件不变；原全仓FAIL保留，模型没有重跑。

首轮全仓PID18953，UTC09:39:21.066271–09:54:13.954148正式exit1：95包PASS/13无测试包SKIP/1FAIL，只有上述旧测试根及4子例失败。原JSONL SHA`62961be13698fd495a4710545d64b07423375c8ded62f4ecffbca519d09786ed`保留。

末版全仓v2 PID3681，UTC09:54:26.818112–10:13:25.514744正式exit0：96包PASS/13无测试包SKIP、零失败/非JSON行，17个HMC265新根同次PASS。日志SHA`9ce1b63f889eb81da797966a10f09aa353e421e6d2de2b70733dd44f8d194f13`。只修一旧测试后再验，不冒称一次全仓成功或重新跑过模型。

UTC10:13:49.487142封存54份既有原件不变、123份逐字节复制（86过程凭证、实际CLI/ZIP、35实际A最终/工具blob/read-run）。177文件SHA索引`9fdc4cf927fb642ceabfce7bb9fe2ed9d468a514d8833f028be46d2e5c3c8e8c`；归档收据SHA`44afacd106d3f746030a515164f8d0c92ee007ac74ca2b3360a546cfe580f80d`。不读/复制provider凭据，不改旧失败。代码push UTC10:14:10.744134–10:14:16.834735正式exit0；最终代码/文档提交及远端clean凭证单列原件根`publication-receipts-v1/publication-v1.json`，不改索引。

## 下一优先级

1. 共享准备错误分型：待选成员可修复，真实坏源/CRC/代次/取消仍保持终止语义；公开Run覆盖先歧义后正确选择，不能只测直调工具。
2. 模型“未取得”与原生“测得零”的证据/教学分层，保合法日志/MCP；旧采样解释继续留账，不追单句求绿。
3. 17.6更多实机格式/外部TS/原生平台按真实材料可得性推进。

原件根`eval/results/hmc_named_query_obligation_20261019`；见[人工审计](parallel_selected_summary_hmc_named_query_obligation_20261019_manual_audit.md)及[统一任务账](../docs/design/hmosperf_implementation_tasks_20260917.md)。
