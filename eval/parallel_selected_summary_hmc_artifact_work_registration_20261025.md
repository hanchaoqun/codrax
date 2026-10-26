# HMC291：代表工作规格交接与只读登记轮换

日期：2026-10-25（本地）；基线 `f883098394d7d9aae8e468fc02433ddef9719d0c`。79 个唯一任务、0 重复，25 完整交付＋54 开放，18 类；31 待实施/17 部分实施/3 待验收/2 验收中/1 持续。完整能力本批新增 **0**，共享生产子能力 **1 组**；本批完整人工 **0/2**，不得用入口命中或原生取数通过冲减开放数。

## 交付范围与参考设计

参考 `HarmonyOS_PerfMcpServer-main/core/batch/closure/select.py` 的稳定有界选择与完整成员保留；其输入已是 manifest/cluster，不能照搬为本项目的自然问题入口或当前执行凭证。参考 `core/query_engine.py` 的真实参数、原查询总体与预览分离。当前缺口在选择器之前，不重写已正确的排名/统计器。

- 可选 `runtime_artifact_work_profile` 独立承载 representative/exhaustive 及每组预算；schema、解析/修补、受理、教学、RequestModel 和 fork 复制同步。仅逐字锚定当前请求的引文，不扫关键词或要求 ASCII 数字，不借答案枚举数、源码选择或因果义务表达采集工作。
- 模型显式调用 `trace_catalog discover request_plan:{view:...}` 后，受理目标、唯一共同时间窗、显式普通 view 与授权目录贯通既有计划登记/代表选择/原生查询。原预算大于工具上限仍保原值并 pending；不悄悄截断、选首目标、猜默认窗或把探索游标当用户目标。
- thread/process 保真实作用域，process 能力判断与普通查询共享。缺声明字段、目标冲突、多窗/比较歧义及已检测的不支持process/view组合保持 pending，不发布前缀计划。普通 discover 不暗选代表；未选查询、重试、原查询总体、Trace 因果投影/自动补齐保持。
- 本片不覆盖无预算代表意图、无窗/多窗自动编译及全部复杂 view 规格；这些继续可用原显式 queries。不得把有界自动入口限制扩大为正常工具禁令。

## 同冻结双例，各一次

v1 冻结 **7356** 输入，SHA `4961995ce765f89996057f85f551c1f7f1b38f3038b5d608a30276578e54ecba`。实际标准 make CLI SHA `d4fb47c64359c895e187a74ec1aec70b6cf16fffaf9e6097a41487bd5e12a4f8`；B 测试二进制 SHA `f15488e06d68737b01b2f62b305626a0917734a2a858f19986cdbcddd8803482`。共同 exec UTC 02:26:59.441247/.441251，实际模型请求交叠 **21.341 秒**，不是只凭进程同时存活。

| 轨道 | 机器/实际入口 | 完整人工与未销范围 |
| --- | --- | --- |
| A 自然代表选择 CLI | exit0；新 profile、request_plan 自然采用；8 计划/2 规格组/4 选/4 未选；8 独立查询结果均符合原件 | FAIL：同次选择快照与持久化索引缺共同执行视图；正文混事件栈、无证配对/代表覆盖、事件数与分组数混用；11.2 不销 |
| B 自然只读登记父 controller | exit1；2 次模型请求，1 次真实来源验证通过；登记 planner/恢复子/新登记执行均 0 | FAIL：恢复 run 无预算时采纳模型 max_batches=1，补证追加触及上限；展示0/1与准入名册不同，未执行补证批已落盘为verifying；18.5 不销 |

A 为四份构造 SQLite 文件（两个已有内容的复制），不是四台设备/独立采集，也不是新二进制格式验收。31 次 trace_query 调用含 pure_memo 复用，只有8独立记录、最终6success/2empty；不能说31次独立解析。选中 a/c，未选 b/d 后续合法执行不改变选择身份。原生窗口、事件、栈帧及端点取数正确。

B `full_cli_run=false`，预先源码修复及历史测试来自 operator 公共工具夹具，非模型自然创作。实际只到父 controller；`evaluation.json` 的 fresh-process 等是场景配置，不证明 child 已执行。收据的 B finished_at 是等待 A 后收割时刻，实际顶层测试30.36秒；不得当作6分41秒模型时长。当前1断言/2命令与旧 operator invocation 分离，两个同字节报告文件只代表一次执行。实际最终报告诚实 blocked/缺行为契约见证，没有假完成。

详见[人工审计](parallel_selected_summary_hmc_artifact_work_registration_20261025_manual_audit.md)。两份原问题、失败、最终MD/HTML/旁路或 typed final、模型上下文和完整原生结果均保留；不追第三例、不改原题求绿。

## 已证实的新共享缺口与 ROI

