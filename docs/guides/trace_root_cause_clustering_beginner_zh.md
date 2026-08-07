# Codrax Trace 根因简化与聚类功能说明（小白版）

## 1. 这个功能要解决什么问题

分析一份 Trace 时，Codrax 可能输出很长的原因说明。

例如：

```text
PID 38291 的 UI 线程在 100.125ms 时等待 Binder:59321。
Binder 服务端任务没有及时完成，UI 线程无法继续执行，
最终造成当前帧出现明显延迟。
```

换一份 Trace 后，可能得到：

```text
PID 40122 的 UI 线程在 210.010ms 等待 Binder:61107，
服务端响应较慢，导致主线程在绘制前无法继续运行。
```

这两段话看起来不同，但真正的原因其实一样：

```text
UI 线程在等待 Binder 服务端完成
```

新功能的目标就是：

1. 保留完整的分析和证据；
2. 提取稳定的根因信息；
3. 去掉 PID、TID、地址和耗时等每次都会变化的信息；
4. 把相同原因的 Trace 放到同一组；
5. 为每组生成简短、统一的原因名称。

---

## 2. 它不是简单截短文字

这个功能不是：

```text
把长文字截取前 20 个字
```

也不是只让 AI 再总结一次。

它使用结构化数据表示根因。

上面的长原因会被转换成类似：

```text
根因类型：binder_wait
问题线程：ui_thread
上游线程：binder_server
发生阶段：pre_wakeup_dependency
因果形态：upstream_completion_wakes_target
```

程序再根据这些稳定字段生成短标签：

```text
binder_wait ui_thread → binder_server @pre_wakeup_dependency
```

用普通中文理解就是：

```text
UI 线程在唤醒前等待 Binder 服务端完成
```

---

## 3. 整体工作流程

```text
分析一份 Trace
    ↓
得到原来的详细答案
    ↓
同时生成结构化 TraceFinding
    ↓
检查根因和证据是否真实、合法
    ↓
去掉 PID、TID、地址、耗时等噪声
    ↓
生成稳定的根因指纹
    ↓
指纹相同的 Trace 放进同一个组
    ↓
为每个组生成简短原因标签
```

详细答案是给人看的。

`TraceFinding` 是给程序保存、检查、比较和聚类的。

---

## 4. TraceFinding 是什么

`TraceFindingV1` 可以理解成一张“标准根因记录卡”。

它主要记录：

| 字段 | 小白解释 |
|---|---|
| FindingID | 这张根因记录卡的编号 |
| AnalysisKey | 本次分析的稳定标识 |
| Artifact | 原始 Trace 文件信息 |
| Scope | 分析范围和目标线程 |
| Symptom | 卡顿、掉帧等现象 |
| PrimaryCause | 最主要的原因 |
| Contributors | 其他贡献因素 |
| Unresolved | 证据不足时的说明 |
| EvidenceRefs | 支持结论的证据编号 |
| CounterEvidenceRefs | 不支持或限制结论的反向证据 |
| Coverage | 本次分析覆盖是否完整 |

### 有证据时

```text
PrimaryCause：有内容
Unresolved：为空
```

### 证据不足时

```text
PrimaryCause：为空
Unresolved：证据不足
```

系统不会因为想得到一个漂亮结果，就强行编造根因。

---

## 5. 系统怎样防止 AI 随便编根因

### 5.1 根因必须来自候选列表

系统先提供允许选择的候选根因编号。

AI 只能从候选列表中选择，不能随便创造一个编号。

例如允许：

```text
candidate-1
candidate-2
```

AI 如果提交：

```text
candidate-999
```

系统会拒绝。

### 5.2 证据必须真实存在

每个主原因和贡献因素必须绑定证据。

证据编号必须来自系统提供的证据列表。

AI 自己编造的证据编号会被拒绝。

### 5.3 根因 Token 必须合法

根因 Token 是系统定义的标准原因名称，例如：

```text
binder_wait
runnable_wait
```

系统会检查 Token 的类别、可加性、线程对象和修复方向是否正确。

### 5.4 不能超过证据能证明的程度

如果现有证据只能说明：

```text
可能是这个原因
```

AI 就不能写成：

```text
已经完全证明是这个原因
```

这样可以减少“证据不足但结论过强”的问题。

---

## 6. 为什么要和原答案一起保存

现在最终结果内部包含两个对象：

```text
Document：原来的详细答案
TraceFinding：结构化根因记录
```

它们会一起保存。

规则是：

```text
详细答案检查成功
+ TraceFinding 检查成功
→ 两者一起保存
```

如果其中任何一个失败：

```text
→ 两者都不保存
```

这样可以减少只保存了一半结果的问题。

普通代码问答不会出现 `trace_finding` 字段。只有系统明确开启 Trace Finding Contract 后，才会要求生成结构化根因。

---

## 7. 系统会去掉哪些无用差异

不同 Trace 中，下面这些信息经常变化，但它们不一定代表根因不同：

- PID；
- TID；
- task ID；
- transaction ID；
- Binder 实例编号；
- thread 和 worker 编号；
- `0x...` 内存地址；
- `ns`、`us`、`ms`、`s` 等耗时数值；
- 英文大小写和多余空格。

