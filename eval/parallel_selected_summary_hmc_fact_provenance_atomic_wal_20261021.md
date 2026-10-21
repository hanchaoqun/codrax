# HMC276：工具输出来源分层、解释原子预算与 WAL 生命周期

基线 `69d6afa0f79d8223176345c9f67073e5d84bffc8`。双侧逐稳定 checkbox 复核：79 唯一、0 重复＝24 已交付＋55 开放；33 待实施、15 部分实施、3 待验收、3 验收中、1 持续执行。两个共享交接修复归 01.3/16.4/18.4，WAL 原型归 17.7，不新建重复父项，不回签历史人工 FAIL。

## 两组共享生产子能力

HMC275 A 的普通四次 awk 试算分别声称同窗 busy=0/5/7/10，以固定 confidence 0.80 平铺进 Known Facts；当时 typed command_measurement_records=0。原生时序及尾段本身已经正确。本批修复真实来源交接，不再改正确统计器。

- 主/子 Explorer 在真实成功 ToolResult 生产 RepoFact 时保私有、不可变的 Key/Value/Source/EvidenceRef 关联；显示实际原件引用、工具、时间及 typed origin。共享投影不再把类别 confidence 当事实正确率。四份尝试及合法 count/VCS/源码/log 仍保留，原始 ToolResult 不改写。
- 来源未知、字段变动、JSON 恢复明确是未关联旧说明；不凭同标签/同文本重接。复制/fork 保不可变关联，getter 防御性复制。同一精确 ID 的 producer/carrier 共用既有 seen 去重，不合并不同原件。typed grounding policy 不贴在自由摘要旁，避免 `hard` 标签误升权。
- 独立解释新增可选 `interpretation_units`，每项 text、boundaries、support_refs 按完整 JSON 整组计费、整组展示或整组省略；引用只定位提议支持，不证明事实或因果。原 900/1800B 模型内容预算不扩，固定省略说明单列，不宣称完整。
- units 存在时旧 interpretation 仅留审计，不重复成文；缺新字段不增加重试。旧短文本兼容，长文本只选完整段落并明确仍可能缺关键限制，不按关键词挑句。纯原生复述角色、当前受理/来源重绑及恢复权限隔离不变。
- schema、工具教学、阶段交接和 Clone/恢复同步；units 只教一次详细形状。真正安全过滤导致内容变化时整项省略，但阶段工具名展示替换和首尾/段间空白规范不能误删合法业务线索及原生引用。原始 JSON 字节保留且计入预算。

这两组修复不删除合法分析、不添加问答关键词硬门、不扩大默认展示或图义务，不改变 Trace 因果投影、链上原因资格、源码责任、自动补齐或工具成功标准。公开实际 Finalizer 回归覆盖四份冲突试算；解释测试覆盖实际 Finalizer 文本和 Extractor handoff renderer，不冒称完整 Extractor agent 执行。

## WAL：生命周期证据与明确 NO-GO

仅两个 Darwin `_test.go`。新增持锁准入后 owner 退出、pin 后 TRUNCATE/RESTART busy、helper 后 checkpoint 实际成功和锁释放、一页后取消无发布。held FD 观察避免旁路 close 释放 writer 进程级锁。

发现真实反证：owner 在 pin 后关闭，最后 readonly SHM munmap 仍改变 ctime，字节/mtime/inode/owner 相同不等于完整强代次不变。原始失败、三次复现和 munmap 紧邻 fstat 探针均保留。原型终态新增完整代次检查，变化则返回错误、清空路径并删除本次私有备份；**拒绝发布不撤销已经发生的副作用**。反证测试 PASS 不是能力 PASS。

生产仍 NO-GO，17.7 保持开放。原子三文件准入/ABA、owner/SHM 生命周期、强杀回收、其它平台/owner/长期压力及正式凭证/共享准备入口未交付。后续不能继续在已证不满足零副作用的方案上无限追加场景，应先完成安全方案决策。

## 对照参考仓

实际重读 `core/query_engine.py` 的参数合并、完整 fetch、展示裁剪和 metadata 白名单，以及 `core/db_pool.py` 全文。可借鉴完整结果与展示投影分离、准备一次复用统一查询；不能照搬省掉查询坐标后以摘要猜来源，也不能把 path+mtime LRU、源库 CREATE INDEX/commit 当严格只读或强身份基础。参考仓解决查询性能和展示成本，本项目还需跨源/窗口/代次/成员证据绑定。

## 验证与冻结

所有 RED 和夹具失败保原。普通工具来源公开 RED→GREEN；同 ID 双 carrier 展示计数另有 RED→GREEN。解释新增字段前置 RED 不冒称客户事故；真实旧摘要风险来自 HMC275 B 原件。过程中 runtime-only 测试前提、wire selector 误传 Columns、旧长文本前缀预期分别修正，未放宽产品门。独立审查发现的合法工具引用/空白误丢均有公开 RED 和修后 GREEN。

Known Facts 最终相关 41 顶层/21 子例、窄 race 7/9；context 全包 414/251 通过。解释相邻 types/tool/agent/skill 与窄 race 通过；最后空白修复正反和 race 另有正式收据。WAL 五根十四叶及 race 通过，其中包含独立 NO-GO 反证。三轨独立静态复核无冻结 blocker。

