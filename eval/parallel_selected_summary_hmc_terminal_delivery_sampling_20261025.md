# HMC293：普通状态同代交付与采样轮换

基线 `5fde3a90345c82c7a53d1b0e0c7c9908fa5c92c8`。79唯一任务＝25完整交付＋54开放，18类；本批完整能力新增0，共享交付子能力1组。实现通过不代替原任务或完整答案验收；旧失败原件不改签。

## 实现与参考

普通目录状态页和代表状态页共用同次Publication字段。已有当前选择时，普通页也返回同代执行索引、代表快照及各自SHA，不要求模型调用特定status动作。保原查询记录、分页、执行覆盖和合法query_calls；跨分支合并不回灌执行权限，旧交付不重写，无选择不造选择，历史材料不恢复选择/成功权限。

参考 `core/batch/closure/select.py` 与 `package.py` 的设计意图是稳定选择和对应材料自包含交付，不是再做一次选择。本仓复用已有SavePublication，不改统计器、Trace因果投影或自动补齐，不新增原文关键词门。`query_engine.py` 的原总体/预览分离与 `llm_contract.py` 的成员字段回查用于审计，不照搬文本相似度硬拒。

§292审计更正：实际三次emit均有4个解释单元，最终仅发布[0,2]；箭头单元和member_notes未进入实际Finalizer。正确组数、真实零、深度边界已到场；7/7行、6/7 notes缺项信息在同源摘要/明细已保留。因此本批不为旧误述臆造摘要污染修复，不删除合法说明或扩大预算。完整旧FAIL保留。

## 冻结与两例

冻结7366执行输入及外部材料，SHA `9616b877dd2089171e87678c5262a54a90fa5b9264ccbae08c74d4bd529e5038`；标准make后实际CLI SHA `87a2890f5d0d61fd18b3ae814d73dba5b8cdb71eeaad052f6b4dcc507b2193b3`。两例各一次，无第三例；实际exec为UTC06:21:39.106258和06:21:39.136618，共享屏障、同一二进制。实际请求重叠27.556秒：A请求UTC06:21:49.045–06:22:19.232包含B的06:21:50.399–06:22:17.955，非仅凭进程并发。原问句不添加系统约束，operator oracle不供模型。

| 槽位 | 原始输入与自然入口 | 进程/机器 | 完整人工 |
|---|---|---|---|
| A | 四份构造SQLite副本，原代表题，普通CLI | exit0；原离线规格审计exit1 | FAIL：线程分组未按要求落实、栈深度被写为调用关系 |
| B | 完整真实30,062,585B PERFILE2副本，原sample-only case/run.sh | exit0，机器smoke PASS | FAIL：unknown采样类型被说成on-CPU，数值正确不抵消 |

A四文件来自两种已有SQLite，不是四个新设备捕获。B源SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；本批使用独立参考parser复核全12000、窗1393/11线程、目标164/259，原件/兄弟目录不改。BRBE/SPE及未知载荷未因此获验收。

A最后真实status是6次尝试/8计划的同代时点，后续当前执行上下文为12条登记/16次尝试；不能把后者回填旧回执，也不把最终12/16误判为幻觉。本轮实际投递2/6解释单元[0,1]，其中4事件误述被最终纠正，调用箭头仍保留；这与§292未投递箭头的旧归因不同。两例均没有实际图，不签图验收；A19表、B3表静态内容审计不冒充像素QA。

## 证据与质量

原始证据根：`eval/results/hmc_terminal_delivery_sampling_20261025_v1`。A `/tmp/codrax-hmc293-representatives-v1`，B该根下 `B/trace_binary_sample_only-20261025-232136`。正式人工记录见[审计](parallel_selected_summary_hmc_terminal_delivery_sampling_20261025_manual_audit.md)。

公开RED 3顶层失败；GREEN v1保留历史快照测试漏pin失败，生产授权未放宽。末版定向28顶层/49子例PASS，race20顶层/8子例PASS；原离线audit未改，公开回归普通status同代8/2/4/4可通过、pending/混代负控仍拒。标准make和diff-check通过。

唯一冻结后全仓PID23958，UTC06:21:37.491060–06:37:00.099473，exit0；97包PASS/12无测试包/零FAIL，28323顶层与33022子例PASS，41顶层/14子例SKIP。日志SHA `f96a24be6452b270c7365c6eb29e1b542c6084cacf0fc60026e16e67cd51b123`，输入前后不变。SKIP不算实际能力覆盖。

304份原始证据及审计材料已逐文件封存；[索引](results/hmc_terminal_delivery_sampling_20261025_v1/artifact-sha256-v1.json) SHA `4135fdc8de845adc945b32726c8e2cbc3ec02850c02d26ad0e323f03aeb1256f`，[封存回执](results/hmc_terminal_delivery_sampling_20261025_v1/archive-v1.json)绑定同一冻结版本。原始失败、实际日志和人工FAIL保原，不收录凭据。代码提交`5659530c0e782e5af25f36a8aaeaceda14a067c7`，文档另笔；远端、工作区、两提交树及封存文件的最终核验以[发布回执](results/hmc_terminal_delivery_sampling_20261025_v1/publication-v1.json)为准。

## 剩余与ROI

本批验收残留3类，均挂现有任务不另增父项：代表任务参数错误后的线程/窗口计划保持；原生计数/深度与正文关系权限一致性；采样unknown与执行状态解释一致性。前者优先检查结构化错误恢复导航和通用/任务教学适用域，保合法手写规格和额外查询，不能禁止探索来凑原矩阵。A实际解释单元存在窗口/深度越界，下一步审其精确引用范围；B错误reason未投递，不能混为同一传播原因。无新共享RED前不反复改统计器或追加同题硬提示。

旧IO解释、计算权限自然未触发、只读登记更宽终态继续留账并轮换；具备新真实材料的17.6能力优先于追跑同题，材料缺口不由构造输入替代。WAL零副作用未达，不接生产。
