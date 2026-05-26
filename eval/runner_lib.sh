#!/usr/bin/env bash

# Shared helpers for eval runners. Kept bash-3.2 compatible because
# macOS still ships that shell.

eval_run_with_timeout() {
  local seconds="$1"
  shift
  # Prefer one cross-platform implementation over platform-specific
  # timeout(1) variants. The Python path starts the command in a fresh
  # process group and always tears down the whole group on timeout, so
  # a timed-out eval cannot leave LLM/tool grandchildren occupying a
  # parallel slot after the worker shell exits.
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$seconds" "$@" <<'PY'
import os
import signal
import subprocess
import sys
import time

timeout = float(sys.argv[1])
cmd = sys.argv[2:]
p = subprocess.Popen(cmd, start_new_session=True)
deadline = time.time() + timeout

def kill_group(sig):
    try:
        os.killpg(p.pid, sig)
    except ProcessLookupError:
        pass

try:
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(rc)
        if time.time() >= deadline:
            break
        time.sleep(min(0.5, max(0.0, deadline - time.time())))
    kill_group(signal.SIGTERM)
    grace_deadline = time.time() + 10
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(124)
        if time.time() >= grace_deadline:
            break
        time.sleep(min(0.25, max(0.0, grace_deadline - time.time())))
    kill_group(signal.SIGKILL)
    p.wait()
    sys.exit(124)
except KeyboardInterrupt:
    kill_group(signal.SIGTERM)
    raise
PY
    return $?
  fi
  if command -v timeout >/dev/null 2>&1; then
    command timeout -k 10 "$seconds" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    command gtimeout -k 10 "$seconds" "$@"
    return $?
  fi
  "$@"
}

eval_latest_result_dir() {
  local results_root="$1"
  local case_id="$2"
  local sweep_start="$3"
  local d ts
  for d in $(ls -dt "${results_root}/${case_id}"-* 2>/dev/null); do
    ts=$(basename "$d" | sed "s/${case_id}-//")
    if [[ "$ts" > "$sweep_start" ]] || [[ "$ts" == "$sweep_start"* ]]; then
      echo "$d"
      return 0
    fi
  done
  return 1
}

eval_metric_field() {
  local file="$1"
  local key="$2"
  local v
  v=$(grep -oE "^${key}=[0-9]+" "$file" 2>/dev/null | head -1 | cut -d= -f2)
  echo "${v:--}"
}

eval_count_pattern() {
  local pattern="$1"
  local file="$2"
  local n
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  n=$(grep -aE -c "$pattern" "$file" 2>/dev/null) || n=0
  echo "${n:-0}"
}

eval_max_context_tokens_estimate() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_tokens() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_window=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_pct() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      tok = 0
      win = 0
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          tok = a[2] + 0
        } else if ($i ~ /^context_window=[0-9]+$/) {
          split($i, b, "=")
          win = b[2] + 0
        }
      }
      if (tok > 0 && win > 0) {
        pct = int((tok * 100 / win) + 0.5)
        if (pct > max) {
          max = pct
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_count_control_pattern() {
  local pattern="$1"
  local file="$2"
  # Control-plane metrics must not match source snippets, model answers, or
  # customer-log payloads. Codrax debug logs are timestamped; requiring that
  # prefix keeps quoted control-looking text out of retry/rewrite counters.
  eval_count_pattern "^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]* ${pattern}" "$file"
}

eval_count_finalizer_rejects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local tool render n
  # Count control-plane events only. Whole-log grep of these strings is unsafe:
  # source snippets and model answers may discuss `finalizer_rejects` or quote
  # customer logs that contain "成文校验未通过".
  tool=$(eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolresult TOOLRESULT emit_answer_document(_patch)? ok=false' "$file")
  render=$(eval_count_control_pattern 'INFO \[render\][[:space:]]+•[[:space:]]+成文[校交]验未通过' "$file")
  n=$((tool + render))
  echo "$n"
}

eval_count_finalizer_rewrites() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[render\][[:space:]]+⟳ 4/4 .*(答案待完善|正在重写答案|检测到 .*前后不一致)' "$file"
}

eval_count_answer_document_patch_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolcall [^:]*tool=emit_answer_document_patch( |$)' "$file"
}

eval_count_midloop_injects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=midloop_inject' "$file"
}

eval_count_tool_calls() {
  local file="$1"
  local tool="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$tool" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolcall [^:]*tool=${tool}( |$)" "$file"
}

eval_count_source_inventory_tool_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Count actual repo_map tool calls only. Prompt text, model answers, and
  # customer logs can mention source_inventory without the model using it.
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=repo_map params=.*"view"[[:space:]]*:[[:space:]]*"source_inventory"' "$file"
}

eval_count_repo_lens_discovery_hints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repo_lens\] discovery_hint ' "$file"
}

eval_count_transient_retry_checkpoints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Counts only the orchestrator control-plane event emitted when a preserved
  # explore-state checkpoint is installed after a stream-level retry. Prompt
  # text and quoted customer logs can mention the same phrase and must not
  # inflate this metric.
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=transient_retry_checkpoint [^:]*installed=true' "$file"
}

eval_sum_answer_contract_violations() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_violations_for_section() {
  local file="$1"
  local section="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$section" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section="$section" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      matched = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "section=" section) {
          matched = 1
        }
      }
      if (!matched) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_count_agent_iterations() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*ASSISTANT content_len=" "$file"
}

eval_count_agent_dispatches() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*DISPATCH stage=" "$file"
}

eval_count_semantic_quality_dispatches() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*verdict' "$file"
}

eval_count_semantic_quality_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*(emitted [1-9]|verdict sufficient=false)' "$file"
}

eval_count_self_consistency_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local reviewer orchestrator n
  reviewer=$(eval_count_control_pattern 'INFO \[self_consistency_reviewer\].*(emitted|consistent=false)' "$file")
  orchestrator=$(eval_count_control_pattern 'DEBUG \[orchestrator\].*self_contradiction' "$file")
  n=$((reviewer + orchestrator))
  echo "$n"
}

eval_running_jobs() {
  jobs -rp | wc -l | tr -d ' '
}

eval_wait_for_slot() {
  local parallel="$1"
  while [[ "$(eval_running_jobs)" -ge "$parallel" ]]; do
    sleep 1
  done
}
