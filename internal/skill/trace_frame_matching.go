package skill

// TraceFrameMatchingSelectionTeaching combines the PerfSage preanalysis
// features and the frame review procedure for a Codrax-generated catalog.
const TraceFrameMatchingSelectionTeaching = `任务。给定目标 Trace 的一帧，在基线 Trace 中选择一个可对照的目录 ID。只做选帧。程序按 ID 校验来源并回填原始线程与边界。证据不足时返回 {"id":"none"}。

一、核对两侧应用、采集层和帧角色。
- 鸿蒙原生按已观测的 Ability、WindowStage、ArkUI 等事件定位应用和页面阶段。分开核对应用侧绘制、提交和 RenderService。用已证实的帧、对象或链路连接它们。
- 卓易通先定位客体应用、窗口和帧，区分客体主线程、渲染线程、桥接和宿主。客体应用帧可以独立比较。比较宿主合成或呈现需要已核实的跨层对象关联和时钟关系。
- 安卓原生按实际输入、Activity 和窗口事件定位阶段。Choreographer#doFrame 可定位应用主线程帧。RenderThread、合成和呈现各用自己的关联证据。
- 各侧按实际观测语义对齐，保持绘制、提交、合成和呈现为不同端点。平台、线程名、帧号和时钟数值本身不能建立对应。目录没有覆盖的层标未知。

二、用业务事件定位，再缩小候选。
- 在目标帧前后找归属于应用的可信锚点，例如应用接收到的输入、生命周期、窗口切换或明确的页面更新。到基线独立找同义事件或同序更新。记录帧是在事件前还是事件后、是第几次相关布局、绘制或提交。同一可信事件后的首个相关帧优先。
- 前后锚点都可信时，优先查基线对应区间。用经过前锚点的时间和距后锚点的剩余时间细化位置。两侧各只出现一次的点位可以成组交叉印证。只有当多个点位在两侧都紧邻，且前后两组的顺序一致时，才用它们估计局部进度。只有一个锚点时保留合理的多种位置。重复事件对齐不唯一时，用下一个可信事件重新定位。全段进度、采集起点和帧号只供导航。
- 对每个候选核对可比阶段、应用、窗口或页面、绘制及提交对象。区分首次布局、再次布局与重复重绘。明确场景冲突的候选只供参考。帧长和丢帧状态仅辅助参考排序。
- 原始窗口更新点位明确给出类型时，核对同一窗口生命周期中的首次布局和第几次仅参数更新。两侧都观察到首次布局时才比较后续更新的顺序。已观察到不同的窗口更新角色属于冲突，缺少点位属于未知。
- 核对视频解码输出进应用窗口内的纹理，还是进独立 SurfaceView。两侧输出路径已证实不同时，先比较常规更新节奏和业务阶段，异常长的帧间隔只供解释性能差异。缺少明确提交路径时沿用其他已观测的证据。
- 客体提交能按窗口和缓冲队列追到宿主合成时，可核对同应用的第二图层是否在同一轮合成中更新。两侧都在首个合成周期取得窗口缓冲时，这项证据只用于复核首选附近的候选。跨周期排队应单独作为性能差异解释。

三、逐项比较帧内与帧外行为。
- 优先复核约五个证据最强的候选。候选未查全或有同样合理的候选被省略时，保留歧义。
- 分别核对主线程及渲染线程的实际工作。比较帧内调用顺序和帧外业务事件顺序。相同点位出现在不同顺序或不同角色中，不能充当同一工作证据。通用动画或提交标签只支持已观察到的角色，动画驱动、提交对象及跨层关系需要实际证据。
- 低频点位使用程序给出的出现帧数和统计范围，按线程角色比较。低频且两侧归属、顺序相符的具名点位优先于通用绘制标签。重复调用只算一帧出现，总在同一批帧共现的嵌套点位合看。帧入口只用于定位。帧内相似时，检查帧前已提取的短时间低频调用组是否共有，以及它们怎样过渡到中心帧。稀有度本身不能证明对应。
- 每项分别判断支持、冲突或未知。完整覆盖下观测到的归属、触发、对象或顺序冲突可以反驳候选。缺采保留未知，已证实同一动作的一侧新增解码、布局、动画或等待可以是版本差异。

四、比较跨帧群体特征，反查首选。
- 对目标、首选和最强替代帧取同来源同逻辑线程前后各四帧。找同一具名点位或同一窗口动作的连续出现段。已采集的邻帧确认没出现该点位时才能确定段边界。比如 [无 measure, 有 measure, 中心帧有 measure, 有 measure, 无 measure] 的中心是三连 measure 的第二帧。比对中心距段首、段尾的帧数。
- 比较同一具名点位自身的逐帧用时。至少三个有效样本才判断趋势。8→5→3 ms 与 6→4→2 ms 支持同一下降阶段，2→4→6 ms 不支持这项依据。段内位置优先于趋势，完整帧耗时不能替代单点耗时。
- 分开核对前后三帧的实际起点间隔、窗口更新和动画起止。复用程序给出的跨帧点位组相对时序、邻域行为相似度和竞争信息。比较点位组相对中心帧的位置和过渡次序。相似的丢帧间隔或 resync 模式需要结合实际回调、渲染内容和阶段复核。确认首选不更适合解释另一轮重复操作或另一个目标帧。缺少这些字段时，标未知并使用已观测的证据。
- 提交前指出哪一条实测观测支持首选并能区分最强替代帧。连续段、趋势或普通点位组可支撑同阶段对照样本。确认唯一对应还需要同一业务动作或同序窗口更新的独立证据。只有位置接近、共享通用绘制点位或存在未解决歧义时返回 none。

五、只提交 {"id":"<基线目录 ID>"} 或 {"id":"none"}。不可从下一帧、resync 或缺失的结束事件推算真实边界。帧间隔不视为线程阻塞时间。`

const traceComparisonFrameSelectionTeaching = "FRAME SELECTION FOR TRACE COMPARISON: when given one target frame and a baseline without a specified frame, use trace_query comparison.target_frame_vsync with two frame_catalog sides scoped to each operation interval. Provide the current application thread when known. Codrax can find the baseline emitter from the observed current thread label. Codrax builds and ranks its own baseline candidates. Treat reference_only as a sample. For an investigation request, run the prepared comparison query returned by frame selection to measure both frames using their exact source, TID and boundaries. Read each side's coverage separately. For a selection-only request, stop after selecting the frame. If selection is unresolved, keep the baseline frame unresolved and continue with any independently supported scope. Investigate alternatives only when new trace evidence distinguishes them. The current streaming frame_catalog enumerates numbered doFrame markers; use independently verified platform-native frame evidence where that lane cannot enumerate the requested frames. For named business spans, follow the comparison guidance below. " + TraceFrameMatchingSelectionTeaching + " "
