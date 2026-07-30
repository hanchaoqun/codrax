# TTY 单 stdin owner 专项设计（T-0）

- 日期：2026-07-29；前置账本：`docs/design/pi_borrow_analysis_20260729.md` §7.6（PIB-2 v1 非 TTY 队列已落地；TTY 全形推迟至本专项）
- 目标：TTY 交互下 pipeline 运行期不锁输入——输入排队回放（follow-up）、esc 中止还原、后续阶段边界注入（steering）；同时根治输入层三处现状真缺陷。
- 本文档兼作专项账本：§5 批次表随落地更新。

## 1. 普查定谳（全部消费点与关键事实）

**TTY 交互车道（r.in==nil 且 stdin 是终端）**的 stdin 消费点：
1. `native_input.go` raw 编辑器（**默认**，非 fallback）：per-prompt `MakeRaw`+新建 `bufio.NewReader(os.Stdin)`+返回即弃；ESC 超时用 fd 级 `unix.Select`（旁路 bufio 缓冲）；bracketed paste 仅 prompt 窗口开启。
2. `readInputBubble`（bubbletea v1.3.6）：**仅当** `!IsTerminal(0)` 或 `MakeRaw` 失败时的 fallback；无 `WithInput`，bubbletea 自管 raw（对非 tty 输入不 raw）。
3. 交互态 captureScanner（`/log` 粘贴、`/paste`）：cooked 态每次新建 scanner 即弃。
4. huh `NewConfirm` 七站点（/approve、/merge、/clear 等）：自起 tea program 直读 os.Stdin 自管 raw；输出写 os.Stderr。
5. Run 期间：**无任何读取者**（cancelListener 在 interactive 时被显式挡掉）。

**脚本车道（r.in!=nil）**：长寿 `r.scanner`（有意复用防 read-ahead 丢失）+ 非 TTY cancelListener 队列（PIB-2 v1）。**本专项不动脚本车道**。

**关键版本事实**：bubbletea v1.3.6 `initInput` 只在 input 是真 tty File 时 MakeRaw——`WithInput(pipe)` 时 bubbletea 完全不碰终端模式（模式管理权可完整归 owner）；huh 有 `Form.WithInput` 可注入。

**现状真缺陷（本专项顺修）**：
- D1 type-ahead 静默丢字节：per-prompt bufio/scanner 弃置时缓冲内已读入的预输入丢失（普查风险#1）。
- D2 强杀路径遗留 raw 终端：`installCancelSignalHandler` 双击路径 `os.Exit(130)` 不跑 defer Restore（风险#3）。
- D3 idle prompt 单次 Ctrl+C 直接退出，与 handler 注释宣称的"首次只警告"矛盾（raw 下 Ctrl+C 是 0x03 字节非 SIGINT，普查 §5）——本专项落地时显式定其语义。

## 2. 架构决策

**核心**：`ttyStdinOwner` 单例——REPL 生命周期内 os.Stdin 的**唯一** `*bufio.Reader` + 终端模式（raw/cooked）的**唯一**管理者。消费点不再自建 reader/自管 MakeRaw，改为向 owner **借窗口**：

- `borrowRaw()`：raw 字节窗口（native 编辑器、T-2 的 Run 期队列读循环）。owner 提供 buffered-aware 的 `readByteWithTimeout`（**先查 `Buffered()>0` 再 fd select**——共享缓冲后 ESC 超时探测若仍直接 select fd 会在缓冲残留时误判裸 Esc；这是共享化的必修细节）。
- `borrowCookedLines()`：cooked 行窗口（交互态 capture 粘贴）。同一 bufio 缓冲跨窗口连续——type-ahead 字节保留到下一窗口（D1 的修复本体）。
- 模式借用带恢复保障：Restore 挂进强杀路径（D2 修复：exit 前显式 Restore）。

**收窄面（均落账，不做）**：
- bubbletea fallback 车道保持现状：它只在非 TTY 交互或 MakeRaw 失败时可达——前者不属于 TTY 车道、后者 owner 自己也无法 raw，让位旧路径是正确降级；且 cancelreader 对非 File reader 不可取消（Quit 挂 500ms），接管性价比为负。
- huh 七站点暂不接管：全部运行在 prompt 间隙（cooked、无并发读者），现状无竞态；type-ahead 残留字节在 confirm 窗口不可见（现状=弃置丢失，新=滞留 owner 缓冲、confirm 后的下一 prompt 重现）——两者都"huh 读不到预输入"，行为等价，滞留重现还是修复方向。T-3 后视需要经 `Form.WithInput` 收编。
- `!cmd` 的 Stdin=/dev/null（交互命令拿不到终端）：已知限制，非本专项。