例如：

```text
pid=38291 Binder:59321 completed at 100.125ms 0xabc
```

会被归一化成类似：

```text
pid=<instance> binder:<instance> completed at <duration> <addr>
```

另一份 Trace 即使数字不同，只要稳定语义相同，仍然可以得到相同根因指纹。

---

## 8. 根因指纹是什么

根因指纹类似根因的“身份证”。

它由稳定字段生成，例如：

```text
根因 Token
根因类别 Lane
问题线程角色
上游线程角色
因果形态
业务阶段
归一化事件
归一化调用栈
Token 词表版本
```

然后使用 SHA-256 生成 Cluster ID：

```text
rc-<一串稳定的哈希值>
```

修改下面内容不会改变 Cluster ID：

- 中文显示名称；
- PID 和 TID；
- 地址和耗时；
- Trace 输入顺序；
- 长篇说明的写法。

只有真正参与根因身份的结构化信息发生变化，Cluster ID 才会变化。

---

## 9. 相同原因怎样聚类

假设有三份 Trace：

```text
Trace A：UI 线程等待 Binder 服务端
Trace B：UI 线程等待 Binder 服务端
Trace C：证据不足，无法判断
```

聚类结果是：

```text
根因组 1
简短原因：UI 线程等待 Binder 服务端完成
成员：Trace A、Trace B
数量：2

未解决
成员：Trace C
原因：证据不足
数量：1
```

系统不会为了增加聚类数量，把 Trace C 强行放进根因组 1。

无论输入顺序是：

```text
A、B、C
```

还是：

```text
C、A、B
```

结果都应当相同。

---

## 10. 主原因和贡献因素为什么分开

一份 Trace 可能有：

```text
主原因：Binder 等待
贡献因素：CPU 调度等待
贡献因素：IO 延迟
```

规则是：

- 一份成功 Finding 最多进入一个主原因组；
- 可以进入多个贡献因素组；
- 也可以进入 unresolved。

主原因占比最多是 100%。

贡献因素可能同时出现，所以它们的累计频率可能超过 100%。

把两者分开，可以防止把贡献因素误当成互斥的主原因统计。

---

## 11. 为什么不同数值不能直接相加

下面两个数虽然单位都是毫秒，但测量范围不同：

```text
10ms：只测选定窗口
20ms：测整份 Trace
```

它们不能直接相加成 30ms。

系统会按照下面信息分桶保存：

```text
单位
可加性
测量口径
```

不同口径进入不同的 Metric Bucket，避免产生错误统计。

---

## 12. 测试已经证明什么

当前测试已经覆盖：

- PID、TID、地址和耗时变化不会改变指纹；
- 输入顺序变化不会改变聚类结果；
- 两个相同原因会进入同一个根因组；
- unresolved 不会被静默丢弃；
- contributor 不会破坏主原因数量守恒；
- 不同测量口径不会进入同一个数值桶；
- 虚构 CandidateID 会被拒绝；
- 虚构证据会被拒绝；
- 超过因果证据上限会被拒绝；
- Token 信息不一致会被拒绝；
- 普通问答不会出现 `trace_finding`；
- Document 和 TraceFinding 可以一起保存和清理。

---

## 13. 现在已经能做到什么

目前代码已经具备：

```text
详细分析
→ 结构化 Finding
→ Finding 校验
→ 去除部分实例噪声
→ 稳定指纹
→ 精确聚类
→ 简短 CanonicalLabel
```

简短标签示例：

```text
binder_wait ui_thread → binder_server @pre_wakeup_dependency
```

这说明底层已经能够生成规范、稳定的简短原因。

---

## 14. 现在还不能自动做到什么

当前还没有完整接通：

- 批量 Trace Manifest；
- 自动并发处理大量 Trace；
- Finding 文件存储和缓存；
- 中断后恢复；
- Patch 修改主因后的 stale 检查；
- 文字答案与 Finding 的完整语义一致性检查；
- 最终的简短原因排行榜；
- CLI 批量入口；
- P2 模糊语义合并 Agent。

因此，如果直接运行现在的普通 Codrax 问答，仍然主要看到原来的详细答案。

底层可以生成短标签，但还没有自动把它输出成最终批次报告。

---

## 15. 用一句话理解当前功能

```text
现在已经做好了“把每份 Trace 的长原因变成标准根因卡片，并把完全相同的根因合并”的核心能力；还需要补上批量运行和最终报告，用户才能直接看到完整的简短原因排行榜。
```

---

## 16. 最终想要的用户输出

完整接通批次报告后，用户最终应该看到类似：

```text
1. UI 线程等待 Binder 服务端完成：27 个 Trace
2. 目标线程处于 runnable 调度等待：13 个 Trace
3. IO 完成延迟：8 个 Trace
4. 证据不足：5 个 Trace
```

点击或展开某个根因组时，还能继续查看：

- 包含哪些 Trace；
- 每份 Trace 的完整分析；
- 主原因绑定了哪些证据；
- 有哪些贡献因素；
- 哪些样本因为证据不足没有归类。

这样既能快速看总体结果，也不会丢失详细证据。
