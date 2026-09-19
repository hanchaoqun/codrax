# 原始二进制 Trace 默认接入对照与最小实施设计（2026-09-17）

## 1. 结论与当前状态

**2026-09-18实施更新**：HMC-17.1–17.3已以`482bfa856`提交推送，最终无skip全仓87包及构建通过。CLI文件附件已接共享准备服务，完整查询材料与模型预览分离；真实RMQ和SIMPLEPERF二进制回归已覆盖。REPL当前只安全承接CLI准备好的附件，`/htrace path`默认转换、未准备typed path、SQLite、二进制stdin及跨平台矩阵仍开放。最终验证/交付收据见主账本§13。以下§1–8保留2026-09-17审计基线，不应将其中“当前未实现”理解为施工后状态；完成状态只以实施任务清单为准。

**2026-09-19实施更新**：17.4默认REPL接入已以`eb2ddd446`推送，17.5普通命名路径协调正在最终验收，见主账本§19–20；SQLite、二进制stdin及格式/平台矩阵仍开放。本记录中的“真实二进制”指测试构造的合法格式字节进入真实转换/查询路径，并不等同于实机捕获；17.6所需代表采集与平台覆盖的重新核实见§9。全仓未使用`-skip`不代表包内不存在条件Skip。

关联主审计 **HMC-17**。本次只读核验基于 Codrax `055bd749d` 及 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`；未修改生产代码、未运行远程模型、未启动参考服务器。

**当前尚未实现“直接附加原始二进制 trace，默认自动转换后分析”。** 已实现的是独立转换入口的 `trace-engine=auto`，两者不是同一能力：

- `codrax trace convert --input capture.sys`：默认先尝试 TraceStreamer/SQLite，必要时走有能力边界的内置解析。
- `codrax --htrace capture.sys --request ...`、REPL `/htrace capture.sys`、`trace_query source=path` 指向二进制：当前会在文本准入处拒绝，要求先转换。
- REPL `/htrace convert ...` 成功后也仅展示产物与下一步，并不会自动把派生产物设成当前附件。

参考仓也不应被夸大为“指标查询自动读任意二进制”：它提供显式 `convert_hitrace_to_sqlite` 工具，随后 `query_metrics` 接受 `sqlite_path`；“先转换再查”仍是调用链的一部分。参考价值是无须用户另找解析器、从源文件通往统一查询存储的工作流，不能照抄后缀路由、同名mtime缓存或目录取首个文件的弱身份策略。

HMC-17 的高 ROI 首批应是：**把现有受约束转换能力接到默认附件接入，共用一个准备服务，保留现有源身份、取消、产物收据、覆盖说明与因果边界。** 不应先新造一个宽松二进制解析器，或删除目前阻止二进制进入文本解析器的安全门。

## 2. 默认真实路径与代码定位

以下位置是本次读取时的行号；参考路径以参考仓为根，其他为 Codrax 根。

| 入口 | 当前真实路径 | 当前结果/边界 |
|---|---|---|
| CLI `--htrace/--atrace path` | `cmd/root.go:1118` `loadAttachedTrace` → `:1145` `loadMultiPathSlice` → `:1239` `readAttachedSourceLimited` → `:1244` `attachment.ReadTextFileLimited` | 仅文本加载；二进制被拒绝。`cmd/multi_attach_test.go:182` 明确固定了“拒绝并提示convert”。非UTF-8日志仍应拒绝，不可因为补trace入口而放松log。 |
| CLI inline/stdin | `cmd/root.go:1192` inline `ValidateText`；`:1253,1261` stdin受限读取/文本校验 | 是有大小上限的文本载体，不是未截断二进制文件。不能把已读到上限的stdin片段交给converter后声称完整采集。 |
| REPL `/htrace path`（`/atrace`同通道） | `internal/repl/repl.go:11632` → `:11682` `ReadTextFileLimited` | 不自动转换；失败不会成功设置新附件。该既有“原附件保持”行为需要新流程继续维护。 |
| REPL `/htrace convert ...` | `internal/repl/repl.go:11727` 构造Options → `:11756` `r.hitraceConvert(context.Background(),opts)` → `:11782`打印next | 独立命令，不自动附加。当前显式命令传Background，新增默认接入必须设计可取消的请求/交互上下文，不能复制这个无取消形状作为新默认。 |
| `trace_query source=path` | `internal/tool/trace_query.go:387` `ValidateTraceInputPath`；`:2753` `traceQueryBuildIndex`；`internal/tracequery/input_admission.go:15` typed准入码 | 在任何索引/流式解析前拒绝二进制。`:653`把失败转换为结构化repair/argv提示，并不执行转换。底层BuildIndex不应自己启动外部工具。 |
| `trace_query source=attached_trace` /直接BusContext字符串 | `internal/tool/trace_query.go:702` 在落盘前 `CheckTextString`；`:3601`附近解析物理blob | 防止绕过CLI/REPL将二进制字符串塞进附件。已有物理blob优先；不能以一个陈腐内存附件污染另一个显式path查询。 |
| 普通问题引用文件路径 | `internal/tool/trace_query.go:3478`附近可从typed artifact/analysis carriers恢复唯一现存路径；后续仍走上述准入 | “唯一文件自动解析定位”不等于“二进制自动转换”；不得扫描用户原文关键词触发硬门。 |
| 参考显式转换工具 | `server.py:547` `convert_hitrace_to_sqlite`；`:612` .db直读；`:632`后缀集；`:676`选择工具；`:684` subprocess | 输入文件/目录先转换得到SQLite；工具进程超时300秒。非zero退出、超时返回错误，没有Codrax的多provider回退账。 |
| 参考指标入口 | `server.py:1290` `query_metrics(sqlite_path,...)` | 直接消费SQLite，不隐式转换任意源文件；不能把参考的工具编排解释成数据入口本身万能。 |

## 3. 二进制/容器支持矩阵：识别不等于支持，更不等于因果可查询

文本入口的精确内容分类在 `internal/attachment/text_check.go:40–54,435–461`；不根据文件扩展名决定它一定是trace。转换器的受持有源、容器、provider与二次内容检查决定能否真正转换成功。

| 输入家族 | Codrax转换器已有能力 | 直接附加/查询现状 | 自动接入设计边界 |
|---|---|---|---|
| UTF-8 ftrace/systrace，包括名字叫 `.sys/.htrace` 的文本 | 本来可直接查询，不需要转换 | 正常文本路径 | 保持字节/语义不变，不能按后缀重新转换已有文本。 |
| Harmony RMQ `.sys`，magic `0x0ace`、version1、file_type1 | `format.go:24–33`、`convert.go:654`附近严格内置decoder；auto先TS再builtin | 精确识别为非文本，要求convert | 接入现有Auto；不能把不同version/file_type喂同一种页结构。 |
| OpenHarmony/Linux ring-buffer raw，包含file_type0及另一raw magic族 | 内置RMQ不支持；官方TS优先。`format.go:24–33`声明另一页/事件布局；`convert.go:692`明确file_type0需TS | 非文本拒绝；当前附件精确magic枚举并不是所有转换格式的完备能力表 | 缺TS时应返回“解析器/平台不可用”，不把不支持的raw格式当损坏文本或让模型反复搜索。 |
| `OHOSPROF`现代profiler容器 | `profiler_container.go`：root/header、protobuf插件、SessionJSON/text载体、typed ftrace与独立sidecar；逐家族coverage、限长、源校验 | 内容已识别，直接附加仍拒绝 | 自动调用已有转换事务；未知插件/不完整结构保留coverage，不把“检测到容器”当“全部事件支持”。 |
| Linux perf/hiperf/simpleperf data | `perf_format.go:76–96`识别PERFILE2；`convert.go:238`附近direct perf路线，官方/内置样本路径、perftrace/bundle能力分层 | 原始perf二进制不能直接当文本 | 产物可能仅CPU sample可查询而无调度trace；不得自动宣称可以做唤醒链/帧根因。附件识别还接受反向magic，但这不证明所有endian均被converter支持。 |
| `SIMPLEPERF` report_sample protobuf | `perf_format.go:89`及perf provider registry有专门输入型 | 没有默认自动接入 | 扩展统一能力探针时复用现有provider支持集，避免只复用较窄的附件magic枚举导致漏掉这条既有转换车道。 |
| gzip | `perf_format.go:87`将gzip纳入perf输入候选，后续还需验证内部载荷 | `input_admission.go:87–95`给 `trace_text_export_required`，不自动转换 | **不能宣称任意gzip trace已经支持**；gzip包裹文本、perf、未知二进制要在受资源限制解包后分型。首批可明确只接已有perf gzip，不把gzip magic当perf证明。 |
| ZIP | `trace_archive_zip.go:171`以内容magic进入；`:350–405`仅选择唯一常规 `.sys/.htrace` 或显式member；CRC/大小/压缩比/重复路径等门 | 直接附加当archive拒绝；显式convert已有ZIP能力 | 原容器、选中member、member摘要和派生链均保留；多个候选不能挑第一个。未来增加text/perf成员须先定义多源/时钟/身份，不是放宽后缀列表就完成。 |
| TraceStreamer SQLite DB | 参考工具可直接返回DB路径；Codrax已有内部DB语义导出器、全表保真器 | `input_admission.go:20,65`拒绝直接SQLite；默认入口无公开“现存DB→持有安全导出”的统一路径 | 单独排队显式DB适配，不把SQLite随便交TS当采集文件；需要schema、owner、clock与数据库源代次验证。 |
| Android Perfetto proto、其他protobuf/压缩格式 | 本审计未确认独立、完整且有真实样本回归的Codrax默认内容适配器；外部TS能否解某文件仍需实测 | 不能从文档中出现“perfetto”一词推导二进制端到端已支持 | 建格式+版本+工具+产物验证矩阵，未知格式返回可行动诊断；不承诺“所有protobuf都可解”。 |
| 未知二进制、Windows驱动`.sys`、空/损坏文件 | 受限探针、严格decoder/provider错误 | 拒绝；empty、line-too-long、source-unavailable各有不同typed码 | 不按后缀将普通二进制自动当trace，不把空文件/超长物理文本行失败硬改成convert重试。 |

参考仓格式声明：`server.py:632`接受 `.systrace/.htrace/.ftrace/.sys`，全部交同一个TS；`core/hiperf_converter.py:73–110`另接`.data`并扩展BRBE/SPE。它没有这里所列每种格式的独立本地解码证明，不能仅按接受后缀集合计算“参考支持更多格式”。

## 4. 自动引擎、平台依赖、错误与保真：哪些已经有，哪些不能破坏

### 4.1 已有provider单源

`internal/hitraceconv/trace_tools.go:79–129`构建统一typed计划；auto顺序为TS→builtin，显式TS/builtin只有一条车道。`convert.go:125`起将原文件绑定为持有的conversion input authority，先探针/冻结输入，再选路；`:268–283`执行TS自动路径，`:359`后是profiler/RMQ兜底。显式模式不应因为新入口悄悄降级；文件换代、发布失败、预算/权限等错误也不能被一概解释为“换decoder再试”。

TS发现顺序在 `trace_tools.go:323–363`：显式Options路径→`CODRAX_TRACE_STREAMER`→Codrax可执行文件邻居→内嵌→PATH→已知位置。没有必要让用户为默认入口再配置另一套搜索表。

默认内嵌只含Windows amd64与Linux amd64；`embedded_trace_streamer_payload_unbundled.go:1–10`明确其他平台gap，darwin未内嵌的原因是参考darwin-aarch64资产实际为x86_64 Mach-O。参考 `_get_trace_streamer_bin`（`core/hiperf_converter.py:16–29`）只按操作系统选文件，不能证明macOS ARM原生可用。**“默认自动转换”不等于“所有平台、所有格式都不需要外部依赖”。** 自动入口必须显示实际选择的provider/能力缺口。

### 4.2 已有取消与输出事务

`ConvertFile(ctx,opts)`多阶段检查ctx，ledger仅在收据/产物/源代次最终验证成功后提交；错误清理本次拥有的产物。`external_tool_command.go:62–79`通过共享process-tree supervisor执行外部工具，不能另写一段shell调用绕过。自动附件转换输出应置于runtime管理的专有目录；显式converter当前默认 `DefaultOutputPath(input)=input+suffix`（`types.go:354`），若自动入口不指定输出，会尝试写输入旁边，不适合只读采集目录或自动多次重试。

不新增任意短墙钟“没有答案就降级”行为。转换等待与LLM首响应/流式静默是不同预算；进度活跃不是失败。用户取消应停止新转换/查询，清理本轮拥有的临时文件，保留已有原始输入/既有成功产物/原附件。

### 4.3 保真不是语义完成

`streamerdb_text_fidelity.go:142–147`已保证SQLite所有非内部表逐cell保留（NULL、有符号整数、REAL位模式、TEXT/BLOB原字节、schema、校验摘要），独立于语义导出器。不要重复报告“GPU/native表未语义聚合”就是转换物理丢失。

但是全表保真范围仅是**TS已经产出的DB表**；TS没有解析出来的二进制插件/原始记录不能靠这条载体自动恢复。原始二进制本身要保留引用，provider parse/error/stat与未支持家族须披露。`Result.TraceDBCoverage/TraceCoverage/TraceDecisions`和bundle能力说明都应进入附件准备结果，不只交一个看起来成功的`.systrace`文件名。

## 5. HMC-17 最小安全设计（待实现）

### 5.1 共享准备服务，而不是多处catch错误拼命令

建议一个位于CLI/REPL/工具边界可共用的准备层（概念名 `PrepareTraceInput`，不是已有API）。它消费**有来源的路径或已冻结材料描述**及ctx，返回结构化材料：

- 原始材料：原路径/display、文件generation/摘要、content kind、源容量、原始文件是否完整。
- 派生材料：本轮产物路径/摘要/类型、原始父材料、converter/provider版本与实际决策、archive/member归属。
- 查询材料：优先经收据验证的`.tracebundle.json`；必要时带严格能力声明的`.systrace/.perftrace`，不能只根据扩展名认定ready。
- 可见预览：有上限的文本preview；明确它不是完整采集、不是查询文件路径的替代品。
- 结果状态：可查询trace、仅可查询sample、仅库存、失败、取消、需选择多member、缺解析器等结构化状态；中文界面用可理解的措辞，不裸露内部枚举当答案。

建议分批接入：

1. **先CLI文件附件+REPL文件附件**，共享prepare流程，文本直通保持现有行为；精确可支持二进制由现有converter自动处理。该阶段不能声称普通问题里的任意path或直接API二进制字符串也已自动支持。
2. **再接typed path材料/trace_query的上层协调入口**。底层 `tracequery.BuildIndex`、流式搜索和持有源准入继续只消费查询就绪材料；不能在parser内启动外部进程，否则引入循环依赖/隐式写入并绕开生命周期管理。
3. **inline/stdin/直接BusContext二进制单独设计**。文本协议保持文本；若要支持二进制stdin，必须在截断前将完整流有界落入独立原始材料并标明实际EOF/limit，不复用字符串preview。没有该能力时给明确路径入口建议，不假装已自动支持。

类型判断读取文件内容/已验证结构，不读取用户问题/模型答案关键词。`KnownBinaryTraceFormat`是拒绝非文本的安全分类，不应直接充当完备的转换能力注册表；尤其gzip/ZIP/SQLite需要下层进一步验证，“是容器”并不证明“是本工具可解trace”。

### 5.2 时间窗、完整范围和多源归属

转换应从**完整原始材料**执行，不受给模型的附件预览上限截断。查询仍按用户显式时间窗与精确单位执行；不能把资源生命周期/窗口外数据自动拓宽成当前响应耗时。允许查询为了carry-in读取前继，但只按既有合同投影。

显式目标/时间窗不因自动准备而消失；`frame_root_cause_bundle`、已有recipe补齐、IO唤醒链、jank_event_sync等消费同一已验证查询材料。原始/派生身份关系进入artifact ledger，而不是仅改`# codrax-source`字符串。若原文件是ZIP，需要保留容器与选中member；多个trace不能flatten成一条时间线（现有 `cmd/root.go:1200`已拒绝多物理trace）。多个时钟/未校准perf分别标注，不把自动转换等同于跨源因果合并。