**Run 期回显纪律**：owner 的任何用户可见输出（入队回显、取消提示）必须经 Renderer 加锁提交（新增最小公开面 `Renderer.CommitUserInputLine`），禁止直写 r.out——dock 的 `\x1b[4A` 重绘计数不容第三方写者（普查风险#11）。

**EOF 语义**：owner 读到真 stdin EOF → 设 sticky eof 标志，所有后续 borrow 立即 EOF（与现状"Goodbye!"路径一致）；不引入 pipe 中间层，无 pipe-EOF 与真 EOF 混淆问题（风险#6 经架构选择消解）。

## 3. 批次计划

| 批 | 范围 | Pin 要点 |
|----|------|---------|
| **T-1 纯重构** | `ttyStdinOwner`（共享 bufio + 模式借用 + buffered-aware ESC 探测 + 强杀 Restore）；native 编辑器接 borrowRaw；交互 capture 接 borrowCookedLines；其余车道不动 | ① type-ahead 保留（prompt A 尾部预输入在 prompt B 可读——现状丢失，新行为 pin 并在账本明示这是 D1 修复而非回归）② 既有 native/input/capture 测试全绿 ③ 强杀路径 Restore 被调（注入 restore hook 断言）④ ESC 序列跨缓冲边界不误判（buffered-aware pin） |
| **T-2 运行期队列** | `runInFlightWrap` TTY 分支向 owner 借 Run 窗口：raw 读循环 + 最小行组装 + 入队（**drain 汇入 PIB-2 v1 的 `r.pendingFollowUps` 回放通路**，TTY/非 TTY 汇合）；0x03 走既有单击/双击取消语义（字节路与 SIGINT 路同一实现）；esc=中止 Run 且队列还原为下一 prompt 预填；入队回显经 Renderer | ① 运行期输入入队+回放顺序 ② 0x03 单击=cancel 双击=退出（与 cooked SIGINT 语义一致）③ esc 还原队列 ④ dock 无第三方写者（回显走 Renderer 锁） |
| **T-3 注入与捆绑件** | steering 阶段边界注入（队列行中 @path 经线程安全通道 → 下一 explore 窗口 `AddPendingRead`+`runForcedReads`；自由文本 → `RetryHint` 车道）；@ 补全与粘贴折叠在单一输入层上重估 | 按 T-2 落地后现状定，先设计后动手 |

**D3 裁定（T-1 内落地）**：idle prompt 的 Ctrl+C 保持"清行/退出"编辑器语义不变（native 现状），但 Loop 侧把编辑器 aborted 与真 EOF 区分，Ctrl+C 首次给"再按一次退出"警告——使实现与 handler 注释的承诺一致。若实现中发现该语义牵连过宽，允许收窄为"维持现状+改注释"，落账。

## 4. 风险映射（普查 13 条 → 处置）

#1 type-ahead→T-1 修复本体；#2 idle Ctrl+C→D3 裁定；#3 raw 归属/强杀遗留→owner 唯一管理+强杀 Restore；#4 脚本态双读→不动（v1 已单独治理，direct-LLM 不起 listener 的现状维持）；#5 bracketed paste 双开关→owner 统一（prompt 窗口开、Run 窗口关，T-2 粘贴多行逐行入队；T-3 重估折叠）；#6 EOF 三义→sticky eof 架构消解；#7 /exit→owner shutdown 挂 Loop 退出路径；#8 三处上限不一致→owner 行组装用 cancelListener 同款 1 MiB 帽+超帽披露；#9 Windows→T-1 保持 per-平台 readByteWithTimeout 原语不变（owner 包装层缓冲优先，平台层只在缓冲空时触达）；#10 huh stderr→收窄不动；#11 dock 并发→Renderer 锁提交纪律；#12 GetSize(0) 硬编码→顺手改显式 tty fd；#13 账本交叉引用已更新。

## 5. 落地记录（随批更新）

**T-1 已落地（2026-07-29，main=ce30508bb）**：`internal/repl/stdin_owner.go` — `ttyStdinOwner`（REPL 生命周期唯一 bufio.Reader + 唯一模式管理者；`borrowRaw`/`borrowCookedLines` 互斥窗口 + release 幂等；`makeRaw` 可注入供测试）。接线三处：native 编辑器 prompt 窗口借共享 reader（不再 per-prompt 自建+自管 MakeRaw）；交互 capture（/log 粘贴、/paste）借 cooked 行窗口（`captureScanner` 接口化 + release，borrow 失败降级旧直读并告警）；强杀路径 `os.Exit(130)` 前 `restoreTTYForExit()`（D2 修复）。ESC 超时 Buffered() 快路径经核实现状已存在（native_input_unix.go），共享化天然安全。D3 收窄：编辑器 0x03 语义保持现状，矛盾点是 handler 注释的过度承诺——T-2 引入 Run 窗口字节路时一并统一表述。pin 四组：type-ahead 跨窗口零丢失（D1 判决性证明）、窗口互斥+release 幂等、强杀恢复 exactly-once + nil 安全、行扫描器 scanner-parity。全仓 83 包零 FAIL。

