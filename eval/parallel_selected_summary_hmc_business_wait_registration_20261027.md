# HMC300：闭合睡眠投影与只读登记轮换

日期：2026-10-27。状态：唯一全仓FAIL、两例完整人工1/2，候选保存，不进main。

## 对账与交付范围

- 基线 `dd9bf170ac9dd1dd9649b5e3a69e356b2df0df41`，候选 `codex/hmc299-wait-interval-candidate`；main仍为 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。
- 79唯一稳定任务＝25已交付＋54开放，18类；17部分实施、31待实施、3待验收、2验收中、1持续执行，无删号/重复销账。
- 完整能力新增0；共享修复2组：跨查询投影实例保真、精确结构分类/零起点展示；另修归档发现污染。不将其计为3个完整能力。
- 原HMC299全仓FAIL、采样正文FAIL与未触发保原。本批不再追同一道采样题求绿；只读登记有限正向与更宽负向能力分开。

## 已证反例与实现边界

原HMC232断言不改即RED。4个真实查询及ledger均有worker闭合睡眠 `[1.009010,1.040010)`31ms及tail，旧generic/V4等值＋主体＋重叠折叠吞闭合实例；完整payload改变ID排序只是暴露此不稳定。初修后R2再把宽/闭合/tail相加为67.5ms，保留FAIL后修共享成员边界。

现以有限、已验证、源绑定的native method/target/window/line filters及已知capture/clock差异作负向反证。generic/R1/V4、R2及展示端共用；不从PartitionID、文本、whole DTO或unknown造新权限。既有complete-state-cover优先，IO精确身份和合法不重叠汇总不变，absorb保全部真实来源，三成员核全部已收成员。原生查询/因果资格/排序选因/自动补齐未改。参考仓的线程实例、状态段裁切、member-field回查设计用于来源保真，未移植最近邻或容差猜因。

结构严格迁移外层唯一新增字段pin，嵌套DTO另pin；真实decoder改登记soft_consumer，无扫描豁免。公开零起点TraceQuery→tracediag RED→GREEN，精确DTO复用原formatter和行帽，无效可选展示不撤原事件，不改反射器全局规则。

旧209封存不变；18旧快照恢复性移到仓外，保映射，原`.go`影子发现消除不靠屏蔽测试。新21快照仓外、封存源码`.go.txt`，历史失败不倒签。

## 冻结前确定性验证

| 范围 | 顶层PASS / 子例PASS | 结果 |
|---|---:|---|
| projection相关 / race（各） | 231 / 210 | PASS |
| 展示相关 / race（各） | 36 / 155 | PASS |
| 完整tracediag包 | 174 / 81 | PASS |
| 结构相关race，2包 | 18 / 7 | PASS |
| Trace/自动补齐/authority/活跃流/跨源保护，3包 | 297 / 64 | PASS |
| 只读登记确定性正反控，1包 | 5 / 2 | PASS |
| 标准make；原B测试二进制构建 | — | exit0 |

所有命令/PID/UTC起止/exit/log SHA在同前缀原始receipt，前缀 `/private/tmp/codrax-hmc300-`。projection真实基线和R2失败保留；`sleep-green-v1`虽名称含green，实际exit1，未倒签。新增SupportRefs fixture错误及展示known+nil期待错误分别记测试预期错误，只改新测试，不削弱旧断言。第一次生成batch的语法失败在任何执行前修复，原记录保留；没有模型或全仓预跑。

构建SHA：CLI `f72652819c9f84791bf543da855cc7e509dbd0cdf01fa9c11af82168210d37d3`；登记测试二进制 `78e55000f2e9b9dcf24e61e1609417c8bdbe7bffdd8ad4e77014ac938522a82e`。生产文件最后写入早于构建；后续仅新增展示测试。冻结包含最终测试版本。

## 冻结与恰好两例

冻结文件 `/private/tmp/codrax-hmc300-frozen-v1.json`，SHA `be705d5eb564b0518ec766446976d694ecd2de2cfd15a6d716bec496b13ba94a`，7419仓库、27外部、21快照；全员ACK停源码/测试/注释/helper写入。提供者密钥文件不读/hash/复制，只向现有真实调用传配置路径。

| 例 | 原问题/合同 | 材料与范围 |
|---|---|---|
| A | 原 `trace_business_io_sequence.case`、15步，OpenDocument响应慢在哪里及线程/存储唤醒时序 | 仓库作者构造的20行ftrace，真实CLI/模型/工具；非实机采集 |
| B | 原 `TestNativeRegistrationLiveNaturalControllerTerminal` | 真实parent/controller/planner＋独立child恢复；源码/历史测试准备与read exploration由operator/脚本，非全CLI/自然源码创作 |

A须核50ms业务窗、44ms主线程S、31msworker闭合S、35ms请求驻留、各1ms就绪和背景47ms独立；不得请求驻留改称设备驻留、相加互相重叠的量尺或将背景升根因。B须核新invocation、readonly、原件/验证/最终报告引用、当前1断言/2命令与历史累计分开。未自然触发的修复分支独立NOT_TRIGGERED，不用离线补查代签。

## 真实终态与覆盖

| 执行 | UTC开始—结束 | 实际结果 | 日志SHA256 |
|---|---|---|---|
| A，外层PID58892 / CLI58945 | 07:57:46.750704—08:02:07.648568 | exit0、机器PASS；完整人工FAIL | `f2ae020eff4251c033461636e75943aca660808bffec69d0961848588a5216ae` |
| B，父58893 / child66466 | 07:57:46.752300—07:59:34.070715 | exit0、机器PASS；有限原合同人工PASS | `e843a032f265965547d5bb2d2ca5e13bb570425b1bdca8be5e222b7328ba9bb1` |
| 唯一全仓，PID58870 | 07:57:46.469349—08:12:42.007692 | exit1；96包PASS/1FAIL/12无测试 | `839c60687b3ba0dc5dfdb78bfa1e9a11db80bc6ffeb57cf4e85fd841806889d3` |

