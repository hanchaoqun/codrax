# HMC §196 双例人工审计

冻结 revision `1eeb563f0258`，26432 正式 exit0；恰好 2 并行 × 1、每例 1200 秒总预算，没有第三例追绿。机器 1/2 PASS，完整答案人工 0/2 PASS。原始机器判分不改写，子能力交付不等于完整答案通过。

| case | 机器 | 完整人工 | 时间 | 实际结论 |
| --- | --- | --- | ---: | --- |
| trace_sqlite_closed_wal_records | PASS | FAIL | 130s | 新入口、两段各 8ms、进程正确；额外系统事件仍 4 写 5，域/名称呈现含混。 |
| read_combo_command_current_source_explanation | FAIL | FAIL | 96s | 390 统计正确，但纯操作路由未读任何源码，将操作自身凭证当成客户仓机制解释。 |

## 1. 二进制：正常关闭、WAL 不存在

新自然问题只要求 1.040–1.080 秒窗口的启动名称、起止、耗时和进程，不把 WAL、未知值、源代次等系统约束写入用户问题。只附 `eval/fixtures/hmosperf_closed_wal/capture.data`；生成器与独立 oracle 不进模型上下文。该合成夹具使用项目固定的 SQLite 实现正常提交/关闭，保 WAL 模式文件头而无 WAL/SHM/journal；不是实机格式覆盖证明。宿主 Apple SQLite 3.51.0 关闭后仍保辅助文件，最初 Python 生成器断言失败，没有偷偷删除辅助文件伪造正常关闭；改用 pinned SQLite 生成器，失败过程未作为产品故障计数。

实际 preparation 位于 `.codrax/trace-input-8c5b01e7af2cba615f1b42a2f2dc23a2/preparation.json`：
main=61,440 字节，`WAL.Absent=true/CheckpointOnly=true/CommitFrame=0`，不存在文件的 SHA/Generation 为空；镜像 61,440 字节，SHA=`37fd812272d0af401ad97b5bdef3bd5781be724ddea03f79041eeb43dc65d733`。本地源文件指纹未变，目录仍无新辅助文件。收据/能力 caveat 明确：只能读交付主库的 checkpoint，无法判断历史上是否遗失过 WAL，不宣称捕获完整或持续写入在线一致快照。

过程只调用 1 次 trace_query，窗口 1.04–1.08，8/8 匹配行完整返回：两段启动的 4 个端点与 4 条系统事件。预览仅到 0.980 秒未阻断查询完整附件；源码读取、repo_map、list_files 均为 0。所求主表正确：
LoadPreferences=1.040–1.048，RestoreTabs=1.064–1.072，均 8ms、owner_pid=27599。

模型接收到正确的四条系统事件：1.046 UI_READY / APP_LAUNCH 域；1.050 STAGE_READY / 域未知；1.052 事件名引用50未解析 / APP_LAUNCH 域 / stage=draw 仅内容；1.058 BOOTSTRAP / APP_LAUNCH 域。日志 1190–1220 和 2032–2037 分别显示探索交接和最终阶段都识别出 4 条；原始结构化数据没有第 5 条。终稿却称 5 条、表格只有 4 行，并把 APP_LAUNCH 域放在“事件”列主体位置。虽比前批保留了“未解析事件名”，仍不能判完整通过。未请求的参考信息增加了错误面与内部术语，不能用主表正确抵消。

上下文峰值 74,984/200,000，约 37%；最终初始 56,817，元数据补齐后 66,813。一个仅 8 行结果仍带 33 caveats、28,755 字符工具结果，含跨领域覆盖资料；需要继续按查询用途投递，不以屏蔽源信息或猜答案解决。预阶段最终消息仍含附件范围 0..0 的单位说明；它被明确限制为附件范围而非所求窗口，本次没有污染数值，但预览/extent 与完整附件的呈现接缝继续挂 01.3/16.4，未凭本次确定其所有来源分支。

本例无图。根因旁路 `.codrax/output/20260929-183330.083-72152.root-causes.json` 为 schema2 空根因，原因 `trace_root_cause_contract_not_active`；未将邻近系统事件升为根因。

## 2. 只读：失败提前到任务路由与完成判定

沿用上批原问题及动态 oracle。当前多了一个生产文件，独立计数与实际完整 find/wc 命令都得到 390（不是把上批 389 当固定答案）。第一份操作计划错误地把 `| wc -l` 当独立命令，现有语法检查拒绝后一次重计划修复，没有执行无生产者管道；这是已有防护有效，不是新增产品退化。

日志43行：`raw_route=operation route=operation source=mixed needs_repo=true current_source=optional needs_operation=true`。于是 CLI 在 `maybeRunSingleShotOperation` 中完成操作后直接返回；没有进入 analyze/explore/finalize，没有源码 read_file/repo_map，也没有源文件内容读取命令。3条实际命令为完整 find/wc、文件列表、ls/head。操作评估在“计数及清单凭证已有”后判定整目标 complete，未保留“基于当前源码解释”的独立证据义务。

