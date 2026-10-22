# HMC280 完整人工审计

本批恰好两个真实模型进程并行各一次，机器2/2通过、完整人工0/2；两例均由主审与独立审计者复核。原始答案、日志、附件、引用、HTML和失败不改写。静态HTML内容审计不冒充浏览器布局/截图验收；本批没有自然图，不代签图修复能力。

## A：资源申请/释放与原始栈帧

原件为既有SQLite构造fixture（非实机采集），SHA `a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`。结果目录 `eval/results/hmc_interpretation_clock_witness_20261022_v1/trace_native_resource_statistics-20261022-011517`。

完整读取primary/principal（各13,104B）、实际MD/transcript（各16,290B）、emit原件、sidecars、HTML main（18,739B），并只读核原SQLite与查询payload。窗口[10,10.05)内4事件、11展开帧、3统计组正确；gallery-main/Gallery两申请768B及一次释放512B，background/Background一次申请128B，10.050右边界事件未纳入。11为按事件展开的帧记录，不是11条不同的物理帧表行。宽窗检查及19条carrier查询未替换统计人口，源/查询身份保持。

实际Finalizer收到原生4/11/3全部记录与限制，同时收到当前4个解释单元中的0/1两个整单元，共1445/2964B；原1800B预算下省略1519B，索引和不完整性显式披露，边界与support_refs未切断。引用为提议定位，非已验证证据；native reference publication=1/2不等于解释或所有事实完整发布。旧§279七单元完全未进入解释通道的确定缺口已局部闭合，不能称本次所有解释都进入。

整答失败：

- primary第11/19行写“最外层”和`UIFrame::render → ImageCache::reserve`路径，原生仅证明源帧/原深度，不证明有向调用链。后文“不是有序调用链”不能撤销前文断言。
- 第13行写“两次带完整栈”，实际`unwind_completeness=unknown`；源记录齐全不等于真实展开完整。
- 第7/85行将subtype缺失归为字典解析失败，原SQLite的sub_type_id本来就是NULL；7/8/9是callchain ID。字典坏引用与NULL不能混同。

完整HTML4表（附件＋3原生）均保值/身份；正文同样保留上述错误。无diagram/SVG/image，root-causes sidecar为unavailable且合同未激活。解释单元1也把callchain误当subtype，属进入上下文的竞争解释；单元2有三申请同栈及有向路径错误但被整单元省略，不能把其未送达原文指认为本次错误因果。

真实状态：3次trace_query成功、1次complete受理，0拒绝/0降级；finalizer1次受理，0拒绝/0重写。analyzer曾因互斥窗口字段拒绝1次，不能算complete拒绝。计数局部正确不等于答案通过。`trace_query_resource_views=0`旧指标不覆盖resource_stack，不能读成没有执行资源查询。附件显示bundle大小使用systrace大小的既有显示问题继续留16.5，不影响实际来源绑定。

独立详细报告 `/tmp/codrax-hmc280-A-manual-v1.md`，SHA `5af1994b70c8e2cd7ff478b9696c1356e57d1fe8f7c11cf0c3811e94d1749e9e`；原生准备、blob、MD/HTML等精确额外路径见对应extra-artifacts清单，随本批封存。

## B：原始时钟记录与换算边界

真实gzip原件SHA `4d66880019663f81462fe5df1615da58b368b6c5225e211dae88600cfc5396e9`，完整429766行，只有两条clock-sync记录：物理429168行170441.373302秒，`realtime_ts=1787277345616`；429170行170441.373322秒，`parent_ts=10143.297852`。结果目录 `eval/results/hmc_interpretation_clock_witness_20261022_v1/trace_clock_sync_witness-20261022-011518`。

主审完整principal已读：原值、协议ms/s和两条计数正确，但肯定“足以把trace时间轴换成真实墙钟”“对整条时间轴加同一个偏移”“任意t”，并称“毫秒量级上换”。差值1787106904.242698的算术本身正确，不证明采样同时、有效区间、漂移、误差或本原件生产者身份；时间格式转换也不证明全程映射关系。明确假设下的估算可给出，本次肯定断言与尾部caveat互相冲突，整答FAIL。

独立复核已确认实际Finalizer日志5645行row-2/row-4完整带精确ns、原值、protocol clock/epoch与实际reference身份unavailable，以及offset/drift/error/interval未知、mapping_status=unmapped_observation_only、完整limitations；该两条原生事实无预算省略。日志5561行另把命令相减的aggregate标签“固定偏移”置policy=hard、authority_ceiling=factual、outputs=summary/mechanism，5936行作为scalar aggregate/principal_answer交接。这是已证共享输入权限竞争，不仅是普通advisory解释。后续应区分来源可复核的算术值与已证实的物理映射关系，不扫问答原文设硬门，也不在未有复验时称模型随机波动；现有证据尚不能证明该输入是输出错误的唯一原因。

完整独立B审计确认primary/principal与sidecar对应文本一致，transcript与实际MD逐字相同（10061B）；HTML main 12736B含3表（附件、模型换算表、系统CPU频率表），无Mermaid。24条CPU频率驻留逐列与同源window_stats核对一致，范围[170441.3732,170441.3849)、CPU0..3、kHz/ms、起末源行与非线程运行/非因果边界保真；是背景补表，不证明时钟关系。本轮未选用原生时钟selector表，不能说“原生表正确即正文换算正确”。

14次trace_query调用/14回执，其中3次index_event_limit状态回退、11次请求完成，合计7唯一结果JSON与2fallback材料；同值查询共享payload但不同query_scope_id，保留而不混并身份。complete为1拒绝＋3受理，0降级，finalizer仅1次受理；中间旧offset算术笔误已被真实重算纠正，未误报为最终值错误。原gzip完整解码54,122,686B与准备文本逐字节相同。正确算术表在全表面均保原，但全程映射/准确度越权也保留；另有“同一次收尾操作”、clock_set_rate驱动行为和unknown解析分类被写为已知独立事件类型的次级解释欠项。

独立B报告 `/tmp/codrax-hmc280-B-manual-v1.md`，SHA `2bec97ae07d0a920260d7ee910d8e6e365c7dc9b864b697616d0e31acaceec39`；报告含全部14条参数/回执与7结果/9查询身份表、完整输入冲突及所有表面路径。详细报告和extra-artifacts清单已封存于本批verification-receipts-v1/process，原查询、准备、HTML及blob独立保留。没有第三次模型运行。

## 交付口径

完整能力新增0/累计24，生产子能力2组（受理解释整单元交接、原生时钟点见证），评测计数修复1组；两份整答欠项仍归原55开放。旧采样unknown→执行状态、权重→耗时错误不因本批未重跑而销账；二进制新格式/平台及只读登记未覆盖终态不代签。WAL未满足零副作用，生产NO-GO。

证据位置、冻结范围漏项的事后补充、正式全仓、下一ROI及发布收据见[批汇总](parallel_selected_summary_hmc_interpretation_clock_witness_20261022.md)。