成功bundle可能包含无调度trace的sample-only数据；可以回答样本热点，但不能因此生成已证调度/唤醒/帧根因。没有合格查询产物时不能以“转换返回nil”判作可分析。

### 5.3 缓存、重试、失败与权限

- 首批可用每轮唯一、受管理目录避免陈腐复用，之后再做缓存。缓存至少绑定原始generation/摘要、archive member、选路/转换参数、语义版本及工具身份；不能参考同名DB的mtime新于trace就无条件复用。现有bundle孩子摘要校验必须保留。
- 不能写到原文件或覆盖用户已有同名产物；不能绕开现有转换 no-replace/ledger/源swap防护。原文件已被替换或中途变更应失败，不拿一份旧preview配另一份新二进制转换。
- CLI失败阻止该次依赖trace的分析；REPL失败保留旧附件但明确新附件未生效，不能悄悄用旧附件回答新trace问题。
- 明确转换失败是否可重试、是否已清理本轮产物、实际选用/失败的provider。不要让模型以grep或格式猜测反复撞同一准入失败。
- 缺TS且该格式builtin不支持，给平台/工具安装或显式路径建议；不偷偷下载/替换客户工具。Windows中文/空格路径走已有argv与私有快照，不能复制shell字符串拼接。
- 引擎auto回退保留第一条失败的typed诊断；取消/源权限/产物抢占等不能当成格式fallback理由。显式TS失败仍失败，不能在新入口改变用户选择。

