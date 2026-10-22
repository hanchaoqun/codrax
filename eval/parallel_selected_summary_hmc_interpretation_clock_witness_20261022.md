# HMC280：解释单元交接、真实完成计数与原生时钟见证

基线 `65f4fb0b9d2d094bbd0d8b91df09408556cc669e`。逐 checkbox 独立重核：79 个唯一编号，24 已交付、55 开放，涉及 18 个能力大类；不把能力增强、持续验收和产品缺陷混作 55 个已发生故障。原 HMC279 两份人工 FAIL 保留。

## 双轨范围与参考设计

1. 缺陷轨：runtime-only 安全分支误将本轮受理的结构化解释单元一并屏蔽。只让显式独立分析、当前受理且来源/查询重新绑定成功的整单元进入建议性通道；仍抑制旧自由摘要、reason、notes、复述、缺角色、JSON恢复、失效来源及 no-directed-path/未证实日志关系。提议引用不是证据，原生事实优先，不改因果投影或自动补齐。
2. 能力轨：05.2 原件中确切 clock-sync 点的查询与成文展示。原始 Trace ns、原十进制值和协议单位沿同一事件库存/原生语义字段通道；协议解释与本原件实际生产者身份分开，后者未知。没有校准偏移、漂移、误差、有效区间或跨日志对齐权限，05.2/05.3不因此整体销账。
3. 评测基础设施：同一 ok=false 结果的头和 rejected 正文不再重复计数；旧 ok=true 语义拒绝只看紧接的结果首行。accepted/downgraded 来自 explorer 的类型化状态观察，不是所有 tool-call 的完备分区，缺日志/多工具批次仍可能未观察。多行日志并非抗伪造收据，实际结果须独立审计。兼修 Bash 3.2 下空工具要求触发未绑定数组错误。

