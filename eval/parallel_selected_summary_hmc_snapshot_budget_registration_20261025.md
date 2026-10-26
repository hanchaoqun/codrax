# HMC292：代表选择同代交付与恢复预算原子准入

日期：2026-10-25（本地）；基线 `27ea317080a7a32b52afcba0d2ecb83bcfdefe61`。独立核对79个唯一任务、25项完整实现交付、54项开放，18类；31待实施/17部分实施/3待验收/2验收中/1持续执行。完整能力本批新增 **0**，共享生产子能力 **2组**。A完整人工FAIL，B限定只读登记流程人工PASS；不能写成两份完整CLI或整个父能力通过。

## 本批交付与参考适用域

1. **代表快照、索引及页面同代交付**：同一次detached发布合并视图生成三者；`saved_index`为内容寻址不可变文件，页面及代表快照绑定完整索引SHA，旧可变导航路径另称`latest_index_alias`。复用既有`immutablefile.Publish`，同字节不重写物理代次；失败不返回成功收据，但不宣称跨文件事务。合并只用于交付，不回灌执行fork或从历史JSON恢复权限。普通页与代表页共用导航编译：本地精确ID用原ref，跨分支新见查询复核完整参数、当前目录准入及保存计划与来源元数据的身份一致性后返回普通源查询；实际源代次/可读性由普通真实查询重新准入，执行后才登记。合法重试、未选查询与旧代原件保留。
2. **恢复预算来源、计费与提交前准入**：通过既有身份/显式恢复门后，缺省上限沿宿主正常seed默认5补齐，已有正限包括1不改；模型`workflow.max_batches`只作规划估算。展示和准入共用`max(唯一非空批ID数, 历史已用下限)`，名册裁剪不退历史消费。新ID在变更前检查，已有ID合法重试不多扣槽。普通决策及累计补证都先验候选，typed超限保原run，再记录真实阻断，不先切active/清handoff/留下未执行verifying批。独立无预算reducer的旧无上限语义保留。

参考仓 `core/batch/closure/select.py` 的纯、有界选择保原成员；`package.py` 一次读取manifest再生成交付；`merge.py` 先校验再写；`engine.py` 按宿主CPU/内存约束worker并发预算（不是本项目durable批数实现），成功缓存与失败重试分开；`query_engine.py` 保真实参数、原总体和预览分层。本项目借这些设计意图修共享交接，未照搬可覆写文件、历史成功缓存为本轮执行权限。`runner.py`及`test_batch_resume.py`另有事后SHA登记，不冒充启动前冻结。

未改Trace因果投影、自动补齐、原查询总体、来源/证明准入，也未加问答关键词硬门。普通显式单文件读取原本可接受RepoRoot外输入，目录遍历准入不是该路径的全局读权限；没有为通过负控而收紧正常工具。

## 同冻结双例，各一次

冻结 **7364** 源/测试输入及24外部输入，SHA `cac5851f35accec758e412316282069fe449012181ae2e2a3aa80c717b96c6aa`。标准CLI SHA `b3beba36396ce322fa1d0a8e2acb1168511b0bba45d334a936423bad2fd3fe0b`；B测试二进制SHA `9c02422b81d3b87a81df1a8fe439aa3dc9cc1bcf6801483a6a3f46019524a872`。A/B实际exec UTC03:50:15.678241/.709494；首个analyzer/controller真实请求至少重叠 **1.358秒**，不是只凭进程存活。前后冻结核验一致，无第三次模型或改题重跑。

| 例 | 实际覆盖 | 验收结论 |
| --- | --- | --- |
| A 自然代表选择标准CLI | 原题、4个构造SQLite、8计划/2规格组/4选/4未选；4次原生查询，3success/1empty；自然入口及正确取数命中 | 进程exit0不等于机器语义PASS。没有`representative_status`，目标同次终态三元组自然验收NOT_TRIGGERED；正文分组数与原生事实冲突、深度升调用关系，完整人工FAIL |
| B 自然父controller/planner＋新进程恢复子controller | 父controller5次、planner2次、子controller2次真实模型请求；真实来源验证、登记、重新执行、持久化终态；预算1/5→2/5→3/5 | 机器exit0、限定流程人工PASS。不是完整CLI、自然源码创作或全部源码交付验收；中间缺证尝试保留 |