## 6. 参考仓可借鉴与不照搬

可借鉴：一次工具调用拿到统一DB、全trace时间范围、可选扩展统计；`.db`现存材料可跳过重复转换；平台工具随包提供；转换与分析采用共享材料路径。

不照搬：

1. `server.py:439–451`目录输入排序取首文件，多个采集时会静默选错。Codrax应保留显式选择/唯一候选门。
2. `server.py:640–674`仅mtime与可读时间范围判断缓存，未绑定内容/工具版本/扩展参数；并可能删除同名损坏DB。默认准备服务必须只清理自己拥有的缓存/暂存，不能照此处理任意用户DB。
3. 后缀允许列表不证明真实格式；一个未知binary改名`.sys`不是可解trace，一份文本改名`.sys`也无需重转。
4. `_get_trace_streamer_bin`仅sys.platform选binary并检查exists，不能替代架构/运行时兼容性和工具完整性证明。
5. 300秒超时属于参考进程上限，不构成Codrax必须新增同样墙钟硬门的理由；更不能把用户要求延长的LLM等待改回此值。

## 7. 验收与已运行的前置验证

### 7.1 当前已实跑：证明现状，而非宣布HMC-17完成

以下全部是现有Go测试，无远程模型、无参考服务。CLI/REPL拒绝测试跑绿证明“当前仍要求手动转换”，不是新需求验收通过。