全仓28366顶层/33246子例PASS，2顶层/10子例FAIL，41顶层/14子例SKIP；251431行均JSON，无重复终态或未结束项。实际命令`go test -json ./... -count=1 -timeout=30m`，未二次全仓。A/B及全仓receipt的source_verified_before/after均true，冻结输入无改动。真实模型请求重叠至少4.737秒，另有B恢复子与A请求完整交叠，不仅凭进程并行推定。

本批完整能力新增0/累计25，共享修复2组仍在候选，主分支已交付子能力新增0；本批剩余4组验收缺陷，另记未触发范围，54项不减：

1. **公共根因候选回归**：`TestSelfRunningScopePublicNativeWindows`两语言5类共10子例，混入1个宽窗时原4项旁路变5项、2个宽窗变6项；exact-only通过。失败发生于发布前真实候选编译/绑定，后面的发布断言未执行，不称全链路通过。共享保留行进入RankedSeats，再以EvidenceID/board/rank生成独立候选的静态链已核；日志未dump具体新增候选，不能断言唯一触发挂点或具体thread/token。原4项断言不改，下一批分离量测保真与物理原因/选定board候选资格，不能倒退吞闭合睡眠。
2. **统一发射覆盖缺口**：`TestTraceNoteKeysEmittedSubsetOfRegistry`未覆盖新soft_consumer `wakeup_measured_wait`。这是分类迁移后严格fixture覆盖未齐，不是外层schema hash失败；下批用真实producer补覆盖，不将字段改回display_only豁免。原HMC232、外层/嵌套schema、carrier及归档P3失败本轮已通过；不回签原§299全仓。
3. **图主体修补交接冲突**：A最初3个英文技术节点及原生wake anchors，后经边界/可见性修补成为6个节点。原节点加业务label的复用尝试被拒，随后中文新ID声明获受理；不是renderer无模型输入自发造图。先公开复现现有typed角色/原生lane/节点映射再修，不做名称相似性硬门或一律强制中文ID。
4. **正文同源事实误述**：A把1.044500业务结束等同1.045000唤醒、5.5ms差写5ms、1.040000完成与1.040010唤醒合称，并用全窗idle推局部竞争机制；这些正确原生值已实际到场，不能当缺字段重写统计器或称已证偶发。

A实际11次原生查询、16项Finalizer前工具结果、6份内容JSON分开；新增等待载体自然触发，31/44ms闭合区间、35ms请求驻留、tail及背景主边界有限正确。没有PID200 rank/wakeup，原宽窄impact折叠反例自然NOT_TRIGGERED。Finalizer最高估计161905 tokens，不是provider账单；上游4条interpretation_units未证进入实际INIT，不能臆称摘要污染。正文/9个HTML表/1个Mermaid源及root-causes已审；浏览器缺可执行导致像素QA未完成，失败诊断保留，无下载/重试。

B真实模型9请求（父5/规划器2/子2），完整读取后登记changes=[]，新invocation验证而非复用历史passed；当前1断言/2命令、累计2报告/2结果/4命令及3批预算3/5分开。父子14/14原件拷贝对应7材料，各3验证引用/2final链接准确，五快照6逻辑pack内容一致、后四持久化引用稳定。source localization weak/apply_checkpoint_missing和自然负向未触发继续留账，不签完整CLI或父能力交付。

## 封存与候选发布

只在同代冻结原件核验后封存，不覆写失败、不追跑第三例或第二次全仓。源码、测试、注释和原执行helper冻结后未变；审计阶段另新建两份只读提取/HTML检查脚本，不参与构建、查询、模型或全仓，单独随审计原件封存，不能宣称它们已包含在原冻结清单。它们也未修写原答案。代码/测试与文档分两笔，验收失败不进main。

原件根：`eval/results/hmc_business_wait_registration_20261027_v1`。封存索引按canonical原路径＋SHA去重，保不同来源；旧209件索引SHA `183d7b4c9cfbabf4f49902f724367e9e4da3cbe080c7046737e07fe7a1699174`，旧18快照迁移receipt SHA `4511d1b3a5b398389dda99b04e8183ba72c7a4f6c50a4587a817e6dbf42e5cea`。

实际封存232个独立原件版本，索引SHA `0a2ab17ece5c9fa22044ad57756a3c393244b6900b5b0c7601fd57bb9b41e9ad`，封存后逐项复验不等于内容通过。代码/测试候选提交 `816139ef4c147e5817b485d9791d11a4f4ad3a15`（13文件），5份文档另笔；只向既有codex候选分支普通推送，禁止main/force。最终 `eval/results/hmc_business_wait_registration_20261027_v1/publication-v1.json` 记录两笔实际SHA、提交树与7419冻结源对应、远端/clean、232原件及原0/0/1进程退出码；该收据只签候选保存完整性，不签验收通过。

详细人工结论见[本批人工审计](parallel_selected_summary_hmc_business_wait_registration_20261027_manual_audit.md)。原始审计 `/private/tmp/codrax-hmc300-A-audit-v1.md`、`B-audit-v1.md`、`A-projection-audit-v1.md`及`full-regression-review-v1.md`统一入封存。旧采样、IO、栈关系、代表/权限覆盖债均在统一账本；WAL零副作用未达仍生产NO-GO。