终稿“当前源码依据”只是文件路径列表，“证据链路”说明本次操作自己的 payload/coverage，而非读到的仓库实现。即使 390 正确，也没有回答用户的源码机制问题；完整人工 FAIL。机器原判为 `answer_surface_receipt_missing dynamic_scalar_binding_missing:tool_non_test_go_recursive:390`：缺标准答案 surface 收据，不能描述成统计数字错误，也不伪造 primary.md 补过判分。保原始 run-1.out 作为实际操作报告。

这是比 §195 更上游的同类系统接缝：生产修复已覆盖“到达分析管线的 typed 次要维度”，本次路由绕过该管线，不能宣称新指导 live 命中，也不能一概称模型偶发误述。新指导的多主意图×多关系维度、真实 finalizer 初始消息、可选维度负控及 schema/skill 同源由公开回归证明，模型效果仍待异构回放。

优先修复方向挂 01.3/16.4/18.5：让混合任务的各证据义务贯穿路由、执行与完成判定，命令完成不等于源码说明完成；通过 typed 分类/权限/已完成凭证选择 read-only handoff 或保留未完成维度，不扫描原问题/答案关键词，不把普通操作的工作目录需求一律当源码分析要求。操作报告缺标准 surface 收据单列呈现验收，不能用改 oracle 掩盖机制缺失。

源码核对：turn_policy.go:581–614 已明确教 current_source 独立于仓库访问、源码关联必须 required、分析中的只读测量不应转 operation；不是完全缺教学。isAnalysisOnlyPolicy 只有精确 required 等信号才纠偏，不能读 reason 自行猜回源码义务。command_operation_cli.go:239 的完整判定则只消费整体 complete/置信度，没有分维源码证明。下一步先以公开多组合矩阵区分路由误分类和义务交接缺口，复用现有精确信号；不再叠一段同义提示，也不把这一次 raw_route 漂移当成代码回归。

## 3. 状态及下一 ROI

- 稳定总数仍 79=16完整交付+63开放，重复0；5个验收父项03.2/04.2/08.3/08.4/18.2未销账。本批可复用子能力2：无WAL主库的只读接入/失效；混合测量说明维度保留（含同源教学和独立scalar/成员数量提示隔离）。
- 两份完整人工 FAIL 保留；其中只读是流程缺口，Trace是已充分供证后的输出错误。不把两者都叫随机波动，不追加单例专名规则。
- 下一完整能力优先04.5进程概览/睡眠树/D状态，其次03.3原生资源栈；17.7持续写入快照、存储类/多源/普通viewer、17.6实机平台仍开放。参考 process_profile 的窗口裁剪和分维结果可复用，但 contention_score 只能作线索排序，不做主因门；跨线程热点不能误写成代表TID独占，top_n=0不等于进程无线程。
- 只读登记完整CLI/controller收尾§192已通过，不重复列未实施；重启恢复、跨模式与图关系验收照原账保留。两例都是读模式，不代签写模式。
- 本批不改链上根因、显式窗/因果投影/自动补齐、写审批和600/300/600秒等待。没有活跃流4分钟降级；本次也未触发超时。

## 4. 原始凭证

目录分别为 `eval/results/trace_sqlite_closed_wal_records-20260929-183124` 与 `eval/results/read_combo_command_current_source_explanation-20260929-183124`。小型原判、metrics、wall及Trace正文/只读实际out随批保存；完整日志、准备材料留本地并固定指纹。

| 工件 | SHA-256 |
| --- | --- |
| 输入数据库 | `7bdc1c4806f71e167ac47782bebb8efa3592f647f47ee16801ccc9eb7b1f28b9` |
| Trace完整日志（72152） | `fcd624659d92056f35dd3132b60a077b5edcf0be53ef09b10d8c2d0d524544ef` |
| Trace最终正文 | `67801ac6dac6f229b4d705afe5bded9590c4ddedf631b3cdd186f0446b2b39c0` |
| Trace准备收据 | `09555cf36c0f05bac903ebe65337ad8b9d171d7e400417ea97b67a37d4bca3bc` |
| Trace根因旁路 | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
| 只读完整日志（72137） | `0145af87dc465b31c74ebadf00edb4fd04453a92c1a1a0aaac2515e5e8c8b175` |
| 只读实际操作报告run-1.out | `7a9f11a21d5488a8495cb3085c9982fd476ec33f590f5221493d57c366345777` |
| runner日志/tmp/codrax-hmc196-live.log | `0caa78fd94d37f312c9c391ba756622bd4fb64db412f22b68e6586d04d4e3a38` |

末版全仓及推送收据见统一账本§196，不以runner exit0当作两例通过。