**T-2 已落地（2026-07-29）**：owner 新增 Run 输入窗口 `borrowRunInput`——tick 轮询（100ms，复用平台 `readByteFDWithTimeout`，goroutine 永不无限期 park 在 Read，drain 一个 tick 内归还，普查"跨界悬挂 Read"隐患结构性消除）+ 最小行组装（1MiB 行帽/32 条队列帽=与既有两车道同族，超帽一次性披露）。三键语义：**Ctrl+C 字节 → `raiseSelfSIGINT` 自举回唯一 SIGINT handler**（单击=取消/双击=退出与 cooked 信号路完全同一实现；Windows 无自举回退直接单击取消，账本披露）；**裸 ESC 才中止**（ESC 后一个 tick 内有后继字节即判转义序列——CSI/SS3 吞到终结符，方向键误触不可能取消 Run，判决性 pin）+ 队列**还原不回放**（单行→prompt 预填，多行→既有粘贴占位符车道折叠还原）；普通行入队并经 `Renderer.CommitUserInputLine` 加锁回显（dock 重绘算术零第三方写者）。排干汇合：正常结束队列并入 PIB-2 v1 的 `pendingFollowUps` 回放通路（TTY/非 TTY 单一回放面），半行成为下一 prompt 预填（运行期击键零丢失）。平台原语抽取为包级 `readByteFDWithTimeout`（unix/windows 双实现，编辑器方法委托，行为不变）。pin 六组：排队+回显+空行不回显、CSI 吞掉/裸 ESC 中止判别、Ctrl+C 路由+半行移交、排干双车道（回放/还原×单多行）、坏 fd 退避不 wedge、既有全套存活。全仓 83 包零 FAIL。

**T-3 已落地（2026-07-29，T-3a 全形 + T-3b 重估裁定）**：
**T-3a 真 steering**：`internal/orchestrator/steering_notes.go` — `steeringIntake`（Run 生命周期开/关、线程安全 push/drain）；REPL 运行窗口每行**先 offer 给运行中管线**（`steeringSink.PushSteeringNote`，接受即回显「已注入本轮探索」不入队），explore 每个 window 边界消费：@path 钉选（复用共享 `internal/userhint.ExtractPinnedFiles`——从 repl 移出成单一实现）→ `AddPendingRead(Origin=explore.user_steering_pin)` 汇入紧随其后的 `runForcedReads`（explorer LLM 看到任何东西之前真正读完），自由文本 %q 引用为 data 追加进既有 window-hint 车道（软引导，精确/嘈声原则合规）。**接受未消费的 notes 在 drain 时经 `TakeUnconsumedSteeringNotes` 一次性交还**并排在回放队列最前——steered 或 replayed，不丢失、不双跑（写模式 Run 无 explore 边界时自动全量回退为回放，零特判）。orchestrator god-file ratchet 再次触发，按其本意瘦身调用点（说明集中在 concern 文件）。pin 五组：intake 生命周期（关闭拒收/空白拒收/交还 exactly-once）、边界消费（真实文件钉选入 PendingReads 带 origin + hint 逐字引用 + 消费后不再交还 + 空 drain 零改动）、窗口 steer 旁路队列（sink 拒绝后回落排队）、未消费交还排序在前。
**T-3b 重估裁定**：粘贴折叠——普查证实 native 编辑器**已有**占位符折叠（bracketed paste + injectPlaceholder + pasteFoldMinChars），PIB-5b 当初按 bubbletea 假设的收窄系过度收窄，现状已覆盖，无需开发（pi 借鉴账本 §7.5 该条勘误指向本记录）；@ 自动补全——纯 polish 件，锚定设计（native 编辑器 refreshSuggest 机制 + repomap FileIndex 候选源 + 前缀/子串两档打分），效益不再被竞态问题绑定，留待常规迭代，不占本专项。
**专项收官**：T-0/T-1/T-2/T-3 全落地；pi 借鉴账本的「PIB-2 TTY 全形」候办就此关闭。