`make` 与真实恢复测试 binary 构建成功。7229 输入冻结，索引 SHA `67480e2a4de07b78a23eeb17f6bff4816a2c0ec06b54f336fb205ed9ef327e0b`；CLI SHA `ee48a296ceccad8ad216887d0ed6734f9a421ad24fefe3f9e769dff36b62533b`；测试 binary SHA `ffbe15ffee9c1a732a05fef78f18ea09f4dfd14baa17c1085dc0528fb0a68031`。

## 两例真实评测与交付状态

冻结后恰好两例各一次并行：A 原自然 `trace_io_inflight_overview`，B `TestNativeRegistrationLiveNaturalControllerTerminal`。B 是真实父 controller/planner 与独立恢复子 controller，源码/旧验证由 operator 公共夹具准备，实际测试由公开 RunTests 执行；明确 `full_cli_run=false`，不冒称从零 CLI 或模型源码编写。

两例机器 **2/2 PASS**，完整人工 **1/2 PASS**。A 四组原生数值、成员和全部13段时序正确，写末1ms保留；正文却无映射断言RQ/BIO同一物理IO、跨层相加，并从部分配对人口推断设备不过载，整答FAIL。原生证据及限制已经充足，不把全部错误归因于缺数据或预算。B 真实父流程/恢复子流程/新执行后终态人工PASS：当前1 assertion/1 pass/2 commands、累计有效2 reports/2 results/4 commands；本轮实际runner尝试3次，中间失败未隐藏，不将归档文件数当执行次数。完整过程、MD/HTML源、旁路及未触发范围见[人工审计](parallel_selected_summary_hmc_fact_provenance_atomic_wal_20261021_manual_audit.md)。

A 新Known Facts来源展示自然触发，普通四次awk冲突分支未触发；解释单元真实生成并受理，但没有进入实际成文预算路径，不能签自然原子预算通过。两次因仍要求旧interpretation键而发生修复，作为共享JSON存在性合同摩擦继续登记。已有runtime-only叙述抑制不应为了覆盖率而关闭。B不覆盖read成文、Trace/WAL或sample-only。旧sample-only解释越权继续保FAIL，本批轮换只读登记不能替它销账；不追加第三例追绿。

全仓PID58084，2026-10-21 UTC09:10:13.902995–09:23:31.819250，正式exit0：**96包PASS、13包无测试、零FAIL/非JSON行**。日志SHA `b8efb507831a4a9ccf92b88ec000839738131c2d384ea027d91067ac3a034967`。A PID58085正式exit0（09:10:13.902994–09:17:21.519093），日志SHA `baffdc665fcd8c382f8bf1cc3ece21b33e4074917c5f883c2b2d01121a370ac6`；B PID58083正式exit0（09:10:13.902988–09:12:18.447108），日志SHA `69bce8593a3062b59c1818b8b7daf75be5208c35ae72a561e071669d5ab732e4`。没有全仓失败后改生产或追加自然评测。

完整能力新增 **0（累计24）**，共享生产子能力 **2组**，test-only生命周期/拒绝保护 **1组**；本批整答剩余验收缺陷 **1组（A）**，历史FAIL独立保留。55开放不减少，子缺陷和验收项不与55重复相加。源代码、文档分笔普通推送，最终发布收据独立于封存证据。

## 下一批ROI顺序与未完成边界

1. 缺陷轨先收敛两种解释载体的JSON存在性合同及共享成文事实/解释消费：原生事实已有完整人口、单位与边界，避免重新自由抄写时越权；以结构化来源/关系资格和真实消费者做公开反例，不加IO专用问答关键词门。原有runtime-only保护与独立业务/源码解释分别覆盖。
2. 能力轨推进17.6尚缺的真实格式/版本/外部工具/原生平台证据，轮换sample-only终稿边界；普通ZIP/gzip运输、尾部读取和只读登记已交付部分不重复施工。缺真实样本/平台时明确前置，不以包装或交叉编译代签。
3. 17.7先做安全方案决策：当前WAL原型已证明不能满足源零副作用，禁止继续接生产或无限增加同类探针。三文件准入/ABA、owner生命周期、强杀回收和正式捕获身份仍留账。来源/权限硬门保持。

所有新增观察归现有01.3/16.4/17.6/17.7/18.3/18.4，不以新增重复父项或放宽退出条件改变55口径。

## 证据封存与分笔发布

2026-10-21 UTC09:28:34.278414封存 **196份文件**，保留原结果目录50原件，复制146份（含30源码快照），另按本轮明确引用验证87条运行时材料路径。索引SHA `710a7a66f8330508d33d6e4d5113810a45652d7e26106a162de4ca0ec9060c1f`，归档收据SHA `b4da2ac03477acd54e05f89b9690ea32de599ee467282e27d4b3a817e9d4f8ab`；索引和收据不递归包含自身，后续发布收据单列，不回写已封存文件。

- [正式归档收据](results/hmc_fact_provenance_atomic_wal_20261021/verification-receipts-v1/archive-v1.json)
- [逐件SHA索引](results/hmc_fact_provenance_atomic_wal_20261021/verification-receipts-v1/artifact-sha256-v1.json)
- [最终发布凭据](results/hmc_fact_provenance_atomic_wal_20261021/publication-receipts-v1/publication-v1.json)：代码/文档commit与普通push独立日志、远端main/本地HEAD一致和clean核验；不改变人工结论。

源码提交 `c5352e214d664f00224e80f772e1527638a15b42`；本文件、人工审计及两份统一账本另笔文档提交。实际发布结果以上述独立凭据为准，生产实现、test-only原型及人工验收仍分开计数。
