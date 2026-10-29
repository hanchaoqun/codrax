# HMC314：精确图重选、代表执行交接及请求关联

2026-10-29（UTC批次日），工作分支 **main**，基线 `c4b4de6ad56356c0eaed0d809bc3da2479652a0e`。此前候选按用户明确指令快进合入；本批不再留候选工作。历史失败不回签，产品自身write模式的隔离与审批不变。

结论：**图旧绑定公开回归已修复，逐成员规格/尝试交接已自然到场，唯一全仓通过；两例完整人工仍未通过，53项继续开放。** 未放宽原断言或退出合同，未追跑第三例。

## 1. 任务与交付数

| 口径 | 结果 |
| --- | --- |
| 稳定任务 | 79唯一＝26已交付＋53开放，18类 |
| 开放状态 | 18部分实施、29待实施、3待验收、2验收中、1持续 |
| 本批完整能力交付 | 0；11.2代表选择原有限合同未满足 |
| 产品有界子能力 | 2组：精确显式图重选；代表成员完整规格/实际尝试导航 |
| 评测基础修复 | 1项：保守请求关联统一接入；解析器随归并WIP已在main保存，本批完成集成验收 |
| 剩余验收 | 2产品原因组＋新重选自然覆盖缺口，归原ID，不新增3个任务 |
| 完整人工 | 0/2；确定性回归或正确原生表不能代签完整答案 |

18类开放数依次为2、1、1、5、2、2、4、6、3、2、1、5、3、5、3、2、2、4。根及独立代理逐复选框核对，不沿用口头数字。见[统一任务清单](../docs/design/hmosperf_implementation_tasks_20260917.md)及[§314审计账本](../docs/design/hmosperf_capability_gap_audit_20260917.md)。

## 2. 参考设计与泛化修复