**尾件消化（2026-07-29 续）**：① **@ 补全已落地**（internal/repl/at_file_suggest.go）：native 编辑器 filterSuggestions 漏斗新增 @ 分支（slash 建议恒胜，仅在其空时查询）；会话级懒建文件索引（一次有界 walk，cap 5000，跳过 .git/node_modules/vendor/.codrax/隐藏目录，每击键只过滤内存切片零 fs 访问）；pi 式四档打分（basename 精确 100/前缀 80/含 50/全路径含 30）+ 同档 basename 短者优先平局；整值替换只改尾部 @token。pin 两组（打分序+边界负臂含 email/裸@/cap、索引跳过规则精确集合断言）。bubbletea fallback 车道不加（既有收窄裁定维持）。② **按 turn-id 导出已落地**（memory.Store.Turn(id) 导出访问器（in-memory recent 优先）+ /export turn-id 分支：仅问答不含附件——sticky 附件属于当前会话不属于历史轮，确认消息明示；未知 id fail-loud 点名；/help 子命令注册）。pin 一组（by-id bundle 形+无附件负臂+未知 id）。


## 6. 客户事故复盘（2026-07-29/30，两案并行）

**案一：REPL 显示/挂死回归（我方 T-2 引入，已止血 58ea9f91c）**。三症状一根因族：Windows 运行窗口 raw 读被非按键控制台记录诱导 park → drain 死等 → REPL 冻结（"整理上下文中"卡死）与输出积压（敲键才批量倾泻）；unix 侧 MakeRaw 清 OPOST 的楼梯化隐患同批预防性修复（cbreak 保 OPOST/ISIG）。止血=Windows 平台整体禁用运行窗口+drain 3s 保险丝+`CODRAX_DISABLE_RUN_INPUT` 总闸。**流程教训（教训条款）：平台不可本地验证（CGO 交叉编译不可行）的输入层改动，必须平台门控默认关闭再逐平台放开；"unix 推理+Windows 放行"不可复现。**

**案二：零探索捏造答案（触发源待客户证据，捏造授权面已封死 FABGATE-1）**。调查定谳（agent 全文另存）：`› 2/4 正在生成最终答案` 的 2/4 来自 soft-notice 的"下一个未开始 slot"投影——**铁证=finalize 通知时渲染器从未见过任何 explore 行**；auto-resume 本案**排除**（复活必打的披露行在 transcript 缺席）但其设计风险实存→已按用户裁定默认关闭+/clear 连清（08a93455b）；确证的捏造授权面=`applyRuntimeTraceSourceOptionalSurfacePlan` 允许"裸路径引用+零附加+零观察"铸 source-optional 豁免，四道门（tier1 零证放行/ta==nil 旁路/引用下限被剥/观察无下限）真空放行。**FABGATE-1 已落地**：裸引用+零附加+零 ledger 工件+零确定性 trace 观察 → 不铸豁免（四判据全 typed 精确信号；判决性 pin=客户形状负臂+附加车道不回归臂）。**fix ① 撤回记录**：ta==nil 臂两次尝试均撞真墙（stub 工件形/既有 pin `IgnoresUnknownState`="未观测状态≠结构空"是有意裁定）——正确落点=forced-finalize 逃生舱（orchestrator.go:5424-5449）加"零投资节点不得强制成文"门，**待客户日志确证 S2 触发后再动**（需 grep `blocked on entry conditions; forcing finalize` / `DAG scheduler stalled`）。**候办 FABGATE-2**：runtime-observation 出厂下限（disposition 激活+external_observation 块 ⇒ ≥1 条确定性 runtime 观察，精确信号 `HasDeterministicRuntimeQueryObservation` 现成，emit 层 typed reject+escape lane）。**向客户取证清单**：① `codrax --version` 的 buildRevision 对表；② `.codrax/logs` grep 上述两行+`stop condition fired`+`read run snapshot seed applied`。规避=显式 `/htrace <路径>` 附加（正规车道，涉事形状不存在）。


## 7. 回归 sweep 与 REGFIX 四批（2026-07-30）

客户两起事故后对当日全部批次做了 25-agent 五维对抗复核（并发挂死 / 平台兼容 / 管线语义 / 显示 / REPL 动词），**19 条确认缺陷（3 high / 16 medium）、1 条证伪**，全部落在我方批次内。分四批根修，全推 main。

**REGFIX-A（运行窗口生命周期，6 条）**：#0(high) 保险丝的放弃路径把 run-N 回调留在会话级对象上——迟到解冻的 reader 能取消未来 Run、把陈旧半行当 steering note 注入其 explorer prompt、并吞掉按键（修：原子 dead 标志，drain 前置位、每次读后与每次回调前复查）；#1 保险丝永不 release 使 owner 永久 brick 且 cbreak 泄漏到正常退出（修：release-on-done watcher + `/exit`/EOF 路径恢复终端）；#2 `cancelToken` 裸指针 -race（改 atomic.Pointer；取消面移入 cancel.go 化解 LOC ratchet）；#3(high) `unix.Select` 的 EINTR（拖 tmux 分屏的 SIGWINCH 风暴即可）被 ESC 探测当错误上抛 → Loop 判会话结束、打印 Goodbye 并丢失已输入行（修：剩余预算内重试）；#4 任何探测错误被当裸 ESC 取消 Run；#5 DEL 作为裸字节进入队列文本（改按 rune 编辑）。

