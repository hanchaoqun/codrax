# HMC284 双例人工审计

本批原自然题各一次并行，不追跑第三例。两例进程都exit0、运行前后冻结校验通过，不代表完整答案正确。A使用构造systrace；B四路径来自两份构造SQLite各复制两份，不冒称实机新增采集。人工结论与旧FAIL分开，不回签历史。

## A：IO在途自然题

进程PID63663，UTC2026-10-23 03:02:17.629398–03:05:57.375896。机器FAIL：原checker缺少根因/阻塞限制表述的正则命中；该离线checker不是生产关键词硬门。人工依据是实际含义而非regex：原生四组24数正确，正文仍从仅已配对的在途统计外推“设备不忙”“没有任何一组出现请求排队”“没有请求堆积”，且把窗内发起6次与含窗前起点的4对相交配对混作同一总体。因此08.3完整退出未满足，人工FAIL，不能以附表正确抵销正文。

四组期望（峰值/全窗均值/有在途时长ms/请求·ms面积/窗内发起）：RQ 8,0读＝2/1.4/10/14/6；RQ 8,0写＝1/0.6/6/6/2；BIO 8,0读＝1/1/10/10/1；RQ 8,1读＝1/0.4/4/4/1。跨层计数、物理硬件状态、调度/线程等待与根因不是这些量测直接支持的结论，合法同组计算仍允许。

实际Finalizer保留精确来源/窗口、配对总体与窗内发起、单位和非因果边界；同时投递调查解释单元0/1，单元0仍声称设备不忙、队列没有堆积，终稿复述。单元2/3因998B预览裁剪未投递，不能把其中D/iowait误述说成最终上下文或正文证据。Finalizer内部估算87706 tokens、Explorer峰值99500，不是provider计费或溢出证明。无语义质量reviewer调度。

唯一一次emit即受理，新原生ID/数组形状修补NOT_TRIGGERED；完成门走旧attached-trace旁路，新引文适用域区分未由A证明。13段时序正确；本例无图，根因合同未激活。HTML正文/四原生表逐项读核，不等于像素级布局通过。完整依据见[封存独立A审计](results/hmc_citation_repair_representative_20261022_v1/verification-receipts-v1/process/codrax-hmc284-A-manual-v1.md)。

## B：代表选择自然题

进程PID63661，UTC2026-10-23 03:02:17.629420–03:10:02.200976。普通CLI没有独立机器checker。原验收仍要求四来源×两个线程/共同窗口的8个精确资源查询规格、2组各2代表、真实执行及4个未选成员。最终8个目标查询已全部真实执行（6success/2empty），原生统计正确；后续已修正初始错域，不能用早期appid=线程号或16未执行计划冒称最终漏查。

完整人工FAIL、11.2代表选择NOT_TRIGGERED：没有representatives或成功representative_status，没有选择快照，未运行需要真实同次status路径/SHA的post helper，不能人工补快照。终稿自称5类每类2代表，按结果画像拆组并引入探索0–100秒宽窗，同线程/窗口组成员重叠。正确原生8目标取数仅是子能力，不替代代表选择。

实际目录和Finalizer均明确35查询记录＝28声明计划＋7额外观察记录，19真实尝试＝17success＋2empty，另16not_executed。终稿误将7个观察记录说成总执行数；a宽窗pattern查询matched_total=19误用observed_count=21，并与b无pattern、d有pattern查询相比较。开头把仅a/c成立的线程行为外推所有来源，b/d空202还套用了a/c的观测名称。源局部身份、筛选条件和独立分母未保持。以上足够判原完整矩阵FAIL，不穷举同类措辞。

多目标诊断已在本次真实结果中自然命中，查询继续正常执行、未自动缩成单线程；实际日志2846–2847亦命中新完成标签`native_runtime_source_citation_not_applicable:observations=69`。没有新原生引用/数组形状诊断命中；首次两个runtime_measurement块载体不合法的拒绝、第二次完整重交成功属于既有结构修补，不能冒充新分支覆盖。最终引文门新增分支的独立自然判别仍未证明。

调查中间reason曾写“8组”，但该说法未出现在实际Finalizer INIT，不能认定它造成终稿冲突。完整原生数据、筛选条件及精确执行账已进入成文上下文。Finalizer首轮内部估算123024 tokens、修补轮129896，Explorer峰值120666；实际INIT原日志区间423367B，便读副本仅多1末尾换行，源offset/两侧SHA独立登记，不冒称完整provider wire。完整读核Markdown256行；HTML核对同源错误文案、15表及来源存在，未逐单元格或像素级验收，无Mermaid。详见[封存独立B审计](results/hmc_citation_repair_representative_20261022_v1/verification-receipts-v1/process/codrax-hmc284-B-manual-v1.md)。

## 原件与审计边界

本批结果根：`eval/results/hmc_citation_repair_representative_20261022_v1`。249文件已按[索引](results/hmc_citation_repair_representative_20261022_v1/verification-receipts-v1/artifact-sha256-v1.json)封存（SHA`65874abfe50abb4dc43a02a61842a5334c48eeb656970995cd825b355cfdd4d5`）；独立审计、全部未通过payload、真实上下文、原生结果、主答案/MD/HTML保留。足以判FAIL后停止扩找同类误述，未核实表面不扩签PASS。完整人工0/2，完整能力新增0，共享子能力3组；两组整答欠项含于54开放，覆盖欠项另记而非新建重复任务。

旧IO解释FAIL、采样越权与计算权限自然覆盖欠项继续留账；WAL不满足零副作用，生产NO-GO。