| 包/测试 | 结果与含义 |
|---|---|
| `./cmd` `TestLoadMultiPathSlice_RejectsBinaryTraceWithConvertHint` | 通过；CLI默认入口确实拒绝二进制并要求convert。 |
| `./internal/repl` `TestREPL_HandleHitraceLoad_RejectsBinaryWithConvertHint` | 通过；REPL文件入口同样未自动转换。 |
| `./internal/attachment` `TestKnownBinaryTraceFormatExactMagics` | 通过；类型判断基于精确内容magic，不按后缀。 |
| `./internal/tracequery` `TestTraceInputAdmissionRejectsBeforeEveryParserLane` | 通过；二进制未进入BuildIndex/流式搜索等解析器。 |
| `./internal/tracequery` `TestTraceInputAdmissionExplicitBundleRejectsBinaryChild` | 通过；不能把二进制塞进bundle child绕过门。 |

自动converter补充验证选取 `trace_streamer_provider_test.go` 的5项：无TS的RMQ回退、file_type0先TS、TS失败回退、SQL优先并保留DB、显式TS缺工具快速失败，**本轮全部通过**（`go test ./internal/hitraceconv -run 'TestConvertFile(NoPerfTraceAutoMissingTraceStreamerFallsBackToBuiltin|LinuxRingBufferFileTypeAutoUsesTraceStreamerBeforeBuiltin|NoPerfSysTraceAutoTraceStreamerFailureFallsBackToBuiltin|TraceStreamerAutoPrefersDBSystraceAndKeepsDB|TraceStreamerExplicitMissingToolFailsFast)$' -count=1`，包耗时1.988s）。使用合成输入与本地fake TraceStreamer，不说明全部客户真实二进制类型均已验证。

