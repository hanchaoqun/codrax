# HMC286 人工审计：窗口成员展示与只读负向恢复

结论：**两例完整人工均FAIL；A的宽查询窗口成员交接子能力通过且自然命中。** 79项中25项完整交付、54项开放不变，不用单测绿或A机器PASS代签完整能力。实际两例模型没有并行，原因和原始启动失败保留如下。

## 原始证据与运行边界

结果根为 `eval/results/hmc_resource_window_projection_20261022_v1`。独立完整报告已封存：[A逐面审计](results/hmc_resource_window_projection_20261022_v1/sealed-v1/tmp/codrax-hmc286-A-manual-v1.md)、[B逐层审计](results/hmc_resource_window_projection_20261022_v1/sealed-v1/tmp/codrax-hmc286-B-manual-v1.md)。报告SHA分别为`5ae6fbd5f34b91f0ab7831e0b9490059181f64c7c5d23fc8b26fde33853ef54c`、`9d0f8ba725f2c141f0cf0a23d5cb36e503034304a40ddce27e6177eeedae4948`。主控复读最终A正文、原生表和两份独立报告，并核正式收据/日志哈希。

A的v1启动器在模型前因dirty build退出2，未产生A结果目录；不是产品回答失败也不算真实例。A改既有`eval/run.sh`后首次实际执行一次，冻结源码/CLI/原件未变。B已先结束，所以实际两例无重叠；未重跑B或追加第三例。真实收据：[A-v2](results/hmc_resource_window_projection_20261022_v1/sealed-v1/tmp/codrax-hmc286-live-A-v2-receipt.json)、[B-v1](results/hmc_resource_window_projection_20261022_v1/sealed-v1/tmp/codrax-hmc286-live-B-v1-receipt.json)。下一批零模型预检应先确认启动器接受冻结但未提交的构建方式。

## A：先概括采集，再比较两个线程的窗口资源事件

构造SQLite原件SHA`a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`，不是实机或新二进制格式。自然问题未指定工具/宽查询，也未带系统边界或oracle。实际CLI PID42225，包装PID42146；UTC07:21:45.709407–07:25:52.101246，exit0。机器PASS仅工具成功等存在性检查，不是语义oracle。

5次真实查询＝1次event_search＋4次resource_stack；无仓库源码读取。资源查询分别为同窗全部主体4事件/11帧、101的3/8、202的1/3、模型自行选择宽窗[10,10.06)的5/14。宽查询原分组保持5条总体，私有成员投影给出4条/11帧并排除右界10.050的一条；实际Finalizer note-25和完整members表11行、最终附加summary均命中。双目标身份未合并，共用callchain不等于同一主体。

| 范围 | 线程 | 申请次数 / 已知字节 | 释放次数 / 已知字节 |
| --- | --- | --- | --- |
| 原件全表 | 101 | 3 / 1024 | 1 / 512 |
| 原件全表 | 202 | 1 / 128 | 0 / 0 |
| [10,10.05) | 101 | 2 / 768 | 1 / 512 |
| [10,10.05) | 202 | 1 / 128 | 0 / 0 |

数值、owner和半开右界均正确；零释放来自当前完整查询中无该操作，不是未知大小补零。全表5事件由SQL与转换rows_read/emitted=5、宽查询完整census共同支持，不是有限预览外推设备全集。原件捕获完整性仍未知，不授设备活动完整性或泄漏结论。

### 人工未通过项

1. **采集范围错用**：原件trace_range是10.000–10.100，100ms；资源观察包络45ms；用户请求窗50ms；转换文本首尾50ms。正文将“采集总跨度”写约50ms。实际预处理只说明附加片段50ms，已送达调查解释unit0又称采集只有50ms，形成同源口径竞争；不能把四种范围互换。
2. **地址语义越权**：known signed=-1及全一位型不证明有效、缺失或哨兵。实际原生说明已到上下文，终稿仍称“哨兵值”。不能替换成0或靠关键词封禁。
3. **栈解释风险**：深度0被称入口、深度2被称最上层，方向箭头与原深度/未知展开声明竞争。箭头可表示列举，不以字符本身作硬FAIL；前两项确证错误已足以判完整FAIL，层级关系继续留账。
4. **复用与冗余**：最终3张distribution、4张summary共7张原生表，summary展示12次、实际仅4独立请求事件；没有算术双加，但缺少同源引用复用。保查询身份及分母，不为去重合并不同查询权限。无原生members最终表；正文描述全11关联帧的符号/深度/未知状态，但无损ID/offset等仅在上下文/JSON，不扩签最终无损帧明细交付。

