# r1080 人工审计：Java 范围引用与 Python 症状修复

## 执行与选例边界

- 清洁提交 `7a4858e887c4`，构建 `0.1.20260916 / 2026-09-16T03:44:08Z`。
- UTC 03:44:42–03:46:45；sweep `20260915-204442`（本机日期）。严格两路并发、各一次，无第三例、追绿重跑或 oracle 修改。
- 243 例为 215 read / 25 apply / 3 plan；按用户影响、当前机制、历史失败、证明可执行性、最近覆盖和成本排序。Java 接续多行注释引用见证，Python 接续可原生运行的症状写修复；未选择已知 oracle 陈腐的 H9。
- `CAP=5 PARALLEL=2 TIMEOUT=1200`，原 read15/apply24 步未改；运行中不修改构建输入、题目或夹具，不向模型提供人工后验。
- 原机器汇总保留在同名前缀 `.md`；runner 收据 `.codrax/tmp/20260916-r1080-runner.log`。下列人审不回填机器结果。

| case | 原机评 / 墙钟 | 人工业务验收 | 过程与剩余项 |
|---|---|---|---|
| `sr_java_handler_impls` | PASS / 70s（工件内68s） | 三个实现、路径及范围引用 PASS | 首次成文、零拒绝/patch；模型将普通进程内路由称 HTTP，P2 留账 |
| `github_issue_dateutil_relativedelta_float_symptom` | PASS / 123s（工件内121s） | 补丁、原生原测试及独立后验 PASS | 正式证明有真实执行；已覆盖义务仍带“缺少观测”诊断，确定性 P2 同批修复 |

## Java：模型选择的完整范围确实穿过了生产边界

结果目录 `eval/results/sr_java_handler_impls-20260915-204442`。本节日志行号均指
`run-1.logs/codrax-20260915-204444-000-75113.log`，不是合并日志。
原答案 `.codrax/output/20260915-204550.746-75113.md` 及同名 HTML 保留。

| 实现 / 路径 | 注解行 | 定义行 | 文档行 |
|---|---:|---:|---:|
| EchoHandler / `/echo` | 7 | 8 | 6 |
| StatsHandler / `/stats` | 13 | 14 | 8–12 |
| UpperHandler / `/upper` | 9 | 10 | 8 |

1. 上下文足够：日志870–914完整预读 Echo/Stats，945–963完整读取 Upper；Handler/Route 亦已读取。最终 MD11–17 正确分开三个来源，MD12–16及31–35保留 Stats 完整注释，不以无图回答冒称图回归。
2. **B1694 自然命中**：日志2089原模型 JSON 中已有 Stats 第8行点引用，但 item 明确选择 `ev-bac51ea8d8bd1f2`。接受证据快照 `.codrax/plans/read_runs/trace-1789530291353851000.json:218–228` 为 `line_range`、8–12行，日志1905亦已供给模型。
3. 日志2090是既有 quote 修复，2092是提前证据ID绑定，2094–2095是既有无用槽清理：实际删去一个未使用的点槽，并重映射引用；“pruned/remapped 6”不等于删去六个引用。最终十条引用包含完整8–12行。新范围复用器不覆盖已有池项，但不能据此声称后续既有清理器端到端保留整池字节；模型另行明确选择的点引用有公共正反回归保护。
4. 一轮 finalizer、一次 full emit、零拒绝/patch/JSON恢复。可见 summary/item description 与原模型 JSON 相同，系统仅承运引用。自然 patch 覆盖为 N/A，不能拿公共 patch 测试冒称本次生产 patch。
5. MD9“HTTP 路径”来自模型，夹具只实现进程内 Router，没有 HTTP 传输证据；并入既有 r1074 模型措辞 P2。不得以这一错误新增原文关键词硬门或系统改写；单次输出也不足证明波动概率。

## Python：正式执行与独立后验分账

结果目录 `eval/results/github_issue_dateutil_relativedelta_float_symptom-20260915-204442`；
plan `plan-1789530374963772000-75111`，正式报告为同此前缀的 `.report.json` / `.final.json`。

