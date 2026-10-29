# HMC307：准备预算、执行适用域与调度范围交接

日期：2026-10-29（UTC批次日；本地2026-10-28）。基线`5942324675935c4b3bcb76c698dac8304fb91a48`，候选`codex/hmc299-wait-interval-candidate`，main保持`788588e365d40a48b30827ffd8db08dae4e2ae9d`。冻结后源码、测试、注释及helper停写；一次全仓、恰好两例各一次，无第三例，无main合并。

## 对账与交付口径

根代理及独立代理逐稳定ID核得79唯一＝25已交付＋54开放、18类，无重复。开放状态为19部分实施/29待实施/3待验收/2验收中/1持续执行。类别01至18开放数依次为`2,1,1,5,2,3,4,6,3,2,1,5,3,5,3,2,2,4`。纠正任务文档类别小结仍引用§228旧59项的陈旧数字，不改历史状态。

- 完整能力交付：本批新增0，累计25；main新增0。
- 已实施候选子能力：3组——共享准备最小包络预算、明确执行入口与源域、调度查询全窗/等待片段范围交接。
- 剩余验收：真实PCAP脚本执行尚未实现；Trace整答/图及同源投影身份仍失败；新入口自然覆盖及旧采样/IO/代表/权限欠项保留。只读登记仅原负向合同有限通过，不代销持续项18.5。

## 参考设计与实现

对照参考`core/query_engine.py`显式preprocess分派、`core/preprocess/context_index.py`完整原件导航、`thread_sched_ops.py`区间裁切，以及网络预处理的window_filter来源信息。保来源、执行与量测范围分别交接的意图；不复制目标零命中后回退全部、全量list或未证时钟映射。

准备预览保完整来源、原件大小、仅头验证/尚未查询状态、epoch边界与两条合法JSON导航；可选教学按原顺序整单元装入剩余字节。原640B消费者断言不改，真实不足仍返回typed required/available错误，路径/JSON/UTF-8不截断，原始包记录未查询不能报完成。

32个view容量行明确声明执行类别，目录和native tool/tracediag/indexed Run复用同一合同。packet仅native tool已实现，其余入口在读源前明确拒绝；新未知类别不默认获得权限。四个Trace流式恢复入口先检查源域，保已知Trace重视图合法降级，不静默把packet请求改成Trace。可枚举、可渲染、可执行分别记账，本批没有实现PCAP脚本适配。

SchedulerLatencyItem/RunnableContextSummary新增可选ContextScope：完整查询窗/行范围、半开等待区间、两组字段归属、忙闲可用性、其它CPU空闲cpu_ms单位。五种原始整窗值与JSON键保留；局部竞争仅用该等待的真实overlap或原精确约束。摘要和既有display-only notes分别带范围，旧无scope保unknown、clone隔离。仅精确逆转新字段恢复历史结构hash，不扩导入白名单或文本消费者。Trace投影、闭合睡眠、原四根因、优先级关系、自动补齐及合法计算均为保护项。

## 冻结前验证与前提审计

| 验证范围 | 结果 |
| --- | --- |
| traceinput/outputdump/attachment完整及race | 各196顶层/185子例PASS，1顶层/2子例SKIP；旧640B消费者公开RED→GREEN |
| types/tracefence/agent/context完整四包 | 7575顶层/7790子例PASS，1顶层SKIP，零FAIL；日志SHA `fd91ab1bcfb686d890b0a330d1876c2f88454fdeec16ac28c93bbaaf90854838` |
| tracequery/tracediag/tool完整三包 | 10498顶层/9746子例PASS，26顶层/12子例SKIP，零FAIL；日志SHA `08687ec70aa6fb45a8fe12877d1350f6d9fa1e6c0521b20b29f58e893bb4fd20` |
| 精确入口、恢复源域、结构演进及scheduler消费者/race | 正反公共路径PASS；旧view集合/数值预算/hash断言不放宽 |
| 真实工具→中英成文上下文新增两项 | 普通/race各2顶层2子例PASS；在四包完整测试启动后新增，由专测及冻结全仓分别覆盖，不冒称先前已运行 |
| helper自测 | 非法UTF-8/NUL、歧义请求、同代链接、旧2020封存与179快照身份校验PASS，零模型/生产查询 |

新增穿透测试初版把“所有背景行出现”当作要求：完整业务145行均保留到候选，背景排名80，32行预算合法省略。原始失败保留，不计为生产缺陷；负控确认原件/来源不变，精确目标单次公开查询自然入选后五种范围注释完整进入成文。后续负控又混淆有界摘要与原JSON，修正测试前提并保留失败日志，没有增加背景曝光。施工期其他公开RED、夹具错误及旧registry捕获新note失败也留原件，不冒称全程绿。

CLI仅构建一次，SHA`db48293eec86872517a3d7e2f5cdf40bae51c394fcf154d86b3877546a66a7c8`；orchestrator测试二进制仅构建一次，SHA`bf857c406ecd6ebd59eb3933038d5341ba6856d0d224e234d8389f90f1332962`。两构建输入一致，7470仓库＋66外部＋31仓外快照冻结SHA`21f420a295ffe50987e598c287914913f060c20a19a6e2426c49cc2f65a0168c`。旧2020证据包含179历史快照身份，不把179另加；18个原污染迁移身份仍核验旧址不存在。凭证文件内容未读/hash/复制。

## 两例并行真实评测

