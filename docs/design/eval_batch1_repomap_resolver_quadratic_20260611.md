# Eval 批次驱动的架构 gap — repomap 调用解析二次方

## 1. 实测批次结论(批 1,6 案)

代表性 6 案按家族优先级、每波 2 并行运行,**6/6 PASS**:

| 案 | 家族 | 判定 |
|---|---|---|
| u3a | 核心代码 root-cause | PASS |
| m1a | 核心代码 enumeration | PASS |
| qf_config_precedence | 配置优先级 | PASS |
| logtri_java | 非 Go 日志三角 | PASS |
| trace_query_frame_timeline_flow | trace 时间线 | PASS |
| mr_implementers | 多仓 implementers | PASS |

答案质量无新架构 gap;零重试(仅 1 次 emit_hypothesis_verdict 良性重发)。

## 2. 性能架构 gap(P0,日志挖出)

三案日志一致显示 repomap "phase rank elapsed≈19.7s"——但 `elapsed` 是 `time.Since(scanStart)` **累计值**,真实空窗在 `index.BuildGraph`。

**逐层 profile(codrax 自身 1687 文件 / 38k 符号)**:
- scan 30ms,parse 1.9s,BuildGraph **19.6s**。
- BuildGraph 内:resolveImportGraph 39ms、populateImplementers 2ms、AddFileSymbols 9ms、Finish 0.1ms、**AddFileRelations 19.5s**。
- AddFileRelations 热点:`Graph.ResolveCallTarget` 的跨包 fallback `for k, s := range g.MethodIndex`——对**每个**未在本包命中的 call relation **全量线性扫描整个 MethodIndex**(一 method 一条,数万条)。

**问题类**:"该索引的查找没建索引"。代码注释的心智模型("几百个 package")与实际数据结构(数万 method)不符;且 map 迭代序使解析结果**非确定**(重复方法名随机选一个)。这是一类通用反模式——任何 per-item 操作里对全量 map 线性查找,都应建倒排。

## 3. 修法(系统级,泛化)

`Graph` 增 memoized `(receiver, name) → 首定义 symbol` 倒排(`resolveReceiverName`,sync.Once,与既有 `FlatSymbolIndex` 同模式;不序列化;`cloneGraphForRanking` 字段级浅拷贝天然不带,克隆体懒重建)。跨包 fallback 从 O(methods) 降到 O(1)。文件序 first-wins 顺带消除非确定性(严格优于旧随机选)。

**结果**:BuildGraph 19.6s → 78ms(~250×);AddFileRelations 19.5s → 25ms。bench 守卫 `BenchmarkBuildGraph_CrossPackageCalls`(200 包同名方法 × 400 调用点)443µs。

## 4. 客户现象关联 + 进展透明度(同根因 + 防御)

客户 WSL `/mnt/d` 上 1417 文件仓在"构建符号关系图:源文件已解析 430/430"卡约 30s——正是本 gap 的现场版(慢 I/O 放大,但 30s 主体是 `AddFileRelations` 的 CPU 二次方)。修复后该阶段降到亚秒级。

**透明度独立加固**(即便修复,超大仓/慢环境下 build_graph 仍可能数秒,不能哑跑):`BuildGraph` 黑盒内的主导子步 `AddFileRelations` 现按文件上报关系解析进度——新增 `BuildGraphWithProgress(files, relationProgress)`,`BuildGraph` 委托 nil;notifier 加 `buildGraphRelations` 发射器(复用既有 2s/200 文件节流,与 parse 阶段同款);事件复用既有通用 `ViewStepsDone/Total` 字段(不发明新字段);渲染层 build_graph 阶段有关系进度时显示"解析关系并构建符号图 N/M",否则回退原"解析 N/N"。三处 BuildGraph 调用点统一经 `buildGraphProgressFn` 接入。快速构建因节流静默,慢构建逐步跳动而非冻结。

## 5. 任务列表

- [x] receiver-name 倒排 memo + 跨包 fallback 改用;确定性 + same-package 优先双测试;bench 守卫;全量 67 包测试绿。
- [x] build_graph 阶段关系解析进度透明度(回调 + notifier 发射 + 双语渲染 + 回退测试)。
- [ ] 后续批次(批 2 起):继续按优先级跑 6 案/批,挖下一类 gap。

## 6. 进度

- 批 1 交付:repomap 调用解析二次方修复(`902e3ff2`)。
- 客户现象插队:同根因确认 + build_graph 进展透明度加固(本 commit)。

## 7. 批 2(6 案)+ 工具使用观察:implementers 视图缺口

批 2 六案 6/6 PASS(s5a / u7a / trace_query_state_churn_window_stats / logtri_rust / mr_cross_repo_compare / data_multifile_reference_projection)。

**工具使用 gap(s5a 暴露)**:"列出本仓所有实现 `LoopController` 的具体类型"——模型用了 **13 次 read_file + 6 次 grep,零次 repo_map**;小接口侥幸答对,大接口必漏且烧预算。根因:系统 `populateImplementers` 已构建 `Symbol.Implements`、`Graph.ImplementersOf` 已实现(analyzer 内部在用),但**从未作为 repo_map 视图暴露给模型**;skill 的 EXHAUSTIVE ENUMERATION 规则只能把实现者枚举指向 `source_inventory`(成员清单,不是实现者关系)。这是一类通用反模式:**结构已算出但未暴露为视图,模型被迫手动重做**。

**修法(系统级)**:新增 `repo_map(view="implementers", query="<Interface>")`——直接消费既有 `ImplementersOf`,SymbolID→file 一次性映射(不依赖未必填充的 `Symbol.File`),未命中时引导 grep 回退。enum/schema/ToolDescription/query 描述同步;explorer skill 的枚举规则软引导实现者/conformer/subclass 形态走该视图(analyzer 已 typed 分类 `is_category_enumeration` 形态 b,这里只是软提示,非关键字匹配)。多仓经既有 path→子仓单图解析,`ImplementersOf` 单图即可,无需另接 MultiGraph。

**活体验证**:对 codrax 自身 `repo_map(view="implementers", query="LoopController")` 一次列出 13 个实现者(含 s5a 期望的 `analyzerEvaluator`)。

## 8. 任务列表(累计)

- [x] 批 1:repomap 调用解析二次方修复 + build_graph 进展透明度。
- [x] 批 2:implementers 视图暴露 + skill 软引导 + 双向测试 + 活体验证。
- [ ] 批 3 起:继续按优先级跑 6 案/批,挖下一类 gap。
