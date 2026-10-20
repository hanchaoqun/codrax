# HMC268：真实gzip文本自然入口与原始事实摘要复用

日期2026-10-19（本地，过程收据UTC）。基线`fd86ce97a8640474720c452389bc5e8410f2f70f`；工作区起点干净、实际远端一致。稳定任务复核79唯一＝24已交付＋55开放、重复0。本批仅两轨，旧采样解释越权及§267两份完整人工FAIL不预销。

## 固定验收范围

A：`trace_gzip_named_text`，普通名称`capture.data`承载参考仓真实gzip ftrace的精确字节副本，无预附/预准备。问题只问170438.0–170438.1秒SaInit0 tid3051的状态、时长及唤醒者，不向模型塞保护规则或oracle。原件3,602,086B、解压54,122,686B；独立新扫描429756事件/429766行，完整解压SHA`f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572`。原行独立积分S96.780ms/runnable1.088ms/running2.132ms、8唤醒，精确坐标/边界/逐区间另存oracle。所有数值仍需与原生工具和最终答案交叉核验，不改oracle追随产品。

B：`trace_dual_capture_metadata`，既有双侧SQLite测试原件，不冒称真实设备采集。沿用自然问题并排查看原始量测、时间和采集元数据；SQL oracle与用户问题分离。旧量测表正确却正文单位/身份越权的FAIL继续复验，summary/无边事实图若自然未选择就记NOT_TRIGGERED，不追第三例。

## 两轨设计与边界

gzip只有内容候选导航改变：复用现有结构化首数据行语法，压缩实际消费及解压前缀均有界；完整准备保CRC/尾部/单member/代次/取消。未知文本或预算内未识别不等于格式不支持，现有显式准备不被撤销。不新造decoder、不提高250k事件索引上限。

raw量测和metadata复用已有producer表格到摘要/无边图，不让模型重抄原值或自证单位/物理身份。来源/代次凭证与投影字段完整性分开，private display seal不新增来源、因果或完成权限；缺失/invalid/parser字符串与真正协议已验证单位分别保留。参考指标中的Hz/running/NULL延长属于具体协议，不按名字移植到任意raw字段。

公开反例先确认两类边界：普通名称gzip文本不能提供原生导航；raw/metadata表格本来可用，但不能作为selector-only原生摘要/无边图。后者是能力扩展，不把旧表格宣称为坏数据。实施中又验证原件当前凭证不能替投影内容完整性：三类raw producer在已验证数据上预封完整稳定record及表格；只有已有producer最后来源装饰点可以在稳定字段未变时封最终record。当前来源、窗口、代次、本轮范围校验仍独立执行；JSON、memo、改行/注释/单位/状态不能重建新展示权限。metadata/逐侧取数状态由当前私有builder封表，同一query ID的等价副本只合并展示资格，不合并异源查询或完成次数。

教学消除纯事实与额外业务解释的冲突，不再让bounded_fact_set必写效果结论；真实效果判断、关系、源码义务及链上根因候选维持原合同。原生summary/事实图不承载模型text/items/columns/关系边/根因标签，已有原生表渲染及当前来源重绑定统一负责全部字段。没有改问答关键词硬门或削弱真正协议Hz正控。

独立审查发现的状态旁路也先取得公开RED：真实catalog和current查询成功后，仅改raw observation的Object为empty，原先新状态表就会同时显示empty和success=1。最小修复使raw producer状态也必须匹配同一私有投影封存；失效标unavailable且不覆盖同ID有效结果，真实执行历史仍保留，不把失败或未执行改写为零。该反例证明共享交接风险，不冒称模型能够直接改内部ToolResult或已有真实事故。

参考差异：`server.py:133–195`的候选发现、`core/query_engine.py:370–499`的结构化summary/unit/empty/truncated保留值得复用；`gpu/freq.yaml`的Hz与running必须有指标协议才能沿用。`page_cache_parser.py:46–76`专属事件、float秒换算、PID别名与固定页大小属于局部parser，不复制到通用导航；本项目采用共享锚定首行语法和既有完整prepare，保持Trace原时标。

## 确定性验证与冻结

gzip定向及race各129根PASS/4项opt-in SKIP；真实原件另跑2根完整解压与底层窗口查询PASS，不能据此代签公开工具的自动窗口选择。raw展示直接旧消费者147文件/808根普通模式PASS；末版状态接缝race、相邻三包与独立17根race PASS。旧版533根agent竞态闭包触发Go默认10分钟包超时，原panic/堆栈保留，未观察到此前断言失败或race诊断，但此分支不记PASS。末版仅教学澄清的三包3根回归PASS：summary/无边图不会抑制默认完整表，不把当前行为宣传为跨面去重已完成。首次新增测试的编译/夹具假设错误与真正产品RED分列保留。

最终构建PID32052 exit0；CLI SHA`523723a666b620faf80cc8a4a857c20453f72ee9d434b5df9906932de2335069`。UTC2026-10-20T02:44:30.553228Z冻结7170输入，SHA`ebd7943fea38e48290945d60034a971e037c9d9dac9e9702d516a5bcbaf80453`。之后源码、测试、案例、问题和CLI不变；同冻结一次全仓与下列两例并行启动，不追第三例。

## 正式真实运行与人工结论

