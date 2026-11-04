# HMC348：生产者单一合同、完整网易云与sample-only轮换

完整能力累计28/79（本批新增0）；已交付子能力1共享组；剩余51项，原状态18部分/29待实施/2待验收/1验收中/1持续不变。main基线f862e3351b9f3ec04cc1ba9c9ede48d4c230008f，源码c6715c27d6c3192e99cbd838913e8ea07b2c48c7，7Go/434增46删（4生产/3测试），没有新建分支。源码通过、机器smoke、有限入口和完整人工验收分别记账，最终main/远端以独立publication核验。

## 本批交付及参考发现

公开原生query→当前事实→actual Finalizer→完整emit摘要/表/无边事实图RED复现§347的新域端点遗漏：成员列/Notes已有正确半开窗，但独立publisher名单没有它，统一来源标题只能未知。修复不再追加另一名单：17个现有生产者在私有单一声明中选择predicate、允许view及interval/aggregate/raw/event/sampling/resource族，注册、视图、raw/generic路由和区间端点opt-in共用。全矩阵/重复/未知枚举及实际publisher正反测试覆盖演进。

声明只是精确路由，不是证据。现有当前来源代次、精确query、完整表/Notes、专用域验证、JSON清权、MemberSet/DisplayPopulation、关系、根因及完成owner不变；篡改不能经Finalize恢复端点，旧未知仍未知。Trace因果投影、原4根因、闭合睡眠实例、链上业务/语义信息与自动补齐不改，没有问答关键词硬门、原断言放宽或新JSON教学分支。

对照参考registry的集中元数据/冲突验证意图与llm_contract逐行列地址职责，采用闭合typed声明，不搬result_schema尚待验证的假设、数值容差或文本/fingerprint门。参考binary parser分别读CPU/TID/time/period：事件名和cycles字段也不能代sample_kind或时长证明。

## 验证与完整实物材料

终版相关四包308顶层/622子、race×2三包44/440、原因果四包21/141 PASS；标准CLI、startup零模型零工具、15项真实runner实参正负检查通过。初次新summary工装、线程标签工装及不存在包路径的失败保原，不删失败或改旧断言追绿。

完整用户网易云ftrace 82838676 B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4；没有裁切采集或改原题。冻前公开双query核11组16成员原始B/E、线程/TGID/打点PID，因果投影不变。live32个业务节点另经整件独立整数微秒LIFO核名字、边界/inclusive/self/全部直接子数量，0差异。原有独立调度oracle核627.645ms总量、194.581ms self及状态；本轮不冒重新全算调度/因果oracle。

原始test_brbe.data完整30062585 B，SHAda713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632，本轮参考解析器实算全12000/窗1393/目标164/period259，CPU0/1各82、权重85/174和各自ns包络分别保真。operator oracle从未注入模型，未验证BRBE语义。

冻结7624仓库输入/332外部/15仓外快照，SHA74e5d0a05e050b81c397494d0c1c285421fa6b86e15effb243e6efaef037623b后代码/测试/教学/helper/case/oracle停写。唯一full UTC10:06:12.462440→10:20:48.921409，OS0/源前后true：98包PASS/12无测试，28756顶层PASS/53SKIP、34534子PASS/14SKIP；268285纯GoJSON，FAIL/重复/未终态0。log SHA1baed618d02a7e2e71354ec293c05404c5547fe483d84b1ef688aaca68c443e7。

## 两例真实并行与主审

| 侧 | 原例，各n1/15steps | 原机器 | root人工 |
|---|---|---|---|
| A | 完整网易云bindApplication启动 | PASS；3原生query/3JSON | 原生/端点PASS，整答FAIL；最终新成员表/图未选 |
| B | 原始sample-only二进制 | PASS；6原生query/6JSON | 自然准备/取数有限PASS，执行态解释FAIL |

精确请求A16/B7、0歧义，最大真实重叠33.115秒，不以barrier或OS0推断。只跑原两例各一次，不追第三例；机器2/2、完整人工0/2。

A实际INIT296989 B的11个新成员selector标题全部恢复`[5969.994941,5970.622586) s`，未省成员行。32个层级及原生状态可支持正确回答；正文却把嵌套睡眠50.090/46.424ms相加、把含睡眠self称纯执行、过度禁止合法不交账户加法。上游错误reason6660 B本次没有传入，不能冒上下文污染。完整MD635601 B/HTML3764746 B、14表含3735条状态时序，最终整组预算和去重仍开放。

B原件/11596 B bundle/14549907 B预览分开，完整12000样本准备成功；164/259与两热点82及174/85、独立首末包络、CPU记录正确，预览40/10行没有替代分母。最终仍在承认sample_kind未知的同时断言只在CPU执行时取样、停顿本质是off-CPU。未投unit2/5不冒污染；已闭18.3有限能力不再计算一次完整交付。HTML/MD还重复两份相同热点表。详[手审](parallel_selected_summary_hmc_producer_contract_netease_binary_20261104_manual_audit.md)。

## 剩余验收4组及全部51项ROI

1. 原生事实到独立正文的适用域、状态与合法运算：保留明确source/query/线程/实例和不交/嵌套边界，已充分给事实的模型误述留原，不无限追同题。
2. 最终完整答案的同源复用/跨块整组预算：context字典和正确原生表不代表MD/HTML去重完成；不能靠删除事实或抬预算掩盖。
3. 有材料二进制、只读自然正负终态及代表/权限/图覆盖：新最终成员selector/图未触发，只读本批NOT_RUN，更多格式/工具/平台未验，不借历史PASS补签。
4. 其它协议/新域与WAL：缺实物/协议列原项，WAL零副作用未满足前生产NO-GO。

P0共享（7）：16.4、01.3、02.4、16.5、01.2、18.4、12.5。

P1有材料验收（6）：08.3、18.5、11.2、03.2、14.1、18.2。

P2复用能力（11）：04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2。

P3协议/材料（21）：04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。

P4新域（6）：07.1、07.2、07.3、07.4、15.2、15.3。51个稳定开放ID无漏项/重复；参考仓相关共享gap及有实物二进制优先，旧IO/采样及未触发权限等继续留账。

## 封存与交付

521件原始材料，index SHAeddba9aa6204d4b24b397e02d67af3ee3a34ccc65a9a42f150dee73f0ed480af、seal SHA390d96a0b9b12cfb784989425b0570dfa3548e1dd366546875736cec22508bbc。此前49代21886件＋本代＝50代22407件，按代非全局去重；物理重核最新347568件及失败344554件，其余47代仅链头。原件大文件只存引用，providers不读/哈希/复制，失败历史不回签publication。

[封存索引](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-index-v1.json)、[封存收据](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-receipt-v1.json)、[发布绑定](results/hmc_producer_contract_netease_binary_20261104_v1/publication-v1.json)。发布绑定只在实际本机Git push及远端同代校验成功后生成；不改密钥/仓库权限、不改用GitHub写API。原生/全仓通过不代表两个完整答案通过，稳定closure=none。
