# HMC315：调查引用状态一致性与只读登记负向验收

本批在 `main`，基线 `f5ed02252e46b6b4a39965817cc7957f078d9260`。目录使用本地日期2026-10-29，正式运行终态为2026-10-30 UTC；不回填或重命名原收据。关联[统一账本§315](../docs/design/hmosperf_capability_gap_audit_20260917.md)、[任务队列](../docs/design/hmosperf_implementation_tasks_20260917.md)和[人工审计](parallel_selected_summary_hmc_temporal_registration_20261029_manual_audit.md)。

## 核账与交付口径

根与独立代理按稳定ID复核：**79唯一＝26已交付＋53开放**，18类；18部分／29待实施／3待验收／2验收中／1持续执行。18类各自开放数依次为2、1、1、5、2、2、4、6、3、2、1、5、3、5、3、2、2、4。

- 完整能力新增：**0**；既有负向验收轮换不重复计数。
- 有界子能力：**1组**，Extract/Finalize原生引用状态一致及逐单元边界；以公开回归、全仓及同代发布为实现交付证据，不等同自然成文已通过。
- 真实评测：恰好2例、各1次；A机器FAIL/完整人工FAIL，B原测试PASS/限定负向人工PASS。失败和新分支未触发不销账，53开放不变。

## 参考实现与修复

对照 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/llm_contract.py` 与 `core/batch/closure/merge.py`：前者以明确簇/代表/字段核来源适用域，后者在校验成功后才合并。借鉴的是**同一事实身份在消费者处复验、来源权限与解释内容分开**，不移植自由正文数字扫描、近似数值容差或重复文本拒绝为本项目硬门。

公开反例确认：Extract预览固定按未验证引用处理，Finalize却使用已有私有source/query/table/row检查；同一当前受理交接可得到相互冲突的地址状态。本批使Extract复用同一检查，不因尚无成文发布收据就否定地址当前性。

共享预览逐单元区分 `current_exact_rows`、`unverified_address`、`no_native_row_address`。无引用的业务线索、合法计算、阴性分析仍完整保留，不能借其它单元的引用升权。状态和原JSON/边界一起占原整单元预算，保旧前缀、七单元容量与零/一/差一/精确预算行为。JSON恢复及真实源变代仍不能恢复私有权限。不从support_refs、正文数字、邻近事件或业务名猜绑定。

**地址当前不验证正文、算术、关系或图注，也不证明行已实际发布。** 旧HMC303允许未绑定解释作为advisory的合同原样保留；本次不删除旧错误文本来伪造绿色。Trace因果投影、自动补齐、原4排序/3实体原因、31ms闭合睡眠、35ms请求驻留、合法2ms及47ms背景合同未修改。

修改仅两个生产文件与一个新增公开测试文件：`extractor_native_fact_handoff.go`、`investigation_interpretation_preview.go`、`interpretation_reference_status_hmc315_public_test.go`，均在 `internal/agent/`。

## 公开回归与唯一全仓

各原始日志位于 `/private/tmp/codrax-hmc315-` 前缀，并按源身份封存；不能把测试过程中早期失败抹掉或把后来加入的断言倒签到早期GREEN。

| 原日志后缀 | 实际结果 | 范围 |
|---|---|---|
| interpretation-red-v1.log | exit1；2顶层FAIL、1子例FAIL/1子例PASS | 原公开冲突；Finalize原路径已通过 |
| interpretation-targeted-v1.log | exit0；12顶层/36子例PASS | 早期GREEN |
| interpretation-targeted-v2.log | exit0；14/41 PASS | 恢复旧前缀，补源变代、JSON与原子预算 |
| interpretation-related-v1.log | exit0；75/259 PASS，3包PASS | 相邻合同；1个既有真实PCAP opt-in SKIP |
| interpretation-race-v1.log | exit0；35/106 PASS，3包PASS | 相邻race；同一既有SKIP |
| graph-guards-v1.log | exit0；3顶层、2包PASS | 原HMC299等待身份、HMC314重选、HMC231业务注释顺序 |
| full-v1.log | exit0；98包PASS/12无测试；28542顶层/33834子例PASS | 43顶层/14子例SKIP，零FAIL/非JSON/重复终态/未结束 |

全仓仅一次 `go test -json ./... -count=1 -timeout=30m`，parent正常等待原PID57207，于 `2026-10-30T01:39:44.945338Z` 结束；不是恢复推断收据。262677条原始JSON，SHA256 `81ae48ff2fb9b5a244a6f8fd2ff339032c78706c056b2785b4b5a998044db3e9`。HMC315新增4顶层/7子例、旧HMC303/HMC280和原4排序/闭合睡眠保护实际通过；未放宽断言或冻后补测。

## 冻结与两例真实运行

编译唯一标准CLI与唯一原测试二进制，不另造假CLI；测试编译不是额外模型例。CLI SHA256 `0c4718f963e701e2d636c231ba6211565998ad0244b311b0f46ff85a6346a2f3`，testbin SHA256 `1fc29cff1c6c6f31bbe8c75726d050684754262cc7e6e4f6813ffe5c3339371d`。两次构建实际exit0、源清单一致。13个helper/计划/命令文件在冻结前逐个审读，selftest通过；不读取/hash/复制providers内容。

冻结 **7510仓库输入／124外部输入／16仓外快照**，SHA256 `30a8faf08f0352277c7237f3ea2ba6c27c6ef6436587e187ff66e4228bacc94d`。之后源码、测试、注释、helper停写，仅补文档和审计报告。Go快照在仓外且以 `.go.txt` 保存，不回灌旧源码污染构建。

| 侧 | 原入口与限定范围 | 正式终态 | 独立结果 |
|---|---|---|---|
| A | 原 `trace_business_io_sequence.case`，原题/构造ftrace材料，15步，n=1，标准CLI | 01:41:13.169235Z，exit0 | 原判据 `FAIL degraded_answer_checks_skipped:1`；完整人工FAIL |
| B | 原 `TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，原问题；真实planner，scripted verify/finish controller，新进程公共RunTests | 01:28:47.990598Z，exit0 | 原测试PASS；限定负向人工PASS；不是full CLI/自然controller |