参考 `core/llm_contract.py` 保代表成员、字段与值坐标并独立回查的分层思想，未复制正文查重/数值扫词硬门。参考 `core/extensions/log_extender.py` 的采集/对齐/查询分层，但不复制中位偏移、文件名时间、`aligned=true` 或白名单救回作为本仓时钟权限。协议依据为 [OpenHarmony MarkClockSync](https://raw.githubusercontent.com/openharmony/hiviewdfx_hitrace/master/utils/common_utils.cpp)：两种参考钟分别采样后写 marker，不是同时读取；记录精度不是误差上界。该代码不证明任意原件的生产者身份。

## 冻结与两例

7270 项范围清单及实际 CLI 冻结：`/tmp/codrax-hmc280-frozen-v1.json`，SHA `4027ebc33f7517b66447ec7d944200a821c6cb90c34a4e1d44f18ccd32fc1c01`；CLI SHA `4d673aff0ee128a50787c22eb68061ae7910e3c94df36339055a5d63cc0e402f`。全仓和 A/B 各一次同时启动，不追加第三例求绿。原件逐侧登记，修正旧 helper 将 B digest 当作 A 原件字段的歧义；旧收据不改写。清单过滤器未包含非Go的internal/testdata；新增505B两行节选在测试后与已冻结原gzip物理429168/429170行逐字一致核验，SHA `66dd89ced013825e604dba9dce3bf6387ddc91d7f601290437e53d835fdb2e91`，独立事后addendum不冒充运行前冻结。生产Go/CLI/用例及两份真实运行原件不受该漏项影响。

| 例 | 自然目标 | 原件 / 独立 oracle | 状态 |
|---|---|---|---|
| A `trace_native_resource_statistics` | 10.000–10.050秒按线程/资源统计申请、释放及调用位置 | 原 SQLite 构造fixture，非实机采集；SHA `a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`。只读4事件/11帧/3组，申请768/128B，释放512B | 机器PASS / 人工FAIL |
| B `trace_clock_sync_witness` | 记录了哪些时钟同步信息，能否换为实际时间 | 真实原 gzip 3,602,086B，SHA `4d66880019663f81462fe5df1615da58b368b6c5225e211dae88600cfc5396e9`；完整429766行，两记录在429168/429170，Trace时刻170441.373302/170441.373322，值1787277345616ms/10143.297852s | 机器PASS / 人工FAIL |

## 验证与诚实边界

解释公共 RED 保原；相关30主例/109子例、race2主例/19子例通过。原 HMC279 A 的7个单元经现有预算仅显示3个完整单元（索引0/1/3，1753/3646B），明确省略1893B；这是确定性预算回放，不是原失败整答改签。新时钟公共查询、indexed/streaming、实际成文、非法/普通marker/空窗/精度及当前源换代重查测试通过。旧未加作用域的历史库存收据仍保留，不宣称全部历史成文被删除或隐藏。

计数反例基线7而正确3，修后通过；历史 A 只读复算4提交/2拒绝/1降级/1受理，原日志SHA不变。整套 runner v2通过。v1相关runner在脚本尚被编辑时执行，发生非冻结读入语法失败，并发现空工具要求兼容问题；原输出保留，不算正式冻结验收。时钟施工期编译错误和两项测试假设错误同样保留记录，不伪称先前原始工具输出已有磁盘日志。

正式全仓PID53114，UTC2026-10-22 08:15:16.579304–08:29:58.358751，`go test -json -count=1 -timeout 20m ./...` exit0，97包PASS/12无测试/零fail事件与非JSON行，日志SHA `4b5ab02dd1aa72eb6caa4a2c969fca409d169e9ce5cdbdf9926580b986b15d47`。`make` exit0，CLI同冻结版；没有后继生产或测试变更、第二轮全仓或追加模型运行。

A PID53195 UTC08:15:17.771988–08:19:10.990422，B PID53314 UTC08:15:18.933974–08:25:28.892916，均exit0。A `emit_investigation_complete` 1提交/0拒绝/0降级/1受理观察；B 4提交/1拒绝/0降级/3受理观察。两例finalizer均1次受理/0拒绝/0重写；这些只是实际状态计数，不证明答案正确。

A当前4解释单元2964B，实际Finalizer保索引0/1两个完整单元1445B，披露省略1519B；不是全量解释投递，原生reference publication为1/2，不能从解释所有权倒推全量事实发布。完整原生4事件/11展开帧/3组另在原生上下文与表格可见。原生事实/数值/HTML局部通过，正文有向调用路径、“完整栈”和NULL subtype原因错误仍FAIL，未传入单元不能被指认为错误唯一来源。

B两名审计者确认单点相减的算术无误，但答案断言整条时间轴/任意t可统一换算，且把毫秒分辨率说成可保证的对齐精度；尾部无漂移/误差/有效期声明不撤销前文断言。明确条件下的估算可以展示，本次没有清楚维持条件边界。实际Finalizer 5645行原生限制完整到场，但5561行把命令相减的aggregate标签“固定偏移”置为policy=hard/authority_ceiling=factual，5936行又作为principal_answer交接。数值有计算依据不等于标签所含物理关系有权威，这是已证共享输入竞争，不能直接归纯模型波动；尚不能证明它是输出错误的唯一原因。14次查询回执中3次为index_event_limit状态回退，不是请求查询成功/零结果；最终24行CPU频率补表逐列同源正确，不能因正文失败禁用自动补齐。完整表面与原流程审计见[人工报告](parallel_selected_summary_hmc_interpretation_clock_witness_20261022_manual_audit.md)。

## 封存与发布

UTC2026-10-22 08:51:50.096817封存242文件，保留结果目录26原件并复制216材料，含25源码/用例/fixture快照、实际CLI、A原SQLite、B原gzip、完整准备/查询/blob/MD/HTML、原始失败和详细独立审计。索引`eval/results/hmc_interpretation_clock_witness_20261022_v1/verification-receipts-v1/artifact-sha256-v1.json` SHA `fa775596ea851a38a5f5d8a7fa79397011b09f6547cd1fa33b4cd8e2a2af945a`；归档收据SHA `918ba129f3861df709f7fffa9daaaceffb967e75e60dbfcee340ef50eaf555b6`。两份独立人工报告SHA分别`5af1994b70c8e2cd7ff478b9696c1356e57d1fe8f7c11cf0c3811e94d1749e9e`、`2bec97ae07d0a920260d7ee910d8e6e365c7dc9b864b697616d0e31acaceec39`。

代码`7d75ced9eb9c219caeef07fa9cdd83511a14b061`普通推送；文档另笔提交推送。最终HEAD/origin/main/真实远端一致、clean、7270项freeze与事后fixtureaddendum、归档索引自身及242文件逐SHA复核见独立`eval/results/hmc_interpretation_clock_witness_20261022_v1/publication-receipts-v1/publication-v1.json`；该收据仅在实际发布终态满足后生成，不修改封存索引或旧FAIL。

## 剩余 ROI 队列

1. 16.4/18.4 P1：command_measurement/aggregate的数值证据与模型标签中隐含的物理关系分开授权，统一原生事实、Claim Binding与principal摘要；保持合法计算，不能由算术工具成功授时钟映射/有序关系/因果资格。不新增字段值或问答扫词硬门。
2. 16.4/01.2：分组统计的完整分组人口与原始成员清单义务区分；不能让任意 distribution 冒充原始全集。消除真实工具修补摩擦，不重写正确计数器。
3. 05.2→05.3：复用现有来源/时钟合同，一次补可审计校准关系与指定窗联查，不再拆marker子片。先取得同设备/启动身份、日志年份时区、独立微核桥及有效期/误差依据；现有两非同时点不足。未知映射保独立阅读，不靠文件名/邻近时间对齐；详见本批封存的一页完整能力方案。
4. 14.1 余项：`native_hook_statistic` 生产协议的累计/增量/reset 证据；事件统计已交付部分不重复施工。17.6有新真实格式/版本原件、可运行工具及原生宿主凭证时提升优先级；既有 ZIP/gzip/PERFILE2 复验不是新增格式。只读登记轮换剩余未覆盖终态，不重做已验收入口。
5. 18.3/18.4：旧采样执行状态、权重/时间和栈顺序解释越权继续留账；已有完整事实时不无限追同题。17.7 WAL零副作用未满足，生产仍NO-GO。18.5冻结清单漏testdata及resource_stack指标覆盖另留低优先级。

其余开放ID完整留在统一任务清单和本批独立roster，不删账、不重复拆成新增故障。当前完整能力新增0/累计24；本批生产子能力2组、评测修复1组，两份整答验收欠项归原18.3/18.4。05.2改部分实施，当前31待实施/17部分实施/3待验收/3验收中/1持续执行，合计55开放。