10.050事件与另外两次申请同属callchain7，“同栈”有原件支持，不误列失败。模型没有泄漏、CPU执行/线程等待/响应根因结论；root-causes旁路为contract未激活，不能宣称根因路径自然验收通过。

### 实际上下文与覆盖

accepted调查载荷有7个解释单元，实际Finalizer只投递3/7（indices0/1/2，省略2210B），不是全部7条。错误50ms属于实际投递unit0；unit4未投递，不用中间思考指控其污染。unit6正确地址解释虽未投递，同一正确边界已在原生字段说明，不能说系统完全缺证。11个members行及原5条统计均在actual INIT，没有预算把本次成员截掉的证据。

最终Markdown与HTML内容已核，无浏览器像素QA。无Mermaid/原生diagram；公开确定性矩阵覆盖摘要/表/无边图不代替自然图验收。机器`trace_query_final_projection_blocks=0`也不否定本次实际note和成员已命中的事实。

## B：补证验证成功、来源缺终态的负向恢复

唯一实际入口为真实planner＋脚本verify/finish恢复controller＋独立子进程＋公开真实RunTests，非完整CLI/自然controller。父PID29209、子PID30107；UTC07:20:06.490480–07:20:22.136052，exit1。一次问题、两轮模型，无传输重试；读value.py和test_value.py成功，一次合法changes:[]及精确原断言登记，无schema拒绝。

| 层次 | 实际状态 | 验收含义 |
| --- | --- | --- |
| 来源批 | 有旧交付/测试，无来源终态 | 旧绿不补造终态 |
| 新原生测试 | 1条assertion passed，2条命令（预检＋unittest），新invocation | 命令数/工件数不是测试数 |
| 补证账本 | verified，4义务/能力覆盖 | 不等于独立完成资格通过 |
| 补证批及整体 | unverified/localization_observed_without_owner | 未到预期补证批verified、整体missing_terminal_verify_verdict |

源码/测试SHA及HEAD前后不变，独立child记录readonly=true；本轮invocation与来源历史不同。真实测试和原件留存子路径可确认，完整负向验收仍FAIL。后续缺来源终态及授权撤销断言没有运行到，不签其当前版本已覆盖。

实际INIT同时有只读豁免和controller_preplan owner要求。静态资格链要求精确来源apply/verify/terminal；当前负向fixture缺终态且不具完整来源artifact资格。既有missing_source_terminal和unverified_source_owner负控保护真实来源欠证，不能简单放宽门或只改期望为绿。后续应拆无新源码义务、来源owner已失败、纯缺来源terminal三种情况。

child归档4原件/1verify引用完整，8/8父子存档字节与6/6报告canonical摘要核验正确。child Fatal使父未镜像当前状态；父旧planned快照与真正child final不匹配，parent archive_complete=false。不是child原件缺失，也不能拿父根目录null报告当末代说明。真实final在`restart-child-delivery-artifacts/originals/`，全部保原。无B用户MD/HTML/图，不套A展示验收。

§206相同受限负控的历史PASS、§203历史FAIL及§276自然controller正向均独立保留，不互相代签当前负向。无来源欠证被洗成整体成功的证据。

## 验证与不可变封存

最后定向三包69顶层/166子例、race两包7顶层/21子例及CLI/测试二进制重建通过。全仓PID27932，UTC07:19:45.410706–07:34:41.213459，exit0，97包PASS/12无测试/零FAIL及非JSON。冻结7332输入前后一致；中间HMC248 JSON回归和旧HMC249断言失败另存，不用最终绿回签。

162文件封存，含两例原始参数/返回、actual INIT、A五个完整查询载荷/准备原件/MD/HTML、B父子状态及真实末代原件、全部失败/验证日志、冻结输入与变更源码快照。索引SHA`f13374c7e134ce20fd978daef89d66f52c2a8b210687203e5f994ba31e107814`，见[索引](results/hmc_resource_window_projection_20261022_v1/artifact-sha256-v1.json)。后继文档与普通提交不修改原件。

完整能力新增0/累计25，共享子能力1组，整答欠项2组仍属于54开放；A自然图、B目标终态及更广完整CLI仍欠。旧IO/采样解释和权限自然覆盖不销账，WAL未满足零副作用，继续生产NO-GO。
