# 客户侧 tracediag 采集配方(customer kits)

远程客户环境采集 typed 证据的现成配方(§29.104.9 战役资产,2026-07-15):
- `collect_case1_main/drill.yaml` — 全量五步形(rank+census+卫星段+边census+blocking);复合视图 1000 行帽下宽窗会截断,先跑再看报尾「输出行=X/Y」自检,截断则用 slim 变体收窄窗。
- `collect_case1_slim_*.yaml` — 瘦身形:每窗单步 root_cause_rank(报文已内嵌边 census),多窗合一文件,配 README 的 awk 瘦身命令可再砍 85% 体积(外发受限场景)。
- `collect_case2_span_rows.yaml` — event_search span 原文行专用(极小,<6KB)。
- `collect_case2_timeline_h1h2.yaml` — 线程逐段账半窗形(可选件)。
命令形:`./codrax --tracediag <yaml> --trace <file> [--trace-window a..b --trace-tid N] --out report.txt`。窗口阶梯与保真切片方法见战役账本 §29.104.9 与 customlogs 交付包说明书。