### 7.2 新需求红绿面

1. CLI和REPL分别以同一完整binary输入，converter stub只调用一次，默认TraceEngine=auto，返回query-ready bundle后同源进入查询；不再出现要求用户手动convert的断点。
2. 文本`.sys`直通、未知驱动`.sys`不误认；RMQ不同version/file_type、OHOSPROF、perf、ZIP单/多member、gzip内部类型分别覆盖，格式支持与拒绝理由可预测。
3. 原始超出preview上限仍转换完整文件；末尾关键wakeup/jank事件可查；模型preview截断只能影响预览声明，不能改变完整查询能力。
4. 模型/用户原文内容任意改写不影响转换选路；唯一决定因素是typed输入、内容类型、provider能力与用户显式配置。
5. 原始文件同名替换、输入读取中变更、并发同输出抢占、缓存被修改、bundle child篡改均不能混合代次；外部命令只接私有受持有快照。
6. 取消在探针/快照/外部工具/归一化/提交各阶段生效；无已取消后的新查询/最终假成功。REPL新附加失败保留旧状态并明确新输入未就绪，不自动用旧材料答新题。
7. sample-only、inventory-only、无事件但有保真table等状态不铸因果能力；原始DB全表保真、typed语义覆盖、viewer可见性分别披露。
8. 显式窗口根因投影与自动补齐回归；链上调度/IO/算力/确定性优化/业务线索不丢；新增资源/邻近事件仍只作背景，不额外发明链边或加冕资格。
9. Windows argv/中文空格路径，macOS缺内嵌但发现外部工具，Linux默认内嵌，slim build外部依赖分别验证；未验证平台必须明确列为待回放。