| 案例 | 正式父进程/UTC区间 | 机器 | 完整人工 | 限定结果与欠项 |
| --- | --- | --- | --- | --- |
| A gzip文本 | PID34250，02:44:37.652933–02:46:38.558399 | PASS | FAIL | 普通命名自然导航、完整文本准备、最终原生状态/唤醒及显式窗补齐通过；唤醒被写成直接运行、FFRT名字被升为调度控制 |
| B 双侧量测 | PID34251，02:44:37.652906–02:47:41.067351 | PASS | FAIL | 原生13/8行正确；模型把两侧量测标题与正文对调，异常REAL60仍写fps、parser标签仍升物理设备 |

日期均为UTC2026-10-20。A正式日志SHA`f94c62c49092dd8c21b268063c80fe9360f17854bb4434d833ff11bec4e8feb6`；B`1e43e108bc1c5fa7b11ec1368664a8c012f459e4286be41a6095811ea65fee42`。exit0及机器判定不替代人工语义验收。

A首次公开工具在54MB/429756事件源上选择整库索引，触发250k上限；`traceQueryBuildIndex`的文件大小分支让合法显式窗未进入现有有界窗口恢复。实际流式回退继续取数，首个thread_timeline报告尾部33µs未覆盖、S96.747ms/总99.967ms；后续wakeup/interaction/系统补齐得到S96.780ms/总100ms，最终采用正确值。后者与独立oracle一致不抹前者差异；差异原因未隔离，不称所有查询完全一致或直接归缓存故障。正文同时有独立runnable1.088ms，却说每次唤醒直接切回运行，故整答仍FAIL。

B最终原生表内部路径/值仍正确，错配发生在实际emit selector与模型自由标题：current表被标baseline，baseline表被标current，后续正文计数也对调。实际工具pair角色和finalizer可选表清单正确；本轮分析合同只有共同时间窗，未发Comparison，不能凭查询角色反向证明已接受的用户侧义务。下一步应统一当前来源、已接受比较侧和展示标题的结构化绑定，不扫描标题/答案关键词，也不因数值相等猜归属。失效REAL与parser标签已到最终上下文，不能把正文越权归解码漏数或未经隔离就说纯模型波动。实际accepted仅两个native members表，新native summary/metadata summary/无边图均NOT_TRIGGERED；也没有跨面默认重复，不用普通kind=summary冒签新选择器已自然启用。

## 剩余数量与下一ROI

完整稳定能力新增0；实现子能力2组（gzip文本自然候选/共享完整准备接入；raw/metadata/status同源摘要与无边事实图，含状态封存修复）。完整人工缺陷2组仍属01.3/16.4/18.3，原79唯一/24已交付/55开放不变；旧采样解释FAIL没有本批反向通过证据。

下一优先：①小而密集Trace的typed索引预算错误→合法显式窗共享恢复，并明确尾部覆盖交接；②原生双侧身份、量尺与展示块同源绑定，保真正协议单位及独立查询侧状态；③精确完整原生summary/图对默认表的覆盖，避免重复但不漏明细/关系义务。先修前两项已证高影响接缝，不为每句越权堆关键词提示。更多格式/真实外部工具/平台、无窗及坏源边界仍各自留账。

## 全仓、封存与发布

同冻结一次全仓PID34249，UTC2026-10-20T02:44:37.652854Z–02:59:34.296076Z，正式exit0：96包PASS/13无测试包SKIP，无失败或非JSON行。日志SHA`02c5d7ef7110ebb9ed0190be68bccd056da87711f7d2d415a7c4f7aa9647df43`。19个本批新根中18PASS、1真实样本opt-in SKIP；该样本已单独完整执行通过，不将全仓SKIP改签PASS。最后包打印PASS后仍等待总进程退出，不以日志片段代收据。

UTC03:00:33.322245Z一次归档成功。30份原始运行材料逐字节未变，复制127份过程/公开失败/定向回归/审计/只读设计，再加实际CLI、真实gzip与77份.codrax原件，共206新增复制；236文件索引SHA`32ef86293960f2b1e62c2e76935151fc0aee5033c0f27108e09f353cdf03261a`，归档收据SHA`8bb2d6c8c00fbe30319e635f7e3e8e6200d730ad1f5f103eb13836214d07f278`独立避免循环。输入7170/实际CLI/源原件及自然问题再验证一致，未复制provider凭据或重跑模型。

结果根`eval/results/hmc_gzip_text_fact_summary_20261019`；正式索引`verification-receipts-v1/artifact-sha256-v1.json`。A/B独立人工审计SHA分别`53700a5233fe315c512ea0a7dbcc3a0c644945b07c8d360ab738b143a4dadda2`、`8785189f380297e544a9eddea56ca7cc33233b1303e2f444b38fdaf5e7314be6`，每个失败和未触发维度另见[人工审计汇总](parallel_selected_summary_hmc_gzip_text_fact_summary_20261019_manual_audit.md)。下一侧绑定只读方案SHA`e4b87dbbbabccfea52831c4c7e6a483c10f3afbf78fb1add7b3ff5c6f983bd3e`，不是本批实现。

代码提交`35a6acb7982b072bf1686380ac01059d2cf74456`已普通推送，push PID27314，UTC03:01:23.097266–03:01:29.903119，exit0，日志SHA`5329cb9579cc83d64443a11f23af8592fec1319b04d3330389a3a1ea8b5b50ef`；文档另笔提交。两笔正常推送及实际远端/clean终态另存`publication-receipts-v1/publication-v1.json`，不重写正式封存索引。完整能力新增0、子能力2组、人工欠项2组，55开放保持。