A两个已有SQLite内容复制成4个独立文件，不是4台设备/新二进制格式。选中a/b，c/d未选且未执行；不固定某个合法平分排序结果。初始索引/选择对仍可核且未被之后4次查询改写，但初始pending快照不替代最终代表状态。未补叫状态工具、未从未返回文件拼装三元组，也未运行离线助手补造覆盖。完整239行MD与HTML、实际原生载荷/成文上下文及旁路详见[人工审计](parallel_selected_summary_hmc_snapshot_budget_registration_20261025_manual_audit.md)。

B原源码修复和历史测试是operator公开工具夹具，实际模型负责本轮父controller、planner和恢复子；原生执行采用真实RunTests/verifyPostHook，不能称模型自主源码创作。当前1断言/2命令与历史invocation分开；中间补证仍保`verification_proof_incomplete`，后续精确证明依赖将其履行，并未覆写失败。最终3批verified、3/5；父/子各保存7件报告原件及3个verify引用，不等于14次执行。五份上下文各6个逻辑包，child_memory的active合并包ID经Store规范化改变，后4份持久ID稳定，不能写五份所有ID相同。

## 验证与未销账

公开RED保留同次索引/选择不同及后续改写、跨分支导航WrongReference、模型预算接管及先落批后超限。最终相关：发布4包74顶层/120子例、预算4包21顶层/8子例、完整writeflow214顶层/154子例PASS；各自race及标准make通过。新测试构造曾错误假定RawMessage空白、macOS路径别名或RepoRoot外单文件权限，已纠正且原失败保留，不冒充产品反例。旧proof身份测试只将已占用单批计数0迁移到1，精确身份断言未放宽。

唯一全仓PID91602，UTC03:50:10.836477–04:05:39.997329，`go test -json ./... -count=1 -timeout=30m`正式exit0：**97包PASS/12无测试/零FAIL**，28319顶层/33022子例PASS、41顶层/14子例SKIP。日志SHA `4b71cf28145ebd0421e030c6b840156e8b51744776febfa49c6bc19f5816bce7`；7364输入前后相同，没有后继生产修改、第二轮全仓或额外模型。

剩余验收分为 **3组**，均含于54项：①11.2目标状态入口/最终同代三元组未自然触发；②16.4/18.4正文组数、深度关系和调查摘要同源一致性FAIL；③18.5只读登记更宽交付边界及旧探索预算展示（本例仍见1/0，未被本次批数预算修复覆盖）。其中B当前报告明确`apply_checkpoint_missing`及源码定位weak/observed_only；不由限定PASS升成完整源码/物化交付。

下一ROI：先复用当前执行账提供准确最终代表交付，再统一原生统计组/计划规格组及栈记录的解释来源，不重写已正确取数器、不扫描正文作硬门。只读登记下轮轮换未覆盖负向/交付范围；17.6有新真实格式、可运行工具、原生宿主材料即提级，不重复包装旧SQLite。旧采样越权、IO解释失败及计算权限自然覆盖缺口继续留账；05.2/05.3校准及14.1协议保材料前提，17.7活跃WAL仍 **生产NO-GO**、无接线。

## 原件封存与发布

**247件**原件已封存，含全部原始失败、两侧问题/准备/oracle、真实日志/实际上下文/原生载荷、最终MD/HTML/旁路或typed报告、18个源码/测试快照、实际CLI/test binary及独立审阅；索引SHA `2eb7c66e9dae89f99bed7c17eba1231129c057c78787ddee30058de4f9b69630`。本地[原件索引](results/hmc_snapshot_budget_registration_20261025_v1/artifact-sha256-v1.json)、[封存凭证](results/hmc_snapshot_budget_registration_20261025_v1/archive-v1.json)；完整文件按绝对源路径镜像于sealed-v1，不以摘要替代原载荷。providers凭证内容未读取/归档；此运行目录按项目规则忽略，不推送敏感载荷。

代码`a22fbce8ce0bc6f46abb6040ba661d10ce66d604`，5份文档另笔普通提交推送；最终远端main/clean、两提交树7364输入逐SHA及247原件复核以[发布凭证](results/hmc_snapshot_budget_registration_20261025_v1/publication-v1.json)为准。独立实现/文档/冻结/全仓审阅无剩余阻断，不改变A完整FAIL、终态分支未触发或B限定PASS，不回签旧失败。