只读对照 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`：`core/batch/closure/select.py:14–25`解决明确manifest内的选择，不能替代自然目标到查询类型的分类；`core/llm_contract.py:61–177`核成员/字段适用域，`closure/merge.py:126`先校验再回写。复用“完整身份、阶段分工、原子校验”的设计意图，不复制自由正文数字/相似文本扫描为硬门。

### 图：显式改绑定与只改显示分开

原公开反例：native selector陈旧时，合法 `ensure_visible` 改了可见标签却仍保留旧绑定。新分支仅对唯一已声明、当前绑定缺失/陈旧的主体提供可选 `native_participant` 枚举；不隐式修正相似ID。旧display-only行为保留。

当前participant_ref、块指纹、唯一声明、原selector与原生身份均精确校验；只替目标selector及作者显式label，保节点ID、其它主体、notes/messages/anchors，最后整单仍过原发射校验。孤立主体操作不接受此字段，不开放任意全块替换。schema每个相关失败只增加一个分支及有限lane枚举，不做笛卡尔展开。

### 代表导航：完整规格和真实尝试同时交接

`ExecutionView.RepresentativeNavigation()`纯投影精确同代query/catalog/artifact/revision/source/outcome/latest-attempt。选中、未选行均保完整member及 `specification`、`query_origin`、`attempt_outcomes`；完整有序attempt仍在原saved_index，不无界重放。

24KiB上下文和64KiB/16条页面按整成员预算，省略量独立披露。另一view、线程或窗口的查询成功不关闭原计划；导航不是测量、因果或完成授权。自然B的8行已真实到场，不能说新增交接未触发，但其实际view仍错，不改签成resource_stack。

### 评测：歧义不能靠清空未响应请求消失

统一复用 `eval/request_intervals.py`。只读原日志字节的真实logger事件；无request ID的重叠组直到已证归零都保持歧义，最后响应不倒签到单个请求。未知模型/非法时钟不替其它请求排空；正式spawn时区及整数微秒用于区间交集。日志未闭合、进程活跃、机器通过分别判断。13例覆盖边界；当前live没有自然歧义，不冒称自然触发了该分支。旧§313原统计及严格更正保留。

## 3. 先定向、再冻结、一次全仓

| 阶段 | 实际结果与范围 |
| --- | --- |
| 图公开RED→GREEN | 旧错误绑定不隐式改写；新显式分支从拒绝到通过；green-v2图自身4顶层/12负向子例PASS |
| 图相关/保护 | 297/495、77/103 PASS；分别保原范围，未相加为唯一用例数 |
| 图race/旧lease race | 10/19、15/40 PASS |
| 原四排序/闭合睡眠 | 5/34、3顶层PASS，原断言不变 |
| 代表公开RED→GREEN | 1顶层/24子例FAIL→PASS |
| 代表定向/相关 | 6/42、77/122 PASS |
| 代表catalog/消费者race | 56/56、62/98 PASS |
| Python关联 | 当前main正式收据13例PASS |
| 唯一全仓 | 98包PASS/12无测试；28538顶层/33827子例PASS；43/14 SKIP，零FAIL |

graph公开green-v2整次5顶层/12子例包含一个代表测试，图自身为4/12；随后新增的精确alias正反针进入最终graph race及唯一全仓，不能把较早green-v2当五个图测试的收据。早期代表新夹具触及默认10组预算、16KiB字段上限的失败保留，修正夹具前提而非放宽上限。代表开发期日志使用原Go JSON终态与实际exec结果，不伪造事后父进程收据。归并前RED/解析器回放与当前main证据分代保留。

冻结7509仓库输入、190外部输入、23仓外快照；SHA `7e094e26359cb2befe6b52e00d16fa3abc78433563660a17972b08fad88ca219`。唯一标准CLI SHA `8870593bfca899573fb63e5351cc144c023a5ae375ccfbf846e506aba1cf5f6b`。源码、测试、注释、helper冻后停写；Go快照只在仓外以.go.txt留存。provider只传配置路径，不读/hash/复制内容。

唯一命令 `go test -json ./... -count=1 -timeout=30m`，Go PID19994，父spawn前13:20:14.677023Z至正式终态13:31:41.451565Z，exit0、recovered=false、前后source_verified=true。262631行JSON，非JSON/重复终态/未结束均0；旧B1638隔离针在原log254050行PASS。不得以全仓通过覆盖自然语义失败。

## 4. 两例原自然题，各一次并行

| 项目 | A：OpenDocument时序 | B：代表选择 |
| --- | --- | --- |
| 原入口/预算 | 原case＋Trace，15步，n=1 | 普通命名captures目录，50步，n=1 |
| 材料 | 既有构造文本Trace | 两既有构造SQLite的四只读路径副本，不是四次设备实采 |
| 进程 | CLI20049，eval父20004，exit0 | CLI20005，exit0 |
| 原判据 | 机器PASS | 原Go结构auditor exit1、FAIL |
| 完整人工 | FAIL：完成/唤醒时点、图注释顺序、就绪运行误写交接等待 | FAIL：选中了thread_timeline计划，未满足resource_stack代表合同 |
| 限定改善 | 三主体两唤醒/上CPU，50/52、31/35、2ms、47背景保留 | 新8行规格/尝试交接；六资源结果与同源展示正确 |
| 未触发边界 | 新显式重选0次patch | 失败后成功重试、最新失败覆盖旧成功未自然发生 |

共同barrier release13:20:19.740119Z，A/B exec13:20:20.424809/424803Z。实际日志A13/B42对、零歧义；最大正交集 **24.550秒**：A原行222–223为13:20:26.368–53.812Z，B453–454为13:20:26.899–51.449Z。正式-25200秒时区偏移来自spawn收据；原非法UTF8字节保留。独核44个正交集，不用进程时长或barrier代替模型请求证据。

### B的同代审计与执行分母

唯一成功普通status，原日志1183–1185，call `call_01a187d2eadc75a290c74a61`。同次result SHA `5b46bfe46c374768b8fdf23a15edee762436bb4f391744ab94a94f41725948b8`，index SHA `f2c45419831ca969042b61c0c1a839e67c6e1b8a8d666f9f67b96797412d52ae`，selection SHA `5359d89be7359dfc9845a6c897e8e73e905a04170aa2982eb7df3acbd7222041`。原Go审核PID47875、13:28:24.557985–13:28:25.642379Z，实际exit1：`cluster specification or full membership differs`。没有成功JSON是失败结果，不是未执行。

该代12记录＝8声明＋4观察，8尝试；原计划4success/4not_executed，资源2success/2empty。后续两次查询后的INIT另为14记录/10尝试，资源4success/2empty。四个选中timeline各成功一次，四个未选各零尝试；6个额外resource查询不能改写原view/选择身份。完整规格交接是限定PASS，选择语义仍FAIL。

## 5. 剩余验收与ROI

| 顺序/原ID | 后续动作与边界 |
| --- | --- |
| 高：16.4/12.5/18.2 | 对齐源事件点/状态区间与调查解释；实际INIT既有正确事实又重传矛盾advisory，是本批已证共享接缝。保业务排查线索，不扫自由文字设硬门。 |
| 完整能力槽：只读登记；11.2继续留账 | 轮换原负向终态；代表意图到精确view/规格继续系统审计，不反复用同题求绿、不预指定工具。 |
| 覆盖：12.5/18.4 | 新explicit native_participant公开回归已绿，但自然0patch，保持NOT_TRIGGERED，不造题触发。 |
| 二进制：17.6 | 有新真实格式/工具/宿主材料才扩验收；已知同字节再包装不算新覆盖。 |

旧采样/IO解释、权限未触发、§310扩大race欠项继续留账。原查询总体、合法计算/重试/未选查询、Trace因果投影与自动补齐未改；WAL未满足零副作用前不接生产。事实充分的孤立模型措辞不无限阻塞其它高ROI工作。

## 6. 封存、提交与推送

474件按原来源身份＋SHA本地封存，索引SHA `9a2f9735bbda6549a6a76694461bffe8af8d11761adce8150f4c987067328dc5`。旧15档4475件/6,679,056,345B及336快照来源身份未变；18历史污染身份是336子集，不重建过期tmp或恢复进仓。原始证据保留在按现有规则忽略的eval/results；代码与文档分笔推送，不声称大型原日志已进入Git。

- [封存索引](results/hmc_repair_pairing_representative_20261029_v1/sealed-index-v1.json) / [封存收据](results/hmc_repair_pairing_representative_20261029_v1/sealed-receipt-v1.json)
- [全仓原日志](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0211-codrax-hmc314-full-v1.log) / [正式终态](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0208-codrax-hmc314-full-v1-receipt.json) / [统计](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0207-codrax-hmc314-full-stats-v1.json)
- [A独审](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0131-codrax-hmc314-A-audit-v1.md) / [B独审](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0150-codrax-hmc314-B-audit-v1.md)
- [B真实结构失败收据](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0168-codrax-hmc314-B-structural-audit-v1-receipt.json) / [原失败日志](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0171-codrax-hmc314-B-structural-audit-v1.log)
- [独立完整性审计](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0249-codrax-hmc314-integrity-v1.md) / [主代理交叉复核](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0445-codrax-hmc314-root-review-v1.md) / [人工表面汇总](parallel_selected_summary_hmc_repair_pairing_representative_20261029_manual_audit.md)

11文件代码提交 `e2c7b55fc1b8bc2370286d952c5bee25e901e2d3`；五文档相邻提交。普通main推送、两提交逐源码冻结字节、旧档和当前封存的一致性以[publication收据](results/hmc_repair_pairing_representative_20261029_v1/publication-v1.json)为准。该收据存在且通过才称已推送，不预写未知docs SHA；它不把机器/人工失败、未触发分支改成验收通过。