真实请求日志：A31请求/31响应，B2/2；精确可配对A18/B2。A有1个已闭合歧义组，不能为其中13请求推算逐请求时长；无待响应事件。最大可证请求交集 **8.441秒**，来自歧义前实际日志，不是barrier或进程重叠推断。A日志存在非UTF8原字节，完整保留；报告摘录不替代原件。

### A：事实保留，不把恢复稿或未触发当通过

最终MD与HTML源码保原生两线程状态表、Trace投影和自动补充；完成1.040000与唤醒1.040010已分别表达，31/35、50/52、合法2ms及47背景保留。图最终未通过核验，被恢复车道隔离；没有Mermaid交付，旁路不能当正常已验证答案。正文仍有“50ms中运行5ms，其余44ms全部睡眠”漏1ms就绪；当前INIT的advisory也仍有业务起点/阻塞解释冲突，正确表不抵消错误正文。

实际先提交过typed native_refs：第一次拒绝，修正后受理；之后3个legacy完成也受理，既有直接完成/fork合入合同都会退役旧handoff。最终3份INIT不含新 `native_reference_status`，因此本批新显示分支自然覆盖 **NOT_TRIGGERED**；原地址校验确已触发，不是从未调用、全部拒绝或新消费者已经自然通过。没有逐fork私有状态快照，不能冒称已定位唯一对象的退役时刻。不能直接删除h=nil退役规则来保留所有旧解释。

一次完整草稿、5次patch：前三次主体身份不符未staged，第四次增加未列入局部权限的边未staged，第五次merged已staged但仍未通过最终图合同。模型有按opaque引用顺序猜主体的证据；精确映射在另一处typed delta，尚不能断言所有合法修补路线都失败。首次成文日志估计148151/200000 tokens（不是真实计费token），最后prompt732076/800000 bytes触发91.5%保护；不以“大上下文”单独证明语义错误原因。

### B：补证通过不替来源倒签

来源具有真实apply坐标但无controller verify/Completion。新进程运行保留的原断言，proof verified，整体仍 `unverified / missing_terminal_verify_verdict`。原件与最终WriteFinalReport逐面审计，当前与历史独立计数：旧1结果/2命令/0登记凭证；当前1/2/1，新的invocation与旧不同。

父/子索引各4原件/1attempt/1link，8份归档副本重算bytes/SHA一致。真正 `.final.json` 的report_id和交付路径绑定本轮内容地址报告；旧报告保留，兼容 `final-report.json` 不冒充WriteFinalReport。源码/测试、HEAD、diff/status只读检查通过；不把执行报告写入说成文件系统零写入。context-pack持久化、自然controller等未覆盖；B也未触发本批解释状态分支。

B辅助参数索引仍为空，但原日志完整保留2次read_file与1次emit_change_plan，不能误报零调用；这是旧§310评测采集欠项，继续归18.4。

## 剩余与下一ROI

按旧ID保留，不制造新稳定任务：

1. **16.4/18.2**：多生产者完成与当前原生交接生命周期公开复现；保合法新完成退役旧权限，查精准同源事实如何进入成文。先验明确系统缺口，再处理已足证据的自由文字误述，不造关键词硬门。
2. **12.5/18.2/18.4**：主体引用与精确身份/关系修补delta的同源导航，以及局部修补共享预算。先保留本批原错误参数和staged状态，对偶测试再定性；新显式重选、当前引用状态自然覆盖继续欠账。
3. 完整能力槽轮换**11.2代表选择**正确view/规格，或**17.6**有新增真实格式/工具/宿主材料的二进制验收；B既有负控不重复包装为新能力。旧采样、IO、权限覆盖及§310扩大race欠项继续保留。

WAL零副作用未满足前生产NO-GO。冻结后不追改、不加第三例、不重跑全仓，旧失败不回签。

## 原件和发布

结果根：`eval/results/hmc_temporal_registration_20261029_v1`。原题/材料/oracle、实际INIT、原生查询、提交/修补、MD/HTML源码、旁路、进程收据、失败与独立A/B/完整性/根复核报告按真实源路径＋SHA封存；HTML本批为源码审计，不声称浏览器像素QA。实际trace_query为16次且均ok=true，14份是独立payload，不能混算；A97/B29个明确source-inventory原件全部hash一致。A13个截断locator与1个缺memory路径保留未解析，不猜补或声称全旁路已恢复。

旧16档4949件/7025591024字节及359快照来源身份保全；18历史污染身份属于359子集，过期tmp不恢复进仓。本批封存327件，索引SHA256 `e82fe168530ebd0b2fc158f1bbafafaf061edaf574d6bdd6f9b0c2e1686d841c`，见该根 `sealed-receipt-v1.json`。代码提交 `11c38b1fcc739658322e7494ef2cf23a71707f5d`（3文件）；相邻五文档提交、普通main推送、工作区/提交blob/冻结输入逐字节一致性见 `publication-v1.json`。发布完整性不是人工验收通过。