1. **同次状态交付一致性，11.2/16.4/18.4**：Catalog.Save 在 detached clone 合并跨分支历史，随后 selection 从局部分支生成；共享 saved_index 固定路径还可被后续查询覆盖。最后快照12次/最终索引31次，7/8成员尝试数不同；a101 latest同为27但3→6，已证明不只是 status 后重查。未保留该瞬间 alias 字节，不能声称逐字节恢复当时索引。下一批应从同一发布视图产生不可变索引/选择材料，保合法重试与跨分支合并，不禁止未选执行。
2. **恢复预算来源、使用量与原子准入，18.4/18.5**：缺省恢复预算被普通 verify 决策的模型计划字段填成硬上限；上下文用 used，准入用名册长度；先追加落盘再拒绝。正常新建 run 初始化5/1，不等同该恢复夹具。先做共享计数/预算来源/提交前准入的公开反例；保显式有限预算、来源和合同门，不为过例抬上限。
3. **原生事实与解释同源，16.4/18.4**：实际 Finalizer 已有正确原生事实，同时解释预览前2/7条带入错误业务画像及“3条全为分配”。正文仍混帧/配对，SQLite二进制引文还使MD/HTML含非法UTF-8；不把调查摘要当新事实，不以末尾限制抵消正文错误。未投后5条不算已投递污染。

旧采样解释越权、IO解释失败、计算权限新分支自然覆盖未触发继续留账；17.6具备新真实材料即提级，不用旧SQLite包装充格式能力；05.2/05.3与14.1保材料前提。17.7活跃WAL未满足零副作用，生产 **NO-GO**，本批未接线。

## 验证及原件

前置公开 RED 保留：新字段缺失、新入口缺失及 process/resource_stack 不可执行组合。修复后完整公共链路从实际 emit_analysis 受理至8/2/4/4及4次普通原生执行；预算过大/缺字段/源权限/目标冲突/process负控及未选合法查询均验证。最终相关4包590顶层/451子例PASS、1专用恢复子进程helper未启用SKIP；race2包6顶层/12子例PASS，工具/context相关及race、标准make均通过。

v1唯一全仓 PID61891，UTC02:26:57.034980–02:42:22.936167，exit1：**96包PASS/12无测试/1包FAIL**，28305顶层/33006子例PASS、41顶层/14子例SKIP。唯一失败为 `TestDiagramIdentityAuthorityCensus`：新合法引文来源校验 owner 尚未登记。生产校验只使用 raw/copy.SourceQuote，并非身份或预算关键词门。原日志SHA `942361f7fbda26d2114a794f3dcc01513f72e7fcba7be747b8579723d5a17df7`，7356输入前后相同。

先封 **276件** v1原件，索引SHA `324d3ef0a5fd6f3d31ce1dde849c7341494e2c231d771ebb81b7733b67720046`；随后仅迁移结构测试：精确登记该owner、锁死两个实参，并新增mode/预算替换的self-red。未改生产/实际CLI，v2冻结7356输入SHA `1a0980a41deb2b1a3c0ad8ab7cf1e53a1bcebef7ecc098f529c70a034056634f`，与v1唯一差异是该测试。后继相关/race各11顶层/24子例PASS；完整tool包PID51655于UTC02:43:43.680788–02:55:49.355247，exit0，7575顶层/7364子例PASS、25顶层/12子例SKIP、零FAIL，日志SHA `6499b0e41a924663dede7397afa0e299a97f940366772b49dc6083aed653fb90`。输入前后不变；这是受影响包复测，不是第二次全仓，不回签v1或两份自然FAIL。

本地原件：[v1索引](results/hmc_artifact_work_registration_20261025_v1/artifact-sha256-v1.json)、[v1封存凭证](results/hmc_artifact_work_registration_20261025_v1/archive-v1.json)、[v2索引](results/hmc_artifact_work_registration_20261025_v1/artifact-sha256-v2.json)、[v2封存凭证](results/hmc_artifact_work_registration_20261025_v1/archive-v2.json)。后继另封16件、共292件，v2索引SHA `a3aab5fa1c54aad7b8756b907bff722180ba99f8df5daafb8adb1ae37f16aeb9`，v1逐件复核不变。完整双例原审计在 `sealed-v1/tmp/codrax-hmc291-{A,B}-manual-v1.md`，原件目录按绝对源路径镜像。此目录按项目规则忽略、不推送敏感运行载荷；可复核结论及代码/测试随仓提交。

代码 `adf6622033b9a8958057e6d04500edb75bd9075b`，文档另笔普通提交推送；最终两提交树7356输入、远端main、clean及292原件逐SHA终态见[发布凭证](results/hmc_artifact_work_registration_20261025_v1/publication-v2.json)。独立文档/测试迁移审查无阻断；其审查不代签后续执行收据。完整能力新增0、共享子能力1组、54开放及两份完整人工FAIL不变。
