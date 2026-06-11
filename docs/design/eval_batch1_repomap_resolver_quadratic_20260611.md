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

## 4. 任务列表

- [x] receiver-name 倒排 memo + 跨包 fallback 改用;确定性 + same-package 优先双测试;bench 守卫;全量 67 包测试绿。
- [ ] 后续批次(批 2 起):继续按优先级跑 6 案/批,挖下一类 gap。

## 5. 进度

- 批 1 交付:repomap 调用解析二次方修复(本文件同 commit)。
