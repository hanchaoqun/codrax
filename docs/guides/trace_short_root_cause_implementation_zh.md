# Codrax Trace 简短根因功能说明

> 本文说明当前代码的实际行为。面向第一次使用 Codrax 的读者。

## 1. 这个功能做什么

Codrax 分析 Trace 后可以保存两类结果：

1. 原来的完整长报告：保存分析过程、详细证据、限制和建议。
2. 可选的简短根因 JSON：当模型从 typed 链上候选中完成选择时，保存根因、影响秒数和简短证据。

简短 JSON 不会替换长报告，也不会修改长报告正文。

## 2. 最核心的变化

旧格式最多只能保存两个根因。新格式改成 `root_causes` 数组：

- 有 1 个可靠根因，就输出 1 个；
- 有 3 个可靠根因，就输出 3 个；
- 有 N 个可靠根因，就输出 N 个；
- 没有可靠根因，就输出空数组 `[]`；
- 不为了凑数量编造根因；
- 不再固定最多两个。

根因数量 N 和顺序由最终分析模型决定；可选项只能来自系统提供的 typed 链上候选清单。

## 3. 新 JSON 长什么样

```json
{
  "schema_version": 2,
  "root_causes": [
    {
      "rank": 1,
      "category": "cpu_scheduling_delay",
      "thread_name": "RenderThread",
      "impact_seconds": 0.0124,
      "summary": "RenderThread线程CPU调度延迟",
      "evidence": [
        "RenderThread 在目标窗口内 runnable 12.4 ms，期间没有获得 CPU"
      ]
    },
    {
      "rank": 2,
      "category": "lock_contention",
      "resource_name": "ClassLinker classes lock",
      "impact_seconds": 0.0081,
      "summary": "ClassLinker classes lock锁竞争",
      "evidence": [
        "Worker 等待该锁 8.1 ms，等待区间覆盖目标卡顿窗口"
      ]
    },
    {
      "rank": 3,
      "category": "synchronous_binder",
      "thread_name": "UIThread",
      "impact_seconds": 0.003,
      "summary": "UIThread线程同步binder",
      "evidence": [
        "UIThread 发起同步 Binder 调用后等待回复 3 ms"
      ]
    }
  ]
}
```

字段含义：

| 字段 | 含义 |
|---|---|
| `schema_version` | JSON 格式版本。新格式是 2。 |
| `root_causes` | 动态长度的根因数组。 |
| `rank` | 重要性排名，从 1 开始。程序根据数组顺序自动填写。 |
| `category` | 根因类型，只能从规定的十一类中选择。 |
| `thread_name` | 涉及的线程名。只有线程类根因需要。 |
| `resource_name` | 锁或资源名。锁竞争需要。 |
| `phase_name` | 阶段名。阶段高负载需要。 |
| `impact_seconds` | 该根因对目标分析窗口产生的有效影响时间，单位是秒。 |
| `summary` | 固定格式的简短中文根因。程序自动生成。 |
| `evidence` | 1 到 4 条简短、具体的 Trace 证据。 |

## 4. N 是怎样决定的

模型会查看 `trace_query`、证据账本、根因排名和因果投影，并只提交选中的 `candidate_id` 顺序。系统再从冻结候选中绑定输出字段。

一个候选要进入模型可选清单，至少应满足：

1. 它属于支持的根因类型；
2. 它有本次 Trace 的直接证据，且 typed `chain_relevance=on_chain`；
3. 它能独立解释一部分目标问题；
4. 它有大于 0、可表达为单线程墙钟毫秒的有效影响时间；
5. 它不是前面某个根因的重复说法。

模型从满足条件的候选中选几项就输出几项，因此 N 不是配置值，也不是固定上限。邻近区、背景区、计数、复合分数和跨线程 CPU-ms 不会进入这份根因选择清单。

数组顺序代表重要性顺序：

- 第一个是证据最强、最能解释目标问题的根因；
- 后面的根因依次变弱；
- 程序按照数组顺序生成连续的 `rank: 1, 2, 3...`。

这里的排序不是系统自动取第一名，也不是简单地只看耗时。模型结合结构化排名、链路关系、证据质量和有效影响时间决定选择与顺序；系统不替模型加冕根因。

## 5. `impact_seconds` 是什么

`impact_seconds` 表示这个根因候选在目标分析窗口内的 typed 有效墙钟影响时间，单位是秒。该值由系统从候选合同绑定，模型不填写或改写。

例如结构化结果给出有效影响是 `12.4 ms`：

```text
12.4 / 1000 = 0.0124 秒
```

JSON 中写成：

```json
"impact_seconds": 0.0124
```

优先使用 `root_cause_rank` 等结构化结果给出的有效或累计归因耗时。以下数值不能直接冒充影响秒数：

- 一个事件在 Trace 中的全部占用时间；
- 正常帧节奏中的 sleep 时间；
- 不同线程相加得到的 CPU 时间；
- 与目标窗口没有因果关系的后台总耗时。

如果无法得到大于 0、且能和目标问题绑定的影响时间，这个候选不应被输出为根因。

## 6. 完整处理流程