**REGFIX-B（steering 语义，4 条）**：#9(high) 并行 explore 车道先组装 worker hint 再 drain steering，worker 又用 pre-drain 值覆盖 clone hint → 文本回显“已注入”却到不了任何 prompt，且因已 drain 也无法回放（修：drain 返回文本，调度器合并进每个 worker hint）；#10/#15 mid-run 斜杠命令与 `!cmd` 被当 steering 散文吞进 prompt（intake 拒收命令行，交由回放漏斗执行）；#11 mid-run @pin 绕过 PIB-5c 的显式 source-exclusion 边界（同款钳制：文本仍进 prompt，仅扣留强制读）；#13 回显在 intake 接受时就宣称“已注入本轮探索”（改为“已交给本轮处理，未采纳则回放”）。

**REGFIX-C（交付面与平台/显示，5 条）**：#8(high) `MergeFromRef` 新分支车道枚举了全链却只 cherry-pick tip——早于 tip 的提交永不落地而 `/merge` 报成功（其文档契约本就写 commits 复数；W-3 的 partial checkpoint 让这个潜伏遗漏可达）；#7 Windows `term.GetSize(stdin)` 失败 → 宽度恒 80 → 帧算术错乱（改 fd→stdout→stderr 递降探测）；#12 usage 只在首个 objective 归零（analyze 后才发、写模式永不发）→ 显示陈旧账（改 pipeline start 归零）；#14 `dockWidthCond` 由窄翻**宽**——仍钉死、仍 locale 无关（BARGRID-1 红线是“禁包级默认”而非“必须窄”），方向改为 fail-safe（欠量会溢出 → 终端换行 → 光标回卷少算 → 行漏进永久滚动区，正是客户症状类；过量只是行短），并修掉省略号自身宽度未预留导致的一列溢出；固定保险列方案先试后否（全歧义行可近两倍窄测量值）。#6 Windows 控制台非按键记录导致的阻塞读**记档不盲修**——正确修法（PeekConsoleInputW 丢弃非按键记录）无法在本机（unix + CGO）编译验证，按复盘的平台裁定不放行未验证的 Windows syscall 代码；仅做有界切片等待，并就地记录限制与既有围堵（运行窗口在 Windows 永不 arm）。

**REGFIX-D（REPL 动词面，3 条）**：#16 回放绕过输入漏斗（`!cmd` 与模板命令被当 LLM 提问送出 → 改为走同一漏斗：shell-bang → 别名 → 模板 → dispatch）；#17 `/import` 违反自身“篡改零挂载”契约（log 先挂载才校验 trace；剥掉 hash 的 manifest 全放行 → 改为全部校验通过后才动 sticky 车道，缺 hash 视为不可验证并拒绝）；#18 native 编辑器 Enter 劫持尾随 @token（可见文件建议时自动接受最高分项并提交 → Enter 只接受斜杠建议，文件建议仍可 Tab 显式接受）。

**教训固化**：① 我自己写的止血保险丝成了新缺陷源（#0/#1）——任何“放弃/降级”路径必须**先失效化再放手**，且必须保留归还资源的回路；② 平台不可验证的改动一律记档 + 围堵，不盲修（#6 按此裁定处理）；③ 对抗复核必须覆盖“当日自己的批次”——本轮 19 条无一由同事引入。


## 8. Windows console peek 证明式修复档案（REGFIX-C #6 根修，2026-07-30）

用户裁定：无法本机运行验证 ≠ 不能修——按公开资料做**证明**去修复。本节即证明档案；代码=`internal/repl/console_input_record.go`（平台无关判定核心）+ `native_input_windows.go`（syscall 薄壳）。

### 8.1 证据源（四路独立）