**收口标准：**默认CLI+REPL文件入口先独立闭环并提交；path/in-memory/新容器格式按实际完成面逐项更新，不能用convert已有测试全绿代替默认入口端到端验收。HMC-17 当前状态仍为“已确认、待实现”。

## 8. 实施前独立复核补充

第二席进一步落实到真实公共入口，仍为设计而非完成声明：

- `hitraceconv.QueryReadySystracePath(Result)` 只认 `Result.OutputPath` 对应唯一主件的就绪收据，采样侧另有 `QueryReadyPerfTracePath`；复用它们，不能用文件存在或 `EventsWritten>0` 代替权限判定。
- `resolveAttachedTraceQueryPath` 当前优先工作目录的 `attached_trace.txt`，否则把 `AttachedHitrace` 字符串物化。仅把转换后预览存回字符串会继续截断完整查询能力；新的 typed 准备载体必须贯通 CLI/REPL → Orchestrator → BusContext → 附件查询路径，同时处理 clear/替换/会话恢复。`# codrax-source` 注释不是完整源路径的权威。
- CLI 的既有 `worktree.InstallSignalHandler` 在 SIGINT 后清理并退出。只给 converter 传 `cmd.Context()` 不能证明收到 Ctrl+C 后转换事务能完成回滚；需协调单一取消/退出所有者，不再注册竞态退出处理。REPL 可使用 `startTurn/runInFlight/endTurn` 的非 LLM 包装，不借会重置 usage 的 DirectLLM 车道。
- 真实二进制回归可复用 `syntheticBinaryHitrace`（RMQ）、`syntheticProfilerTraceFile`（OHOSPROF）、`trace_archive_zip_test.go` 的 ZIP 以及 `input_authority_route_release_contract_test.go` 的取消/回滚夹具。独立前置复跑4项通过1.178s；是合成真实二进制字节，不是客户各版本真实输入均已回放。

最小交付仍必须包括“预览截断之后的尾部事件能通过 attached_trace 查询”的端到端正例，以及转换失败/取消时旧附件不被替换的反例。该完整接入比多加一个 convert 调用范围大，单独立批，不混入已验收的搜索覆盖修复。