```text
真实 Trace 文件
    ↓
trace_query 提取线程状态、阻塞、唤醒链和耗时
    ↓
内部证据账本与根因排名整理候选
    ↓
最终模型生成完整长报告，并可选择 typed candidate_id 数组
    ↓
JSON Schema 限制字段、类型和根因类别
    ↓
程序按 candidate_id 绑定影响秒数、身份字段和证据，并校验重复项
    ↓
程序生成 rank 和固定格式 summary
    ↓
长报告与 .root-causes.json 分开保存
```

简短根因不是在长报告写完以后，再把结尾压缩一次。它和长报告在同一次最终成文中生成，使用同一批结构化证据。

## 7. 程序会做哪些强制检查

旁路 JSON 自身会拒绝下面这些错误；这些错误不会拒绝或替换完整长报告：

- `schema_version` 不是 2；
- `root_causes` 中出现 `null`；
- `candidate_id` 不在本轮 schema 暴露的 typed 链上清单；
- 同一个 `candidate_id` 重复选择；
- 候选的单位不能安全转换为墙钟秒；
- 数组中出现相同的标准根因。

模型输入只包含 `candidate_id`。程序从冻结候选生成其余字段：

- `rank` 按模型选择的数组顺序生成；
- `category`、线程/锁/阶段身份、`impact_seconds` 和 `evidence` 从 typed 候选绑定；
- `summary` 按根因类型和绑定身份生成。

## 8. 十一种固定根因类型

| `category` | 程序生成的 `summary` | 必要身份字段 |
|---|---|---|
| `io_blocking` | `<线程名>线程IO阻塞` | `thread_name` |
| `lock_contention` | `<锁名或资源名>锁竞争` | `resource_name` |
| `synchronous_binder` | `<线程名>线程同步binder` | `thread_name` |
| `priority_inversion` | `<线程名>线程优先级反转` | `thread_name` |
| `gc_long_pause` | `GC耗时长` | 无 |
| `cpu_scheduling_delay` | `<线程名>线程CPU调度延迟` | `thread_name` |
| `phase_high_load` | `<阶段名>阶段高负载` | `phase_name` |
| `jit_compilation` | `<线程名>线程JIT编译耗时` | `thread_name` |
| `shader_compilation` | `<线程名>线程Shader编译` | `thread_name` |
| `sleep_blocking` | `<线程名>线程阻塞` | `thread_name` |
| `compute_supply_shortage` | `供给不足` | 无 |

固定 `summary` 便于统计和聚类；`evidence` 由 typed 候选的量值与证据引用生成，不接受模型自由改写。

## 9. 输出文件在哪里

完整答案继续保存为 `.md`，并可生成 `.html`。简短根因保存为同名的 `.root-causes.json`。

例如：

```text
20260811-123456-1000.md
20260811-123456-1000.html
20260811-123456-1000.root-causes.json
```

当模型明确提交空选择时，JSON 仍然有效；也可以完全不提交旁路字段：

```json
{
  "schema_version": 2,
  "root_causes": []
}
```

## 10. Trace 文件怎样传入

建议显式传入，不要只把文件放到 exe 同一目录后等待自动发现：

```powershell
.\codrax.exe --htrace ".\demo.systrace" -r "分析这个 Trace 的丢帧根因"
```

交互模式可以使用：

```text
/htrace .\demo.systrace
/htrace show
```

`/htrace show` 用来确认当前任务确实加载了目标 Trace。

## 11. 需要注意的边界

- 根因候选选择、数量和顺序由最终模型完成；系统负责 typed 候选编译、字段绑定和校验，不会凭空替代模型诊断。
- 简短 JSON 是可选旁路。缺失或字段级格式错误不会导致完整答案丢失；此时只是不生成新的 `.root-causes.json`。
- 如果 `trace_query` 返回 `trace_input_source_unavailable`，说明结构化查询没有成功读取 Trace。这时应先修复输入路径或文件格式。
- JSON 格式正确，不等于诊断一定正确。仍要核对模型选择的候选顺序与完整长报告是否一致。
- 简短根因功能不依赖批量聚类。单个 Trace 在模型提交有效候选选择时也可生成 `.root-causes.json`。

## 12. 关键代码位置

- `internal/agent/trace_finding_contract.go`：向最终模型提供精简的 typed 候选清单。
- `internal/analysis/tracefinding/root_cause_report.go`：把模型选择的 candidate ID 绑定成完整输出字段。
- `internal/tool/answer_document_dynamic_schema.go`：定义动态 `root_causes` 数组的 JSON Schema。
- `internal/types/trace_root_cause_report.go`：定义 schema v2、十一类根因和运行时校验。
- `internal/tool/final_answer_artifacts_mutation.go`：接收并校验根因报告。
- `internal/tool/emit_answer_document_v2.go`：同时接收长答案和根因 JSON。
- `internal/outputdump/output_dump.go`：把根因报告保存为 `.root-causes.json`。

## 13. 用一句话理解

先用结构化查询从 Trace 中收集证据，再让模型从 typed 链上清单自主选择并排序 N 个候选，最后由程序绑定量值、证据、名称和 JSON 格式，同时完整长报告保持不变且不受旁路失败影响。
