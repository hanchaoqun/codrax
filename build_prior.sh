#!/bin/bash
# Construct the REPL dispatch string by concatenating:
#   "## Prior conversation\n" + BuildContext output + "\n\n## Current request\n" + line
# Then run codrax single-shot with it as --request.
# This mimics REPL dispatch without needing a TTY.

cat > /tmp/codrax_req.txt <<'EOF'
## Prior conversation
### Recent conversation
- You (13:46:34): 有几个pipeline状态，分别详细展开讲讲
  Codrax: 系统中的pipeline状态涉及协调器驱动的4阶段拓扑和任务状态的管理。关键定义如"Orchestrator drives 4-stage topology"和"TaskState captures PipelineStage"帮助描绘这个复杂结构。init函数设置访问限制，虽然多文件被分析，具体符号仍不明确。 引用： •  internal/orchestrator/orchestrator.go:20  — Orchestrator drives 4-stage topology •  internal/types/context.go:9  — TaskState captures PipelineStage -------- *由于未确定的pipeline状态符号，解释可能不完整。
- You (13:48:25): repomap的都提供了那些公开接口
  Codrax: (previous attempt ended in error — details omitted from memory)
- You (13:49:25): repomap的作用是啥
  Codrax: (previous attempt ended in error — details omitted from memory)

## Current request
repomap的作用
EOF

./codrax --log-level debug --request "$(cat /tmp/codrax_req.txt)" 2>repro_stderr.log 1>repro_stdout.log
echo "exit=$?"