## 9. 格式/平台矩阵复核与后续拆分（2026-09-19，HMC-17.6仍开放）

以下是源码及本地材料只读清点，不是新增运行验收。此前默认入口的合成格式字节回归和转换器单测不能倒签实机代表性。`internal/hitraceconv/representative_sys_fixture_test.go::TestRepresentativeSysTraceFixtures`在没有代表性manifest时明确Skip；目前对应目录仅README，实机RMQ/OHOSPROF覆盖仍缺。

| 家族 | 现有证据 | 尚需补足 |
|---|---|---|
| RMQ version1/file_type1 | 生成的合法页格式已经过CLI/REPL/Prepare/named-path完整公开路径 | 实机代表样本；未知version/file_type及缺TS的默认入口失败矩阵 |
| OHOSPROF | 转换器有容器/插件/资源限制/未知插件回归 | Prepare→公开查询正负例；未知插件库存不能变成查询就绪 |
| PERFILE2 | 转换器多attr、质量计数、样本及符号回归 | 默认入口收据与sample-only验收；参考实机采集本地专项 |
| SIMPLEPERF protobuf | Prepare→查询已保sample-only与CPU未知 | CLI/REPL格式面及实机导出样本，不倒签全部protobuf |
| ZIP | converter有成员身份、单/多成员、ZIP64、CRC/大小/比例保护 | 共享入口到完整查询及多成员失败；只自动选择现有`.sys/.htrace`候选 |
| gzip-perf | 现有封闭profile只接受解压PERFILE2，保来源变换身份 | 默认入口/完整性/取消；不代表gzip文本或RMQ已支持 |
| 其他raw/proto | 部分需外部TS，没有独立默认入口实机证明 | 按版本/工具可用性验收，不能据magic或TS路径存在签支持 |

参考`tests/fixtures/hiperf_brbe_001/test_brbe.data`是PERFILE2采集，可在本地只读专项中复核样本/cohort/质量；不擅自把私人采集拷入仓，基础sample可查不等于BRBE/SPE语义交付（另HMC-09）。参考`tests/fixtures/logandtrace/record_trace_20260821095539@6YF0126116-5955.sys.gz`解压头是`# tracer: nop`文本，非RMQ；参考测试先解压，因此不证明其生产工具原生支持gzip。将安全gzip文本运输作为17.6的单独能力片：完整性、解压上限、来源代次和受管发布先成立，不放宽成任意gzip/protobuf。

当前主机darwin/arm64运行回归；Linux/Windows跨编译与内嵌载荷检查不等于原生运行。参考目录当前`trace_streamer_mac`实际是Git LFS pointer，不是可执行工具；不能以文件存在推定macOS支持。该本地快照观察与§4.1历史资产来源是不同对象/时间，保留两者而不据此下载或替换工具。

17.6按原ID逐片推进：先统一OHOSPROF/PERFILE2/ZIP/gzip-perf默认入口矩阵，再补gzip文本运输；实机样本、真实外部TS和原生平台运行分别登记。新样本须明确可使用范围，尚未取得的材料/环境保持未覆盖，不把仅增加用例或交付一个格式算17.6整体完成。

本地专项追加（本机当前构建，非模型eval、非默认入口验收）：上述两份参考采集均通过显式`trace convert`指定独立`/tmp/codrax-hmc176-local.PNKHgE/`输出，未写参考目录。PERFILE2转换exit0，raw fallback保留12000条样本，产生perftrace/bundle而不生成systrace；转换器交叉检查为12000行，参考实际测试断言也是12000，其文件头18009的旧注释不采信。已查看原始产物的`branch_stack`跳过披露，未将基础采样成功写成BRBE能力完成；未运行额外查询或模型回答，引用/答案能力不由本次转换倒签。gzip文本转换exit1，当前darwin/arm64无TS，回退最终给出RMQ invalid_magic；本例保守拒绝正确，但诊断没有把“已压缩文本尚未支持”与RMQ格式错误清楚分开。该可行动诊断与安全gzip文本运输同归17.6下一片，不让模型反复重试。两份bounded诊断与运行日志留在上述本地目录，原始私人采集及派生内容不提交。