1. durable commit `9589511b835fbccab8973fbf9bbc186915eb2ea4` 仅修改 `relativedelta.py`（+11/-2）：months/years 共用正规化，整数值 float 转 int，其余有限非整数 float 拒绝；原非 float 路径保留。README/原测试不改。
2. 实际执行在 `run-1.logs/codrax-20260915-204615-000-76834.log:677–686`：`/usr/bin/python3 -m unittest "test_relativedelta.py" -v` 一次，退出0、47.614458ms、原有四条断言均通过，不是只导入模块或扫描源码。
3. 当前 plan 的 `post_apply_verify`、目标行为能力、精确项目测试合同引用相符，正式终态 `verified/all_batches_verified` 与 strong 有依据。hard0、required soft1 `calendar-arith-type-safe` 满足；九条 planning 并不等于九条独立业务要求已各自动态证明。
4. `probe_count=1` 实际数的是一条 `VerificationConfidence` 项目测试观测，并非执行了动态 probe；`project_runner_commands=2` 是语法预检加一次 unittest，不是测试重复两次。命名债继续留账，不篡改原计数或原报告。
5. 初始两批提案在规划阶段收敛，源码误路径读取后恢复；无新的 JSON 合同自冲突。最终 applied-tree、保留 worktree 和 durable commit 三份源码 blob 一致，scratch main 仍为种子 `4adfe998d791`。
6. 独立后验 `.codrax/tmp/r1080-dateutil-postcheck.Sq6g2v/r1080-delivery-postcheck.log:17`：**572/572**，含原四测试、504格日期算术、16格有限小数拒绝、12格整数值 float、32格原月末/年份边界和4格非日期操作；原实现139通过/433失败。NaN/±Inf六条仅记录观察，不新增题目未约定的异常合同。历史 r1070 只作 harness 烟测，不算本轮重跑。
7. 这些后验仅用于人工验收，不发送模型、不回填正式证明。源码/原工件18项保全见该目录 `r1080-preservation.log`。

## 新发现与收尾修复

**B1678 诊断一致性 P2**：原 `.final.json:441–450` 的 required placeholder 已 `covered`，却仍保留 `behavior_contract_observation_missing` 和缺失说明；431–438邻接真实项目测试观测是有效的。这不是证明假绿，而是现有 missing→covered 转移只改状态、未同步说明；changed_symbol 同构分支亦有缺口。

修复提交 `fb362358c` 仅在这两个现有成功转移中更新占位义务的 ReasonCode/Detail，以“累计证明”描述本层实际权限，不改变覆盖键、PlanID、资格、状态判据、原始 covered/unverified 观测或终验；不扫描 Detail 文本，不添加模型字段。公共 BuildLedger/Profile/FinalReport 初版七场景先红后绿，末版追加跨 PlanID artifacts 同合同闭合/不同合同 derived veto、ID/PlanID/ReportPlanID 保全及历史 covered 旧文案不迁移。旧原件不写回、缺失与无关引用反控、重复构建、JSON往返及输入不变性均保留。
原 RED/初版 GREEN 收据保留在 `.codrax/tmp/20260915-b1678-ledger-diagnostic-*`。末版冻结 `final2-{count3,race3,types,freeze-verify}.log` 分别 PASS 0.752s/2.158s/44.735s/指纹一致；独立冷审和末版复审无阻断。附加调度器整包 `20260916-b1678-diagnostic-orchestrator.log` PASS15.788s，LLM默认与活跃流专项count3 `20260916-active-stream-boundary-count3.log` PASS12.939s。本修复不借用前一提交全仓86包收据冒称新的全仓验证。

## 保真、覆盖限制与下一优先级

- 原 case/fixture：`.codrax/tmp/20260916-r1080-{cases,fixtures}-before.sha`；原答案/HTML/机器汇总：`20260916-r1080-output-before-audit.sha`；原日志/out/verdict/metrics/正式报告：`20260916-r1080-formal-before-audit.sha`。全部只读人审，不修饰结果。
- 前一引用修复末版全仓86个有测试包通过；本批自然 full 引用正证成立。图/Trace 的生产覆盖均 N/A，未宣称显式窗、因果投影、链上双轴、IO/语义/业务线索或 root 旁路获得新生产验证。
- 真实请求日志仍为首响应600s、流中实际字节静默300s、非流600s。短请求不是长活跃流生产回归；额外专项已通过心跳续期、4ms间隔分段字节、无流式HTTP总年龄上限及调用方取消的确定性回归。没有新增4ms/旧4m无正文降级，caller deadline 等独立预算保留。
- 下一优先级仍是跨语言原生证明资格、完整关系/时序/逻辑表达及 Trace 模型解释；`probe_count` 命名、unknown/source_static_only 说明继续 OPEN。没有新P1或第二轮追绿；不把尚存模型措辞归咎于系统JSON，也不以关键词拦截替代证据供给。