1. **微软官方文档**（learn.microsoft.com/en-us/windows/console/*：input-record-str、key-event-record-str、focus/menu-event-record-str、peekconsoleinput、readconsoleinput、readconsole、setconsolemode、low-level-console-input-functions、reading-input-buffer-events；win32/api/synchapi/nf-synchapi-waitforsingleobject、win32/sync/wait-functions、win32/winprog/windows-data-types）。
2. **本仓锁定版本依赖源码**（$GOMODCACHE：x/sys v0.44.0、x/term v0.41.0）——x/sys 无 Peek/ReadConsoleInput/INPUT_RECORD（普查零命中），但有事件类型常量（KEY_EVENT=0x0001 等，与文档值一致）、WaitForSingleObject、NewLazySystemDLL；x/term Windows makeRaw 清 ECHO|PROCESSED|LINE 并**置 ENABLE_VIRTUAL_TERMINAL_INPUT**（编辑器车道处于 VT 输入模式）。
3. **四个实战 Go 实现互证**（mattn/go-tty tty_windows.go、gdamore/tcell v2.7.4 console_win.go、Azure/go-ansiterm winterm/api.go + moby/term 消费端、erikgeiser/coninput=Bubble Tea Windows 依赖）：20 字节布局四家全一致（三家显式 padding+[16]byte payload，tcell 另以手工字节偏移 0/4/6/8/10/12 解码独立佐证）；全部只从 kernel32 懒加载 W 变体；coninput 是唯一同时加载 PeekConsoleInputW 者且用 windows.NewLazySystemDLL（本修同款）。
4. **仓内普查**：Windows 上本原语唯一可达车道=真控制台上的 prompt 编辑器 ESC/CSI 探测（25ms），IsTerminal + MakeRaw(GetConsoleMode) 双闸把关（后者非字面 IsTerminal，但同一判据、非控制台句柄同样拒绝）；运行窗口结构性不 arm（run_input_mode_other.go 无条件报错）；`d<=0` 全仓无调用点（顺手把 Windows 版对齐 unix 即时轮询语义）。

### 8.2 布局证明（推导，非文档原文——文档只给类型不给偏移）

Windows 数据类型（winprog/windows-data-types）：WORD=16 位、BOOL=int=32 位、DWORD=32 位、WCHAR=16 位。INPUT_RECORD＝WORD EventType@0 + 2 字节自然对齐 padding + 16 字节 union@4（KEY_EVENT_RECORD 为最大成员，其首 BOOL 强制 4 对齐），总 20 字节；KEY_EVENT_RECORD 内偏移：bKeyDown@0、wRepeatCount@4、wVirtualKeyCode@6、wVirtualScanCode@8、uChar@10、dwControlKeyState@12。无成员超过 4 字节对齐 ⇒ 386/amd64/arm64 布局同一；Windows 全架构小端。四家实现与该推导逐字节一致。宿主机可跑 pin：`TestConsoleInputRecord_WireLayout`（sizeof=20/offsetof 断言——Go 对这些字段形状的布局与 GOOS 无关，宿主断言即真断言）。

### 8.3 「永不更差」论证（判定核心的正确性骨架）

分类唯一职责：peek 到队首记录后判「确证无字节 ⇒ 丢弃」或「可能有字节 ⇒ 提交阻塞读」。两类错误不对称：**误提交**=退回修前行为（阻塞等待，即今日线上行为）；**误丢弃**=丢用户输入（不可接受）。故一切不确定分支落提交。丢弃集逐项证明：

- **非 KEY_EVENT 记录**（focus/menu/mouse/resize/未知）：文档原文 focus/menu "used internally and should be ignored"；mouse/resize 文档明言被 ReadFile/ReadConsole 丢弃（resize 只有 ReadConsoleInput 能读）。⇒ ReadFile 永不为其产字节；我们用 ReadConsoleInput 提前丢弃与 ReadFile 自身行为**等价**，仅少了那次可能永不返回的阻塞等待。这正是客户 wedge 的机制闭环：句柄 signaled 条件=「缓冲区非空」（文档两句合取），不限记录类型。
- **key-UP（除一例外）**：ReadFile 只从 key-DOWN 提取字符——tcell 与 moby/term 十年生产无条件丢弃全部 key-up 而无丢键报告，是最强实践证据；且 key-up 常回显 uChar（go-tty 的 numpad guard 逻辑反证），故本臂**不得**以 uChar 判定，否则每次击键尾随的 release 会让探测继续阻塞、修复失效。唯一例外：**Alt+numpad 合成字符落在 VK_MENU 自身的 key-UP 且 uChar≠0**（go-tty 专门 arm 佐证）⇒ 该形提交。
- **无字符修饰键 key-DOWN**（SHIFT/CTRL/MENU/CAPITAL/L·RWIN/NUMLOCK/SCROLL/VK_LSHIFT..VK_RMENU 闭集白名单）：本身永不产字节；字符翻译在记录**入队时**已完成（uChar 即译文、dwControlKeyState 每记录自带修饰态），丢弃修饰键记录不影响后续记录的译文。三家实现均静默吞掉未映射裸修饰键。
- **其余一律提交**：uChar≠0 的 key-down（可打印/控制字符 0x03、ESC 0x1b、IME VK_PACKET、VT 模式下方向键序列字节）、uChar==0 的非修饰 key-down（死键、IME 预组合 VK_PROCESSKEY 0xE5、非 VT 方向键、Ctrl+Space 的 NUL——闭集白名单的存在理由）、畸形 payload、Peek 不可用/失败、ReadConsoleInput 失败、n==0 竞态（continue 重等，预算封顶）。

**前置稳定性前提（对抗复核 low 判定，记档为约束）**：「丢弃消费的就是 peek 到的那条」依赖记录只从队尾入队。conhost 源码确证：repeat 合并只改 `_storage.back()`、Read 从队首弹出——单 stdin owner 前提下成立。已知历史例外：2019 前的 conhost（GH#1637 之前）把 VT 查询应答（如 ESC[6n 的光标位置报告）**前插**队首；本仓 Windows 车道零 VT 查询发射（grep 确证），故不可达。**约束**：未来若在 Windows 上加任何光标探测/VT 查询特性，必须先重估本前提（老 conhost 上 peek-丢弃间隙的前插会让丢弃吃掉应答首字节）。

**有意的行为差异（非回归）**：预算到点时老代码会因任意记录 signaled 而**越权阻塞**在 ReadFile 里超过声明的 25ms 预算（正是 wedge 属性本身），新代码诚实超时、字节留待下一次读取——与 unix 孪生行为一致；等价场景下无字节丢失、无乱序。

### 8.3.1 conhost 源码级确证（对抗复核带回的正向真金）

复核 agent 直读 microsoft/terminal（conhost）源码，把三条实践论证升级为源码论证：① ReadFile 的 key-up 字符交付条件逐字为 `uChar != 0 && vk == VK_MENU`（ALTNUMPAD_BIT 只影响解码不影响交付）——分类器 key-up 臂的提交条件与之**恰好相等**，等价性从「实践证据」升为「实现定义」；② 唯一从无字符 key-down 合成字节的车道（Ctrl+Space/Ctrl+@ 的 NUL，经 VkKeyScanW(0)）结构上不可能落在修饰键 VK 上（VkKeyScanW 只返回产字符键的 VK 或 0xFF）——修饰键白名单丢弃臂由此闭环；③ Alt+numpad 组合出 NUL（Alt+numpad0）时 ReadFile 同样不交付（GetChar 外层 uChar!=0 门先行）——分类器丢弃 VK_MENU-up-uChar==0 与之精确等价；④ Go Alignof 2 vs C 4 的分歧已用零宽 [0]uint32 前导字段抹平（尺寸/偏移不变，pin 断言 Alignof==4）。

### 8.4 验证矩阵（能验的全验了，不能验的如实披露）

- 判定核心 27 例真值表 + 布局 pin + 事件常量 pin：**宿主机真跑全绿**（console_input_record_test.go）。
- syscall 薄壳：隔离模块（副本 diff 字节一致 + nativeLineInput stub + x/sys v0.44.0 锁定）`GOOS=windows CGO_ENABLED=0 go build` **amd64/386/arm64 三架构全过** + `go vet` 干净。调用模式=syscall.SyscallN(proc.Addr(), …uintptr(unsafe.Pointer(&x))…)，与 x/sys 生成代码同款（编译器保活语义）；proc.Find() 前置探测避免 Addr() 缺失 panic。
- **未验证并披露**：真 Windows 控制台上的运行时行为。故运行窗口在 Windows 维持不 arm（收益面=prompt 编辑器探测不再被 focus/mouse/key-up 记录卡住）；待真机后再议重新武装。
- 残留已知项（记档不扩批）：控制台代码页——仓内从不 SetConsoleCP(65001)，中文 Windows 默认 OEM 代码页下 ReadFile 返回 GBK 字节而编辑器按 UTF-8 解码，系**修前既有**的独立缺口，与本修无交互。


### 8.5 对抗复核记录（提交前，wf_7e1f7a04-6e8）

四路独立证伪（布局重推导/永不更差反例构造/Go-Win32 调用力学/集成与档案事实核查）：**4/4 sound，零 high/medium**。2 条 low 均已吸收：前置稳定性前提落 §8.3 约束条款；「双重 IsTerminal」措辞收紧。复核另独立复现了隔离编译矩阵（三架构 build+vet 离线全过）、逐项核对档案事实（27 例计数、版本、普查断言全部对上），并确认旧「记档不修」注释全仓零残留。机制审查全绿：SyscallN 四参形正确且保活、Find() 成功缓存、WAIT_TIMEOUT 类型双关经显式转换无害、无热旋（每条 continue 都回到内核等待且被 deadline 封顶）、bufio 无饥饿（丢弃车道只动控制台缓冲、永不动 bufio）。


## 9. 第二轮自查 sweep 与 SWEEPFIX-EF/G/H（2026-07-30）

按 §7 教训③（收工必对本人当日批次做对抗复核），对 REGFIX-A/B/C/D + 事故响应件（止血/auto-resume 默认关/FABGATE-1）+ WINPEEK-1 吸收增量做了 46-agent 六维 sweep（六路 finder → 每条 finding 双镜头 refuter 证伪）：**20 条幸存、0 条被驳**（refuter 双确认全为真缺陷），三批根修全推 main。

**SWEEPFIX-EF**：S0 commitLine 先清 partial 后查 dead → Enter 撞上 drain 时整行蒸发（改全程持锁、dead 先查、死窗口保留 partial 给 drain 快照）；S1 dead 路径丢弃已消费字节（UTF-8 lead byte 变下个窗口的乱码 type-ahead）→ UnreadByte 归还；S2 保险丝 watcher 在 unwedge 的任意时刻恢复 termios、砸在 fallback 编辑器脚下 → abandon 即还终端（restoreModeEarly）、watcher 只还借用；**S7(high) cherryPickReportedEmpty 子串匹配自由文本**——提交主题含 "nothing to commit" 的真冲突被 --skip 静默丢弃变更而 /merge 报成功（主题是 LLM plan Summary，可达）；S8 匹配词组被 gettext 本地化（zh_CN 客户 locale 下修复完全不生效）；S9 --skip 在 git<2.23 不存在 → **根修=删除整个字符串匹配**，改 `git cherry` patch-id 等价**预计算**（plumbing 机器格式、LC_ALL=C、fail-open 到不跳过=响亮中止不丢数据）；S10 worktree 车道补同款跳过（两车道同链同行为）；S11 补四个判决性 pin（全链落地/链中等价跳过/S7 回归形/回滚完整）。

**SWEEPFIX-G**：S5（最锋利）运行窗口无括号粘贴车道——粘贴的 "!git clean -fdx"（聊天记录/jupyter/markdown）逐行进 commitLine、被 intake 拒收、排队、**被回放漏斗真实 shell 执行** → 窗口识别 ESC[200~/201~，粘贴=单一 verbatim blob，follow-up 变 typed（pendingFollowUp{Text,Verbatim}），verbatim 条目回放永不进命令漏斗；S4 intake 拒收从「/ \\ ! 前缀」噪声启发式改为**漏斗自身精确谓词**（shell-bang 前缀+别名注册表；模板命令在 REPL trySteer 包装层拒收）——"/var/log/app.log 里有 panic" 这类路径开头散文恢复为 steering；S3 source-exclusion 边界下 hint 仍宣称「pins 已排队读取」=对 LLM 的假话 → 三臂真话词面；S6 回放顺序改为命令先、未消费散文后（散文通常预设命令的效果；真实交错不可重建，裁定落账）。

**SWEEPFIX-H**：S12 objective-start 残留归零在 read 模式擦掉 analyze 账（pipeline start 成唯一边界，旧 PIB-5 pin 改钉新裁定）；S13 direct-LLM 车道（/chat 等）补发 EventPipelineStart（上轮账不再渗入）；S14 tailByDisplayWidth 走包级默认条件与行帽混用两种几何 → 统一 dockWidthCond；S15 几何 pin 注释描述的是被否设计 → 改述已交付的 fail-safe 宽测量及否决理由；S16 Enter 劫持在斜杠行复活（"/write … @dock" 的面板来自 @-file provider 而分类按 "/" 前缀）→ 车道按**面板归属 provider** 判定（与 filterSuggestions 优先级同构）；S17 /import 镜像臂——manifest 声明 hash 而 payload 文件缺失/空/不可读时静默部分导入 → 整包响亮拒绝；S18 auto-resume 披露在候选安装时宣称「已恢复」而软校验仍可拒 seed 且无更正 → 候选语义词面；S19 ClearAll 按 JSON 内部 RunID 删（文件名≠内部 id 的复制件在 /clear 后存活为活候选、IsNotExist 空操作还计入删除数）→ 按文件名枚举删除、只计真实删除。

**过程教训**：① sweep 六路 20/20 幸存、0 误报——双镜头（正确性复现 + 设计意图/账本对照）refuter 组合有效；② 我上一轮在 REGFIX-C 里现写的 cherryPickReportedEmpty 是当天新增缺陷里最重的一条（high）——**修复批自身必须进下一轮 sweep 的靶面**，本轮正是这么抓住的；③ 输入层的"数据/命令"边界（粘贴、路径开头散文）是回归高发面，一切拒收/分类谓词必须与漏斗自身谓词同源。