| 侧 | 固定范围 | UTC进程起止 | 机器 | 人工 |
| --- | --- | --- | --- | --- |
| A | 原OpenDocument自然时序问题、15步；人工编写ftrace，不称新设备采集 | 03:51:16.714883–03:57:19.843985 | PASS | 完整FAIL |
| B | 原缺来源终态只读登记负向合同，20分钟上限 | 03:51:16.712071–03:51:35.583174 | PASS | 限定PASS |

日期均2026-10-29。共同ready barrier；唯一可配对实际请求A16/B2，1歧义保留不猜配，最大已证请求重叠6.017秒。不是以进程重叠代替模型并行；没有第三例/替换题/期望数值注入。B的真实planner、操作员公共来源构造、脚本controller及独立进程公共run_tests边界在问题外记录，非自然controller/fullCLI。

B新proof确为verified，来源只有apply、无controller verify/VerifyRef/Completion，总体仍unverified/missing_terminal_verify_verdict；source旧真实测试报告未删除，不拿其历史结果冒充终态。新1断言结果/2命令，旧1/2，累计2/4为同断言两次执行；4 typed原件/1verify引用/1真实link逐字节和同代核对。readonly源码/测试/HEAD未变。context_pack_durability=false、apply_checkpoint_missing、来源定位及其它框架/完整CLI保持未验收。详见[人工审计](parallel_selected_summary_hmc_scheduler_registration_20261029_manual_audit.md)。

A最终恢复稿已正确区分50ms业务内5ms运行与52ms查询内7ms、31ms闭合等待与35ms请求驻留，保合法两段1ms小计、背景47ms及线程身份；但时序图仍未通过证据校验被隔离，正文存在端点/时长和相对量级错述。更重要的是系统投影把同一闭合等待列为两席31ms并显示全席64ms，与正文唯一席冲突，须按物理来源/请求/实例身份公开复现，不能仅凭数值相同合并。详细原件、同代关系及未触发范围以独立审计为准，机器PASS不签完整通过。

独立只读追踪将重复收敛到明确接缝：PID100选择的`676ac386…/a7ace14d…`与thread=app-main选择的`d54ca862…`原生board深比较相同，rank1对象SHA`6c4f9e5e4778705e15b8ae2500e55cf6bb1c88bcbf442b14d0f4d08f732f209c`。物理来源路径/2312B/修改代次、解析目标PID100/TGID100、查询窗、board fingerprint、worker200、block_rq/dev12,80/R/sector923339752/len64、issue/complete行9/11及闭合行10/12全一致。实际成文INIT却有两个candidate、同relation_member_ref `#1@1ff28327`和不同query row_identity。

静态路径：`tool/trace_query.go`保原始PID/thread selector到SourceRef，`types/trace_io_projection_identity.go`把原selector纳入发布身份，`trace_causal_projection.go`直接拿完整发布身份当IO实例去重键。原4项公开测试只用pid=100覆盖宽窗/反序/异源，未覆盖同代PID/名称选择别名；因此旧断言绿与本例失败并不冲突。私有generation指针未序列化，不能声称仅JSON就复原内部key；正式输出与源码路径共同支持下一公开反例方向。本批冻结后未运行额外查询/测试或修复，不把候选源代码引入时点断言为此历史接缝的起因。

## 唯一全仓与封存

正式全仓只运行`go test -json ./... -count=1 -timeout=30m`一次：UTC03:51:14.584561–04:02:44.517647，exit0；98包PASS/12无测试，28469顶层/33547子例PASS，43顶层/14子例SKIP，零FAIL。253970行全JSON，无重复终态或未结束；SHA`97a8dacebb9cd27e1ac2b9aad46f7316bc1f56729426dd29938d4b2e26247c86`。原640B失败消费者及新增成文两项在同次全仓通过。冻结7470＋66＋31输入前后不变；不二次全仓，未通过的整答不会因消费者转绿被回签。

288件本批原始证据已按真实路径＋SHA身份封存；[索引](results/hmc_scheduler_registration_20261029_v1/sealed-index-v1.json) SHA`177528cd39a6647aa4b99e2f98acadb9f49fe6ffdf4fff8d31c6eefd2d42104d`，[封存收据](results/hmc_scheduler_registration_20261029_v1/sealed-receipt-v1.json)保留两例与全仓的实际进程终态。旧2020件封存和179历史快照身份、7470＋66＋31冻结输入复核不变；封存不推导PASS。

候选代码提交`9f750f8f0d9b1fe7e33c2ed8eeaaacceb7f9f4a4`（20文件），五份文档相邻独立提交。最终普通推送、远端同代、clean、两提交冻结源码字节及main不变的独立证明见[发布收据](results/hmc_scheduler_registration_20261029_v1/publication-v1.json)。本收据仅证明候选发布完整性，不授main验收、不回签任何FAIL/未触发，不减少54项开放。

## 下一ROI

1. 缺陷轨先精确复现同源闭合等待在父/构成投影重复席位；量测原件独立保留与根因身份去重分开，保闭合睡眠/原4合同，不按31ms或单type拟合。
2. 能力轨完成06.1真实PCAP诊断脚本适配和原完整性/有界/时钟合同；本批正确拒绝不算真实接入。05.2校时和06.2会话独立，不作为无界追加门槛。
3. 之后轮换08.3/只读登记/sample-only既定验收，不让已知自由正文错误无限占两例；采样、IO、栈关系、代表与权限覆盖债继续原ID。
4. 17.6真实新格式、外部工具或原生宿主材料到位才提级；旧材料包装、stub和跨编译不代签。WAL零副作用未满足继续生产NO-GO。
